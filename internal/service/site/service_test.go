package site

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/libops/api/internal/auth"
	"github.com/libops/api/internal/db"
	"github.com/libops/api/internal/testutils"
	libopsv1 "github.com/libops/api/proto/libops/v1"
	commonv1 "github.com/libops/api/proto/libops/v1/common"
)

// TestGetSite tests the GetSite method of the SiteService.
func TestGetSite(t *testing.T) {
	projID := uuid.New()
	siteID := uuid.New()

	tests := []struct {
		name         string
		siteID       string
		setupMock    func() *testutils.MockQuerier
		expectedCode connect.Code
		expectedID   string
	}{
		{
			name:   "Success",
			siteID: siteID.String(),
			setupMock: func() *testutils.MockQuerier {
				return &testutils.MockQuerier{
					GetProjectByIDFunc: func(ctx context.Context, id int64) (db.GetProjectByIDRow, error) {
						return db.GetProjectByIDRow{ID: 1, PublicID: projID.String(), OrganizationID: 1}, nil
					},
					GetSiteFunc: func(ctx context.Context, publicID string) (db.GetSiteRow, error) {
						if publicID == siteID.String() {
							return db.GetSiteRow{
								ID:        1,
								ProjectID: 1,
								PublicID:  siteID.String(),
								Name:      "test-site",
								Status:    db.NullSitesStatus{SitesStatus: db.SitesStatusActive, Valid: true},
							}, nil
						}
						return db.GetSiteRow{}, sql.ErrNoRows
					},
					GetSiteMemberFunc: func(ctx context.Context, arg db.GetSiteMemberParams) (db.GetSiteMemberRow, error) {
						if arg.SiteID == 1 && arg.AccountID == 1 {
							return db.GetSiteMemberRow{Role: "owner"}, nil
						}
						return db.GetSiteMemberRow{}, sql.ErrNoRows
					},
					GetProjectMemberFunc: func(ctx context.Context, arg db.GetProjectMemberParams) (db.GetProjectMemberRow, error) {
						return db.GetProjectMemberRow{}, sql.ErrNoRows
					},
					GetOrganizationMemberFunc: func(ctx context.Context, arg db.GetOrganizationMemberParams) (db.GetOrganizationMemberRow, error) {
						return db.GetOrganizationMemberRow{}, sql.ErrNoRows
					},
				}
			},
			expectedCode: 0,
			expectedID:   siteID.String(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDB := tt.setupMock()
			svc := NewSiteService(mockDB)

			// Set up context with authorizer and user info
			authorizer := auth.NewAuthorizer(mockDB)
			ctx := auth.WithAuthorizer(context.Background(), authorizer)
			ctx = context.WithValue(ctx, auth.UserContextKey, &auth.UserInfo{
				AccountID: 1,
				Email:     "test@example.com",
			})

			req := connect.NewRequest(&libopsv1.GetSiteRequest{
				SiteId: tt.siteID,
			})
			resp, err := svc.GetSite(ctx, req)

			if tt.expectedCode != 0 {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedCode, connect.CodeOf(err))
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedID, resp.Msg.Site.SiteId)
				assert.Equal(t, "test-site", resp.Msg.Site.SiteName)
			}
		})
	}
}

