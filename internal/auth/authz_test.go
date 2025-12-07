package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"

	optionsv1 "github.com/libops/api/proto/libops/v1/options"
)

// =============================================================================
// TESTS: Scope Checking Logic - OAuth vs API Key
// =============================================================================

// TestHasScope_OAuthUserNoScopes_ReturnsFalse verifies that an OAuth user with no explicit scopes will return false for any scope check, as the interceptor handles OAuth authorization separately.
func TestHasScope_OAuthUserNoScopes_ReturnsFalse(t *testing.T) {
	// OAuth users have no scopes - the interceptor bypasses scope check when len(scopes) == 0
	userScopes := []Scope{}

	scopeRule := &optionsv1.ScopeRule{
		Resource: optionsv1.ResourceType_RESOURCE_TYPE_ORGANIZATION,
		Level:    optionsv1.AccessLevel_ACCESS_LEVEL_READ,
	}

	hasScope := HasScope(userScopes, scopeRule)
	assert.False(t, hasScope, "Empty scopes should return false - interceptor handles OAuth separately")
}

// TestHasScope_APIKey_ExactMatch verifies that an API key with an exactly matching scope grants access.
func TestHasScope_APIKey_ExactMatch(t *testing.T) {
	userScopes := []Scope{
		{
			Resource: optionsv1.ResourceType_RESOURCE_TYPE_ORGANIZATION,
			Level:    optionsv1.AccessLevel_ACCESS_LEVEL_READ,
		},
	}

	scopeRule := &optionsv1.ScopeRule{
		Resource: optionsv1.ResourceType_RESOURCE_TYPE_ORGANIZATION,
		Level:    optionsv1.AccessLevel_ACCESS_LEVEL_READ,
	}

	hasScope := HasScope(userScopes, scopeRule)
	assert.True(t, hasScope, "Exact scope match should return true")
}

// TestHasScope_APIKey_HigherLevelDoesNotSatisfyLower verifies that scope matching requires exact level match.
func TestHasScope_APIKey_HigherLevelDoesNotSatisfyLower(t *testing.T) {
	userScopes := []Scope{
		{
			Resource: optionsv1.ResourceType_RESOURCE_TYPE_ORGANIZATION,
			Level:    optionsv1.AccessLevel_ACCESS_LEVEL_WRITE,
		},
	}

	scopeRule := &optionsv1.ScopeRule{
		Resource: optionsv1.ResourceType_RESOURCE_TYPE_ORGANIZATION,
		Level:    optionsv1.AccessLevel_ACCESS_LEVEL_READ,
	}

	hasScope := HasScope(userScopes, scopeRule)
	assert.False(t, hasScope, "WRITE level should NOT satisfy READ requirement (exact match only)")
}

// TestHasScope_APIKey_AdminExactMatchOnly verifies that scope matching requires exact level match even for ADMIN.
func TestHasScope_APIKey_AdminExactMatchOnly(t *testing.T) {
	userScopes := []Scope{
		{
			Resource: optionsv1.ResourceType_RESOURCE_TYPE_PROJECT,
			Level:    optionsv1.AccessLevel_ACCESS_LEVEL_ADMIN,
		},
	}

	scopeRule := &optionsv1.ScopeRule{
		Resource: optionsv1.ResourceType_RESOURCE_TYPE_PROJECT,
		Level:    optionsv1.AccessLevel_ACCESS_LEVEL_READ,
	}
	assert.False(t, HasScope(userScopes, scopeRule), "ADMIN should NOT satisfy READ (exact match only)")

	scopeRule.Level = optionsv1.AccessLevel_ACCESS_LEVEL_WRITE
	assert.False(t, HasScope(userScopes, scopeRule), "ADMIN should NOT satisfy WRITE (exact match only)")

	scopeRule.Level = optionsv1.AccessLevel_ACCESS_LEVEL_ADMIN
	assert.True(t, HasScope(userScopes, scopeRule), "ADMIN should satisfy ADMIN (exact match)")
}

// TestHasScope_APIKey_InsufficientLevel verifies that an API key with an insufficient access level (e.g., READ) cannot satisfy a higher access level requirement (e.g., WRITE).
func TestHasScope_APIKey_InsufficientLevel(t *testing.T) {
	userScopes := []Scope{
		{
			Resource: optionsv1.ResourceType_RESOURCE_TYPE_ORGANIZATION,
			Level:    optionsv1.AccessLevel_ACCESS_LEVEL_READ,
		},
	}

	scopeRule := &optionsv1.ScopeRule{
		Resource: optionsv1.ResourceType_RESOURCE_TYPE_ORGANIZATION,
		Level:    optionsv1.AccessLevel_ACCESS_LEVEL_WRITE,
	}

	hasScope := HasScope(userScopes, scopeRule)
	assert.False(t, hasScope, "READ level should NOT satisfy WRITE requirement")
}

