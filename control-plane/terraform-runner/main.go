// Package main implements the LibOps Terraform Runner
// This runs as a Cloud Run Job in customer projects to execute terraform
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"google.golang.org/api/idtoken"
)

// Config holds terraform runner configuration
type Config struct {
	// API URL to fetch run details
	APIURL string

	// API audience for JWT authentication
	APIAudience string

	// Run ID to execute
	RunID string

	// Terraform workspace directory
	WorkspaceDir string

	// GCS bucket for terraform state
	StateBucket string

	// Service account email for authentication
	ServiceAccount string

	// Bootstrap mode
	Bootstrap bool
}

// ReconciliationRun represents a terraform run
type ReconciliationRun struct {
	RunID              string   `json:"run_id"`
	RunType            string   `json:"run_type"`
	ReconciliationType *string  `json:"reconciliation_type,omitempty"`
	Modules            []string `json:"modules"`
	TargetSiteIDs      []string `json:"target_site_ids"`
	EventIDs           []string `json:"event_ids"`
	OrganizationID     *int64   `json:"organization_id,omitempty"`
	ProjectID          *int64   `json:"project_id,omitempty"`
	SiteID             *int64   `json:"site_id,omitempty"`
	Status             string   `json:"status"`
}

// TerraformVarsResponse from API
type TerraformVarsResponse struct {
	TfvarsJSON string `json:"tfvars_json"`
}

func main() {
	// Setup structured logging
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	slog.Info("starting terraform runner")

	// Load configuration
	config := loadConfig()

	// Validate configuration
	if err := validateConfig(config); err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	// Create context with timeout (Cloud Run Jobs have max 60 minutes)
	ctx, cancel := context.WithTimeout(context.Background(), 55*time.Minute)
	defer cancel()

	// Run terraform
	if err := runTerraform(ctx, config); err != nil {
		slog.Error("terraform execution failed", "error", err)
		os.Exit(1)
	}

	slog.Info("terraform execution completed successfully")
}

// loadConfig loads configuration from environment variables
func loadConfig() *Config {
	apiURL := os.Getenv("LIBOPS_API_URL")
	if apiURL == "" {
		// Try PSC endpoint
		pscIP := os.Getenv("LIBOPS_PSC_IP")
		if pscIP != "" {
			apiURL = fmt.Sprintf("https://%s", pscIP)
		} else {
			apiURL = "https://psc-api.libops.internal"
		}
	}

	apiAudience := os.Getenv("LIBOPS_API_AUDIENCE")
	if apiAudience == "" {
		apiAudience = "https://api.libops.io"
	}

	workspaceDir := os.Getenv("TERRAFORM_WORKSPACE")
	if workspaceDir == "" {
		workspaceDir = "/workspace/terraform"
	}

	stateBucket := os.Getenv("TERRAFORM_STATE_BUCKET")
	if stateBucket == "" {
		stateBucket = "libops-terraform-state"
	}

	return &Config{
		APIURL:         apiURL,
		APIAudience:    apiAudience,
		RunID:          os.Getenv("RUN_ID"),
		WorkspaceDir:   workspaceDir,
		StateBucket:    stateBucket,
		ServiceAccount: os.Getenv("SERVICE_ACCOUNT"),
		Bootstrap:      os.Getenv("BOOTSTRAP") == "true",
	}
}

// validateConfig validates configuration
func validateConfig(config *Config) error {
	if config.RunID == "" {
		return fmt.Errorf("RUN_ID environment variable is required")
	}
	if config.APIURL == "" {
		return fmt.Errorf("LIBOPS_API_URL or LIBOPS_PSC_IP must be set")
	}
	if config.StateBucket == "" {
		return fmt.Errorf("TERRAFORM_STATE_BUCKET is required")
	}
	return nil
}

