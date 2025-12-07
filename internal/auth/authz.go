package auth

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/libops/api/internal/db"
	"github.com/libops/api/internal/gcp"
)

// Permission represents the type of access required.
type Permission string

const (
	PermissionRead  Permission = "read"
	PermissionWrite Permission = "write"
	PermissionOwner Permission = "owner"
	PermissionAdmin Permission = "admin"
)

// ResourceType represents the type of resource being accessed.
type ResourceType string

const (
	ResourceOrganization ResourceType = "organization"
	ResourceProject      ResourceType = "project"
	ResourceSite         ResourceType = "site"
	ResourceAccount      ResourceType = "account"
)

// Role represents a user's role in a resource.
type Role string

const (
	RoleOwner     Role = "owner"
	RoleDeveloper Role = "developer"
	RoleViewer    Role = "viewer"
)

// Authorizer handles authorization checks.
type Authorizer struct {
	db          db.Querier
	adminEmails []string
	cedarEngine *CedarEngine
}

// NewAuthorizer creates a new authorizer.
func NewAuthorizer(querier db.Querier, adminEmails []string) *Authorizer {
	engine, err := NewCedarEngine()
	if err != nil {
		// In a real app we might want to panic or return error, but signature is fixed.
		// Ensure we panic if policy is broken to prevent insecure startup.
		panic(fmt.Errorf("failed to initialize cedar engine: %w", err))
	}

	return &Authorizer{
		db:          querier,
		adminEmails: adminEmails,
		cedarEngine: engine,
	}
}

// IsAdmin checks if the user is a libops admin.
func (a *Authorizer) IsAdmin(ctx context.Context, userInfo *UserInfo) bool {
	if userInfo == nil {
		return false
	}

	for _, adminEmail := range a.adminEmails {
		if strings.EqualFold(userInfo.Email, adminEmail) {
			return true
		}
	}

	// Platform service accounts are NOT global admins
	// They only have admin rights for their specific organization
	return false
}

// IsPlatformServiceAccount checks if the user is a platform service account.
func (a *Authorizer) IsPlatformServiceAccount(ctx context.Context, userInfo *UserInfo) bool {
	if userInfo == nil {
		return false
	}
	return gcp.IsPlatformServiceAccount(userInfo.Email)
}

// GetServiceAccountOrganizationID returns the organization ID that a platform service account belongs to
// Returns 0 if not a platform service account or if organization not found.
func (a *Authorizer) GetServiceAccountOrganizationID(ctx context.Context, userInfo *UserInfo) (int64, error) {
	if !a.IsPlatformServiceAccount(ctx, userInfo) {
		return 0, fmt.Errorf("not a platform service account")
	}

	projectID := gcp.ExtractProjectIDFromServiceAccount(userInfo.Email)
	if projectID == "" {
		return 0, fmt.Errorf("failed to extract project ID from service account email")
	}

	organization, err := a.db.GetOrganizationByGCPProjectID(ctx, sql.NullString{
		String: projectID,
		Valid:  true,
	})
	if err != nil {
		return 0, fmt.Errorf("organization not found for project ID %s: %w", projectID, err)
	}

	return organization.ID, nil
}

// GetAccountID gets the internal account ID from context.
func (a *Authorizer) GetAccountID(ctx context.Context, userInfo *UserInfo) (int64, error) {
	if userInfo == nil {
		return 0, fmt.Errorf("user not authenticated")
	}

	if userInfo.AccountID == 0 {
		return 0, fmt.Errorf("account ID not found in token")
	}

	return userInfo.AccountID, nil
}

