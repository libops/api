package auth

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/libops/api/internal/db"
	"github.com/libops/api/internal/testutils"
)

// TestCheckSiteAccess_Hierarchical tests hierarchical access control for site resources.
func TestCheckSiteAccess_Hierarchical(t *testing.T) {
	accountID := int64(1)
	orgID := int64(10)
	projectID := int64(20)
	siteID := int64(30)

	orgPublicID := uuid.New()
	projectPublicID := uuid.New()
	sitePublicID := uuid.New()

	mockDB := &testutils.MockQuerier{
		GetSiteFunc: func(ctx context.Context, pid string) (db.GetSiteRow, error) {
			if pid == sitePublicID.String() {
				return db.GetSiteRow{ID: siteID, ProjectID: projectID, PublicID: sitePublicID.String()}, nil
			}
			return db.GetSiteRow{}, sql.ErrNoRows
		},
		GetProjectByIDFunc: func(ctx context.Context, id int64) (db.GetProjectByIDRow, error) {
			if id == projectID {
				return db.GetProjectByIDRow{ID: projectID, OrganizationID: orgID, PublicID: projectPublicID.String()}, nil
			}
			return db.GetProjectByIDRow{}, sql.ErrNoRows
		},
		GetProjectFunc: func(ctx context.Context, pid string) (db.GetProjectRow, error) {
			if pid == projectPublicID.String() {
				return db.GetProjectRow{ID: projectID, OrganizationID: orgID, PublicID: projectPublicID.String()}, nil
			}
			return db.GetProjectRow{}, sql.ErrNoRows
		},
		GetOrganizationByIDFunc: func(ctx context.Context, id int64) (db.GetOrganizationByIDRow, error) {
			if id == orgID {
				return db.GetOrganizationByIDRow{ID: orgID, PublicID: orgPublicID.String()}, nil
			}
			return db.GetOrganizationByIDRow{}, sql.ErrNoRows
		},
		GetOrganizationFunc: func(ctx context.Context, pid string) (db.GetOrganizationRow, error) {
			if pid == orgPublicID.String() {
				return db.GetOrganizationRow{ID: orgID, PublicID: orgPublicID.String()}, nil
			}
			return db.GetOrganizationRow{}, sql.ErrNoRows
		},
	}

	authorizer := NewAuthorizer(mockDB)
	userInfo := &UserInfo{AccountID: accountID, Email: "user@example.com"}

	t.Run("OrgOwner_AccessSite", func(t *testing.T) {
		mockDB.GetSiteMemberFunc = func(ctx context.Context, arg db.GetSiteMemberParams) (db.GetSiteMemberRow, error) {
			return db.GetSiteMemberRow{}, sql.ErrNoRows
		}
		mockDB.GetProjectMemberFunc = func(ctx context.Context, arg db.GetProjectMemberParams) (db.GetProjectMemberRow, error) {
			return db.GetProjectMemberRow{}, sql.ErrNoRows
		}
		mockDB.GetOrganizationMemberFunc = func(ctx context.Context, arg db.GetOrganizationMemberParams) (db.GetOrganizationMemberRow, error) {
			if arg.OrganizationID == orgID && arg.AccountID == accountID {
				return db.GetOrganizationMemberRow{Role: "owner"}, nil
			}
			return db.GetOrganizationMemberRow{}, sql.ErrNoRows
		}

		err := authorizer.CheckSiteAccess(context.Background(), userInfo, sitePublicID, PermissionWrite)
		assert.NoError(t, err)
	})

	t.Run("ProjectDeveloper_AccessSite", func(t *testing.T) {
		mockDB.GetSiteMemberFunc = func(ctx context.Context, arg db.GetSiteMemberParams) (db.GetSiteMemberRow, error) {
			return db.GetSiteMemberRow{}, sql.ErrNoRows
		}
		mockDB.GetProjectMemberFunc = func(ctx context.Context, arg db.GetProjectMemberParams) (db.GetProjectMemberRow, error) {
			if arg.ProjectID == projectID && arg.AccountID == accountID {
				return db.GetProjectMemberRow{Role: "developer"}, nil
			}
			return db.GetProjectMemberRow{}, sql.ErrNoRows
		}
		mockDB.GetOrganizationMemberFunc = func(ctx context.Context, arg db.GetOrganizationMemberParams) (db.GetOrganizationMemberRow, error) {
			return db.GetOrganizationMemberRow{}, sql.ErrNoRows
		}

		err := authorizer.CheckSiteAccess(context.Background(), userInfo, sitePublicID, PermissionWrite)
		assert.NoError(t, err)
	})

	t.Run("NoMembership_DenySite", func(t *testing.T) {
		mockDB.GetSiteMemberFunc = func(ctx context.Context, arg db.GetSiteMemberParams) (db.GetSiteMemberRow, error) {
			return db.GetSiteMemberRow{}, sql.ErrNoRows
		}
		mockDB.GetProjectMemberFunc = func(ctx context.Context, arg db.GetProjectMemberParams) (db.GetProjectMemberRow, error) {
			return db.GetProjectMemberRow{}, sql.ErrNoRows
		}
		mockDB.GetOrganizationMemberFunc = func(ctx context.Context, arg db.GetOrganizationMemberParams) (db.GetOrganizationMemberRow, error) {
			return db.GetOrganizationMemberRow{}, sql.ErrNoRows
		}

		err := authorizer.CheckSiteAccess(context.Background(), userInfo, sitePublicID, PermissionRead)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "access denied")
	})

	t.Run("OrgOwner_AccessProject", func(t *testing.T) {
		mockDB.GetProjectMemberFunc = func(ctx context.Context, arg db.GetProjectMemberParams) (db.GetProjectMemberRow, error) {
			return db.GetProjectMemberRow{}, sql.ErrNoRows
		}
		mockDB.GetOrganizationMemberFunc = func(ctx context.Context, arg db.GetOrganizationMemberParams) (db.GetOrganizationMemberRow, error) {
			if arg.OrganizationID == orgID && arg.AccountID == accountID {
				return db.GetOrganizationMemberRow{Role: "owner"}, nil
			}
			return db.GetOrganizationMemberRow{}, sql.ErrNoRows
		}

		err := authorizer.CheckProjectAccess(context.Background(), userInfo, projectPublicID, PermissionWrite)
		assert.NoError(t, err)
	})

	t.Run("OrgDeveloper_AllowProjectWrite", func(t *testing.T) {
		mockDB.GetProjectMemberFunc = func(ctx context.Context, arg db.GetProjectMemberParams) (db.GetProjectMemberRow, error) {
			return db.GetProjectMemberRow{}, sql.ErrNoRows
		}
		mockDB.GetOrganizationMemberFunc = func(ctx context.Context, arg db.GetOrganizationMemberParams) (db.GetOrganizationMemberRow, error) {
			if arg.OrganizationID == orgID && arg.AccountID == accountID {
				return db.GetOrganizationMemberRow{Role: "developer"}, nil // Developer role
			}
			return db.GetOrganizationMemberRow{}, sql.ErrNoRows
		}

		err := authorizer.CheckProjectAccess(context.Background(), userInfo, projectPublicID, PermissionWrite)
		assert.NoError(t, err)
	})

	t.Run("OrgDeveloper_ReadSite", func(t *testing.T) {
		mockDB.GetSiteMemberFunc = func(ctx context.Context, arg db.GetSiteMemberParams) (db.GetSiteMemberRow, error) {
			return db.GetSiteMemberRow{}, sql.ErrNoRows
		}
		mockDB.GetProjectMemberFunc = func(ctx context.Context, arg db.GetProjectMemberParams) (db.GetProjectMemberRow, error) {
			return db.GetProjectMemberRow{}, sql.ErrNoRows
		}
		mockDB.GetOrganizationMemberFunc = func(ctx context.Context, arg db.GetOrganizationMemberParams) (db.GetOrganizationMemberRow, error) {
			if arg.OrganizationID == orgID && arg.AccountID == accountID {
				return db.GetOrganizationMemberRow{Role: "developer"}, nil
			}
			return db.GetOrganizationMemberRow{}, sql.ErrNoRows
		}

		err := authorizer.CheckSiteAccess(context.Background(), userInfo, sitePublicID, PermissionRead)
		assert.NoError(t, err)
	})
}
