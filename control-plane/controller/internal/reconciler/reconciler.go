package reconciler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Reconciler handles VM-level reconciliation of configuration
type Reconciler struct {
	apiURL     string
	siteID     string
	httpClient *http.Client
}

// NewReconciler creates a new VM reconciler
func NewReconciler(apiURL, siteID string) *Reconciler {
	return &Reconciler{
		apiURL: apiURL,
		siteID: siteID,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Member represents a team member with SSH access
type Member struct {
	PublicID string   `json:"public_id"` // UUID used as username
	Name     string   `json:"name"`
	Email    string   `json:"email"`
	SSHKeys  []SSHKey `json:"ssh_keys"`
}

// SSHKey represents an SSH public key
type SSHKey struct {
	PublicKey   string `json:"public_key"`
	Fingerprint string `json:"fingerprint"`
}

// Secret represents a secret key-value pair
type Secret struct {
	ID    string `json:"id"`    // Secret ID for status tracking
	Key   string `json:"key"`
	Value string `json:"value"`
}

// FirewallRule represents a firewall rule
type FirewallRule struct {
	ID       string `json:"id"`       // Firewall rule ID for status tracking
	Protocol string `json:"protocol"`
	Port     int    `json:"port"`
	Source   string `json:"source"`
	Action   string `json:"action"`
}

// Deployment represents deployment configuration
type Deployment struct {
	GitHubRepo     string            `json:"github_repo"`     // e.g., "org/repo"
	GitHubRef      string            `json:"github_ref"`      // e.g., "main" or commit SHA
	GitHubToken    string            `json:"github_token"`    // GitHub access token
	DeploymentPath string            `json:"deployment_path"` // Where to clone/deploy
	ComposeFile    string            `json:"compose_file"`    // docker-compose.yml path
	Environment    map[string]string `json:"environment"`     // Additional env vars
	DeploymentID   string            `json:"deployment_id"`   // Unique deployment ID
	CommitSHA      string            `json:"commit_sha"`      // Commit being deployed
	CommitMessage  string            `json:"commit_message"`  // Commit message
	CommitAuthor   string            `json:"commit_author"`   // Who triggered deployment
}

// ReconcileAll runs all reconciliation types (excluding deployment)
func (r *Reconciler) ReconcileAll(ctx context.Context) error {
	slog.Info("starting full reconciliation", "site_id", r.siteID)

	// Run all reconciliation types
	if err := r.ReconcileSSHKeys(ctx); err != nil {
		slog.Error("SSH key reconciliation failed", "error", err)
		// Continue with other reconciliations
	}

	if err := r.ReconcileSecrets(ctx); err != nil {
		slog.Error("secrets reconciliation failed", "error", err)
		// Continue with other reconciliations
	}

	if err := r.ReconcileFirewall(ctx); err != nil {
		slog.Error("firewall reconciliation failed", "error", err)
		// Continue with other reconciliations
	}

	// Note: Deployment is NOT run on periodic reconciliation
	// It is only triggered manually or via webhook

	// Update check-in timestamp
	if err := r.CheckIn(ctx); err != nil {
		slog.Error("check-in failed", "error", err)
	}

	slog.Info("full reconciliation completed", "site_id", r.siteID)
	return nil
}

// ReconcileSSHKeys fetches SSH keys from API and applies them
func (r *Reconciler) ReconcileSSHKeys(ctx context.Context) error {
	slog.Info("reconciling SSH keys", "site_id", r.siteID)

	// 1. Get VM service account token
	token, err := r.getVMServiceAccountToken(ctx)
	if err != nil {
		return fmt.Errorf("failed to get service account token: %w", err)
	}

	// 2. Fetch members with SSH keys from admin API
	members, err := r.fetchMembers(ctx, token)
	if err != nil {
		return fmt.Errorf("failed to fetch members: %w", err)
	}

	// 3. Reconcile user accounts and SSH keys on host
	if err := r.reconcileMembers(members); err != nil {
		// Report failure
		r.reportReconciliationStatus(ctx, token, "ssh_keys", nil, "failed", err.Error())
		return fmt.Errorf("failed to reconcile members: %w", err)
	}

	// 4. Report successful reconciliation to API (marks members as active)
	memberIDs := make([]string, len(members))
	for i, member := range members {
		memberIDs[i] = member.PublicID
	}
	if err := r.reportReconciliationStatus(ctx, token, "ssh_keys", memberIDs, "active", ""); err != nil {
		slog.Warn("failed to report ssh_keys reconciliation status", "error", err)
		// Don't fail the reconciliation if status reporting fails
	}

	slog.Info("SSH keys reconciled successfully",
		"site_id", r.siteID,
		"member_count", len(members))

	return nil
}

// ReconcileSecrets fetches secrets from API and applies them
func (r *Reconciler) ReconcileSecrets(ctx context.Context) error {
	slog.Info("reconciling secrets", "site_id", r.siteID)

	// 1. Get VM service account token
	token, err := r.getVMServiceAccountToken(ctx)
	if err != nil {
		return fmt.Errorf("failed to get service account token: %w", err)
	}

	// 2. Fetch secrets from admin API
	secrets, err := r.fetchSecrets(ctx, token)
	if err != nil {
		return fmt.Errorf("failed to fetch secrets: %w", err)
	}

	// 3. Apply secrets to .env file
	if err := r.applySecrets(secrets); err != nil {
		// Report failure
		r.reportReconciliationStatus(ctx, token, "secrets", nil, "failed", err.Error())
		return fmt.Errorf("failed to apply secrets: %w", err)
	}

	// 4. Report successful reconciliation to API (marks secrets as active)
	secretIDs := make([]string, len(secrets))
	for i, secret := range secrets {
		secretIDs[i] = secret.ID
	}
	if err := r.reportReconciliationStatus(ctx, token, "secrets", secretIDs, "active", ""); err != nil {
		slog.Warn("failed to report secrets reconciliation status", "error", err)
		// Don't fail the reconciliation if status reporting fails
	}

	slog.Info("secrets reconciled successfully",
		"site_id", r.siteID,
		"secret_count", len(secrets))

	return nil
}

// ReconcileFirewall fetches firewall rules from API and applies them
func (r *Reconciler) ReconcileFirewall(ctx context.Context) error {
	slog.Info("reconciling firewall rules", "site_id", r.siteID)

	// 1. Get VM service account token
	token, err := r.getVMServiceAccountToken(ctx)
	if err != nil {
		return fmt.Errorf("failed to get service account token: %w", err)
	}

	// 2. Fetch firewall rules from admin API
	rules, err := r.fetchFirewallRules(ctx, token)
	if err != nil {
		return fmt.Errorf("failed to fetch firewall rules: %w", err)
	}

	// 3. Apply firewall rules via iptables
	if err := r.applyFirewallRules(rules); err != nil {
		// Report failure
		r.reportReconciliationStatus(ctx, token, "firewall", nil, "failed", err.Error())
		return fmt.Errorf("failed to apply firewall rules: %w", err)
	}

	// 4. Report successful reconciliation to API (marks firewall rules as active)
	ruleIDs := make([]string, len(rules))
	for i, rule := range rules {
		ruleIDs[i] = rule.ID
	}
	if err := r.reportReconciliationStatus(ctx, token, "firewall", ruleIDs, "active", ""); err != nil {
		slog.Warn("failed to report firewall reconciliation status", "error", err)
		// Don't fail the reconciliation if status reporting fails
	}

	slog.Info("firewall rules reconciled successfully",
		"site_id", r.siteID,
		"rule_count", len(rules))

	return nil
}

// ReconcileDeployment fetches deployment config from API and deploys
func (r *Reconciler) ReconcileDeployment(ctx context.Context) error {
	slog.Info("reconciling deployment", "site_id", r.siteID)

	// 1. Get VM service account token
	token, err := r.getVMServiceAccountToken(ctx)
	if err != nil {
		return fmt.Errorf("failed to get service account token: %w", err)
	}

	// 2. Fetch deployment config from admin API
	deployment, err := r.fetchDeployment(ctx, token)
	if err != nil {
		return fmt.Errorf("failed to fetch deployment config: %w", err)
	}

	// 3. Execute deployment
	if err := r.executeDeployment(ctx, deployment); err != nil {
		// Report deployment failure to API (both endpoints)
		r.reportDeploymentStatus(ctx, token, deployment.DeploymentID, "failed", err.Error())
		r.reportReconciliationStatus(ctx, token, "deployment", []string{deployment.DeploymentID}, "failed", err.Error())
		return fmt.Errorf("failed to execute deployment: %w", err)
	}

	// 4. Report deployment success to API
	if err := r.reportDeploymentStatus(ctx, token, deployment.DeploymentID, "success", ""); err != nil {
		slog.Warn("failed to report deployment status to deployment endpoint", "error", err)
	}

	// Also report via generic reconciliation endpoint (marks deployment as active)
	if err := r.reportReconciliationStatus(ctx, token, "deployment", []string{deployment.DeploymentID}, "active", ""); err != nil {
		slog.Warn("failed to report deployment reconciliation status", "error", err)
	}

	slog.Info("deployment reconciled successfully",
		"site_id", r.siteID,
		"deployment_id", deployment.DeploymentID,
		"commit_sha", deployment.CommitSHA)

	return nil
}

// CheckIn updates the site's check-in timestamp
func (r *Reconciler) CheckIn(ctx context.Context) error {
	// Get VM service account token
	token, err := r.getVMServiceAccountToken(ctx)
	if err != nil {
		return fmt.Errorf("failed to get service account token: %w", err)
	}

	endpoint := fmt.Sprintf("%s/admin/sites/%s/checkin", r.apiURL, r.siteID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to call check-in endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("check-in returned status %d: %s", resp.StatusCode, string(body))
	}

	slog.Debug("check-in successful", "site_id", r.siteID)
	return nil
}

// getVMServiceAccountToken fetches JWT from Google metadata server
func (r *Reconciler) getVMServiceAccountToken(ctx context.Context) (string, error) {
	endpoint := "http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Metadata-Flavor", "Google")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch token from metadata server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("metadata server returned status %d", resp.StatusCode)
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		TokenType   string `json:"token_type"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("failed to decode token response: %w", err)
	}

	return tokenResp.AccessToken, nil
}

// fetchMembers fetches members with SSH keys from admin API
func (r *Reconciler) fetchMembers(ctx context.Context, token string) ([]Member, error) {
	endpoint := fmt.Sprintf("%s/admin/sites/%s/members", r.apiURL, r.siteID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch members: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Members []Member `json:"members"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Members, nil
}

// fetchFirewallRules fetches firewall rules from admin API
func (r *Reconciler) fetchFirewallRules(ctx context.Context, token string) ([]FirewallRule, error) {
	endpoint := fmt.Sprintf("%s/admin/sites/%s/firewall", r.apiURL, r.siteID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch firewall rules: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Rules []FirewallRule `json:"rules"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Rules, nil
}

// fetchSecrets fetches secrets from admin API
func (r *Reconciler) fetchSecrets(ctx context.Context, token string) ([]Secret, error) {
	endpoint := fmt.Sprintf("%s/admin/sites/%s/secrets", r.apiURL, r.siteID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch secrets: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Secrets []Secret `json:"secrets"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Secrets, nil
}

// reconcileMembers ensures user accounts exist on host and SSH keys are configured
func (r *Reconciler) reconcileMembers(members []Member) error {
	// 1. Get list of existing users from host
	existingUsers, err := r.getExistingLibOpsUsers()
	if err != nil {
		return fmt.Errorf("failed to get existing users: %w", err)
	}

	// 2. Track desired users
	desiredUsers := make(map[string]bool)
	for _, member := range members {
		desiredUsers[member.PublicID] = true
	}

	// 3. Create/update users and SSH keys
	for _, member := range members {
		username := member.PublicID

		// Check if user exists on host
		userExists := existingUsers[username]

		if !userExists {
			// Create user on host
			slog.Info("creating user account on host", "username", username, "name", member.Name)
			if err := r.createUserOnHost(username, member.Name); err != nil {
				slog.Error("failed to create user", "username", username, "error", err)
				continue
			}
		}

		// Update SSH keys for user
		if err := r.updateUserSSHKeys(username, member.SSHKeys); err != nil {
			slog.Error("failed to update SSH keys", "username", username, "error", err)
			continue
		}

		slog.Debug("reconciled user", "username", username, "key_count", len(member.SSHKeys))
	}

	// 4. Remove users that are no longer members
	for username := range existingUsers {
		if !desiredUsers[username] {
			slog.Info("removing user account from host", "username", username)
			if err := r.deleteUserFromHost(username); err != nil {
				slog.Error("failed to delete user", "username", username, "error", err)
			}
		}
	}

	return nil
}

// getExistingLibOpsUsers returns map of LibOps-managed users (UUIDs) on the host
func (r *Reconciler) getExistingLibOpsUsers() (map[string]bool, error) {
	users := make(map[string]bool)

	// Read /etc/passwd to find users with home dirs matching UUID pattern
	cmd := exec.Command("getent", "passwd")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to read passwd: %w", err)
	}

	// Parse passwd entries and find UUID-named users
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		fields := strings.Split(line, ":")
		if len(fields) < 6 {
			continue
		}

		username := fields[0]
		homeDir := fields[5]

		// Check if username matches UUID pattern and home is /home/<uuid>
		if r.isUUID(username) && homeDir == fmt.Sprintf("/home/%s", username) {
			users[username] = true
		}
	}

	return users, nil
}

