#!/usr/bin/env python3
"""
Master Seed Data Generator for LibOps Platform
Generates consistent seed.sql and vault-init.sh from a single source of truth.
"""

import hashlib
import json
import os
import uuid

def get_uuid(seed):
    """Generate a deterministic valid UUID from a seed string using UUIDv5."""
    return str(uuid.uuid5(uuid.NAMESPACE_DNS, seed))

# =============================================================================
# DATA DEFINITIONS
# =============================================================================

class Resource:
    def __init__(self, name, id_seed):
        self.name = name
        self.id_seed = id_seed
        self.uuid = get_uuid(id_seed)
        # Integer ID will be assigned during processing
        self.id = 0 

class Account(Resource):
    def __init__(self, name, email, id_seed, password="password123"):
        super().__init__(name, "account-" + id_seed)
        self.email = email
        self.password = password
        self.raw_seed = id_seed
        self.api_keys = []
        self.ssh_keys = []

class Organization(Resource):
    def __init__(self, name, id_seed, owner, region="us-central1"):
        super().__init__(name, "org-" + id_seed)
        self.owner = owner
        self.region = region
        self.members = [] # list of (Account, role)
        self.secrets = []
        self.firewall_rules = []

class Project(Resource):
    def __init__(self, name, org, id_seed, owner, region="us-central1"):
        super().__init__(name, "proj-" + id_seed)
        self.org = org
        self.owner = owner
        self.region = region
        self.members = [] # list of (Account, role)
        self.secrets = []
        self.firewall_rules = []

class Site(Resource):
    def __init__(self, name, project, id_seed, owner):
        super().__init__(name, "site-" + id_seed)
        self.project = project
        self.owner = owner
        self.members = [] # list of (Account, role)
        self.secrets = []
        self.firewall_rules = []

class APIKey:
    def __init__(self, name, account, secret_value, id_seed, scopes=None, description=""):
        self.name = name
        self.account = account
        self.secret_value = secret_value
        self.uuid = get_uuid("apikey-" + id_seed)
        self.scopes = scopes if scopes else []
        self.description = description

# =============================================================================
# DATA POPULATION (The "Seinfeld" Dataset)
# =============================================================================

# Accounts
accounts = [
    Account("System Administrator", "admin@libops.io", "admin"),
    Account("Art Vandelay", "art.vandelay@vandelay.com", "art"),
    Account("Jerry Seinfeld", "jerry.seinfeld@vandelay.com", "jerry"),
    Account("Elaine Benes", "elaine.benes@vandelay.com", "elaine"),
    Account("George Costanza", "george.costanza@vandelay.com", "george"),
    Account("Cosmo Kramer", "cosmo.kramer@vandelay.com", "kramer"),
    Account("H.E. Pennypacker", "h.e.pennypacker@pennypacker.com", "pennypacker"),
    Account("Newman", "newman@pennypacker.com", "newman"),
    Account("Bob Sacamano", "bob.sacamano@vandelay.com", "bob"),
    Account("Joe Davola", "joe.davola@vandelay.com", "joe"),
    Account("Soup Nazi", "soup.nazi@vandelay.com", "soup"),
    Account("Babu Bhatt", "babu.bhatt@vandelay.com", "babu"),
    Account("Jackie Chiles", "jackie.chiles@pennypacker.com", "jackie"),
    Account("J. Peterman", "j.peterman@pennypacker.com", "peterman"),
    Account("David Puddy", "david.puddy@vandelay.com", "puddy"),
    Account("Uncle Leo", "uncle.leo@vandelay.com", "leo"),
    Account("No Access User", "noaccess@test.com", "noaccess"),
]

# Lookup helper
acc_map = {a.raw_seed: a for a in accounts}

