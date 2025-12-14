package auth

import (
	"fmt"
	"strings"

	optionsv1 "github.com/libops/api/proto/libops/v1/options"
)

// Scope represents a permission scope in the format: resource:access_level
// Examples: "organization:read", "project:write", "site:admin".
type Scope struct {
	Resource optionsv1.ResourceType
	Level    optionsv1.AccessLevel
}

// String returns the string representation of the scope.
func (s Scope) String() string {
	return fmt.Sprintf("%s:%s", resourceTypeToString(s.Resource), accessLevelToString(s.Level))
}

// ParseScope parses a scope string into a Scope struct
// Format: "resource:level" (e.g., "organization:read", "project:write").
func ParseScope(scopeStr string) (Scope, error) {
	parts := strings.Split(scopeStr, ":")
	if len(parts) != 2 {
		return Scope{}, fmt.Errorf("invalid scope format: %s (expected resource:level)", scopeStr)
	}

	resource, err := parseResourceType(parts[0])
	if err != nil {
		return Scope{}, err
	}

	level, err := parseAccessLevel(parts[1])
	if err != nil {
		return Scope{}, err
	}

	return Scope{Resource: resource, Level: level}, nil
}

// ParseScopes parses a list of scope strings.
func ParseScopes(scopeStrs []string) ([]Scope, error) {
	scopes := make([]Scope, 0, len(scopeStrs))
	for _, s := range scopeStrs {
		scope, err := ParseScope(s)
		if err != nil {
			return nil, err
		}
		scopes = append(scopes, scope)
	}
	return scopes, nil
}

// ScopesToStrings converts a list of Scopes to string representations.
func ScopesToStrings(scopes []Scope) []string {
	strs := make([]string, len(scopes))
	for i, s := range scopes {
		strs[i] = s.String()
	}
	return strs
}

func HasScope(userScopes []Scope, required *optionsv1.ScopeRule) bool {
	// No requirement means access is granted
	if required == nil {
		return true
	}

	// Scope check is EXACT matching only - no hierarchy
	// If an API key has a scope, it must exactly match the required resource and level
	// Hierarchy is handled by the RBAC interceptor
	for _, userScope := range userScopes {
		if userScope.Resource == required.Resource && userScope.Level == required.Level {
			return true
		}
	}

	return false
}

// Helper functions for converting between proto enums and strings

// resourceTypeToString converts a ResourceType enum to its string representation.
func resourceTypeToString(rt optionsv1.ResourceType) string {
	switch rt {
	case optionsv1.ResourceType_RESOURCE_TYPE_SYSTEM:
		return "system"
	case optionsv1.ResourceType_RESOURCE_TYPE_ACCOUNT:
		return "account"
	case optionsv1.ResourceType_RESOURCE_TYPE_ORGANIZATION:
		return "organization"
	case optionsv1.ResourceType_RESOURCE_TYPE_PROJECT:
		return "project"
	case optionsv1.ResourceType_RESOURCE_TYPE_SITE:
		return "site"
	default:
		return "unspecified"
	}
}

// parseResourceType parses a string into a ResourceType enum.
func parseResourceType(s string) (optionsv1.ResourceType, error) {
	switch strings.ToLower(s) {
	case "system":
		return optionsv1.ResourceType_RESOURCE_TYPE_SYSTEM, nil
	case "account":
		return optionsv1.ResourceType_RESOURCE_TYPE_ACCOUNT, nil
	case "organization", "org":
		return optionsv1.ResourceType_RESOURCE_TYPE_ORGANIZATION, nil
	case "project":
		return optionsv1.ResourceType_RESOURCE_TYPE_PROJECT, nil
	case "site":
		return optionsv1.ResourceType_RESOURCE_TYPE_SITE, nil
	default:
		return optionsv1.ResourceType_RESOURCE_TYPE_UNSPECIFIED, fmt.Errorf("unknown resource type: %s", s)
	}
}