// isUUID checks if string matches UUID pattern
func (r *Reconciler) isUUID(s string) bool {
	// Simple UUID v4 pattern check: 8-4-4-4-12 hex digits
	if len(s) != 36 {
		return false
	}
	parts := strings.Split(s, "-")
	if len(parts) != 5 {
		return false
	}
	return len(parts[0]) == 8 && len(parts[1]) == 4 && len(parts[2]) == 4 &&
		len(parts[3]) == 4 && len(parts[4]) == 12
}

// createUserOnHost creates a user account on the host system
func (r *Reconciler) createUserOnHost(username, fullName string) error {
	// Use adduser to create user
	// --disabled-password: no password login (SSH key only)
	// --gecos: set full name without prompting
	cmd := exec.Command("adduser", "--disabled-password", "--gecos", fullName, username)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("adduser failed: %s: %w", string(output), err)
	}

	// Add user to docker group for container access if needed
	cmd = exec.Command("usermod", "-aG", "docker", username)
	if err := cmd.Run(); err != nil {
		slog.Warn("failed to add user to docker group", "username", username, "error", err)
	}

	return nil
}

// deleteUserFromHost removes a user account from the host system
func (r *Reconciler) deleteUserFromHost(username string) error {
	// Use deluser to remove user and home directory
	cmd := exec.Command("deluser", "--remove-home", username)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("deluser failed: %s: %w", string(output), err)
	}

	return nil
}

