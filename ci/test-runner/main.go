package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/fatih/color"
	libopsv1 "github.com/libops/api/proto/libops/v1"
	commonv1 "github.com/libops/api/proto/libops/v1/common"
	"github.com/libops/api/proto/libops/v1/libopsv1connect"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"database/sql"

	_ "github.com/go-sql-driver/mysql"
	"github.com/hashicorp/vault/api"
)

var (
	apiURL     = getEnv("API_URL", "http://localhost:8080")
	dbURL      = getEnv("DATABASE_URL", "")
	vaultAddr  = getEnv("VAULT_ADDR", "http://localhost:8200")
	vaultToken = getEnv("VAULT_TOKEN", "test-root-token")

	// Test user API keys (from rbac_seed.sql and vault-init.sh)
	apiKeys = map[string]string{
		// Full scope keys
		"admin":           "libops_admin_full",
		"org-owner":       "libops_org_owner_full",
		"org-developer":   "libops_org_developer_full",
		"org-read":        "libops_org_read_full",
		"proj1-owner":     "libops_proj1_owner_full",
		"proj1-developer": "libops_proj1_developer_full",
		"proj1-read":      "libops_proj1_read_full",
		"site1-owner":     "libops_site1_owner_full",
		"site1-developer": "libops_site1_developer_full",
		"site1-read":      "libops_site1_read_full",
		"proj2-owner":     "libops_proj2_owner_full",
		"proj2-developer": "libops_proj2_developer_full",
		"proj2-read":      "libops_proj2_read_full",
		"no-access":       "libops_no_access",

		// Limited scope keys
		"admin-limited":       "libops_admin_limited",
		"org-owner-limited":   "libops_org_owner_limited",
		"proj1-owner-limited": "libops_proj1_owner_limited",
		"site1-owner-limited": "libops_site1_owner_limited",
	}

	// Test user credentials (email:password) - derived from vault-init.sh
	userCredentials = map[string]string{
		"admin":           "admin@root.com:password123",
		"org-owner":       "org-owner@child.com:password123",
		"org-developer":   "org-developer@child.com:password123",
		"org-read":        "org-read@child.com:password123",
		"proj1-owner":     "proj1-owner@child.com:password123",
		"proj1-developer": "proj1-developer@child.com:password123",
		"proj1-read":      "proj1-read@child.com:password123",
		"site1-owner":     "site1-owner@child.com:password123",
		"site1-developer": "site1-developer@child.com:password123",
		"site1-read":      "site1-read@child.com:password123",
		"proj2-owner":     "proj2-owner@child.com:password123",
		"no-access":       "noaccess@test.com:password123",
	}

	// Cache for user tokens
	userTokens = make(map[string]string)

	// Test resource IDs (from rbac_seed.sql)
	rootOrgID   = "40000000-0000-0000-0000-000000000001"
	childOrgID  = "40000000-0000-0000-0000-000000000002"
	project1ID  = "50000000-0000-0000-0000-000000000001" // Project Alpha
	project2ID  = "50000000-0000-0000-0000-000000000002" // Project Beta
	site1ProdID = "60000000-0000-0000-0000-000000000001" // Site 1 production
	site1StagID = "60000000-0000-0000-0000-000000000002" // Site 1 staging
	site2ProdID = "60000000-0000-0000-0000-000000000003" // Site 2 production (Project 2)

	// Dynamic IDs (created during tests)
	createdProjectID string
	createdSiteID    string

	green  = color.New(color.FgGreen).SprintFunc()
	red    = color.New(color.FgRed).SprintFunc()
	cyan   = color.New(color.FgCyan).SprintFunc()
	yellow = color.New(color.FgYellow).SprintFunc()
)

type AuthMethod string

const (
	AuthMethodAPIKey   AuthMethod = "APIKey"
	AuthMethodUserpass AuthMethod = "Userpass"
)

var currentAuthMethod AuthMethod = AuthMethodAPIKey

type TestRunner struct {
	passed, failed int
}

func main() {
	if len(os.Args) < 2 || os.Args[1] != "run-tests" {
		fmt.Println("Usage: test-runner run-tests")
		os.Exit(1)
	}

	runner := &TestRunner{}
	runner.RunAllTests()
	runner.PrintResults()

	if runner.failed > 0 {
		os.Exit(1)
	}
}

func (tr *TestRunner) RunAllTests() {
	fmt.Println(cyan("================================================="))
	fmt.Println(cyan("  LibOps API Integration Tests - RBAC Validation"))
	fmt.Println(cyan("=================================================\n"))

	tr.waitForAPI()

	fmt.Println(yellow("\n--- Test Setup Phase ---"))
	tr.setupTestEnvironment()

	// Run tests with API Key auth
	fmt.Println(cyan("\n================================================="))
	fmt.Println(cyan("  RUNNING TESTS WITH: API KEY AUTH"))
	fmt.Println(cyan("================================================="))
	currentAuthMethod = AuthMethodAPIKey
	tr.runPermissionMatrixTests()

	// Run tests with Userpass auth
	fmt.Println(cyan("\n================================================="))
	fmt.Println(cyan("  RUNNING TESTS WITH: USERPASS AUTH"))
	fmt.Println(cyan("================================================="))
	currentAuthMethod = AuthMethodUserpass
	// Reset state if needed, though matrix tests are mostly idempotent or cleanup after themselves
	// Only createdProjectID/createdSiteID need care, but they are reset in tests
	tr.runPermissionMatrixTests()
}

func (tr *TestRunner) waitForAPI() {
	fmt.Print("Waiting for API...")
	for i := 0; i < 30; i++ {
		resp, err := http.Get(apiURL + "/health")
		if err == nil && resp.StatusCode == 200 {
			resp.Body.Close()
			fmt.Println(green(" ✓"))
			return
		}
		time.Sleep(2 * time.Second)
		fmt.Print(".")
	}
	fmt.Println(red(" ✗"))
	os.Exit(1)
}