// accessLevelToString converts an AccessLevel enum to its string representation.
func accessLevelToString(al optionsv1.AccessLevel) string {
	switch al {
	case optionsv1.AccessLevel_ACCESS_LEVEL_READ:
		return "read"
	case optionsv1.AccessLevel_ACCESS_LEVEL_WRITE:
		return "write"
	case optionsv1.AccessLevel_ACCESS_LEVEL_ADMIN:
		return "admin"
	default:
		return "unspecified"
	}
}

// parseAccessLevel parses a string into an AccessLevel enum.
func parseAccessLevel(s string) (optionsv1.AccessLevel, error) {
	switch strings.ToLower(s) {
	case "read":
		return optionsv1.AccessLevel_ACCESS_LEVEL_READ, nil
	case "write":
		return optionsv1.AccessLevel_ACCESS_LEVEL_WRITE, nil
	case "admin":
		return optionsv1.AccessLevel_ACCESS_LEVEL_ADMIN, nil
	default:
		return optionsv1.AccessLevel_ACCESS_LEVEL_UNSPECIFIED, fmt.Errorf("unknown access level: %s", s)
	}
}

// GetAccountScopesForOAuth returns the default scopes for an OAuth login
// OAuth users get NO scopes - authorization is based purely on membership checks
// This is different from API keys which can have restricted scopes.
func GetAccountScopesForOAuth() []Scope {
	// Return empty slice - OAuth users are authorized by membership, not scopes
	// Services must check membership via CheckOrganizationAccess/CheckProjectAccess/CheckSiteAccess
	return []Scope{}
}

// GetAccountScopesForAPIKey returns the scopes for an API key
// If the key has no scopes (empty list), it returns empty scopes - meaning no scope restrictions
// Authorization is then based purely on RBAC (role membership)
// Otherwise, it maps the OAuth scope strings to structured scopes for exact matching.
func GetAccountScopesForAPIKey(scopes []string) []Scope {
	if len(scopes) == 0 {
		// No scopes = no scope restrictions (rely on RBAC only)
		return []Scope{}
	}

	// Has scopes = restricted access - map OAuth scopes to structured scopes
	return MapOAuthScopesToStructured(scopes)
}