# SSH Key for Admin
acc_map["admin"].ssh_keys.append({
    "uuid": get_uuid("ssh-admin-1"),
    "key": "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQC7xKqvqL8YqF9zHjZ8sK7YxJ5wL8qN2vR9sT3uV4wX5yZ6aB7cD8eF9gH0iJ1kL2mN3oP4qR5sT6uV7wX8yZ9aB0cD1eF2gH3iJ4kL5mN6oP7qR8sT9uV0wX1yZ2aB3cD4eF5gH6iJ7kL8mN9oP0qR1sT2uV3wX4yZ5aB6cD7eF8gH9iJ0kL1mN2oP3qR4sT5uV6wX7yZ8aB9cD0eF1gH2iJ3kL4mN5oP6qR7sT8uV9wX0yZ1aB2cD3eF4gH5iJ6kL7mN8oP9qR0sT1uV2wX3yZ4aB5cD6eF7gH8iJ9kL0mN1oP2qR3sT4uV5wX6yZ7aB8cD9eF0gH1iJ2kL3mN4oP5qR6sT7uV8wX9yZ0aB1cD2eF3gH4iJ5kL6mN7oP8qR9sT0uV1wX2yZ3aB4cD5eF6gH7iJ8kL9mN0oP1qR2sT3uV4wX5yZ6aB7cD8eF9gH0iJ1kL2mN3oP4qR5sT6uV7wX8yZ9aB0cD1eF2gH3iJ4kL5mN6oP7qR8sT9uV0wX1yZ2aB3cD4eF5gH6iJ7kL8== admin@libops.io",
    "name": "Admin LibOps Workstation",
    "fingerprint": "SHA256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
})

# API Keys (Full Access)
for acc in accounts:
    key_name = f"{acc.name} Full"
    secret_val = f"libops_{acc.raw_seed}_full"
    # Special case for "No Access User" -> libops_no_access
    if acc.raw_seed == "noaccess": secret_val = "libops_no_access"
    
    acc.api_keys.append(APIKey(key_name, acc, secret_val, f"{acc.raw_seed}-full", [], "Full access"))

# Limited Scope Keys
limited_keys = [
    # Admin (Read Org)
    ("admin", "Admin Limited", "libops_admin_limited", "admin-limited", ["read:organization"]),
    # Art (Read Project)
    ("art", "Art Limited", "libops_art_limited", "art-limited", ["read:project"]),
    # Bob (Delete Project)
    ("bob", "Bob Limited", "libops_bob_limited", "bob-limited", ["delete:project"]),
    # Soup Nazi (Read Site)
    ("soup", "Soup Nazi Limited", "libops_soup_limited", "soup-limited", ["read:site"]),
]

for seed, name, secret, id_suffix, scopes in limited_keys:
    acc_map[seed].api_keys.append(APIKey(name, acc_map[seed], secret, id_suffix, scopes, "Limited scope"))


# Organizations
orgs = [
    Organization("LibOps Platform", "libops", acc_map["admin"]),
    Organization("Vandelay Industries", "vandelay", acc_map["art"], region="us-east1"),
    Organization("Pennypacker LLC", "pennypacker", acc_map["pennypacker"], region="us-west1"),
]
org_map = {o.id_seed: o for o in orgs}

# Org Memberships
org_map["org-libops"].members.append((acc_map["admin"], "owner"))

org_map["org-vandelay"].members.append((acc_map["art"], "owner"))
org_map["org-vandelay"].members.append((acc_map["jerry"], "developer"))
org_map["org-vandelay"].members.append((acc_map["elaine"], "developer"))
org_map["org-vandelay"].members.append((acc_map["george"], "developer"))
org_map["org-vandelay"].members.append((acc_map["kramer"], "read"))

org_map["org-pennypacker"].members.append((acc_map["pennypacker"], "owner"))
org_map["org-pennypacker"].members.append((acc_map["newman"], "developer"))

# Org Secrets
org_map["org-libops"].secrets.append(("LIBOPS_MASTER_KEY", get_uuid("sec-org-libops-1")))
org_map["org-libops"].secrets.append(("LIBOPS_API_TOKEN", get_uuid("sec-org-libops-2")))
org_map["org-vandelay"].secrets.append(("VANDELAY_IMPORT_KEY", get_uuid("sec-org-vandelay-1")))
org_map["org-vandelay"].secrets.append(("VANDELAY_EXPORT_KEY", get_uuid("sec-org-vandelay-2")))

