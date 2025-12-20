package reconciliation

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"

	"connectrpc.com/connect"

	"github.com/libops/api/db"
	libopsv1 "github.com/libops/api/proto/libops/v1"
	"github.com/libops/api/proto/libops/v1/libopsv1connect"
)

// AdminReconciliationService implements the admin reconciliation API.
// This service is called by Cloud Run reconciliation services with GSA authentication.
type AdminReconciliationService struct {
	mainQuerier    db.Querier // Main API database
	controlQuerier db.Querier // Control-plane database
}

// Compile-time check.
var _ libopsv1connect.AdminReconciliationServiceHandler = (*AdminReconciliationService)(nil)

// NewAdminReconciliationService creates a new admin reconciliation service.
func NewAdminReconciliationService(mainQuerier db.Querier, controlQuerier db.Querier) *AdminReconciliationService {
	return &AdminReconciliationService{
		mainQuerier:    mainQuerier,
		controlQuerier: controlQuerier,
	}
}

// GetReconciliationRun fetches reconciliation run details from control-plane database.
func (s *AdminReconciliationService) GetReconciliationRun(
	ctx context.Context,
	req *connect.Request[libopsv1.GetReconciliationRunRequest],
) (*connect.Response[libopsv1.GetReconciliationRunResponse], error) {
	runID := req.Msg.RunId
	if runID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("run_id is required"))
	}

	// Query control-plane database for run details
	query := `SELECT run_id, run_type, reconciliation_type, modules, target_site_ids, event_ids,
	                 organization_id, project_id, site_id, status
	          FROM reconciliations
	          WHERE run_id = ?`

	var run libopsv1.GetReconciliationRunResponse
	var modulesJSON, targetSiteIDsJSON, eventIDsJSON []byte
	var orgID, projID, siteID *int64
	var reconciliationType *string

	rows, err := s.controlQuerier.(*db.Queries).GetDB().QueryContext(ctx, query, runID)
	if err != nil {
		slog.Error("failed to query reconciliation run", "run_id", runID, "error", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to query run: %w", err))
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("reconciliation run not found: %s", runID))
	}

	err = rows.Scan(
		&run.RunId,
		&run.RunType,
		&reconciliationType,
		&modulesJSON,
		&targetSiteIDsJSON,
		&eventIDsJSON,
		&orgID,
		&projID,
		&siteID,
		&run.Status,
	)
	if err != nil {
		slog.Error("failed to scan reconciliation run", "run_id", runID, "error", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to scan run: %w", err))
	}

	// Parse JSON fields
	if modulesJSON != nil {
		if err := json.Unmarshal(modulesJSON, &run.Modules); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to parse modules: %w", err))
		}
	}

	if targetSiteIDsJSON != nil {
		if err := json.Unmarshal(targetSiteIDsJSON, &run.TargetSiteIds); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to parse target_site_ids: %w", err))
		}
	}

	if eventIDsJSON != nil {
		if err := json.Unmarshal(eventIDsJSON, &run.EventIds); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to parse event_ids: %w", err))
		}
	}

	// Set optional fields
	if reconciliationType != nil {
		run.ReconciliationType = reconciliationType
	}
	if orgID != nil {
		run.OrganizationId = orgID
	}
	if projID != nil {
		run.ProjectId = projID
	}
	if siteID != nil {
		run.SiteId = siteID
	}

	return connect.NewResponse(&run), nil
}

