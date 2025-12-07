package testutils

import (
	"context"
	"database/sql"

	"github.com/libops/api/internal/db"
)

// MockQuerier is a mock implementation of the db.Querier interface for testing purposes.
type MockQuerier struct {
	GetSiteFunc                                       func(ctx context.Context, publicID string) (db.GetSiteRow, error)
	GetSiteMemberFunc                                 func(ctx context.Context, arg db.GetSiteMemberParams) (db.GetSiteMemberRow, error)
	GetSiteMemberByAccountAndSiteFunc                 func(ctx context.Context, arg db.GetSiteMemberByAccountAndSiteParams) (db.SiteMember, error)
	GetProjectFunc                                    func(ctx context.Context, publicID string) (db.GetProjectRow, error)
	GetProjectByIDFunc                                func(ctx context.Context, id int64) (db.GetProjectByIDRow, error)
	GetProjectMemberFunc                              func(ctx context.Context, arg db.GetProjectMemberParams) (db.GetProjectMemberRow, error)
	GetProjectMemberByAccountAndProjectFunc           func(ctx context.Context, arg db.GetProjectMemberByAccountAndProjectParams) (db.ProjectMember, error)
	GetOrganizationFunc                               func(ctx context.Context, publicID string) (db.GetOrganizationRow, error)
	GetOrganizationByIDFunc                           func(ctx context.Context, id int64) (db.GetOrganizationByIDRow, error)
	GetOrganizationMemberFunc                         func(ctx context.Context, arg db.GetOrganizationMemberParams) (db.GetOrganizationMemberRow, error)
	GetOrganizationMemberByAccountAndOrganizationFunc func(ctx context.Context, arg db.GetOrganizationMemberByAccountAndOrganizationParams) (db.OrganizationMember, error)
	GetAccountFunc                                    func(ctx context.Context, publicID string) (db.GetAccountRow, error)
	GetAccountByIDFunc                                func(ctx context.Context, id int64) (db.GetAccountByIDRow, error)
	ListOrganizationsFunc                             func(ctx context.Context, arg db.ListOrganizationsParams) ([]db.ListOrganizationsRow, error)
	ListAccountOrganizationsFunc                      func(ctx context.Context, arg db.ListAccountOrganizationsParams) ([]db.ListAccountOrganizationsRow, error)
	GetAccountByEmailFunc                             func(ctx context.Context, email string) (db.GetAccountByEmailRow, error)
	CreateOrganizationFunc                            func(ctx context.Context, arg db.CreateOrganizationParams) error
	CreateProjectFunc                                 func(ctx context.Context, arg db.CreateProjectParams) error
	CreateSiteFunc                                    func(ctx context.Context, arg db.CreateSiteParams) error
	GetSiteByProjectAndNameFunc                       func(ctx context.Context, arg db.GetSiteByProjectAndNameParams) (db.GetSiteByProjectAndNameRow, error)
	ListProjectSitesFunc                              func(ctx context.Context, arg db.ListProjectSitesParams) ([]db.ListProjectSitesRow, error)
	ListUserProjectsFunc                              func(ctx context.Context, arg db.ListUserProjectsParams) ([]db.ListUserProjectsRow, error)
	ListUserSitesFunc                                 func(ctx context.Context, arg db.ListUserSitesParams) ([]db.ListUserSitesRow, error)
}

func (m *MockQuerier) GetSite(ctx context.Context, publicID string) (db.GetSiteRow, error) {
	if m.GetSiteFunc != nil {
		return m.GetSiteFunc(ctx, publicID)
	}
	return db.GetSiteRow{}, nil
}

func (m *MockQuerier) GetSiteMember(ctx context.Context, arg db.GetSiteMemberParams) (db.GetSiteMemberRow, error) {
	if m.GetSiteMemberFunc != nil {
		return m.GetSiteMemberFunc(ctx, arg)
	}
	return db.GetSiteMemberRow{}, sql.ErrNoRows
}

