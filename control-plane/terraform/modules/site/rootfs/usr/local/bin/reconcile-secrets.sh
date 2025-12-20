#!/usr/bin/env bash
#
# Secrets Reconciliation Service
# Fetches secrets from Vault (org, project, site) and writes to disk
#

set -euo pipefail

# Load environment variables
if [ -f /etc/default/reconcile-secrets ]; then
    # shellcheck source=/dev/null
    source /etc/default/reconcile-secrets
fi

# Configuration
VAULT_ADDR="${VAULT_ADDR:-https://vault.libops.io}"
PROJECT_ID="${LIBOPS_PROJECT_ID:-}"
SITE_ID="${LIBOPS_SITE_ID:-}"
ORG_ID="${LIBOPS_ORG_ID:-}"
SECRETS_DIR="${SECRETS_DIR:-/mnt/disks/data/compose/secrets}"
LOG_PREFIX="[reconcile-secrets]"

log() {
    echo "${LOG_PREFIX} $*" >&2
}

error() {
    log "ERROR: $*"
    exit 1
}

# Get Vault token from GCP metadata server
get_vault_token() {
    local gcp_token
    gcp_token=$(curl -s -H "Metadata-Flavor: Google" \
        "http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token" \
        | jq -r '.access_token')

    # Exchange GCP token for Vault token
    local vault_token
    vault_token=$(curl -s -f -X POST \
        -H "Content-Type: application/json" \
        -d "{\"role\": \"site\", \"jwt\": \"${gcp_token}\"}" \
        "${VAULT_ADDR}/v1/auth/gcp/login" \
        | jq -r '.auth.client_token')

    echo "${vault_token}"
}

# List secrets at a given Vault path
list_secrets() {
    local path="$1"
    local token="$2"

    log "Listing secrets at ${path}"

    local response
    response=$(curl -s -f -X LIST \
        -H "X-Vault-Token: ${token}" \
        "${VAULT_ADDR}/v1/${path}" 2>/dev/null) || {
        log "No secrets found at ${path}"
        return 0
    }

    echo "${response}" | jq -r '.data.keys[]?' || true
}

# Fetch a secret value from Vault
fetch_secret() {
    local path="$1"
    local token="$2"

    log "Fetching secret: ${path}"

    local response
    response=$(curl -s -f -X GET \
        -H "X-Vault-Token: ${token}" \
        "${VAULT_ADDR}/v1/${path}") || {
        error "Failed to fetch secret: ${path}"
    }

    echo "${response}" | jq -r '.data.value'
}

# Write secret to disk
write_secret() {
    local secret_name="$1"
    local secret_value="$2"
    local output_file="${SECRETS_DIR}/${secret_name}"

    log "Writing secret to ${output_file}"

    echo "${secret_value}" > "${output_file}"
    chmod 600 "${output_file}"
}

# Reconcile secrets from a given path
reconcile_path() {
    local path="$1"
    local token="$2"

    log "Reconciling secrets from ${path}"

    local secrets
    secrets=$(list_secrets "${path}" "${token}")

    if [ -z "${secrets}" ]; then
        log "No secrets to reconcile from ${path}"
        return 0
    fi

    while IFS= read -r secret_name; do
        [ -z "${secret_name}" ] && continue

        local secret_path="${path}/${secret_name}"
        local secret_value
        secret_value=$(fetch_secret "${secret_path}" "${token}")

        write_secret "${secret_name}" "${secret_value}"
    done <<< "${secrets}"
}

# Main reconciliation logic
main() {
    log "Starting secrets reconciliation"

    # Validate environment
    if [ -z "${SITE_ID}" ]; then
        error "LIBOPS_SITE_ID environment variable is required"
    fi

    if [ -z "${PROJECT_ID}" ]; then
        error "LIBOPS_PROJECT_ID environment variable is required"
    fi

    if [ -z "${ORG_ID}" ]; then
        error "LIBOPS_ORG_ID environment variable is required"
    fi

    # Create secrets directory
    mkdir -p "${SECRETS_DIR}"
    chmod 700 "${SECRETS_DIR}"

    # Get Vault token
    log "Obtaining Vault token"
    local vault_token
    vault_token=$(get_vault_token) || error "Failed to get Vault token"

    # Reconcile secrets in order: org -> project -> site
    # Later secrets override earlier ones with the same name

    log "Reconciling org secrets"
    reconcile_path "secret/data/orgs/${ORG_ID}" "${vault_token}"

    log "Reconciling project secrets"
    reconcile_path "secret/data/projects/${PROJECT_ID}" "${vault_token}"

    log "Reconciling site secrets"
    reconcile_path "secret/data/sites/${SITE_ID}" "${vault_token}"

    log "Secrets reconciliation completed successfully"
}

main "$@"