// CheckOrganizationAccess checks if user has access to a organization (by public_id UUID).
func (a *Authorizer) CheckOrganizationAccess(ctx context.Context, userInfo *UserInfo, organizationPublicID uuid.UUID, required Permission) error {
	// Global admins can do anything
	if a.IsAdmin(ctx, userInfo) {
		return nil
	}

	organization, err := a.db.GetOrganization(ctx, organizationPublicID.String())
	if err != nil {
		return fmt.Errorf("organization not found: %w", err)
	}

	// Platform service accounts have admin access to their own organization only
	if a.IsPlatformServiceAccount(ctx, userInfo) {
		saOrganizationID, err := a.GetServiceAccountOrganizationID(ctx, userInfo)
		if err != nil {
			return fmt.Errorf("failed to get service account organization: %w", err)
		}

		if saOrganizationID != organization.ID {
			return fmt.Errorf("access denied: service account can only access its own organization")
		}

		// Platform service accounts have owner-level access to their organization
		return nil
	}

	accountID, err := a.GetAccountID(ctx, userInfo)
	if err != nil {
		return fmt.Errorf("unauthorized: %w", err)
	}

	// Initialize Cedar Graph Builder
	builder := NewGraphBuilder(fmt.Sprint(accountID))
	orgUID := builder.AddResource(TypeOrganization, fmt.Sprint(organization.ID), nil)

	// 1. Direct Membership
	member, err := a.db.GetOrganizationMember(ctx, db.GetOrganizationMemberParams{
		OrganizationID: organization.ID,
		AccountID:      accountID,
	})
	if err == nil {
		builder.AddUserRole(fmt.Sprint(organization.ID), string(member.Role))
	}

	// 2. Relationship Access
	relationships, err := a.db.ListOrganizationRelationships(ctx, db.ListOrganizationRelationshipsParams{
		SourceOrganizationID: organization.ID,
		TargetOrganizationID: organization.ID,
	})
	if err == nil {
		for _, rel := range relationships {
			if rel.Status == "approved" && rel.TargetOrganizationID == organization.ID {
				sourceMember, err := a.db.GetOrganizationMember(ctx, db.GetOrganizationMemberParams{
					OrganizationID: rel.SourceOrganizationID,
					AccountID:      accountID,
				})
				if err == nil {
					builder.AddResource(TypeOrganization, fmt.Sprint(rel.SourceOrganizationID), nil)
					builder.AddUserRole(fmt.Sprint(rel.SourceOrganizationID), string(sourceMember.Role))

					builder.AddHierarchyLink(fmt.Sprint(rel.SourceOrganizationID), fmt.Sprint(organization.ID), "owner")
					builder.AddHierarchyLink(fmt.Sprint(rel.SourceOrganizationID), fmt.Sprint(organization.ID), "developer")
					builder.AddHierarchyLink(fmt.Sprint(rel.SourceOrganizationID), fmt.Sprint(organization.ID), "viewer")
				}
			}
		}
	}

	// 3. Upwards Inheritance (Read Access Only)
	if required == PermissionRead {
		hasProjectAccess, _ := a.db.HasUserProjectAccessInOrganization(ctx, db.HasUserProjectAccessInOrganizationParams{
			TargetOrganizationID: organization.ID,
			AccountID:            accountID,
			OrganizationID:       organization.ID,
		})
		if hasProjectAccess {
			builder.AddSyntheticUserRole(fmt.Sprint(organization.ID), "viewer")
		} else {
			hasSiteAccess, _ := a.db.HasUserSiteAccessInOrganization(ctx, db.HasUserSiteAccessInOrganizationParams{
				TargetOrganizationID: organization.ID,
				AccountID:            accountID,
				OrganizationID:       organization.ID,
			})
			if hasSiteAccess {
				builder.AddSyntheticUserRole(fmt.Sprint(organization.ID), "viewer")
			}
		}
	}

	// Evaluate Policy
	ok, err := a.cedarEngine.Authorize(builder.UserUID, PermissionToAction(required), orgUID, builder.Build())
	if err != nil {
		return fmt.Errorf("authorization error: %w", err)
	}
	if !ok {
		return fmt.Errorf("access denied")
	}

	return nil
}