// TestHasScope_APIKey_WrongResource verifies that an API key for one resource type cannot access another resource type when hierarchical access is disabled.
func TestHasScope_APIKey_WrongResource(t *testing.T) {
	userScopes := []Scope{
		{
			Resource: optionsv1.ResourceType_RESOURCE_TYPE_PROJECT,
			Level:    optionsv1.AccessLevel_ACCESS_LEVEL_ADMIN,
		},
	}

	// Case 1: WRITE access - Child scope should NOT satisfy Parent requirement
	scopeRule := &optionsv1.ScopeRule{
		Resource:          optionsv1.ResourceType_RESOURCE_TYPE_ORGANIZATION,
		Level:             optionsv1.AccessLevel_ACCESS_LEVEL_WRITE,
		AllowParentAccess: false,
	}
	hasScope := HasScope(userScopes, scopeRule)
	assert.False(t, hasScope, "PROJECT scope should not match ORGANIZATION WRITE without parent access")

	// Case 2: READ access - Child scope should NOT satisfy Parent requirement (Strict Hierarchy)
	scopeRule.Level = optionsv1.AccessLevel_ACCESS_LEVEL_READ
	hasScope = HasScope(userScopes, scopeRule)
	assert.False(t, hasScope, "PROJECT scope should NOT satisfy ORGANIZATION READ (implicit inheritance removed)")
}

// ... (TestHasScope_APIKey_MultipleScopes_FindsMatch remains the same) ...

// TestHasScope_ChildScope_Behavior verifies that a child resource scope behaves correctly against parent requirements.
// It should NOT satisfy ANY requirements (Read or Write) implicitly.
func TestHasScope_ChildScope_Behavior(t *testing.T) {
	// Test 1: SITE does NOT satisfy PROJECT
	userScopes := []Scope{
		{Resource: optionsv1.ResourceType_RESOURCE_TYPE_SITE, Level: optionsv1.AccessLevel_ACCESS_LEVEL_ADMIN},
	}

	// READ
	scopeRule := &optionsv1.ScopeRule{
		Resource:          optionsv1.ResourceType_RESOURCE_TYPE_PROJECT,
		Level:             optionsv1.AccessLevel_ACCESS_LEVEL_READ,
		AllowParentAccess: true,
	}
	assert.False(t, HasScope(userScopes, scopeRule), "SITE scope should NOT satisfy PROJECT READ")

	// WRITE
	scopeRule.Level = optionsv1.AccessLevel_ACCESS_LEVEL_WRITE
	assert.False(t, HasScope(userScopes, scopeRule), "SITE scope should NOT satisfy PROJECT WRITE")

	// Test 2: PROJECT does NOT satisfy ORGANIZATION
	userScopes = []Scope{
		{Resource: optionsv1.ResourceType_RESOURCE_TYPE_PROJECT, Level: optionsv1.AccessLevel_ACCESS_LEVEL_ADMIN},
	}

	// READ
	scopeRule = &optionsv1.ScopeRule{
		Resource:          optionsv1.ResourceType_RESOURCE_TYPE_ORGANIZATION,
		Level:             optionsv1.AccessLevel_ACCESS_LEVEL_READ,
		AllowParentAccess: true,
	}
	assert.False(t, HasScope(userScopes, scopeRule), "PROJECT scope should NOT satisfy ORGANIZATION READ")

	// WRITE
	scopeRule.Level = optionsv1.AccessLevel_ACCESS_LEVEL_WRITE
	assert.False(t, HasScope(userScopes, scopeRule), "PROJECT scope should NOT satisfy ORGANIZATION WRITE")

	// Test 3: ORGANIZATION does NOT satisfy ACCOUNT
	userScopes = []Scope{
		{Resource: optionsv1.ResourceType_RESOURCE_TYPE_ORGANIZATION, Level: optionsv1.AccessLevel_ACCESS_LEVEL_ADMIN},
	}

	// READ
	scopeRule = &optionsv1.ScopeRule{
		Resource:          optionsv1.ResourceType_RESOURCE_TYPE_ACCOUNT,
		Level:             optionsv1.AccessLevel_ACCESS_LEVEL_READ,
		AllowParentAccess: true,
	}
	assert.False(t, HasScope(userScopes, scopeRule), "ORGANIZATION scope should NOT satisfy ACCOUNT READ")

	// WRITE
	scopeRule.Level = optionsv1.AccessLevel_ACCESS_LEVEL_WRITE
	assert.False(t, HasScope(userScopes, scopeRule), "ORGANIZATION scope should NOT satisfy ACCOUNT WRITE")
}