// MapOAuthScopesToStructured converts OAuth 2.0 scope strings to structured scopes with 1:1 mapping.
// Each OAuth scope maps to exactly ONE structured scope (resource:level).
// Hierarchy is handled by the RBAC interceptor, not by scope expansion.
func MapOAuthScopesToStructured(oauthScopes []string) []Scope {
	mapping := map[string]Scope{
		// Account scopes
		"read:user":          {Resource: optionsv1.ResourceType_RESOURCE_TYPE_ACCOUNT, Level: optionsv1.AccessLevel_ACCESS_LEVEL_READ},
		"write:user":         {Resource: optionsv1.ResourceType_RESOURCE_TYPE_ACCOUNT, Level: optionsv1.AccessLevel_ACCESS_LEVEL_WRITE},
		"read:organizations": {Resource: optionsv1.ResourceType_RESOURCE_TYPE_ACCOUNT, Level: optionsv1.AccessLevel_ACCESS_LEVEL_READ},

		// Organization scopes
		"read:organization":   {Resource: optionsv1.ResourceType_RESOURCE_TYPE_ORGANIZATION, Level: optionsv1.AccessLevel_ACCESS_LEVEL_READ},
		"write:organization":  {Resource: optionsv1.ResourceType_RESOURCE_TYPE_ORGANIZATION, Level: optionsv1.AccessLevel_ACCESS_LEVEL_WRITE},
		"delete:organization": {Resource: optionsv1.ResourceType_RESOURCE_TYPE_ORGANIZATION, Level: optionsv1.AccessLevel_ACCESS_LEVEL_ADMIN},

		// Project scopes (1:1 mapping - no hierarchy expansion)
		"read:project":   {Resource: optionsv1.ResourceType_RESOURCE_TYPE_PROJECT, Level: optionsv1.AccessLevel_ACCESS_LEVEL_READ},
		"write:project":  {Resource: optionsv1.ResourceType_RESOURCE_TYPE_PROJECT, Level: optionsv1.AccessLevel_ACCESS_LEVEL_WRITE},
		"delete:project": {Resource: optionsv1.ResourceType_RESOURCE_TYPE_PROJECT, Level: optionsv1.AccessLevel_ACCESS_LEVEL_ADMIN},

		// Plural project scopes map to organization level
		"read:projects":   {Resource: optionsv1.ResourceType_RESOURCE_TYPE_ORGANIZATION, Level: optionsv1.AccessLevel_ACCESS_LEVEL_READ},
		"create:projects": {Resource: optionsv1.ResourceType_RESOURCE_TYPE_ORGANIZATION, Level: optionsv1.AccessLevel_ACCESS_LEVEL_WRITE},
		"write:projects":  {Resource: optionsv1.ResourceType_RESOURCE_TYPE_ORGANIZATION, Level: optionsv1.AccessLevel_ACCESS_LEVEL_WRITE},
		"delete:projects": {Resource: optionsv1.ResourceType_RESOURCE_TYPE_ORGANIZATION, Level: optionsv1.AccessLevel_ACCESS_LEVEL_ADMIN},

		// Site scopes (1:1 mapping - no hierarchy expansion)
		"read:site":   {Resource: optionsv1.ResourceType_RESOURCE_TYPE_SITE, Level: optionsv1.AccessLevel_ACCESS_LEVEL_READ},
		"write:site":  {Resource: optionsv1.ResourceType_RESOURCE_TYPE_SITE, Level: optionsv1.AccessLevel_ACCESS_LEVEL_WRITE},
		"delete:site": {Resource: optionsv1.ResourceType_RESOURCE_TYPE_SITE, Level: optionsv1.AccessLevel_ACCESS_LEVEL_ADMIN},

		// Plural site scopes map to project level
		"read:sites":    {Resource: optionsv1.ResourceType_RESOURCE_TYPE_PROJECT, Level: optionsv1.AccessLevel_ACCESS_LEVEL_READ},
		"write:sites":   {Resource: optionsv1.ResourceType_RESOURCE_TYPE_PROJECT, Level: optionsv1.AccessLevel_ACCESS_LEVEL_WRITE},
		"delete:sites":  {Resource: optionsv1.ResourceType_RESOURCE_TYPE_PROJECT, Level: optionsv1.AccessLevel_ACCESS_LEVEL_ADMIN},
		"promote_sites": {Resource: optionsv1.ResourceType_RESOURCE_TYPE_PROJECT, Level: optionsv1.AccessLevel_ACCESS_LEVEL_ADMIN},

		// Member scopes
		"read:members":   {Resource: optionsv1.ResourceType_RESOURCE_TYPE_ORGANIZATION, Level: optionsv1.AccessLevel_ACCESS_LEVEL_READ},
		"write:members":  {Resource: optionsv1.ResourceType_RESOURCE_TYPE_ORGANIZATION, Level: optionsv1.AccessLevel_ACCESS_LEVEL_WRITE},
		"delete:members": {Resource: optionsv1.ResourceType_RESOURCE_TYPE_ORGANIZATION, Level: optionsv1.AccessLevel_ACCESS_LEVEL_ADMIN},

		// Firewall scopes
		"read:firewall":   {Resource: optionsv1.ResourceType_RESOURCE_TYPE_ORGANIZATION, Level: optionsv1.AccessLevel_ACCESS_LEVEL_READ},
		"write:firewall":  {Resource: optionsv1.ResourceType_RESOURCE_TYPE_ORGANIZATION, Level: optionsv1.AccessLevel_ACCESS_LEVEL_WRITE},
		"delete:firewall": {Resource: optionsv1.ResourceType_RESOURCE_TYPE_ORGANIZATION, Level: optionsv1.AccessLevel_ACCESS_LEVEL_ADMIN},

		// System scope
		"admin:system": {Resource: optionsv1.ResourceType_RESOURCE_TYPE_SYSTEM, Level: optionsv1.AccessLevel_ACCESS_LEVEL_ADMIN},
	}

	scopes := make([]Scope, 0, len(oauthScopes))
	seen := make(map[Scope]bool)

	for _, oauthScope := range oauthScopes {
		if scope, ok := mapping[oauthScope]; ok {
			if !seen[scope] {
				scopes = append(scopes, scope)
				seen[scope] = true
			}
		}
	}

	return scopes
}
