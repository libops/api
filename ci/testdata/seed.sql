-- Integration test seed data
-- Creates test accounts with different permission levels and organizational structures

-- Test Accounts
-- Account 1: System Admin
INSERT INTO accounts (id, public_id, email, name, auth_method, verified, vault_entity_id, created_at)
VALUES (
    1,
    UNHEX(REPLACE('11111111-1111-1111-1111-111111111111', '-', '')),
    'admin@test.libops.io',
    'System Admin',
    'userpass',
    TRUE,
    'entity-admin',
    NOW()
);

-- Account 2: Organization Owner (Acme Corp)
INSERT INTO accounts (id, public_id, email, name, auth_method, verified, vault_entity_id, created_at)
VALUES (
    2,
    UNHEX(REPLACE('22222222-2222-2222-2222-222222222222', '-', '')),
    'owner@acme.com',
    'Acme Owner',
    'userpass',
    TRUE,
    'entity-acme-owner',
    NOW()
);

-- Account 3: Organization Read-Only (Acme Corp)
INSERT INTO accounts (id, public_id, email, name, auth_method, verified, vault_entity_id, created_at)
VALUES (
    3,
    UNHEX(REPLACE('33333333-3333-3333-3333-333333333333', '-', '')),
    'viewer@acme.com',
    'Acme Viewer',
    'userpass',
    TRUE,
    'entity-acme-viewer',
    NOW()
);

-- Account 4: Project Developer (Web App)
INSERT INTO accounts (id, public_id, email, name, auth_method, verified, vault_entity_id, created_at)
VALUES (
    4,
    UNHEX(REPLACE('44444444-4444-4444-4444-444444444444', '-', '')),
    'dev@acme.com',
    'Web Developer',
    'userpass',
    TRUE,
    'entity-dev',
    NOW()
);

-- Account 5: Project Read-Only (Web App)
INSERT INTO accounts (id, public_id, email, name, auth_method, verified, vault_entity_id, created_at)
VALUES (
    5,
    UNHEX(REPLACE('55555555-5555-5555-5555-555555555555', '-', '')),
    'readonly@acme.com',
    'Read Only User',
    'userpass',
    TRUE,
    'entity-readonly',
    NOW()
);

-- Account 6: Site Admin (Staging)
INSERT INTO accounts (id, public_id, email, name, auth_method, verified, vault_entity_id, created_at)
VALUES (
    6,
    UNHEX(REPLACE('66666666-6666-6666-6666-666666666666', '-', '')),
    'staging-admin@acme.com',
    'Staging Admin',
    'userpass',
    TRUE,
    'entity-staging-admin',
    NOW()
);

-- Account 7: No Permissions
INSERT INTO accounts (id, public_id, email, name, auth_method, verified, vault_entity_id, created_at)
VALUES (
    7,
    UNHEX(REPLACE('77777777-7777-7777-7777-777777777777', '-', '')),
    'noperms@test.com',
    'No Permissions User',
    'userpass',
    TRUE,
    'entity-noperms',
    NOW()
);

-- Account 8: Different Organization Owner (TechStart)
INSERT INTO accounts (id, public_id, email, name, auth_method, verified, vault_entity_id, created_at)
VALUES (
    8,
    UNHEX(REPLACE('88888888-8888-8888-8888-888888888888', '-', '')),
    'owner@techstart.io',
    'TechStart Owner',
    'userpass',
    TRUE,
    'entity-techstart-owner',
    NOW()
);

-- API Keys
INSERT INTO api_keys (public_id, account_id, name, description, scopes, active, created_by, created_at)
VALUES
    (UNHEX(REPLACE('11111111-1111-1111-1111-111111111111', '-', '')), 1, 'Admin Key', 'System admin key', '["admin:system"]', TRUE, 1, NOW()),
    (UNHEX(REPLACE('22222222-2222-2222-2222-222222222222', '-', '')), 2, 'Org Owner Key', 'Organization owner key', '["admin:organization"]', TRUE, 2, NOW()),
    (UNHEX(REPLACE('33333333-3333-3333-3333-333333333333', '-', '')), 3, 'Org Read Key', 'Organization read-only key', '["read:organization"]', TRUE, 3, NOW()),
    (UNHEX(REPLACE('44444444-4444-4444-4444-444444444444', '-', '')), 4, 'Project Dev Key', 'Project developer key', '["write:project"]', TRUE, 4, NOW()),
    (UNHEX(REPLACE('55555555-5555-5555-5555-555555555555', '-', '')), 5, 'Project Read Key', 'Project read-only key', '["read:project"]', TRUE, 5, NOW()),
    (UNHEX(REPLACE('66666666-6666-6666-6666-666666666666', '-', '')), 6, 'Site Admin Key', 'Site admin key', '["admin:site"]', TRUE, 6, NOW()),
    (UNHEX(REPLACE('77777777-7777-7777-7777-777777777777', '-', '')), 7, 'No Perms Key', 'No permissions key', '[]', TRUE, 7, NOW());

