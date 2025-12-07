package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"

	optionsv1 "github.com/libops/api/proto/libops/v1/options"
)

func TestHasScope(t *testing.T) {
	tests := []struct {
		name           string
		userScopes     []Scope
		required       *optionsv1.ScopeRule
		expectedResult bool
	}{
		{
			name:           "No requirement",
			userScopes:     []Scope{},
			required:       nil,
			expectedResult: true,
		},
		{
			name: "Exact match",
			userScopes: []Scope{
				{Resource: optionsv1.ResourceType_RESOURCE_TYPE_PROJECT, Level: optionsv1.AccessLevel_ACCESS_LEVEL_READ},
			},
			required: &optionsv1.ScopeRule{
				Resource: optionsv1.ResourceType_RESOURCE_TYPE_PROJECT,
				Level:    optionsv1.AccessLevel_ACCESS_LEVEL_READ,
			},
			expectedResult: true,
		},
		{
			name: "Higher level does NOT satisfy lower level (exact match required)",
			userScopes: []Scope{
				{Resource: optionsv1.ResourceType_RESOURCE_TYPE_PROJECT, Level: optionsv1.AccessLevel_ACCESS_LEVEL_WRITE},
			},
			required: &optionsv1.ScopeRule{
				Resource: optionsv1.ResourceType_RESOURCE_TYPE_PROJECT,
				Level:    optionsv1.AccessLevel_ACCESS_LEVEL_READ,
			},
			expectedResult: false,
		},
		{
			name: "Lower level does not satisfy higher level",
			userScopes: []Scope{
				{Resource: optionsv1.ResourceType_RESOURCE_TYPE_PROJECT, Level: optionsv1.AccessLevel_ACCESS_LEVEL_READ},
			},
			required: &optionsv1.ScopeRule{
				Resource: optionsv1.ResourceType_RESOURCE_TYPE_PROJECT,
				Level:    optionsv1.AccessLevel_ACCESS_LEVEL_WRITE,
			},
			expectedResult: false,
		},
		{
			name: "Parent resource does NOT satisfy child requirement (exact match required)",
			userScopes: []Scope{
				{Resource: optionsv1.ResourceType_RESOURCE_TYPE_ORGANIZATION, Level: optionsv1.AccessLevel_ACCESS_LEVEL_READ},
			},
			required: &optionsv1.ScopeRule{
				Resource:          optionsv1.ResourceType_RESOURCE_TYPE_PROJECT,
				Level:             optionsv1.AccessLevel_ACCESS_LEVEL_READ,
				AllowParentAccess: true,
			},
			expectedResult: false,
		},
		{
			name: "Parent resource does NOT satisfy child requirement (if AllowParentAccess is false)",
			userScopes: []Scope{
				{Resource: optionsv1.ResourceType_RESOURCE_TYPE_ORGANIZATION, Level: optionsv1.AccessLevel_ACCESS_LEVEL_READ},
			},
			required: &optionsv1.ScopeRule{
				Resource:          optionsv1.ResourceType_RESOURCE_TYPE_PROJECT,
				Level:             optionsv1.AccessLevel_ACCESS_LEVEL_READ,
				AllowParentAccess: false,
			},
			expectedResult: false,
		},
		{
			name: "Child resource does NOT satisfy parent requirement (Strict Hierarchy - Project -> Org)",
			userScopes: []Scope{
				{Resource: optionsv1.ResourceType_RESOURCE_TYPE_PROJECT, Level: optionsv1.AccessLevel_ACCESS_LEVEL_READ},
			},
			required: &optionsv1.ScopeRule{
				Resource: optionsv1.ResourceType_RESOURCE_TYPE_ORGANIZATION,
				Level:    optionsv1.AccessLevel_ACCESS_LEVEL_READ,
			},
			expectedResult: false,
		},
		{
			name: "Child resource does NOT satisfy parent requirement (Strict Hierarchy - Site -> Org)",
			userScopes: []Scope{
				{Resource: optionsv1.ResourceType_RESOURCE_TYPE_SITE, Level: optionsv1.AccessLevel_ACCESS_LEVEL_READ},
			},
			required: &optionsv1.ScopeRule{
				Resource: optionsv1.ResourceType_RESOURCE_TYPE_ORGANIZATION,
				Level:    optionsv1.AccessLevel_ACCESS_LEVEL_READ,
			},
			expectedResult: false,
		},
		{
			name: "Child resource does NOT satisfy parent requirement for WRITE",
			userScopes: []Scope{
				{Resource: optionsv1.ResourceType_RESOURCE_TYPE_PROJECT, Level: optionsv1.AccessLevel_ACCESS_LEVEL_ADMIN},
			},
			required: &optionsv1.ScopeRule{
				Resource: optionsv1.ResourceType_RESOURCE_TYPE_ORGANIZATION,
				Level:    optionsv1.AccessLevel_ACCESS_LEVEL_WRITE,
			},
			expectedResult: false,
		},
		{
			name: "Mixed scopes - one match",
			userScopes: []Scope{
				{Resource: optionsv1.ResourceType_RESOURCE_TYPE_SITE, Level: optionsv1.AccessLevel_ACCESS_LEVEL_READ},
				{Resource: optionsv1.ResourceType_RESOURCE_TYPE_PROJECT, Level: optionsv1.AccessLevel_ACCESS_LEVEL_READ},
			},
			required: &optionsv1.ScopeRule{
				Resource: optionsv1.ResourceType_RESOURCE_TYPE_PROJECT,
				Level:    optionsv1.AccessLevel_ACCESS_LEVEL_READ,
			},
			expectedResult: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasScope(tt.userScopes, tt.required)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}
