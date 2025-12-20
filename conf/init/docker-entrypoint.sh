#!/usr/bin/env bash

set -eou pipefail

vault_write_idempotent() {
    local path="$1"
    local data_args=("$@")
    data_args=("${data_args[@]:1}")

    echo "Configuring: $path"
    if ! vault write "$path" "${data_args[@]}"; then
        echo "ERROR: Failed to write configuration to $path. Check Vault logs and token permissions."
        exit 1
    fi
}

enable_auth() {
    path=$1
    type=$2
    if vault auth list | grep -q "^$path/"; then
        echo "Auth engine at $path/ already enabled"
    else
        vault auth enable -path="$path" $type
        echo "Enabled $path KV engine"
    fi
}

enable_secrets() {
    path=$1
    if vault secrets list | grep -q "^$path/"; then
        echo "Secrets engine at $path/ already enabled"
    else
        vault secrets enable -path=$path -version=1 kv
        echo "Enabled $path KV engine"
    fi
}

enable_secrets keys
enable_secrets secret

OIDC_PATH="oidc"
enable_auth "$OIDC_PATH" oidc

vault write identity/oidc/key/libops-api \
  allowed_client_ids="*" \
  verification_ttl="2h" \
  rotation_period="24h" \
  algorithm="RS256"

vault write identity/oidc/client/libops-api \
    redirect_uris="http://api:8080/auth/callback" \
    key="libops-api" \
    id_token_ttl="30m" \
    access_token_ttl="1h"

vault write identity/oidc/provider/libops-api \
  allowed_client_ids="*" \
  scopes="openid,email,profile" \
  issuer_host="http://vault.libops.io"

# Create an OIDC role for token generation
# This binds the key and template for direct token requests
vault write identity/oidc/role/libops-api \
  key="libops-api" \
  template="{\"account_id\": {{identity.entity.metadata.account_id}},\"email\": {{identity.entity.metadata.email}},\"name\": {{identity.entity.name}}}" \
  ttl="1h"

enable_auth userpass userpass

# allow api vault agent to get a vault token
enable_auth jwt jwt
vault write auth/jwt/config \
    oidc_discovery_url="https://accounts.google.com" \
    bound_issuer="https://accounts.google.com"
vault write auth/jwt/role/libops-api -<<EOF
{
  "user_claim": "sub",
  "bound_audiences": "https://vault.libops.io",
  "role_type": "jwt",
  "policies": "api",
  "ttl": "1h",
  "bound_claims": { "email": ["api-production@libops-api.iam.gserviceaccount.com"] }
}
EOF

# Create a token role that allows the API to create entity tokens with specific policies
vault write auth/token/roles/entity-token \
    allowed_policies="default,libops-user" \
    allowed_entity_aliases="*" \
    orphan=true \
    renewable=true \
    token_type="service"

# create policies defined in our policy dir
for FILE in policies/*; do
  FILE=$(basename "$FILE")
  ROLE=${FILE%%.*}
  vault policy write "$ROLE" "policies/$FILE"
done

echo "Vault initialization complete!"


# The snippet below list all the secret files referenced by the docker-compose.yml file.
# For each it will generate a random password.
readonly CHARACTERS='[A-Za-z0-9]'
readonly LENGTH=32
pushd /
yq -r '.secrets[].file' /docker-compose.yaml | uniq | while read -r SECRET; do
  NAME=$(basename "${SECRET}")
  if [ "${NAME}" = "GOOGLE_APPLICATION_CREDENTIALS" ]; then
    continue
  fi

  if [ ! -f "${SECRET}" ]; then
    echo "Creating: ${SECRET}" >&2
    DIR=$(dirname "${SECRET}")
    if [ ! -d "${DIR}" ]; then
      mkdir -p "$DIR"
    fi
    (grep -ao "${CHARACTERS}" < /dev/urandom || true) | head "-${LENGTH}" | tr -d '\n' > "${SECRET}"
  fi
done
