#!/bin/sh
set -e
echo "Initializing Vault with AUTO-GENERATED data..."


# Helper to enable secrets engine if not already enabled
enable_secrets() {
    path=$1
    if vault secrets list | grep -q "^$path/" ; then
        echo "Secrets engine at $path/ already enabled"
    else
        vault secrets enable -path=$path -version=1 kv
        echo "Enabled $path KV engine"
    fi
}

enable_secrets "secret-organization"
enable_secrets "secret-project"
enable_secrets "secret-site"

# Enable KV v1 secrets engine for API keys (application expects v1 at 'keys/')
if vault secrets list | grep -q "^keys/" ; then
    echo "Secrets engine at keys/ already enabled"
else
    vault secrets enable -path=keys -version=1 kv
    echo "Enabled keys KV v1 engine"
fi

# Enable userpass auth method
if vault auth list | grep -q "^userpass/" ; then
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
vault write identity/oidc/role/libops-api key='libops-api' template='{"account_id": {{identity.entity.metadata.account_id}},"email": {{identity.entity.metadata.email}},"name": {{identity.entity.name}}}' ttl='1h'

# Create a token role that allows the API to create entity tokens with specific policies
vault write auth/token/roles/entity-token \
    allowed_policies="default,libops-user" \
    allowed_entity_aliases="*" \
    orphan=true \
    renewable=true \
    token_type="service"

# Create libops-user policy
vault policy write libops-user - <<EOF
path "identity/oidc/token/libops-api" {
  capabilities = ["read", "update"]
}
path "keys/{{identity.entity.metadata.account_uuid}}/*" {
  capabilities = ["create", "update", "read", "delete", "list"]
}
path "secret-organization/*" {
  capabilities = ["create", "update", "read", "delete", "list"]
}
path "secret-project/*" {
  capabilities = ["create", "update", "read", "delete", "list"]
}
path "secret-site/*" {
  capabilities = ["create", "update", "read", "delete", "list"]
}

EOF

vault policy write api - <<EOF
path "keys/*" {
  capabilities = ["create", "read", "update", "delete", "list"]
}

path "identity/oidc/client/libops-api" {
  capabilities = [ "read" ]
}

path "identity/entity" {
  capabilities = [ "create", "update" ]
}

path "identity/entity/id/*" {
  capabilities = [ "create", "read", "update", "delete" ]
}

path "identity/entity-alias" {
  capabilities = [ "create", "update" ]
}

path "identity/entity-alias/id/*" {
  capabilities = [ "read", "update", "delete", "list" ]
}

path "auth/userpass/users/*" {
  capabilities = [ "create", "read", "update", "delete", "list" ]
}

path "auth/token/create/entity-token" {
  capabilities = [ "create", "update"]
}

path "sys/auth" {
  capabilities = ["read", "list"]
}

path "secret/libops-api" {
  capabilities = ["read", "list"]
}

path "secret/libops-api/*" {
  capabilities = ["read"]
}

path "secret-organization/*" {
  capabilities = ["create", "update", "read", "delete", "list"]
}
path "secret-project/*" {
  capabilities = ["create", "update", "read", "delete", "list"]
}
path "secret-site/*" {
  capabilities = ["create", "update", "read", "delete", "list"]
}
EOF

create_test_user() {
    email=$1
    password=$2
    account_id=$3
    entity_name=$4
    account_uuid=$5

    # Convert UUID to lowercase no-dashes format for account_uuid metadata
    account_uuid_no_dashes=$(echo "$account_uuid" | tr -d '-' | tr '[:upper:]' '[:lower:]')

    vault_username=$(echo "$email" | tr '@' '_')
    vault write "auth/userpass/users/$vault_username" password="$password" policies="libops-user"
    vault write identity/entity name="$entity_name" metadata="email=$email" metadata="account_id=$account_id" metadata="account_uuid=$account_uuid_no_dashes"
    entity_id=$(vault read -field=id identity/entity/name/$entity_name)
    accessor=$(vault auth list | grep "^userpass/" | awk '{print $3}')
    vault write identity/entity-alias name="$vault_username" canonical_id="$entity_id" mount_accessor=$accessor
    echo "Created user: $vault_username ($entity_id) with account_uuid=$account_uuid_no_dashes"
}

