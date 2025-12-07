-- Comprehensive RBAC Integration Test Seed Data
-- Creates a root org, child org, projects, sites, and complete member matrix

-- ============================================
-- ACCOUNTS
-- ============================================

-- Admin - Root Organization Owner (has access to everything)
INSERT INTO accounts (id, public_id, email, name, auth_method, verified, vault_entity_id, created_at)
VALUES (
    1,
    UNHEX(REPLACE('10000000-0000-0000-0000-000000000001', '-', '')),
    'admin@root.com',
    'Admin Root',
    'userpass',
    TRUE,
    'e0000000-0000-0000-0000-000000000001',
    NOW()
);

-- Second Org Members
INSERT INTO accounts (id, public_id, email, name, auth_method, verified, vault_entity_id, created_at)
VALUES
    (2, UNHEX(REPLACE('10000000-0000-0000-0000-000000000002', '-', '')), 'org-owner@child.com', 'Org Owner', 'userpass', TRUE, 'e0000000-0000-0000-0000-000000000002', NOW()),
    (3, UNHEX(REPLACE('10000000-0000-0000-0000-000000000003', '-', '')), 'org-developer@child.com', 'Org Developer', 'userpass', TRUE, 'e0000000-0000-0000-0000-000000000003', NOW()),
    (4, UNHEX(REPLACE('10000000-0000-0000-0000-000000000004', '-', '')), 'org-read@child.com', 'Org Read', 'userpass', TRUE, 'e0000000-0000-0000-0000-000000000004', NOW());

-- Project 1 Members
INSERT INTO accounts (id, public_id, email, name, auth_method, verified, vault_entity_id, created_at)
VALUES
    (5, UNHEX(REPLACE('10000000-0000-0000-0000-000000000005', '-', '')), 'proj1-owner@child.com', 'Project 1 Owner', 'userpass', TRUE, 'e0000000-0000-0000-0000-000000000005', NOW()),
    (6, UNHEX(REPLACE('10000000-0000-0000-0000-000000000006', '-', '')), 'proj1-developer@child.com', 'Project 1 Developer', 'userpass', TRUE, 'e0000000-0000-0000-0000-000000000006', NOW()),
    (7, UNHEX(REPLACE('10000000-0000-0000-0000-000000000007', '-', '')), 'proj1-read@child.com', 'Project 1 Read', 'userpass', TRUE, 'e0000000-0000-0000-0000-000000000007', NOW());

-- Site 1 Members
INSERT INTO accounts (id, public_id, email, name, auth_method, verified, vault_entity_id, created_at)
VALUES
    (8, UNHEX(REPLACE('10000000-0000-0000-0000-000000000008', '-', '')), 'site1-owner@child.com', 'Site 1 Owner', 'userpass', TRUE, 'e0000000-0000-0000-0000-000000000008', NOW()),
    (9, UNHEX(REPLACE('10000000-0000-0000-0000-000000000009', '-', '')), 'site1-developer@child.com', 'Site 1 Developer', 'userpass', TRUE, 'e0000000-0000-0000-0000-000000000009', NOW()),
    (10, UNHEX(REPLACE('10000000-0000-0000-0000-000000000010', '-', '')), 'site1-read@child.com', 'Site 1 Read', 'userpass', TRUE, 'e0000000-0000-0000-0000-000000000010', NOW());

-- Project 2 Members (for isolation testing)
INSERT INTO accounts (id, public_id, email, name, auth_method, verified, vault_entity_id, created_at)
VALUES
    (11, UNHEX(REPLACE('10000000-0000-0000-0000-000000000011', '-', '')), 'proj2-owner@child.com', 'Project 2 Owner', 'userpass', TRUE, 'e0000000-0000-0000-0000-000000000011', NOW()),
    (12, UNHEX(REPLACE('10000000-0000-0000-0000-000000000012', '-', '')), 'proj2-developer@child.com', 'Project 2 Developer', 'userpass', TRUE, 'e0000000-0000-0000-0000-000000000012', NOW()),
    (13, UNHEX(REPLACE('10000000-0000-0000-0000-000000000013', '-', '')), 'proj2-read@child.com', 'Project 2 Read', 'userpass', TRUE, 'e0000000-0000-0000-0000-000000000013', NOW());

-- User with no permissions (for negative testing)
INSERT INTO accounts (id, public_id, email, name, auth_method, verified, vault_entity_id, created_at)
VALUES
    (14, UNHEX(REPLACE('10000000-0000-0000-0000-000000000014', '-', '')), 'noaccess@test.com', 'No Access User', 'userpass', TRUE, 'e0000000-0000-0000-0000-000000000014', NOW());

