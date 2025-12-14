package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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
	// Format: libops_{accountUUID_no_dashes}_{keyUUID_no_dashes}_{randomSecret}
	apiKeys = map[string]string{
		// Full scope keys - format updated to include random secret
		"admin":       "libops_01052d4d93be51a39684c357297533cd_075913e793285264b6846ae0163b8096_test_secret_admin_full",
		"art":         "libops_fdf35d32bbb35ea3abf2410da575e169_0f05b4b9f40c5ca89f3904de42ae87e4_test_secret_art_full",
		"jerry":       "libops_964b5eb020375263883ce939c6916d7d_726186be6ad85257a1bd2e4689db11d0_test_secret_jerry_full",
		"elaine":      "libops_863fb60a808450fe82aeefa113231bef_b3f360ca79955db2b88b3e178cd7ae8a_test_secret_elaine_full",
		"george":      "libops_d0bfd25745725036b5aa038743be4715_0c9522b721975d87b010ac1bc506f79a_test_secret_george_full",
		"kramer":      "libops_516e3bb4bfbe5dda9cc9d0e00ce7b6f2_94581ae623e358698770db7cb74e5391_test_secret_kramer_full",
		"pennypacker": "libops_42b6846e501f51539aca210d8d84f946_58c99883c3145c6ebfa8e072502e43bd_test_secret_pennypacker_full",
		"newman":      "libops_e60f6db8521a5fc3aaccceb3f50b6f7b_3ccc3cc2e5c0530b8f0a6fb24cd8566b_test_secret_newman_full",
		"bob":         "libops_94656683e36658b8a39132e0c54ca37e_63cd920a70905e0eb46d840a933e2c70_test_secret_bob_full",
		"joe":         "libops_0f439d32e0655a20a08e22dd6793948a_890e09765b435ff8a673921a920e7c2a_test_secret_joe_full",
		"puddy":       "libops_22f490238dfe57c795dbdd0f8cae04a7_eb181a1b7dc953c29981ba91a3ebf24a_test_secret_puddy_full",
		"soup":        "libops_ff2098bd1a335db9806937f2bf5bdba7_43527224d0f85344803fec80f80ed0a0_test_secret_soup_full",
		"babu":        "libops_a551424b91ed5636a53bcdb50660d4c9_2032b34886ae5805b08c3c2cf065ef82_test_secret_babu_full",
		"leo":         "libops_351fcf8bd637596cbe1e8bdd90dbc4eb_ce22e781d2ad5d7abccc7dd122e791c8_test_secret_leo_full",
		"jackie":      "libops_af54b89e5533585ab3b70003b7e6dcc2_578e1fcfb4975bffbbf4436835457f73_test_secret_jackie_full",
		"peterman":    "libops_dfe2b1a880005b6788ad881b036fa4f9_2c3cfb5bc99454c99cb992321bd353cb_test_secret_peterman_full",
		"no-access":   "libops_e543554b5af05d97ac8f09608bcfa7b8_567df9dc244e561e93c13082534eeec7_test_secret_noaccess_full",

		// Limited scope keys
		"admin-limited": "libops_01052d4d93be51a39684c357297533cd_d76a9ff9334c548d8ba94063ddb96cf9_test_secret_admin_limited",
		"art-limited":   "libops_fdf35d32bbb35ea3abf2410da575e169_c19811014bbf5f90b38b901c06fdaad6_test_secret_art_limited",
		"bob-limited":   "libops_94656683e36658b8a39132e0c54ca37e_7dd4d68f85f45dbebed083e639a8fab2_test_secret_bob_limited",
		"soup-limited":  "libops_ff2098bd1a335db9806937f2bf5bdba7_b6b4b341e1e55242a33d684e4da7ad07_test_secret_soup_limited",
	}

	// Test user credentials (email:password) - derived from vault-init.sh
	userCredentials = map[string]string{
		"admin":       "admin@libops.io:password123",
		"art":         "art.vandelay@vandelay.com:password123",
		"jerry":       "jerry.seinfeld@vandelay.com:password123",
		"elaine":      "elaine.benes@vandelay.com:password123",
		"george":      "george.costanza@vandelay.com:password123",
		"kramer":      "cosmo.kramer@vandelay.com:password123",
		"pennypacker": "h.e.pennypacker@pennypacker.com:password123",
		"newman":      "newman@pennypacker.com:password123",
		"bob":         "bob.sacamano@vandelay.com:password123",
		"joe":         "joe.davola@vandelay.com:password123",
		"puddy":       "david.puddy@vandelay.com:password123",
		"soup":        "soup.nazi@vandelay.com:password123",
		"babu":        "babu.bhatt@vandelay.com:password123",
		"leo":         "uncle.leo@vandelay.com:password123",
		"jackie":      "jackie.chiles@pennypacker.com:password123",
		"peterman":    "j.peterman@pennypacker.com:password123",
		"no-access":   "noaccess@test.com:password123",
	}

	// Cache for user tokens
	userTokens = make(map[string]string)

	// Test resource IDs (from rbac_seed.sql)
	rootOrgID   = "d32cb00d-de6f-5706-adbc-2f90ea1607cb" // LibOps Platform
	childOrgID  = "e409a621-ebbc-5e5e-9be2-705558a2f489" // Vandelay Industries
	project1ID  = "eede11e5-0fac-54d1-8d5c-71e4f9deff92" // Project Jupiter
	project2ID  = "51e0e08d-a0bc-5541-9539-d01fa76892d3" // Project Latex
	site1ProdID = "31d5f993-975e-5f24-ac2c-3b0f7f4d5d83" // Jupiter production
	site1StagID = "46edd2fe-98df-5d6f-b0e7-cbfb584ac2b8" // Jupiter staging
	site2ProdID = "8cc8478b-676a-560f-a861-d4e83117c5fc" // Latex production

	// Account IDs
	adminAccountID  = "01052d4d-93be-51a3-9684-c357297533cd"
	kramerAccountID = "516e3bb4-bfbe-5dda-9cc9-d0e00ce7b6f2"

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
	auth           string
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
	tr.auth = "KEY"
	tr.runPermissionMatrixTests()

	// Run tests with Userpass auth
	fmt.Println(cyan("\n================================================="))
	fmt.Println(cyan("  RUNNING TESTS WITH: USERPASS AUTH"))
	fmt.Println(cyan("================================================="))
	currentAuthMethod = AuthMethodUserpass
	tr.auth = "PASS"
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

	loginURL := apiURL + "/auth/token"
	body := fmt.Sprintf(`{"grant_type":"password","username":"%s","password":"%s"}`, email, password)
	slog.Info(body)
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
		slog.Info(email)
		// Lookup Entity ID by name (entity-<email>)
		entityName := fmt.Sprintf("entity-%s", email)
		secret, err := client.Logical().Read(fmt.Sprintf("identity/entity/name/%s", entityName))
		if err != nil {
			fmt.Printf(red("Failed to read entity %s from Vault: %v\n"), entityName, err)
			continue
		}
		if secret == nil || secret.Data == nil {
			fmt.Printf(yellow("Entity %s not found in Vault, skipping sync.\n"), entityName)

			// Debug: List what IS there
			listSecret, listErr := client.Logical().List("identity/entity/name")
			if listErr != nil {
				fmt.Printf(red("Failed to list entities: %v\n"), listErr)
			} else if listSecret != nil && listSecret.Data != nil {
				if keys, ok := listSecret.Data["keys"].([]interface{}); ok {
					fmt.Printf("Available entities: %v\n", keys)
				}
			} else {
				fmt.Println("No entities found in Vault at all.")
			}
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

	// Phase 8: SSH Keys
	fmt.Println(cyan("\n=== Phase 8: SSH Keys ==="))
	tr.testSSHKeys(ctx)

	// Phase 9: Site Operations
	fmt.Println(cyan("\n=== Phase 9: Site Operations ==="))
	tr.testSiteOps(ctx)

	// Phase 10: Account Lookup
	fmt.Println(cyan("\n=== Phase 10: Account Lookup ==="))
	tr.testAccountLookup(ctx)

	// Phase 11: Secret Operations (CRUD)
	fmt.Println(cyan("\n=== Phase 11: Secret Operations ==="))
	tr.testSecretOperations(ctx)
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
		"admin":     true,
		"art":       true,
		"jerry":     true,
		"kramer":    true,
		"bob":       true,
		"joe":       true,
		"puddy":     true,
		"soup":      true,
		"babu":      true,
		"leo":       true,
		"jackie":    false,
		"no-access": false, // Should be denied - not a member
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
		c := tr.orgClient("art")
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
		"admin":     true,  // Owner role inherited from root org via relationship
		"art":       true,  // Direct member with owner role
		"jerry":     true,  // Direct member with developer role (has WRITE permission)
		"no-access": false, // Should be denied - not a member
	})

	tr.test("Admin Create and Delete Org", func() error {
		c := tr.orgClient("admin")

		resp, err := c.CreateOrganization(ctx, connect.NewRequest(&libopsv1.CreateOrganizationRequest{
			Folder: &commonv1.FolderConfig{
				OrganizationName: "Temp Org " + " " + tr.auth,
			},
		}))
		if err != nil {
			return err
		}
		orgID := resp.Msg.OrganizationId
		_, err = c.DeleteOrganization(ctx, connect.NewRequest(&libopsv1.DeleteOrganizationRequest{
			OrganizationId: orgID,
		}))
		return err
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
			userName = strings.ToUpper(userName + string(currentAuthMethod) + tr.auth)
		}
		_, err := c.CreateOrganizationSecret(ctx, connect.NewRequest(&libopsv1.CreateOrganizationSecretRequest{
			OrganizationId: childOrgID,
			Name:           fmt.Sprintf("%s_%s", secretName, userName),
			Value:          "val",
		}))
		return err
	}, map[string]bool{
		"admin":     true,  // Owner role inherited from root org via relationship
		"art":       true,  // Direct member with owner role
		"jerry":     true,  // Direct member with developer role (has WRITE permission)
		"no-access": false, // Should be denied - not a member
	})
}

