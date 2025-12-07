package project

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

// TestGetProject tests the GetProject method of the ProjectService.
func TestGetProject(t *testing.T) {
	projID := uuid.New()

	tests := []struct {
		name         string
		projectID    string
		setupMock    func() *testutils.MockQuerier
		expectedCode connect.Code
		expectedName string
	}{
		{
			name:      "Success",
			projectID: projID.String(),
			setupMock: func() *testutils.MockQuerier {
				return &testutils.MockQuerier{
					GetProjectFunc: func(ctx context.Context, publicID string) (db.GetProjectRow, error) {
						if publicID == projID.String() {
							return db.GetProjectRow{
								PublicID:          projID.String(),
								Name:              "Test Project",
								OrganizationID:    123,
								Status:            db.NullProjectsStatus{ProjectsStatus: db.ProjectsStatusActive, Valid: true},
								CreateBranchSites: sql.NullBool{Bool: true, Valid: true},
							}, nil
						}
						return db.GetProjectRow{}, sql.ErrNoRows
					},
				}
			},
			expectedCode: 0,
			expectedName: "Test Project",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewProjectService(tt.setupMock())
			req := connect.NewRequest(&libopsv1.GetProjectRequest{ProjectId: tt.projectID})
			resp, err := svc.GetProject(context.Background(), req)

			if tt.expectedCode != 0 {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedCode, connect.CodeOf(err))
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.projectID, resp.Msg.Project.ProjectId)
				assert.Equal(t, tt.expectedName, resp.Msg.Project.ProjectName)
			}
		})
	}
}

