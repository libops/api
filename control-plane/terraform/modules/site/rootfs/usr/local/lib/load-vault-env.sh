#!/usr/bin/env bash
#
# Vault Environment Variable Loader
# Loads environment variables from Vault secret mount at /vault/secrets
# Falls back to environment variables if not found in Vault
# Waits up to 120 seconds for required variables to appear
#

VAULT_SECRETS_DIR="${VAULT_SECRETS_DIR:-/vault/secrets}"
TIMEOUT_SECONDS=120
LOG_PREFIX="${LOG_PREFIX:-[load-vault-env]}"

log() {
    echo "${LOG_PREFIX} $*" >&2
}

error() {
    log "ERROR: $*"
    return 1
}

# load_env_var attempts to load an environment variable
# Priority: 1) Current environment, 2) Vault secrets file
# Args:
#   $1: variable name
#   $2: required (true/false) - if true, will wait for variable to appear
load_env_var() {
    local var_name="$1"
    local required="${2:-false}"
    local vault_file="${VAULT_SECRETS_DIR}/${var_name}"
    local start_time=$(date +%s)
    local current_time
    local elapsed

    # Check if already set in environment
    if [ -n "${!var_name}" ]; then
        log "Using environment variable: ${var_name}"
        return 0
    fi

    # If not required, try to load from Vault once
    if [ "${required}" = "false" ]; then
        if [ -f "${vault_file}" ]; then
            local value
            value=$(cat "${vault_file}")
            if [ -n "${value}" ]; then
                export "${var_name}=${value}"
                log "Loaded ${var_name} from Vault: ${vault_file}"
                return 0
            fi
        fi
        log "Variable ${var_name} not found (optional)"
        return 0
    fi

    # Required variable - wait up to TIMEOUT_SECONDS for it to appear
    log "Waiting for required variable: ${var_name}"
    while true; do
        current_time=$(date +%s)
        elapsed=$((current_time - start_time))

        # Check Vault secrets directory
        if [ -f "${vault_file}" ]; then
            local value
            value=$(cat "${vault_file}")
            if [ -n "${value}" ]; then
                export "${var_name}=${value}"
                log "Loaded ${var_name} from Vault after ${elapsed}s: ${vault_file}"
                return 0
            fi
        fi

        # Check timeout
        if [ ${elapsed} -ge ${TIMEOUT_SECONDS} ]; then
            error "Timeout waiting for required variable: ${var_name} (waited ${TIMEOUT_SECONDS}s)"
            return 1
        fi

        # Wait 2 seconds before checking again
        sleep 2
    done
}

# load_config_file sources a traditional config file if it exists
# This maintains backward compatibility with /etc/default/* files
load_config_file() {
    local config_file="$1"

    if [ -f "${config_file}" ]; then
        log "Loading config file: ${config_file}"
        # shellcheck source=/dev/null
        source "${config_file}"
        return 0
    fi

    return 1
}

# Convenience function to load multiple variables at once
# Usage: load_env_vars "VAR1:required" "VAR2:optional" "VAR3:required"
load_env_vars() {
    local var_spec
    local var_name
    local is_required
    local failed=false

    for var_spec in "$@"; do
        # Parse var_spec format: "VAR_NAME:required" or "VAR_NAME:optional" or just "VAR_NAME"
        if [[ "${var_spec}" == *:* ]]; then
            var_name="${var_spec%%:*}"
            is_required="${var_spec##*:}"
        else
            var_name="${var_spec}"
            is_required="optional"
        fi

        # Convert "required"/"optional" to true/false
        if [ "${is_required}" = "required" ]; then
            is_required="true"
        else
            is_required="false"
        fi

        if ! load_env_var "${var_name}" "${is_required}"; then
            failed=true
        fi
    done

    if [ "${failed}" = "true" ]; then
        return 1
    fi
    return 0
}