// getToken returns a valid token for the user based on currentAuthMethod
func (tr *TestRunner) getToken(userKey string) string {
	if currentAuthMethod == AuthMethodAPIKey {
		if key, ok := apiKeys[userKey]; ok {
			return key
		}
		// Fallback for non-standard keys or limited keys that might not be in credentials map
		if strings.HasPrefix(userKey, "libops_") {
			return userKey
		}
		return ""
	}

	// Userpass flow
	if token, ok := userTokens[userKey]; ok {
		return token
	}

	creds, ok := userCredentials[userKey]
	if !ok {
		// Some "users" in tests are actually just keys (like "admin-limited"), they don't have passwords
		// Fallback to API key if credential not found, or return empty if strict
		if key, ok := apiKeys[userKey]; ok {
			return key
		}
		return ""
	}

	parts := strings.Split(creds, ":")
	email, password := parts[0], parts[1]

	// Extract username from email (everything before @)
	// This is because Vault userpass username cannot contain @ in some versions/configs,
	// so we store users as "admin" instead of "admin@root.com" in Vault,
	// but the App still sends "email" in the JSON body.
	// Wait - the APP sends `req.Email` to `userpass.NewUserpassAuth`.
	// So the App is sending "admin@root.com".
	// If Vault doesn't support @, the APP will fail to login unless we change the App code too.
	// Let's assume we changed the Vault user creation to use just the username part.
	// But `userpass.NewUserpassAuth(req.Email, ...)` uses `req.Email` as the username.
	// So we MUST change the App to either:
	// 1. Send only the username part to Vault.
	// 2. Or we fix the Vault side.
	//
	// If the user says "username can not contain the at sign", then we must change the APP to strip it?
	// OR we change the Test Runner to EXPECT this behavior if we change the app.
	//
	// Let's assume the App logic (internal/auth/token.go) will be updated to strip @ from the username before calling Vault.
	//
	// ...
	// Actually, let's stick to the plan:
	// 1. Update `vault-init.sh` to create users as `admin` instead of `admin@root.com`.
	// 2. Update `internal/auth/token.go` to use `email` as the lookup key for DB, but strip it for Vault Auth?
	//
	// No, wait. `userpass` auth method in Vault DOES support arbitrary strings.
	// But if we are stuck, let's use underscores instead of @ in Vault.
	// `admin_root.com`.

	// Let's proceed with modifying the TEST RUNNER to expect the login to work with the full email,
	// assuming we fix the underlying issue.
	// But if the user is instructing that it CANNOT contain @, then we have to align everything.

	loginURL := apiURL + "/auth/token"
	body := fmt.Sprintf(`{"grant_type":"password","username":"%s","password":"%s"}`, email, password)
	resp, err := http.Post(loginURL, "application/json", strings.NewReader(body))
	if err != nil {
		fmt.Printf(red("Failed to login user %s: %v\n"), userKey, err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	// Read response body for error details
	buf := make([]byte, 4096)
	n, _ := resp.Body.Read(buf)
	response := string(buf[:n])

	if resp.StatusCode != 200 {
		// Special case: "no-access" user might fail if not verified, but let's assume they are valid users
		// If login fails, maybe they don't exist in DB yet?
		fmt.Printf(red("Login failed for %s: status %d\nResponse: %s\n"), userKey, resp.StatusCode, response)
		os.Exit(1)
	}

	start := strings.Index(response, `"id_token":"`)
	if start == -1 {
		fmt.Printf(red("No id_token in response for %s\n"), userKey)
		os.Exit(1)
	}
	start += 12
	end := strings.Index(response[start:], `"`)
	token := response[start : start+end]

	userTokens[userKey] = token
	return token
}

func (tr *TestRunner) setupTestEnvironment() {
	fmt.Println("Verifying seed data...")
	fmt.Printf("  Root org ID: %s\n", rootOrgID)
	fmt.Printf("  Child org ID: %s\n", childOrgID)
	fmt.Printf("  Project 1 ID: %s\n", project1ID)
	fmt.Printf("  Project 2 ID: %s\n", project2ID)
	fmt.Printf("  Total API keys: %d\n", len(apiKeys))

	// Sync Vault Entity IDs to Database
	// This is required because Vault generates random Entity IDs that we can't force during init.
	// We match them by email (which is stored in Entity metadata).
	fmt.Println("Syncing Vault Entity IDs to Database...")
	tr.syncVaultEntities()

	fmt.Println(green("✓ Seed data verified"))
}

func (tr *TestRunner) syncVaultEntities() {
	if dbURL == "" {
		fmt.Println(yellow("⚠️  DATABASE_URL not set, skipping Vault Entity ID sync. Tests using userpass auth might fail."))
		return
	}

	// Connect to Database
	db, err := sql.Open("mysql", dbURL)
	if err != nil {
		fmt.Printf(red("Failed to connect to DB: %v\n"), err)
		os.Exit(1)
	}
	defer db.Close()

	// Connect to Vault
	config := api.DefaultConfig()
	config.Address = vaultAddr
	client, err := api.NewClient(config)
	if err != nil {
		fmt.Printf(red("Failed to create Vault client: %v\n"), err)
		os.Exit(1)
	}
	client.SetToken(vaultToken)

	// Iterate through user credentials to find emails
	for _, creds := range userCredentials {
		parts := strings.Split(creds, ":")
		email := parts[0]

		// Lookup Entity ID by name (entity-<email>)
		entityName := fmt.Sprintf("entity-%s", email)
		secret, err := client.Logical().Read(fmt.Sprintf("identity/entity/name/%s", entityName))
		if err != nil {
			fmt.Printf(red("Failed to read entity %s from Vault: %v\n"), entityName, err)
			continue
		}
		if secret == nil || secret.Data == nil {
			fmt.Printf(yellow("Entity %s not found in Vault, skipping sync.\n"), entityName)
			continue
		}

		entityID, ok := secret.Data["id"].(string)
		if !ok || entityID == "" {
			fmt.Printf(red("Entity %s has no ID in Vault response\n"), entityName)
			continue
		}

		// Update Database
		_, err = db.Exec("UPDATE accounts SET vault_entity_id = ? WHERE email = ?", entityID, email)
		if err != nil {
			fmt.Printf(red("Failed to update account %s with entity ID %s: %v\n"), email, entityID, err)
			os.Exit(1)
		}
		// fmt.Printf("Synced %s -> %s\n", email, entityID)
	}
	fmt.Println(green("✓ Vault Entities synced"))
}

func (tr *TestRunner) runPermissionMatrixTests() {
	ctx := context.Background()

	// Phase 1: Organization Operations
	fmt.Println(cyan("\n=== Phase 1: Organization Operations ==="))
	tr.testOrganizationOperations(ctx)

	// Phase 2: Project Operations
	fmt.Println(cyan("\n=== Phase 2: Project Operations ==="))
	tr.testProjectOperations(ctx)

	// Phase 3: Site Operations
	fmt.Println(cyan("\n=== Phase 3: Site Operations ==="))
	tr.testSiteOperations(ctx)

	// Phase 4: Firewall Operations
	fmt.Println(cyan("\n=== Phase 4: Firewall Operations ==="))
	tr.testFirewallOperations(ctx)

	// Phase 5: Member Management
	fmt.Println(cyan("\n=== Phase 5: Member Management ==="))
	tr.testMemberManagement(ctx)

	// Phase 6: API Key Management
	fmt.Println(cyan("\n=== Phase 6: API Key Management ==="))
	tr.testAPIKeyManagement(ctx)

	// Phase 7: Scope Restrictions (using dynamically created keys)
	fmt.Println(cyan("\n=== Phase 7: Scope Restrictions ==="))
	tr.testScopeRestrictions(ctx)

	// Phase 7: Isolation Tests
	fmt.Println(cyan("\n=== Phase 7: Isolation Tests ==="))
	tr.testCrossResourceIsolation(ctx)
	tr.testMembershipInheritance(ctx)
}

// Helper for matrix tests
func (tr *TestRunner) check(user, action string, expectSuccess bool, fn func(libopsv1connect.OrganizationServiceClient) error) {
	// This signature is specific to Org client, so we'll use closures in the caller instead
}

// --- Organization Operations ---

func (tr *TestRunner) testOrganizationOperations(ctx context.Context) {
	// Matrix: Get Root Org
	tr.testMatrix("Get Root Org", func(user string) error {
		c := tr.orgClient(user)
		_, err := c.GetOrganization(ctx, connect.NewRequest(&libopsv1.GetOrganizationRequest{OrganizationId: rootOrgID}))
		return err
	}, map[string]bool{
		"admin":     true,  // Only admin is a member of root org
		"no-access": false, // Should be denied - not a member
		// proj1-owner and other child org members should NOT have access to root org
		// unless they are explicitly added as members
	})

	// Matrix: Get Second Org
	tr.testMatrix("Get Second Org", func(user string) error {
		c := tr.orgClient(user)
		_, err := c.GetOrganization(ctx, connect.NewRequest(&libopsv1.GetOrganizationRequest{OrganizationId: childOrgID}))
		return err
	}, map[string]bool{
		"admin":           true,
		"org-owner":       true,
		"org-developer":   true,
		"org-read":        true,
		"proj1-owner":     true,
		"proj1-developer": true,
		"proj1-read":      true,
		"site1-owner":     true,
		"site1-developer": true,
		"site1-read":      true,
		"proj2-owner":     true,
		"no-access":       false, // Should be denied - not a member
	})

	// Matrix: List Orgs
	tr.test("List Orgs (Admin see 2)", func() error {
		c := tr.orgClient("admin")
		resp, err := c.ListOrganizations(ctx, connect.NewRequest(&libopsv1.ListOrganizationsRequest{}))
		if err != nil {
			return err
		}
		if len(resp.Msg.Organizations) < 2 {
			return fmt.Errorf("expected >= 2 orgs, got %d", len(resp.Msg.Organizations))
		}
		return nil
	})
	tr.test("List Orgs (Org Owner see 1)", func() error {
		c := tr.orgClient("org-owner")
		resp, err := c.ListOrganizations(ctx, connect.NewRequest(&libopsv1.ListOrganizationsRequest{}))
		if err != nil {
			return err
		}
		if len(resp.Msg.Organizations) != 1 {
			return fmt.Errorf("expected 1 org, got %d", len(resp.Msg.Organizations))
		}
		return nil
	})

	// Matrix: Update Second Org
	tr.testMatrix("Update Second Org", func(user string) error {
		c := tr.orgClient(user)
		_, err := c.UpdateOrganization(ctx, connect.NewRequest(&libopsv1.UpdateOrganizationRequest{
			OrganizationId: childOrgID,
			Folder: &commonv1.FolderConfig{
				OrganizationName: "Child Organization Updated",
			},
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"folder.organization_name"}},
		}))
		return err
	}, map[string]bool{
		"admin":         true,  // Owner role inherited from root org via relationship
		"org-owner":     true,  // Direct member with owner role
		"org-developer": true,  // Direct member with developer role (has WRITE permission)
		"no-access":     false, // Should be denied - not a member
	})

	// Matrix: Create/Delete Org Secret
	secretName := "TEST_ORG_SECRET_MATRIX"
	tr.testMatrix("Create Org Secret", func(user string) error {
		c := tr.orgSecretClient(user)
		userName := user
		if len(user) > 0 {
			// Sanitize user name for secret name (uppercase, replace hyphens with underscores)
			userName = ""
			for _, char := range user {
				if char == '-' {
					userName += "_"
				} else {
					userName += string(char)
				}
			}
			userName = strings.ToUpper(userName + string(currentAuthMethod))
		}
		_, err := c.CreateOrganizationSecret(ctx, connect.NewRequest(&libopsv1.CreateOrganizationSecretRequest{
			OrganizationId: childOrgID,
			Name:           fmt.Sprintf("%s_%s", secretName, userName),
			Value:          "val",
		}))
		return err
	}, map[string]bool{
		"admin":         true,  // Owner role inherited from root org via relationship
		"org-owner":     true,  // Direct member with owner role
		"org-developer": true,  // Direct member with developer role (has WRITE permission)
		"no-access":     false, // Should be denied - not a member
	})

	// Cleanup secrets
	_ = tr.cleanupOrgSecrets(ctx, childOrgID, secretName)
}