// =============================================================================
// TESTS: OAuth vs API Key Scope Assignment
// =============================================================================

// TestGetAccountScopesForOAuth_ReturnsEmpty verifies that OAuth users are assigned an empty list of scopes, as their authorization is membership-based.
func TestGetAccountScopesForOAuth_ReturnsEmpty(t *testing.T) {
	// OAuth users should get NO scopes - authorization is purely membership-based
	scopes := GetAccountScopesForOAuth()

	assert.Empty(t, scopes, "OAuth users must have empty scopes - authorization is membership-based")
	assert.Equal(t, 0, len(scopes), "OAuth scope list length should be 0")
}

// TestParseScopes_Valid tests the parsing of valid API key scope strings.
func TestParseScopes_Valid(t *testing.T) {
	apiKeyScopes := []string{
		"organization:read",
		"project:write",
		"site:admin",
	}

	scopes, err := ParseScopes(apiKeyScopes)
	assert.NoError(t, err)

	expected := []Scope{
		{Resource: optionsv1.ResourceType_RESOURCE_TYPE_ORGANIZATION, Level: optionsv1.AccessLevel_ACCESS_LEVEL_READ},
		{Resource: optionsv1.ResourceType_RESOURCE_TYPE_PROJECT, Level: optionsv1.AccessLevel_ACCESS_LEVEL_WRITE},
		{Resource: optionsv1.ResourceType_RESOURCE_TYPE_SITE, Level: optionsv1.AccessLevel_ACCESS_LEVEL_ADMIN},
	}

	assert.Equal(t, expected, scopes, "Should parse scopes correctly")
}

// TestParseScopes_Invalid tests the parsing of invalid API key scope strings.
func TestParseScopes_Invalid(t *testing.T) {
	apiKeyScopes := []string{
		"organization:read", // Valid
		"invalid",           // Invalid format - should cause error
	}

	_, err := ParseScopes(apiKeyScopes)
	assert.Error(t, err, "Should return error for invalid scope format")
}

// TestParseScope_ValidFormats tests the parsing of individual valid scope strings.
func TestParseScope_ValidFormats(t *testing.T) {
	tests := []struct {
		input    string
		expected Scope
	}{
		{"organization:read", Scope{Resource: optionsv1.ResourceType_RESOURCE_TYPE_ORGANIZATION, Level: optionsv1.AccessLevel_ACCESS_LEVEL_READ}},
		{"project:write", Scope{Resource: optionsv1.ResourceType_RESOURCE_TYPE_PROJECT, Level: optionsv1.AccessLevel_ACCESS_LEVEL_WRITE}},
		{"site:admin", Scope{Resource: optionsv1.ResourceType_RESOURCE_TYPE_SITE, Level: optionsv1.AccessLevel_ACCESS_LEVEL_ADMIN}},
		{"account:admin", Scope{Resource: optionsv1.ResourceType_RESOURCE_TYPE_ACCOUNT, Level: optionsv1.AccessLevel_ACCESS_LEVEL_ADMIN}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			scope, err := ParseScope(tt.input)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, scope)
		})
	}
}

// TestParseScope_InvalidFormats tests the parsing of individual invalid scope strings.
func TestParseScope_InvalidFormats(t *testing.T) {
	tests := []string{
		"invalid",          // No colon
		"badresource:read", // Invalid resource
		"project:badlevel", // Invalid level
		"",                 // Empty string
		"organization:",    // Missing level
		":read",            // Missing resource
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			_, err := ParseScope(input)
			assert.Error(t, err, "Should return error for invalid input: %s", input)
		})
	}
}

// =============================================================================
// TESTS: Scope String Conversion
// =============================================================================

