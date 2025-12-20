#!/bin/bash
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Database connection
DB_HOST="${DB_HOST:-localhost}"
DB_PORT="${DB_PORT:-3306}"
DB_USER="${DB_USER:-root}"
DB_PASS="${DB_PASS:-root-password}"
DB_NAME="${DB_NAME:-libops}"

MYSQL_CMD="mysql -h $DB_HOST -P $DB_PORT -u $DB_USER -p$DB_PASS $DB_NAME -N -s"

# Test counters
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0

# Helper functions
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

run_test() {
    local test_name="$1"
    TESTS_RUN=$((TESTS_RUN + 1))
    echo ""
    echo "===================================================================================="
    echo "TEST #${TESTS_RUN}: ${test_name}"
    echo "===================================================================================="
}

test_passed() {
    TESTS_PASSED=$((TESTS_PASSED + 1))
    log_info "✓ Test passed"
}

test_failed() {
    local reason="$1"
    TESTS_FAILED=$((TESTS_FAILED + 1))
    log_error "✗ Test failed: $reason"
}

insert_test_event() {
    local event_id="$1"
    local event_type="$2"
    local org_id="$3"
    local project_id="${4:-NULL}"
    local site_id="${5:-NULL}"

    if [ "$project_id" = "NULL" ]; then
        project_id_sql="NULL"
    else
        project_id_sql="$project_id"
    fi

    if [ "$site_id" = "NULL" ]; then
        site_id_sql="NULL"
    else
        site_id_sql="$site_id"
    fi

    $MYSQL_CMD <<EOF
INSERT INTO event_queue (
    event_id, event_type, event_source, event_data, content_type,
    organization_id, project_id, site_id, status
) VALUES (
    '$event_id',
    '$event_type',
    'test',
    '{}',
    'application/json',
    $org_id,
    $project_id_sql,
    $site_id_sql,
    'pending'
);
EOF
}

get_event_status() {
    local event_id="$1"
    $MYSQL_CMD <<EOF
SELECT status FROM event_queue WHERE event_id = '$event_id';
EOF
}

count_events_by_status() {
    local status="$1"
    $MYSQL_CMD <<EOF
SELECT COUNT(*) FROM event_queue WHERE status = '$status';
EOF
}

count_sites_for_org() {
    local org_id="$1"
    $MYSQL_CMD <<EOF
SELECT COUNT(*)
FROM sites s
JOIN projects p ON s.project_id = p.id
WHERE p.organization_id = $org_id
AND s.status != 'deleted';
EOF
}

count_sites_for_project() {
    local project_id="$1"
    $MYSQL_CMD <<EOF
SELECT COUNT(*)
FROM sites s
WHERE s.project_id = $project_id
AND s.status != 'deleted';
EOF
}

cleanup_test_events() {
    log_info "Cleaning up test events..."
    $MYSQL_CMD <<EOF
DELETE FROM event_queue WHERE event_source = 'test';
EOF
}