// updateUserSSHKeys writes SSH keys to user's authorized_keys file
func (r *Reconciler) updateUserSSHKeys(username string, keys []SSHKey) error {
	homeDir := fmt.Sprintf("/home/%s", username)
	sshDir := fmt.Sprintf("%s/.ssh", homeDir)
	authorizedKeysPath := fmt.Sprintf("%s/authorized_keys", sshDir)

	// Build authorized_keys content
	var content strings.Builder
	content.WriteString("# LibOps managed SSH keys - DO NOT EDIT MANUALLY\n")
	content.WriteString(fmt.Sprintf("# Last updated: %s\n\n", time.Now().Format(time.RFC3339)))

	for _, key := range keys {
		content.WriteString(fmt.Sprintf("# Fingerprint: %s\n", key.Fingerprint))
		content.WriteString(key.PublicKey)
		if !strings.HasSuffix(key.PublicKey, "\n") {
			content.WriteString("\n")
		}
		content.WriteString("\n")
	}

	// Create .ssh directory
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		return fmt.Errorf("failed to create .ssh directory: %w", err)
	}

	// Write authorized_keys file atomically
	tempPath := authorizedKeysPath + ".tmp"
	if err := os.WriteFile(tempPath, []byte(content.String()), 0600); err != nil {
		return fmt.Errorf("failed to write temporary file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tempPath, authorizedKeysPath); err != nil {
		return fmt.Errorf("failed to rename temporary file: %w", err)
	}

	// Set ownership
	userGroup := fmt.Sprintf("%s:%s", username, username)
	cmd := exec.Command("chown", "-R", userGroup, sshDir)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to chown .ssh: %w", err)
	}

	return nil
}

