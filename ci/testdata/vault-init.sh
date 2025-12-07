#!/bin/sh

set -e

echo "Initializing Vault for RBAC integration tests..."

# Helper to enable secrets engine if not already enabled
enable_secrets() {
    path=$1
    if vault secrets list | grep -q "^$path/"; then
        echo "Secrets engine at $path/ already enabled"
    else
        vault secrets enable -path=$path -version=1 kv
        echo "Enabled $path KV engine"
    fi
}

# Enable KV v2 secrets engine for organization secrets
enable_secrets "secret-organization"

# Enable KV v2 secrets engine for global secrets (used by organization secret service)
enable_secrets "secret-global"

# Enable KV v2 secrets engine for project secrets
enable_secrets "secret-project"

# Enable KV v2 secrets engine for site secrets
enable_secrets "secret-site"

# Enable KV v1 secrets engine for API keys (application expects v1 at 'keys/')
if vault secrets list | grep -q "^keys/"; then
    echo "Secrets engine at keys/ already enabled"
else
    vault secrets enable -path=keys -version=1 kv
    echo "Enabled keys KV v1 engine"
fi

# Enable KV v1 secrets engine for reverse API key lookups
if vault secrets list | grep -q "^keys-by-uuid/"; then
    echo "Secrets engine at keys-by-uuid/ already enabled"
else
    vault secrets enable -path=keys-by-uuid -version=1 kv
    echo "Enabled keys-by-uuid KV v1 engine"
fi

# Enable userpass auth method
if vault auth list | grep -q "^userpass/"; then
    echo "Auth method userpass/ already enabled"
else
    vault auth enable userpass
    echo "Enabled userpass auth method"
fi


# Configure OIDC Provider for API startup checks
# The API server attempts to fetch the discovery document from Vault on startup.
# Even if tests don't use OIDC login, the provider must exist to prevent startup failure.

echo "Configuring OIDC Provider..."

# Create a key for the OIDC provider
# Use 'libops-api' key to ensure visibility in JWKS
vault write identity/oidc/key/libops-api \
  allowed_client_ids="*" \
  verification_ttl="2h" \
  rotation_period="24h" \
  algorithm="RS256"

vault write identity/oidc/client/api \
    redirect_uris="http://api:8080/auth/callback" \
    key="libops-api" \
    id_token_ttl="30m" \
    access_token_ttl="1h"

# Create the OIDC provider
# This enables the .well-known/openid-configuration endpoint
vault write identity/oidc/provider/libops-api \
  allowed_client_ids="*" \
  scopes="openid,email,profile" \
  issuer_host="http://vault:8200"
echo "Created OIDC provider 'libops-api'"

# Create an OIDC role for token generation
# This binds the key and template for direct token requests
vault write identity/oidc/role/libops-api \
  key="libops-api" \
  template="{\"account_id\": {{identity.entity.metadata.account_id}},\"email\": {{identity.entity.metadata.email}},\"name\": {{identity.entity.name}}}" \
  ttl="1h"
echo "Created OIDC role 'libops-api'"

echo "Vault initialization complete!"
echo "Created 18 API keys (14 full-scope + 4 limited-scope)"


# Create libops-user policy to allow OIDC token generation
vault policy write libops-user - <<EOF
path "identity/oidc/token/libops-api" {
  capabilities = ["read", "update"]
}
EOF
echo "Created libops-user policy"

# Function to create userpass user, entity, and alias
create_test_user() {
    email=$1
    password=$2
    account_id=$3
    # entity_id argument is ignored as we let Vault generate it to ensure compatibility

    # Replace @ with _ for the Vault username
    vault_username=$(echo "$email" | tr '@' '_')

    # Create userpass user with libops-user policy
    vault write "auth/userpass/users/$vault_username" password="$password" policies="libops-user"

    # Create entity by name (Vault assigns ID)
    # capture the output ID
    vault write identity/entity name="entity-$email" metadata="email=$email" metadata="account_id=$account_id"

    # Read the entity ID back using the name
    entity_id=$(vault read -field=id identity/entity/name/entity-$email)

    # Get accessor
    accessor=$(vault auth list | grep "^userpass/" | awk '{print $3}')

    # Create alias to link the userpass login to the entity
    vault write identity/entity-alias name="$vault_username" canonical_id="$entity_id" mount_accessor=$accessor

    echo "Created user: $vault_username ($entity_id) for email $email (account_id: $account_id)"
}