-- ============================================
-- API KEYS (Full Access - No Scope Restrictions)
-- ============================================
-- These keys have no scopes (empty array), meaning they bypass scope checks
-- and rely purely on RBAC (role-based membership). Used for testing phases 1-5.
INSERT INTO api_keys (public_id, account_id, name, description, scopes, active, created_by, created_at)
VALUES
    -- Admin - root org owner (no scope restrictions)
    (UNHEX(REPLACE('20000000-0000-0000-0000-000000000001', '-', '')), 1, 'Admin Full', 'Admin full access - no scope restrictions', '[]', TRUE, 1, NOW()),

    -- Second org members (no scope restrictions)
    (UNHEX(REPLACE('20000000-0000-0000-0000-000000000002', '-', '')), 2, 'Org Owner Full', 'Org owner full access - no scope restrictions', '[]', TRUE, 2, NOW()),
    (UNHEX(REPLACE('20000000-0000-0000-0000-000000000003', '-', '')), 3, 'Org Developer Full', 'Org developer full access - no scope restrictions', '[]', TRUE, 3, NOW()),
    (UNHEX(REPLACE('20000000-0000-0000-0000-000000000004', '-', '')), 4, 'Org Read Full', 'Org read full access - no scope restrictions', '[]', TRUE, 4, NOW()),

    -- Project 1 members (no scope restrictions)
    (UNHEX(REPLACE('20000000-0000-0000-0000-000000000005', '-', '')), 5, 'Proj1 Owner Full', 'Project 1 owner full access - no scope restrictions', '[]', TRUE, 5, NOW()),
    (UNHEX(REPLACE('20000000-0000-0000-0000-000000000006', '-', '')), 6, 'Proj1 Developer Full', 'Project 1 developer full access - no scope restrictions', '[]', TRUE, 6, NOW()),
    (UNHEX(REPLACE('20000000-0000-0000-0000-000000000007', '-', '')), 7, 'Proj1 Read Full', 'Project 1 read full access - no scope restrictions', '[]', TRUE, 7, NOW()),

    -- Site 1 members (no scope restrictions)
    (UNHEX(REPLACE('20000000-0000-0000-0000-000000000008', '-', '')), 8, 'Site1 Owner Full', 'Site 1 owner full access - no scope restrictions', '[]', TRUE, 8, NOW()),
    (UNHEX(REPLACE('20000000-0000-0000-0000-000000000009', '-', '')), 9, 'Site1 Developer Full', 'Site 1 developer full access - no scope restrictions', '[]', TRUE, 9, NOW()),
    (UNHEX(REPLACE('20000000-0000-0000-0000-000000000010', '-', '')), 10, 'Site1 Read Full', 'Site 1 read full access - no scope restrictions', '[]', TRUE, 10, NOW()),

    -- Project 2 members (no scope restrictions)
    (UNHEX(REPLACE('20000000-0000-0000-0000-000000000011', '-', '')), 11, 'Proj2 Owner Full', 'Project 2 owner full access - no scope restrictions', '[]', TRUE, 11, NOW()),
    (UNHEX(REPLACE('20000000-0000-0000-0000-000000000012', '-', '')), 12, 'Proj2 Developer Full', 'Project 2 developer full access - no scope restrictions', '[]', TRUE, 12, NOW()),
    (UNHEX(REPLACE('20000000-0000-0000-0000-000000000013', '-', '')), 13, 'Proj2 Read Full', 'Project 2 read full access - no scope restrictions', '[]', TRUE, 13, NOW()),

    -- No access user (no scope restrictions, no memberships)
    (UNHEX(REPLACE('20000000-0000-0000-0000-000000000014', '-', '')), 14, 'No Access', 'No permissions', '[]', TRUE, 14, NOW());