// CheckProjectAccess checks if user has access to a project (by public_id UUID).
func (a *Authorizer) CheckProjectAccess(ctx context.Context, userInfo *UserInfo, projectPublicID uuid.UUID, required Permission) error {
	// Global admins can do anything
	if a.IsAdmin(ctx, userInfo) {
		return nil
	}

	project, err := a.db.GetProject(ctx, projectPublicID.String())
	if err != nil {
		return fmt.Errorf("project not found: %w", err)
	}

	// Platform service accounts can access projects in their organization
	if a.IsPlatformServiceAccount(ctx, userInfo) {
		saOrganizationID, err := a.GetServiceAccountOrganizationID(ctx, userInfo)
		if err != nil {
			return fmt.Errorf("failed to get service account organization: %w", err)
		}

		if saOrganizationID != project.OrganizationID {
			return fmt.Errorf("access denied: service account can only access projects in its own organization")
		}

		// Platform service accounts have owner-level access
		return nil
	}

	accountID, err := a.GetAccountID(ctx, userInfo)
	if err != nil {
		return fmt.Errorf("unauthorized: %w", err)
	}

	builder := NewGraphBuilder(fmt.Sprint(accountID))
	orgUID := builder.AddResource(TypeOrganization, fmt.Sprint(project.OrganizationID), nil)
	projUID := builder.AddResource(TypeProject, fmt.Sprint(project.ID), &orgUID)

	// Link Organization roles to Project roles (Downwards inheritance)
	builder.AddHierarchyLink(fmt.Sprint(project.OrganizationID), fmt.Sprint(project.ID), "owner")
	builder.AddHierarchyLink(fmt.Sprint(project.OrganizationID), fmt.Sprint(project.ID), "developer")
	builder.AddHierarchyLink(fmt.Sprint(project.OrganizationID), fmt.Sprint(project.ID), "viewer")

	// 1. Direct Project Membership
	projectMember, err := a.db.GetProjectMember(ctx, db.GetProjectMemberParams{
		ProjectID: project.ID,
		AccountID: accountID,
	})
	if err == nil {
		builder.AddUserRole(fmt.Sprint(project.ID), string(projectMember.Role))
	}

	// 2. Organization Membership (Downwards)
	orgMember, err := a.db.GetOrganizationMember(ctx, db.GetOrganizationMemberParams{
		OrganizationID: project.OrganizationID,
		AccountID:      accountID,
	})
	if err == nil {
		builder.AddUserRole(fmt.Sprint(project.OrganizationID), string(orgMember.Role))
	}

	// 3. Relationship Access (via Org)
	relationships, err := a.db.ListOrganizationRelationships(ctx, db.ListOrganizationRelationshipsParams{
		SourceOrganizationID: project.OrganizationID,
		TargetOrganizationID: project.OrganizationID,
	})
	if err == nil {
		for _, rel := range relationships {
			if rel.Status == "approved" && rel.TargetOrganizationID == project.OrganizationID {
				sourceMember, err := a.db.GetOrganizationMember(ctx, db.GetOrganizationMemberParams{
					OrganizationID: rel.SourceOrganizationID,
					AccountID:      accountID,
				})
				if err == nil {
					builder.AddResource(TypeOrganization, fmt.Sprint(rel.SourceOrganizationID), nil)
					builder.AddUserRole(fmt.Sprint(rel.SourceOrganizationID), string(sourceMember.Role))

					builder.AddHierarchyLink(fmt.Sprint(rel.SourceOrganizationID), fmt.Sprint(project.OrganizationID), "owner")
					builder.AddHierarchyLink(fmt.Sprint(rel.SourceOrganizationID), fmt.Sprint(project.OrganizationID), "developer")
					builder.AddHierarchyLink(fmt.Sprint(rel.SourceOrganizationID), fmt.Sprint(project.OrganizationID), "viewer")
				}
			}
		}
	}

	// 4. Upwards Inheritance from Site (Read Only)
	if required == PermissionRead {
		hasSiteAccess, _ := a.db.HasUserSiteAccessInProject(ctx, db.HasUserSiteAccessInProjectParams{
			ID:        project.ID, // Target project for relationship checking
			AccountID: accountID,
			ProjectID: project.ID,
		})
		if hasSiteAccess {
			builder.AddSyntheticUserRole(fmt.Sprint(project.ID), "viewer")
		}
	}

	// Evaluate Policy
	ok, err := a.cedarEngine.Authorize(builder.UserUID, PermissionToAction(required), projUID, builder.Build())
	if err != nil {
		return fmt.Errorf("authorization error: %w", err)
	}
	if !ok {
		return fmt.Errorf("access denied")
	}

	return nil
}

