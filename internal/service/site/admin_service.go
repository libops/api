package site

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/libops/api/db"
	"github.com/libops/api/internal/auth"
	"github.com/libops/api/internal/service"
	libopsv1 "github.com/libops/api/proto/libops/v1"
	adminv1 "github.com/libops/api/proto/libops/v1/admin"
	commonv1 "github.com/libops/api/proto/libops/v1/common"
	"github.com/libops/api/proto/libops/v1/libopsv1connect"
)

// AdminSiteService implements the admin-level site API.
type AdminSiteService struct {
	repo *Repository
}

// Compile-time check.
var _ libopsv1connect.AdminSiteServiceHandler = (*AdminSiteService)(nil)

// NewAdminSiteService creates a new admin site service.
func NewAdminSiteService(querier db.Querier) *AdminSiteService {
	return &AdminSiteService{
		repo: NewRepository(querier),
	}
}

// ListSites lists sites for a project (admin view).
func (s *AdminSiteService) ListSites(
	ctx context.Context,
	req *connect.Request[libopsv1.AdminListSitesRequest],
) (*connect.Response[libopsv1.AdminListSitesResponse], error) {
	var projectID string
	if req.Msg.ProjectId != nil {
		projectID = *req.Msg.ProjectId
	}

	if projectID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("project_id is required"))
	}

	projectPublicID, err := uuid.Parse(projectID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid project_id format: %w", err))
	}

	project, err := s.repo.GetProjectByPublicID(ctx, projectPublicID)
	if err != nil {
		return nil, err
	}

	pageSize := req.Msg.PageSize
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 100 {
		pageSize = 100
	}

	offset, err := service.ParsePageToken(req.Msg.PageToken)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid page_token: %w", err))
	}

	params := db.ListProjectSitesParams{
		ProjectID: project.ID,
		Limit:     pageSize,
		Offset:    int32(offset),
	}

	sites, err := s.repo.ListProjectSites(ctx, params)
	if err != nil {
		return nil, err
	}

	var organizationID string
	if req.Msg.OrganizationId != nil {
		organizationID = *req.Msg.OrganizationId
	}

	protoSites := make([]*adminv1.AdminSiteConfig, 0, len(sites))
	for _, site := range sites {
		protoSites = append(protoSites, &adminv1.AdminSiteConfig{
			Config: &commonv1.SiteConfig{
				SiteId:           site.PublicID,
				OrganizationId:   organizationID,
				ProjectId:        projectID,
				SiteName:         site.Name,
				GithubRepository: site.GithubRepository,
				GithubRef:        site.GithubRef,
				ComposePath:      site.ComposePath.String,
				ComposeFile:      site.ComposeFile.String,
				Port:             site.Port.Int32,
				ApplicationType:  site.ApplicationType.String,
				UpCmd:            service.FromJSONStringArray(site.UpCmd),
				InitCmd:          service.FromJSONStringArray(site.InitCmd),
				RolloutCmd:       service.FromJSONStringArray(site.RolloutCmd),
				OverlayVolumes:   service.FromJSONStringArray(site.OverlayVolumes),
				Os:               service.FromNullString(site.Os),
				IsProduction:     site.IsProduction.Bool,
				Status:           service.DbSiteStatusToProto(site.Status),
			},
			GcpInstanceName: nil,
			GcpExternalIp:   service.FromNullStringPtr(site.GcpExternalIp),
			GcpInternalIp:   nil,
			GithubTeamId:    service.FromNullStringPtr(site.GithubTeamID),
		})
	}

	nextPageToken := ""
	if len(sites) == int(pageSize) {
		nextPageToken = service.GeneratePageToken(offset + int(pageSize))
	}

	return connect.NewResponse(&libopsv1.AdminListSitesResponse{
		Sites:         protoSites,
		NextPageToken: nextPageToken,
	}), nil
}

