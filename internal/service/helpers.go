// Package service provides common helper functions and utilities used across various services.
package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/libops/api/db"
	"github.com/libops/api/db/types"
	commonv1 "github.com/libops/api/proto/libops/v1/common"
)

// Helper functions shared across all services

// ==============================================================================
// Access Control Helpers
// ==============================================================================

// CheckRelationshipAccess checks if a user has access to an organization via a relationship.
// It returns nil if access is granted, or an error if not.
// If checkWrite is true, requires 'owner' or 'developer' role in the source organization.
func CheckRelationshipAccess(ctx context.Context, querier db.Querier, accountID int64, targetOrgID int64, checkWrite bool) error {
	relationships, err := querier.ListOrganizationRelationships(ctx, db.ListOrganizationRelationshipsParams{
		SourceOrganizationID: targetOrgID,
		TargetOrganizationID: targetOrgID,
	})
	if err != nil {
		return err
	}

	for _, rel := range relationships {
		if rel.Status == db.RelationshipsStatusApproved && rel.TargetOrganizationID == targetOrgID {
			sourceMember, err := querier.GetOrganizationMemberByAccountAndOrganization(ctx, db.GetOrganizationMemberByAccountAndOrganizationParams{
				AccountID:      accountID,
				OrganizationID: rel.SourceOrganizationID,
			})
			if err == nil {
				if !checkWrite {
					return nil
				}
				if sourceMember.Role == db.OrganizationMembersRoleOwner || sourceMember.Role == db.OrganizationMembersRoleDeveloper {
					return nil
				}
			}
		}
	}

	return connect.NewError(connect.CodePermissionDenied, fmt.Errorf("not a member of this site, project, or organization"))
}

// ==============================================================================
// Pointer Helpers
// ==============================================================================
// These functions convert Go values to pointers, which is useful for working
// with proto3 optional fields that use pointers to distinguish between
// "not set" (nil) and "set to zero value" (pointer to zero value).

// stringPtr returns a pointer to the given string.
func stringPtr(s string) *string {
	return &s
}

// int32Ptr returns a pointer to the given int32.
func int32Ptr(i int32) *int32 {
	return &i
}

// int64Ptr returns a pointer to the given int64.
func int64Ptr(i int64) *int64 {
	return &i
}

// float64Ptr returns a pointer to the given float64.
func float64Ptr(f float64) *float64 {
	return &f
}

// ==============================================================================
// Value Extraction Helpers
// ==============================================================================
// These functions safely extract values from pointers, providing defaults
// when the pointer is nil. Used when converting from proto3 optional fields
// to Go values.

// stringValue extracts a string from a pointer, returning defaultValue if nil or empty.
func stringValue(ptr *string, defaultValue string) string {
	if ptr != nil && *ptr != "" {
		return *ptr
	}
	return defaultValue
}

// int32Value extracts an int32 from a pointer, returning defaultValue if nil.
func int32Value(ptr *int32, defaultValue int32) int32 {
	if ptr != nil {
		return *ptr
	}
	return defaultValue
}

// int64Value extracts an int64 from a pointer, returning defaultValue if nil.
func int64Value(ptr *int64, defaultValue int64) int64 {
	if ptr != nil {
		return *ptr
	}
	return defaultValue
}

// boolValue extracts a bool from a pointer, returning defaultValue if nil.
func boolValue(ptr *bool, defaultValue bool) bool {
	if ptr != nil {
		return *ptr
	}
	return defaultValue
}

// ==============================================================================
// Time Conversion Helpers
// ==============================================================================
// These functions convert between database nullable timestamps and proto3
// Timestamp messages.

// timeToProto converts a SQL nullable timestamp to a proto Timestamp.
// Returns nil for SQL NULL values to properly represent absence in the API.
func timeToProto(t sql.NullTime) *timestamppb.Timestamp {
	if t.Valid {
		return timestamppb.New(t.Time)
	}
	return nil
}

// protoToTime converts a proto Timestamp to a SQL nullable timestamp.
// Returns an invalid NullTime for nil proto timestamps.
func protoToTime(ts *timestamppb.Timestamp) sql.NullTime {
	if ts != nil {
		return sql.NullTime{
			Time:  ts.AsTime(),
			Valid: true,
		}
	}
	return sql.NullTime{Valid: false}
}

