#!/usr/bin/env bash
#
# Rollout Reconciliation Service
# Runs the rollout script to deploy application updates
#

set -euo pipefail

# Load environment variables
if [ -f /etc/default/reconcile-rollout ]; then
    # shellcheck source=/dev/null
    source /etc/default/reconcile-rollout
fi

# Configuration
ROLLOUT_SCRIPT="${ROLLOUT_SCRIPT:-/mnt/disks/data/libops/rollout.sh}"
LOG_PREFIX="[reconcile-rollout]"

log() {
    echo "${LOG_PREFIX} $*" >&2
}

error() {
    log "ERROR: $*"
    exit 1
}

# Main reconciliation logic
main() {
    log "Starting rollout reconciliation"

    # Check if rollout script exists
    if [ ! -f "${ROLLOUT_SCRIPT}" ]; then
        error "Rollout script not found: ${ROLLOUT_SCRIPT}"
    fi

    if [ ! -x "${ROLLOUT_SCRIPT}" ]; then
        log "Making rollout script executable"
        chmod +x "${ROLLOUT_SCRIPT}"
    fi

    # Run the rollout script
    log "Running rollout script: ${ROLLOUT_SCRIPT}"
    if bash "${ROLLOUT_SCRIPT}"; then
        log "Rollout completed successfully"
    else
        local exit_code=$?
        error "Rollout failed with exit code ${exit_code}"
    fi

    log "Rollout reconciliation completed successfully"
}

main "$@"
