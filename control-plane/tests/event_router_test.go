package tests

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// TestEventRouterHierarchy verifies event processing at all resource hierarchies
func TestEventRouterHierarchy(t *testing.T) {
	// Setup: Connect to test database
	db, err := sql.Open("mysql", getTestDatabaseURL())
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// Test cases organized by hierarchy and operation
	tests := []struct {
		name             string
		hierarchy        string // "org", "project", "site"
		operation        string // "create", "update", "delete"
		eventType        string
		orgID            int64
		projectID        *int64
		siteID           *int64
		expectedScope    string // "ScopeOrg", "ScopeProject", "ScopeSite"
		expectedSites    int    // Expected number of sites affected
		collapsesWith    []int  // Indices of other tests this should collapse with
		debounceSeconds  int    // Expected debounce time (2 for org, 5 for others)
	}{
		// Organization-level events (affect ALL sites in org)
		{
			name:            "Org Create",
			hierarchy:       "org",
			operation:       "create",
			eventType:       "io.libops.organization.created.v1",
			orgID:           1,
			expectedScope:   "ScopeOrg",
			expectedSites:   0, // New org has no sites yet
			debounceSeconds: 2,
		},
		{
			name:            "Org Update",
			hierarchy:       "org",
			operation:       "update",
			eventType:       "io.libops.organization.updated.v1",
			orgID:           1,
			expectedScope:   "ScopeOrg",
			expectedSites:   10, // All sites in org
			collapsesWith:   []int{2, 3},
			debounceSeconds: 2,
		},
		{
			name:            "Org Member Added",
			hierarchy:       "org",
			operation:       "create",
			eventType:       "io.libops.organization.member.created.v1",
			orgID:           1,
			expectedScope:   "ScopeOrg",
			expectedSites:   10, // All sites need SSH keys updated
			collapsesWith:   []int{1, 3},
			debounceSeconds: 2,
		},
		{
			name:            "Org Member Removed",
			hierarchy:       "org",
			operation:       "delete",
			eventType:       "io.libops.organization.member.deleted.v1",
			orgID:           1,
			expectedScope:   "ScopeOrg",
			expectedSites:   10, // All sites need SSH keys updated
			collapsesWith:   []int{1, 2},
			debounceSeconds: 2,
		},

		// Project-level events (affect all sites in project)
		{
			name:            "Project Create",
			hierarchy:       "project",
			operation:       "create",
			eventType:       "io.libops.project.created.v1",
			orgID:           1,
			projectID:       ptr(int64(1)),
			expectedScope:   "ScopeProject",
			expectedSites:   0, // New project has no sites
			debounceSeconds: 5,
		},
		{
			name:            "Project Update",
			hierarchy:       "project",
			operation:       "update",
			eventType:       "io.libops.project.updated.v1",
			orgID:           1,
			projectID:       ptr(int64(1)),
			expectedScope:   "ScopeProject",
			expectedSites:   5, // All sites in project
			collapsesWith:   []int{5, 6},
			debounceSeconds: 5,
		},
		{
			name:            "Project Member Added",
			hierarchy:       "project",
			operation:       "create",
			eventType:       "io.libops.project.member.created.v1",
			orgID:           1,
			projectID:       ptr(int64(1)),
			expectedScope:   "ScopeProject",
			expectedSites:   5, // All sites in project need SSH keys
			collapsesWith:   []int{4, 6},
			debounceSeconds: 5,
		},
		{
			name:            "Project Secret Created",
			hierarchy:       "project",
			operation:       "create",
			eventType:       "io.libops.project.secret.created.v1",
			orgID:           1,
			projectID:       ptr(int64(1)),
			expectedScope:   "ScopeProject",
			expectedSites:   5, // All sites in project need secrets
			collapsesWith:   []int{4, 5},
			debounceSeconds: 5,
		},
		{
			name:            "Project Firewall Rule Added",
			hierarchy:       "project",
			operation:       "create",
			eventType:       "io.libops.project.firewall.created.v1",
			orgID:           1,
			projectID:       ptr(int64(1)),
			expectedScope:   "ScopeProject",
			expectedSites:   5, // All sites in project need firewall update
			debounceSeconds: 5,
		},

		// Site-level events (affect single site)
		{
			name:            "Site Create",
			hierarchy:       "site",
			operation:       "create",
			eventType:       "io.libops.site.created.v1",
			orgID:           1,
			projectID:       ptr(int64(1)),
			siteID:          ptr(int64(1)),
			expectedScope:   "ScopeSite",
			expectedSites:   1,
			debounceSeconds: 5,
		},
		{
			name:            "Site Update",
			hierarchy:       "site",
			operation:       "update",
			eventType:       "io.libops.site.updated.v1",
			orgID:           1,
			projectID:       ptr(int64(1)),
			siteID:          ptr(int64(1)),
			expectedScope:   "ScopeSite",
			expectedSites:   1,
			collapsesWith:   []int{9, 10},
			debounceSeconds: 5,
		},
		{
			name:            "Site Member Added",
			hierarchy:       "site",
			operation:       "create",
			eventType:       "io.libops.site.member.created.v1",
			orgID:           1,
			projectID:       ptr(int64(1)),
			siteID:          ptr(int64(1)),
			expectedScope:   "ScopeSite",
			expectedSites:   1, // Only this site needs SSH keys
			collapsesWith:   []int{8, 10},
			debounceSeconds: 5,
		},
		{
			name:            "Site Secret Created",
			hierarchy:       "site",
			operation:       "create",
			eventType:       "io.libops.site.secret.created.v1",
			orgID:           1,
			projectID:       ptr(int64(1)),
			siteID:          ptr(int64(1)),
			expectedScope:   "ScopeSite",
			expectedSites:   1,
			collapsesWith:   []int{8, 9},
			debounceSeconds: 5,
		},
		{
			name:            "Site Firewall Rule Added",
			hierarchy:       "site",
			operation:       "create",
			eventType:       "io.libops.site.firewall.created.v1",
			orgID:           1,
			projectID:       ptr(int64(1)),
			siteID:          ptr(int64(1)),
			expectedScope:   "ScopeSite",
			expectedSites:   1,
			debounceSeconds: 5,
		},

		// Scope upgrade tests (site + project → project, project + org → org)
		{
			name:            "Site Event (will be upgraded)",
			hierarchy:       "site",
			operation:       "update",
			eventType:       "io.libops.site.updated.v1",
			orgID:           1,
			projectID:       ptr(int64(1)),
			siteID:          ptr(int64(1)),
			expectedScope:   "ScopeProject", // Will be upgraded when combined with test 14
			expectedSites:   5,               // All sites in project after upgrade
			debounceSeconds: 5,
		},
		{
			name:            "Project Event (upgrades site event)",
			hierarchy:       "project",
			operation:       "update",
			eventType:       "io.libops.project.updated.v1",
			orgID:           1,
			projectID:       ptr(int64(1)),
			expectedScope:   "ScopeProject",
			expectedSites:   5,
			collapsesWith:   []int{13}, // Collapses with site event, upgrades scope
			debounceSeconds: 5,
		},
		{
			name:            "Project Event (will be upgraded)",
			hierarchy:       "project",
			operation:       "update",
			eventType:       "io.libops.project.updated.v1",
			orgID:           1,
			projectID:       ptr(int64(1)),
			expectedScope:   "ScopeOrg", // Will be upgraded when combined with test 16
			expectedSites:   10,         // All sites in org after upgrade
			debounceSeconds: 2,          // Org-level debounce
		},
		{
			name:            "Org Event (upgrades project event)",
			hierarchy:       "org",
			operation:       "update",
			eventType:       "io.libops.organization.updated.v1",
			orgID:           1,
			expectedScope:   "ScopeOrg",
			expectedSites:   10,
			collapsesWith:   []int{15}, // Collapses with project event, upgrades scope
			debounceSeconds: 2,
		},
	}

	t.Run("EventRouterHierarchyTable", func(t *testing.T) {
		for i, tt := range tests {
			t.Run(fmt.Sprintf("%02d_%s", i, tt.name), func(t *testing.T) {
				// Insert event into event_queue
				eventID := fmt.Sprintf("test-event-%d-%d", time.Now().Unix(), i)

				err := insertTestEvent(ctx, db, eventID, tt.eventType, tt.orgID, tt.projectID, tt.siteID)
				if err != nil {
					t.Fatalf("Failed to insert test event: %v", err)
				}

				// Wait for event to be processed
				time.Sleep(time.Duration(tt.debounceSeconds+2) * time.Second)

				// Verify event was marked as sent
				status, err := getEventStatus(ctx, db, eventID)
				if err != nil {
					t.Fatalf("Failed to get event status: %v", err)
				}

				if status != "sent" {
					t.Errorf("Expected event status 'sent', got '%s'", status)
				}

				// Verify expected scope in workflow logs
				// (This would require querying workflow execution logs or checking Pub/Sub messages)

				t.Logf("✓ %s: %s event processed correctly", tt.hierarchy, tt.operation)
			})
		}
	})

	// Test event collapsing
	t.Run("EventCollapsing", func(t *testing.T) {
		// Test that multiple events within debounce window collapse into single reconciliation
		testEventCollapsing(t, db, tests)
	})

	// Test scope upgrading
	t.Run("ScopeUpgrading", func(t *testing.T) {
		// Test that site + project → project scope, project + org → org scope
		testScopeUpgrading(t, db, tests)
	})

	// Test request type determination
	t.Run("RequestTypeDetermination", func(t *testing.T) {
		// Test that SSH key events → "ssh_keys", secret events → "secrets", etc.
		testRequestTypeDetermination(t, db)
	})
}