// ==============================================================================
// Page Token Encoding/Decoding (Internal)
// ==============================================================================

// ParsePageToken decodes a page token string to an offset integer.
// Returns 0 for empty tokens (first page).
func ParsePageToken(token string) (int, error) {
	if token == "" {
		return 0, nil
	}
	offset, err := strconv.Atoi(token)
	if err != nil {
		return 0, fmt.Errorf("invalid page token: %w", err)
	}
	return offset, nil
}

// GeneratePageToken encodes an offset integer to a page token string.
func GeneratePageToken(offset int) string {
	return strconv.Itoa(offset)
}

// ==============================================================================
// Validation Helpers
// ==============================================================================

// isValidMemberRole checks if the given role is one of the allowed member roles.
// Valid roles are: owner, developer, read.
func IsValidMemberRole(role string) bool {
	return role == "owner" || role == "developer" || role == "read"
}

// SQL helpers to convert between nullable types.
// ToNullString converts a string to a sql.NullString, setting Valid to false if the string is empty.
func ToNullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{String: s, Valid: true}
}

// FromNullString extracts the string value from a sql.NullString, returning an empty string if not valid.
func FromNullString(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}

// ToNullInt64 converts an int64 to a sql.NullInt64, setting Valid to false if the int64 is 0.
func ToNullInt64(i int64) sql.NullInt64 {
	if i == 0 {
		return sql.NullInt64{Valid: false}
	}
	return sql.NullInt64{Int64: i, Valid: true}
}

// ToNullInt32 converts an int32 to a sql.NullInt32, setting Valid to false if the int32 is 0.
func ToNullInt32(i int32) sql.NullInt32 {
	if i == 0 {
		return sql.NullInt32{Valid: false}
	}
	return sql.NullInt32{Int32: i, Valid: true}
}

// FromNullInt64 extracts the int64 value from a sql.NullInt64, returning 0 if not valid.
func FromNullInt64(ni sql.NullInt64) int64 {
	if ni.Valid {
		return ni.Int64
	}
	return 0
}

// FromNullInt32 extracts the int32 value from a sql.NullInt32, returning 0 if not valid.
func FromNullInt32(ni sql.NullInt32) int32 {
	if ni.Valid {
		return ni.Int32
	}
	return 0
}

// ToNullBool converts a bool to a sql.NullBool.
func ToNullBool(b bool) sql.NullBool {
	return sql.NullBool{Bool: b, Valid: true}
}

// Convert sql.NullString to optional proto field (*string).
func FromNullStringPtr(ns sql.NullString) *string {
	if ns.Valid && ns.String != "" {
		return &ns.String
	}
	return nil
}

// Convert proto optional field (*string) to string for ToNullString.
func PtrToString(ptr *string) string {
	if ptr != nil {
		return *ptr
	}
	return ""
}

// ToJSON converts any value to types.RawJSON.
// It returns nil if the input is nil or if marshalling fails.
func ToJSON(v any) types.RawJSON {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		slog.Error("failed to marshal to JSON", "error", err)
		return nil
	}
	return types.RawJSON(b)
}

// FromJSONStringArray converts types.RawJSON to []string.
// It returns an empty slice if input is nil or unmarshalling fails.
func FromJSONStringArray(raw types.RawJSON) []string {
	if raw == nil {
		return []string{}
	}
	var res []string
	if err := json.Unmarshal(raw, &res); err != nil {
		slog.Error("failed to unmarshal JSON to string array", "error", err)
		return []string{}
	}
	return res
}

// ==============================================================================
// UUID Parsing Helper
// ==============================================================================

// parseUUID parses and validates a UUID string, returning a connect error on failure.
func parseUUID(id string, fieldName string) (uuid.UUID, error) {
	if id == "" {
		return uuid.Nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("%s is required", fieldName))
	}
	parsed, err := uuid.Parse(id)
	if err != nil {
		return uuid.Nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid %s format: %w", fieldName, err))
	}
	return parsed, nil
}

// ==============================================================================
// Pagination Helpers
// ==============================================================================

// Pagination constants.
const (
	DefaultPageSize = 50
	MaxPageSize     = 100
)