// UpdateReconciliationStatus updates the reconciliation run status in control-plane database.
func (s *AdminReconciliationService) UpdateReconciliationStatus(
	ctx context.Context,
	req *connect.Request[libopsv1.UpdateReconciliationStatusRequest],
) (*connect.Response[libopsv1.UpdateReconciliationStatusResponse], error) {
	runID := req.Msg.RunId
	status := req.Msg.Status
	errorMsg := ""
	if req.Msg.ErrorMessage != nil {
		errorMsg = *req.Msg.ErrorMessage
	}

	if runID == "" || status == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("run_id and status are required"))
	}

	// Update control-plane database
	query := `UPDATE reconciliations
	          SET status = ?,
	              started_at = CASE WHEN ? = 'running' AND started_at IS NULL THEN CURRENT_TIMESTAMP ELSE started_at END,
	              completed_at = CASE WHEN ? IN ('completed', 'failed') THEN CURRENT_TIMESTAMP ELSE completed_at END,
	              error_message = ?
	          WHERE run_id = ?`

	_, err := s.controlQuerier.(*db.Queries).GetDB().ExecContext(ctx, query, status, status, status, errorMsg, runID)
	if err != nil {
		slog.Error("failed to update reconciliation status",
			"run_id", runID,
			"status", status,
			"error", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update status: %w", err))
	}

	slog.Info("reconciliation status updated",
		"run_id", runID,
		"status", status)

	return connect.NewResponse(&libopsv1.UpdateReconciliationStatusResponse{
		Success: true,
	}), nil
}

// GenerateTerraformVars generates terraform variables JSON from database state.
func (s *AdminReconciliationService) GenerateTerraformVars(
	ctx context.Context,
	req *connect.Request[libopsv1.GenerateTerraformVarsRequest],
) (*connect.Response[libopsv1.GenerateTerraformVarsResponse], error) {
	orgID := req.Msg.OrganizationId
	projectID := req.Msg.ProjectId
	siteID := req.Msg.SiteId

	// Build the complete terraform variable structure
	tfvars := map[string]interface{}{
		"organizations": make(map[string]interface{}),
		"projects":      make(map[string]interface{}),
		"sites":         make(map[string]interface{}),
	}

	// Determine scope and query accordingly
	if siteID != nil {
		// Site scope - query just this site
		if err := s.addSiteToTfvars(ctx, *siteID, tfvars); err != nil {
			return nil, err
		}
	} else if projectID != nil {
		// Project scope - query this project and all its sites
		if err := s.addProjectToTfvars(ctx, *projectID, tfvars); err != nil {
			return nil, err
		}
		if err := s.addProjectSitesToTfvars(ctx, *projectID, tfvars); err != nil {
			return nil, err
		}
	} else if orgID != nil {
		// Organization scope - query this org, all its projects, and all sites
		if err := s.addOrganizationToTfvars(ctx, *orgID, tfvars); err != nil {
			return nil, err
		}
		if err := s.addOrganizationProjectsToTfvars(ctx, *orgID, tfvars); err != nil {
			return nil, err
		}
		if err := s.addOrganizationSitesToTfvars(ctx, *orgID, tfvars); err != nil {
			return nil, err
		}
	}

	// Convert to JSON
	tfvarsJSON, err := json.Marshal(tfvars)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to marshal tfvars: %w", err))
	}

	slog.Info("generated terraform vars",
		"org_id", orgID,
		"project_id", projectID,
		"site_id", siteID,
		"org_count", len(tfvars["organizations"].(map[string]interface{})),
		"project_count", len(tfvars["projects"].(map[string]interface{})),
		"site_count", len(tfvars["sites"].(map[string]interface{})))

	return connect.NewResponse(&libopsv1.GenerateTerraformVarsResponse{
		TfvarsJson: string(tfvarsJSON),
	}), nil
}

// addOrganizationToTfvars adds a single organization to the tfvars structure
func (s *AdminReconciliationService) addOrganizationToTfvars(ctx context.Context, orgID int64, tfvars map[string]interface{}) error {
	query := `SELECT BIN_TO_UUID(public_id) AS public_id, name, gcp_org_id, gcp_billing_account, gcp_parent, location
	          FROM organizations WHERE id = ?`

	var publicID, name, gcpOrgID, gcpBillingAccount, gcpParent, location string
	err := s.mainQuerier.(*db.Queries).GetDB().QueryRowContext(ctx, query, orgID).Scan(
		&publicID, &name, &gcpOrgID, &gcpBillingAccount, &gcpParent, &location)
	if err != nil {
		slog.Error("failed to query organization", "org_id", orgID, "error", err)
		return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to query organization: %w", err))
	}

	orgs := tfvars["organizations"].(map[string]interface{})
	orgs[publicID] = map[string]interface{}{
		"name":                name,
		"gcp_org_id":          gcpOrgID,
		"gcp_billing_account": gcpBillingAccount,
		"gcp_parent":          gcpParent,
		"location":            location,
	}

	return nil
}