// GetSite retrieves site configuration (admin view - includes internal GCP details).
func (s *AdminSiteService) GetSite(
	ctx context.Context,
	req *connect.Request[libopsv1.AdminGetSiteRequest],
) (*connect.Response[libopsv1.AdminGetSiteResponse], error) {
	projectID := req.Msg.ProjectId
	siteName := req.Msg.SiteName

	if projectID == "" || siteName == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("project_id and site_name are required"))
	}

	projectPublicID, err := uuid.Parse(projectID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid project_id format: %w", err))
	}

	project, err := s.repo.GetProjectByPublicID(ctx, projectPublicID)
	if err != nil {
		return nil, err
	}

	site, err := s.repo.GetSiteByProjectAndName(ctx, project.ID, siteName)
	if err != nil {
		return nil, err
	}

	protoSite := &adminv1.AdminSiteConfig{
		Config: &commonv1.SiteConfig{
			SiteId:           site.PublicID,
			OrganizationId:   req.Msg.OrganizationId,
			ProjectId:        projectID,
			SiteName:         site.Name,
			GithubRepository: site.GithubRepository,
			GithubRef:        site.GithubRef,
			ComposePath:      site.ComposePath.String,
			ComposeFile:      site.ComposeFile.String,
			Port:             site.Port.Int32,
			ApplicationType:  site.ApplicationType.String,
			UpCmd:            service.FromJSONStringArray(site.UpCmd),
			InitCmd:          service.FromJSONStringArray(site.InitCmd),
			RolloutCmd:       service.FromJSONStringArray(site.RolloutCmd),
			OverlayVolumes:   service.FromJSONStringArray(site.OverlayVolumes),
			Os:               service.FromNullString(site.Os),
			IsProduction:     site.IsProduction.Bool,
			Status:           service.DbSiteStatusToProto(site.Status),
		},
		GcpInstanceName: nil,
		GcpExternalIp:   service.FromNullStringPtr(site.GcpExternalIp),
		GcpInternalIp:   nil,
		GithubTeamId:    service.FromNullStringPtr(site.GithubTeamID),
	}

	return connect.NewResponse(&libopsv1.AdminGetSiteResponse{
		Site: protoSite,
	}), nil
}

// CreateSite creates a new site (admin - can set all fields).
func (s *AdminSiteService) CreateSite(
	ctx context.Context,
	req *connect.Request[libopsv1.AdminCreateSiteRequest],
) (*connect.Response[libopsv1.AdminCreateSiteResponse], error) {
	projectID := req.Msg.ProjectId
	site := req.Msg.Site

	if projectID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("project_id is required"))
	}

	if site == nil || site.Config == nil || site.Config.SiteName == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("site with site_name is required"))
	}

	projectPublicID, err := uuid.Parse(projectID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid project_id format: %w", err))
	}

	userInfo, ok := auth.GetUserFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("authentication required"))
	}

	accountID := userInfo.AccountID

	project, err := s.repo.GetProjectByPublicID(ctx, projectPublicID)
	if err != nil {
		return nil, err
	}

	params := db.CreateSiteParams{
		ProjectID:        project.ID,
		Name:             site.Config.SiteName,
		GithubRepository: site.Config.GithubRepository,
		GithubRef:        site.Config.GithubRef,
		ComposePath:      service.ToNullString(site.Config.ComposePath),
		ComposeFile:      service.ToNullString(site.Config.ComposeFile),
		Port:             service.ToNullInt32(site.Config.Port),
		ApplicationType:  service.ToNullString(site.Config.ApplicationType),
		UpCmd:            service.ToJSON(site.Config.UpCmd),
		InitCmd:          service.ToJSON(site.Config.InitCmd),
		RolloutCmd:       service.ToJSON(site.Config.RolloutCmd),
		GcpExternalIp:    service.ToNullString(service.PtrToString(site.GcpExternalIp)),
		GithubTeamID:     service.ToNullString(service.PtrToString(site.GithubTeamId)),
		Status:           db.NullSitesStatus{SitesStatus: db.SitesStatusProvisioning, Valid: true},
		CreatedBy:        sql.NullInt64{Int64: accountID, Valid: true},
		UpdatedBy:        sql.NullInt64{Int64: accountID, Valid: true},
	}

	err = s.repo.CreateSite(ctx, params)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&libopsv1.AdminCreateSiteResponse{
		Site: site,
	}), nil
}

