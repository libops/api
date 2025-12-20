#!/usr/bin/env bash
#
# Firewall Trigger Watcher
# Watches for changes to the firewall trigger file and runs reconciliation
#

set -euo pipefail

TRIGGER_FILE="/tmp/triggers/firewall.trigger"
RECONCILE_SCRIPT="/usr/local/bin/reconcile-firewall.sh"
LOG_PREFIX="[watch-firewall-trigger]"

log() {
    echo "${LOG_PREFIX} $*" >&2
}

error() {
    log "ERROR: $*"
    exit 1
}

# Ensure trigger directory exists
mkdir -p "$(dirname "${TRIGGER_FILE}")"

# Create trigger file if it doesn't exist
if [ ! -f "${TRIGGER_FILE}" ]; then
    touch "${TRIGGER_FILE}"
    log "Created trigger file: ${TRIGGER_FILE}"
fi

log "Starting watch on ${TRIGGER_FILE}"

# Use inotifywait to monitor the trigger file
inotifywait -m -e modify -e create -e moved_to "${TRIGGER_FILE}" 2>&1 | while read -r directory events filename
do
    log "Trigger detected: ${events} on ${filename}"

    # Run the reconciliation script
    if [ -x "${RECONCILE_SCRIPT}" ]; then
        log "Running firewall reconciliation"
        if "${RECONCILE_SCRIPT}"; then
            log "Firewall reconciliation completed successfully"
        else
            log "ERROR: Firewall reconciliation failed with exit code $?"
        fi
    else
        error "Reconciliation script not found or not executable: ${RECONCILE_SCRIPT}"
    fi
done