// addProjectToTfvars adds a single project to the tfvars structure
func (s *AdminReconciliationService) addProjectToTfvars(ctx context.Context, projectID int64, tfvars map[string]interface{}) error {
	query := `SELECT BIN_TO_UUID(p.public_id) AS public_id, p.name, BIN_TO_UUID(o.public_id) AS organization_id,
	                 o.gcp_folder_id, p.github_repository, o.gcp_billing_account, p.machine_type, p.disk_size_gb
	          FROM projects p
	          JOIN organizations o ON p.organization_id = o.id
	          WHERE p.id = ?`

	var publicID, name, orgPublicID, machineType string
	var gcpFolderID, githubRepo sql.NullString
	var gcpBillingAccount string
	var diskSize int32

	err := s.mainQuerier.(*db.Queries).GetDB().QueryRowContext(ctx, query, projectID).Scan(
		&publicID, &name, &orgPublicID, &gcpFolderID, &githubRepo, &gcpBillingAccount, &machineType, &diskSize)
	if err != nil {
		slog.Error("failed to query project", "project_id", projectID, "error", err)
		return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to query project: %w", err))
	}

	projects := tfvars["projects"].(map[string]interface{})
	projectData := map[string]interface{}{
		"name":                name,
		"organization_id":     orgPublicID,
		"gcp_billing_account": gcpBillingAccount,
		"machine_type":        machineType,
		"disk_size":           diskSize,
	}
	if gcpFolderID.Valid {
		projectData["organization_folder_id"] = gcpFolderID.String
	}
	if githubRepo.Valid {
		projectData["github_repository"] = githubRepo.String
	}
	projects[publicID] = projectData

	return nil
}

// addSiteToTfvars adds a single site to the tfvars structure
func (s *AdminReconciliationService) addSiteToTfvars(ctx context.Context, siteID int64, tfvars map[string]interface{}) error {
	query := `SELECT BIN_TO_UUID(s.public_id) AS public_id, s.name, BIN_TO_UUID(p.public_id) AS project_id,
	                 p.gcp_project_id, p.gcp_project_number, s.github_ref, s.github_repository,
	                 p.machine_type, p.disk_size_gb, p.gcp_zone
	          FROM sites s
	          JOIN projects p ON s.project_id = p.id
	          WHERE s.id = ?`

	var publicID, name, projectPublicID, gcpProjectID, gcpProjectNumber, githubRef, githubRepo, machineType, zone string
	var diskSize int32

	err := s.mainQuerier.(*db.Queries).GetDB().QueryRowContext(ctx, query, siteID).Scan(
		&publicID, &name, &projectPublicID, &gcpProjectID, &gcpProjectNumber, &githubRef, &githubRepo, &machineType, &diskSize, &zone)
	if err != nil {
		slog.Error("failed to query site", "site_id", siteID, "error", err)
		return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to query site: %w", err))
	}

	// Query firewall rules
	firewallRules, err := s.getSiteFirewallRules(ctx, siteID)
	if err != nil {
		return err
	}

	// Query members
	members, err := s.getSiteMembers(ctx, siteID)
	if err != nil {
		return err
	}

	// Query secrets
	secrets, err := s.getSiteSecrets(ctx, siteID)
	if err != nil {
		return err
	}

	sites := tfvars["sites"].(map[string]interface{})
	sites[publicID] = map[string]interface{}{
		"name":               name,
		"project_id":         projectPublicID,
		"gcp_project_id":     gcpProjectID,
		"gcp_project_number": gcpProjectNumber,
		"github_ref":         githubRef,
		"github_repo":        githubRepo,
		"machine_type":       machineType,
		"disk_size":          diskSize,
		"zone":               zone,
		"firewall_rules":     firewallRules,
		"members":            members,
		"secrets":            secrets,
	}

	return nil
}

