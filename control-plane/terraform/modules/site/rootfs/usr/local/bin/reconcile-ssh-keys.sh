#!/usr/bin/env bash
#
# SSH Keys Reconciliation Service
# Fetches SSH keys from LibOps API and updates user authorized_keys
#

set -euo pipefail

# Load environment variables
if [ -f /etc/default/reconcile-ssh-keys ]; then
    # shellcheck source=/dev/null
    source /etc/default/reconcile-ssh-keys
fi

# Configuration
API_BASE_URL="${LIBOPS_API_URL:-https://api.libops.io}"
PROJECT_ID="${LIBOPS_PROJECT_ID:-}"
SITE_ID="${LIBOPS_SITE_ID:-}"
LOG_PREFIX="[reconcile-ssh-keys]"

log() {
    echo "${LOG_PREFIX} $*" >&2
}

error() {
    log "ERROR: $*"
    exit 1
}

# Get access token from metadata server
get_access_token() {
    curl -s -H "Metadata-Flavor: Google" \
        "http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token" \
        | jq -r '.access_token'
}

# Fetch SSH keys for a given resource (project or site)
fetch_ssh_keys() {
    local resource_type="$1"  # "projects" or "sites"
    local resource_id="$2"
    local token="$3"

    local url="${API_BASE_URL}/v1/${resource_type}/${resource_id}/ssh-keys"

    log "Fetching SSH keys from ${url}"

    local response
    response=$(curl -s -f -H "Authorization: Bearer ${token}" "${url}") || {
        error "Failed to fetch SSH keys from ${url}"
    }

    echo "${response}" | jq -r '.ssh_keys[]'
}

# Update authorized_keys for a user
update_user_ssh_keys() {
    local username="$1"
    shift
    local keys=("$@")

    log "Updating SSH keys for user: ${username}"

    # Check if user exists
    if ! id "${username}" &>/dev/null; then
        log "User ${username} does not exist, skipping"
        return 0
    fi

    local user_home
    user_home=$(eval echo "~${username}")
    local ssh_dir="${user_home}/.ssh"
    local authorized_keys="${ssh_dir}/authorized_keys"

    # Create .ssh directory if it doesn't exist
    if [ ! -d "${ssh_dir}" ]; then
        mkdir -p "${ssh_dir}"
        chown "${username}:${username}" "${ssh_dir}"
        chmod 700 "${ssh_dir}"
    fi

    # Write keys to authorized_keys
    if [ ${#keys[@]} -eq 0 ]; then
        log "No SSH keys for ${username}, clearing authorized_keys"
        : > "${authorized_keys}"
    else
        log "Writing ${#keys[@]} SSH key(s) for ${username}"
        printf '%s\n' "${keys[@]}" > "${authorized_keys}"
    fi

    # Set proper permissions
    chown "${username}:${username}" "${authorized_keys}"
    chmod 600 "${authorized_keys}"

    log "Successfully updated SSH keys for ${username}"
}

# Main reconciliation logic
main() {
    log "Starting SSH keys reconciliation"

    # Validate environment
    if [ -z "${SITE_ID}" ]; then
        error "LIBOPS_SITE_ID environment variable is required"
    fi

    if [ -z "${PROJECT_ID}" ]; then
        error "LIBOPS_PROJECT_ID environment variable is required"
    fi

    # Get access token
    log "Obtaining access token from metadata server"
    local token
    token=$(get_access_token) || error "Failed to get access token"

    # Fetch project SSH keys
    log "Fetching SSH keys for project: ${PROJECT_ID}"
    local project_keys
    mapfile -t project_keys < <(fetch_ssh_keys "projects" "${PROJECT_ID}" "${token}")

    # Fetch site SSH keys
    log "Fetching SSH keys for site: ${SITE_ID}"
    local site_keys
    mapfile -t site_keys < <(fetch_ssh_keys "sites" "${SITE_ID}" "${token}")

    # Update project user
    update_user_ssh_keys "${PROJECT_ID}" "${project_keys[@]}"

    # Update site user
    update_user_ssh_keys "${SITE_ID}" "${site_keys[@]}"

    log "SSH keys reconciliation completed successfully"
}

main "$@"