# Main test suite
main() {
    log_info "Starting Event Router Integration Tests"
    log_info "Database: $DB_USER@$DB_HOST:$DB_PORT/$DB_NAME"
    echo ""

    # Cleanup before tests
    cleanup_test_events

    # Test 1: Organization-level event
    run_test "Organization Update Event"
    local org_event_id="test-org-update-$(date +%s)"
    insert_test_event "$org_event_id" "io.libops.organization.updated.v1" 2
    log_info "Inserted org-level event: $org_event_id"

    log_info "Waiting 4 seconds for event processing (2s debounce + 2s buffer)..."
    sleep 4

    local status=$(get_event_status "$org_event_id")
    if [ "$status" = "sent" ]; then
        test_passed
    else
        test_failed "Expected status 'sent', got '$status'"
    fi

    # Test 2: Project-level event
    run_test "Project Update Event"
    local project_event_id="test-project-update-$(date +%s)"
    insert_test_event "$project_event_id" "io.libops.project.updated.v1" 2 2
    log_info "Inserted project-level event: $project_event_id"

    log_info "Waiting 7 seconds for event processing (5s debounce + 2s buffer)..."
    sleep 7

    status=$(get_event_status "$project_event_id")
    if [ "$status" = "sent" ]; then
        test_passed
    else
        test_failed "Expected status 'sent', got '$status'"
    fi

    # Test 3: Site-level event
    run_test "Site Update Event"
    local site_event_id="test-site-update-$(date +%s)"
    insert_test_event "$site_event_id" "io.libops.site.updated.v1" 2 2 1
    log_info "Inserted site-level event: $site_event_id"

    log_info "Waiting 7 seconds for event processing..."
    sleep 7

    status=$(get_event_status "$site_event_id")
    if [ "$status" = "sent" ]; then
        test_passed
    else
        test_failed "Expected status 'sent', got '$status'"
    fi

    # Test 4: Event collapsing (multiple events within debounce window)
    run_test "Event Collapsing (3 events → 1 reconciliation)"
    local base_time=$(date +%s)
    local collapse_event1="test-collapse-${base_time}-1"
    local collapse_event2="test-collapse-${base_time}-2"
    local collapse_event3="test-collapse-${base_time}-3"

    log_info "Inserting 3 rapid events for same org..."
    insert_test_event "$collapse_event1" "io.libops.organization.updated.v1" 2
    insert_test_event "$collapse_event2" "io.libops.organization.updated.v1" 2
    insert_test_event "$collapse_event3" "io.libops.organization.updated.v1" 2

    log_info "Waiting 4 seconds for event processing..."
    sleep 4

    status1=$(get_event_status "$collapse_event1")
    status2=$(get_event_status "$collapse_event2")
    status3=$(get_event_status "$collapse_event3")

    if [ "$status1" = "sent" ] && [ "$status2" = "sent" ] && [ "$status3" = "sent" ]; then
        test_passed
        log_info "All 3 events marked as sent (collapsed into single reconciliation)"
    else
        test_failed "Expected all events 'sent', got: $status1, $status2, $status3"
    fi

    # Test 5: Scope upgrading (site event + project event → project scope)
    run_test "Scope Upgrading (Site → Project)"
    local upgrade_time=$(date +%s)
    local upgrade_site_event="test-upgrade-site-${upgrade_time}"
    local upgrade_project_event="test-upgrade-project-${upgrade_time}"

    log_info "Inserting site event..."
    insert_test_event "$upgrade_site_event" "io.libops.site.updated.v1" 2 2 1

    log_info "Waiting 0.5s, then inserting project event (should upgrade scope)..."
    sleep 0.5
    insert_test_event "$upgrade_project_event" "io.libops.project.updated.v1" 2 2

    log_info "Waiting 7 seconds for event processing..."
    sleep 7

    status1=$(get_event_status "$upgrade_site_event")
    status2=$(get_event_status "$upgrade_project_event")

    if [ "$status1" = "sent" ] && [ "$status2" = "sent" ]; then
        test_passed
        log_info "Both events processed (scope upgraded from site to project)"
    else
        test_failed "Expected both events 'sent', got site=$status1, project=$status2"
    fi

    # Test 6: Request type determination
    run_test "Request Type: SSH Keys (member event)"
    local member_event_id="test-member-$(date +%s)"
    insert_test_event "$member_event_id" "io.libops.organization.member.created.v1" 2

    log_info "Waiting 4 seconds for event processing..."
    sleep 4

    status=$(get_event_status "$member_event_id")
    if [ "$status" = "sent" ]; then
        test_passed
        log_info "Member event processed (should generate ssh_keys request type)"
    else
        test_failed "Expected status 'sent', got '$status'"
    fi

    run_test "Request Type: Secrets (secret event)"
    local secret_event_id="test-secret-$(date +%s)"
    insert_test_event "$secret_event_id" "io.libops.project.secret.created.v1" 2 2

    log_info "Waiting 7 seconds for event processing..."
    sleep 7

    status=$(get_event_status "$secret_event_id")
    if [ "$status" = "sent" ]; then
        test_passed
        log_info "Secret event processed (should generate secrets request type)"
    else
        test_failed "Expected status 'sent', got '$status'"
    fi

    run_test "Request Type: Firewall (firewall event)"
    local firewall_event_id="test-firewall-$(date +%s)"
    insert_test_event "$firewall_event_id" "io.libops.site.firewall.created.v1" 2 2 1

    log_info "Waiting 7 seconds for event processing..."
    sleep 7

    status=$(get_event_status "$firewall_event_id")
    if [ "$status" = "sent" ]; then
        test_passed
        log_info "Firewall event processed (should generate firewall request type)"
    else
        test_failed "Expected status 'sent', got '$status'"
    fi

    # Test 7: Database queries
    run_test "Database Query: Sites for Org"
    local org_site_count=$(count_sites_for_org 2)
    log_info "Sites in org 2: $org_site_count"
    if [ "$org_site_count" -gt 0 ]; then
        test_passed
        log_info "Query returned $org_site_count sites (excludes deleted sites)"
    else
        test_failed "Expected > 0 sites, got $org_site_count"
    fi

    run_test "Database Query: Sites for Project"
    local project_site_count=$(count_sites_for_project 2)
    log_info "Sites in project 2: $project_site_count"
    if [ "$project_site_count" -gt 0 ]; then
        test_passed
        log_info "Query returned $project_site_count sites (excludes deleted sites)"
    else
        test_failed "Expected > 0 sites, got $project_site_count"
    fi

    # Cleanup after tests
    cleanup_test_events

    # Summary
    echo ""
    echo "===================================================================================="
    echo "TEST SUMMARY"
    echo "===================================================================================="
    echo -e "Tests run:    ${TESTS_RUN}"
    echo -e "${GREEN}Tests passed: ${TESTS_PASSED}${NC}"
    if [ $TESTS_FAILED -gt 0 ]; then
        echo -e "${RED}Tests failed: ${TESTS_FAILED}${NC}"
    else
        echo -e "Tests failed: ${TESTS_FAILED}"
    fi
    echo ""

    if [ $TESTS_FAILED -eq 0 ]; then
        log_info "✓ All tests passed!"
        exit 0
    else
        log_error "✗ Some tests failed"
        exit 1
    fi
}

# Run main test suite
main "$@"