func (m *MockQuerier) GetProject(ctx context.Context, publicID string) (db.GetProjectRow, error) {
	if m.GetProjectFunc != nil {
		return m.GetProjectFunc(ctx, publicID)
	}
	return db.GetProjectRow{}, nil
}

func (m *MockQuerier) GetProjectByID(ctx context.Context, id int64) (db.GetProjectByIDRow, error) {
	if m.GetProjectByIDFunc != nil {
		return m.GetProjectByIDFunc(ctx, id)
	}
	return db.GetProjectByIDRow{}, nil
}

func (m *MockQuerier) GetProjectMember(ctx context.Context, arg db.GetProjectMemberParams) (db.GetProjectMemberRow, error) {
	if m.GetProjectMemberFunc != nil {
		return m.GetProjectMemberFunc(ctx, arg)
	}
	return db.GetProjectMemberRow{}, sql.ErrNoRows
}

func (m *MockQuerier) GetOrganization(ctx context.Context, publicID string) (db.GetOrganizationRow, error) {
	if m.GetOrganizationFunc != nil {
		return m.GetOrganizationFunc(ctx, publicID)
	}
	return db.GetOrganizationRow{}, nil
}

func (m *MockQuerier) GetOrganizationByID(ctx context.Context, id int64) (db.GetOrganizationByIDRow, error) {
	if m.GetOrganizationByIDFunc != nil {
		return m.GetOrganizationByIDFunc(ctx, id)
	}
	return db.GetOrganizationByIDRow{}, nil
}

func (m *MockQuerier) GetOrganizationByGCPProjectID(ctx context.Context, gcpProjectID sql.NullString) (db.GetOrganizationByGCPProjectIDRow, error) {
	return db.GetOrganizationByGCPProjectIDRow{}, sql.ErrNoRows
}

func (m *MockQuerier) GetOrganizationMember(ctx context.Context, arg db.GetOrganizationMemberParams) (db.GetOrganizationMemberRow, error) {
	if m.GetOrganizationMemberFunc != nil {
		return m.GetOrganizationMemberFunc(ctx, arg)
	}
	return db.GetOrganizationMemberRow{}, sql.ErrNoRows
}

func (m *MockQuerier) GetAccount(ctx context.Context, publicID string) (db.GetAccountRow, error) {
	if m.GetAccountFunc != nil {
		return m.GetAccountFunc(ctx, publicID)
	}
	return db.GetAccountRow{}, nil
}

func (m *MockQuerier) GetAccountByID(ctx context.Context, id int64) (db.GetAccountByIDRow, error) {
	if m.GetAccountByIDFunc != nil {
		return m.GetAccountByIDFunc(ctx, id)
	}
	return db.GetAccountByIDRow{}, nil
}

func (m *MockQuerier) ListOrganizations(ctx context.Context, arg db.ListOrganizationsParams) ([]db.ListOrganizationsRow, error) {
	if m.ListOrganizationsFunc != nil {
		return m.ListOrganizationsFunc(ctx, arg)
	}
	return nil, nil
}

func (m *MockQuerier) ListAccountOrganizations(ctx context.Context, arg db.ListAccountOrganizationsParams) ([]db.ListAccountOrganizationsRow, error) {
	if m.ListAccountOrganizationsFunc != nil {
		return m.ListAccountOrganizationsFunc(ctx, arg)
	}
	return nil, nil
}

func (m *MockQuerier) GetAccountByEmail(ctx context.Context, email string) (db.GetAccountByEmailRow, error) {
	if m.GetAccountByEmailFunc != nil {
		return m.GetAccountByEmailFunc(ctx, email)
	}
	return db.GetAccountByEmailRow{}, nil
}

// --- Stubs for other methods ---