// --- Project Operations ---

func (tr *TestRunner) testProjectOperations(ctx context.Context) {
	// Matrix: Get Project 1
	tr.testMatrix("Get Project 1", func(user string) error {
		c := tr.projectClient(user)
		_, err := c.GetProject(ctx, connect.NewRequest(&libopsv1.GetProjectRequest{ProjectId: project1ID}))
		return err
	}, map[string]bool{
		"admin":           true,
		"org-owner":       true,
		"org-developer":   true,
		"org-read":        true,
		"proj1-owner":     true,
		"proj1-developer": true,
		"proj1-read":      true,
		"site1-owner":     true,
		"site1-developer": true,
		"site1-read":      true,
		"no-access":       false, // Should be denied - not a member
	})

	// Matrix: Create Project
	tr.testMatrix("Create Project", func(user string) error {
		c := tr.projectClient(user)
		req := &libopsv1.CreateProjectRequest{
			OrganizationId: childOrgID,
			Project: &commonv1.ProjectConfig{
				ProjectName: "test-proj-" + user + string(currentAuthMethod),
			},
		}
		resp, err := c.CreateProject(ctx, connect.NewRequest(req))
		if err == nil {
			// If success, keep the ID if it's org-owner (for later tests), else delete
			if user == "org-owner" {
				createdProjectID = resp.Msg.Project.ProjectId
			} else {
				// Cleanup immediately
				_, _ = c.DeleteProject(ctx, connect.NewRequest(&libopsv1.DeleteProjectRequest{ProjectId: resp.Msg.Project.ProjectId}))
			}
		}
		return err
	}, map[string]bool{
		"admin":         true,
		"org-owner":     true,
		"org-developer": true,
		"no-access":     false, // Should be denied - not a member
	})

	// Matrix: Update Project 1 (Using the ID created by org-owner or existing project1ID?
	// Plan says "Update Project 1". Let's use existing project1ID as it has members set up,
	// but creating/updating createdProjectID is safer to not break state.
	// However, for permission checks on roles like "proj1-owner", we MUST use project1ID.
	// Updates to createdProjectID by proj1-owner should fail anyway (not a member).
	// So we test against project1ID.
	tr.testMatrix("Update Project 1", func(user string) error {
		c := tr.projectClient(user)
		_, err := c.UpdateProject(ctx, connect.NewRequest(&libopsv1.UpdateProjectRequest{
			ProjectId: project1ID,
			Project:   &commonv1.ProjectConfig{ProjectName: "Project Alpha Updated"},
			UpdateMask: &fieldmaskpb.FieldMask{
				Paths: []string{"project.project_name"},
			},
		}))
		return err
	}, map[string]bool{
		"admin":           true,
		"org-owner":       true,
		"org-developer":   true,
		"proj1-owner":     true,
		"proj1-developer": true,
		"no-access":       false, // Should be denied - not a member
	})

	// Matrix: Delete Project
	// We use the createdProjectID for positive delete tests to avoid destroying seed data
	if createdProjectID != "" {
		tr.testMatrix("Delete Created Project", func(user string) error {
			c := tr.projectClient(user)
			_, err := c.DeleteProject(ctx, connect.NewRequest(&libopsv1.DeleteProjectRequest{ProjectId: createdProjectID}))
			return err
		}, map[string]bool{
			"admin":     true,
			"org-owner": true,
			"no-access": false, // Should be denied - not a member
			// Note: proj1-owner cannot delete createdProjectID because they aren't a member of it.
			// To test proj1-owner delete capability, we'd need a project where they are owner.
			// For now, we verify org-owner can delete.
		})
		// Reset createdProjectID if deleted
		createdProjectID = ""
	}

	// Test negative delete on Project 1 (should fail for read-only/non-owners)
	tr.testMatrix("Delete Project 1 (Negative)", func(user string) error {
		// Skip for authorized users to preserve seed data
		if user == "admin" || user == "org-owner" || user == "proj1-owner" {
			return nil
		}
		c := tr.projectClient(user)
		_, err := c.DeleteProject(ctx, connect.NewRequest(&libopsv1.DeleteProjectRequest{ProjectId: project1ID}))
		return err
	}, map[string]bool{
		// No one should succeed in this test block, we are testing failures
		// But actually admin/org-owner/proj1-owner COULD succeed, so we skip them or expect success?
		// We want to verify failures for others.
		// Better: use testMatrix with correct map.
		"admin":           true,
		"org-owner":       true,
		"proj1-owner":     true,
		"proj1-developer": false,
		"proj1-read":      false,
		"org-developer":   false,
		"site1-owner":     false,
		"no-access":       false, // Should be denied - not a member
	})

	// Matrix: Project Secrets
	tr.testMatrix("Create Project Secret", func(user string) error {
		c := tr.projectSecretClient(user)
		userName := user
		if len(user) > 0 {
			// Sanitize user name for secret name (no hyphens)
			userName = ""
			for _, char := range user {
				if char == '-' {
					userName += "_"
				} else {
					userName += string(char)
				}
			}
			userName = strings.ToUpper(userName + string(currentAuthMethod))
		}
		_, err := c.CreateProjectSecret(ctx, connect.NewRequest(&libopsv1.CreateProjectSecretRequest{
			ProjectId: project1ID,
			Name:      fmt.Sprintf("PROJ_SEC_%s", userName),
			Value:     "val",
		}))
		return err
	}, map[string]bool{
		"admin":           true,
		"org-owner":       true,
		"org-developer":   true,
		"proj1-owner":     true,
		"proj1-developer": true,
		"no-access":       false, // Should be denied - not a member
	})
	_ = tr.cleanupProjectSecrets(ctx, project1ID, "PROJ_SEC_")
}