# Org Firewall
org_map["org-libops"].firewall_rules.append(("LibOps HQ Office", "10.0.0.0/8", get_uuid("fw-org-libops-1")))
org_map["org-libops"].firewall_rules.append(("LibOps VPN", "172.16.0.0/12", get_uuid("fw-org-libops-2")))
org_map["org-vandelay"].firewall_rules.append(("Vandelay Office NYC", "192.168.1.0/24", get_uuid("fw-org-vandelay-1")))


# Projects
projects = [
    Project("LibOps Core Platform", org_map["org-libops"], "libops-core", acc_map["admin"]),
    Project("Project Jupiter", org_map["org-vandelay"], "jupiter", acc_map["bob"], region="us-east1"),
    Project("Project Latex", org_map["org-pennypacker"], "latex", acc_map["jackie"], region="us-west1"),
]
proj_map = {p.id_seed: p for p in projects}

# Project Members
proj_map["proj-jupiter"].members.append((acc_map["bob"], "owner"))
proj_map["proj-jupiter"].members.append((acc_map["joe"], "developer"))
proj_map["proj-jupiter"].members.append((acc_map["puddy"], "read"))

proj_map["proj-latex"].members.append((acc_map["jackie"], "owner"))
proj_map["proj-latex"].members.append((acc_map["peterman"], "developer"))

# Project Secrets
proj_map["proj-jupiter"].secrets.append(("JUPITER_DB_PASSWORD", get_uuid("sec-proj-jupiter-1")))
proj_map["proj-jupiter"].secrets.append(("JUPITER_API_SECRET", get_uuid("sec-proj-jupiter-2")))

# Project Firewall
proj_map["proj-jupiter"].firewall_rules.append(("Jupiter Dev Team", "192.168.10.0/24", get_uuid("fw-proj-jupiter-1")))


# Sites
sites = [
    Site("production", proj_map["proj-jupiter"], "jupiter-prod", acc_map["soup"]),
    Site("staging", proj_map["proj-jupiter"], "jupiter-staging", acc_map["soup"]),
    Site("production", proj_map["proj-latex"], "latex-prod", acc_map["jackie"]),
]
site_map = {s.id_seed: s for s in sites}

# Site Members
site_map["site-jupiter-prod"].members.append((acc_map["soup"], "owner"))
site_map["site-jupiter-prod"].members.append((acc_map["babu"], "developer"))
site_map["site-jupiter-prod"].members.append((acc_map["leo"], "read"))

# Site Secrets
site_map["site-jupiter-prod"].secrets.append(("PROD_SESSION_KEY", get_uuid("sec-site-jupiter-1")))
site_map["site-jupiter-prod"].secrets.append(("PROD_ENCRYPTION_KEY", get_uuid("sec-site-jupiter-2")))

# Site Firewall
site_map["site-jupiter-prod"].firewall_rules.append(("Production CDN", "192.168.100.0/24", get_uuid("fw-site-jupiter-1")))


# =============================================================================
# GENERATORS
# =============================================================================