-- ============================================
-- API KEYS (Limited Scope - for scope restriction testing in Phase 6)
-- ============================================
INSERT INTO api_keys (public_id, account_id, name, description, scopes, active, created_by, created_at)
VALUES
    -- Admin with limited scope (read-only despite being owner)
    (UNHEX(REPLACE('30000000-0000-0000-0000-000000000001', '-', '')), 1, 'Admin Limited', 'Admin read-only scope', '["read:organization", "read:organization"]', TRUE, 1, NOW()),

    -- Org owner with project-only scope
    (UNHEX(REPLACE('30000000-0000-0000-0000-000000000002', '-', '')), 2, 'Org Owner Limited', 'Org owner project scope only', '["read:project"]', TRUE, 2, NOW()),

    -- Project 1 owner with delete:project scope (for scope restriction testing)
    (UNHEX(REPLACE('30000000-0000-0000-0000-000000000005', '-', '')), 5, 'Proj1 Owner Limited', 'Project 1 owner with delete:project scope only', '["delete:project"]', TRUE, 5, NOW()),

    -- Site 1 owner with read:site scope (for scope restriction testing)
    (UNHEX(REPLACE('30000000-0000-0000-0000-000000000008', '-', '')), 8, 'Site1 Owner Limited', 'Site 1 owner with delete:site scope only', '["read:site"]', TRUE, 8, NOW());

-- ============================================
-- ORGANIZATIONS
-- ============================================

-- Root Organization (Joe is owner)
INSERT INTO organizations (id, public_id, name, gcp_org_id, gcp_billing_account, gcp_parent, location, region, gcp_folder_id, status, gcp_project_id, gcp_project_number, created_by, created_at)
VALUES (
    1,
    UNHEX(REPLACE('40000000-0000-0000-0000-000000000001', '-', '')),
    'Root Organization',
    '111111111',
    'ROOT-BILLING-ACCOUNT',
    'organizations/111111111',
    'us',
    'us-central1',
    'folders/100000000001',
    'active',
    'root-org-platform',
    '100000001',
    1,
    NOW()
);

-- Child Organization (member of Root)
INSERT INTO organizations (id, public_id, name, gcp_org_id, gcp_billing_account, gcp_parent, location, region, gcp_folder_id, status, gcp_project_id, gcp_project_number, created_by, created_at)
VALUES (
    2,
    UNHEX(REPLACE('40000000-0000-0000-0000-000000000002', '-', '')),
    'Child Organization',
    '222222222',
    'CHILD-BILLING-ACCOUNT',
    'organizations/222222222',
    'us',
    'us-east1',
    'folders/200000000002',
    'active',
    'child-org-platform',
    '200000002',
    1,
    NOW()
);

INSERT INTO relationships (id, public_id, source_organization_id, target_organization_id, relationship_type, status)
VALUES (
    1,
    UNHEX(REPLACE('50000000-0000-0000-0000-000000000002', '-', '')),
    1,
    2,
    'access',
    'approved'
);

-- ============================================
-- ORGANIZATION MEMBERS
-- ============================================

-- Root org members
INSERT INTO organization_members (organization_id, account_id, role, status, created_by, created_at)
VALUES
    (1, 1, 'owner', 'active', 1, NOW());

-- Child org members
INSERT INTO organization_members (organization_id, account_id, role, status, created_by, created_at)
VALUES
    (2, 2, 'owner', 'active', 1, NOW()),
    (2, 3, 'developer', 'active', 2, NOW()),
    (2, 4, 'read', 'active', 2, NOW());

-- ============================================
-- PROJECTS
-- ============================================

-- Project 1 (in Child Org)
INSERT INTO projects (id, public_id, organization_id, name, github_repository, github_branch, gcp_region, gcp_zone, machine_type, gcp_project_id, gcp_project_number, status, organization_project, created_by, created_at)
VALUES (
    1,
    UNHEX(REPLACE('50000000-0000-0000-0000-000000000001', '-', '')),
    2,  -- child org
    'Project Alpha',
    'child-org/project-alpha',
    'main',
    'us-east1',
    'us-east1-b',
    'e2-medium',
    'child-project-alpha',
    '300000001',
    'active',
    TRUE, -- Set as organization project
    2,
    NOW()
);

-- Project 3 (in Root Org, for secrets)
INSERT INTO projects (id, public_id, organization_id, name, github_repository, github_branch, gcp_region, gcp_zone, machine_type, gcp_project_id, gcp_project_number, status, organization_project, created_by, created_at)
VALUES (
    3,
    UNHEX(REPLACE('50000000-0000-0000-0000-000000000003', '-', '')),
    1,  -- root org
    'Root Platform',
    'root-org/platform',
    'main',
    'us-central1',
    'us-central1-a',
    'e2-medium',
    'root-org-platform',
    '100000001',
    'active',
    TRUE, -- Set as organization project
    1,
    NOW()
);