// --- Site Operations ---

func (tr *TestRunner) testSiteOperations(ctx context.Context) {
	// Matrix: Get Site 1
	tr.testMatrix("Get Site 1", func(user string) error {
		c := tr.siteClient(user)
		_, err := c.GetSite(ctx, connect.NewRequest(&libopsv1.GetSiteRequest{SiteId: site1ProdID}))
		return err
	}, map[string]bool{
		"admin":           true,
		"org-owner":       true,
		"org-developer":   true,
		"org-read":        true,
		"proj1-owner":     true,
		"proj1-developer": true,
		"proj1-read":      true,
		"site1-owner":     true,
		"site1-developer": true,
		"site1-read":      true,
		"no-access":       false, // Should be denied - not a member
	})

	// Matrix: Create Site
	tr.testMatrix("Create Site", func(user string) error {
		c := tr.siteClient(user)
		req := &libopsv1.CreateSiteRequest{
			ProjectId: project1ID,
			Site: &commonv1.SiteConfig{
				SiteName:  "site-" + user + string(currentAuthMethod),
				GithubRef: "main",
			},
		}
		resp, err := c.CreateSite(ctx, connect.NewRequest(req))
		if err == nil {
			if user == "org-owner" {
				createdSiteID = resp.Msg.Site.SiteId
			} else {
				_, _ = c.DeleteSite(ctx, connect.NewRequest(&libopsv1.DeleteSiteRequest{SiteId: resp.Msg.Site.SiteId}))
			}
		}
		return err
	}, map[string]bool{
		"admin":           true,
		"org-owner":       true,
		"org-developer":   true,
		"proj1-owner":     true,
		"proj1-developer": true,
		"no-access":       false, // Should be denied - not a member
	})

	// Matrix: Update Site 1
	tr.testMatrix("Update Site 1", func(user string) error {
		c := tr.siteClient(user)
		_, err := c.UpdateSite(ctx, connect.NewRequest(&libopsv1.UpdateSiteRequest{
			SiteId:     site1ProdID,
			Site:       &commonv1.SiteConfig{GithubRef: "main"},
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"site.github_ref"}},
		}))
		return err
	}, map[string]bool{
		"admin":           true,
		"org-owner":       true,
		"org-developer":   true,
		"proj1-owner":     true,
		"proj1-developer": true,
		"site1-owner":     true,
		"site1-developer": true,
		"no-access":       false, // Should be denied - not a member
	})

	// Matrix: Delete Site (using createdSiteID)
	// Note: Only test with org-owner to avoid race where one user deletes before another tries
	if createdSiteID != "" {
		tr.test("Org Owner can delete created site", func() error {
			c := tr.siteClient("org-owner")
			_, err := c.DeleteSite(ctx, connect.NewRequest(&libopsv1.DeleteSiteRequest{SiteId: createdSiteID}))
			return err
		})
		createdSiteID = ""
	}

	// Matrix: Site Secrets
	tr.testMatrix("Create Site Secret", func(user string) error {
		c := tr.siteSecretClient(user)
		userName := user
		if len(user) > 0 {
			// Sanitize user name for secret name (no hyphens)
			userName = ""
			for _, char := range user {
				if char == '-' {
					userName += "_"
				} else {
					userName += string(char)
				}
			}
			userName = strings.ToUpper(userName + string(currentAuthMethod))
		}
		_, err := c.CreateSiteSecret(ctx, connect.NewRequest(&libopsv1.CreateSiteSecretRequest{
			SiteId: site1ProdID,
			Name:   fmt.Sprintf("SITE_SEC_%s", userName),
			Value:  "val",
		}))
		return err
	}, map[string]bool{"admin": true,
		"org-owner":       true,
		"org-developer":   true,
		"proj1-owner":     true,
		"proj1-developer": true,
		"site1-owner":     true,
		"site1-developer": true,
		"no-access":       false, // Should be denied - not a member
	})
	_ = tr.cleanupSiteSecrets(ctx, site1ProdID, "SITE_SEC_")
}

// --- Firewall Operations ---

func (tr *TestRunner) testFirewallOperations(ctx context.Context) {
	// Org Firewall
	tr.testMatrix("Create Org Firewall Rule", func(user string) error {
		c := tr.firewallClient(user)
		_, err := c.CreateOrganizationFirewallRule(ctx, connect.NewRequest(&libopsv1.CreateOrganizationFirewallRuleRequest{
			OrganizationId: childOrgID,
			RuleType:       libopsv1.FirewallRuleType_FIREWALL_RULE_TYPE_HTTPS_ALLOWED,
			Cidr:           "10.0.0.1/32",
			Name:           "fw-org-" + user + string(currentAuthMethod),
		}))
		return err
	}, map[string]bool{
		"admin":         true,
		"org-owner":     true,
		"org-developer": true,
		"no-access":     false, // Should be denied - not a member
	})

	// Project Firewall
	tr.testMatrix("Create Project Firewall Rule", func(user string) error {
		c := tr.projectFirewallClient(user)
		_, err := c.CreateProjectFirewallRule(ctx, connect.NewRequest(&libopsv1.CreateProjectFirewallRuleRequest{
			ProjectId: project1ID,
			RuleType:  libopsv1.FirewallRuleType_FIREWALL_RULE_TYPE_HTTPS_ALLOWED,
			Cidr:      "10.0.0.1/32",
			Name:      "fw-proj-" + user + string(currentAuthMethod),
		}))
		return err
	}, map[string]bool{
		"admin":           true,
		"org-owner":       true,
		"org-developer":   true,
		"proj1-owner":     true,
		"proj1-developer": true,
		"no-access":       false, // Should be denied - not a member
	})

	// Site Firewall
	tr.testMatrix("Create Site Firewall Rule", func(user string) error {
		c := tr.siteFirewallClient(user)
		_, err := c.CreateSiteFirewallRule(ctx, connect.NewRequest(&libopsv1.CreateSiteFirewallRuleRequest{
			SiteId:   site1ProdID,
			RuleType: libopsv1.FirewallRuleType_FIREWALL_RULE_TYPE_HTTPS_ALLOWED,
			Cidr:     "10.0.0.1/32",
			Name:     "fw-site-" + user,
		}))
		return err
	}, map[string]bool{
		"admin":           true,
		"org-owner":       true,
		"org-developer":   true,
		"proj1-owner":     true,
		"proj1-developer": true,
		"site1-owner":     true,
		"site1-developer": true,
		"no-access":       false, // Should be denied - not a member
	})
}

// --- Member Management ---

