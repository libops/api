INSERT IGNORE INTO accounts
(
    id,
    public_id,
    email,
    name,
    auth_method,
    verified,
    vault_entity_id,
    created_at
)
SELECT
    1,
    UNHEX(REPLACE('10000000-0000-0000-0000-000000000001', '-', '')),
    'joe@libops.io',
    'Joe Corall',
    'google',
    TRUE,
    'e0000000-0000-0000-0000-000000000001',
    NOW()
FROM
    DUAL
WHERE
    (SELECT COUNT(*) FROM accounts) = 0
UNION ALL
SELECT
    2,
    UNHEX(REPLACE('20000000-0000-0000-0000-000000000001', '-', '')),
    'joe.corall@gmail.com',
    'Joe Corall (external)',
    'google',
    TRUE,
    'e0000000-0000-0000-0000-000000000002',
    NOW()
FROM
    DUAL
WHERE
    (SELECT COUNT(*) FROM organizations) = 0;

INSERT IGNORE INTO organizations
(
    id,
    public_id,
    name,
    gcp_org_id,
    gcp_billing_account,
    gcp_parent,
    location,
    region,
    gcp_folder_id,
    status,
    gcp_project_id,
    gcp_project_number,
    created_by,
    created_at
)
-- Root Organization
SELECT
    1,
    UNHEX(REPLACE('40000000-0000-0000-0000-000000000001', '-', '')),
    'libops',
    '',
    '',
    '',
    'us',
    'us-east5',
    '',
    'active',
    'libops-api',
    '',
    1,
    NOW()
FROM
    DUAL
WHERE
    (SELECT COUNT(*) FROM organizations) = 0
UNION ALL
-- Child Organization (member of Root)
SELECT
    2,
    UNHEX(REPLACE('40000000-0000-0000-0000-000000000002', '-', '')),
    'customer site',
    '',
    '',
    '',
    'us',
    'us-east1',
    '',
    'active',
    '',
    '',
    1,
    NOW()
FROM
    DUAL
WHERE
    (SELECT COUNT(*) FROM organizations) = 0;

-- ------------------------------
-- 3. relationships Table Seeding
-- ------------------------------
INSERT IGNORE INTO relationships
(
    id,
    public_id,
    source_organization_id,
    target_organization_id,
    relationship_type,
    status
)
SELECT
    1,
    UNHEX(REPLACE('50000000-0000-0000-0000-000000000002', '-', '')),
    1,
    2,
    'access',
    'approved'
FROM
    DUAL
WHERE
    (SELECT COUNT(*) FROM relationships) = 0;


INSERT IGNORE INTO organization_members
(
    organization_id,
    account_id,
    role,
    status,
    created_by,
    created_at
)
SELECT 1, 1, 'owner', 'active', 1, NOW()
FROM DUAL WHERE (SELECT COUNT(*) FROM organization_members) = 0
UNION ALL
SELECT 2, 2, 'owner', 'active', 1, NOW()
FROM DUAL WHERE (SELECT COUNT(*) FROM organization_members) = 0;