// addOrganizationProjectsToTfvars adds all projects in an organization
func (s *AdminReconciliationService) addOrganizationProjectsToTfvars(ctx context.Context, orgID int64, tfvars map[string]interface{}) error {
	query := `SELECT id FROM projects WHERE organization_id = ? AND status != 'deleted'`

	rows, err := s.mainQuerier.(*db.Queries).GetDB().QueryContext(ctx, query, orgID)
	if err != nil {
		slog.Error("failed to query organization projects", "org_id", orgID, "error", err)
		return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to query projects: %w", err))
	}
	defer rows.Close()

	for rows.Next() {
		var projectID int64
		if err := rows.Scan(&projectID); err != nil {
			return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to scan project: %w", err))
		}
		if err := s.addProjectToTfvars(ctx, projectID, tfvars); err != nil {
			return err
		}
	}

	return nil
}

// addProjectSitesToTfvars adds all sites in a project
func (s *AdminReconciliationService) addProjectSitesToTfvars(ctx context.Context, projectID int64, tfvars map[string]interface{}) error {
	query := `SELECT id FROM sites WHERE project_id = ? AND status != 'deleted'`

	rows, err := s.mainQuerier.(*db.Queries).GetDB().QueryContext(ctx, query, projectID)
	if err != nil {
		slog.Error("failed to query project sites", "project_id", projectID, "error", err)
		return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to query sites: %w", err))
	}
	defer rows.Close()

	for rows.Next() {
		var siteID int64
		if err := rows.Scan(&siteID); err != nil {
			return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to scan site: %w", err))
		}
		if err := s.addSiteToTfvars(ctx, siteID, tfvars); err != nil {
			return err
		}
	}

	return nil
}

// addOrganizationSitesToTfvars adds all sites in an organization
func (s *AdminReconciliationService) addOrganizationSitesToTfvars(ctx context.Context, orgID int64, tfvars map[string]interface{}) error {
	query := `SELECT s.id FROM sites s
	          JOIN projects p ON s.project_id = p.id
	          WHERE p.organization_id = ? AND s.status != 'deleted'`

	rows, err := s.mainQuerier.(*db.Queries).GetDB().QueryContext(ctx, query, orgID)
	if err != nil {
		slog.Error("failed to query organization sites", "org_id", orgID, "error", err)
		return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to query sites: %w", err))
	}
	defer rows.Close()

	for rows.Next() {
		var siteID int64
		if err := rows.Scan(&siteID); err != nil {
			return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to scan site: %w", err))
		}
		if err := s.addSiteToTfvars(ctx, siteID, tfvars); err != nil {
			return err
		}
	}

	return nil
}

// getSiteFirewallRules returns firewall rules for a site (org + project + site rules)
func (s *AdminReconciliationService) getSiteFirewallRules(ctx context.Context, siteID int64) ([]map[string]interface{}, error) {
	query := `
		SELECT name, rule_type, cidr FROM (
			SELECT ofr.name, ofr.rule_type, ofr.cidr, 1 as priority
			FROM organization_firewall_rules ofr
			JOIN projects p ON ofr.organization_id = p.organization_id
			JOIN sites s ON s.project_id = p.id
			WHERE s.id = ? AND ofr.status != 'deleted'

			UNION ALL

			SELECT pfr.name, pfr.rule_type, pfr.cidr, 2 as priority
			FROM project_firewall_rules pfr
			JOIN sites s ON pfr.project_id = s.project_id
			WHERE s.id = ? AND pfr.status != 'deleted'

			UNION ALL

			SELECT sfr.name, sfr.rule_type, sfr.cidr, 3 as priority
			FROM site_firewall_rules sfr
			WHERE sfr.site_id = ? AND sfr.status != 'deleted'
		) AS all_rules
		ORDER BY priority, name`

	rows, err := s.mainQuerier.(*db.Queries).GetDB().QueryContext(ctx, query, siteID, siteID, siteID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to query firewall rules: %w", err))
	}
	defer rows.Close()

	var rules []map[string]interface{}
	for rows.Next() {
		var name, ruleType, cidr string
		if err := rows.Scan(&name, &ruleType, &cidr); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to scan firewall rule: %w", err))
		}
		rules = append(rules, map[string]interface{}{
			"name":      name,
			"rule_type": ruleType,
			"cidr":      cidr,
		})
	}

	if rules == nil {
		rules = []map[string]interface{}{}
	}

	return rules, nil
}

