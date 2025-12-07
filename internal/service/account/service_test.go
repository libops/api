package account

import (
	"context"
	"database/sql"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/libops/api/internal/auth"
	"github.com/libops/api/internal/db"
	"github.com/libops/api/internal/testutils"
	libopsv1 "github.com/libops/api/proto/libops/v1"
)

// TestGetAccountByEmail tests the GetAccountByEmail method of the AccountService.
func TestGetAccountByEmail(t *testing.T) {
	expectedAccountID := uuid.New().String()

	tests := []struct {
		name          string
		email         string
		setupContext  func() context.Context
		setupMock     func() *testutils.MockQuerier
		expectedCode  connect.Code
		expectedEmail string
	}{
		{
			name:  "Unauthenticated",
			email: "test@example.com",
			setupContext: func() context.Context {
				return context.Background()
			},
			setupMock: func() *testutils.MockQuerier {
				return &testutils.MockQuerier{}
			},
			expectedCode: connect.CodeUnauthenticated,
		},
		{
			name:  "NoOrganizationMembership",
			email: "test@example.com",
			setupContext: func() context.Context {
				return context.WithValue(context.Background(), auth.UserContextKey, &auth.UserInfo{AccountID: 1})
			},
			setupMock: func() *testutils.MockQuerier {
				return &testutils.MockQuerier{
					ListAccountOrganizationsFunc: func(ctx context.Context, arg db.ListAccountOrganizationsParams) ([]db.ListAccountOrganizationsRow, error) {
						return []db.ListAccountOrganizationsRow{}, nil
					},
				}
			},
			expectedCode: connect.CodePermissionDenied,
		},
		{
			name:  "Success",
			email: "test@example.com",
			setupContext: func() context.Context {
				return context.WithValue(context.Background(), auth.UserContextKey, &auth.UserInfo{AccountID: 1})
			},
			setupMock: func() *testutils.MockQuerier {
				return &testutils.MockQuerier{
					ListAccountOrganizationsFunc: func(ctx context.Context, arg db.ListAccountOrganizationsParams) ([]db.ListAccountOrganizationsRow, error) {
						return []db.ListAccountOrganizationsRow{{Name: "Test Org"}}, nil
					},
					GetAccountByEmailFunc: func(ctx context.Context, email string) (db.GetAccountByEmailRow, error) {
						return db.GetAccountByEmailRow{
							PublicID: expectedAccountID,
							Email:    email,
							Name:     sql.NullString{String: "Test User", Valid: true},
						}, nil
					},
				}
			},
			expectedCode:  0,
			expectedEmail: "test@example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewAccountService(tt.setupMock(), nil)
			req := connect.NewRequest(&libopsv1.GetAccountByEmailRequest{Email: tt.email})

			resp, err := svc.GetAccountByEmail(tt.setupContext(), req)

			if tt.expectedCode != 0 {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedCode, connect.CodeOf(err))
			} else {
				assert.NoError(t, err)
				assert.Equal(t, expectedAccountID, resp.Msg.Account.AccountId)
				assert.Equal(t, tt.expectedEmail, resp.Msg.Account.Email)
			}
		})
	}
}