def generate_sql():
    lines = []
    lines.append("-- AUTO-GENERATED SEED DATA")
    lines.append("-- Generated by ci/testdata/generate_seed.py")
    lines.append("")
    
    # Assign integer IDs
    acc_id_counter = 1
    for acc in accounts:
        acc.id = acc_id_counter
        acc_id_counter += 1
        lines.append(f"-- Account: {acc.name}")
        lines.append("INSERT INTO accounts (id, public_id, email, name, auth_method, verified, vault_entity_id, created_at)")
        lines.append(f"VALUES ({acc.id}, UNHEX(REPLACE('{acc.uuid}', '-', '')), '{acc.email}', '{acc.name}', 'userpass', TRUE, 'entity-{acc.email}', NOW());")
        lines.append("")
        
        # API Keys for Account
        for key in acc.api_keys:
            scope_json = json.dumps(key.scopes)
            lines.append(f"INSERT INTO api_keys (public_id, account_id, name, description, scopes, active, created_by, created_at)")
            lines.append(f"VALUES (UNHEX(REPLACE('{key.uuid}', '-', '')), {acc.id}, '{key.name}', '{key.description}', '{scope_json}', TRUE, {acc.id}, NOW());")
        
        # SSH Keys
        for ssh in acc.ssh_keys:
             lines.append(f"INSERT INTO ssh_keys (public_id, account_id, public_key, name, fingerprint, created_at)")
             lines.append(f"VALUES (UNHEX(REPLACE('{ssh['uuid']}', '-', '')), {acc.id}, '{ssh['key']}', '{ssh['name']}', '{ssh['fingerprint']}', NOW());")
        
        lines.append("")

    org_id_counter = 1
    for org in orgs:
        org.id = org_id_counter
        org_id_counter += 1
        
        lines.append(f"-- Organization: {org.name}")
        lines.append("INSERT INTO organizations (id, public_id, name, gcp_org_id, gcp_billing_account, gcp_parent, location, region, gcp_folder_id, status, gcp_project_id, gcp_project_number, created_by, created_at)")
        lines.append(f"VALUES ({org.id}, UNHEX(REPLACE('{org.uuid}', '-', '')), '{org.name}', '1{org.id}000', 'BILL-{org.id}', 'organizations/1{org.id}000', 'us', '{org.region}', 'folders/2{org.id}000', 'active', 'org-{org.id}-proj', '3{org.id}000', {org.owner.id}, NOW());")
        lines.append("")
        
        # Relationships (hardcoded for now to link all to LibOps if not LibOps)
        if org.id_seed != "org-libops":
            rel_uuid = get_uuid(f"rel-libops-{org.id_seed}")
            lines.append(f"INSERT INTO relationships (id, public_id, source_organization_id, target_organization_id, relationship_type, status)")
            lines.append(f"VALUES ({org.id}, UNHEX(REPLACE('{rel_uuid}', '-', '')), 1, {org.id}, 'access', 'approved');")
            lines.append("")
            
        for member, role in org.members:
            mem_uuid = get_uuid(f"orgmem-{org.id_seed}-{member.raw_seed}")
            lines.append(f"INSERT INTO organization_members (public_id, organization_id, account_id, role, status, created_by, created_at)")
            lines.append(f"VALUES (UNHEX(REPLACE('{mem_uuid}', '-', '')), {org.id}, {member.id}, '{role}', 'active', {org.owner.id}, NOW());")
        
        for name, uuid in org.secrets:
             lines.append(f"INSERT INTO organization_secrets (public_id, organization_id, name, vault_path, status, created_at, updated_at, created_by)")
             lines.append(f"VALUES (UNHEX(REPLACE('{uuid}', '-', '')), {org.id}, '{name}', 'secret-organization/{org.id}/{name}', 'active', UNIX_TIMESTAMP(), UNIX_TIMESTAMP(), {org.owner.id});")
             
        for name, cidr, uuid in org.firewall_rules:
             lines.append(f"INSERT INTO organization_firewall_rules (public_id, organization_id, name, cidr, rule_type, status, created_at, updated_at, created_by)")
             lines.append(f"VALUES (UNHEX(REPLACE('{uuid}', '-', '')), {org.id}, '{name}', '{cidr}', 'https_allowed', 'active', NOW(), NOW(), {org.owner.id});")
        lines.append("")
        
    proj_id_counter = 1
    for proj in projects:
        proj.id = proj_id_counter
        proj_id_counter += 1
        
        lines.append(f"-- Project: {proj.name}")
        lines.append("INSERT INTO projects (id, public_id, organization_id, name, github_repository, github_branch, gcp_region, gcp_zone, machine_type, gcp_project_id, gcp_project_number, status, organization_project, created_by, created_at)")
        lines.append(f"VALUES ({proj.id}, UNHEX(REPLACE('{proj.uuid}', '-', '')), {proj.org.id}, '{proj.name}', 'repo/{proj.id_seed}', 'main', '{proj.region}', '{proj.region}-b', 'e2-medium', 'proj-{proj.id}-gcp', '4{proj.id}000', 'active', TRUE, {proj.owner.id}, NOW());")
        lines.append("")

        for member, role in proj.members:
            mem_uuid = get_uuid(f"projmem-{proj.id_seed}-{member.raw_seed}")
            lines.append(f"INSERT INTO project_members (public_id, project_id, account_id, role, status, created_by, created_at)")
            lines.append(f"VALUES (UNHEX(REPLACE('{mem_uuid}', '-', '')), {proj.id}, {member.id}, '{role}', 'active', {proj.owner.id}, NOW());")
            
        for name, uuid in proj.secrets:
             lines.append(f"INSERT INTO project_secrets (public_id, project_id, name, vault_path, status, created_at, updated_at, created_by)")
             lines.append(f"VALUES (UNHEX(REPLACE('{uuid}', '-', '')), {proj.id}, '{name}', 'secret-project/{proj.id}/{name}', 'active', UNIX_TIMESTAMP(), UNIX_TIMESTAMP(), {proj.owner.id});")

        for name, cidr, uuid in proj.firewall_rules:
             lines.append(f"INSERT INTO project_firewall_rules (public_id, project_id, name, cidr, rule_type, status, created_at, updated_at, created_by)")
             lines.append(f"VALUES (UNHEX(REPLACE('{uuid}', '-', '')), {proj.id}, '{name}', '{cidr}', 'https_allowed', 'active', NOW(), NOW(), {proj.owner.id});")
        lines.append("")

    site_id_counter = 1
    for site in sites:
        site.id = site_id_counter
        site_id_counter += 1
        
        lines.append(f"-- Site: {site.name}")
        lines.append("INSERT INTO sites (id, public_id, project_id, name, github_ref, gcp_external_ip, status, created_by, created_at)")
        lines.append(f"VALUES ({site.id}, UNHEX(REPLACE('{site.uuid}', '-', '')), {site.project.id}, '{site.name}', 'tags/v1.0.0', '1.2.3.{site.id}', 'active', {site.owner.id}, NOW());")
        lines.append("")

        for member, role in site.members:
            mem_uuid = get_uuid(f"sitemem-{site.id_seed}-{member.raw_seed}")
            lines.append(f"INSERT INTO site_members (public_id, site_id, account_id, role, status, created_by, created_at)")
            lines.append(f"VALUES (UNHEX(REPLACE('{mem_uuid}', '-', '')), {site.id}, {member.id}, '{role}', 'active', {site.owner.id}, NOW());")

        for name, uuid in site.secrets:
             lines.append(f"INSERT INTO site_secrets (public_id, site_id, name, vault_path, status, created_at, updated_at, created_by)")
             lines.append(f"VALUES (UNHEX(REPLACE('{uuid}', '-', '')), {site.id}, '{name}', 'secret-site/{site.id}/{name}', 'active', UNIX_TIMESTAMP(), UNIX_TIMESTAMP(), {site.owner.id});")
             
        for name, cidr, uuid in site.firewall_rules:
             lines.append(f"INSERT INTO site_firewall_rules (public_id, site_id, name, cidr, rule_type, status, created_at, updated_at, created_by)")
             lines.append(f"VALUES (UNHEX(REPLACE('{uuid}', '-', '')), {site.id}, '{name}', '{cidr}', 'https_allowed', 'active', NOW(), NOW(), {site.owner.id});")

        lines.append("")

    return "\n".join(lines)

