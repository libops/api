package terraform

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"time"

	"google.golang.org/api/idtoken"
)

// TerraformRunner executes terraform runs by calling the API for data
type TerraformRunner struct {
	apiClient    *APIClient
	terraformDir string
}

// NewTerraformRunner creates a new terraform runner
func NewTerraformRunner(apiBaseURL string, terraformDir string) *TerraformRunner {
	return &TerraformRunner{
		apiClient:    NewAPIClient(apiBaseURL),
		terraformDir: terraformDir,
	}
}

// TerraformRunDetails represents run details from API
type TerraformRunDetails struct {
	RunID          string   `json:"run_id"`
	Modules        []string `json:"modules"`
	OrganizationID *int64   `json:"organization_id"`
	ProjectID      *int64   `json:"project_id"`
	SiteID         *int64   `json:"site_id"`
}

// ExecuteTerraformRun executes a terraform run
func (r *TerraformRunner) ExecuteTerraformRun(ctx context.Context, runID string) error {
	slog.Info("starting terraform execution", "run_id", runID)

	// 1. Call API: GetReconciliationRun to get run details
	runDetails, err := r.apiClient.GetReconciliationRun(ctx, runID)
	if err != nil {
		return fmt.Errorf("failed to get run details: %w", err)
	}

	// 2. Call API: UpdateReconciliationStatus to 'running'
	if err := r.apiClient.UpdateReconciliationStatus(ctx, runID, "running", ""); err != nil {
		return fmt.Errorf("failed to update status to running: %w", err)
	}

	// 3. Call API: GenerateTerraformVars to get tfvars JSON
	tfvarsJSON, err := r.apiClient.GenerateTerraformVars(ctx,
		runDetails.OrganizationID,
		runDetails.ProjectID,
		runDetails.SiteID)
	if err != nil {
		return fmt.Errorf("failed to generate terraform vars: %w", err)
	}

	// 4. Execute terraform apply for each module
	for _, module := range runDetails.Modules {
		slog.Info("executing terraform module",
			"run_id", runID,
			"module", module)

		output, exitCode, err := r.executeModule(ctx, module, tfvarsJSON)
		if err != nil || exitCode != 0 {
			errorMsg := fmt.Sprintf("terraform failed (exit %d): %s", exitCode, output)
			slog.Error("terraform module failed",
				"run_id", runID,
				"module", module,
				"exit_code", exitCode,
				"output", output)

			// Update status to failed
			if err := r.apiClient.UpdateReconciliationStatus(ctx, runID, "failed", errorMsg); err != nil {
				slog.Error("failed to update status", "error", err)
			}

			return fmt.Errorf("%s", errorMsg)
		}

		slog.Info("terraform module completed",
			"run_id", runID,
			"module", module)
	}

	// 5. All modules succeeded - update status to completed
	if err := r.apiClient.UpdateReconciliationStatus(ctx, runID, "completed", ""); err != nil {
		return fmt.Errorf("failed to update status to completed: %w", err)
	}

	slog.Info("terraform execution completed successfully",
		"run_id", runID,
		"modules", runDetails.Modules)

	return nil
}

// executeModule executes a single terraform module
// Returns: (output, exitCode, error)
func (r *TerraformRunner) executeModule(ctx context.Context, module string, tfvarsJSON json.RawMessage) (string, int, error) {
	moduleDir := fmt.Sprintf("%s/modules/%s", r.terraformDir, module)

	// Write tfvars to temporary file
	tfvarsFile := fmt.Sprintf("%s/terraform.tfvars.json", moduleDir)
	if err := os.WriteFile(tfvarsFile, tfvarsJSON, 0644); err != nil {
		return "", 1, fmt.Errorf("failed to write tfvars: %w", err)
	}
	defer os.Remove(tfvarsFile)

	// Run terraform init
	initCmd := exec.CommandContext(ctx, "terraform", "init")
	initCmd.Dir = moduleDir
	initOutput, err := initCmd.CombinedOutput()
	if err != nil {
		return string(initOutput), 1, fmt.Errorf("terraform init failed: %w", err)
	}

	// Run terraform apply
	applyCmd := exec.CommandContext(ctx, "terraform", "apply", "-auto-approve", "-var-file=terraform.tfvars.json")
	applyCmd.Dir = moduleDir
	applyOutput, err := applyCmd.CombinedOutput()

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	return string(applyOutput), exitCode, nil
}

// APIClient handles HTTP calls to the main API
type APIClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewAPIClient creates a new API client
func NewAPIClient(baseURL string) *APIClient {
	return &APIClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// GetReconciliationRun fetches run details from API
func (c *APIClient) GetReconciliationRun(ctx context.Context, runID string) (*TerraformRunDetails, error) {
	endpoint := fmt.Sprintf("%s/admin/reconciliation/runs/%s", c.baseURL, runID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	// Add authentication using Google ADC
	if err := c.addAuth(ctx, req); err != nil {
		return nil, fmt.Errorf("failed to add auth: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var runDetails TerraformRunDetails
	if err := json.NewDecoder(resp.Body).Decode(&runDetails); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &runDetails, nil
}

// UpdateReconciliationStatus updates run status via API
func (c *APIClient) UpdateReconciliationStatus(ctx context.Context, runID, status, errorMsg string) error {
	endpoint := fmt.Sprintf("%s/admin/reconciliation/runs/%s/status", c.baseURL, runID)

	payload := map[string]string{
		"status":        status,
		"error_message": errorMsg,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	// Add authentication
	if err := c.addAuth(ctx, req); err != nil {
		return fmt.Errorf("failed to add auth: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	return nil
}

// GenerateTerraformVars fetches terraform variables from API
func (c *APIClient) GenerateTerraformVars(ctx context.Context, orgID, projectID, siteID *int64) (json.RawMessage, error) {
	endpoint := fmt.Sprintf("%s/admin/reconciliation/generate-tfvars", c.baseURL)

	payload := map[string]*int64{
		"organization_id": orgID,
		"project_id":      projectID,
		"site_id":         siteID,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	// Add authentication
	if err := c.addAuth(ctx, req); err != nil {
		return nil, fmt.Errorf("failed to add auth: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var result struct {
		TFVars json.RawMessage `json:"tfvars"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.TFVars, nil
}

// addAuth adds Google Application Default Credentials to request
func (c *APIClient) addAuth(ctx context.Context, req *http.Request) error {
	// Get ID token for service account
	// This uses Application Default Credentials (ADC)
	tokenSource, err := idtoken.NewTokenSource(ctx, c.baseURL)
	if err != nil {
		// If ADC fails, log warning but continue (for local dev)
		slog.Warn("failed to get ID token, continuing without auth", "error", err)
		return nil
	}

	token, err := tokenSource.Token()
	if err != nil {
		slog.Warn("failed to get token from source", "error", err)
		return nil
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token.AccessToken))
	return nil
}
