package terraform

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
)

// Manager executes terraform commands
type Manager struct {
	workingDir  string // Main terraform directory (contains main.tf)
	stateBucket string
	dryRun      bool // If true, only run plan, don't apply (for LIBOPS_ENV=development)
}

// ApplyResult contains the result of a terraform apply
type ApplyResult struct {
	Success bool
	Output  string
	Outputs map[string]interface{}
	DryRun  bool
}

// NewManager creates a new terraform manager
func NewManager(workingDir, stateBucket string) *Manager {
	// Check if we're in development mode
	isDev := os.Getenv("LIBOPS_ENV") == "development"

	return &Manager{
		workingDir:  workingDir,
		stateBucket: stateBucket,
		dryRun:      isDev,
	}
}

// NewManagerWithDryRun creates a new terraform manager with explicit dry-run mode
func NewManagerWithDryRun(workingDir, stateBucket string, dryRun bool) *Manager {
	return &Manager{
		workingDir:  workingDir,
		stateBucket: stateBucket,
		dryRun:      dryRun,
	}
}

// Apply runs terraform apply with optional targeting
// If targets is empty, runs full apply. Otherwise targets specific modules.
func (m *Manager) Apply(ctx context.Context, tfvarsJSON string, targets []string) (*ApplyResult, error) {
	slog.Info("running terraform apply",
		"working_dir", m.workingDir,
		"targets", targets)

	// Write tfvars.json
	tfvarsPath := filepath.Join(m.workingDir, "terraform.tfvars.json")
	if err := os.WriteFile(tfvarsPath, []byte(tfvarsJSON), 0644); err != nil {
		return nil, fmt.Errorf("failed to write tfvars: %w", err)
	}

	// Run terraform init
	initArgs := []string{"init", "-backend-config=bucket=" + m.stateBucket}
	if err := m.runCommand(ctx, m.workingDir, "terraform", initArgs...); err != nil {
		return nil, fmt.Errorf("terraform init failed: %w", err)
	}

	// Build plan arguments
	planFile := filepath.Join(m.workingDir, "tfplan")
	planArgs := []string{"plan", "-out=" + planFile}

	// Add targets if specified
	for _, target := range targets {
		planArgs = append(planArgs, "-target="+target)
	}

	// Run terraform plan
	planOutput, err := m.runCommandWithOutput(ctx, m.workingDir, "terraform", planArgs...)
	if err != nil {
		return &ApplyResult{
			Success: false,
			Output:  planOutput,
			DryRun:  m.dryRun,
		}, fmt.Errorf("terraform plan failed: %w", err)
	}

	// If in dry-run mode, only return plan results
	if m.dryRun {
		slog.Info("DRY-RUN MODE: Skipping terraform apply (LIBOPS_ENV=development)",
			"working_dir", m.workingDir,
			"targets", targets)
		return &ApplyResult{
			Success: true,
			Output:  planOutput + "\n\n[DRY-RUN MODE] Would have applied the above plan",
			Outputs: make(map[string]interface{}),
			DryRun:  true,
		}, nil
	}

	// Run terraform apply
	applyOutput, err := m.runCommandWithOutput(ctx, m.workingDir, "terraform", "apply", "-auto-approve", planFile)
	if err != nil {
		return &ApplyResult{
			Success: false,
			Output:  applyOutput,
			DryRun:  false,
		}, fmt.Errorf("terraform apply failed: %w", err)
	}

	// Get terraform outputs
	outputs, err := m.getOutputs(ctx, m.workingDir)
	if err != nil {
		return &ApplyResult{
			Success: false,
			Output:  applyOutput,
			DryRun:  false,
		}, fmt.Errorf("failed to get outputs: %w", err)
	}

	return &ApplyResult{
		Success: true,
		Output:  applyOutput,
		Outputs: outputs,
		DryRun:  false,
	}, nil
}

// ApplyOrganization runs terraform for an organization
func (m *Manager) ApplyOrganization(ctx context.Context, tfvarsJSON, orgPublicID string) (*ApplyResult, error) {
	targets := []string{
		fmt.Sprintf("module.organizations[\"%s\"]", orgPublicID),
	}
	return m.Apply(ctx, tfvarsJSON, targets)
}

// ApplyProject runs terraform for a project
func (m *Manager) ApplyProject(ctx context.Context, tfvarsJSON, projectPublicID string) (*ApplyResult, error) {
	targets := []string{
		fmt.Sprintf("module.projects[\"%s\"]", projectPublicID),
	}
	return m.Apply(ctx, tfvarsJSON, targets)
}

// ApplySite runs terraform for a site
func (m *Manager) ApplySite(ctx context.Context, tfvarsJSON, sitePublicID string) (*ApplyResult, error) {
	targets := []string{
		fmt.Sprintf("module.sites[\"%s\"]", sitePublicID),
	}
	return m.Apply(ctx, tfvarsJSON, targets)
}

// ApplyAllSites runs terraform for all sites (useful when org/project firewall changes)
func (m *Manager) ApplyAllSites(ctx context.Context, tfvarsJSON string, sitePublicIDs []string) (*ApplyResult, error) {
	targets := make([]string, len(sitePublicIDs))
	for i, siteID := range sitePublicIDs {
		targets[i] = fmt.Sprintf("module.sites[\"%s\"]", siteID)
	}
	return m.Apply(ctx, tfvarsJSON, targets)
}

// runCommand runs a command and logs output
func (m *Manager) runCommand(ctx context.Context, dir string, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	cmdStr := name + " " + fmt.Sprint(args)
	slog.Debug("running command", "cmd", cmdStr, "dir", dir)

	return cmd.Run()
}

// runCommandWithOutput runs a command and returns output as string
func (m *Manager) runCommandWithOutput(ctx context.Context, dir string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir

	output, err := cmd.CombinedOutput()
	return string(output), err
}

// getOutputs gets terraform outputs as JSON
func (m *Manager) getOutputs(ctx context.Context, dir string) (map[string]interface{}, error) {
	cmd := exec.CommandContext(ctx, "terraform", "output", "-json")
	cmd.Dir = dir

	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var rawOutputs map[string]interface{}
	if err := json.Unmarshal(output, &rawOutputs); err != nil {
		return nil, err
	}

	// Terraform output format: {"key": {"value": "actual_value", "type": "string"}}
	// Extract just the values
	outputs := make(map[string]interface{})
	for key, val := range rawOutputs {
		if m, ok := val.(map[string]interface{}); ok {
			if value, ok := m["value"]; ok {
				outputs[key] = value
			}
		}
	}

	return outputs, nil
}