// TestCreateProject tests the CreateProject method of the ProjectService.
func TestCreateProject(t *testing.T) {
	orgID := uuid.New()
	accountID := int64(123)
	orgInternalID := int64(456)

	tests := []struct {
		name           string
		organizationID string
		projectConfig  *commonv1.ProjectConfig
		setupMock      func(*testing.T) *testutils.MockQuerier
		wantErr        bool
		wantCode       connect.Code
		validateParams func(*testing.T, db.CreateProjectParams)
	}{
		{
			name:           "creates project with correct parameters",
			organizationID: orgID.String(),
			projectConfig: &commonv1.ProjectConfig{
				ProjectName:       "test-project",
				GithubRepo:        stringPtr("libops/api"),
				Region:            "us-central1",
				Zone:              "us-central1-a",
				MachineType:       "e2-medium",
				DiskSizeGb:        100,
				CreateBranchSites: true,
			},
			setupMock: func(t *testing.T) *testutils.MockQuerier {
				t.Helper()
				return &testutils.MockQuerier{
					GetOrganizationFunc: func(ctx context.Context, publicID string) (db.GetOrganizationRow, error) {
						return db.GetOrganizationRow{
							ID:       orgInternalID,
							PublicID: orgID.String(),
						}, nil
					},
					CreateProjectFunc: func(ctx context.Context, params db.CreateProjectParams) error {
						return nil
					},
				}
			},
			wantErr: false,
			validateParams: func(t *testing.T, params db.CreateProjectParams) {
				t.Helper()
				assert.Equal(t, orgInternalID, params.OrganizationID)
				assert.Equal(t, "test-project", params.Name)
				assert.Equal(t, "libops/api", params.GithubRepository.String)
				assert.True(t, params.GithubRepository.Valid)
				assert.Equal(t, "main", params.GithubBranch.String)
				assert.Equal(t, "us-central1", params.GcpRegion.String)
				assert.True(t, params.GcpRegion.Valid)
				assert.Equal(t, "us-central1-a", params.GcpZone.String)
				assert.True(t, params.GcpZone.Valid)
				assert.Equal(t, "e2-medium", params.MachineType.String)
				assert.True(t, params.MachineType.Valid)
				assert.Equal(t, int32(100), params.DiskSizeGb.Int32)
				assert.True(t, params.DiskSizeGb.Valid)
				assert.Equal(t, "docker-compose.yml", params.ComposeFile.String)
				assert.Equal(t, "generic", params.ApplicationType.String)
				assert.True(t, params.CreateBranchSites.Bool)
				assert.Equal(t, db.ProjectsStatusProvisioning, params.Status.ProjectsStatus)
				assert.Equal(t, accountID, params.CreatedBy.Int64)
				assert.Equal(t, accountID, params.UpdatedBy.Int64)
			},
		},
		{
			name:           "handles nil optional fields correctly",
			organizationID: orgID.String(),
			projectConfig: &commonv1.ProjectConfig{
				ProjectName: "minimal-project",
			},
			setupMock: func(t *testing.T) *testutils.MockQuerier {
				t.Helper()
				return &testutils.MockQuerier{
					GetOrganizationFunc: func(ctx context.Context, publicID string) (db.GetOrganizationRow, error) {
						return db.GetOrganizationRow{ID: orgInternalID, PublicID: orgID.String()}, nil
					},
					CreateProjectFunc: func(ctx context.Context, params db.CreateProjectParams) error {
						return nil
					},
				}
			},
			wantErr: false,
			validateParams: func(t *testing.T, params db.CreateProjectParams) {
				t.Helper()
				assert.False(t, params.GithubRepository.Valid)
				assert.False(t, params.GcpRegion.Valid)
				assert.False(t, params.GcpZone.Valid)
				assert.False(t, params.MachineType.Valid)
				assert.False(t, params.DiskSizeGb.Valid)
			},
		},
		{
			name:           "returns error for invalid organization UUID",
			organizationID: "not-a-uuid",
			projectConfig: &commonv1.ProjectConfig{
				ProjectName: "test-project",
			},
			setupMock: func(t *testing.T) *testutils.MockQuerier {
				t.Helper()
				return &testutils.MockQuerier{}
			},
			wantErr:  true,
			wantCode: connect.CodeInvalidArgument,
		},
		{
			name:           "returns error when project is nil",
			organizationID: orgID.String(),
			projectConfig:  nil,
			setupMock: func(t *testing.T) *testutils.MockQuerier {
				t.Helper()
				return &testutils.MockQuerier{}
			},
			wantErr:  true,
			wantCode: connect.CodeInvalidArgument,
		},
		{
			name:           "returns error for invalid project name",
			organizationID: orgID.String(),
			projectConfig: &commonv1.ProjectConfig{
				ProjectName: "",
			},
			setupMock: func(t *testing.T) *testutils.MockQuerier {
				t.Helper()
				return &testutils.MockQuerier{}
			},
			wantErr:  true,
			wantCode: connect.CodeInvalidArgument,
		},
		{
			name:           "returns error when organization not found",
			organizationID: orgID.String(),
			projectConfig: &commonv1.ProjectConfig{
				ProjectName: "test-project",
			},
			setupMock: func(t *testing.T) *testutils.MockQuerier {
				t.Helper()
				return &testutils.MockQuerier{
					GetOrganizationFunc: func(ctx context.Context, publicID string) (db.GetOrganizationRow, error) {
						return db.GetOrganizationRow{}, sql.ErrNoRows
					},
				}
			},
			wantErr:  true,
			wantCode: connect.CodeNotFound,
		},
		{
			name:           "returns error when database create fails",
			organizationID: orgID.String(),
			projectConfig: &commonv1.ProjectConfig{
				ProjectName: "test-project",
			},
			setupMock: func(t *testing.T) *testutils.MockQuerier {
				t.Helper()
				return &testutils.MockQuerier{
					GetOrganizationFunc: func(ctx context.Context, publicID string) (db.GetOrganizationRow, error) {
						return db.GetOrganizationRow{ID: orgInternalID, PublicID: orgID.String()}, nil
					},
					CreateProjectFunc: func(ctx context.Context, params db.CreateProjectParams) error {
						return fmt.Errorf("database error")
					},
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedParams db.CreateProjectParams
			mockDB := tt.setupMock(t)

			if mockDB.CreateProjectFunc != nil {
				originalCreate := mockDB.CreateProjectFunc
				mockDB.CreateProjectFunc = func(ctx context.Context, params db.CreateProjectParams) error {
					capturedParams = params
					return originalCreate(ctx, params)
				}
			}

			svc := NewProjectService(mockDB)

			authorizer := auth.NewAuthorizer(mockDB, []string{})
			ctx := auth.WithAuthorizer(context.Background(), authorizer)
			ctx = context.WithValue(ctx, auth.UserContextKey, &auth.UserInfo{
				AccountID: accountID,
				Email:     "test@example.com",
			})

			req := connect.NewRequest(&libopsv1.CreateProjectRequest{
				OrganizationId: tt.organizationID,
				Project:        tt.projectConfig,
			})
			resp, err := svc.CreateProject(ctx, req)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.wantCode != 0 {
					assert.Equal(t, tt.wantCode, connect.CodeOf(err))
				}
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, resp)
			assert.Equal(t, tt.projectConfig.ProjectName, resp.Msg.Project.ProjectName)

			if tt.validateParams != nil {
				tt.validateParams(t, capturedParams)
			}
		})
	}
}

// stringPtr is a test helper function that returns a pointer to a string.
func stringPtr(s string) *string {
	return &s
}