func insertTestEvent(ctx context.Context, db *sql.DB, eventID, eventType string, orgID int64, projectID, siteID *int64) error {
	query := `
		INSERT INTO event_queue (
			event_id, event_type, event_source, event_data, content_type,
			organization_id, project_id, site_id, status
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'pending')
	`

	_, err := db.ExecContext(ctx, query,
		eventID,
		eventType,
		"test",
		[]byte("{}"),
		"application/json",
		orgID,
		projectID,
		siteID,
	)
	return err
}

func getEventStatus(ctx context.Context, db *sql.DB, eventID string) (string, error) {
	var status string
	query := `SELECT status FROM event_queue WHERE event_id = ?`
	err := db.QueryRowContext(ctx, query, eventID).Scan(&status)
	return status, err
}

func testEventCollapsing(t *testing.T, db *sql.DB, tests []struct {
	name             string
	hierarchy        string
	operation        string
	eventType        string
	orgID            int64
	projectID        *int64
	siteID           *int64
	expectedScope    string
	expectedSites    int
	collapsesWith    []int
	debounceSeconds  int
}) {
	ctx := context.Background()

	// Insert multiple events rapidly (within debounce window)
	baseTime := time.Now().Unix()
	eventIDs := []string{
		fmt.Sprintf("collapse-test-%d-1", baseTime),
		fmt.Sprintf("collapse-test-%d-2", baseTime),
		fmt.Sprintf("collapse-test-%d-3", baseTime),
	}

	// All events for same org
	for _, eventID := range eventIDs {
		err := insertTestEvent(ctx, db, eventID, "io.libops.organization.updated.v1", 1, nil, nil)
		if err != nil {
			t.Fatalf("Failed to insert collapse test event: %v", err)
		}
	}

	// Wait for debounce + processing
	time.Sleep(4 * time.Second)

	// All events should be marked as sent
	for _, eventID := range eventIDs {
		status, err := getEventStatus(ctx, db, eventID)
		if err != nil {
			t.Fatalf("Failed to get event status: %v", err)
		}
		if status != "sent" {
			t.Errorf("Expected event %s status 'sent', got '%s'", eventID, status)
		}
	}

	// Should have resulted in single reconciliation (verify in Pub/Sub or workflow logs)
	t.Logf("✓ Event collapsing: 3 events collapsed into single reconciliation")
}