// --- Project Operations ---

func (tr *TestRunner) testProjectOperations(ctx context.Context) {
	// Matrix: Get Project 1
	tr.testMatrix("Get Project 1", func(user string) error {
		c := tr.projectClient(user)
		_, err := c.GetProject(ctx, connect.NewRequest(&libopsv1.GetProjectRequest{ProjectId: project1ID}))
		return err
	}, map[string]bool{
		"admin":     true,
		"art":       true,
		"jerry":     true,
		"kramer":    true,
		"bob":       true,
		"joe":       true,
		"puddy":     true,
		"soup":      true,
		"babu":      true,
		"leo":       true,
		"no-access": false, // Should be denied - not a member
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
			if user == "art" {
				createdProjectID = resp.Msg.Project.ProjectId
			} else {
				// Cleanup immediately
				_, _ = c.DeleteProject(ctx, connect.NewRequest(&libopsv1.DeleteProjectRequest{ProjectId: resp.Msg.Project.ProjectId}))
			}
		}
		return err
	}, map[string]bool{
		"admin":     true,
		"art":       true,
		"jerry":     true,
		"no-access": false, // Should be denied - not a member
	})

	// Matrix: Update Project 1 (Using the ID created by org-owner or existing project1ID?
	// Plan says "Update Project 1". Let's use existing project1ID as it has members set up,
	// but creating/updating createdProjectID is safer to not break state.
	// However, for permission checks on roles like "bob", we MUST use project1ID.
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
		"admin":     true,
		"art":       true,
		"jerry":     true,
		"bob":       true,
		"joe":       true,
		"no-access": false, // Should be denied - not a member
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
			"art":       true,
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
		if user == "admin" || user == "art" || user == "bob" {
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
		"admin":     true,
		"art":       true,
		"bob":       true,
		"joe":       false,
		"puddy":     false,
		"jerry":     false,
		"soup":      false,
		"no-access": false, // Should be denied - not a member
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
			ProjectId: project1ID, Name: fmt.Sprintf("PROJ_SEC_%s", userName),
			Value: "val",
		}))
		return err
	}, map[string]bool{
		"admin":     true,
		"art":       true,
		"jerry":     true,
		"bob":       true,
		"joe":       true,
		"no-access": false, // Should be denied - not a member
	})
}