// CheckSiteAccess checks if user has access to a site (by public_id UUID).
func (a *Authorizer) CheckSiteAccess(ctx context.Context, userInfo *UserInfo, sitePublicID uuid.UUID, required Permission) error {
	// Global admins can do anything
	if a.IsAdmin(ctx, userInfo) {
		return nil
	}

	site, err := a.db.GetSite(ctx, sitePublicID.String())
	if err != nil {
		return fmt.Errorf("site not found: %w", err)
	}

	project, err := a.db.GetProjectByID(ctx, site.ProjectID)
	if err != nil {
		return fmt.Errorf("project not found: %w", err)
	}

	// Platform service accounts can access sites in their organization
	if a.IsPlatformServiceAccount(ctx, userInfo) {
		saOrganizationID, err := a.GetServiceAccountOrganizationID(ctx, userInfo)
		if err != nil {
			return fmt.Errorf("failed to get service account organization: %w", err)
		}

		if saOrganizationID != project.OrganizationID {
			return fmt.Errorf("access denied: service account can only access sites in its own organization")
		}

		// Platform service accounts have owner-level access
		return nil
	}

	accountID, err := a.GetAccountID(ctx, userInfo)
	if err != nil {
		return fmt.Errorf("unauthorized: %w", err)
	}

	builder := NewGraphBuilder(fmt.Sprint(accountID))
	orgUID := builder.AddResource(TypeOrganization, fmt.Sprint(project.OrganizationID), nil)
	projUID := builder.AddResource(TypeProject, fmt.Sprint(project.ID), &orgUID)
	siteUID := builder.AddResource(TypeSite, fmt.Sprint(site.ID), &projUID)

	// Link Organization roles to Project roles
	builder.AddHierarchyLink(fmt.Sprint(project.OrganizationID), fmt.Sprint(project.ID), "owner")
	builder.AddHierarchyLink(fmt.Sprint(project.OrganizationID), fmt.Sprint(project.ID), "developer")
	builder.AddHierarchyLink(fmt.Sprint(project.OrganizationID), fmt.Sprint(project.ID), "viewer")

	// Link Project roles to Site roles
	builder.AddHierarchyLink(fmt.Sprint(project.ID), fmt.Sprint(site.ID), "owner")
	builder.AddHierarchyLink(fmt.Sprint(project.ID), fmt.Sprint(site.ID), "developer")
	builder.AddHierarchyLink(fmt.Sprint(project.ID), fmt.Sprint(site.ID), "viewer")

	// 1. Direct Site Membership
	siteMember, err := a.db.GetSiteMember(ctx, db.GetSiteMemberParams{
		SiteID:    site.ID,
		AccountID: accountID,
	})
	if err == nil {
		builder.AddUserRole(fmt.Sprint(site.ID), string(siteMember.Role))
	}

	// 2. Project Membership (Downwards)
	projectMember, err := a.db.GetProjectMember(ctx, db.GetProjectMemberParams{
		ProjectID: site.ProjectID,
		AccountID: accountID,
	})
	if err == nil {
		builder.AddUserRole(fmt.Sprint(site.ProjectID), string(projectMember.Role))
	}

	// 3. Organization Membership (Downwards)
	orgMember, err := a.db.GetOrganizationMember(ctx, db.GetOrganizationMemberParams{
		OrganizationID: project.OrganizationID,
		AccountID:      accountID,
	})
	if err == nil {
		builder.AddUserRole(fmt.Sprint(project.OrganizationID), string(orgMember.Role))
	}

	// 4. Relationship Access (via Org)
	relationships, err := a.db.ListOrganizationRelationships(ctx, db.ListOrganizationRelationshipsParams{
		SourceOrganizationID: project.OrganizationID,
		TargetOrganizationID: project.OrganizationID,
	})
	if err == nil {
		for _, rel := range relationships {
			if rel.Status == "approved" && rel.TargetOrganizationID == project.OrganizationID {
				sourceMember, err := a.db.GetOrganizationMember(ctx, db.GetOrganizationMemberParams{
					OrganizationID: rel.SourceOrganizationID,
					AccountID:      accountID,
				})
				if err == nil {
					builder.AddResource(TypeOrganization, fmt.Sprint(rel.SourceOrganizationID), nil)
					builder.AddUserRole(fmt.Sprint(rel.SourceOrganizationID), string(sourceMember.Role))

					builder.AddHierarchyLink(fmt.Sprint(rel.SourceOrganizationID), fmt.Sprint(project.OrganizationID), "owner")
					builder.AddHierarchyLink(fmt.Sprint(rel.SourceOrganizationID), fmt.Sprint(project.OrganizationID), "developer")
					builder.AddHierarchyLink(fmt.Sprint(rel.SourceOrganizationID), fmt.Sprint(project.OrganizationID), "viewer")
				}
			}
		}
	}

	// Evaluate Policy
	ok, err := a.cedarEngine.Authorize(builder.UserUID, PermissionToAction(required), siteUID, builder.Build())
	if err != nil {
		return fmt.Errorf("authorization error: %w", err)
	}
	if !ok {
		return fmt.Errorf("access denied")
	}

	return nil
}

// CheckAccountAccess checks if user can access an account (by public_id UUID)
// Users can always read/write their own account.
func (a *Authorizer) CheckAccountAccess(ctx context.Context, userInfo *UserInfo, targetAccountPublicID uuid.UUID, required Permission) error {
	// Admins can do anything
	if a.IsAdmin(ctx, userInfo) {
		return nil
	}

	accountID, err := a.GetAccountID(ctx, userInfo)
	if err != nil {
		return fmt.Errorf("unauthorized: %w", err)
	}

	targetAccount, err := a.db.GetAccount(ctx, targetAccountPublicID.String())
	if err != nil {
		return fmt.Errorf("account not found: %w", err)
	}

	// Users can access their own account
	if accountID == targetAccount.ID {
		return nil
	}

	return fmt.Errorf("access denied: can only access your own account")
}

// RequireAuthentication checks that a user is authenticated.
func (a *Authorizer) RequireAuthentication(ctx context.Context) (*UserInfo, error) {
	userInfo, ok := GetUserFromContext(ctx)
	if !ok {
		return nil, fmt.Errorf("not authenticated")
	}
	return userInfo, nil
}