-- Organizations
-- Organization 1: Acme Corp
INSERT INTO organizations (id, public_id, name, gcp_org_id, gcp_billing_account, gcp_parent, location, region, gcp_folder_id, status, gcp_project_id, gcp_project_number, created_by, created_at)
VALUES (
    1,
    UNHEX(REPLACE('aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa', '-', '')),
    'Acme Corp',
    '123456789',
    'ABCDEF-123456-GHIJKL',
    'organizations/123456789',
    'us',
    'us-central1',
    'folders/111111111111',
    'active',
    'acme-corp-platform',
    '987654321',
    2,
    NOW()
);

-- Organization 2: TechStart Inc
INSERT INTO organizations (id, public_id, name, gcp_org_id, gcp_billing_account, gcp_parent, location, region, gcp_folder_id, status, gcp_project_id, gcp_project_number, created_by, created_at)
VALUES (
    2,
    UNHEX(REPLACE('bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb', '-', '')),
    'TechStart Inc',
    '987654321',
    'ZYXWVU-654321-TSRQPO',
    'organizations/987654321',
    'eu',
    'europe-west1',
    'folders/222222222222',
    'active',
    'techstart-platform',
    '123456789',
    8,
    NOW()
);

-- Organization Members
-- Acme Corp members
INSERT INTO organization_members (organization_id, account_id, role, status, created_by, created_at)
VALUES
    (1, 2, 'owner', 'active', 2, NOW()),
    (1, 3, 'read', 'active', 2, NOW()),
    (1, 4, 'developer', 'active', 2, NOW()),
    (1, 5, 'read', 'active', 2, NOW()),
    (1, 6, 'developer', 'active', 2, NOW());

-- TechStart members
INSERT INTO organization_members (organization_id, account_id, role, status, created_by, created_at)
VALUES
    (2, 8, 'owner', 'active', 8, NOW());

-- Projects
-- Project 1: Acme Web App
INSERT INTO projects (id, public_id, organization_id, name, github_repository, github_branch, gcp_region, gcp_zone, machine_type, gcp_project_id, gcp_project_number, status, created_by, created_at)
VALUES (
    1,
    UNHEX(REPLACE('cccccccc-cccc-cccc-cccc-cccccccccccc', '-', '')),
    1,
    'Web App',
    'acme/web-app',
    'main',
    'us-central1',
    'us-central1-c',
    'e2-medium',
    'acme-web-app',
    '111222333',
    'active',
    2,
    NOW()
);

-- Project 2: Acme API
INSERT INTO projects (id, public_id, organization_id, name, github_repository, github_branch, gcp_region, gcp_zone, machine_type, gcp_project_id, gcp_project_number, status, created_by, created_at)
VALUES (
    2,
    UNHEX(REPLACE('dddddddd-dddd-dddd-dddd-dddddddddddd', '-', '')),
    1,
    'API Backend',
    'acme/api',
    'main',
    'us-central1',
    'us-central1-c',
    'e2-standard-2',
    'acme-api-backend',
    '444555666',
    'active',
    2,
    NOW()
);

-- Project 3: TechStart Platform
INSERT INTO projects (id, public_id, organization_id, name, github_repository, github_branch, gcp_region, gcp_zone, machine_type, gcp_project_id, gcp_project_number, status, created_by, created_at)
VALUES (
    3,
    UNHEX(REPLACE('eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee', '-', '')),
    2,
    'Platform',
    'techstart/platform',
    'main',
    'europe-west1',
    'europe-west1-b',
    'e2-medium',
    'techstart-platform-prod',
    '777888999',
    'active',
    8,
    NOW()
);

-- Project Members
-- Web App project members
INSERT INTO project_members (project_id, account_id, role, status, created_by, created_at)
VALUES
    (1, 2, 'owner', 'active', 2, NOW()),
    (1, 4, 'developer', 'active', 2, NOW()),
    (1, 5, 'read', 'active', 2, NOW()),
    (1, 6, 'developer', 'active', 2, NOW());

-- API Backend project members
INSERT INTO project_members (project_id, account_id, role, status, created_by, created_at)
VALUES
    (2, 2, 'owner', 'active', 2, NOW()),
    (2, 4, 'developer', 'active', 2, NOW());

-- TechStart Platform members
INSERT INTO project_members (project_id, account_id, role, status, created_by, created_at)
VALUES
    (3, 8, 'owner', 'active', 8, NOW());