echo "Creating userpass users and entities..."
create_test_user "admin@root.com" "password123" "1"
create_test_user "org-owner@child.com" "password123" "2"
create_test_user "org-developer@child.com" "password123" "3"
create_test_user "org-read@child.com" "password123" "4"
create_test_user "proj1-owner@child.com" "password123" "5"
create_test_user "proj1-developer@child.com" "password123" "6"
create_test_user "proj1-read@child.com" "password123" "7"
create_test_user "site1-owner@child.com" "password123" "8"
create_test_user "site1-developer@child.com" "password123" "9"
create_test_user "site1-read@child.com" "password123" "10"
create_test_user "proj2-owner@child.com" "password123" "11"
create_test_user "proj2-developer@child.com" "password123" "12"
create_test_user "proj2-read@child.com" "password123" "13"
create_test_user "noaccess@test.com" "password123" "14"

# Create test API keys matching rbac_seed.sql data
# The application expects the secret value (token) to start with 'libops_'

echo "Creating full-scope API keys..."

# Joe - Root org owner (Account 1)
vault write keys/libops_admin_full \
  account_uuid="10000000-0000-0000-0000-000000000001" \
  api_key_uuid="20000000-0000-0000-0000-000000000001"
vault write keys-by-uuid/20000000-0000-0000-0000-000000000001 \
  secret_value="libops_admin_full"
echo "Created Joe (root owner) full API key"

# Org Owner - Child org owner (Account 2)
vault write keys/libops_org_owner_full \
  account_uuid="10000000-0000-0000-0000-000000000002" \
  api_key_uuid="20000000-0000-0000-0000-000000000002"
vault write keys-by-uuid/20000000-0000-0000-0000-000000000002 \
  secret_value="libops_org_owner_full"
echo "Created org-owner full API key"

# Org Developer (Account 3)
vault write keys/libops_org_developer_full \
  account_uuid="10000000-0000-0000-0000-000000000003" \
  api_key_uuid="20000000-0000-0000-0000-000000000003"
vault write keys-by-uuid/20000000-0000-0000-0000-000000000003 \
  secret_value="libops_org_developer_full"
echo "Created org-developer full API key"

# Org Read (Account 4)
vault write keys/libops_org_read_full \
  account_uuid="10000000-0000-0000-0000-000000000004" \
  api_key_uuid="20000000-0000-0000-0000-000000000004"
vault write keys-by-uuid/20000000-0000-0000-0000-000000000004 \
  secret_value="libops_org_read_full"
echo "Created org-read full API key"

# Project 1 Owner (Account 5)
vault write keys/libops_proj1_owner_full \
  account_uuid="10000000-0000-0000-0000-000000000005" \
  api_key_uuid="20000000-0000-0000-0000-000000000005"
vault write keys-by-uuid/20000000-0000-0000-0000-000000000005 \
  secret_value="libops_proj1_owner_full"
echo "Created proj1-owner full API key"

# Project 1 Developer (Account 6)
vault write keys/libops_proj1_developer_full \
  account_uuid="10000000-0000-0000-0000-000000000006" \
  api_key_uuid="20000000-0000-0000-0000-000000000006"
vault write keys-by-uuid/20000000-0000-0000-0000-000000000006 \
  secret_value="libops_proj1_developer_full"
echo "Created proj1-developer full API key"

# Project 1 Read (Account 7)
vault write keys/libops_proj1_read_full \
  account_uuid="10000000-0000-0000-0000-000000000007" \
  api_key_uuid="20000000-0000-0000-0000-000000000007"
vault write keys-by-uuid/20000000-0000-0000-0000-000000000007 \
  secret_value="libops_proj1_read_full"
echo "Created proj1-read full API key"

# Site 1 Owner (Account 8)
vault write keys/libops_site1_owner_full \
  account_uuid="10000000-0000-0000-0000-000000000008" \
  api_key_uuid="20000000-0000-0000-0000-000000000008"