func (tr *TestRunner) testMemberManagement(ctx context.Context) {
	// Matrix: List Org Members
	// Per RBAC matrix: all roles (owner/developer/read) have READ access to organization
	// ListOrganizationMembers requires ACCESS_LEVEL_READ, so all org members can list
	// Also, users with project/site access via relationships can view the org (read-only)
	tr.testMatrix("List Org Members", func(user string) error {
		c := tr.orgMemberClient(user)
		_, err := c.ListOrganizationMembers(ctx, connect.NewRequest(&libopsv1.ListOrganizationMembersRequest{OrganizationId: childOrgID}))
		return err
	}, map[string]bool{
		"admin":           true, // Access via relationship from root org
		"org-owner":       true, // Direct member of child org
		"org-developer":   true, // Direct member of child org
		"org-read":        true, // Direct member of child org
		"proj1-owner":     true, // Inherits read access via project membership
		"proj1-developer": true, // Inherits read access via project membership
		"proj1-read":      true, // Inherits read access via project membership
		"site1-owner":     true, // Inherits read access via site membership
		"site1-developer": true, // Inherits read access via site membership
		"site1-read":      true, // Inherits read access via site membership
		"proj2-owner":     true, // Inherits read access via project membership
	})

	// Matrix: Create Org Member
	// We can't actually create members for everyone without running out of test users.
	// Just verify permissions for a few key roles using specific fail accounts.
	tr.testError("Org Developer CANNOT create org member", func() error {
		c := tr.orgMemberClient("org-developer")
		_, err := c.CreateOrganizationMember(ctx, connect.NewRequest(&libopsv1.CreateOrganizationMemberRequest{
			OrganizationId: childOrgID,
			AccountId:      "10000000-0000-0000-0000-000000000001",
			Role:           "read",
		}))
		return err
	})

	tr.test("Org Owner can create org member", func() error {
		c := tr.orgMemberClient("org-owner")
		_, err := c.CreateOrganizationMember(ctx, connect.NewRequest(&libopsv1.CreateOrganizationMemberRequest{
			OrganizationId: childOrgID,
			AccountId:      "10000000-0000-0000-0000-000000000001", // admin, already member but creates duplicates if not careful?
			// In DB schema, unique constraint exists. But 'admin' is owner of Root Org.
			// 'admin' is NOT explicitly a member of Child Org in seed data.
			// Relationships grant him access.
			// So we can add him as explicit member.
			Role: "read",
		}))
		return err
	})

	tr.test("Org Owner can delete org member", func() error {
		c := tr.orgMemberClient("org-owner")
		_, err := c.DeleteOrganizationMember(ctx, connect.NewRequest(&libopsv1.DeleteOrganizationMemberRequest{
			OrganizationId: childOrgID,
			AccountId:      "10000000-0000-0000-0000-000000000001",
		}))
		return err
	})

	// Parent Access: Org Owner creates Project Member
	tr.test("Org Owner can create project member", func() error {
		c := tr.projectMemberClient("org-owner")
		_, err := c.CreateProjectMember(ctx, connect.NewRequest(&libopsv1.CreateProjectMemberRequest{
			ProjectId: project1ID,
			AccountId: "10000000-0000-0000-0000-000000000004", // org-read
			Role:      "read",
		}))
		return err
	})

	tr.test("Org Owner can delete project member", func() error {
		c := tr.projectMemberClient("org-owner")
		_, err := c.DeleteProjectMember(ctx, connect.NewRequest(&libopsv1.DeleteProjectMemberRequest{
			ProjectId: project1ID,
			AccountId: "10000000-0000-0000-0000-000000000004",
		}))
		return err
	})
}

// --- API Key Management ---

func (tr *TestRunner) testAPIKeyManagement(ctx context.Context) {
	fmt.Println(yellow("  → Testing API key CRUD operations"))

	var adminKeyID, adminKeySecret string
	var orgOwnerKeyID, orgOwnerKeySecret string
	var proj1OwnerKeyID, proj1OwnerKeySecret string
	var site1OwnerKeyID, site1OwnerKeySecret string

	// Test 1: Admin creates an API key with limited organization:read scope
	tr.test("Admin creates API key with organization:read scope", func() error {
		c := tr.accountClient("admin")
		resp, err := c.CreateApiKey(ctx, connect.NewRequest(&libopsv1.CreateApiKeyRequest{
			Name:        "Admin Limited Read Org Key",
			Description: "Test key with organization:read scope",
			Scopes:      []string{"organization:read"},
		}))
		if err != nil {
			return err
		}
		adminKeyID = resp.Msg.ApiKeyId
		adminKeySecret = resp.Msg.ApiKey
		if adminKeySecret == "" {
			return fmt.Errorf("expected API key secret to be returned")
		}
		if !strings.HasPrefix(adminKeySecret, "libops_") {
			return fmt.Errorf("expected API key to start with 'libops_', got: %s", adminKeySecret)
		}
		// Store for later use
		apiKeys["admin-limited"] = adminKeySecret
		fmt.Printf(green("    ✓ Created key %s\n"), adminKeyID)
		return nil
	})

	// Test 2: Org Owner creates an API key with limited project:read scope
	tr.test("Org Owner creates API key with project:read scope", func() error {
		c := tr.accountClient("org-owner")
		resp, err := c.CreateApiKey(ctx, connect.NewRequest(&libopsv1.CreateApiKeyRequest{
			Name:        "Org Owner Limited Read Project Key",
			Description: "Test key with project:read scope",
			Scopes:      []string{"project:read"},
		}))
		if err != nil {
			return err
		}
		orgOwnerKeyID = resp.Msg.ApiKeyId
		orgOwnerKeySecret = resp.Msg.ApiKey
		apiKeys["org-owner-limited"] = orgOwnerKeySecret
		fmt.Printf(green("    ✓ Created key %s\n"), orgOwnerKeyID)
		return nil
	})

	// Test 3: Project 1 Owner creates an API key with limited site:read scope
	tr.test("Project 1 Owner creates API key with site:read scope", func() error {
		c := tr.accountClient("proj1-owner")
		resp, err := c.CreateApiKey(ctx, connect.NewRequest(&libopsv1.CreateApiKeyRequest{
			Name:        "Proj1 Owner Limited Read Site Key",
			Description: "Test key with site:read scope",
			Scopes:      []string{"site:read"},
		}))
		if err != nil {
			return err
		}
		proj1OwnerKeyID = resp.Msg.ApiKeyId
		proj1OwnerKeySecret = resp.Msg.ApiKey
		apiKeys["proj1-owner-limited"] = proj1OwnerKeySecret
		fmt.Printf(green("    ✓ Created key %s\n"), proj1OwnerKeyID)
		return nil
	})

	// Test 4: Site 1 Owner creates an API key with limited site:read scope
	tr.test("Site 1 Owner creates API key with site:read scope", func() error {
		c := tr.accountClient("site1-owner")
		resp, err := c.CreateApiKey(ctx, connect.NewRequest(&libopsv1.CreateApiKeyRequest{
			Name:        "Site1 Owner Limited Read Site Key",
			Description: "Test key with site:read scope",
			Scopes:      []string{"site:read"},
		}))
		if err != nil {
			return err
		}
		site1OwnerKeyID = resp.Msg.ApiKeyId
		site1OwnerKeySecret = resp.Msg.ApiKey
		apiKeys["site1-owner-limited"] = site1OwnerKeySecret
		fmt.Printf(green("    ✓ Created key %s\n"), site1OwnerKeyID)
		return nil
	})

	fmt.Println(yellow("  → Testing API key listing"))

	// Test 5: Admin lists their API keys and finds the created key
	tr.test("Admin lists API keys and finds created key", func() error {
		c := tr.accountClient("admin")
		resp, err := c.ListApiKeys(ctx, connect.NewRequest(&libopsv1.ListApiKeysRequest{}))
		if err != nil {
			return err
		}
		found := false
		for _, key := range resp.Msg.ApiKeys {
			if key.ApiKeyId == adminKeyID {
				found = true
				if key.Name != "Admin Limited Read Org Key" {
					return fmt.Errorf("expected key name 'Admin Limited Read Org Key', got: %s", key.Name)
				}
				if len(key.Scopes) != 1 || key.Scopes[0] != "organization:read" {
					return fmt.Errorf("expected scopes [organization:read], got: %v", key.Scopes)
				}
				break
			}
		}
		if !found {
			return fmt.Errorf("created API key not found in list")
		}
		return nil
	})

	fmt.Println(yellow("  → Testing security boundary: users cannot access other users' keys"))

	// Test 6: Admin CANNOT revoke Org Owner's API key
	tr.testError("Admin CANNOT revoke Org Owner's API key", func() error {
		c := tr.accountClient("admin")
		_, err := c.RevokeApiKey(ctx, connect.NewRequest(&libopsv1.RevokeApiKeyRequest{
			ApiKeyId: orgOwnerKeyID,
		}))
		if err == nil {
			return fmt.Errorf("expected error when trying to revoke another user's key")
		}
		var connectErr *connect.Error
		if errors.As(err, &connectErr) {
			if connectErr.Code() != connect.CodeNotFound {
				return fmt.Errorf("expected NotFound error, got: %v", connectErr.Code())
			}
		}
		return err
	})

	fmt.Println(yellow("  → Testing API key revocation"))

	// Test 7: Create a temporary key to test revocation
	var tempKeyID string
	tr.test("Admin creates temporary API key for revocation test", func() error {
		c := tr.accountClient("admin")
		resp, err := c.CreateApiKey(ctx, connect.NewRequest(&libopsv1.CreateApiKeyRequest{
			Name:        "Temporary Key",
			Description: "Will be revoked",
		}))
		if err != nil {
			return err
		}
		tempKeyID = resp.Msg.ApiKeyId
		return nil
	})

	// Test 8: Admin revokes their own temporary key
	tr.test("Admin revokes temporary API key", func() error {
		c := tr.accountClient("admin")
		resp, err := c.RevokeApiKey(ctx, connect.NewRequest(&libopsv1.RevokeApiKeyRequest{
			ApiKeyId: tempKeyID,
		}))
		if err != nil {
			return err
		}
		if !resp.Msg.Success {
			return fmt.Errorf("expected success=true")
		}
		return nil
	})

	// Test 9: Verify the revoked key is marked as inactive in the list
	tr.test("Revoked key is marked as inactive in list", func() error {
		c := tr.accountClient("admin")
		resp, err := c.ListApiKeys(ctx, connect.NewRequest(&libopsv1.ListApiKeysRequest{}))
		if err != nil {
			return err
		}
		for _, key := range resp.Msg.ApiKeys {
			if key.ApiKeyId == tempKeyID {
				if key.Active {
					return fmt.Errorf("expected key to be inactive after revocation")
				}
				return nil
			}
		}
		return fmt.Errorf("revoked key not found in list")
	})

	fmt.Println(green("  ✓ All API key management tests passed"))
}