// UpdateSite updates site configuration (admin - can update all fields).
func (s *AdminSiteService) UpdateSite(
	ctx context.Context,
	req *connect.Request[libopsv1.AdminUpdateSiteRequest],
) (*connect.Response[libopsv1.AdminUpdateSiteResponse], error) {
	projectID := req.Msg.ProjectId
	siteName := req.Msg.SiteName
	site := req.Msg.Site

	if projectID == "" || siteName == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("project_id and site_name are required"))
	}

	if site == nil || site.Config == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("site is required"))
	}

	projectPublicID, err := uuid.Parse(projectID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid project_id format: %w", err))
	}

	userInfo, ok := auth.GetUserFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("authentication required"))
	}

	accountID := userInfo.AccountID

	project, err := s.repo.GetProjectByPublicID(ctx, projectPublicID)
	if err != nil {
		return nil, err
	}

	existing, err := s.repo.GetSiteByProjectAndName(ctx, project.ID, siteName)
	if err != nil {
		return nil, err
	}

	name := existing.Name
	githubRepository := existing.GithubRepository
	githubRef := existing.GithubRef
	composePath := existing.ComposePath
	composeFile := existing.ComposeFile
	port := existing.Port
	applicationType := existing.ApplicationType
	upCmd := existing.UpCmd
	initCmd := existing.InitCmd
	rolloutCmd := existing.RolloutCmd
	gcpExternalIp := existing.GcpExternalIp
	githubTeamID := existing.GithubTeamID

	if service.ShouldUpdateField(req.Msg.UpdateMask, "site.config.site_name") {
		name = site.Config.SiteName
	}
	if service.ShouldUpdateField(req.Msg.UpdateMask, "site.config.github_repository") {
		githubRepository = site.Config.GithubRepository
	}
	if service.ShouldUpdateField(req.Msg.UpdateMask, "site.config.github_ref") {
		githubRef = site.Config.GithubRef
	}
	if service.ShouldUpdateField(req.Msg.UpdateMask, "site.config.compose_path") {
		composePath = service.ToNullString(site.Config.ComposePath)
	}
	if service.ShouldUpdateField(req.Msg.UpdateMask, "site.config.compose_file") {
		composeFile = service.ToNullString(site.Config.ComposeFile)
	}
	if service.ShouldUpdateField(req.Msg.UpdateMask, "site.config.port") {
		port = service.ToNullInt32(site.Config.Port)
	}
	if service.ShouldUpdateField(req.Msg.UpdateMask, "site.config.application_type") {
		applicationType = service.ToNullString(site.Config.ApplicationType)
	}
	if service.ShouldUpdateField(req.Msg.UpdateMask, "site.config.up_cmd") {
		upCmd = service.ToJSON(site.Config.UpCmd)
	}
	if service.ShouldUpdateField(req.Msg.UpdateMask, "site.config.init_cmd") {
		initCmd = service.ToJSON(site.Config.InitCmd)
	}
	if service.ShouldUpdateField(req.Msg.UpdateMask, "site.config.rollout_cmd") {
		rolloutCmd = service.ToJSON(site.Config.RolloutCmd)
	}
	if service.ShouldUpdateField(req.Msg.UpdateMask, "site.gcp_external_ip") {
		gcpExternalIp = service.ToNullString(service.PtrToString(site.GcpExternalIp))
	}
	if service.ShouldUpdateField(req.Msg.UpdateMask, "site.github_team_id") {
		githubTeamID = service.ToNullString(service.PtrToString(site.GithubTeamId))
	}

	siteUUID, err := uuid.Parse(existing.PublicID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("invalid site public_id: %w", err))
	}

	params := db.UpdateSiteParams{
		Name:             name,
		GithubRepository: githubRepository,
		GithubRef:        githubRef,
		GithubTeamID:     githubTeamID,
		ComposePath:      composePath,
		ComposeFile:      composeFile,
		Port:             port,
		ApplicationType:  applicationType,
		UpCmd:            upCmd,
		InitCmd:          initCmd,
		RolloutCmd:       rolloutCmd,
		GcpExternalIp:    gcpExternalIp,
		Status:           db.NullSitesStatus{SitesStatus: db.SitesStatusActive, Valid: true},
		UpdatedBy:        sql.NullInt64{Int64: accountID, Valid: true},
		PublicID:         siteUUID.String(),
	}

	err = s.repo.UpdateSite(ctx, params)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&libopsv1.AdminUpdateSiteResponse{
		Site: site,
	}), nil
}

// DeleteSite deletes a site.
func (s *AdminSiteService) DeleteSite(
	ctx context.Context,
	req *connect.Request[libopsv1.AdminDeleteSiteRequest],
) (*connect.Response[emptypb.Empty], error) {
	projectID := req.Msg.ProjectId
	siteName := req.Msg.SiteName

	if projectID == "" || siteName == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("project_id and site_name are required"))
	}

	projectPublicID, err := uuid.Parse(projectID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid project_id format: %w", err))
	}

	project, err := s.repo.GetProjectByPublicID(ctx, projectPublicID)
	if err != nil {
		return nil, err
	}

	site, err := s.repo.GetSiteByProjectAndName(ctx, project.ID, siteName)
	if err != nil {
		return nil, err
	}

	err = s.repo.DeleteSite(ctx, site.PublicID)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&emptypb.Empty{}), nil
}