vault write keys-by-uuid/20000000-0000-0000-0000-000000000008 \
  secret_value="libops_site1_owner_full"
echo "Created site1-owner full API key"

# Site 1 Developer (Account 9)
vault write keys/libops_site1_developer_full \
  account_uuid="10000000-0000-0000-0000-000000000009" \
  api_key_uuid="20000000-0000-0000-0000-000000000009"
vault write keys-by-uuid/20000000-0000-0000-0000-000000000009 \
  secret_value="libops_site1_developer_full"
echo "Created site1-developer full API key"

# Site 1 Read (Account 10)
vault write keys/libops_site1_read_full \
  account_uuid="10000000-0000-0000-0000-000000000010" \
  api_key_uuid="20000000-0000-0000-0000-000000000010"
vault write keys-by-uuid/20000000-0000-0000-0000-000000000010 \
  secret_value="libops_site1_read_full"
echo "Created site1-read full API key"

# Project 2 Owner (Account 11)
vault write keys/libops_proj2_owner_full \
  account_uuid="10000000-0000-0000-0000-000000000011" \
  api_key_uuid="20000000-0000-0000-0000-000000000011"
vault write keys-by-uuid/20000000-0000-0000-0000-000000000011 \
  secret_value="libops_proj2_owner_full"
echo "Created proj2-owner full API key"

# Project 2 Developer (Account 12)
vault write keys/libops_proj2_developer_full \
  account_uuid="10000000-0000-0000-0000-000000000012" \
  api_key_uuid="20000000-0000-0000-0000-000000000012"
vault write keys-by-uuid/20000000-0000-0000-0000-000000000012 \
  secret_value="libops_proj2_developer_full"
echo "Created proj2-developer full API key"

# Project 2 Read (Account 13)
vault write keys/libops_proj2_read_full \
  account_uuid="10000000-0000-0000-0000-000000000013" \
  api_key_uuid="20000000-0000-0000-0000-000000000013"
vault write keys-by-uuid/20000000-0000-0000-0000-000000000013 \
  secret_value="libops_proj2_read_full"
echo "Created proj2-read full API key"

# No Access User (Account 14)
vault write keys/libops_no_access \
  account_uuid="10000000-0000-0000-0000-000000000014" \
  api_key_uuid="20000000-0000-0000-0000-000000000014"
vault write keys-by-uuid/20000000-0000-0000-0000-000000000014 \
  secret_value="libops_no_access"
echo "Created no-access API key"

echo "Creating limited-scope API keys..."

# Joe - Limited scope (read-only despite being owner)
vault write keys/libops_admin_limited \
  account_uuid="10000000-0000-0000-0000-000000000001" \
  api_key_uuid="30000000-0000-0000-0000-000000000001"
vault write keys-by-uuid/30000000-0000-0000-0000-000000000001 \
  secret_value="libops_admin_limited"
echo "Created Joe limited (read-only) API key"

# Org Owner - Limited to project scope
vault write keys/libops_org_owner_limited \
  account_uuid="10000000-0000-0000-0000-000000000002" \
  api_key_uuid="30000000-0000-0000-0000-000000000002"
vault write keys-by-uuid/30000000-0000-0000-0000-000000000002 \
  secret_value="libops_org_owner_limited"
echo "Created org-owner limited (project-only) API key"

# Project 1 Owner - Limited to read-only
vault write keys/libops_proj1_owner_limited \
  account_uuid="10000000-0000-0000-0000-000000000005" \
  api_key_uuid="30000000-0000-0000-0000-000000000005"
vault write keys-by-uuid/30000000-0000-0000-0000-000000000005 \
  secret_value="libops_proj1_owner_limited"
echo "Created proj1-owner limited (read-only) API key"

# Site 1 Owner - Limited to read-only
vault write keys/libops_site1_owner_limited \
  account_uuid="10000000-0000-0000-0000-000000000008" \
  api_key_uuid="30000000-0000-0000-0000-000000000008"
vault write keys-by-uuid/30000000-0000-0000-0000-000000000008 \
  secret_value="libops_site1_owner_limited"
echo "Created site1-owner limited (read-only) API key"