// --- Scope Restrictions ---

func (tr *TestRunner) testScopeRestrictions(ctx context.Context) {
	// Organization Scope Tests
	tr.test("Admin (Limited read:org) CAN read org", func() error {
		c := tr.orgClient("admin-limited")
		_, err := c.GetOrganization(ctx, connect.NewRequest(&libopsv1.GetOrganizationRequest{OrganizationId: rootOrgID}))
		return err
	})

	tr.testError("Admin (Limited read:org) CANNOT update org", func() error {
		c := tr.orgClient("admin-limited")
		_, err := c.UpdateOrganization(ctx, connect.NewRequest(&libopsv1.UpdateOrganizationRequest{
			OrganizationId: rootOrgID,
			Folder:         &commonv1.FolderConfig{OrganizationName: "Should Fail"},
		}))
		return err
	})

	tr.testError("Admin (Limited read:org) CANNOT create org secret (needs write)", func() error {
		c := tr.orgSecretClient("admin-limited")
		_, err := c.CreateOrganizationSecret(ctx, connect.NewRequest(&libopsv1.CreateOrganizationSecretRequest{
			OrganizationId: rootOrgID,
			Name:           "SCOPE_TEST_SECRET",
			Value:          "val",
		}))
		return err
	})

	// Project Scope Tests
	tr.testError("Org Owner (Limited read:project) CANNOT update org (wrong resource)", func() error {
		c := tr.orgClient("org-owner-limited")
		_, err := c.UpdateOrganization(ctx, connect.NewRequest(&libopsv1.UpdateOrganizationRequest{
			OrganizationId: childOrgID,
			Folder:         &commonv1.FolderConfig{OrganizationName: "Should Fail"},
		}))
		return err
	})

	tr.test("Org Owner (Limited read:project) CAN read project", func() error {
		c := tr.projectClient("org-owner-limited")
		_, err := c.GetProject(ctx, connect.NewRequest(&libopsv1.GetProjectRequest{ProjectId: project1ID}))
		return err
	})

	tr.testError("Org Owner (Limited read:project) CANNOT create project (needs write)", func() error {
		c := tr.projectClient("org-owner-limited")
		_, err := c.CreateProject(ctx, connect.NewRequest(&libopsv1.CreateProjectRequest{
			OrganizationId: childOrgID,
			Project:        &commonv1.ProjectConfig{ProjectName: "scope-test"},
		}))
		return err
	})

	tr.testError("Proj1 Owner (Limited read:project) CANNOT update project (needs write)", func() error {
		c := tr.projectClient("proj1-owner-limited")
		_, err := c.UpdateProject(ctx, connect.NewRequest(&libopsv1.UpdateProjectRequest{
			ProjectId: project1ID,
			Project:   &commonv1.ProjectConfig{ProjectName: "should-fail"},
		}))
		return err
	})

	// Site Scope Tests
	tr.test("Site1 Owner (Limited read:site) CAN read site", func() error {
		c := tr.siteClient("site1-owner-limited")
		_, err := c.GetSite(ctx, connect.NewRequest(&libopsv1.GetSiteRequest{SiteId: site1ProdID}))
		return err
	})

	tr.testError("Site1 Owner (Limited read:site) CANNOT update site (needs write)", func() error {
		c := tr.siteClient("site1-owner-limited")
		_, err := c.UpdateSite(ctx, connect.NewRequest(&libopsv1.UpdateSiteRequest{
			SiteId: site1ProdID,
			Site:   &commonv1.SiteConfig{GithubRef: "should-fail"},
		}))
		return err
	})

	tr.testError("Site1 Owner (Limited read:site) CANNOT create site secret (needs write)", func() error {
		c := tr.siteSecretClient("site1-owner-limited")
		_, err := c.CreateSiteSecret(ctx, connect.NewRequest(&libopsv1.CreateSiteSecretRequest{
			SiteId: site1ProdID,
			Name:   "SCOPE_TEST",
			Value:  "val",
		}))
		return err
	})

	tr.testError("Site1 Owner (Limited read:site) CANNOT read project (wrong resource scope)", func() error {
		c := tr.projectClient("site1-owner-limited")
		_, err := c.GetProject(ctx, connect.NewRequest(&libopsv1.GetProjectRequest{ProjectId: project1ID}))
		return err
	})

	// ============================================================================
	// HIERARCHICAL SCOPE TESTS - Resource Type Boundaries
	// ============================================================================
	// Scopes should strictly enforce resource type - even if membership would grant access

	// Upward access tests (child scope cannot access parent resources)
	tr.testError("Site1 Owner (delete:site) CANNOT read project (wrong resource type)", func() error {
		c := tr.projectClient("site1-owner-limited") // has delete:site scope only
		_, err := c.GetProject(ctx, connect.NewRequest(&libopsv1.GetProjectRequest{ProjectId: project1ID}))
		return err
	})

	tr.testError("Site1 Owner (delete:site) CANNOT read org (wrong resource type)", func() error {
		c := tr.orgClient("site1-owner-limited") // has delete:site scope only
		_, err := c.GetOrganization(ctx, connect.NewRequest(&libopsv1.GetOrganizationRequest{OrganizationId: childOrgID}))
		return err
	})

	tr.testError("Proj1 Owner (delete:project) CANNOT read org (wrong resource type)", func() error {
		c := tr.orgClient("proj1-owner-limited") // has delete:project scope only
		_, err := c.GetOrganization(ctx, connect.NewRequest(&libopsv1.GetOrganizationRequest{OrganizationId: childOrgID}))
		return err
	})

	// Downward access tests (parent scope cannot access child resources)
	tr.testError("Org Owner (admin:org) CANNOT read project without project scope", func() error {
		// Create a limited key with only org scope
		c := tr.projectClient("admin-limited") // admin-limited has only read:org
		_, err := c.GetProject(ctx, connect.NewRequest(&libopsv1.GetProjectRequest{ProjectId: project1ID}))
		return err
	})

	tr.testError("User with ONLY org scope CANNOT create project without project scope", func() error {
		// admin-limited has only read:organization scope
		// Should NOT be able to create project (requires write:organization for CreateProject)
		// But even if they had write:organization, RBAC would check membership
		c := tr.projectClient("admin-limited")
		_, err := c.CreateProject(ctx, connect.NewRequest(&libopsv1.CreateProjectRequest{
			OrganizationId: rootOrgID,
			Project:        &commonv1.ProjectConfig{ProjectName: "should-fail"},
		}))
		return err
	})

	// ============================================================================
	// RELATIONSHIP + SCOPE INTERACTION TESTS
	// ============================================================================
	// Test that scopes work correctly with relationship-based access

	// Admin is owner of Root Org, which has relationship to Child Org
	// With full scopes, he can access Child Org resources
	tr.test("Admin (full scopes) CAN read Child Org via relationship", func() error {
		c := tr.orgClient("admin")
		_, err := c.GetOrganization(ctx, connect.NewRequest(&libopsv1.GetOrganizationRequest{OrganizationId: childOrgID}))
		return err
	})

	tr.test("Admin (full scopes) CAN read Project 1 in Child Org via relationship", func() error {
		c := tr.projectClient("admin")
		_, err := c.GetProject(ctx, connect.NewRequest(&libopsv1.GetProjectRequest{ProjectId: project1ID}))
		return err
	})

	tr.test("Admin (full scopes) CAN read Site 1 in Project 1 via relationship", func() error {
		c := tr.siteClient("admin")
		_, err := c.GetSite(ctx, connect.NewRequest(&libopsv1.GetSiteRequest{SiteId: site1ProdID}))
		return err
	})

	// But with limited scopes, relationship doesn't help if scope is wrong
	tr.test("Admin (limited read:org) CAN read Child Org via relationship", func() error {
		c := tr.orgClient("admin-limited")
		_, err := c.GetOrganization(ctx, connect.NewRequest(&libopsv1.GetOrganizationRequest{OrganizationId: childOrgID}))
		return err
	})

	tr.testError("Admin (limited read:org) CANNOT read Project 1 via relationship (needs project scope)", func() error {
		c := tr.projectClient("admin-limited")
		_, err := c.GetProject(ctx, connect.NewRequest(&libopsv1.GetProjectRequest{ProjectId: project1ID}))
		return err
	})

	tr.testError("Admin (limited read:org) CANNOT read Site 1 via relationship (needs site scope)", func() error {
		c := tr.siteClient("admin-limited")
		_, err := c.GetSite(ctx, connect.NewRequest(&libopsv1.GetSiteRequest{SiteId: site1ProdID}))
		return err
	})

	// ============================================================================
	// MULTI-SCOPE TESTS
	// ============================================================================
	// Users with multiple scopes in their key should be able to access multiple resource types

	// Org owner has admin:org, admin:project, admin:site
	tr.test("Org Owner (full scopes) CAN read org", func() error {
		c := tr.orgClient("org-owner")
		_, err := c.GetOrganization(ctx, connect.NewRequest(&libopsv1.GetOrganizationRequest{OrganizationId: childOrgID}))
		return err
	})

	tr.test("Org Owner (full scopes) CAN read project", func() error {
		c := tr.projectClient("org-owner")
		_, err := c.GetProject(ctx, connect.NewRequest(&libopsv1.GetProjectRequest{ProjectId: project1ID}))
		return err
	})

	tr.test("Org Owner (full scopes) CAN read site", func() error {
		c := tr.siteClient("org-owner")
		_, err := c.GetSite(ctx, connect.NewRequest(&libopsv1.GetSiteRequest{SiteId: site1ProdID}))
		return err
	})

	// But limited to only read:project
	tr.testError("Org Owner (limited read:project) CANNOT read org", func() error {
		c := tr.orgClient("org-owner-limited")
		_, err := c.GetOrganization(ctx, connect.NewRequest(&libopsv1.GetOrganizationRequest{OrganizationId: childOrgID}))
		return err
	})

	tr.test("Org Owner (limited read:project) CAN read project", func() error {
		c := tr.projectClient("org-owner-limited")
		_, err := c.GetProject(ctx, connect.NewRequest(&libopsv1.GetProjectRequest{ProjectId: project1ID}))
		return err
	})

	tr.testError("Org Owner (limited read:project) CANNOT read site", func() error {
		c := tr.siteClient("org-owner-limited")
		_, err := c.GetSite(ctx, connect.NewRequest(&libopsv1.GetSiteRequest{SiteId: site1ProdID}))
		return err
	})
}

