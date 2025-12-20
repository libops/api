#!/bin/bash

# Helper script to test reconciliation logic with fake VMs
# This script provides tools for verifying the reconciliation flow

set -e

# Color codes
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

COMPOSE_FILE="${COMPOSE_FILE:-../docker-compose.yaml:../docker-compose.ci.yaml:../docker-compose.control-plane.yaml}"
export COMPOSE_FILE

show_help() {
  echo "Usage: $0 [COMMAND]"
  echo ""
  echo "Commands:"
  echo "  status       Show event queue status and processed events"
  echo "  logs         Show logs from orchestrator and fake VMs"
  echo "  insert       Insert a test reconciliation event"
  echo "  watch        Watch for reconciliation activity in real-time"
  echo "  verify       Verify reconciliation messages were received"
  echo "  help         Show this help message"
  echo ""
}

show_status() {
  echo -e "${BLUE}=== Event Queue Status ===${NC}"
  echo ""

  echo "Event counts by status:"
  docker compose exec -T mariadb mariadb -u libops -plibops-test-pass libops -e \
    "SELECT status, COUNT(*) as count FROM event_queue GROUP BY status"

  echo ""
  echo "Test events:"
  docker compose exec -T mariadb mariadb -u libops -plibops-test-pass libops -e \
    "SELECT event_id, event_type, status, created_at FROM event_queue WHERE event_id LIKE 'test-evt-%' ORDER BY created_at DESC LIMIT 10"

  echo ""
  echo "Latest 5 events:"
  docker compose exec -T mariadb mariadb -u libops -plibops-test-pass libops -e \
    "SELECT event_id, event_type, status, organization_id, project_id, site_id FROM event_queue ORDER BY created_at DESC LIMIT 5"
}

show_logs() {
  echo -e "${BLUE}=== Orchestrator Logs (last 50 lines) ===${NC}"
  docker compose logs --tail=50 orchestrator

  echo ""
  echo -e "${BLUE}=== Fake Org VM 2 Logs (for Vandelay Industries) ===${NC}"
  docker compose logs --tail=30 fake-org-vm-2

  echo ""
  echo -e "${BLUE}=== All Fake VM Reconciliation Messages ===${NC}"
  echo ""
  echo "Site reconciliation messages:"
  docker compose logs fake-site-vm-1 fake-site-vm-2 fake-site-vm-3 2>/dev/null | grep "reconcile site" || echo "  (none)"

  echo ""
  echo "Terraform run messages:"
  docker compose logs fake-org-vm-1 fake-org-vm-2 fake-org-vm-3 2>/dev/null | grep "terraform run" || echo "  (none)"
}

insert_test_event() {
  echo -e "${YELLOW}Inserting test reconciliation event...${NC}"
  echo ""
  echo "Select event type:"
  echo "  1) SSH key created (site-level)"
  echo "  2) Site member created (site-level)"
  echo "  3) Site secret updated (site-level)"
  echo "  4) Organization firewall created (org-level)"
  echo "  5) Project firewall created (project-level)"
  read -p "Enter choice [1-5]: " choice

  EVENT_ID="test-evt-manual-$(date +%s)"

  case $choice in
    1)
      EVENT_TYPE="io.libops.ssh_key.created.v1"
      ORG_ID=2
      PROJ_ID=2
      SITE_ID=1
      ;;
    2)
      EVENT_TYPE="io.libops.site_member.created.v1"
      ORG_ID=2
      PROJ_ID=2
      SITE_ID=1
      ;;
    3)
      EVENT_TYPE="io.libops.site_secret.updated.v1"
      ORG_ID=2
      PROJ_ID=2
      SITE_ID=1
      ;;
    4)
      EVENT_TYPE="io.libops.organization_firewall.created.v1"
      ORG_ID=2
      PROJ_ID=NULL
      SITE_ID=NULL
      ;;
    5)
      EVENT_TYPE="io.libops.project_firewall.created.v1"
      ORG_ID=2
      PROJ_ID=2
      SITE_ID=NULL
      ;;
    *)
      echo "Invalid choice"
      exit 1
      ;;
  esac

  SQL="INSERT INTO event_queue (
    event_id, event_type, event_source, event_subject,
    event_data, content_type,
    organization_id, project_id, site_id,
    status, created_at
  ) VALUES (
    '$EVENT_ID',
    '$EVENT_TYPE',
    'manual-test',
    'test/manual',
    '',
    'application/protobuf',
    $ORG_ID, $PROJ_ID, $SITE_ID,
    'pending',
    NOW()
  );"

  docker compose exec -T mariadb mariadb -u libops -plibops-test-pass libops -e "$SQL"

  echo -e "${GREEN}✓ Event inserted: $EVENT_ID${NC}"
  echo "  Type: $EVENT_TYPE"
  echo "  Org: $ORG_ID, Project: $PROJ_ID, Site: $SITE_ID"
  echo ""
  echo "Wait a few seconds, then run: $0 verify"
}