-- Sites
-- Web App Sites
INSERT INTO sites (id, public_id, project_id, name, github_ref, gcp_external_ip, status, created_by, created_at)
VALUES
    (1, UNHEX(REPLACE('11111111-aaaa-aaaa-aaaa-111111111111', '-', '')), 1, 'production', 'tags/v1.0.0', '34.123.45.67', 'active', 2, NOW()),
    (2, UNHEX(REPLACE('22222222-aaaa-aaaa-aaaa-222222222222', '-', '')), 1, 'staging', 'heads/develop', '34.123.45.68', 'active', 2, NOW()),
    (3, UNHEX(REPLACE('33333333-aaaa-aaaa-aaaa-333333333333', '-', '')), 1, 'dev', 'heads/main', '34.123.45.69', 'active', 4, NOW());

-- API Backend Sites
INSERT INTO sites (id, public_id, project_id, name, github_ref, gcp_external_ip, status, created_by, created_at)
VALUES
    (4, UNHEX(REPLACE('44444444-aaaa-aaaa-aaaa-444444444444', '-', '')), 2, 'production', 'tags/v2.1.0', '35.234.56.78', 'active', 2, NOW()),
    (5, UNHEX(REPLACE('55555555-aaaa-aaaa-aaaa-555555555555', '-', '')), 2, 'staging', 'heads/develop', '35.234.56.79', 'active', 2, NOW());

-- TechStart Platform Sites
INSERT INTO sites (id, public_id, project_id, name, github_ref, gcp_external_ip, status, created_by, created_at)
VALUES
    (6, UNHEX(REPLACE('66666666-aaaa-aaaa-aaaa-666666666666', '-', '')), 3, 'production', 'tags/v1.5.2', '34.77.88.99', 'active', 8, NOW());

-- Site Members
-- Web App staging site members
INSERT INTO site_members (site_id, account_id, role, status, created_by, created_at)
VALUES
    (2, 6, 'owner', 'active', 2, NOW()),
    (2, 4, 'developer', 'active', 2, NOW());

-- Secrets
-- Organization secrets
INSERT INTO organization_secrets (public_id, organization_id, name, vault_path, status, created_at, updated_at, created_by)
VALUES
    (UNHEX(REPLACE('aaaabbbb-1111-2222-3333-444444444444', '-', '')), 1, 'GCP_SERVICE_ACCOUNT', 'secret-organization/1/GCP_SERVICE_ACCOUNT', 'active', UNIX_TIMESTAMP(), UNIX_TIMESTAMP(), 2),
    (UNHEX(REPLACE('aaaabbbb-2222-2222-3333-444444444444', '-', '')), 1, 'GITHUB_TOKEN', 'secret-organization/1/GITHUB_TOKEN', 'active', UNIX_TIMESTAMP(), UNIX_TIMESTAMP(), 2);

-- Project secrets
INSERT INTO project_secrets (public_id, project_id, name, vault_path, status, created_at, updated_at, created_by)
VALUES
    (UNHEX(REPLACE('bbbbcccc-1111-2222-3333-444444444444', '-', '')), 1, 'DATABASE_URL', 'secret-project/1/DATABASE_URL', 'active', UNIX_TIMESTAMP(), UNIX_TIMESTAMP(), 2),
    (UNHEX(REPLACE('bbbbcccc-2222-2222-3333-444444444444', '-', '')), 1, 'API_KEY', 'secret-project/1/API_KEY', 'active', UNIX_TIMESTAMP(), UNIX_TIMESTAMP(), 2);

-- Site secrets
INSERT INTO site_secrets (public_id, site_id, name, vault_path, status, created_at, updated_at, created_by)
VALUES
    (UNHEX(REPLACE('ccccdddd-1111-2222-3333-444444444444', '-', '')), 2, 'STRIPE_KEY', 'secret-site/2/STRIPE_KEY', 'active', UNIX_TIMESTAMP(), UNIX_TIMESTAMP(), 6),
    (UNHEX(REPLACE('ccccdddd-2222-2222-3333-444444444444', '-', '')), 2, 'JWT_SECRET', 'secret-site/2/JWT_SECRET', 'active', UNIX_TIMESTAMP(), UNIX_TIMESTAMP(), 6);

-- Summary of test data:
-- Organizations: 2 (Acme Corp, TechStart Inc)
-- Accounts: 8 (admin, org owners, developers, readers, site admin, no perms)
-- Projects: 3 (2 for Acme, 1 for TechStart)
-- Sites: 6 (3 for Web App, 2 for API, 1 for TechStart)
--
-- Permission Hierarchy:
-- - Account 1: System admin (all access)
-- - Account 2: Acme owner (can manage Acme org, projects, sites)
-- - Account 3: Acme viewer (read-only access to Acme resources)
-- - Account 4: Web App & API developer (write access to projects 1 & 2)
-- - Account 5: Web App reader (read-only access to project 1)
-- - Account 6: Staging site admin (admin access to staging site only)
-- - Account 7: No permissions (should be denied all operations)
-- - Account 8: TechStart owner (can manage TechStart org, cannot access Acme)