// TestCreateSite tests the CreateSite method of the SiteService.
func TestCreateSite(t *testing.T) {
	projID := uuid.New()
	projectInternalID := int64(789)
	accountID := int64(123)

	tests := []struct {
		name           string
		projectID      string
		siteConfig     *commonv1.SiteConfig
		setupMock      func(*testing.T) *testutils.MockQuerier
		wantErr        bool
		wantCode       connect.Code
		validateParams func(*testing.T, db.CreateSiteParams)
	}{
		{
			name:      "creates site with correct parameters",
			projectID: projID.String(),
			siteConfig: &commonv1.SiteConfig{
				SiteName:  "test-site",
				GithubRef: "main",
			},
			setupMock: func(t *testing.T) *testutils.MockQuerier {
				t.Helper()
				sitePublicID := uuid.New().String()
				orgID := int64(456)
				orgPublicID := uuid.New().String()
				return &testutils.MockQuerier{
					GetProjectFunc: func(ctx context.Context, publicID string) (db.GetProjectRow, error) {
						return db.GetProjectRow{
							ID:             projectInternalID,
							PublicID:       projID.String(),
							OrganizationID: orgID,
						}, nil
					},
					CreateSiteFunc: func(ctx context.Context, params db.CreateSiteParams) error {
						return nil
					},
					GetSiteByProjectAndNameFunc: func(ctx context.Context, params db.GetSiteByProjectAndNameParams) (db.GetSiteByProjectAndNameRow, error) {
						return db.GetSiteByProjectAndNameRow{
							ID:        int64(999),
							PublicID:  sitePublicID,
							ProjectID: projectInternalID,
							Name:      "test-site",
							GithubRef: "main",
							Status:    db.NullSitesStatus{SitesStatus: db.SitesStatusProvisioning, Valid: true},
						}, nil
					},
					GetOrganizationByIDFunc: func(ctx context.Context, id int64) (db.GetOrganizationByIDRow, error) {
						return db.GetOrganizationByIDRow{
							ID:       orgID,
							PublicID: orgPublicID,
						}, nil
					},
				}
			},
			wantErr: false,
			validateParams: func(t *testing.T, params db.CreateSiteParams) {
				t.Helper()
				assert.Equal(t, projectInternalID, params.ProjectID)
				assert.Equal(t, "test-site", params.Name)
				assert.Equal(t, "main", params.GithubRef)
				assert.False(t, params.GcpExternalIp.Valid)
				assert.Equal(t, db.SitesStatusProvisioning, params.Status.SitesStatus)
				assert.True(t, params.Status.Valid)
				assert.Equal(t, accountID, params.CreatedBy.Int64)
				assert.True(t, params.CreatedBy.Valid)
				assert.Equal(t, accountID, params.UpdatedBy.Int64)
				assert.True(t, params.UpdatedBy.Valid)
			},
		},
		{
			name:      "returns error for invalid project UUID",
			projectID: "not-a-uuid",
			siteConfig: &commonv1.SiteConfig{
				SiteName: "test-site",
			},
			setupMock: func(t *testing.T) *testutils.MockQuerier {
				t.Helper()
				return &testutils.MockQuerier{}
			},
			wantErr:  true,
			wantCode: connect.CodeInvalidArgument,
		},
		{
			name:       "returns error when site is nil",
			projectID:  projID.String(),
			siteConfig: nil,
			setupMock: func(t *testing.T) *testutils.MockQuerier {
				t.Helper()
				return &testutils.MockQuerier{}
			},
			wantErr:  true,
			wantCode: connect.CodeInvalidArgument,
		},
		{
			name:      "returns error for invalid site name",
			projectID: projID.String(),
			siteConfig: &commonv1.SiteConfig{
				SiteName: "",
			},
			setupMock: func(t *testing.T) *testutils.MockQuerier {
				t.Helper()
				return &testutils.MockQuerier{}
			},
			wantErr:  true,
			wantCode: connect.CodeInvalidArgument,
		},
		{
			name:      "returns error when project not found",
			projectID: projID.String(),
			siteConfig: &commonv1.SiteConfig{
				SiteName: "test-site",
			},
			setupMock: func(t *testing.T) *testutils.MockQuerier {
				t.Helper()
				return &testutils.MockQuerier{
					GetProjectFunc: func(ctx context.Context, publicID string) (db.GetProjectRow, error) {
						return db.GetProjectRow{}, sql.ErrNoRows
					},
				}
			},
			wantErr:  true,
			wantCode: connect.CodeNotFound,
		},
		{
			name:      "returns error when database create fails",
			projectID: projID.String(),
			siteConfig: &commonv1.SiteConfig{
				SiteName: "test-site",
			},
			setupMock: func(t *testing.T) *testutils.MockQuerier {
				t.Helper()
				return &testutils.MockQuerier{
					GetProjectFunc: func(ctx context.Context, publicID string) (db.GetProjectRow, error) {
						return db.GetProjectRow{
							ID:       projectInternalID,
							PublicID: projID.String(),
						}, nil
					},
					CreateSiteFunc: func(ctx context.Context, params db.CreateSiteParams) error {
						return fmt.Errorf("database error")
					},
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedParams db.CreateSiteParams
			mockDB := tt.setupMock(t)

			if mockDB.CreateSiteFunc != nil {
				originalCreate := mockDB.CreateSiteFunc
				mockDB.CreateSiteFunc = func(ctx context.Context, params db.CreateSiteParams) error {
					capturedParams = params
					return originalCreate(ctx, params)
				}
			}

			svc := NewSiteService(mockDB)

			authorizer := auth.NewAuthorizer(mockDB)
			ctx := auth.WithAuthorizer(context.Background(), authorizer)
			ctx = context.WithValue(ctx, auth.UserContextKey, &auth.UserInfo{
				AccountID: accountID,
				Email:     "test@example.com",
			})

			req := connect.NewRequest(&libopsv1.CreateSiteRequest{
				ProjectId: tt.projectID,
				Site:      tt.siteConfig,
			})
			resp, err := svc.CreateSite(ctx, req)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.wantCode != 0 {
					assert.Equal(t, tt.wantCode, connect.CodeOf(err))
				}
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, resp)
			assert.Equal(t, tt.siteConfig.SiteName, resp.Msg.Site.SiteName)

			if tt.validateParams != nil {
				tt.validateParams(t, capturedParams)
			}
		})
	}
}