watch_activity() {
  echo -e "${YELLOW}Watching for reconciliation activity... (Ctrl+C to stop)${NC}"
  echo ""
  docker compose logs -f orchestrator fake-org-vm-1 fake-org-vm-2 fake-site-vm-1 2>&1 | \
    grep --line-buffered -E "(Processing events|reconcile|terraform|Received event|Dispatching)"
}

verify_reconciliation() {
  echo -e "${BLUE}=== Verifying Reconciliation ===${NC}"
  echo ""

  # Count processed events
  PROCESSED=$(docker compose exec -T mariadb mariadb -u libops -plibops-test-pass libops -N -e \
    "SELECT COUNT(*) FROM event_queue WHERE status IN ('sent', 'executed', 'collapsed')")

  echo "Processed events: $PROCESSED"

  # Count reconciliation messages in logs
  SITE_RECON=$(docker compose logs fake-site-vm-1 fake-site-vm-2 fake-site-vm-3 2>/dev/null | grep -c "\[FAKE\] would reconcile site" || echo "0")
  ORG_TERRAFORM=$(docker compose logs fake-org-vm-1 fake-org-vm-2 fake-org-vm-3 2>/dev/null | grep -c "\[FAKE\] would run terraform" || echo "0")

  echo "Site reconciliation messages: $SITE_RECON"
  echo "Org terraform messages: $ORG_TERRAFORM"

  echo ""
  if [ "$SITE_RECON" -gt 0 ] || [ "$ORG_TERRAFORM" -gt 0 ]; then
    echo -e "${GREEN}✓ Reconciliation is working!${NC}"

    if [ "$SITE_RECON" -gt 0 ]; then
      echo ""
      echo "Recent site reconciliation messages:"
      docker compose logs fake-site-vm-1 fake-site-vm-2 fake-site-vm-3 2>/dev/null | grep "reconcile site" | tail -5
    fi

    if [ "$ORG_TERRAFORM" -gt 0 ]; then
      echo ""
      echo "Recent terraform run messages:"
      docker compose logs fake-org-vm-1 fake-org-vm-2 fake-org-vm-3 2>/dev/null | grep "terraform run" | tail -5
    fi
  else
    echo -e "${RED}✗ No reconciliation messages found${NC}"
    echo ""
    echo "Troubleshooting tips:"
    echo "  1. Check if orchestrator is running: docker compose ps orchestrator"
    echo "  2. Check if fake VMs are connected: docker compose logs fake-org-vm-2 | grep connected"
    echo "  3. View orchestrator logs: $0 logs"
    echo "  4. Check event queue: $0 status"
  fi
}

# Main command dispatcher
case "${1:-help}" in
  status)
    show_status
    ;;
  logs)
    show_logs
    ;;
  insert)
    insert_test_event
    ;;
  watch)
    watch_activity
    ;;
  verify)
    verify_reconciliation
    ;;
  help|--help|-h)
    show_help
    ;;
  *)
    echo "Unknown command: $1"
    echo ""
    show_help
    exit 1
    ;;
esac