// PaginationParams holds validated pagination parameters ready for database queries.
type PaginationParams struct {
	Limit  int32
	Offset int32
}

// PaginationResult contains the next page token for pagination responses.
type PaginationResult struct {
	NextPageToken string
}

// ParsePagination validates and normalizes pagination parameters from API requests.
// It handles default page sizes, max limits, and token parsing.
func ParsePagination(pageSize int32, pageToken string) (PaginationParams, error) {
	if pageSize <= 0 {
		pageSize = DefaultPageSize
	}
	if pageSize > MaxPageSize {
		pageSize = MaxPageSize
	}

	offset, err := ParsePageToken(pageToken)
	if err != nil {
		return PaginationParams{}, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid page_token: %w", err))
	}

	return PaginationParams{Limit: pageSize, Offset: int32(offset)}, nil
}

// MakePaginationResult generates a pagination result with next page token if needed.
// It determines if there are more results by checking if the result count equals the limit.
func MakePaginationResult(resultCount int, params PaginationParams) PaginationResult {
	if resultCount == int(params.Limit) {
		return PaginationResult{NextPageToken: GeneratePageToken(int(params.Offset) + resultCount)}
	}
	return PaginationResult{NextPageToken: ""}
}

// ==============================================================================
// Entity Lookup Helpers
// ==============================================================================

// getSiteByProjectAndName is a common pattern: lookup project, then lookup site within project.
func GetSiteByProjectAndName(
	ctx context.Context,
	querier db.Querier,
	projectID string,
	siteName string,
) (db.GetSiteByProjectAndNameRow, int64, error) {
	projectPublicID, err := parseUUID(projectID, "project_id")
	if err != nil {
		return db.GetSiteByProjectAndNameRow{}, 0, err
	}

	project, err := querier.GetProject(ctx, projectPublicID.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return db.GetSiteByProjectAndNameRow{}, 0, connect.NewError(connect.CodeNotFound, fmt.Errorf("resource not found"))
		}
		slog.Error("database error getting project", "err", err)
		return db.GetSiteByProjectAndNameRow{}, 0, connect.NewError(connect.CodeInternal, fmt.Errorf("internal server error"))
	}

	site, err := querier.GetSiteByProjectAndName(ctx, db.GetSiteByProjectAndNameParams{
		ProjectID: project.ID,
		Name:      siteName,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return db.GetSiteByProjectAndNameRow{}, 0, connect.NewError(connect.CodeNotFound, fmt.Errorf("resource not found"))
		}
		slog.Error("database error getting site", "err", err)
		return db.GetSiteByProjectAndNameRow{}, 0, connect.NewError(connect.CodeInternal, fmt.Errorf("internal server error"))
	}

	return site, project.ID, nil
}

// ==============================================================================
// Field Mask Helpers
// ==============================================================================

// shouldUpdateField checks if a field should be updated based on the field mask
// If the field mask is nil or empty, all fields should be updated
// Otherwise, only fields present in the mask should be updated.
func ShouldUpdateField(mask *fieldmaskpb.FieldMask, fieldPath string) bool {
	if mask == nil || len(mask.Paths) == 0 {
		return true
	}
	for _, path := range mask.Paths {
		if path == fieldPath {
			return true
		}
	}
	return false
}

// ==============================================================================
// Additional Entity Lookup Helpers
// ==============================================================================

// getOrganizationByPublicID looks up a organization by public ID and returns it with error handling.
func GetOrganizationByPublicID(ctx context.Context, querier db.Querier, publicID string) (db.GetOrganizationRow, error) {
	organizationUUID, err := parseUUID(publicID, "organization_id")
	if err != nil {
		return db.GetOrganizationRow{}, err
	}

	organization, err := querier.GetOrganization(ctx, organizationUUID.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return db.GetOrganizationRow{}, connect.NewError(connect.CodeNotFound, fmt.Errorf("resource not found"))
		}
		slog.Error("database error getting organization", "err", err)
		return db.GetOrganizationRow{}, connect.NewError(connect.CodeInternal, fmt.Errorf("internal server error"))
	}

	return organization, nil
}