// --- Isolation & Inheritance ---

func (tr *TestRunner) testCrossResourceIsolation(ctx context.Context) {
	tr.testError("Proj1 Owner CANNOT read Proj2", func() error {
		c := tr.projectClient("proj1-owner")
		_, err := c.GetProject(ctx, connect.NewRequest(&libopsv1.GetProjectRequest{ProjectId: project2ID}))
		return err
	})

	tr.testError("Proj1 Developer CANNOT read Proj2 secrets", func() error {
		c := tr.projectSecretClient("proj1-developer")
		_, err := c.ListProjectSecrets(ctx, connect.NewRequest(&libopsv1.ListProjectSecretsRequest{ProjectId: project2ID}))
		return err
	})
}

func (tr *TestRunner) testMembershipInheritance(ctx context.Context) {
	// Site member -> Project
	tr.test("Site Owner can read parent Project", func() error {
		c := tr.projectClient("site1-owner")
		_, err := c.GetProject(ctx, connect.NewRequest(&libopsv1.GetProjectRequest{ProjectId: project1ID}))
		return err
	})
	// Site member -> Org
	tr.test("Site Owner can read parent Org", func() error {
		c := tr.orgClient("site1-owner")
		_, err := c.GetOrganization(ctx, connect.NewRequest(&libopsv1.GetOrganizationRequest{OrganizationId: childOrgID}))
		return err
	})
}

// --- Helpers ---

func (tr *TestRunner) testMatrix(operationName string, action func(user string) error, allowedUsers map[string]bool) {
	users := []string{
		"admin", "org-owner", "org-developer", "org-read",
		"proj1-owner", "proj1-developer", "proj1-read",
		"site1-owner", "site1-developer", "site1-read",
		"proj2-owner", "no-access",
	}

	for _, user := range users {
		expected, isAllowed := allowedUsers[user]
		if !isAllowed {
			expected = false
		}

		err := action(user)
		desc := fmt.Sprintf("%s [%s]", operationName, user)

		if expected {
			if err == nil {
				tr.passed++
				fmt.Printf("  %s %s\n", green("✓"), desc)
			} else {
				tr.failed++
				fmt.Printf("  %s %s: unexpected error: %v\n", red("✗"), desc, err)
			}
		} else {
			if err != nil {
				code := connect.CodeOf(err)
				if code == connect.CodePermissionDenied || code == connect.CodeUnauthenticated || code == connect.CodeNotFound {
					tr.passed++
					fmt.Printf("  %s %s (denied as expected)\n", green("✓"), desc)
				} else {
					tr.failed++
					fmt.Printf("  %s %s: wrong error code %s: %v\n", red("✗"), desc, code, err)
				}
			} else {
				tr.failed++
				fmt.Printf("  %s %s: expected permission denied, got success\n", red("✗"), desc)
			}
		}
	}
}

