-- name: ListUserFirewallRules :many
WITH RECURSIVE user_orgs AS (
    SELECT organization_id FROM organization_members WHERE organization_members.account_id = sqlc.arg(account_id) AND organization_members.status = 'active'
    UNION DISTINCT
    SELECT r.target_organization_id
    FROM relationships r
    INNER JOIN user_orgs uo ON r.source_organization_id = uo.organization_id
    WHERE r.status = 'approved'
)
SELECT * FROM (
    SELECT
        ofr.id, BIN_TO_UUID(ofr.public_id) AS public_id, ofr.name, ofr.status, ofr.created_at, ofr.updated_at, ofr.rule_type, ofr.cidr,
        'organization' AS parent_type,
        o.name AS parent_name,
        BIN_TO_UUID(o.public_id) AS parent_public_id
    FROM organization_firewall_rules ofr
    JOIN organizations o ON ofr.organization_id = o.id
    JOIN user_orgs uo ON o.id = uo.organization_id
    WHERE ofr.status != 'deleted'

    UNION ALL

    SELECT
        pfr.id, BIN_TO_UUID(pfr.public_id) AS public_id, pfr.name, pfr.status, pfr.created_at, pfr.updated_at, pfr.rule_type, pfr.cidr,
        'project' AS parent_type,
        p.name AS parent_name,
        BIN_TO_UUID(p.public_id) AS parent_public_id
    FROM project_firewall_rules pfr
    JOIN projects p ON pfr.project_id = p.id
    LEFT JOIN project_members pm ON p.id = pm.project_id AND pm.account_id = sqlc.arg(account_id) AND pm.status = 'active'
    LEFT JOIN user_orgs uo ON p.organization_id = uo.organization_id
    WHERE pfr.status != 'deleted'
    AND (pm.id IS NOT NULL OR uo.organization_id IS NOT NULL)

    UNION ALL

    SELECT
        sfr.id, BIN_TO_UUID(sfr.public_id) AS public_id, sfr.name, sfr.status, sfr.created_at, sfr.updated_at, sfr.rule_type, sfr.cidr,
        'site' AS parent_type,
        s.name AS parent_name,
        BIN_TO_UUID(s.public_id) AS parent_public_id
    FROM site_firewall_rules sfr
    JOIN sites s ON sfr.site_id = s.id
    JOIN projects p ON s.project_id = p.id
    LEFT JOIN site_members sm ON s.id = sm.site_id AND sm.account_id = sqlc.arg(account_id) AND sm.status = 'active'
    LEFT JOIN project_members pm ON p.id = pm.project_id AND pm.account_id = sqlc.arg(account_id) AND pm.status = 'active'
    LEFT JOIN user_orgs uo ON p.organization_id = uo.organization_id
    WHERE sfr.status != 'deleted'
    AND (sm.id IS NOT NULL OR pm.id IS NOT NULL OR uo.organization_id IS NOT NULL)
) AS all_rules
ORDER BY created_at DESC
LIMIT ? OFFSET ?;