// ListAllSites lists all sites across all organizations/projects (admin only).
func (s *AdminSiteService) ListAllSites(
	ctx context.Context,
	req *connect.Request[libopsv1.AdminListAllSitesRequest],
) (*connect.Response[libopsv1.AdminListAllSitesResponse], error) {
	// TODO: Implement cross-project site listing
	return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("list all sites not yet implemented"))
}

// GetSiteSSHKeys returns SSH keys for a site VM (called by VM controller with GSA auth).
func (s *AdminSiteService) GetSiteSSHKeys(
	ctx context.Context,
	req *connect.Request[libopsv1.GetSiteSSHKeysRequest],
) (*connect.Response[libopsv1.GetSiteSSHKeysResponse], error) {
	siteID := req.Msg.SiteId
	if siteID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("site_id is required"))
	}

	sitePublicID, err := uuid.Parse(siteID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid site_id format: %w", err))
	}

	// Get site to verify it exists
	site, err := s.repo.GetSiteByPublicID(ctx, sitePublicID)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("site not found: %w", err))
	}

	// Query SSH keys with inheritance (site → project → org → relationships)
	keys, err := s.repo.db.GetSiteSSHKeysForVM(ctx, db.GetSiteSSHKeysForVMParams{
		SiteID: site.ID,
		ID:     site.ID,
		ID_2:   site.ID,
		ID_3:   site.ID,
	})
	if err != nil {
		slog.Error("failed to fetch site SSH keys", "site_id", siteID, "error", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch SSH keys: %w", err))
	}

	// Convert to proto format
	protoKeys := make([]*libopsv1.SSHKey, 0, len(keys))
	for _, key := range keys {
		protoKeys = append(protoKeys, &libopsv1.SSHKey{
			PublicKey:      key.PublicKey,
			Name:           service.FromNullString(key.Name),
			Fingerprint:    service.FromNullString(key.Fingerprint),
			GithubUsername: service.FromNullString(key.GithubUsername),
		})
	}

	return connect.NewResponse(&libopsv1.GetSiteSSHKeysResponse{
		Keys: protoKeys,
	}), nil
}

// GetSiteSecrets returns secrets for a site VM (called by VM controller with GSA auth).
func (s *AdminSiteService) GetSiteSecrets(
	ctx context.Context,
	req *connect.Request[libopsv1.GetSiteSecretsRequest],
) (*connect.Response[libopsv1.GetSiteSecretsResponse], error) {
	siteID := req.Msg.SiteId
	if siteID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("site_id is required"))
	}

	sitePublicID, err := uuid.Parse(siteID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid site_id format: %w", err))
	}

	// Get site to verify it exists
	site, err := s.repo.GetSiteByPublicID(ctx, sitePublicID)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("site not found: %w", err))
	}

	// Query secrets with inheritance (site → project → org)
	secrets, err := s.repo.db.GetSiteSecretsForVM(ctx, db.GetSiteSecretsForVMParams{
		SiteID: site.ID,
		ID:     site.ID,
		ID_2:   site.ID,
	})
	if err != nil {
		slog.Error("failed to fetch site secrets", "site_id", siteID, "error", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch secrets: %w", err))
	}

	// Convert to proto format
	protoSecrets := make([]*libopsv1.Secret, 0, len(secrets))
	for _, secret := range secrets {
		protoSecrets = append(protoSecrets, &libopsv1.Secret{
			Key:   secret.Key,
			Value: secret.Value,
		})
	}

	return connect.NewResponse(&libopsv1.GetSiteSecretsResponse{
		Secrets: protoSecrets,
	}), nil
}

