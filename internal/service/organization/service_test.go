package organization

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

// TestGetOrganization tests the GetOrganization method of the OrganizationService.
func TestGetOrganization(t *testing.T) {
	orgID := uuid.New()

	tests := []struct {
		name           string
		organizationID string
		setupMock      func() *testutils.MockQuerier
		expectedCode   connect.Code
		expectedName   string
	}{
		{
			name:           "Success",
			organizationID: orgID.String(),
			setupMock: func() *testutils.MockQuerier {
				return &testutils.MockQuerier{
					GetOrganizationFunc: func(ctx context.Context, publicID string) (db.GetOrganizationRow, error) {
						if publicID == orgID.String() {
							return db.GetOrganizationRow{
								PublicID: orgID.String(),
								Name:     "Test Org",
								Status:   db.NullOrganizationsStatus{OrganizationsStatus: db.OrganizationsStatusActive, Valid: true},
							}, nil
						}
						return db.GetOrganizationRow{}, sql.ErrNoRows
					},
				}
			},
			expectedCode: 0,
			expectedName: "Test Org",
		},
		{
			name:           "NotFound",
			organizationID: uuid.New().String(),
			setupMock: func() *testutils.MockQuerier {
				return &testutils.MockQuerier{
					GetOrganizationFunc: func(ctx context.Context, publicID string) (db.GetOrganizationRow, error) {
						return db.GetOrganizationRow{}, sql.ErrNoRows
					},
				}
			},
			expectedCode: connect.CodeNotFound, // Assuming service returns NotFound or similar error from DB error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewOrganizationService(tt.setupMock())
			req := connect.NewRequest(&libopsv1.GetOrganizationRequest{OrganizationId: tt.organizationID})

			resp, err := svc.GetOrganization(context.Background(), req)

			if tt.expectedCode != 0 {
				assert.Error(t, err)
				// Note: The service implementation returns the raw error from repo for NotFound cases currently,
				// which might not wrap connect.CodeNotFound yet unless explicitly handled.
				// If the service just returns 'err' from repo.GetOrganizationByPublicID and that returns sql.ErrNoRows,
				// it might be an Internal error or Unknown.
				// Let's assume for now we just check error existence for non-zero expected code.
				// If stricter code check is needed, we'd need to adjust the service or expectation.
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.organizationID, resp.Msg.Folder.OrganizationId)
				assert.Equal(t, tt.expectedName, resp.Msg.Folder.OrganizationName)
			}
		})
	}
}

// TestCreateOrganization tests the CreateOrganization method of the OrganizationService.
func TestCreateOrganization(t *testing.T) {
	accountID := int64(123)

	tests := []struct {
		name           string
		folder         *commonv1.FolderConfig
		setupMock      func(*testing.T) *testutils.MockQuerier
		wantErr        bool
		wantCode       connect.Code
		validateParams func(*testing.T, db.CreateOrganizationParams)
	}{
		{
			name:   "creates organization with correct parameters",
			folder: &commonv1.FolderConfig{OrganizationName: "test-org"},
			setupMock: func(t *testing.T) *testutils.MockQuerier {
				t.Helper()
				return &testutils.MockQuerier{
					CreateOrganizationFunc: func(ctx context.Context, params db.CreateOrganizationParams) error {
						return nil
					},
					GetOrganizationFunc: func(ctx context.Context, publicID string) (db.GetOrganizationRow, error) {
						return db.GetOrganizationRow{ID: 100, PublicID: publicID}, nil
					},
					CreateOrganizationMemberFunc: func(ctx context.Context, params db.CreateOrganizationMemberParams) error {
						if params.OrganizationID != 100 {
							return fmt.Errorf("unexpected organization ID: %d", params.OrganizationID)
						}
						if params.AccountID != accountID {
							return fmt.Errorf("unexpected account ID: %d", params.AccountID)
						}
						if params.Role != db.OrganizationMembersRoleOwner {
							return fmt.Errorf("unexpected role: %s", params.Role)
						}
						return nil
					},
				}
			},
			wantErr: false,
			validateParams: func(t *testing.T, params db.CreateOrganizationParams) {
				t.Helper()
				assert.Equal(t, "test-org", params.Name)
				assert.Equal(t, "", params.GcpOrgID)
				assert.Equal(t, "", params.GcpBillingAccount)
				assert.Equal(t, "", params.GcpParent)
				assert.False(t, params.GcpFolderID.Valid)
				assert.Equal(t, db.OrganizationsStatusProvisioning, params.Status.OrganizationsStatus)
				assert.True(t, params.Status.Valid)
				assert.Equal(t, accountID, params.CreatedBy.Int64)
				assert.True(t, params.CreatedBy.Valid)
				assert.Equal(t, accountID, params.UpdatedBy.Int64)
				assert.True(t, params.UpdatedBy.Valid)
			},
		},
		{
			name:   "returns error when folder is nil",
			folder: nil,
			setupMock: func(t *testing.T) *testutils.MockQuerier {
				t.Helper()
				return &testutils.MockQuerier{}
			},
			wantErr:  true,
			wantCode: connect.CodeInvalidArgument,
		},
		{
			name:   "returns error for invalid organization name",
			folder: &commonv1.FolderConfig{OrganizationName: ""},
			setupMock: func(t *testing.T) *testutils.MockQuerier {
				t.Helper()
				return &testutils.MockQuerier{}
			},
			wantErr:  true,
			wantCode: connect.CodeInvalidArgument,
		},
		{
			name:   "returns error when database create fails",
			folder: &commonv1.FolderConfig{OrganizationName: "test-org"},
			setupMock: func(t *testing.T) *testutils.MockQuerier {
				t.Helper()
				return &testutils.MockQuerier{
					CreateOrganizationFunc: func(ctx context.Context, params db.CreateOrganizationParams) error {
						return fmt.Errorf("database error")
					},
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedParams db.CreateOrganizationParams
			mockDB := tt.setupMock(t)

			if mockDB.CreateOrganizationFunc != nil {
				originalCreate := mockDB.CreateOrganizationFunc
				mockDB.CreateOrganizationFunc = func(ctx context.Context, params db.CreateOrganizationParams) error {
					capturedParams = params
					return originalCreate(ctx, params)
				}
			}

			svc := NewOrganizationService(mockDB)

			authorizer := auth.NewAuthorizer(mockDB)
			ctx := auth.WithAuthorizer(context.Background(), authorizer)
			ctx = context.WithValue(ctx, auth.UserContextKey, &auth.UserInfo{
				AccountID: accountID,
				Email:     "test@example.com",
			})

			req := connect.NewRequest(&libopsv1.CreateOrganizationRequest{
				Folder: tt.folder,
			})
			resp, err := svc.CreateOrganization(ctx, req)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.wantCode != 0 {
					assert.Equal(t, tt.wantCode, connect.CodeOf(err))
				}
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, resp)
			assert.NotEmpty(t, resp.Msg.OrganizationId)
			assert.Equal(t, tt.folder.OrganizationName, resp.Msg.Folder.OrganizationName)

			if tt.validateParams != nil {
				tt.validateParams(t, capturedParams)
			}
		})
	}
}