// TestScopesToStrings tests the conversion of a slice of Scope structs to a slice of strings.
func TestScopesToStrings(t *testing.T) {
	scopes := []Scope{
		{Resource: optionsv1.ResourceType_RESOURCE_TYPE_ORGANIZATION, Level: optionsv1.AccessLevel_ACCESS_LEVEL_READ},
		{Resource: optionsv1.ResourceType_RESOURCE_TYPE_PROJECT, Level: optionsv1.AccessLevel_ACCESS_LEVEL_WRITE},
		{Resource: optionsv1.ResourceType_RESOURCE_TYPE_SITE, Level: optionsv1.AccessLevel_ACCESS_LEVEL_ADMIN},
	}

	strings := ScopesToStrings(scopes)

	expected := []string{
		"organization:read",
		"project:write",
		"site:admin",
	}

	assert.Equal(t, expected, strings, "Scopes should convert to strings correctly")
}

// TestScopesToStrings_EmptyList tests the conversion of an empty slice of Scope structs to a slice of strings.
func TestScopesToStrings_EmptyList(t *testing.T) {
	scopes := []Scope{}

	strings := ScopesToStrings(scopes)

	assert.Empty(t, strings, "Empty scope list should produce empty string list")
}

// =============================================================================
// TESTS: Comprehensive Authorization Scenarios
// =============================================================================

// TestAuthorizationScenario_OAuthUser_MembershipOnly tests that an OAuth user's authorization is membership-based and not scope-based.
func TestAuthorizationScenario_OAuthUser_MembershipOnly(t *testing.T) {
	// OAuth users should get NO scopes - authorization is purely membership-based
	scopes := GetAccountScopesForOAuth()
	assert.Empty(t, scopes, "OAuth users must have empty scopes - authorization is membership-based")
	assert.Equal(t, 0, len(scopes), "OAuth scope list length should be 0")

	// When interceptor checks scopes for any endpoint
	scopeRule := &optionsv1.ScopeRule{
		Resource: optionsv1.ResourceType_RESOURCE_TYPE_ORGANIZATION,
		Level:    optionsv1.AccessLevel_ACCESS_LEVEL_WRITE,
	}

	hasScope := HasScope(scopes, scopeRule)
	assert.False(t, hasScope, "OAuth user has no scopes, scope check returns false")

	// In the interceptor, when len(scopes) == 0, it skips to membership check
	// This test confirms the scope check behaves correctly for OAuth
}

// TestAuthorizationScenario_APIKey_FullAccess verifies that an API key with no explicit scope restrictions has no scopes (relies on RBAC only).
func TestAuthorizationScenario_APIKey_FullAccess(t *testing.T) {
	// Scenario: Unrestricted API key (empty scopes)
	// Expected: Gets empty scopes, authorization relies purely on RBAC

	apiKeyScopes := []string{}
	scopes := GetAccountScopesForAPIKey(apiKeyScopes)

	// Should have empty scopes (no restrictions)
	assert.Equal(t, 0, len(scopes), "Unrestricted API key should have no scopes")

	// With no scopes, the scope interceptor bypasses scope checks
	// Authorization is handled entirely by RBAC based on role membership
}

// TestAuthorizationScenario_APIKey_Restricted verifies that an API key with restricted scopes correctly enforces exact match access limitations.
func TestAuthorizationScenario_APIKey_Restricted(t *testing.T) {
	// Scenario: API key restricted to organization:read only
	// Expected: Can only match organization:read exactly, everything else denied by scope check

	apiKeyScopes := []string{"read:organization"}
	scopes := GetAccountScopesForAPIKey(apiKeyScopes)

	// Should allow organization:read (exact match)
	scopeRule := &optionsv1.ScopeRule{
		Resource: optionsv1.ResourceType_RESOURCE_TYPE_ORGANIZATION,
		Level:    optionsv1.AccessLevel_ACCESS_LEVEL_READ,
	}
	assert.True(t, HasScope(scopes, scopeRule), "Should allow organization:read (exact match)")

	// Should deny organization:write (no match)
	scopeRule.Level = optionsv1.AccessLevel_ACCESS_LEVEL_WRITE
	assert.False(t, HasScope(scopes, scopeRule), "Should deny organization:write (no exact match)")

	// Should deny project:read (scope check is exact, no hierarchy)
	scopeRule = &optionsv1.ScopeRule{
		Resource:          optionsv1.ResourceType_RESOURCE_TYPE_PROJECT,
		Level:             optionsv1.AccessLevel_ACCESS_LEVEL_READ,
		AllowParentAccess: true,
	}
	assert.False(t, HasScope(scopes, scopeRule), "Should deny project:read (scope check requires exact match, RBAC handles hierarchy)")

	// Should deny project:write
	scopeRule.Level = optionsv1.AccessLevel_ACCESS_LEVEL_WRITE
	assert.False(t, HasScope(scopes, scopeRule), "Should deny project:write")
}