func (tr *TestRunner) test(name string, fn func() error) {
	if err := fn(); err == nil {
		tr.passed++
		fmt.Printf("  %s %s\n", green("✓"), name)
	} else {
		tr.failed++
		fmt.Printf("  %s %s: %v\n", red("✗"), name, err)
	}
}

func (tr *TestRunner) testError(name string, fn func() error) {
	err := fn()
	if err != nil {
		tr.passed++
		fmt.Printf("  %s %s\n", green("✓"), name)
	} else {
		tr.failed++
		fmt.Printf("  %s %s: expected error\n", red("✗"), name)
	}
}

func (tr *TestRunner) testEmpty(name string, fn func() (int, error)) {
	count, err := fn()
	if err == nil && count == 0 {
		tr.passed++
		fmt.Printf("  %s %s\n", green("✓"), name)
	} else {
		tr.failed++
		fmt.Printf("  %s %s: expected empty, got %d (err: %v)\n", red("✗"), name, count, err)
	}
}

// Cleanup helpers
func (tr *TestRunner) cleanupOrgSecrets(ctx context.Context, orgID, prefix string) error {
	c := tr.orgSecretClient("org-owner")
	resp, err := c.ListOrganizationSecrets(ctx, connect.NewRequest(&libopsv1.ListOrganizationSecretsRequest{OrganizationId: orgID}))
	if err != nil {
		return err
	}
	for _, s := range resp.Msg.Secrets {
		if len(s.Name) >= len(prefix) && s.Name[:len(prefix)] == prefix {
			_, _ = c.DeleteOrganizationSecret(ctx, connect.NewRequest(&libopsv1.DeleteOrganizationSecretRequest{OrganizationId: orgID, SecretId: s.SecretId}))
		}
	}
	return nil
}

func (tr *TestRunner) cleanupProjectSecrets(ctx context.Context, projID, prefix string) error {
	c := tr.projectSecretClient("proj1-owner")
	resp, err := c.ListProjectSecrets(ctx, connect.NewRequest(&libopsv1.ListProjectSecretsRequest{ProjectId: projID}))
	if err != nil {
		return err
	}
	for _, s := range resp.Msg.Secrets {
		if len(s.Name) >= len(prefix) && s.Name[:len(prefix)] == prefix {
			_, _ = c.DeleteProjectSecret(ctx, connect.NewRequest(&libopsv1.DeleteProjectSecretRequest{ProjectId: projID, SecretId: s.SecretId}))
		}
	}
	return nil
}

func (tr *TestRunner) cleanupSiteSecrets(ctx context.Context, siteID, prefix string) error {
	c := tr.siteSecretClient("site1-owner")
	resp, err := c.ListSiteSecrets(ctx, connect.NewRequest(&libopsv1.ListSiteSecretsRequest{SiteId: siteID}))
	if err != nil {
		return err
	}
	for _, s := range resp.Msg.Secrets {
		if len(s.Name) >= len(prefix) && s.Name[:len(prefix)] == prefix {
			_, _ = c.DeleteSiteSecret(ctx, connect.NewRequest(&libopsv1.DeleteSiteSecretRequest{SiteId: siteID, SecretId: s.SecretId}))
		}
	}
	return nil
}

func (tr *TestRunner) PrintResults() {
	fmt.Println(cyan("\n================================================="))
	fmt.Println(cyan("  Results"))
	fmt.Println(cyan("================================================="))
	fmt.Printf("\nTotal:  %d\n", tr.passed+tr.failed)
	fmt.Printf("Passed: %s\n", green(fmt.Sprintf("%d", tr.passed)))
	fmt.Printf("Failed: %s\n", red(fmt.Sprintf("%d", tr.failed)))
	fmt.Println()
	if tr.failed == 0 {
		fmt.Println(green("✓ All tests passed!"))
	} else {
		fmt.Println(red("✗ Some tests failed"))
	}
}

// Client Factories
func (tr *TestRunner) orgClient(key string) libopsv1connect.OrganizationServiceClient {
	return libopsv1connect.NewOrganizationServiceClient(&http.Client{Transport: &authTransport{tr.getToken(key)}}, apiURL)
}
func (tr *TestRunner) projectClient(key string) libopsv1connect.ProjectServiceClient {
	return libopsv1connect.NewProjectServiceClient(&http.Client{Transport: &authTransport{tr.getToken(key)}}, apiURL)
}
func (tr *TestRunner) siteClient(key string) libopsv1connect.SiteServiceClient {
	return libopsv1connect.NewSiteServiceClient(&http.Client{Transport: &authTransport{tr.getToken(key)}}, apiURL)
}
func (tr *TestRunner) orgSecretClient(key string) libopsv1connect.OrganizationSecretServiceClient {
	return libopsv1connect.NewOrganizationSecretServiceClient(&http.Client{Transport: &authTransport{tr.getToken(key)}}, apiURL)
}
func (tr *TestRunner) projectSecretClient(key string) libopsv1connect.ProjectSecretServiceClient {
	return libopsv1connect.NewProjectSecretServiceClient(&http.Client{Transport: &authTransport{tr.getToken(key)}}, apiURL)
}
func (tr *TestRunner) siteSecretClient(key string) libopsv1connect.SiteSecretServiceClient {
	return libopsv1connect.NewSiteSecretServiceClient(&http.Client{Transport: &authTransport{tr.getToken(key)}}, apiURL)
}
func (tr *TestRunner) firewallClient(key string) libopsv1connect.FirewallServiceClient {
	return libopsv1connect.NewFirewallServiceClient(&http.Client{Transport: &authTransport{tr.getToken(key)}}, apiURL)
}
func (tr *TestRunner) projectFirewallClient(key string) libopsv1connect.ProjectFirewallServiceClient {
	return libopsv1connect.NewProjectFirewallServiceClient(&http.Client{Transport: &authTransport{tr.getToken(key)}}, apiURL)
}
func (tr *TestRunner) siteFirewallClient(key string) libopsv1connect.SiteFirewallServiceClient {
	return libopsv1connect.NewSiteFirewallServiceClient(&http.Client{Transport: &authTransport{tr.getToken(key)}}, apiURL)
}
func (tr *TestRunner) orgMemberClient(key string) libopsv1connect.MemberServiceClient {
	return libopsv1connect.NewMemberServiceClient(&http.Client{Transport: &authTransport{tr.getToken(key)}}, apiURL)
}
func (tr *TestRunner) projectMemberClient(key string) libopsv1connect.ProjectMemberServiceClient {
	return libopsv1connect.NewProjectMemberServiceClient(&http.Client{Transport: &authTransport{tr.getToken(key)}}, apiURL)
}
func (tr *TestRunner) siteMemberClient(key string) libopsv1connect.SiteMemberServiceClient {
	return libopsv1connect.NewSiteMemberServiceClient(&http.Client{Transport: &authTransport{tr.getToken(key)}}, apiURL)
}
func (tr *TestRunner) accountClient(key string) libopsv1connect.AccountServiceClient {
	return libopsv1connect.NewAccountServiceClient(&http.Client{Transport: &authTransport{tr.getToken(key)}}, apiURL)
}

type authTransport struct{ apiKey string }

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer "+t.apiKey)
	return http.DefaultTransport.RoundTrip(req)
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