// getProjectByPublicID looks up a project by public ID and returns it with error handling.
func GetProjectByPublicID(ctx context.Context, querier db.Querier, publicID string) (db.GetProjectRow, error) {
	projectUUID, err := parseUUID(publicID, "project_id")
	if err != nil {
		return db.GetProjectRow{}, err
	}

	project, err := querier.GetProject(ctx, projectUUID.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return db.GetProjectRow{}, connect.NewError(connect.CodeNotFound, fmt.Errorf("resource not found"))
		}
		slog.Error("database error getting project", "err", err)
		return db.GetProjectRow{}, connect.NewError(connect.CodeInternal, fmt.Errorf("internal server error"))
	}

	return project, nil
}

// getSiteByPublicID looks up a site by public ID and returns it with error handling.
func GetSiteByPublicID(ctx context.Context, querier db.Querier, publicID string) (db.GetSiteRow, error) {
	siteUUID, err := parseUUID(publicID, "site_id")
	if err != nil {
		return db.GetSiteRow{}, err
	}

	site, err := querier.GetSite(ctx, siteUUID.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return db.GetSiteRow{}, connect.NewError(connect.CodeNotFound, fmt.Errorf("resource not found"))
		}
		slog.Error("database error getting site", "err", err)
		return db.GetSiteRow{}, connect.NewError(connect.CodeInternal, fmt.Errorf("internal server error"))
	}

	return site, nil
}

// ==============================================================================
// Status Conversion Helpers
// ==============================================================================

// DbStatusToProto converts database status enums to proto Status.
func DbStatusToProto(status string) commonv1.Status {
	switch status {
	case "unspecified":
		return commonv1.Status_STATUS_UNSPECIFIED
	case "active":
		return commonv1.Status_STATUS_ACTIVE
	case "provisioning":
		return commonv1.Status_STATUS_PROVISIONING
	case "failed":
		return commonv1.Status_STATUS_FAILED
	case "suspended":
		return commonv1.Status_STATUS_SUSPENDED
	case "deleted":
		return commonv1.Status_STATUS_DELETED
	case "pending":
		return commonv1.Status_STATUS_PROVISIONING
	default:
		return commonv1.Status_STATUS_UNSPECIFIED
	}
}

// DbOrganizationStatusToProto converts NullOrganizationsStatus to proto Status.
func DbOrganizationStatusToProto(status db.NullOrganizationsStatus) commonv1.Status {
	if !status.Valid {
		return commonv1.Status_STATUS_UNSPECIFIED
	}
	return DbStatusToProto(string(status.OrganizationsStatus))
}

// DbProjectStatusToProto converts NullProjectsStatus to proto Status.
func DbProjectStatusToProto(status db.NullProjectsStatus) commonv1.Status {
	if !status.Valid {
		return commonv1.Status_STATUS_UNSPECIFIED
	}
	return DbStatusToProto(string(status.ProjectsStatus))
}

// DbSiteStatusToProto converts NullSitesStatus to proto Status.
func DbSiteStatusToProto(status db.NullSitesStatus) commonv1.Status {
	if !status.Valid {
		return commonv1.Status_STATUS_UNSPECIFIED
	}
	return DbStatusToProto(string(status.SitesStatus))
}

// DbPromoteStrategyToProto converts NullProjectsPromoteStrategy to proto PromoteStrategy.
func DbPromoteStrategyToProto(strategy db.NullProjectsPromoteStrategy) commonv1.PromoteStrategy {
	if !strategy.Valid {
		return commonv1.PromoteStrategy_PROMOTE_STRATEGY_UNSPECIFIED
	}
	switch strategy.ProjectsPromoteStrategy {
	case db.ProjectsPromoteStrategyGithubTag:
		return commonv1.PromoteStrategy_PROMOTE_STRATEGY_GITHUB_TAG
	case db.ProjectsPromoteStrategyGithubRelease:
		return commonv1.PromoteStrategy_PROMOTE_STRATEGY_GITHUB_RELEASE
	default:
		return commonv1.PromoteStrategy_PROMOTE_STRATEGY_UNSPECIFIED
	}
}