def generate_vault_init():
    lines = []
    lines.append("#!/bin/sh")
    lines.append("set -e")
    lines.append('echo "Initializing Vault with AUTO-GENERATED data..."')
    lines.append("")
    
    # Common setup
    lines.append("""
# Helper to enable secrets engine if not already enabled
enable_secrets() {
    path=$1
    if vault secrets list | grep -q \"^$path/\" ; then
        echo "Secrets engine at $path/ already enabled"
    else
        vault secrets enable -path=$path -version=1 kv
        echo "Enabled $path KV engine"
    fi
}

# Enable KV v2 secrets engine for organization secrets
enable_secrets \"secret-organization\"
enable_secrets \"secret-global\"
enable_secrets \"secret-project\"
enable_secrets \"secret-site\"

# Enable KV v1 secrets engine for API keys (application expects v1 at 'keys/')
if vault secrets list | grep -q \"^keys/\" ; then
    echo "Secrets engine at keys/ already enabled"
else
    vault secrets enable -path=keys -version=1 kv
    echo "Enabled keys KV v1 engine"
fi

# Enable userpass auth method
if vault auth list | grep -q \"^userpass/\" ; then
    echo "Auth method userpass/ already enabled"
else
    vault auth enable userpass
    echo "Enabled userpass auth method"
fi

# Configure OIDC Provider
echo "Configuring OIDC Provider..."
vault write identity/oidc/key/libops-api allowed_client_ids='*' verification_ttl='2h' rotation_period='24h' algorithm='RS256'
vault write identity/oidc/client/libops-api redirect_uris='http://api:8080/auth/callback' key='libops-api' id_token_ttl='30m' access_token_ttl='1h'
vault write identity/oidc/provider/libops-api allowed_client_ids='*' scopes='openid,email,profile' issuer_host='http://vault:8200'
vault write identity/oidc/role/libops-api key='libops-api' template='{\"account_id\": {{identity.entity.metadata.account_id}},\"email\": {{identity.entity.metadata.email}},\"name\": {{identity.entity.name}}}' ttl='1h'

# Create libops-user policy
vault policy write libops-user - <<EOF
path "identity/oidc/token/libops-api" {
  capabilities = ["read", "update"]
}
EOF

create_test_user() {
    email=$1
    password=$2
    account_id=$3
    entity_name=$4
    
    vault_username=$(echo "$email" | tr '@' '_')
    vault write "auth/userpass/users/$vault_username" password="$password" policies="libops-user"
    vault write identity/entity name="$entity_name" metadata="email=$email" metadata="account_id=$account_id"
    entity_id=$(vault read -field=id identity/entity/name/$entity_name)
    accessor=$(vault auth list | grep "^userpass/" | awk '{print $3}')
    vault write identity/entity-alias name="$vault_username" canonical_id="$entity_id" mount_accessor=$accessor
    echo "Created user: $vault_username ($entity_id)"
}
""")
    
    lines.append("echo 'Creating users...'")
    for acc in accounts:
        acc_id = accounts.index(acc) + 1
        lines.append(f'create_test_user "{acc.email}" "{acc.password}" "{acc_id}" "entity-{acc.email}"')
        
    lines.append("")
    lines.append("echo 'Creating API keys...'")
    for acc in accounts:
        for key in acc.api_keys:
            lines.append(f"# {key.name}")
            lines.append(f"vault write keys/{key.secret_value} account_uuid=\"{acc.uuid}\" api_key_uuid=\"{key.uuid}\"")
            lines.append(f"vault write keys/uuid/{key.uuid} secret_value=\"{key.secret_value}\"")
    
    lines.append("")
    lines.append("echo 'Vault initialization complete!'")
    
    return "\n".join(lines)