-- Project 2 (in Child Org, for isolation testing)
INSERT INTO projects (id, public_id, organization_id, name, github_repository, github_branch, gcp_region, gcp_zone, machine_type, gcp_project_id, gcp_project_number, status, created_by, created_at)
VALUES (
    2,
    UNHEX(REPLACE('50000000-0000-0000-0000-000000000002', '-', '')),
    2,  -- child org
    'Project Beta',
    'child-org/project-beta',
    'main',
    'us-east1',
    'us-east1-c',
    'e2-standard-2',
    'child-project-beta',
    '300000002',
    'active',
    2,
    NOW()
);

-- ============================================
-- PROJECT MEMBERS
-- ============================================

-- Project 1 members
INSERT INTO project_members (project_id, account_id, role, status, created_by, created_at)
VALUES
    (1, 5, 'owner', 'active', 2, NOW()),
    (1, 6, 'developer', 'active', 5, NOW()),
    (1, 7, 'read', 'active', 5, NOW());

-- Project 2 members
INSERT INTO project_members (project_id, account_id, role, status, created_by, created_at)
VALUES
    (2, 11, 'owner', 'active', 2, NOW()),
    (2, 12, 'developer', 'active', 11, NOW()),
    (2, 13, 'read', 'active', 11, NOW());

-- ============================================
-- SITES
-- ============================================

-- Site 1 (in Project 1)
INSERT INTO sites (id, public_id, project_id, name, github_ref, gcp_external_ip, status, created_by, created_at)
VALUES
    (1, UNHEX(REPLACE('60000000-0000-0000-0000-000000000001', '-', '')), 1, 'production', 'tags/v1.0.0', '34.100.100.1', 'active', 5, NOW()),
    (2, UNHEX(REPLACE('60000000-0000-0000-0000-000000000002', '-', '')), 1, 'staging', 'heads/develop', '34.100.100.2', 'active', 5, NOW());

-- Site 3 (in Project 2, for isolation testing)
INSERT INTO sites (id, public_id, project_id, name, github_ref, gcp_external_ip, status, created_by, created_at)
VALUES
    (3, UNHEX(REPLACE('60000000-0000-0000-0000-000000000003', '-', '')), 2, 'production', 'tags/v1.0.0', '34.100.100.3', 'active', 11, NOW());

-- ============================================
-- SITE MEMBERS
-- ============================================

-- Site 1 (production) members
INSERT INTO site_members (site_id, account_id, role, status, created_by, created_at)
VALUES
    (1, 8, 'owner', 'active', 5, NOW()),
    (1, 9, 'developer', 'active', 8, NOW()),
    (1, 10, 'read', 'active', 8, NOW());

-- ============================================
-- SECRETS
-- ============================================

-- Root org secrets
INSERT INTO organization_secrets (public_id, organization_id, name, vault_path, status, created_at, updated_at, created_by)
VALUES
    (UNHEX(REPLACE('70000000-0000-0000-0000-000000000001', '-', '')), 1, 'ROOT_SECRET_1', 'secret-organization/1/ROOT_SECRET_1', 'active', UNIX_TIMESTAMP(), UNIX_TIMESTAMP(), 1),
    (UNHEX(REPLACE('70000000-0000-0000-0000-000000000002', '-', '')), 1, 'ROOT_SECRET_2', 'secret-organization/1/ROOT_SECRET_2', 'active', UNIX_TIMESTAMP(), UNIX_TIMESTAMP(), 1);

-- Child org secrets
INSERT INTO organization_secrets (public_id, organization_id, name, vault_path, status, created_at, updated_at, created_by)
VALUES
    (UNHEX(REPLACE('70000000-0000-0000-0000-000000000003', '-', '')), 2, 'CHILD_ORG_SECRET_1', 'secret-organization/2/CHILD_ORG_SECRET_1', 'active', UNIX_TIMESTAMP(), UNIX_TIMESTAMP(), 2),
    (UNHEX(REPLACE('70000000-0000-0000-0000-000000000004', '-', '')), 2, 'CHILD_ORG_SECRET_2', 'secret-organization/2/CHILD_ORG_SECRET_2', 'active', UNIX_TIMESTAMP(), UNIX_TIMESTAMP(), 2);