echo 'Creating users...'
create_test_user "admin@libops.io" "password123" "1" "entity-admin@libops.io" "01052d4d-93be-51a3-9684-c357297533cd"
create_test_user "art.vandelay@vandelay.com" "password123" "2" "entity-art.vandelay@vandelay.com" "fdf35d32-bbb3-5ea3-abf2-410da575e169"
create_test_user "jerry.seinfeld@vandelay.com" "password123" "3" "entity-jerry.seinfeld@vandelay.com" "964b5eb0-2037-5263-883c-e939c6916d7d"
create_test_user "elaine.benes@vandelay.com" "password123" "4" "entity-elaine.benes@vandelay.com" "863fb60a-8084-50fe-82ae-efa113231bef"
create_test_user "george.costanza@vandelay.com" "password123" "5" "entity-george.costanza@vandelay.com" "d0bfd257-4572-5036-b5aa-038743be4715"
create_test_user "cosmo.kramer@vandelay.com" "password123" "6" "entity-cosmo.kramer@vandelay.com" "516e3bb4-bfbe-5dda-9cc9-d0e00ce7b6f2"
create_test_user "h.e.pennypacker@pennypacker.com" "password123" "7" "entity-h.e.pennypacker@pennypacker.com" "42b6846e-501f-5153-9aca-210d8d84f946"
create_test_user "newman@pennypacker.com" "password123" "8" "entity-newman@pennypacker.com" "e60f6db8-521a-5fc3-aacc-ceb3f50b6f7b"
create_test_user "bob.sacamano@vandelay.com" "password123" "9" "entity-bob.sacamano@vandelay.com" "94656683-e366-58b8-a391-32e0c54ca37e"
create_test_user "joe.davola@vandelay.com" "password123" "10" "entity-joe.davola@vandelay.com" "0f439d32-e065-5a20-a08e-22dd6793948a"
create_test_user "soup.nazi@vandelay.com" "password123" "11" "entity-soup.nazi@vandelay.com" "ff2098bd-1a33-5db9-8069-37f2bf5bdba7"
create_test_user "babu.bhatt@vandelay.com" "password123" "12" "entity-babu.bhatt@vandelay.com" "a551424b-91ed-5636-a53b-cdb50660d4c9"
create_test_user "jackie.chiles@pennypacker.com" "password123" "13" "entity-jackie.chiles@pennypacker.com" "af54b89e-5533-585a-b3b7-0003b7e6dcc2"
create_test_user "j.peterman@pennypacker.com" "password123" "14" "entity-j.peterman@pennypacker.com" "dfe2b1a8-8000-5b67-88ad-881b036fa4f9"
create_test_user "david.puddy@vandelay.com" "password123" "15" "entity-david.puddy@vandelay.com" "22f49023-8dfe-57c7-95db-dd0f8cae04a7"
create_test_user "uncle.leo@vandelay.com" "password123" "16" "entity-uncle.leo@vandelay.com" "351fcf8b-d637-596c-be1e-8bdd90dbc4eb"
create_test_user "noaccess@test.com" "password123" "17" "entity-noaccess@test.com" "e543554b-5af0-5d97-ac8f-09608bcfa7b8"

echo 'Creating API keys with format: libops_{accountUUID_no_dashes}_{keyUUID_no_dashes}_{randomSecret}...'
# Helper function to create API key in new format
create_api_key() {
  local account_uuid=$1
  local key_uuid=$2
  local random_secret=$3
  # Strip dashes and convert to lowercase for UUIDs
  local account_no_dashes=$(echo "$account_uuid" | tr -d '-' | tr '[:upper:]' '[:lower:]')
  local key_no_dashes=$(echo "$key_uuid" | tr -d '-' | tr '[:upper:]' '[:lower:]')
  # Format: libops_{accountUUID}_{keyUUID}_{randomSecret}
  local full_key="libops_${account_no_dashes}_${key_no_dashes}_${random_secret}"
  # Store in Vault at keys/{accountUUID}/{keyUUID} with the random secret as the value
  vault write keys/"${account_no_dashes}/${key_no_dashes}" secret="$random_secret"
  echo "$full_key"
}

