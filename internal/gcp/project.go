// Package gcp provides utilities for interacting with Google Cloud Platform services.
package gcp

import (
	"fmt"
	"strings"
)

// ProjectNamePrefix is the prefix used for all LibOps organization projects in GCP.
const ProjectNamePrefix = "libops-"

// MaxProjectIDLength is the maximum allowed length for a GCP project ID.
const MaxProjectIDLength = 30

// PlatformServiceAccountName is the base name for the platform service account created in each organization project.
const PlatformServiceAccountName = "libops-platform"

// GenerateProjectID generates a GCP project ID from a organization UUID
// Uses collision handling strategy:
// 1. First try: libops-{first 8 chars of UUID without dashes} (15 chars total)
// 2. On collision: libops-{first 13 chars of UUID with dashes} (21 chars total)
// 3. On collision: libops-{UUID with dashes removed} (30 chars total - max allowed).
func GenerateProjectID(organizationUUID string, collisionAttempt int) string {
	switch collisionAttempt {
	case 0:
		// First attempt: libops-{8 chars no dashes} = 15 chars total
		cleanUUID := strings.ReplaceAll(organizationUUID, "-", "")
		if len(cleanUUID) >= 8 {
			return ProjectNamePrefix + cleanUUID[:8]
		}
		return ProjectNamePrefix + cleanUUID

	case 1:
		// Second attempt: libops-{13 chars with dashes} = 21 chars total
		// Keep the original UUID format with dashes
		if len(organizationUUID) >= 13 {
			return ProjectNamePrefix + organizationUUID[:13]
		}
		return ProjectNamePrefix + organizationUUID

	default:
		// Third attempt: libops-{full UUID no dashes} = up to 30 chars
		cleanUUID := strings.ReplaceAll(organizationUUID, "-", "")
		fullID := ProjectNamePrefix + cleanUUID
		if len(fullID) > MaxProjectIDLength {
			return fullID[:MaxProjectIDLength]
		}
		return fullID
	}
}

// GetPlatformServiceAccountEmail returns the email address for the platform service account
// Format: libops-platform@{project-id}.iam.gserviceaccount.com.
func GetPlatformServiceAccountEmail(projectID string) string {
	return fmt.Sprintf("%s@%s.iam.gserviceaccount.com", PlatformServiceAccountName, projectID)
}

// IsPlatformServiceAccount checks if an email is a libops platform service account.
func IsPlatformServiceAccount(email string) bool {
	if !strings.HasPrefix(email, PlatformServiceAccountName+"@") {
		return false
	}
	if !strings.HasSuffix(email, ".iam.gserviceaccount.com") {
		return false
	}

	projectID := ExtractProjectIDFromServiceAccount(email)
	return strings.HasPrefix(projectID, ProjectNamePrefix)
}

// ExtractProjectIDFromServiceAccount extracts the project ID from a service account email
// Returns empty string if not a valid service account email.
func ExtractProjectIDFromServiceAccount(email string) string {
	if !strings.HasSuffix(email, ".iam.gserviceaccount.com") {
		return ""
	}

	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return ""
	}

	domainParts := strings.Split(parts[1], ".")
	if len(domainParts) < 3 {
		return ""
	}

	return domainParts[0]
}