// ProtoStatusToOrganizationDB converts proto Status to OrganizationsStatus.
func ProtoStatusToOrganizationDB(status commonv1.Status) db.OrganizationsStatus {
	switch status {
	case commonv1.Status_STATUS_ACTIVE:
		return db.OrganizationsStatusActive
	case commonv1.Status_STATUS_PROVISIONING:
		return db.OrganizationsStatusProvisioning
	case commonv1.Status_STATUS_FAILED:
		return db.OrganizationsStatusFailed
	case commonv1.Status_STATUS_SUSPENDED:
		return db.OrganizationsStatusSuspended
	case commonv1.Status_STATUS_DELETED:
		return db.OrganizationsStatusDeleted
	default:
		return db.OrganizationsStatusUnspecified
	}
}

// ProtoStatusToProjectDB converts proto Status to ProjectsStatus.
func ProtoStatusToProjectDB(status commonv1.Status) db.ProjectsStatus {
	switch status {
	case commonv1.Status_STATUS_ACTIVE:
		return db.ProjectsStatusActive
	case commonv1.Status_STATUS_PROVISIONING:
		return db.ProjectsStatusProvisioning
	case commonv1.Status_STATUS_FAILED:
		return db.ProjectsStatusFailed
	case commonv1.Status_STATUS_SUSPENDED:
		return db.ProjectsStatusSuspended
	case commonv1.Status_STATUS_DELETED:
		return db.ProjectsStatusDeleted
	default:
		return db.ProjectsStatusUnspecified
	}
}

// ProtoStatusToSiteDB converts proto Status to SitesStatus.
func ProtoStatusToSiteDB(status commonv1.Status) db.SitesStatus {
	switch status {
	case commonv1.Status_STATUS_ACTIVE:
		return db.SitesStatusActive
	case commonv1.Status_STATUS_PROVISIONING:
		return db.SitesStatusProvisioning
	case commonv1.Status_STATUS_FAILED:
		return db.SitesStatusFailed
	case commonv1.Status_STATUS_SUSPENDED:
		return db.SitesStatusSuspended
	case commonv1.Status_STATUS_DELETED:
		return db.SitesStatusDeleted
	default:
		return db.SitesStatusUnspecified
	}
}

// DbOrganizationFirewallRuleStatusToProto converts NullOrganizationFirewallRulesStatus to proto Status.
func DbOrganizationFirewallRuleStatusToProto(status db.NullOrganizationFirewallRulesStatus) commonv1.Status {
	if !status.Valid {
		return commonv1.Status_STATUS_UNSPECIFIED
	}
	return DbStatusToProto(string(status.OrganizationFirewallRulesStatus))
}

// DbProjectFirewallRuleStatusToProto converts NullProjectFirewallRulesStatus to proto Status.
func DbProjectFirewallRuleStatusToProto(status db.NullProjectFirewallRulesStatus) commonv1.Status {
	if !status.Valid {
		return commonv1.Status_STATUS_UNSPECIFIED
	}
	return DbStatusToProto(string(status.ProjectFirewallRulesStatus))
}

// DbSiteFirewallRuleStatusToProto converts NullSiteFirewallRulesStatus to proto Status.
func DbSiteFirewallRuleStatusToProto(status db.NullSiteFirewallRulesStatus) commonv1.Status {
	if !status.Valid {
		return commonv1.Status_STATUS_UNSPECIFIED
	}
	return DbStatusToProto(string(status.SiteFirewallRulesStatus))
}

// DbOrganizationMemberStatusToProto converts NullOrganizationMembersStatus to proto Status.
func DbOrganizationMemberStatusToProto(status db.NullOrganizationMembersStatus) commonv1.Status {
	if !status.Valid {
		return commonv1.Status_STATUS_UNSPECIFIED
	}
	return DbStatusToProto(string(status.OrganizationMembersStatus))
}

// DbProjectMemberStatusToProto converts NullProjectMembersStatus to proto Status.
func DbProjectMemberStatusToProto(status db.NullProjectMembersStatus) commonv1.Status {
	if !status.Valid {
		return commonv1.Status_STATUS_UNSPECIFIED
	}
	return DbStatusToProto(string(status.ProjectMembersStatus))
}

// DbSiteMemberStatusToProto converts NullSiteMembersStatus to proto Status.
func DbSiteMemberStatusToProto(status db.NullSiteMembersStatus) commonv1.Status {
	if !status.Valid {
		return commonv1.Status_STATUS_UNSPECIFIED
	}
	return DbStatusToProto(string(status.SiteMembersStatus))
}
