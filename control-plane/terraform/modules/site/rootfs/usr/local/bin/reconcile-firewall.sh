#!/usr/bin/env bash
#
# Firewall Reconciliation Service
# Fetches firewall rules from LibOps API and updates iptables for SSH access
#

set -euo pipefail

# Load environment variables
if [ -f /etc/default/reconcile-firewall ]; then
    # shellcheck source=/dev/null
    source /etc/default/reconcile-firewall
fi

# Configuration
API_BASE_URL="${LIBOPS_API_URL:-https://api.libops.io}"
PROJECT_ID="${LIBOPS_PROJECT_ID:-}"
SITE_ID="${LIBOPS_SITE_ID:-}"
ORG_ID="${LIBOPS_ORG_ID:-}"
LOG_PREFIX="[reconcile-firewall]"

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

# Fetch SSH allowed rules for a resource
fetch_ssh_rules() {
    local resource_type="$1"  # "orgs", "projects", or "sites"
    local resource_id="$2"
    local token="$3"

    local url="${API_BASE_URL}/v1/${resource_type}/${resource_id}/firewall-rules"

    log "Fetching firewall rules from ${url}"

    local response
    response=$(curl -s -f -H "Authorization: Bearer ${token}" "${url}") || {
        log "Warning: Failed to fetch firewall rules from ${url}"
        return 0
    }

    echo "${response}" | jq -r '.firewall_rules[] | select(.rule_type == "ssh_allowed") | .cidr'
}

# Update iptables rules for SSH access
update_iptables() {
    local -a cidrs=("$@")

    log "Updating iptables rules for SSH access"

    # Flush existing SSH rules
    iptables -D INPUT -p tcp --dport 22 -j LIBOPS_SSH 2>/dev/null || true
    iptables -F LIBOPS_SSH 2>/dev/null || true
    iptables -X LIBOPS_SSH 2>/dev/null || true

    # Create new chain
    iptables -N LIBOPS_SSH

    if [ ${#cidrs[@]} -eq 0 ]; then
        log "No CIDR rules specified, denying all SSH access"
        iptables -A LIBOPS_SSH -j DROP
    else
        log "Adding ${#cidrs[@]} SSH allow rule(s)"
        for cidr in "${cidrs[@]}"; do
            log "  Allowing SSH from ${cidr}"
            iptables -A LIBOPS_SSH -s "${cidr}" -j ACCEPT
        done
        # Drop everything else
        iptables -A LIBOPS_SSH -j DROP
    fi

    # Insert rule at the beginning of INPUT chain
    iptables -I INPUT 1 -p tcp --dport 22 -j LIBOPS_SSH

    log "iptables rules updated successfully"
}

# Save iptables rules to persist across reboots
save_iptables() {
    log "Saving iptables rules"

    if command -v iptables-save >/dev/null && command -v netfilter-persistent >/dev/null; then
        iptables-save > /etc/iptables/rules.v4
        log "iptables rules saved to /etc/iptables/rules.v4"
    else
        log "Warning: netfilter-persistent not available, rules may not persist across reboots"
    fi
}

# Main reconciliation logic
main() {
    log "Starting firewall reconciliation"

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

    # Get access token
    log "Obtaining access token from metadata server"
    local token
    token=$(get_access_token) || error "Failed to get access token"

    # Aggregate all SSH rules from org, project, and site
    local -a all_cidrs=()

    # Fetch org rules
    log "Fetching SSH rules for org: ${ORG_ID}"
    while IFS= read -r cidr; do
        [ -n "${cidr}" ] && all_cidrs+=("${cidr}")
    done < <(fetch_ssh_rules "orgs" "${ORG_ID}" "${token}")

    # Fetch project rules
    log "Fetching SSH rules for project: ${PROJECT_ID}"
    while IFS= read -r cidr; do
        [ -n "${cidr}" ] && all_cidrs+=("${cidr}")
    done < <(fetch_ssh_rules "projects" "${PROJECT_ID}" "${token}")

    # Fetch site rules
    log "Fetching SSH rules for site: ${SITE_ID}"
    while IFS= read -r cidr; do
        [ -n "${cidr}" ] && all_cidrs+=("${cidr}")
    done < <(fetch_ssh_rules "sites" "${SITE_ID}" "${token}")

    # Deduplicate CIDRs
    local -a unique_cidrs
    IFS=$'\n' unique_cidrs=($(printf '%s\n' "${all_cidrs[@]}" | sort -u))

    log "Found ${#unique_cidrs[@]} unique CIDR(s) for SSH access"

    # Update iptables
    update_iptables "${unique_cidrs[@]}"

    # Save rules
    save_iptables

    log "Firewall reconciliation completed successfully"
}

main "$@"
