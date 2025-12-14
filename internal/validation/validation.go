// Package validation provides utility functions for validating various data inputs.
package validation

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/mail"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Error represents a validation error.
type Error struct {
	Field   string
	Message string
}

// Error returns a formatted string representation of the validation error.
func (e *Error) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// NewError creates a new validation error.
func NewError(field, message string) *Error {
	return &Error{Field: field, Message: message}
}

// Email validates an email address format.
func Email(email string) error {
	if email == "" {
		return NewError("email", "email is required")
	}

	// Use net/mail for RFC 5322 compliance
	addr, err := mail.ParseAddress(email)
	if err != nil {
		return NewError("email", "invalid email format")
	}

	if addr.Address != email {
		return NewError("email", "invalid email format")
	}

	// Check length (RFC 5321)
	if len(email) > 254 {
		return NewError("email", "email too long (max 254 characters)")
	}

	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return NewError("email", "invalid email format")
	}

	localPart, domain := parts[0], parts[1]

	// Local part max 64 chars
	if len(localPart) > 64 {
		return NewError("email", "email local part too long (max 64 characters)")
	}

	// Domain must have at least one dot
	if !strings.Contains(domain, ".") {
		return NewError("email", "invalid domain")
	}

	return nil
}

// CIDR validates a CIDR notation IP range.
func CIDR(cidr string) error {
	if cidr == "" {
		return NewError("cidr", "CIDR is required")
	}

	_, _, err := net.ParseCIDR(cidr)
	if err != nil {
		return NewError("cidr", "invalid CIDR format")
	}

	return nil
}

// IPAddress validates an IP address (IPv4 or IPv6).
func IPAddress(ip string) error {
	if ip == "" {
		return NewError("ip", "IP address is required")
	}

	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return NewError("ip", "invalid IP address format")
	}

	return nil
}

// UUID validates a UUID string.
func UUID(uuidStr string) error {
	if uuidStr == "" {
		return NewError("uuid", "UUID is required")
	}

	_, err := uuid.Parse(uuidStr)
	if err != nil {
		return NewError("uuid", "invalid UUID format")
	}

	return nil
}

// StringLength validates string length constraints.
func StringLength(fieldName, value string, minLength, maxLength int) error {
	length := utf8.RuneCountInString(value)

	if minLength > 0 && length < minLength {
		return NewError(fieldName, fmt.Sprintf("must be at least %d characters", minLength))
	}

	if maxLength > 0 && length > maxLength {
		return NewError(fieldName, fmt.Sprintf("must be at most %d characters", maxLength))
	}

	return nil
}

// RequiredString validates that a string is not empty.
func RequiredString(fieldName, value string) error {
	if strings.TrimSpace(value) == "" {
		return NewError(fieldName, "is required")
	}
	return nil
}

// GCPProjectID validates a GCP project ID format
// Requirements:
// - 6-30 characters
// - Only lowercase letters, digits, and hyphens
// - Must start with a letter
// - Cannot end with a hyphen.
func GCPProjectID(projectID string) error {
	if projectID == "" {
		return NewError("project_id", "GCP project ID is required")
	}

	if len(projectID) < 6 || len(projectID) > 30 {
		return NewError("project_id", "GCP project ID must be 6-30 characters")
	}

	// Regex pattern for GCP project ID
	pattern := `^[a-z][a-z0-9-]*[a-z0-9]$`
	matched, err := regexp.MatchString(pattern, projectID)
	if err != nil {
		return NewError("project_id", "error validating project ID")
	}

	if !matched {
		return NewError("project_id", "invalid GCP project ID format (must start with letter, contain only lowercase letters, digits, and hyphens, and not end with hyphen)")
	}

	return nil
}

// GitHubRepo validates a GitHub repository format (owner/repo).
func GitHubRepo(repo string) error {
	if repo == "" {
		return NewError("github_repo", "GitHub repository is required")
	}

	parts := strings.Split(repo, "/")
	if len(parts) != 2 {
		return NewError("github_repo", "must be in format 'owner/repo'")
	}

	owner, repoName := parts[0], parts[1]

	if owner == "" || repoName == "" {
		return NewError("github_repo", "owner and repository name cannot be empty")
	}

	// GitHub username/org name restrictions
	// - 1-39 characters
	// - Only alphanumeric and hyphens
	// - Cannot start or end with hyphen
	// - Cannot have consecutive hyphens
	ownerPattern := `^[a-zA-Z0-9]([a-zA-Z0-9-]{0,37}[a-zA-Z0-9])?$`
	matched, _ := regexp.MatchString(ownerPattern, owner)
	if !matched {
		return NewError("github_repo", "invalid GitHub owner/organization name")
	}

	// Repository name restrictions
	// Similar to owner but can contain dots and underscores
	repoPattern := `^[a-zA-Z0-9._-]+$`
	matched, _ = regexp.MatchString(repoPattern, repoName)
	if !matched {
		return NewError("github_repo", "invalid GitHub repository name")
	}

	if len(repoName) > 100 {
		return NewError("github_repo", "repository name too long (max 100 characters)")
	}

	return nil
}