// applySecrets writes secrets to environment file
func (r *Reconciler) applySecrets(secrets []Secret) error {
	slog.Info("applying secrets", "secret_count", len(secrets))

	secretsDir := "/etc/libops"
	secretsPath := fmt.Sprintf("%s/secrets.env", secretsDir)

	// Create directory
	if err := os.MkdirAll(secretsDir, 0755); err != nil {
		return fmt.Errorf("failed to create secrets directory: %w", err)
	}

	// Build environment file content
	var content strings.Builder
	content.WriteString("# LibOps Secrets - Auto-generated, do not edit manually\n")
	content.WriteString(fmt.Sprintf("# Generated at: %s\n\n", time.Now().UTC().Format(time.RFC3339)))

	for _, secret := range secrets {
		// Escape value for shell safety
		value := strings.ReplaceAll(secret.Value, "\"", "\\\"")
		content.WriteString(fmt.Sprintf("%s=\"%s\"\n", secret.Key, value))
	}

	// Write file atomically
	tempPath := secretsPath + ".tmp"
	if err := os.WriteFile(tempPath, []byte(content.String()), 0600); err != nil {
		return fmt.Errorf("failed to write temporary secrets file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tempPath, secretsPath); err != nil {
		return fmt.Errorf("failed to rename temporary secrets file: %w", err)
	}

	slog.Info("secrets file updated", "path", secretsPath, "count", len(secrets))
	return nil
}

// applyFirewallRules applies firewall rules via iptables
func (r *Reconciler) applyFirewallRules(rules []FirewallRule) error {
	// Flush existing LibOps rules
	// (In production, use a dedicated chain for LibOps rules)
	slog.Info("applying firewall rules", "rule_count", len(rules))

	// Create LibOps chain if it doesn't exist
	createChainCmd := exec.Command("iptables", "-N", "LIBOPS-FIREWALL")
	_ = createChainCmd.Run() // Ignore error if chain already exists

	// Flush existing rules in LibOps chain
	flushCmd := exec.Command("iptables", "-F", "LIBOPS-FIREWALL")
	if err := flushCmd.Run(); err != nil {
		return fmt.Errorf("failed to flush LibOps chain: %w", err)
	}

	// Apply each rule
	for _, rule := range rules {
		var args []string

		// Build iptables command
		args = append(args, "-A", "LIBOPS-FIREWALL")

		if rule.Protocol != "" {
			args = append(args, "-p", rule.Protocol)
		}

		if rule.Port > 0 {
			args = append(args, "--dport", fmt.Sprintf("%d", rule.Port))
		}

		if rule.Source != "" {
			args = append(args, "-s", rule.Source)
		}

		// Map action to iptables target
		target := "ACCEPT"
		if rule.Action == "deny" || rule.Action == "drop" {
			target = "DROP"
		} else if rule.Action == "reject" {
			target = "REJECT"
		}
		args = append(args, "-j", target)

		cmd := exec.Command("iptables", args...)
		if err := cmd.Run(); err != nil {
			slog.Error("failed to apply firewall rule",
				"protocol", rule.Protocol,
				"port", rule.Port,
				"source", rule.Source,
				"action", rule.Action,
				"error", err)
			// Continue with other rules
		}
	}

	// Ensure LibOps chain is referenced in INPUT chain
	// Check if jump rule already exists
	checkCmd := exec.Command("iptables", "-C", "INPUT", "-j", "LIBOPS-FIREWALL")
	if err := checkCmd.Run(); err != nil {
		// Rule doesn't exist, add it
		jumpCmd := exec.Command("iptables", "-I", "INPUT", "1", "-j", "LIBOPS-FIREWALL")
		if err := jumpCmd.Run(); err != nil {
			return fmt.Errorf("failed to add jump rule: %w", err)
		}
	}

	return nil
}

// fetchDeployment fetches deployment config from admin API
func (r *Reconciler) fetchDeployment(ctx context.Context, token string) (*Deployment, error) {
	endpoint := fmt.Sprintf("%s/admin/sites/%s/deployment", r.apiURL, r.siteID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch deployment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var deployment Deployment
	if err := json.NewDecoder(resp.Body).Decode(&deployment); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &deployment, nil
}

// reportDeploymentStatus reports deployment status back to API
func (r *Reconciler) reportDeploymentStatus(ctx context.Context, token, deploymentID, status, errorMsg string) error {
	endpoint := fmt.Sprintf("%s/admin/deployments/%s/status", r.apiURL, deploymentID)

	payload := map[string]string{
		"status": status,
		"error":  errorMsg,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(body)))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to report status: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// reportReconciliationStatus reports the status of a reconciliation to the API
// This marks resources as "active" after successful reconciliation
func (r *Reconciler) reportReconciliationStatus(ctx context.Context, token, reconciliationType string, resourceIDs []string, status string, errorMsg string) error {
	endpoint := fmt.Sprintf("%s/admin/sites/%s/reconciliation/status", r.apiURL, r.siteID)

	payload := map[string]interface{}{
		"type":         reconciliationType, // "ssh_keys", "secrets", "firewall", "deployment"
		"status":       status,              // "active", "failed"
		"resource_ids": resourceIDs,         // IDs of resources that were reconciled
		"error":        errorMsg,
		"timestamp":    time.Now().UTC().Format(time.RFC3339),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(body)))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to report status: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	slog.Info("reported reconciliation status",
		"type", reconciliationType,
		"status", status,
		"resource_count", len(resourceIDs))

	return nil
}

// executeDeployment performs the actual deployment
func (r *Reconciler) executeDeployment(ctx context.Context, deployment *Deployment) error {
	slog.Info("executing deployment",
		"deployment_id", deployment.DeploymentID,
		"repo", deployment.GitHubRepo,
		"ref", deployment.GitHubRef,
		"commit_sha", deployment.CommitSHA)

	deployPath := deployment.DeploymentPath
	if deployPath == "" {
		deployPath = "/opt/app"
	}

	// 1. Clone or update repository
	if err := r.cloneOrUpdateRepo(ctx, deployment, deployPath); err != nil {
		return fmt.Errorf("failed to clone/update repo: %w", err)
	}

	// 2. Write environment variables
	if err := r.writeDeploymentEnv(deployment, deployPath); err != nil {
		return fmt.Errorf("failed to write environment: %w", err)
	}

	// 3. Run docker-compose
	composeFile := deployment.ComposeFile
	if composeFile == "" {
		composeFile = "docker-compose.yml"
	}

	if err := r.deployWithCompose(ctx, deployPath, composeFile); err != nil {
		return fmt.Errorf("failed to deploy with docker-compose: %w", err)
	}

	slog.Info("deployment executed successfully", "deployment_id", deployment.DeploymentID)
	return nil
}

// cloneOrUpdateRepo clones the repository or updates it if it already exists
func (r *Reconciler) cloneOrUpdateRepo(ctx context.Context, deployment *Deployment, deployPath string) error {
	// Check if repo already exists
	_, err := exec.Command("git", "-C", deployPath, "status").Output()
	if err != nil {
		return err
	}
	// Repo exists, update it
	slog.Info("updating existing repository", "path", deployPath)

	// Fetch latest
	cmd := exec.CommandContext(ctx, "git", "-C", deployPath, "fetch", "origin")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git fetch failed: %s: %w", string(output), err)
	}

	// Checkout the specific ref/commit
	cmd = exec.CommandContext(ctx, "git", "-C", deployPath, "checkout", deployment.GitHubRef)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git checkout failed: %s: %w", string(output), err)
	}

	// Pull latest changes
	cmd = exec.CommandContext(ctx, "git", "-C", deployPath, "pull", "origin", deployment.GitHubRef)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git pull failed: %s: %w", string(output), err)
	}

	// Verify we're on the correct commit
	cmd = exec.CommandContext(ctx, "git", "-C", deployPath, "rev-parse", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get current commit: %w", err)
	}

	currentSHA := strings.TrimSpace(string(output))
	if deployment.CommitSHA != "" && currentSHA != deployment.CommitSHA {
		return fmt.Errorf("commit SHA mismatch: expected %s, got %s", deployment.CommitSHA, currentSHA)
	}

	slog.Info("repository ready", "path", deployPath, "commit", currentSHA)
	return nil
}

