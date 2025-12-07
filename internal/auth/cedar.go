package auth

import (
	_ "embed"
	"fmt"

	"github.com/cedar-policy/cedar-go"
	"github.com/cedar-policy/cedar-go/types"
)

//go:embed policies.cedar
var policiesSource []byte

// CedarEngine manages the Cedar policy engine and evaluation.
type CedarEngine struct {
	policySet *cedar.PolicySet
}

// NewCedarEngine creates a new Cedar engine instance.
func NewCedarEngine() (*CedarEngine, error) {
	ps, err := cedar.NewPolicySetFromBytes("main", policiesSource)
	if err != nil {
		return nil, fmt.Errorf("failed to parse cedar policies: %w", err)
	}

	return &CedarEngine{
		policySet: ps,
	}, nil
}

// Entity types
const (
	TypeUser         = "User"
	TypeOrganization = "Organization"
	TypeProject      = "Project"
	TypeSite         = "Site"
	TypeRole         = "Role"
	TypeAction       = "Action"
)

// Actions
var (
	ActionRead  = types.EntityUID{Type: TypeAction, ID: types.String("read")}
	ActionWrite = types.EntityUID{Type: TypeAction, ID: types.String("write")}
	ActionOwner = types.EntityUID{Type: TypeAction, ID: types.String("owner")}
)

// GraphBuilder helps construct the entity graph for a request.
type GraphBuilder struct {
	UserUID       types.EntityUID
	resourceAttrs map[types.EntityUID]types.RecordMap
	entityParents map[types.EntityUID][]types.EntityUID
}

// NewGraphBuilder creates a new graph builder for a user.
func NewGraphBuilder(userID string) *GraphBuilder {
	return &GraphBuilder{
		UserUID:       types.EntityUID{Type: TypeUser, ID: types.String(userID)},
		resourceAttrs: make(map[types.EntityUID]types.RecordMap),
		entityParents: make(map[types.EntityUID][]types.EntityUID),
	}
}

// AddResource adds a resource and its associated roles to the graph.
func (b *GraphBuilder) AddResource(resourceType, resourceID string, parentUID *types.EntityUID) types.EntityUID {
	resUID := types.EntityUID{Type: types.EntityType(resourceType), ID: types.String(resourceID)}

	// Create Role entities for this resource
	ownerRole := b.addRole(resourceID, "owner")
	devRole := b.addRole(resourceID, "developer")
	viewRole := b.addRole(resourceID, "viewer")

	// Attributes for the resource pointing to its roles
	attrs := types.RecordMap{
		types.String("owner_role"):     ownerRole,
		types.String("developer_role"): devRole,
		types.String("viewer_role"):    viewRole,
	}

	b.resourceAttrs[resUID] = attrs

	// Note: We don't set parents for the Resource entity itself in this model,
	// as inheritance is via Roles.
	// If we wanted, we could add parentUID to parents, but avoiding for now.

	return resUID
}

// addRole creates a role entity and links it to parent roles if they exist.
func (b *GraphBuilder) addRole(resourceID, roleName string) types.EntityUID {
	roleUID := types.EntityUID{
		Type: TypeRole,
		ID:   types.String(fmt.Sprintf("%s:%s", resourceID, roleName)),
	}

	// Initialize parents list if needed
	if _, ok := b.entityParents[roleUID]; !ok {
		b.entityParents[roleUID] = []types.EntityUID{}
	}

	// 1. Role Hierarchy (Owner -> Developer -> Viewer)
	switch roleName {
	case "owner":
		b.addParent(roleUID, types.EntityUID{
			Type: TypeRole,
			ID:   types.String(fmt.Sprintf("%s:%s", resourceID, "developer")),
		})
	case "developer":
		b.addParent(roleUID, types.EntityUID{
			Type: TypeRole,
			ID:   types.String(fmt.Sprintf("%s:%s", resourceID, "viewer")),
		})
	}

	return roleUID
}

// addParent adds a parent to an entity's parent list safely.
func (b *GraphBuilder) addParent(child, parent types.EntityUID) {
	parents := b.entityParents[child]
	for _, p := range parents {
		if p == parent {
			return
		}
	}
	b.entityParents[child] = append(parents, parent)
}

// AddHierarchyLink links a parent resource role to a child resource role.
func (b *GraphBuilder) AddHierarchyLink(parentResID, childResID, roleName string) {
	parentRoleUID := types.EntityUID{Type: TypeRole, ID: types.String(fmt.Sprintf("%s:%s", parentResID, roleName))}
	childRoleUID := types.EntityUID{Type: TypeRole, ID: types.String(fmt.Sprintf("%s:%s", childResID, roleName))}

	// ParentRole implies ChildRole -> ParentRole has ChildRole as a parent
	b.addParent(parentRoleUID, childRoleUID)
}

// AddUserRole adds the user to a specific role.
func (b *GraphBuilder) AddUserRole(resourceID, roleName string) {
	// Normalize role names from DB
	normalizedRole := roleName
	switch roleName {
	case "read":
		normalizedRole = "viewer"
	case "admin":
		normalizedRole = "owner"
	}

	roleUID := types.EntityUID{
		Type: TypeRole,
		ID:   types.String(fmt.Sprintf("%s:%s", resourceID, normalizedRole)),
	}

	b.addParent(b.UserUID, roleUID)
}

// AddSyntheticUserRole adds a user to a role directly.
func (b *GraphBuilder) AddSyntheticUserRole(resourceID, roleName string) {
	b.AddUserRole(resourceID, roleName)
}

// Build constructs the immutable EntityMap.
func (b *GraphBuilder) Build() types.EntityMap {
	entities := make(types.EntityMap)

	// Add User
	userParents := b.entityParents[b.UserUID]
	entities[b.UserUID] = types.Entity{
		UID:        b.UserUID,
		Attributes: types.NewRecord(nil),
		Parents:    types.NewEntityUIDSet(userParents...),
	}

	// Add Resources
	for resUID, attrs := range b.resourceAttrs {
		parents := b.entityParents[resUID]
		entities[resUID] = types.Entity{
			UID:        resUID,
			Attributes: types.NewRecord(attrs),
			Parents:    types.NewEntityUIDSet(parents...),
		}
	}

	// Add Roles (implied from entityParents)
	// We iterate entityParents to find all roles referenced
	// Note: keys in entityParents includes roles and user/resources.
	// We should filter for TypeRole.
	for uid, parents := range b.entityParents {
		if uid.Type == TypeRole {
			entities[uid] = types.Entity{
				UID:        uid,
				Attributes: types.NewRecord(nil),
				Parents:    types.NewEntityUIDSet(parents...),
			}
		}
	}

	return entities
}

// Authorize performs the authorization check.
func (e *CedarEngine) Authorize(principal, action, resource types.EntityUID, entities types.EntityMap) (bool, error) {
	req := types.Request{
		Principal: principal,
		Action:    action,
		Resource:  resource,
		Context:   types.NewRecord(nil),
	}

	decision, _ := e.policySet.IsAuthorized(entities, req) //nolint:staticcheck // Will upgrade when newer cedar-go version is available
	return decision == types.Allow, nil
}

// Helper to convert Permission to Action UID
func PermissionToAction(p Permission) types.EntityUID {
	switch p {
	case PermissionRead:
		return ActionRead
	case PermissionWrite:
		return ActionWrite
	case PermissionOwner, PermissionAdmin:
		return ActionOwner
	default:
		return ActionRead
	}
}