-- Project 1 secrets
INSERT INTO project_secrets (public_id, project_id, name, vault_path, status, created_at, updated_at, created_by)
VALUES
    (UNHEX(REPLACE('71000000-0000-0000-0000-000000000001', '-', '')), 1, 'PROJ1_SECRET_1', 'secret-project/1/PROJ1_SECRET_1', 'active', UNIX_TIMESTAMP(), UNIX_TIMESTAMP(), 5),
    (UNHEX(REPLACE('71000000-0000-0000-0000-000000000002', '-', '')), 1, 'PROJ1_SECRET_2', 'secret-project/1/PROJ1_SECRET_2', 'active', UNIX_TIMESTAMP(), UNIX_TIMESTAMP(), 5);

-- Project 2 secrets
INSERT INTO project_secrets (public_id, project_id, name, vault_path, status, created_at, updated_at, created_by)
VALUES
    (UNHEX(REPLACE('71000000-0000-0000-0000-000000000003', '-', '')), 2, 'PROJ2_SECRET_1', 'secret-project/2/PROJ2_SECRET_1', 'active', UNIX_TIMESTAMP(), UNIX_TIMESTAMP(), 11);

-- Site 1 secrets
INSERT INTO site_secrets (public_id, site_id, name, vault_path, status, created_at, updated_at, created_by)
VALUES
    (UNHEX(REPLACE('72000000-0000-0000-0000-000000000001', '-', '')), 1, 'SITE1_SECRET_1', 'secret-site/1/SITE1_SECRET_1', 'active', UNIX_TIMESTAMP(), UNIX_TIMESTAMP(), 8),
    (UNHEX(REPLACE('72000000-0000-0000-0000-000000000002', '-', '')), 1, 'SITE1_SECRET_2', 'secret-site/1/SITE1_SECRET_2', 'active', UNIX_TIMESTAMP(), UNIX_TIMESTAMP(), 8);

-- ============================================
-- FIREWALL RULES
-- ============================================

-- Root org firewall rules
INSERT INTO organization_firewall_rules (public_id, organization_id, description, source_ip, status, created_at, updated_at, created_by)
VALUES
    (UNHEX(REPLACE('80000000-0000-0000-0000-000000000001', '-', '')), 1, 'Root org rule 1', '10.0.0.0/8', 'active', NOW(), NOW(), 1);

-- Child org firewall rules
INSERT INTO organization_firewall_rules (public_id, organization_id, description, source_ip, status, created_at, updated_at, created_by)
VALUES
    (UNHEX(REPLACE('80000000-0000-0000-0000-000000000002', '-', '')), 2, 'Child org rule 1', '172.16.0.0/12', 'active', NOW(), NOW(), 2);

-- Project 1 firewall rules
INSERT INTO project_firewall_rules (public_id, project_id, description, source_ip, status, created_at, updated_at, created_by)
VALUES
    (UNHEX(REPLACE('81000000-0000-0000-0000-000000000001', '-', '')), 1, 'Project 1 rule 1', '192.168.1.0/24', 'active', NOW(), NOW(), 5);

-- Project 2 firewall rules
INSERT INTO project_firewall_rules (public_id, project_id, description, source_ip, status, created_at, updated_at, created_by)
VALUES
    (UNHEX(REPLACE('81000000-0000-0000-0000-000000000002', '-', '')), 2, 'Project 2 rule 1', '192.168.2.0/24', 'active', NOW(), NOW(), 11);

-- Site 1 firewall rules
INSERT INTO site_firewall_rules (public_id, site_id, description, source_ip, status, created_at, updated_at, created_by)
VALUES
    (UNHEX(REPLACE('82000000-0000-0000-0000-000000000001', '-', '')), 1, 'Site 1 rule 1', '192.168.100.0/24', 'active', NOW(), NOW(), 8);

-- ============================================
-- SUMMARY
-- ============================================
-- Resource Hierarchy:
--   Root Org (admin=owner)
--    └── Child Org (org-owner=owner, org-developer=developer, org-read=read)
--         ├── Project 1 (proj1-owner=owner, proj1-developer=developer, proj1-read=read)
--         │    ├── Site 1 production (site1-owner=owner, site1-developer=developer, site1-read=read)
--         │    └── Site 2 staging (no additional members)
--         └── Project 2 (proj2-owner=owner, proj2-developer=developer, proj2-read=read)
--              └── Site 3 production (no additional members)
--
-- Total Accounts: 14
-- Total API Keys: 18 (14 full scope + 4 limited scope)
-- Total Organizations: 2
-- Total Projects: 2
-- Total Sites: 3
-- Total Secrets: 7 (2 root org, 2 child org, 3 project, 2 site)
-- Total Firewall Rules: 5 (1 root org, 1 child org, 2 project, 1 site)