// GitHubRepoIsPublic checks if a GitHub repository is publicly accessible.
// This function makes an HTTP request to the GitHub API to verify the repository exists and is public.
func GitHubRepoIsPublic(ctx context.Context, repo string) error {
	if err := GitHubRepo(repo); err != nil {
		return err
	}

	// GitHub API endpoint for repository information
	url := fmt.Sprintf("https://api.github.com/repos/%s", repo)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return NewError("github_repo", "failed to create request to verify repository")
	}

	// Set User-Agent header as required by GitHub API
	req.Header.Set("User-Agent", "libops-api")
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return NewError("github_repo", "failed to verify repository")
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		// Repository exists and is public - decode to verify it's not private
		var repoData struct {
			Private bool `json:"private"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&repoData); err != nil {
			return NewError("github_repo", "failed to parse repository information")
		}

		if repoData.Private {
			return NewError("github_repo", "repository must be public (private repositories are not yet supported)")
		}

		return nil

	case http.StatusNotFound:
		return NewError("github_repo", "repository not found or is private")

	case http.StatusForbidden, http.StatusUnauthorized:
		return NewError("github_repo", "unable to verify repository accessibility")

	default:
		return NewError("github_repo", fmt.Sprintf("unexpected response from GitHub API: %d", resp.StatusCode))
	}
}

// PasswordComplexity validates password complexity requirements.
func PasswordComplexity(password string) error {
	if password == "" {
		return NewError("password", "password is required")
	}

	if len(password) < 12 {
		return NewError("password", "password must be at least 12 characters")
	}

	// Maximum length (for bcrypt compatibility)
	if len(password) > 72 {
		return NewError("password", "password must be at most 72 characters")
	}

	hasUpper := regexp.MustCompile(`[A-Z]`).MatchString(password)
	if !hasUpper {
		return NewError("password", "password must contain at least one uppercase letter")
	}

	hasLower := regexp.MustCompile(`[a-z]`).MatchString(password)
	if !hasLower {
		return NewError("password", "password must contain at least one lowercase letter")
	}

	hasDigit := regexp.MustCompile(`[0-9]`).MatchString(password)
	if !hasDigit {
		return NewError("password", "password must contain at least one digit")
	}

	hasSpecial := regexp.MustCompile(`[!@#$%^&*()_+\-=\[\]{};':"\\|,.<>\/?]`).MatchString(password)
	if !hasSpecial {
		return NewError("password", "password must contain at least one special character")
	}

	return nil
}

// OrganizationName validates a organization name.
func OrganizationName(name string) error {
	if err := RequiredString("organization_name", name); err != nil {
		return err
	}

	if err := StringLength("organization_name", name, 1, 255); err != nil {
		return err
	}

	// Only allow alphanumeric, spaces, hyphens, underscores
	pattern := `^[a-zA-Z0-9\s_-]+$`
	matched, _ := regexp.MatchString(pattern, name)
	if !matched {
		return NewError("organization_name", "can only contain letters, numbers, spaces, hyphens, and underscores")
	}

	return nil
}

// ProjectName validates a project name.
func ProjectName(name string) error {
	if err := RequiredString("project_name", name); err != nil {
		return err
	}

	if err := StringLength("project_name", name, 1, 255); err != nil {
		return err
	}

	return nil
}

// SiteName validates a site name.
func SiteName(name string) error {
	if err := RequiredString("site_name", name); err != nil {
		return err
	}

	if err := StringLength("site_name", name, 1, 255); err != nil {
		return err
	}

	return nil
}

// Port validates a network port number.
func Port(port int32) error {
	if port < 1 || port > 65535 {
		return NewError("port", "port must be between 1 and 65535")
	}
	return nil
}

// SSHPublicKey validates an SSH public key format.
func SSHPublicKey(key string) error {
	if key == "" {
		return NewError("ssh_key", "SSH public key is required")
	}

	// Basic format check - should start with ssh-rsa, ssh-ed25519, ecdsa-sha2-, etc.
	validPrefixes := []string{"ssh-rsa", "ssh-ed25519", "ecdsa-sha2-nistp256", "ecdsa-sha2-nistp384", "ecdsa-sha2-nistp521"}
	hasValidPrefix := false
	for _, prefix := range validPrefixes {
		if strings.HasPrefix(key, prefix) {
			hasValidPrefix = true
			break
		}
	}

	if !hasValidPrefix {
		return NewError("ssh_key", "invalid SSH public key format")
	}

	if len(key) < 80 {
		return NewError("ssh_key", "SSH public key too short")
	}

	if len(key) > 8192 {
		return NewError("ssh_key", "SSH public key too long")
	}

	parts := strings.Fields(key)
	if len(parts) < 2 {
		return NewError("ssh_key", "invalid SSH public key format")
	}

	return nil
}

// GCPZoneMatchesRegion validates that a GCP zone is prefixed by the specified region.
// For example, zone "us-central1-f" must be in region "us-central1".
func GCPZoneMatchesRegion(region, zone string) error {
	if region == "" {
		return NewError("region", "region is required")
	}

	if zone == "" {
		return NewError("zone", "zone is required")
	}

	// Zone should be in format: region-letter (e.g., us-central1-a, us-central1-f)
	if !strings.HasPrefix(zone, region+"-") {
		return NewError("zone", fmt.Sprintf("zone must be in region %s (zone should start with '%s-')", region, region))
	}

	// Verify the zone suffix is a single letter
	zoneSuffix := strings.TrimPrefix(zone, region+"-")
	if len(zoneSuffix) != 1 || (zoneSuffix[0] < 'a' || zoneSuffix[0] > 'z') {
		return NewError("zone", fmt.Sprintf("invalid zone format (expected format: %s-[a-z])", region))
	}

	return nil
}
