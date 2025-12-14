package gcp

import (
	"strings"
	"testing"
)

// TestGenerateProjectID tests the GenerateProjectID function for correct project ID generation based on organization UUID and collision attempt.
func TestGenerateProjectID(t *testing.T) {
	tests := []struct {
		name             string
		organizationUUID string
		collisionAttempt int
		expectedPrefix   string
		maxLength        int
	}{
		{
			name:             "First attempt with standard UUID",
			organizationUUID: "12345678-1234-1234-1234-123456789abc",
			collisionAttempt: 0,
			expectedPrefix:   "libops-12345678",
			maxLength:        16,
		},
		{
			name:             "Second attempt with standard UUID",
			organizationUUID: "12345678-1234-1234-1234-123456789abc",
			collisionAttempt: 1,
			expectedPrefix:   "libops-12345678-",
			maxLength:        21,
		},
		{
			name:             "Third attempt with standard UUID",
			organizationUUID: "12345678-1234-1234-1234-123456789abc",
			collisionAttempt: 2,
			expectedPrefix:   "libops-12345678123412341234123456",
			maxLength:        30,
		},
		{
			name:             "Short UUID first attempt",
			organizationUUID: "1234",
			collisionAttempt: 0,
			expectedPrefix:   "libops-1234",
			maxLength:        12,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateProjectID(tt.organizationUUID, tt.collisionAttempt)

			if !strings.HasPrefix(result, "libops-") {
				t.Errorf("Expected result to start with 'libops-', got: %s", result)
			}

			if len(result) > MaxProjectIDLength {
				t.Errorf("Result length %d exceeds max project ID length %d", len(result), MaxProjectIDLength)
			}

			if len(result) > tt.maxLength {
				t.Errorf("Result length %d exceeds expected max length %d for attempt %d", len(result), tt.maxLength, tt.collisionAttempt)
			}

			t.Logf("Attempt %d: %s (length: %d)", tt.collisionAttempt, result, len(result))
		})
	}
}

// TestGetPlatformServiceAccountEmail tests the GetPlatformServiceAccountEmail function for correct email generation.
func TestGetPlatformServiceAccountEmail(t *testing.T) {
	tests := []struct {
		name      string
		projectID string
		expected  string
	}{
		{
			name:      "Standard project ID",
			projectID: "libops-12345678",
			expected:  "libops-platform@libops-12345678.iam.gserviceaccount.com",
		},
		{
			name:      "Long project ID",
			projectID: "libops-123456781234123412341234",
			expected:  "libops-platform@libops-123456781234123412341234.iam.gserviceaccount.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetPlatformServiceAccountEmail(tt.projectID)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

// TestIsPlatformServiceAccount tests the IsPlatformServiceAccount function for correctly identifying platform service accounts.
func TestIsPlatformServiceAccount(t *testing.T) {
	tests := []struct {
		name     string
		email    string
		expected bool
	}{
		{
			name:     "Valid platform service account",
			email:    "libops-platform@libops-12345678.iam.gserviceaccount.com",
			expected: true,
		},
		{
			name:     "Valid platform service account with longer project ID",
			email:    "libops-platform@libops-123456781234.iam.gserviceaccount.com",
			expected: true,
		},
		{
			name:     "Regular user email",
			email:    "user@example.com",
			expected: false,
		},
		{
			name:     "Different service account",
			email:    "other-sa@project.iam.gserviceaccount.com",
			expected: false,
		},
		{
			name:     "Platform SA but not libops project",
			email:    "libops-platform@other-project.iam.gserviceaccount.com",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsPlatformServiceAccount(tt.email)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v for email: %s", tt.expected, result, tt.email)
			}
		})
	}
}

// TestExtractProjectIDFromServiceAccount tests the ExtractProjectIDFromServiceAccount function for correctly extracting project IDs from service account emails.
func TestExtractProjectIDFromServiceAccount(t *testing.T) {
	tests := []struct {
		name     string
		email    string
		expected string
	}{
		{
			name:     "Valid platform service account",
			email:    "libops-platform@libops-12345678.iam.gserviceaccount.com",
			expected: "libops-12345678",
		},
		{
			name:     "Valid service account with different name",
			email:    "my-sa@my-project.iam.gserviceaccount.com",
			expected: "my-project",
		},
		{
			name:     "Regular email",
			email:    "user@example.com",
			expected: "",
		},
		{
			name:     "Malformed service account",
			email:    "invalid@format",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractProjectIDFromServiceAccount(tt.email)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s for email: %s", tt.expected, result, tt.email)
			}
		})
	}
}