// GetSiteFirewall returns firewall rules for a site VM (called by VM controller with GSA auth).
func (s *AdminSiteService) GetSiteFirewall(
	ctx context.Context,
	req *connect.Request[libopsv1.GetSiteFirewallRequest],
) (*connect.Response[libopsv1.GetSiteFirewallResponse], error) {
	siteID := req.Msg.SiteId
	if siteID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("site_id is required"))
	}

	sitePublicID, err := uuid.Parse(siteID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid site_id format: %w", err))
	}

	// Get site to verify it exists
	site, err := s.repo.GetSiteByPublicID(ctx, sitePublicID)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("site not found: %w", err))
	}

	// Query firewall rules with inheritance (site → project → org)
	rules, err := s.repo.db.GetSiteFirewallForVM(ctx, db.GetSiteFirewallForVMParams{
		SiteID: sql.NullInt64{Int64: site.ID, Valid: true},
		ID:     site.ID,
		ID_2:   site.ID,
	})
	if err != nil {
		slog.Error("failed to fetch site firewall rules", "site_id", siteID, "error", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch firewall rules: %w", err))
	}

	// Convert to proto format
	protoRules := make([]*libopsv1.FirewallRule, 0, len(rules))
	for _, rule := range rules {
		// Map database rule_type to protocol/port/action
		var protocol string
		var port int32
		var action string

		switch rule.RuleType {
		case db.SiteFirewallRulesRuleTypeHttpsAllowed:
			protocol = "tcp"
			port = 443
			action = "accept"
		case db.SiteFirewallRulesRuleTypeSshAllowed:
			protocol = "tcp"
			port = 22
			action = "accept"
		case db.SiteFirewallRulesRuleTypeBlocked:
			protocol = "all"
			port = 0
			action = "deny"
		default:
			protocol = "all"
			port = 0
			action = "deny"
		}

		protoRules = append(protoRules, &libopsv1.FirewallRule{
			Protocol: protocol,
			Port:     port,
			Source:   rule.Cidr,
			Action:   action,
		})
	}

	return connect.NewResponse(&libopsv1.GetSiteFirewallResponse{
		Rules: protoRules,
	}), nil
}

// SiteCheckIn updates the site's check-in timestamp (called by VM controller).
func (s *AdminSiteService) SiteCheckIn(
	ctx context.Context,
	req *connect.Request[libopsv1.SiteCheckInRequest],
) (*connect.Response[libopsv1.SiteCheckInResponse], error) {
	siteID := req.Msg.SiteId
	if siteID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("site_id is required"))
	}

	sitePublicID, err := uuid.Parse(siteID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid site_id format: %w", err))
	}

	// Get site to verify it exists
	site, err := s.repo.GetSiteByPublicID(ctx, sitePublicID)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("site not found: %w", err))
	}

	// Update check-in timestamp
	err = s.repo.db.UpdateSiteCheckIn(ctx, site.ID)
	if err != nil {
		slog.Error("failed to update site check-in", "site_id", siteID, "error", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update check-in: %w", err))
	}

	slog.Info("site checked in successfully", "site_id", siteID)

	return connect.NewResponse(&libopsv1.SiteCheckInResponse{
		Success: true,
		Message: "Check-in successful",
	}), nil
}

// SshKeysResponse is the JSON response format for SSH keys.
type SshKeysResponse struct {
	SshKeys []string `json:"ssh_keys"`
}

// SyncManifest returns the manifest for a site with ETag support (eventual consistency).
// Called by site VMs every ~24h to ensure eventual consistency with control plane state.
func (s *AdminSiteService) SyncManifest(
	ctx context.Context,
	req *connect.Request[libopsv1.SyncManifestRequest],
) (*connect.Response[libopsv1.SyncManifestResponse], error) {
	// TODO: Implement manifest sync with GCS-backed state blobs
	// This will:
	// 1. Get site by site_id
	// 2. Check If-None-Match header against target_state_hash (ETag optimization)
	// 3. Materialize current state (SSH keys, secrets, firewall) if needed
	// 4. Store blobs in GCS under blobs/{hash}/{filename}
	// 5. Generate signed URLs for blobs (15 min expiry)
	// 6. Return state hash and signed URLs
	return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("manifest sync not yet implemented"))
}

// GetBlob returns a blob directly (fallback for expired signed URLs).
// Optional - VMs can fetch blobs directly from GCS using signed URLs from SyncManifest.
func (s *AdminSiteService) GetBlob(
	ctx context.Context,
	req *connect.Request[libopsv1.GetBlobRequest],
) (*connect.Response[libopsv1.GetBlobResponse], error) {
	// TODO: Implement direct blob fetch
	// This will:
	// 1. Validate site_id and blob_type
	// 2. Fetch blob from GCS
	// 3. If not found, regenerate state and retry
	// 4. Return blob data
	return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("get blob not yet implemented"))
}

// HandleSiteSshKeys returns SSH keys for all owners and developers of a site.
// This is a plain HTTP handler for VM reconciliation services.
func (s *AdminSiteService) HandleSiteSshKeys(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("siteId")
	if siteID == "" {
		http.Error(w, "site ID is required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	keys, err := s.repo.db.ListSshKeysBySite(ctx, db.ListSshKeysBySiteParams{
		SitePublicID: siteID,
	})
	if err != nil {
		slog.Error("failed to fetch site SSH keys", "site_id", siteID, "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	response := SshKeysResponse{
		SshKeys: keys,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		slog.Error("failed to encode response", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
}