// writeDeploymentEnv writes environment variables to .env file
func (r *Reconciler) writeDeploymentEnv(deployment *Deployment, deployPath string) error {
	if len(deployment.Environment) == 0 {
		return nil
	}

	envPath := fmt.Sprintf("%s/.env", deployPath)

	var content strings.Builder
	content.WriteString("# LibOps deployment environment - Auto-generated\n")
	content.WriteString(fmt.Sprintf("# Deployment ID: %s\n", deployment.DeploymentID))
	content.WriteString(fmt.Sprintf("# Updated: %s\n\n", time.Now().Format(time.RFC3339)))

	for key, value := range deployment.Environment {
		// Escape values with quotes if they contain special characters
		escapedValue := value
		if strings.ContainsAny(value, " \t\n\"'$") {
			escapedValue = fmt.Sprintf("\"%s\"", strings.ReplaceAll(value, "\"", "\\\""))
		}
		content.WriteString(fmt.Sprintf("%s=%s\n", key, escapedValue))
	}

	if err := os.WriteFile(envPath, []byte(content.String()), 0600); err != nil {
		return fmt.Errorf("failed to write .env file: %w", err)
	}

	slog.Info("environment variables written", "path", envPath, "var_count", len(deployment.Environment))
	return nil
}

// deployWithCompose deploys the application using docker-compose
func (r *Reconciler) deployWithCompose(ctx context.Context, deployPath, composeFile string) error {
	composePath := fmt.Sprintf("%s/%s", deployPath, composeFile)

	// Check if compose file exists
	if _, err := os.Stat(composePath); err != nil {
		return fmt.Errorf("compose file not found: %s: %w", composePath, err)
	}

	slog.Info("deploying with docker compose", "compose_file", composePath)

	// Pull latest images
	cmd := exec.CommandContext(ctx, "docker", "compose", "-f", composePath, "pull")
	cmd.Dir = deployPath
	if output, err := cmd.CombinedOutput(); err != nil {
		slog.Warn("docker-compose pull failed", "error", err, "output", string(output))
		// Don't fail on pull errors, continue with deployment
	}

	// Stop existing containers
	cmd = exec.CommandContext(ctx, "docker", "compose", "-f", composePath, "down")
	cmd.Dir = deployPath
	if output, err := cmd.CombinedOutput(); err != nil {
		slog.Warn("docker-compose down failed", "error", err, "output", string(output))
	}

	// Start containers
	cmd = exec.CommandContext(ctx, "docker", "compose", "-f", composePath, "up", "-d", "--remove-orphans")
	cmd.Dir = deployPath
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker-compose up failed: %s: %w", string(output), err)
	}

	slog.Info("deployment successful via docker-compose")
	return nil
}