// runTerraform executes the terraform workflow
func runTerraform(ctx context.Context, config *Config) error {
	// 1. Update status to 'running'
	if err := updateStatus(ctx, config, "running", nil); err != nil {
		return fmt.Errorf("failed to update status to running: %w", err)
	}

	// 2. Fetch reconciliation run details
	run, err := fetchReconciliationRun(ctx, config)
	if err != nil {
		updateStatus(ctx, config, "failed", err)
		return fmt.Errorf("failed to fetch reconciliation run: %w", err)
	}

	slog.Info("fetched reconciliation run",
		"run_id", run.RunID,
		"run_type", run.RunType,
		"modules", run.Modules,
		"bootstrap", config.Bootstrap)

	// 3. Generate terraform vars
	tfvarsJSON, err := generateTerraformVars(ctx, config, run)
	if err != nil {
		updateStatus(ctx, config, "failed", err)
		return fmt.Errorf("failed to generate terraform vars: %w", err)
	}

	// 4. Write tfvars to file
	tfvarsPath := filepath.Join(config.WorkspaceDir, "terraform.tfvars.json")
	if err := os.WriteFile(tfvarsPath, []byte(tfvarsJSON), 0644); err != nil {
		updateStatus(ctx, config, "failed", err)
		return fmt.Errorf("failed to write tfvars file: %w", err)
	}

	slog.Info("wrote terraform vars", "path", tfvarsPath)

	if config.Bootstrap {
		// Bootstrap flow
		slog.Info("starting bootstrap flow")

		// Disable GCS backend to use local state initially
		if err := disableBackend(config.WorkspaceDir); err != nil {
			updateStatus(ctx, config, "failed", err)
			return fmt.Errorf("failed to disable backend: %w", err)
		}

		// Init without backend config (local)
		if err := terraformInit(ctx, config, false); err != nil {
			updateStatus(ctx, config, "failed", err)
			return fmt.Errorf("terraform init (local) failed: %w", err)
		}

		// Plan
		if err := terraformPlan(ctx, config, run); err != nil {
			updateStatus(ctx, config, "failed", err)
			return fmt.Errorf("terraform plan (local) failed: %w", err)
		}

		// Apply (creates bucket)
		if err := terraformApply(ctx, config, run); err != nil {
			updateStatus(ctx, config, "failed", err)
			return fmt.Errorf("terraform apply (local) failed: %w", err)
		}

		// Enable GCS backend
		if err := enableBackend(config.WorkspaceDir); err != nil {
			updateStatus(ctx, config, "failed", err)
			return fmt.Errorf("failed to enable backend: %w", err)
		}

		// Init with migration to GCS
		slog.Info("migrating state to GCS bucket")
		if err := terraformInit(ctx, config, true, "-migrate-state", "-force-copy"); err != nil {
			updateStatus(ctx, config, "failed", err)
			return fmt.Errorf("terraform init (migrate) failed: %w", err)
		}

	} else {
		// Standard flow

		// 5. Initialize terraform
		if err := terraformInit(ctx, config, true); err != nil {
			updateStatus(ctx, config, "failed", err)
			return fmt.Errorf("terraform init failed: %w", err)
		}

		// 6. Run terraform plan
		if err := terraformPlan(ctx, config, run); err != nil {
			updateStatus(ctx, config, "failed", err)
			return fmt.Errorf("terraform plan failed: %w", err)
		}

		// 7. Run terraform apply
		if err := terraformApply(ctx, config, run); err != nil {
			updateStatus(ctx, config, "failed", err)
			return fmt.Errorf("terraform apply failed: %w", err)
		}
	}

	// 8. Update status to 'completed'
	if err := updateStatus(ctx, config, "completed", nil); err != nil {
		return fmt.Errorf("failed to update status to completed: %w", err)
	}

	return nil
}

// fetchReconciliationRun fetches run details from API
func fetchReconciliationRun(ctx context.Context, config *Config) (*ReconciliationRun, error) {
	url := fmt.Sprintf("%s/admin/v1/reconciliations/%s", config.APIURL, config.RunID)

	token, err := getIDToken(ctx, config.APIAudience)
	if err != nil {
		return nil, fmt.Errorf("failed to get ID token: %w", err)
	}

	// Execute HTTP request
	cmd := exec.CommandContext(ctx, "curl", "-s", "-H", fmt.Sprintf("Authorization: Bearer %s", token), url)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch run: %w", err)
	}

	var run ReconciliationRun
	if err := json.Unmarshal(output, &run); err != nil {
		return nil, fmt.Errorf("failed to parse run response: %w", err)
	}

	return &run, nil
}

// generateTerraformVars generates terraform vars from API
func generateTerraformVars(ctx context.Context, config *Config, run *ReconciliationRun) (string, error) {
	url := fmt.Sprintf("%s/admin/v1/reconciliations/terraform-vars", config.APIURL)

	token, err := getIDToken(ctx, config.APIAudience)
	if err != nil {
		return "", fmt.Errorf("failed to get ID token: %w", err)
	}

	// Build request body
	reqBody := map[string]interface{}{}
	if run.OrganizationID != nil {
		reqBody["organization_id"] = *run.OrganizationID
	}
	if run.ProjectID != nil {
		reqBody["project_id"] = *run.ProjectID
	}
	if run.SiteID != nil {
		reqBody["site_id"] = *run.SiteID
	}

	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// Execute HTTP request
	cmd := exec.CommandContext(ctx, "curl", "-s",
		"-X", "POST",
		"-H", "Content-Type: application/json",
		"-H", fmt.Sprintf("Authorization: Bearer %s", token),
		"-d", string(reqJSON),
		url)

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to generate vars: %w", err)
	}

	var resp TerraformVarsResponse
	if err := json.Unmarshal(output, &resp); err != nil {
		return "", fmt.Errorf("failed to parse vars response: %w", err)
	}

	return resp.TfvarsJSON, nil
}

