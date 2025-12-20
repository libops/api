package onboard

import (
	"context"
	"database/sql"
	"time"

	"github.com/libops/api/db"
)

// SessionManager handles onboarding session operations
type SessionManager struct {
	db db.Querier
}

// NewSessionManager creates a new session manager
func NewSessionManager(querier db.Querier) *SessionManager {
	return &SessionManager{db: querier}
}

// GetOrCreateSession gets an existing incomplete session or creates a new one
func (sm *SessionManager) GetOrCreateSession(ctx context.Context, accountID int64) (*db.GetOnboardingSessionByAccountIDRow, error) {
	// Try to get existing incomplete session
	session, err := sm.db.GetOnboardingSessionByAccountID(ctx, accountID)
	if err == nil {
		// Check if expired
		if session.ExpiresAt.Valid && session.ExpiresAt.Time.Before(time.Now()) {
			// Create new session if expired
			return sm.createNewSession(ctx, accountID)
		}
		return &session, nil
	}

	if err == sql.ErrNoRows {
		// Create new session
		return sm.createNewSession(ctx, accountID)
	}

	return nil, err
}

// createNewSession creates a new onboarding session
func (sm *SessionManager) createNewSession(ctx context.Context, accountID int64) (*db.GetOnboardingSessionByAccountIDRow, error) {
	expiresAt := time.Now().Add(24 * time.Hour)

	_, err := sm.db.CreateOnboardingSession(ctx, db.CreateOnboardingSessionParams{
		AccountID:               accountID,
		OrgName:                 sql.NullString{Valid: false},
		MachineType:             sql.NullString{Valid: false},
		MachinePriceID:          sql.NullString{Valid: false},
		DiskSizeGb:              sql.NullInt32{Valid: false},
		StripeCheckoutSessionID: sql.NullString{Valid: false},
		StripeSubscriptionID:    sql.NullString{Valid: false},
		OrganizationID:          sql.NullInt64{Valid: false},
		ProjectName:             sql.NullString{Valid: false},
		GcpCountry:              sql.NullString{Valid: false},
		GcpRegion:               sql.NullString{Valid: false},
		SiteName:                sql.NullString{Valid: false},
		GithubRepoUrl:           sql.NullString{Valid: false},
		Port:                    sql.NullInt32{Valid: false},
		FirewallIp:              sql.NullString{Valid: false},
		CurrentStep:             sql.NullInt32{Int32: 1, Valid: true},
		Completed:               sql.NullBool{Bool: false, Valid: true},
		ExpiresAt: sql.NullTime{
			Time:  expiresAt,
			Valid: true,
		},
	})

	if err != nil {
		return nil, err
	}

	// Retrieve the newly created session
	session, err := sm.db.GetOnboardingSessionByAccountID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	return &session, nil
}

// UpdateStep1 updates the session with step 1 data (organization name)
func (sm *SessionManager) UpdateStep1(ctx context.Context, sessionID int64, orgName string) error {
	session, err := sm.db.GetOnboardingSession(ctx, "")
	if err != nil {
		return err
	}

	return sm.db.UpdateOnboardingSession(ctx, db.UpdateOnboardingSessionParams{
		OrgName: sql.NullString{
			String: orgName,
			Valid:  true,
		},
		MachineType:             session.MachineType,
		MachinePriceID:          session.MachinePriceID,
		DiskSizeGb:              session.DiskSizeGb,
		StripeCheckoutSessionID: session.StripeCheckoutSessionID,
		StripeCheckoutUrl:       session.StripeCheckoutUrl,
		StripeSubscriptionID:    session.StripeSubscriptionID,
		OrganizationID:          session.OrganizationID,
		ProjectName:             session.ProjectName,
		GcpCountry:              session.GcpCountry,
		GcpRegion:               session.GcpRegion,
		SiteName:                session.SiteName,
		GithubRepoUrl:           session.GithubRepoUrl,
		Port:                    session.Port,
		FirewallIp:              session.FirewallIp,
		CurrentStep:             sql.NullInt32{Int32: 2, Valid: true},
		Completed:               sql.NullBool{Bool: false, Valid: true},
		ID:                      sessionID,
	})
}

// getOrgPublicID safely extracts the organization public ID from the interface{} type
func getOrgPublicID(val interface{}) string {
	if val == nil {
		return ""
	}
	if str, ok := val.(string); ok {
		return str
	}
	if b, ok := val.([]byte); ok {
		return string(b)
	}
	return ""
}

// ToResponse converts a database session to an API response
func ToResponse(session *db.GetOnboardingSessionByAccountIDRow) OnboardingSessionResponse {
	resp := OnboardingSessionResponse{
		SessionID:   session.PublicID,
		CurrentStep: int(session.CurrentStep.Int32),
	}

	if session.OrgName.Valid {
		resp.OrgName = &session.OrgName.String
	}

	// Handle organization_public_id which could be interface{} (nil or string from CASE statement)
	if session.OrganizationPublicID != nil {
		orgPublicID := getOrgPublicID(session.OrganizationPublicID)
		if orgPublicID != "" {
			resp.OrganizationPublicID = &orgPublicID
		}
	}
	if session.MachineType.Valid {
		resp.MachineType = &session.MachineType.String
	}
	if session.DiskSizeGb.Valid {
		diskSize := int(session.DiskSizeGb.Int32)
		resp.DiskSizeGB = &diskSize
	}
	if session.ProjectName.Valid {
		resp.ProjectName = &session.ProjectName.String
	}
	if session.GcpCountry.Valid {
		resp.GCPCountry = &session.GcpCountry.String
	}
	if session.GcpRegion.Valid {
		resp.GCPRegion = &session.GcpRegion.String
	}
	if session.SiteName.Valid {
		resp.SiteName = &session.SiteName.String
	}
	if session.GithubRepoUrl.Valid {
		resp.GitHubRepoURL = &session.GithubRepoUrl.String
	}
	if session.Port.Valid {
		port := int(session.Port.Int32)
		resp.Port = &port
	}
	if session.FirewallIp.Valid {
		resp.FirewallIP = &session.FirewallIp.String
	}
	if session.OrganizationID.Valid {
		resp.OrganizationID = &session.OrganizationID.Int64
	}
	if session.StripeCheckoutSessionID.Valid {
		resp.StripeCheckoutID = &session.StripeCheckoutSessionID.String
	}
	if session.StripeCheckoutUrl.Valid {
		resp.StripeCheckoutURL = &session.StripeCheckoutUrl.String
	}

	return resp
}
