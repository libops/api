#!/usr/bin/env bash
#
# Setup script for controller trigger watchers
# This script is run by cloud-init to install and configure all watcher services
#

set -euo pipefail

LOG_PREFIX="[setup-watchers]"

log() {
    echo "${LOG_PREFIX} $*" >&2
}

log "Installing inotify-tools and iptables-persistent"
apt-get update -qq
apt-get install -y inotify-tools iptables-persistent netfilter-persistent

log "Creating trigger directory"
mkdir -p /tmp/triggers
chmod 755 /tmp/triggers

log "Creating iptables rules directory"
mkdir -p /etc/iptables
chmod 755 /etc/iptables

log "Making all watcher scripts executable"
chmod +x /usr/local/bin/watch-ssh-keys-trigger.sh
chmod +x /usr/local/bin/watch-firewall-trigger.sh
chmod +x /usr/local/bin/watch-rollout-trigger.sh

log "Making all reconciliation scripts executable"
chmod +x /usr/local/bin/reconcile-ssh-keys.sh
chmod +x /usr/local/bin/reconcile-firewall.sh
chmod +x /usr/local/bin/reconcile-secrets.sh
chmod +x /usr/local/bin/reconcile-rollout.sh

log "Reloading systemd daemon"
systemctl daemon-reload

log "Enabling watcher services"
systemctl enable watch-ssh-keys-trigger.service
systemctl enable watch-firewall-trigger.service
systemctl enable watch-rollout-trigger.service

log "Starting watcher services"
systemctl start watch-ssh-keys-trigger.service
systemctl start watch-firewall-trigger.service
systemctl start watch-rollout-trigger.service

log "All trigger watchers setup completed"