# =============================================================================
# MAIN
# =============================================================================

def main():
    sql_content = generate_sql()
    vault_content = generate_vault_init()
    
    with open('ci/testdata/rbac_seed.sql', 'w') as f:
        f.write(sql_content)
    print("Updated ci/testdata/rbac_seed.sql")
        
    with open('ci/testdata/vault-init.sh', 'w') as f:
        f.write(vault_content)
    os.chmod('ci/testdata/vault-init.sh', 0o755)
    print("Updated ci/testdata/vault-init.sh")

    # Print IDs for updating main.go
    print("\n--- IDs for main.go ---")
    print(f'rootOrgID   = "{org_map["org-libops"].uuid}"')
    print(f'childOrgID  = "{org_map["org-vandelay"].uuid}"')
    print(f'project1ID  = "{proj_map["proj-jupiter"].uuid}"')
    print(f'project2ID  = "{proj_map["proj-latex"].uuid}"')
    print(f'site1ProdID = "{site_map["site-jupiter-prod"].uuid}"')
    print(f'site1StagID = "{site_map["site-jupiter-staging"].uuid}"')
    print(f'site2ProdID = "{site_map["site-latex-prod"].uuid}"')

    print("\n--- Account IDs ---")
    print(f'adminAccountID  = "{acc_map["admin"].uuid}"')
    print(f'artAccountID    = "{acc_map["art"].uuid}"')
    print(f'kramerAccountID = "{acc_map["kramer"].uuid}"')
    print(f'soupAccountID   = "{acc_map["soup"].uuid}"')
    print(f'babuAccountID   = "{acc_map["babu"].uuid}"')

if __name__ == "__main__":
    main()