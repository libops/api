// Package database provides queries for finding affected resources
package database

import (
	"context"
	"database/sql"
	"fmt"
)

// Site represents a site from the database
type Site struct {
	ID              int64
	PublicID        string
	ProjectID       int64
	ProjectPublicID string
	OrgID           int64
	OrgPublicID     string
}

// Querier provides methods to query the database for affected resources
type Querier struct {
	db *sql.DB
}

// NewQuerier creates a new database querier
func NewQuerier(db *sql.DB) *Querier {
	return &Querier{db: db}
}

// GetSitesForOrg returns all sites in an organization
func (q *Querier) GetSitesForOrg(ctx context.Context, orgID int64) ([]Site, error) {
	query := `
		SELECT s.id, BIN_TO_UUID(s.public_id), s.project_id, BIN_TO_UUID(p.public_id),
		       p.organization_id, BIN_TO_UUID(o.public_id)
		FROM sites s
		JOIN projects p ON s.project_id = p.id
		JOIN organizations o ON p.organization_id = o.id
		WHERE p.organization_id = ?
		AND s.status != 'deleted'
	`

	rows, err := q.db.QueryContext(ctx, query, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to query sites for org %d: %w", orgID, err)
	}
	defer rows.Close()

	var sites []Site
	for rows.Next() {
		var site Site
		if err := rows.Scan(&site.ID, &site.PublicID, &site.ProjectID, &site.ProjectPublicID, &site.OrgID, &site.OrgPublicID); err != nil {
			return nil, fmt.Errorf("failed to scan site: %w", err)
		}
		sites = append(sites, site)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating sites: %w", err)
	}

	return sites, nil
}

// GetSitesForProject returns all sites in a project
func (q *Querier) GetSitesForProject(ctx context.Context, projectID int64) ([]Site, error) {
	query := `
		SELECT s.id, BIN_TO_UUID(s.public_id), s.project_id, BIN_TO_UUID(p.public_id),
		       p.organization_id, BIN_TO_UUID(o.public_id)
		FROM sites s
		JOIN projects p ON s.project_id = p.id
		JOIN organizations o ON p.organization_id = o.id
		WHERE s.project_id = ?
		AND s.status != 'deleted'
	`

	rows, err := q.db.QueryContext(ctx, query, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to query sites for project %d: %w", projectID, err)
	}
	defer rows.Close()

	var sites []Site
	for rows.Next() {
		var site Site
		if err := rows.Scan(&site.ID, &site.PublicID, &site.ProjectID, &site.ProjectPublicID, &site.OrgID, &site.OrgPublicID); err != nil {
			return nil, fmt.Errorf("failed to scan site: %w", err)
		}
		sites = append(sites, site)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating sites: %w", err)
	}

	return sites, nil
}

// GetSite returns a single site by ID
func (q *Querier) GetSite(ctx context.Context, siteID int64) (*Site, error) {
	query := `
		SELECT s.id, BIN_TO_UUID(s.public_id), s.project_id, BIN_TO_UUID(p.public_id),
		       p.organization_id, BIN_TO_UUID(o.public_id)
		FROM sites s
		JOIN projects p ON s.project_id = p.id
		JOIN organizations o ON p.organization_id = o.id
		WHERE s.id = ?
		AND s.status != 'deleted'
	`

	var site Site
	err := q.db.QueryRowContext(ctx, query, siteID).Scan(
		&site.ID,
		&site.PublicID,
		&site.ProjectID,
		&site.ProjectPublicID,
		&site.OrgID,
		&site.OrgPublicID,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("site %d not found", siteID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query site %d: %w", siteID, err)
	}

	return &site, nil
}

// GetSitesForOrgMembers returns all sites that need SSH key reconciliation when an org member is added/removed
func (q *Querier) GetSitesForOrgMembers(ctx context.Context, orgID int64) ([]Site, error) {
	// Same as GetSitesForOrg - all sites in org need SSH keys updated
	return q.GetSitesForOrg(ctx, orgID)
}

// GetSitesForProjectMembers returns all sites that need SSH key reconciliation when a project member is added/removed
func (q *Querier) GetSitesForProjectMembers(ctx context.Context, projectID int64) ([]Site, error) {
	// Same as GetSitesForProject - all sites in project need SSH keys updated
	return q.GetSitesForProject(ctx, projectID)
}

// GetSitesForSiteMembers returns the site that needs SSH key reconciliation when a site member is added/removed
func (q *Querier) GetSitesForSiteMembers(ctx context.Context, siteID int64) ([]Site, error) {
	site, err := q.GetSite(ctx, siteID)
	if err != nil {
		return nil, err
	}
	return []Site{*site}, nil
}