// terraformInit initializes terraform
func terraformInit(ctx context.Context, config *Config, useBackend bool, extraArgs ...string) error {
	slog.Info("running terraform init", "use_backend", useBackend, "extra_args", extraArgs)

	args := []string{"init"}
	if useBackend {
		args = append(args,
			"-backend-config=bucket="+config.StateBucket,
			"-backend-config=prefix=terraform/state",
		)
	} else {
		args = append(args, "-backend=false")
	}
	args = append(args, extraArgs...)

	cmd := exec.CommandContext(ctx, "terraform", args...)
	cmd.Dir = config.WorkspaceDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("terraform init failed: %w", err)
	}

	return nil
}

// terraformPlan runs terraform plan
func terraformPlan(ctx context.Context, config *Config, run *ReconciliationRun) error {
	slog.Info("running terraform plan")

	args := []string{"plan", "-out=tfplan"}

	// Add targets based on modules
	for _, module := range run.Modules {
		switch module {
		case "organization":
			if run.OrganizationID != nil {
				// Target specific organization module
				args = append(args, fmt.Sprintf("-target=module.organizations[%d]", *run.OrganizationID))
			}
		case "project":
			if run.ProjectID != nil {
				args = append(args, fmt.Sprintf("-target=module.projects[%d]", *run.ProjectID))
			}
		case "site":
			if run.SiteID != nil {
				args = append(args, fmt.Sprintf("-target=module.sites[%d]", *run.SiteID))
			}
		}
	}

	cmd := exec.CommandContext(ctx, "terraform", args...)
	cmd.Dir = config.WorkspaceDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("terraform plan failed: %w", err)
	}

	return nil
}

// terraformApply runs terraform apply
func terraformApply(ctx context.Context, config *Config, run *ReconciliationRun) error {
	slog.Info("running terraform apply")

	cmd := exec.CommandContext(ctx, "terraform", "apply", "-auto-approve", "tfplan")
	cmd.Dir = config.WorkspaceDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("terraform apply failed: %w", err)
	}

	return nil
}

// disableBackend comments out the backend block in main.tf
func disableBackend(dir string) error {
	path := filepath.Join(dir, "main.tf")
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	// We look for the exact backend block string.
	// Note: precise indentation matching is required.
	block := `  backend "gcs" {
    # Bucket configured via -backend-config flag
    prefix = "libops"
  }`
	
	commentedBlock := `/*
  backend "gcs" {
    # Bucket configured via -backend-config flag
    prefix = "libops"
  }
*/`

	newContent := strings.Replace(string(content), block, commentedBlock, 1)
	if newContent == string(content) {
		return fmt.Errorf("backend block not found in main.tf")
	}

	return os.WriteFile(path, []byte(newContent), 0644)
}

// enableBackend restores the backend block in main.tf
func enableBackend(dir string) error {
	path := filepath.Join(dir, "main.tf")
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	block := `  backend "gcs" {
    # Bucket configured via -backend-config flag
    prefix = "libops"
  }`
	
	commentedBlock := `/*
  backend "gcs" {
    # Bucket configured via -backend-config flag
    prefix = "libops"
  }
*/`

	newContent := strings.Replace(string(content), commentedBlock, block, 1)
	if newContent == string(content) {
		return fmt.Errorf("commented backend block not found in main.tf")
	}

	return os.WriteFile(path, []byte(newContent), 0644)
}

// updateStatus updates reconciliation run status in API
func updateStatus(ctx context.Context, config *Config, status string, err error) error {
	url := fmt.Sprintf("%s/admin/v1/reconciliations/%s/status", config.APIURL, config.RunID)

	token, tokenErr := getIDToken(ctx, config.APIAudience)
	if tokenErr != nil {
		slog.Error("failed to get ID token for status update", "error", tokenErr)
		return tokenErr
	}

	reqBody := map[string]interface{}{
		"run_id": config.RunID,
		"status": status,
	}
	if err != nil {
		errMsg := err.Error()
		reqBody["error_message"] = errMsg
	}

	reqJSON, _ := json.Marshal(reqBody)

	// Execute HTTP request
	cmd := exec.CommandContext(ctx, "curl", "-s",
		"-X", "POST",
		"-H", "Content-Type: application/json",
		"-H", fmt.Sprintf("Authorization: Bearer %s", token),
		"-d", string(reqJSON),
		url)

	output, execErr := cmd.CombinedOutput()
	if execErr != nil {
		slog.Error("failed to update status",
			"status", status,
			"error", execErr,
			"output", string(output))
		return execErr
	}

	slog.Info("updated reconciliation status", "status", status)
	return nil
}

// getIDToken gets an ID token from GCP metadata service
func getIDToken(ctx context.Context, audience string) (string, error) {
	ts, err := idtoken.NewTokenSource(ctx, audience)
	if err != nil {
		return "", fmt.Errorf("failed to create token source: %w", err)
	}

	token, err := ts.Token()
	if err != nil {
		return "", fmt.Errorf("failed to get token: %w", err)
	}

	return token.AccessToken, nil
}

// logTerraformOutput logs terraform command output
func logTerraformOutput(output []byte) {
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if line != "" {
			slog.Info("terraform output", "line", line)
		}
	}
}