# System Administrator Full
ADMIN_FULL=$(create_api_key "01052d4d-93be-51a3-9684-c357297533cd" "075913e7-9328-5264-b684-6ae0163b8096" "test_secret_admin_full")
# Admin Limited
ADMIN_LIMITED=$(create_api_key "01052d4d-93be-51a3-9684-c357297533cd" "d76a9ff9-334c-548d-8ba9-4063ddb96cf9" "test_secret_admin_limited")
# Art Vandelay Full
ART_FULL=$(create_api_key "fdf35d32-bbb3-5ea3-abf2-410da575e169" "0f05b4b9-f40c-5ca8-9f39-04de42ae87e4" "test_secret_art_full")
# Art Limited
ART_LIMITED=$(create_api_key "fdf35d32-bbb3-5ea3-abf2-410da575e169" "c1981101-4bbf-5f90-b38b-901c06fdaad6" "test_secret_art_limited")
# Jerry Seinfeld Full
JERRY_FULL=$(create_api_key "964b5eb0-2037-5263-883c-e939c6916d7d" "726186be-6ad8-5257-a1bd-2e4689db11d0" "test_secret_jerry_full")
# Elaine Benes Full
ELAINE_FULL=$(create_api_key "863fb60a-8084-50fe-82ae-efa113231bef" "b3f360ca-7995-5db2-b88b-3e178cd7ae8a" "test_secret_elaine_full")
# George Costanza Full
GEORGE_FULL=$(create_api_key "d0bfd257-4572-5036-b5aa-038743be4715" "0c9522b7-2197-5d87-b010-ac1bc506f79a" "test_secret_george_full")
# Cosmo Kramer Full
KRAMER_FULL=$(create_api_key "516e3bb4-bfbe-5dda-9cc9-d0e00ce7b6f2" "94581ae6-23e3-5869-8770-db7cb74e5391" "test_secret_kramer_full")
# H.E. Pennypacker Full
PENNYPACKER_FULL=$(create_api_key "42b6846e-501f-5153-9aca-210d8d84f946" "58c99883-c314-5c6e-bfa8-e072502e43bd" "test_secret_pennypacker_full")
# Newman Full
NEWMAN_FULL=$(create_api_key "e60f6db8-521a-5fc3-aacc-ceb3f50b6f7b" "3ccc3cc2-e5c0-530b-8f0a-6fb24cd8566b" "test_secret_newman_full")
# Bob Sacamano Full
BOB_FULL=$(create_api_key "94656683-e366-58b8-a391-32e0c54ca37e" "63cd920a-7090-5e0e-b46d-840a933e2c70" "test_secret_bob_full")
# Bob Limited
BOB_LIMITED=$(create_api_key "94656683-e366-58b8-a391-32e0c54ca37e" "7dd4d68f-85f4-5dbe-bed0-83e639a8fab2" "test_secret_bob_limited")
# Joe Davola Full
JOE_FULL=$(create_api_key "0f439d32-e065-5a20-a08e-22dd6793948a" "890e0976-5b43-5ff8-a673-921a920e7c2a" "test_secret_joe_full")
# Soup Nazi Full
SOUP_FULL=$(create_api_key "ff2098bd-1a33-5db9-8069-37f2bf5bdba7" "43527224-d0f8-5344-803f-ec80f80ed0a0" "test_secret_soup_full")
# Soup Nazi Limited
SOUP_LIMITED=$(create_api_key "ff2098bd-1a33-5db9-8069-37f2bf5bdba7" "b6b4b341-e1e5-5242-a33d-684e4da7ad07" "test_secret_soup_limited")
# Babu Bhatt Full
BABU_FULL=$(create_api_key "a551424b-91ed-5636-a53b-cdb50660d4c9" "2032b348-86ae-5805-b08c-3c2cf065ef82" "test_secret_babu_full")
# Jackie Chiles Full
JACKIE_FULL=$(create_api_key "af54b89e-5533-585a-b3b7-0003b7e6dcc2" "578e1fcf-b497-5bff-bbf4-436835457f73" "test_secret_jackie_full")
# J. Peterman Full
PETERMAN_FULL=$(create_api_key "dfe2b1a8-8000-5b67-88ad-881b036fa4f9" "2c3cfb5b-c994-54c9-9cb9-92321bd353cb" "test_secret_peterman_full")
# David Puddy Full
PUDDY_FULL=$(create_api_key "22f49023-8dfe-57c7-95db-dd0f8cae04a7" "eb181a1b-7dc9-53c2-9981-ba91a3ebf24a" "test_secret_puddy_full")
# Uncle Leo Full
LEO_FULL=$(create_api_key "351fcf8b-d637-596c-be1e-8bdd90dbc4eb" "ce22e781-d2ad-5d7a-bccc-7dd122e791c8" "test_secret_leo_full")
# No Access User Full
NO_ACCESS=$(create_api_key "e543554b-5af0-5d97-ac8f-09608bcfa7b8" "567df9dc-244e-561e-93c1-3082534eeec7" "test_secret_noaccess_full")

echo 'Vault initialization complete!'