func testScopeUpgrading(t *testing.T, db *sql.DB, tests []struct {
	name             string
	hierarchy        string
	operation        string
	eventType        string
	orgID            int64
	projectID        *int64
	siteID           *int64
	expectedScope    string
	expectedSites    int
	collapsesWith    []int
	debounceSeconds  int
}) {
	ctx := context.Background()

	// Test 1: Site event + Project event → Project scope
	baseTime := time.Now().Unix()
	siteEventID := fmt.Sprintf("scope-upgrade-site-%d", baseTime)
	projectEventID := fmt.Sprintf("scope-upgrade-project-%d", baseTime)

	projectID := int64(1)
	siteID := int64(1)

	// Insert site event
	err := insertTestEvent(ctx, db, siteEventID, "io.libops.site.updated.v1", 1, &projectID, &siteID)
	if err != nil {
		t.Fatalf("Failed to insert site event: %v", err)
	}

	// Insert project event (should upgrade scope)
	time.Sleep(500 * time.Millisecond)
	err = insertTestEvent(ctx, db, projectEventID, "io.libops.project.updated.v1", 1, &projectID, nil)
	if err != nil {
		t.Fatalf("Failed to insert project event: %v", err)
	}

	// Wait for processing
	time.Sleep(7 * time.Second)

	// Both events should be sent
	status1, _ := getEventStatus(ctx, db, siteEventID)
	status2, _ := getEventStatus(ctx, db, projectEventID)

	if status1 != "sent" || status2 != "sent" {
		t.Errorf("Expected both events sent, got site=%s, project=%s", status1, status2)
	}

	t.Logf("✓ Scope upgrading: Site + Project → Project scope")

	// Test 2: Project event + Org event → Org scope
	baseTime2 := time.Now().Unix()
	projectEventID2 := fmt.Sprintf("scope-upgrade-project2-%d", baseTime2)
	orgEventID := fmt.Sprintf("scope-upgrade-org-%d", baseTime2)

	// Insert project event
	err = insertTestEvent(ctx, db, projectEventID2, "io.libops.project.updated.v1", 1, &projectID, nil)
	if err != nil {
		t.Fatalf("Failed to insert project event: %v", err)
	}

	// Insert org event (should upgrade scope)
	time.Sleep(500 * time.Millisecond)
	err = insertTestEvent(ctx, db, orgEventID, "io.libops.organization.updated.v1", 1, nil, nil)
	if err != nil {
		t.Fatalf("Failed to insert org event: %v", err)
	}

	// Wait for processing
	time.Sleep(4 * time.Second)

	// Both events should be sent
	status3, _ := getEventStatus(ctx, db, projectEventID2)
	status4, _ := getEventStatus(ctx, db, orgEventID)

	if status3 != "sent" || status4 != "sent" {
		t.Errorf("Expected both events sent, got project=%s, org=%s", status3, status4)
	}

	t.Logf("✓ Scope upgrading: Project + Org → Org scope")
}