func (m *MockQuerier) ApproveRelationship(ctx context.Context, arg db.ApproveRelationshipParams) (sql.Result, error) {
	return nil, nil
}
func (m *MockQuerier) ClaimPendingEvents(ctx context.Context, arg db.ClaimPendingEventsParams) (sql.Result, error) {
	return nil, nil
}
func (m *MockQuerier) CleanupExpiredVerificationTokens(ctx context.Context) error { return nil }
func (m *MockQuerier) CleanupOldEvents(ctx context.Context, dateSUB any) error    { return nil }
func (m *MockQuerier) CountOrganizationSecrets(ctx context.Context, organizationID int64) (int64, error) {
	return 0, nil
}
func (m *MockQuerier) CountProjectSecrets(ctx context.Context, projectID int64) (int64, error) {
	return 0, nil
}
func (m *MockQuerier) CountSiteSecrets(ctx context.Context, siteID int64) (int64, error) {
	return 0, nil
}
func (m *MockQuerier) CreateAPIKey(ctx context.Context, arg db.CreateAPIKeyParams) error { return nil }
func (m *MockQuerier) CreateAccount(ctx context.Context, arg db.CreateAccountParams) error {
	return nil
}
func (m *MockQuerier) CreateAuditEvent(ctx context.Context, arg db.CreateAuditEventParams) error {
	return nil
}
func (m *MockQuerier) CreateDeployment(ctx context.Context, arg db.CreateDeploymentParams) error {
	return nil
}
func (m *MockQuerier) CreateDomain(ctx context.Context, arg db.CreateDomainParams) error { return nil }
func (m *MockQuerier) CreateEmailVerificationToken(ctx context.Context, arg db.CreateEmailVerificationTokenParams) error {
	return nil
}
func (m *MockQuerier) CreateOrganization(ctx context.Context, arg db.CreateOrganizationParams) error {
	if m.CreateOrganizationFunc != nil {
		return m.CreateOrganizationFunc(ctx, arg)
	}
	return nil
}
func (m *MockQuerier) CreateOrganizationFirewallRule(ctx context.Context, arg db.CreateOrganizationFirewallRuleParams) error {
	return nil
}
func (m *MockQuerier) CreateOrganizationMember(ctx context.Context, arg db.CreateOrganizationMemberParams) error {
	return nil
}
func (m *MockQuerier) CreateOrganizationSecret(ctx context.Context, arg db.CreateOrganizationSecretParams) (sql.Result, error) {
	return nil, nil
}
func (m *MockQuerier) CreateProject(ctx context.Context, arg db.CreateProjectParams) error {
	if m.CreateProjectFunc != nil {
		return m.CreateProjectFunc(ctx, arg)
	}
	return nil
}
func (m *MockQuerier) CreateProjectFirewallRule(ctx context.Context, arg db.CreateProjectFirewallRuleParams) error {
	return nil
}
func (m *MockQuerier) CreateProjectMember(ctx context.Context, arg db.CreateProjectMemberParams) error {
	return nil
}
func (m *MockQuerier) CreateProjectSecret(ctx context.Context, arg db.CreateProjectSecretParams) (sql.Result, error) {
	return nil, nil
}
func (m *MockQuerier) CreateRelationship(ctx context.Context, arg db.CreateRelationshipParams) (sql.Result, error) {
	return nil, nil
}
func (m *MockQuerier) CreateSite(ctx context.Context, arg db.CreateSiteParams) error {
	if m.CreateSiteFunc != nil {
		return m.CreateSiteFunc(ctx, arg)
	}
	return nil
}
func (m *MockQuerier) CreateSiteFirewallRule(ctx context.Context, arg db.CreateSiteFirewallRuleParams) error {
	return nil
}
func (m *MockQuerier) CreateSiteMember(ctx context.Context, arg db.CreateSiteMemberParams) error {
	return nil
}
func (m *MockQuerier) CreateSiteSecret(ctx context.Context, arg db.CreateSiteSecretParams) (sql.Result, error) {
	return nil, nil
}
func (m *MockQuerier) CreateSshAccess(ctx context.Context, arg db.CreateSshAccessParams) error {
	return nil
}
func (m *MockQuerier) CreateSshKey(ctx context.Context, arg db.CreateSshKeyParams) (sql.Result, error) {
	return nil, nil
}
func (m *MockQuerier) DeleteAPIKey(ctx context.Context, apiKeyUuid string) error       { return nil }
func (m *MockQuerier) DeleteAccount(ctx context.Context, publicID string) error        { return nil }
func (m *MockQuerier) DeleteDeployment(ctx context.Context, deploymentID string) error { return nil }
func (m *MockQuerier) DeleteDomain(ctx context.Context, id int64) error                { return nil }
func (m *MockQuerier) DeleteEmailVerificationToken(ctx context.Context, email string) error {
	return nil
}
func (m *MockQuerier) DeleteOrganization(ctx context.Context, publicID string) error      { return nil }
func (m *MockQuerier) DeleteOrganizationFirewallRule(ctx context.Context, id int64) error { return nil }
func (m *MockQuerier) DeleteOrganizationMember(ctx context.Context, arg db.DeleteOrganizationMemberParams) error {
	return nil
}
func (m *MockQuerier) DeleteOrganizationSecret(ctx context.Context, arg db.DeleteOrganizationSecretParams) error {
	return nil
}
func (m *MockQuerier) DeleteProject(ctx context.Context, publicID string) error      { return nil }
func (m *MockQuerier) DeleteProjectFirewallRule(ctx context.Context, id int64) error { return nil }
func (m *MockQuerier) DeleteProjectMember(ctx context.Context, arg db.DeleteProjectMemberParams) error {
	return nil
}
func (m *MockQuerier) DeleteProjectSecret(ctx context.Context, arg db.DeleteProjectSecretParams) error {
	return nil
}
func (m *MockQuerier) DeleteSite(ctx context.Context, publicID string) error      { return nil }
func (m *MockQuerier) DeleteSiteFirewallRule(ctx context.Context, id int64) error { return nil }
func (m *MockQuerier) DeleteSiteMember(ctx context.Context, arg db.DeleteSiteMemberParams) error {
	return nil
}
func (m *MockQuerier) DeleteSiteSecret(ctx context.Context, arg db.DeleteSiteSecretParams) error {
	return nil
}
func (m *MockQuerier) DeleteSshAccess(ctx context.Context, arg db.DeleteSshAccessParams) error {
	return nil
}
func (m *MockQuerier) DeleteSshKey(ctx context.Context, publicID string) error           { return nil }
func (m *MockQuerier) EnqueueEvent(ctx context.Context, arg db.EnqueueEventParams) error { return nil }
func (m *MockQuerier) GetAPIKeyByID(ctx context.Context, id int64) (db.GetAPIKeyByIDRow, error) {
	return db.GetAPIKeyByIDRow{}, nil
}
func (m *MockQuerier) GetAPIKeyByUUID(ctx context.Context, apiKeyUuid string) (db.GetAPIKeyByUUIDRow, error) {
	return db.GetAPIKeyByUUIDRow{}, nil
}
func (m *MockQuerier) GetAccountByVaultEntityID(ctx context.Context, vaultEntityID sql.NullString) (db.GetAccountByVaultEntityIDRow, error) {
	return db.GetAccountByVaultEntityIDRow{}, nil
}
func (m *MockQuerier) GetActiveAPIKeyByUUID(ctx context.Context, apiKeyUuid string) (db.GetActiveAPIKeyByUUIDRow, error) {
	return db.GetActiveAPIKeyByUUIDRow{}, nil
}
func (m *MockQuerier) GetClaimedEvents(ctx context.Context, processingBy sql.NullString) ([]db.GetClaimedEventsRow, error) {
	return nil, nil
}
func (m *MockQuerier) HasUserRelationshipAccessToOrganization(ctx context.Context, arg db.HasUserRelationshipAccessToOrganizationParams) (bool, error) {
	return false, nil
}
func (m *MockQuerier) HasUserProjectAccessInOrganization(ctx context.Context, arg db.HasUserProjectAccessInOrganizationParams) (bool, error) {
	return false, nil
}
func (m *MockQuerier) HasUserSiteAccessInOrganization(ctx context.Context, arg db.HasUserSiteAccessInOrganizationParams) (bool, error) {
	return false, nil
}
func (m *MockQuerier) HasUserSiteAccessInProject(ctx context.Context, arg db.HasUserSiteAccessInProjectParams) (bool, error) {
	return false, nil
}
func (m *MockQuerier) GetDeployment(ctx context.Context, deploymentID string) (db.Deployment, error) {
	return db.Deployment{}, nil
}
func (m *MockQuerier) GetDomain(ctx context.Context, id int64) (db.Domain, error) {
	return db.Domain{}, nil
}
func (m *MockQuerier) GetDomainByName(ctx context.Context, domain string) (db.Domain, error) {
	return db.Domain{}, nil
}
func (m *MockQuerier) GetEmailVerificationToken(ctx context.Context, arg db.GetEmailVerificationTokenParams) (db.EmailVerificationToken, error) {
	return db.EmailVerificationToken{}, nil
}
func (m *MockQuerier) GetEmailVerificationTokenByEmail(ctx context.Context, email string) (db.EmailVerificationToken, error) {
	return db.EmailVerificationToken{}, nil
}
func (m *MockQuerier) GetLatestSiteDeployment(ctx context.Context, siteID string) (db.Deployment, error) {
	return db.Deployment{}, nil
}
func (m *MockQuerier) GetOrganizationFirewallRule(ctx context.Context, id int64) (db.GetOrganizationFirewallRuleRow, error) {
	return db.GetOrganizationFirewallRuleRow{}, nil
}
func (m *MockQuerier) GetOrganizationMemberByAccountAndOrganization(ctx context.Context, arg db.GetOrganizationMemberByAccountAndOrganizationParams) (db.OrganizationMember, error) {
	if m.GetOrganizationMemberByAccountAndOrganizationFunc != nil {
		return m.GetOrganizationMemberByAccountAndOrganizationFunc(ctx, arg)
	}
	return db.OrganizationMember{}, nil
}
func (m *MockQuerier) GetOrganizationProjectByOrganizationID(ctx context.Context, organizationID int64) (db.GetOrganizationProjectByOrganizationIDRow, error) {
	return db.GetOrganizationProjectByOrganizationIDRow{}, nil
}
func (m *MockQuerier) GetOrganizationSecretByID(ctx context.Context, id int64) (db.GetOrganizationSecretByIDRow, error) {
	return db.GetOrganizationSecretByIDRow{}, nil
}
func (m *MockQuerier) GetOrganizationSecretByName(ctx context.Context, arg db.GetOrganizationSecretByNameParams) (db.GetOrganizationSecretByNameRow, error) {
	return db.GetOrganizationSecretByNameRow{}, nil
}
func (m *MockQuerier) GetOrganizationSecretByPublicID(ctx context.Context, publicID string) (db.GetOrganizationSecretByPublicIDRow, error) {
	return db.GetOrganizationSecretByPublicIDRow{}, nil
}
func (m *MockQuerier) GetProjectByGCPProjectID(ctx context.Context, gcpProjectID sql.NullString) (db.GetProjectByGCPProjectIDRow, error) {
	return db.GetProjectByGCPProjectIDRow{}, nil
}
func (m *MockQuerier) GetProjectFirewallRule(ctx context.Context, id int64) (db.GetProjectFirewallRuleRow, error) {
	return db.GetProjectFirewallRuleRow{}, nil
}
func (m *MockQuerier) GetProjectMemberByAccountAndProject(ctx context.Context, arg db.GetProjectMemberByAccountAndProjectParams) (db.ProjectMember, error) {
	if m.GetProjectMemberByAccountAndProjectFunc != nil {
		return m.GetProjectMemberByAccountAndProjectFunc(ctx, arg)
	}
	return db.ProjectMember{}, nil
}
func (m *MockQuerier) GetProjectSecretByID(ctx context.Context, id int64) (db.GetProjectSecretByIDRow, error) {
	return db.GetProjectSecretByIDRow{}, nil
}
func (m *MockQuerier) GetProjectSecretByName(ctx context.Context, arg db.GetProjectSecretByNameParams) (db.GetProjectSecretByNameRow, error) {
	return db.GetProjectSecretByNameRow{}, nil
}
func (m *MockQuerier) GetProjectSecretByPublicID(ctx context.Context, publicID string) (db.GetProjectSecretByPublicIDRow, error) {
	return db.GetProjectSecretByPublicIDRow{}, nil
}
func (m *MockQuerier) GetProjectWithOrganization(ctx context.Context, publicID string) (db.GetProjectWithOrganizationRow, error) {
	return db.GetProjectWithOrganizationRow{}, nil
}
func (m *MockQuerier) GetQueueStats(ctx context.Context) (db.GetQueueStatsRow, error) {
	return db.GetQueueStatsRow{}, nil
}
func (m *MockQuerier) GetRelationship(ctx context.Context, publicID string) (db.GetRelationshipRow, error) {
	return db.GetRelationshipRow{}, nil
}
func (m *MockQuerier) GetSiteByID(ctx context.Context, id int64) (db.GetSiteByIDRow, error) {
	return db.GetSiteByIDRow{}, nil
}
func (m *MockQuerier) GetSiteByProjectAndName(ctx context.Context, arg db.GetSiteByProjectAndNameParams) (db.GetSiteByProjectAndNameRow, error) {
	if m.GetSiteByProjectAndNameFunc != nil {
		return m.GetSiteByProjectAndNameFunc(ctx, arg)
	}
	return db.GetSiteByProjectAndNameRow{}, nil
}
func (m *MockQuerier) GetSiteFirewallRule(ctx context.Context, id int64) (db.GetSiteFirewallRuleRow, error) {
	return db.GetSiteFirewallRuleRow{}, nil
}
func (m *MockQuerier) GetSiteMemberByAccountAndSite(ctx context.Context, arg db.GetSiteMemberByAccountAndSiteParams) (db.SiteMember, error) {
	if m.GetSiteMemberByAccountAndSiteFunc != nil {
		return m.GetSiteMemberByAccountAndSiteFunc(ctx, arg)
	}
	return db.SiteMember{}, nil
}
func (m *MockQuerier) GetSiteSecretByID(ctx context.Context, id int64) (db.GetSiteSecretByIDRow, error) {
	return db.GetSiteSecretByIDRow{}, nil
}
func (m *MockQuerier) GetSiteSecretByName(ctx context.Context, arg db.GetSiteSecretByNameParams) (db.GetSiteSecretByNameRow, error) {
	return db.GetSiteSecretByNameRow{}, nil
}
func (m *MockQuerier) GetSiteSecretByPublicID(ctx context.Context, publicID string) (db.GetSiteSecretByPublicIDRow, error) {
	return db.GetSiteSecretByPublicIDRow{}, nil
}
func (m *MockQuerier) GetSshAccess(ctx context.Context, arg db.GetSshAccessParams) (db.SshAccess, error) {
	return db.SshAccess{}, nil
}
func (m *MockQuerier) GetSshKey(ctx context.Context, publicID string) (db.GetSshKeyRow, error) {
	return db.GetSshKeyRow{}, nil
}
func (m *MockQuerier) IncrementFailedLoginAttempts(ctx context.Context, id int64) error { return nil }
func (m *MockQuerier) ListAPIKeysByAccount(ctx context.Context, arg db.ListAPIKeysByAccountParams) ([]db.ListAPIKeysByAccountRow, error) {
	return nil, nil
}
func (m *MockQuerier) ListAccountProjects(ctx context.Context, arg db.ListAccountProjectsParams) ([]db.ListAccountProjectsRow, error) {
	return nil, nil
}
func (m *MockQuerier) ListAccountSites(ctx context.Context, arg db.ListAccountSitesParams) ([]db.ListAccountSitesRow, error) {
	return nil, nil
}
func (m *MockQuerier) ListAccountSshAccess(ctx context.Context, arg db.ListAccountSshAccessParams) ([]db.SshAccess, error) {
	return nil, nil
}
func (m *MockQuerier) ListAccounts(ctx context.Context, arg db.ListAccountsParams) ([]db.ListAccountsRow, error) {
	return nil, nil
}
func (m *MockQuerier) ListOrganizationFirewallRules(ctx context.Context, organizationID sql.NullInt64) ([]db.ListOrganizationFirewallRulesRow, error) {
	return nil, nil
}
func (m *MockQuerier) ListOrganizationMembers(ctx context.Context, arg db.ListOrganizationMembersParams) ([]db.ListOrganizationMembersRow, error) {
	return nil, nil
}
func (m *MockQuerier) ListOrganizationProjects(ctx context.Context, arg db.ListOrganizationProjectsParams) ([]db.ListOrganizationProjectsRow, error) {
	return nil, nil
}
func (m *MockQuerier) ListOrganizationRelationships(ctx context.Context, arg db.ListOrganizationRelationshipsParams) ([]db.ListOrganizationRelationshipsRow, error) {
	return nil, nil
}
func (m *MockQuerier) ListOrganizationSecrets(ctx context.Context, arg db.ListOrganizationSecretsParams) ([]db.ListOrganizationSecretsRow, error) {
	return nil, nil
}
func (m *MockQuerier) ListPendingApprovals(ctx context.Context, targetOrganizationID int64) ([]db.ListPendingApprovalsRow, error) {
	return nil, nil
}
func (m *MockQuerier) ListProjectFirewallRules(ctx context.Context, projectID sql.NullInt64) ([]db.ListProjectFirewallRulesRow, error) {
	return nil, nil
}
func (m *MockQuerier) ListProjectMembers(ctx context.Context, arg db.ListProjectMembersParams) ([]db.ListProjectMembersRow, error) {
	return nil, nil
}
func (m *MockQuerier) ListProjectSecrets(ctx context.Context, arg db.ListProjectSecretsParams) ([]db.ListProjectSecretsRow, error) {
	return nil, nil
}
func (m *MockQuerier) ListProjectSites(ctx context.Context, arg db.ListProjectSitesParams) ([]db.ListProjectSitesRow, error) {
	if m.ListProjectSitesFunc != nil {
		return m.ListProjectSitesFunc(ctx, arg)
	}
	return nil, nil
}
func (m *MockQuerier) ListProjects(ctx context.Context, arg db.ListProjectsParams) ([]db.ListProjectsRow, error) {
	return nil, nil
}
func (m *MockQuerier) ListSiteDeployments(ctx context.Context, arg db.ListSiteDeploymentsParams) ([]db.Deployment, error) {
	return nil, nil
}
func (m *MockQuerier) ListSiteDomains(ctx context.Context, arg db.ListSiteDomainsParams) ([]db.Domain, error) {
	return nil, nil
}
func (m *MockQuerier) ListSiteFirewallRules(ctx context.Context, siteID sql.NullInt64) ([]db.ListSiteFirewallRulesRow, error) {
	return nil, nil
}
func (m *MockQuerier) ListSiteMembers(ctx context.Context, arg db.ListSiteMembersParams) ([]db.ListSiteMembersRow, error) {
	return nil, nil
}
func (m *MockQuerier) ListSiteSecrets(ctx context.Context, arg db.ListSiteSecretsParams) ([]db.ListSiteSecretsRow, error) {
	return nil, nil
}
func (m *MockQuerier) ListSiteSshAccess(ctx context.Context, arg db.ListSiteSshAccessParams) ([]db.ListSiteSshAccessRow, error) {
	return nil, nil
}
func (m *MockQuerier) ListSites(ctx context.Context, arg db.ListSitesParams) ([]db.ListSitesRow, error) {
	return nil, nil
}
func (m *MockQuerier) ListSshKeysByAccount(ctx context.Context, publicID string) ([]db.ListSshKeysByAccountRow, error) {
	return nil, nil
}