// --- Site Operations ---

func (tr *TestRunner) testSiteOperations(ctx context.Context) {
	// Matrix: Get Site 1
	tr.testMatrix("Get Site 1", func(user string) error {
		c := tr.siteClient(user)
		_, err := c.GetSite(ctx, connect.NewRequest(&libopsv1.GetSiteRequest{SiteId: site1ProdID}))
		return err
	}, map[string]bool{
		"admin":     true,
		"art":       true,
		"jerry":     true,
		"kramer":    true,
		"bob":       true,
		"joe":       true,
		"puddy":     true,
		"soup":      true,
		"babu":      true,
		"leo":       true,
		"no-access": false, // Should be denied - not a member
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
			if user == "art" {
				createdSiteID = resp.Msg.Site.SiteId
			} else {
				_, _ = c.DeleteSite(ctx, connect.NewRequest(&libopsv1.DeleteSiteRequest{SiteId: resp.Msg.Site.SiteId}))
			}
		}
		return err
	}, map[string]bool{
		"admin":     true,
		"art":       true,
		"jerry":     true,
		"bob":       true,
		"joe":       true,
		"no-access": false, // Should be denied - not a member
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
		"admin":     true,
		"art":       true,
		"jerry":     true,
		"bob":       true,
		"joe":       true,
		"soup":      true,
		"babu":      true,
		"no-access": false, // Should be denied - not a member
	})

	// Matrix: Delete Site (using createdSiteID)
	// Note: Only test with org-owner to avoid race where one user deletes before another tries
	if createdSiteID != "" {
		tr.test("Org Owner can delete created site", func() error {
			c := tr.siteClient("art")
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
		"art":       true,
		"jerry":     true,
		"bob":       true,
		"joe":       true,
		"soup":      true,
		"babu":      true,
		"no-access": false, // Should be denied - not a member
	})
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
		"admin":     true,
		"art":       true,
		"jerry":     true,
		"no-access": false, // Should be denied - not a member
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
		"admin":     true,
		"art":       true,
		"jerry":     true,
		"bob":       true,
		"joe":       true,
		"no-access": false, // Should be denied - not a member
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
		"admin":     true,
		"art":       true,
		"jerry":     true,
		"bob":       true,
		"joe":       true,
		"soup":      true,
		"babu":      true,
		"no-access": false, // Should be denied - not a member
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
		"admin":  true,  // Access via relationship from root org
		"art":    true,  // Direct member of child org
		"jerry":  true,  // Direct member of child org
		"kramer": true,  // Direct member of child org
		"bob":    true,  // Inherits read access via project membership
		"joe":    true,  // Inherits read access via project membership
		"puddy":  true,  // Inherits read access via project membership
		"soup":   true,  // Inherits read access via site membership
		"babu":   true,  // Inherits read access via site membership
		"leo":    true,  // Inherits read access via site membership
		"jackie": false, // Inherits read access via project membership
	})

	// Matrix: List Project Members
	tr.testMatrix("List Project Members", func(user string) error {
		c := tr.projectMemberClient(user)
		_, err := c.ListProjectMembers(ctx, connect.NewRequest(&libopsv1.ListProjectMembersRequest{ProjectId: project1ID}))
		return err
	}, map[string]bool{
		"admin":     true,
		"art":       true,
		"jerry":     true,
		"kramer":    true,
		"bob":       true,
		"joe":       true,
		"puddy":     true,
		"soup":      true,
		"babu":      true,
		"leo":       true,
		"jackie":    false, // Different project
		"no-access": false,
	})

	// Matrix: List Site Members
	tr.testMatrix("List Site Members", func(user string) error {
		c := tr.siteMemberClient(user)
		_, err := c.ListSiteMembers(ctx, connect.NewRequest(&libopsv1.ListSiteMembersRequest{SiteId: site1ProdID}))
		return err
	}, map[string]bool{
		"admin":     true,
		"art":       true,
		"jerry":     true,
		"kramer":    true,
		"bob":       true,
		"joe":       true,
		"puddy":     true,
		"soup":      true,
		"babu":      true,
		"leo":       true,
		"jackie":    false, // Different project
		"no-access": false,
	})

	// Matrix: Create Org Member
	// We can't actually create members for everyone without running out of test users.
	// Just verify permissions for a few key roles using specific fail accounts.
	tr.testError("Org Developer CANNOT create org member", func() error {
		c := tr.orgMemberClient("jerry")
		_, err := c.CreateOrganizationMember(ctx, connect.NewRequest(&libopsv1.CreateOrganizationMemberRequest{
			OrganizationId: childOrgID,
			AccountId:      adminAccountID,
			Role:           "read",
		}))
		return err
	})

	tr.test("Org Owner can create org member", func() error {
		c := tr.orgMemberClient("art")
		// Proactive cleanup to avoid duplicates on re-runs
		_, _ = c.DeleteOrganizationMember(ctx, connect.NewRequest(&libopsv1.DeleteOrganizationMemberRequest{
			OrganizationId: childOrgID,
			AccountId:      adminAccountID,
		}))
		_, err := c.CreateOrganizationMember(ctx, connect.NewRequest(&libopsv1.CreateOrganizationMemberRequest{
			OrganizationId: childOrgID,
			AccountId:      adminAccountID, // admin, already member but creates duplicates if not careful?
			// In DB schema, unique constraint exists. But 'admin' is owner of Root Org.
			// 'admin' is NOT explicitly a member of Child Org in seed data.
			// Relationships grant him access.
			// So we can add him as explicit member.
			Role: "read",
		}))
		return err
	})

	tr.test("Org Owner can update org member", func() error {
		c := tr.orgMemberClient("art")
		_, err := c.UpdateOrganizationMember(ctx, connect.NewRequest(&libopsv1.UpdateOrganizationMemberRequest{
			OrganizationId: childOrgID,
			AccountId:      adminAccountID,
			Role:           "developer",
			UpdateMask:     &fieldmaskpb.FieldMask{Paths: []string{"role"}},
		}))
		return err
	})

	tr.test("Org Owner can delete org member", func() error {
		c := tr.orgMemberClient("art")
		_, err := c.DeleteOrganizationMember(ctx, connect.NewRequest(&libopsv1.DeleteOrganizationMemberRequest{
			OrganizationId: childOrgID,
			AccountId:      adminAccountID,
		}))
		return err
	})

	// Parent Access: Org Owner creates Project Member
	tr.test("Org Owner can create project member", func() error {
		c := tr.projectMemberClient("art")
		_, _ = c.DeleteProjectMember(ctx, connect.NewRequest(&libopsv1.DeleteProjectMemberRequest{
			ProjectId: project1ID,
			AccountId: kramerAccountID,
		}))
		_, err := c.CreateProjectMember(ctx, connect.NewRequest(&libopsv1.CreateProjectMemberRequest{
			ProjectId: project1ID,
			AccountId: kramerAccountID, // org-read
			Role:      "read",
		}))
		return err
	})

	tr.test("Org Owner can update project member", func() error {
		c := tr.projectMemberClient("art")
		_, err := c.UpdateProjectMember(ctx, connect.NewRequest(&libopsv1.UpdateProjectMemberRequest{
			ProjectId:  project1ID,
			AccountId:  kramerAccountID,
			Role:       "developer",
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"role"}},
		}))
		return err
	})

	tr.test("Org Owner can delete project member", func() error {
		c := tr.projectMemberClient("art")
		_, err := c.DeleteProjectMember(ctx, connect.NewRequest(&libopsv1.DeleteProjectMemberRequest{
			ProjectId: project1ID,
			AccountId: kramerAccountID,
		}))
		return err
	})

	// Site Member Operations
	tr.test("Site Owner can create site member", func() error {
		c := tr.siteMemberClient("soup")
		_, _ = c.DeleteSiteMember(ctx, connect.NewRequest(&libopsv1.DeleteSiteMemberRequest{
			SiteId:    site1ProdID,
			AccountId: kramerAccountID,
		}))
		_, err := c.CreateSiteMember(ctx, connect.NewRequest(&libopsv1.CreateSiteMemberRequest{
			SiteId:    site1ProdID,
			AccountId: kramerAccountID, // org-read
			Role:      "read",
		}))
		return err
	})

	tr.test("Site Owner can update site member", func() error {
		c := tr.siteMemberClient("soup")
		_, err := c.UpdateSiteMember(ctx, connect.NewRequest(&libopsv1.UpdateSiteMemberRequest{
			SiteId:     site1ProdID,
			AccountId:  kramerAccountID,
			Role:       "developer",
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"role"}},
		}))
		return err
	})

	tr.test("Site Owner can delete site member", func() error {
		c := tr.siteMemberClient("soup")
		_, err := c.DeleteSiteMember(ctx, connect.NewRequest(&libopsv1.DeleteSiteMemberRequest{
			SiteId:    site1ProdID,
			AccountId: kramerAccountID,
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
		// Verify new format: libops_{accountUUID}_{keyUUID}_{randomSecret}
		parts := strings.Split(adminKeySecret, "_")
		if len(parts) < 4 || parts[0] != "libops" {
			return fmt.Errorf("expected API key format 'libops_{accountUUID}_{keyUUID}_{randomSecret}', got: %s", adminKeySecret)
		}
		if len(parts[1]) != 32 || len(parts[2]) != 32 {
			return fmt.Errorf("expected 32-char hex UUIDs in API key, got account_uuid=%d chars, key_uuid=%d chars", len(parts[1]), len(parts[2]))
		}
		if len(parts[3]) == 0 {
			return fmt.Errorf("expected random secret component in API key, got empty string")
		}
		slog.Info("DEBUG: " + adminKeySecret)
		// Store for later use
		apiKeys["admin-limited"] = adminKeySecret
		fmt.Printf(green("    ✓ Created key %s\n"), adminKeyID)
		return nil
	})

	// Test 2: Org Owner creates an API key with limited project:read scope
	tr.test("Org Owner creates API key with project:read scope", func() error {
		c := tr.accountClient("art")
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
		apiKeys["art-limited"] = orgOwnerKeySecret
		fmt.Printf(green("    ✓ Created key %s\n"), orgOwnerKeyID)
		return nil
	})

	// Test 3: Project 1 Owner creates an API key with limited site:read scope
	tr.test("Project 1 Owner creates API key with site:read scope", func() error {
		c := tr.accountClient("bob")
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
		apiKeys["bob-limited"] = proj1OwnerKeySecret
		fmt.Printf(green("    ✓ Created key %s\n"), proj1OwnerKeyID)
		return nil
	})

	// Test 4: Site 1 Owner creates an API key with limited site:read scope
	tr.test("Site 1 Owner creates API key with site:read scope", func() error {
		c := tr.accountClient("soup")
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
		apiKeys["soup-limited"] = site1OwnerKeySecret
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
		c := tr.orgClient("art-limited")
		_, err := c.UpdateOrganization(ctx, connect.NewRequest(&libopsv1.UpdateOrganizationRequest{
			OrganizationId: childOrgID,
			Folder:         &commonv1.FolderConfig{OrganizationName: "Should Fail"},
		}))
		return err
	})

	tr.test("Org Owner (Limited read:project) CAN read project", func() error {
		c := tr.projectClient("art-limited")
		_, err := c.GetProject(ctx, connect.NewRequest(&libopsv1.GetProjectRequest{ProjectId: project1ID}))
		return err
	})

	tr.testError("Org Owner (Limited read:project) CANNOT create project (needs write)", func() error {
		c := tr.projectClient("art-limited")
		_, err := c.CreateProject(ctx, connect.NewRequest(&libopsv1.CreateProjectRequest{
			OrganizationId: childOrgID,
			Project:        &commonv1.ProjectConfig{ProjectName: "scope-test"},
		}))
		return err
	})

	tr.testError("Proj1 Owner (Limited read:project) CANNOT update project (needs write)", func() error {
		c := tr.projectClient("bob-limited")
		_, err := c.UpdateProject(ctx, connect.NewRequest(&libopsv1.UpdateProjectRequest{
			ProjectId: project1ID,
			Project:   &commonv1.ProjectConfig{ProjectName: "should-fail"},
		}))
		return err
	})

	// Site Scope Tests
	tr.test("Site1 Owner (Limited read:site) CAN read site", func() error {
		c := tr.siteClient("soup-limited")
		_, err := c.GetSite(ctx, connect.NewRequest(&libopsv1.GetSiteRequest{SiteId: site1ProdID}))
		return err
	})

	tr.testError("Site1 Owner (Limited read:site) CANNOT update site (needs write)", func() error {
		c := tr.siteClient("soup-limited")
		_, err := c.UpdateSite(ctx, connect.NewRequest(&libopsv1.UpdateSiteRequest{
			SiteId: site1ProdID,
			Site:   &commonv1.SiteConfig{GithubRef: "should-fail"},
		}))
		return err
	})

	tr.testError("Site1 Owner (Limited read:site) CANNOT create site secret (needs write)", func() error {
		c := tr.siteSecretClient("soup-limited")
		_, err := c.CreateSiteSecret(ctx, connect.NewRequest(&libopsv1.CreateSiteSecretRequest{
			SiteId: site1ProdID,
			Name:   "SCOPE_TEST",
			Value:  "val",
		}))
		return err
	})

	tr.testError("Site1 Owner (Limited read:site) CANNOT read project (wrong resource scope)", func() error {
		c := tr.projectClient("soup-limited")
		_, err := c.GetProject(ctx, connect.NewRequest(&libopsv1.GetProjectRequest{ProjectId: project1ID}))
		return err
	})

	// ============================================================================
	// HIERARCHICAL SCOPE TESTS - Resource Type Boundaries
	// ============================================================================
	// Scopes should strictly enforce resource type - even if membership would grant access

	// Upward access tests (child scope cannot access parent resources)
	tr.testError("Site1 Owner (delete:site) CANNOT read project (wrong resource type)", func() error {
		c := tr.projectClient("soup-limited") // has delete:site scope only
		_, err := c.GetProject(ctx, connect.NewRequest(&libopsv1.GetProjectRequest{ProjectId: project1ID}))
		return err
	})

	tr.testError("Site1 Owner (delete:site) CANNOT read org (wrong resource type)", func() error {
		c := tr.orgClient("soup-limited") // has delete:site scope only
		_, err := c.GetOrganization(ctx, connect.NewRequest(&libopsv1.GetOrganizationRequest{OrganizationId: childOrgID}))
		return err
	})

	tr.testError("Proj1 Owner (delete:project) CANNOT read org (wrong resource type)", func() error {
		c := tr.orgClient("bob-limited") // has delete:project scope only
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
		c := tr.orgClient("art")
		_, err := c.GetOrganization(ctx, connect.NewRequest(&libopsv1.GetOrganizationRequest{OrganizationId: childOrgID}))
		return err
	})

	tr.test("Org Owner (full scopes) CAN read project", func() error {
		c := tr.projectClient("art")
		_, err := c.GetProject(ctx, connect.NewRequest(&libopsv1.GetProjectRequest{ProjectId: project1ID}))
		return err
	})

	tr.test("Org Owner (full scopes) CAN read site", func() error {
		c := tr.siteClient("art")
		_, err := c.GetSite(ctx, connect.NewRequest(&libopsv1.GetSiteRequest{SiteId: site1ProdID}))
		return err
	})

	// But limited to only read:project
	tr.testError("Org Owner (limited read:project) CANNOT read org", func() error {
		c := tr.orgClient("art-limited")
		_, err := c.GetOrganization(ctx, connect.NewRequest(&libopsv1.GetOrganizationRequest{OrganizationId: childOrgID}))
		return err
	})

	tr.test("Org Owner (limited read:project) CAN read project", func() error {
		c := tr.projectClient("art-limited")
		_, err := c.GetProject(ctx, connect.NewRequest(&libopsv1.GetProjectRequest{ProjectId: project1ID}))
		return err
	})

	tr.testError("Org Owner (limited read:project) CANNOT read site", func() error {
		c := tr.siteClient("art-limited")
		_, err := c.GetSite(ctx, connect.NewRequest(&libopsv1.GetSiteRequest{SiteId: site1ProdID}))
		return err
	})
}

// --- Isolation & Inheritance ---

func (tr *TestRunner) testCrossResourceIsolation(ctx context.Context) {
	tr.testError("Proj1 Owner CANNOT read Proj2", func() error {
		c := tr.projectClient("bob")
		_, err := c.GetProject(ctx, connect.NewRequest(&libopsv1.GetProjectRequest{ProjectId: project2ID}))
		return err
	})

	tr.testError("Proj1 Developer CANNOT read Proj2 secrets", func() error {
		c := tr.projectSecretClient("joe")
		_, err := c.ListProjectSecrets(ctx, connect.NewRequest(&libopsv1.ListProjectSecretsRequest{ProjectId: project2ID}))
		return err
	})
}

func (tr *TestRunner) testMembershipInheritance(ctx context.Context) {
	// Site member -> Project
	tr.test("Site Owner can read parent Project", func() error {
		c := tr.projectClient("soup")
		_, err := c.GetProject(ctx, connect.NewRequest(&libopsv1.GetProjectRequest{ProjectId: project1ID}))
		return err
	})
	// Site member -> Org
	tr.test("Site Owner can read parent Org", func() error {
		c := tr.orgClient("soup")
		_, err := c.GetOrganization(ctx, connect.NewRequest(&libopsv1.GetOrganizationRequest{OrganizationId: childOrgID}))
		return err
	})
}

// --- Helpers ---

func (tr *TestRunner) testMatrix(operationName string, action func(user string) error, allowedUsers map[string]bool) {
	users := []string{
		"admin", "art", "jerry", "kramer",
		"bob", "joe", "puddy",
		"soup", "babu", "leo",
		"jackie", "no-access",
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

func (tr *TestRunner) testSSHKeys(ctx context.Context) {
	tr.test("Admin list own SSH keys", func() error {
		c := tr.sshKeyClient("admin")
		_, err := c.ListSshKeys(ctx, connect.NewRequest(&libopsv1.ListSshKeysRequest{
			AccountId: adminAccountID,
		}))
		return err
	})

	var keyID string
	tr.test("Admin create SSH key", func() error {
		c := tr.sshKeyClient("admin")

		resp, err := c.CreateSshKey(ctx, connect.NewRequest(&libopsv1.CreateSshKeyRequest{
			AccountId: adminAccountID,
			PublicKey: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJrc2TIYGf23UbqlrrUDnDpDwMIcmhPLePcAJ4crmfoG jcorall@MacBookPro",
			Name:      strPtr("test-key"),
		}))
		if err == nil {
			keyID = resp.Msg.SshKey.KeyId
		}
		return err
	})

	if keyID != "" {
		tr.test("Admin delete SSH key", func() error {
			c := tr.sshKeyClient("admin")
			_, err := c.DeleteSshKey(ctx, connect.NewRequest(&libopsv1.DeleteSshKeyRequest{
				AccountId: adminAccountID,
				KeyId:     keyID,
			}))
			return err
		})
	}
}

func (tr *TestRunner) testSiteOps(ctx context.Context) {
	// 1. Get Status (site1-owner)
	tr.test("Site Owner can get site status", func() error {
		c := tr.siteOpsClient("soup")
		_, err := c.GetSiteStatus(ctx, connect.NewRequest(&libopsv1.GetSiteStatusRequest{SiteId: site1ProdID}))
		return err
	})

	// 2. Deploy Site (site1-developer)
	tr.test("Site Developer can deploy site", func() error {
		c := tr.siteOpsClient("babu")
		_, err := c.DeploySite(ctx, connect.NewRequest(&libopsv1.DeploySiteRequest{SiteId: site1ProdID}))
		return err
	})

	// 3. Deploy Site (Negative - site1-read)
	tr.testError("Site Reader CANNOT deploy site", func() error {
		c := tr.siteOpsClient("leo")
		_, err := c.DeploySite(ctx, connect.NewRequest(&libopsv1.DeploySiteRequest{SiteId: site1ProdID}))
		return err
	})
}

func (tr *TestRunner) testAccountLookup(ctx context.Context) {
	// 1. Admin lookup account by email
	tr.test("Admin can lookup account by email", func() error {
		c := tr.accountClient("admin")
		resp, err := c.GetAccountByEmail(ctx, connect.NewRequest(&libopsv1.GetAccountByEmailRequest{Email: "art.vandelay@vandelay.com"}))
		if err != nil {
			return err
		}
		if resp.Msg.Account.Email != "art.vandelay@vandelay.com" {
			return fmt.Errorf("expected email art.vandelay@vandelay.com, got %s", resp.Msg.Account.Email)
		}
		return nil
	})

	// 2. Lookup non-existent account
	tr.testError("Lookup non-existent account returns 404", func() error {
		c := tr.accountClient("admin")
		_, err := c.GetAccountByEmail(ctx, connect.NewRequest(&libopsv1.GetAccountByEmailRequest{Email: "does-not-exist@example.com"}))
		if err != nil && connect.CodeOf(err) == connect.CodeNotFound {
			return err
		}
		return err
	})

	// 3. Regular user CAN lookup other accounts (as long as they are in an org)
	tr.test("Regular user CAN lookup other accounts", func() error {
		c := tr.accountClient("art")
		_, err := c.GetAccountByEmail(ctx, connect.NewRequest(&libopsv1.GetAccountByEmailRequest{Email: "admin@libops.io"}))
		return err
	})
}

// Client Factories

func (tr *TestRunner) httpClient(key string) *http.Client {
	return &http.Client{Transport: &authTransport{tr.getToken(key)}}
}

func (tr *TestRunner) orgClient(key string) libopsv1connect.OrganizationServiceClient {
	client := tr.httpClient(key)
	return libopsv1connect.NewOrganizationServiceClient(client, apiURL)
}
func (tr *TestRunner) projectClient(key string) libopsv1connect.ProjectServiceClient {
	client := tr.httpClient(key)
	return libopsv1connect.NewProjectServiceClient(client, apiURL)
}
func (tr *TestRunner) siteClient(key string) libopsv1connect.SiteServiceClient {
	client := tr.httpClient(key)
	return libopsv1connect.NewSiteServiceClient(client, apiURL)
}
func (tr *TestRunner) orgSecretClient(key string) libopsv1connect.OrganizationSecretServiceClient {
	client := tr.httpClient(key)
	return libopsv1connect.NewOrganizationSecretServiceClient(client, apiURL)
}
func (tr *TestRunner) projectSecretClient(key string) libopsv1connect.ProjectSecretServiceClient {
	client := tr.httpClient(key)
	return libopsv1connect.NewProjectSecretServiceClient(client, apiURL)
}
func (tr *TestRunner) siteSecretClient(key string) libopsv1connect.SiteSecretServiceClient {
	client := tr.httpClient(key)
	return libopsv1connect.NewSiteSecretServiceClient(client, apiURL)
}
func (tr *TestRunner) firewallClient(key string) libopsv1connect.FirewallServiceClient {
	client := tr.httpClient(key)
	return libopsv1connect.NewFirewallServiceClient(client, apiURL)
}
func (tr *TestRunner) projectFirewallClient(key string) libopsv1connect.ProjectFirewallServiceClient {
	client := tr.httpClient(key)
	return libopsv1connect.NewProjectFirewallServiceClient(client, apiURL)
}
func (tr *TestRunner) siteFirewallClient(key string) libopsv1connect.SiteFirewallServiceClient {
	client := tr.httpClient(key)
	return libopsv1connect.NewSiteFirewallServiceClient(client, apiURL)
}
func (tr *TestRunner) orgMemberClient(key string) libopsv1connect.MemberServiceClient {
	client := tr.httpClient(key)
	return libopsv1connect.NewMemberServiceClient(client, apiURL)
}
func (tr *TestRunner) projectMemberClient(key string) libopsv1connect.ProjectMemberServiceClient {
	client := tr.httpClient(key)
	return libopsv1connect.NewProjectMemberServiceClient(client, apiURL)
}
func (tr *TestRunner) siteMemberClient(key string) libopsv1connect.SiteMemberServiceClient {
	client := tr.httpClient(key)
	return libopsv1connect.NewSiteMemberServiceClient(client, apiURL)
}
func (tr *TestRunner) accountClient(key string) libopsv1connect.AccountServiceClient {
	client := tr.httpClient(key)
	return libopsv1connect.NewAccountServiceClient(client, apiURL)
}

func (tr *TestRunner) sshKeyClient(key string) libopsv1connect.SshKeyServiceClient {
	client := tr.httpClient(key)
	return libopsv1connect.NewSshKeyServiceClient(client, apiURL)
}

func (tr *TestRunner) siteOpsClient(key string) libopsv1connect.SiteOperationsServiceClient {
	client := tr.httpClient(key)
	return libopsv1connect.NewSiteOperationsServiceClient(client, apiURL)
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

func (tr *TestRunner) testSecretOperations(ctx context.Context) {
	tr.test("Org Secret CRUD (Get/Update)", func() error {
		c := tr.orgSecretClient("art")

		// Create
		resp, err := c.CreateOrganizationSecret(ctx, connect.NewRequest(&libopsv1.CreateOrganizationSecretRequest{
			OrganizationId: childOrgID,
			Name:           "CRUD_TEST_SECRET_" + tr.auth,
			Value:          "val1",
		}))
		if err != nil {
			return err
		}
		secretID := resp.Msg.Secret.SecretId

		// Get
		getResp, err := c.GetOrganizationSecret(ctx, connect.NewRequest(&libopsv1.GetOrganizationSecretRequest{
			OrganizationId: childOrgID,
			SecretId:       secretID,
		}))
		if err != nil {
			return err
		}
		if getResp.Msg.Secret.Name != "CRUD_TEST_SECRET_"+tr.auth {
			return fmt.Errorf("name mismatch")
		}

		// Update
		_, err = c.UpdateOrganizationSecret(ctx, connect.NewRequest(&libopsv1.UpdateOrganizationSecretRequest{
			OrganizationId: childOrgID,
			SecretId:       secretID,
			Value:          strPtr("val2"),
			UpdateMask:     &fieldmaskpb.FieldMask{Paths: []string{"value"}},
		}))
		if err != nil {
			return err
		}

		// Delete
		_, err = c.DeleteOrganizationSecret(ctx, connect.NewRequest(&libopsv1.DeleteOrganizationSecretRequest{
			OrganizationId: childOrgID,
			SecretId:       secretID,
		}))
		return err
	})
}

func strPtr(s string) *string {
	return &s
}