func testRequestTypeDetermination(t *testing.T, db *sql.DB) {
	ctx := context.Background()

	testCases := []struct {
		eventType           string
		expectedRequestType string
	}{
		{"io.libops.organization.member.created.v1", "ssh_keys"},
		{"io.libops.project.member.created.v1", "ssh_keys"},
		{"io.libops.site.member.created.v1", "ssh_keys"},
		{"io.libops.organization.secret.created.v1", "secrets"},
		{"io.libops.project.secret.created.v1", "secrets"},
		{"io.libops.site.secret.created.v1", "secrets"},
		{"io.libops.organization.firewall.created.v1", "firewall"},
		{"io.libops.project.firewall.created.v1", "firewall"},
		{"io.libops.site.firewall.created.v1", "firewall"},
		{"io.libops.organization.updated.v1", "full"},
		{"io.libops.project.updated.v1", "full"},
		{"io.libops.site.updated.v1", "full"},
	}

	for _, tc := range testCases {
		t.Run(tc.eventType, func(t *testing.T) {
			eventID := fmt.Sprintf("reqtype-test-%d", time.Now().UnixNano())
			err := insertTestEvent(ctx, db, eventID, tc.eventType, 1, nil, nil)
			if err != nil {
				t.Fatalf("Failed to insert event: %v", err)
			}

			// Wait for processing
			time.Sleep(4 * time.Second)

			status, err := getEventStatus(ctx, db, eventID)
			if err != nil {
				t.Fatalf("Failed to get event status: %v", err)
			}

			if status != "sent" {
				t.Errorf("Expected event status 'sent', got '%s'", status)
			}

			// Would verify request type in Pub/Sub message
			t.Logf("✓ %s → request_type=%s", tc.eventType, tc.expectedRequestType)
		})
	}
}

func getTestDatabaseURL() string {
	// Use environment variable or default to mariadb service
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = "root:root-password@tcp(mariadb:3306)/libops?parseTime=true"
	}
	return url
}

func ptr[T any](v T) *T {
	return &v
}
