package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"

	optionsv1 "github.com/libops/api/proto/libops/v1/options"
)

// TestMapOAuthScopesToStructured_Comprehensive tests the 1:1 mapping of OAuth scopes to structured internal Scope representations.
func TestMapOAuthScopesToStructured_Comprehensive(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []Scope
	}{
		{
			name: "Account Scopes",
			input: []string{
				"read:user",
				"write:user",
				"read:organizations",
			},
			expected: []Scope{
				{Resource: optionsv1.ResourceType_RESOURCE_TYPE_ACCOUNT, Level: optionsv1.AccessLevel_ACCESS_LEVEL_READ},
				{Resource: optionsv1.ResourceType_RESOURCE_TYPE_ACCOUNT, Level: optionsv1.AccessLevel_ACCESS_LEVEL_WRITE},
				// read:organizations maps to account:read, which is duplicate of read:user mapping (deduped)
			},
		},
		{
			name: "Project Scopes (1:1 mapping only)",
			input: []string{
				"read:project",
				"write:project",
				"delete:project",
			},
			expected: []Scope{
				// Each scope maps to exactly ONE structured scope
				{Resource: optionsv1.ResourceType_RESOURCE_TYPE_PROJECT, Level: optionsv1.AccessLevel_ACCESS_LEVEL_READ},
				{Resource: optionsv1.ResourceType_RESOURCE_TYPE_PROJECT, Level: optionsv1.AccessLevel_ACCESS_LEVEL_WRITE},
				{Resource: optionsv1.ResourceType_RESOURCE_TYPE_PROJECT, Level: optionsv1.AccessLevel_ACCESS_LEVEL_ADMIN},
			},
		},
		{
			name: "Site Scopes (1:1 mapping only)",
			input: []string{
				"read:site",
				"write:site",
				"delete:site",
			},
			expected: []Scope{
				// Each scope maps to exactly ONE structured scope
				{Resource: optionsv1.ResourceType_RESOURCE_TYPE_SITE, Level: optionsv1.AccessLevel_ACCESS_LEVEL_READ},
				{Resource: optionsv1.ResourceType_RESOURCE_TYPE_SITE, Level: optionsv1.AccessLevel_ACCESS_LEVEL_WRITE},
				{Resource: optionsv1.ResourceType_RESOURCE_TYPE_SITE, Level: optionsv1.AccessLevel_ACCESS_LEVEL_ADMIN},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MapOAuthScopesToStructured(tt.input)
			assert.ElementsMatch(t, tt.expected, got)
		})
	}
}

// TestIndividualScopeMappings verifies each individual OAuth scope string maps to exactly one internal structured Scope (1:1 mapping).
func TestIndividualScopeMappings(t *testing.T) {
	// This test verifies 1:1 mapping - each OAuth scope maps to exactly ONE structured scope
	// Hierarchy is handled by RBAC interceptor, not scope expansion

	testCases := map[string][]Scope{
		// Project - 1:1 mapping only
		"read:project": {
			{Resource: optionsv1.ResourceType_RESOURCE_TYPE_PROJECT, Level: optionsv1.AccessLevel_ACCESS_LEVEL_READ},
		},
		"write:project": {
			{Resource: optionsv1.ResourceType_RESOURCE_TYPE_PROJECT, Level: optionsv1.AccessLevel_ACCESS_LEVEL_WRITE},
		},

		// Site - 1:1 mapping only
		"read:site": {
			{Resource: optionsv1.ResourceType_RESOURCE_TYPE_SITE, Level: optionsv1.AccessLevel_ACCESS_LEVEL_READ},
		},
	}

	for scopeStr, expectedScopes := range testCases {
		t.Run(scopeStr, func(t *testing.T) {
			got := MapOAuthScopesToStructured([]string{scopeStr})
			assert.ElementsMatch(t, expectedScopes, got)
		})
	}
}