// getSiteMembers returns members for a site (org + project + site members)
func (s *AdminReconciliationService) getSiteMembers(ctx context.Context, siteID int64) ([]map[string]interface{}, error) {
	query := `
		SELECT DISTINCT a.email,
		       CASE
		           WHEN sm.role IS NOT NULL THEN sm.role
		           WHEN pm.role IS NOT NULL THEN pm.role
		           ELSE om.role
		       END as role
		FROM (
			SELECT account_id, role FROM site_members WHERE site_id = ? AND status = 'active'
			UNION
			SELECT pm.account_id, pm.role FROM project_members pm
			JOIN sites s ON pm.project_id = s.project_id
			WHERE s.id = ? AND pm.status = 'active'
			UNION
			SELECT om.account_id, om.role FROM organization_members om
			JOIN projects p ON om.organization_id = p.organization_id
			JOIN sites s ON s.project_id = p.id
			WHERE s.id = ? AND om.status = 'active'
		) AS all_members
		LEFT JOIN site_members sm ON all_members.account_id = sm.account_id AND sm.site_id = ?
		LEFT JOIN project_members pm ON all_members.account_id = pm.account_id AND pm.project_id = (SELECT project_id FROM sites WHERE id = ?)
		LEFT JOIN organization_members om ON all_members.account_id = om.account_id AND om.organization_id = (SELECT p.organization_id FROM sites s JOIN projects p ON s.project_id = p.id WHERE s.id = ?)
		JOIN accounts a ON all_members.account_id = a.id`

	rows, err := s.mainQuerier.(*db.Queries).GetDB().QueryContext(ctx, query, siteID, siteID, siteID, siteID, siteID, siteID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to query members: %w", err))
	}
	defer rows.Close()

	var members []map[string]interface{}
	for rows.Next() {
		var email, role string
		if err := rows.Scan(&email, &role); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to scan member: %w", err))
		}
		members = append(members, map[string]interface{}{
			"email": email,
			"role":  role,
		})
	}

	if members == nil {
		members = []map[string]interface{}{}
	}

	return members, nil
}

// getSiteSecrets returns secrets for a site (org + project + site secrets)
func (s *AdminReconciliationService) getSiteSecrets(ctx context.Context, siteID int64) ([]map[string]interface{}, error) {
	query := `
		SELECT name, vault_path FROM (
			SELECT os.name, os.vault_path, 1 as priority
			FROM organization_secrets os
			JOIN projects p ON os.organization_id = p.organization_id
			JOIN sites s ON s.project_id = p.id
			WHERE s.id = ? AND os.status != 'deleted'

			UNION ALL

			SELECT ps.name, ps.vault_path, 2 as priority
			FROM project_secrets ps
			JOIN sites s ON ps.project_id = s.project_id
			WHERE s.id = ? AND ps.status != 'deleted'

			UNION ALL

			SELECT ss.name, ss.vault_path, 3 as priority
			FROM site_secrets ss
			WHERE ss.site_id = ? AND ss.status != 'deleted'
		) AS all_secrets
		ORDER BY priority, name`

	rows, err := s.mainQuerier.(*db.Queries).GetDB().QueryContext(ctx, query, siteID, siteID, siteID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to query secrets: %w", err))
	}
	defer rows.Close()

	var secrets []map[string]interface{}
	for rows.Next() {
		var name, vaultPath string
		if err := rows.Scan(&name, &vaultPath); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to scan secret: %w", err))
		}
		secrets = append(secrets, map[string]interface{}{
			"name":       name,
			"vault_path": vaultPath,
		})
	}

	if secrets == nil {
		secrets = []map[string]interface{}{}
	}

	return secrets, nil
}