func (m *MockQuerier) ListUserProjects(ctx context.Context, arg db.ListUserProjectsParams) ([]db.ListUserProjectsRow, error) {
	if m.ListUserProjectsFunc != nil {
		return m.ListUserProjectsFunc(ctx, arg)
	}
	return nil, nil
}

func (m *MockQuerier) ListUserSites(ctx context.Context, arg db.ListUserSitesParams) ([]db.ListUserSitesRow, error) {
	if m.ListUserSitesFunc != nil {
		return m.ListUserSitesFunc(ctx, arg)
	}
	return nil, nil
}

func (m *MockQuerier) MarkEventDeadLetter(ctx context.Context, arg db.MarkEventDeadLetterParams) error {
	return nil
}
func (m *MockQuerier) MarkEventFailed(ctx context.Context, arg db.MarkEventFailedParams) error {
	return nil
}
func (m *MockQuerier) MarkEventSent(ctx context.Context, id int64) error { return nil }
func (m *MockQuerier) RecoverStaleProcessing(ctx context.Context, dateSUB any) error {
	return nil
}
func (m *MockQuerier) RejectRelationship(ctx context.Context, arg db.RejectRelationshipParams) (sql.Result, error) {
	return nil, nil
}
func (m *MockQuerier) ResetFailedLoginAttempts(ctx context.Context, id int64) error { return nil }
func (m *MockQuerier) UpdateAPIKeyActive(ctx context.Context, arg db.UpdateAPIKeyActiveParams) error {
	return nil
}
func (m *MockQuerier) UpdateAPIKeyLastUsed(ctx context.Context, apiKeyUuid string) error { return nil }
func (m *MockQuerier) UpdateAccount(ctx context.Context, arg db.UpdateAccountParams) error {
	return nil
}
func (m *MockQuerier) UpdateDeployment(ctx context.Context, arg db.UpdateDeploymentParams) error {
	return nil
}
func (m *MockQuerier) UpdateOrganization(ctx context.Context, arg db.UpdateOrganizationParams) error {
	return nil
}
func (m *MockQuerier) UpdateOrganizationMember(ctx context.Context, arg db.UpdateOrganizationMemberParams) error {
	return nil
}
func (m *MockQuerier) UpdateOrganizationSecret(ctx context.Context, arg db.UpdateOrganizationSecretParams) error {
	return nil
}
func (m *MockQuerier) UpdateProject(ctx context.Context, arg db.UpdateProjectParams) error {
	return nil
}
func (m *MockQuerier) UpdateProjectMember(ctx context.Context, arg db.UpdateProjectMemberParams) error {
	return nil
}
func (m *MockQuerier) UpdateProjectSecret(ctx context.Context, arg db.UpdateProjectSecretParams) error {
	return nil
}
func (m *MockQuerier) UpdateSite(ctx context.Context, arg db.UpdateSiteParams) error { return nil }
func (m *MockQuerier) UpdateSiteMember(ctx context.Context, arg db.UpdateSiteMemberParams) error {
	return nil
}
func (m *MockQuerier) UpdateSiteSecret(ctx context.Context, arg db.UpdateSiteSecretParams) error {
	return nil
}
func (m *MockQuerier) UpdateSshKey(ctx context.Context, arg db.UpdateSshKeyParams) (sql.Result, error) {
	return nil, nil
}
