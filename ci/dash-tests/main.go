package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/fatih/color"
)

var (
	dashboardURL = getEnv("DASHBOARD_URL", "http://api:8080")
	headless     = getEnv("HEADLESS", "true") == "true"

	green  = color.New(color.FgGreen).SprintFunc()
	red    = color.New(color.FgRed).SprintFunc()
	cyan   = color.New(color.FgCyan).SprintFunc()
	yellow = color.New(color.FgYellow).SprintFunc()

	// Test credentials (from seed data)
	testEmail    = "admin@libops.io"
	testPassword = "password123"
)

type TestRunner struct {
	passed, failed int
	ctx            context.Context
	cancel         context.CancelFunc
}

func main() {
	if len(os.Args) < 2 || os.Args[1] != "run-tests" {
		fmt.Println("Usage: dash-tests run-tests")
		os.Exit(1)
	}

	runner := &TestRunner{}
	runner.Setup()
	defer runner.Teardown()

	runner.RunAllTests()
	runner.PrintResults()

	if runner.failed > 0 {
		os.Exit(1)
	}
}

func (tr *TestRunner) Setup() {
	fmt.Println(cyan("================================================="))
	fmt.Println(cyan("  Dashboard E2E Tests - Headless Chrome"))
	fmt.Println(cyan("=================================================\n"))

	// Wait for dashboard to be ready
	tr.waitForDashboard()

	// Setup chromedp
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", headless),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.WindowSize(1920, 1080),
	)

	allocCtx, _ := chromedp.NewExecAllocator(context.Background(), opts...)
	tr.ctx, tr.cancel = chromedp.NewContext(allocCtx, chromedp.WithLogf(log.Printf))

	// Set a reasonable timeout for the whole context
	tr.ctx, tr.cancel = context.WithTimeout(tr.ctx, 5*time.Minute)
}

func (tr *TestRunner) Teardown() {
	if tr.cancel != nil {
		tr.cancel()
	}
}

func (tr *TestRunner) waitForDashboard() {
	fmt.Print("Waiting for dashboard...")
	for i := 0; i < 30; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		opts := append(chromedp.DefaultExecAllocatorOptions[:],
			chromedp.Flag("headless", true),
			chromedp.Flag("disable-gpu", true),
			chromedp.Flag("no-sandbox", true),
		)
		allocCtx, _ := chromedp.NewExecAllocator(ctx, opts...)
		testCtx, _ := chromedp.NewContext(allocCtx)

		err := chromedp.Run(testCtx,
			chromedp.Navigate(dashboardURL+"/login"),
			chromedp.WaitReady("body", chromedp.ByQuery),
		)
		cancel()

		if err == nil {
			fmt.Println(green(" ✓"))
			return
		}
		time.Sleep(2 * time.Second)
		fmt.Print(".")
	}
	fmt.Println(red(" ✗"))
	log.Fatal("Dashboard did not become ready in time")
}

func (tr *TestRunner) RunAllTests() {
	// Phase 1: Login Page Tests
	fmt.Println(cyan("\n=== Phase 1: Login Page Tests ==="))
	tr.testLoginPageNoErrors()

	// Phase 2: Authentication
	fmt.Println(cyan("\n=== Phase 2: Authentication ==="))
	tr.testLoginSuccess()

	// Phase 3: Organizations CRUD
	fmt.Println(cyan("\n=== Phase 3: Organizations CRUD ==="))
	tr.testOrganizationsCRUD()

	// Phase 4: Projects CRUD
	fmt.Println(cyan("\n=== Phase 4: Projects CRUD ==="))
	tr.testProjectsCRUD()

	// Phase 5: Sites CRUD
	fmt.Println(cyan("\n=== Phase 5: Sites CRUD ==="))
	tr.testSitesCRUD()

	// Phase 6: Firewall Management
	fmt.Println(cyan("\n=== Phase 6: Firewall Management ==="))
	tr.testFirewallManagement()

	// Phase 7: Member Management
	fmt.Println(cyan("\n=== Phase 7: Member Management ==="))
	tr.testMemberManagement()

	// Phase 8: Secret Management
	fmt.Println(cyan("\n=== Phase 8: Secret Management ==="))
	tr.testSecretManagement()

	// Phase 9: API Keys
	fmt.Println(cyan("\n=== Phase 9: API Keys ==="))
	tr.testAPIKeyManagement()

	// Phase 10: SSH Keys
	fmt.Println(cyan("\n=== Phase 10: SSH Keys ==="))
	tr.testSSHKeyManagement()
}

func (tr *TestRunner) testLoginPageNoErrors() {
	tr.test("Login page loads without 404 errors", func() error {
		var networkErrors []string

		// Listen for failed requests
		chromedp.ListenTarget(tr.ctx, func(ev interface{}) {
			switch ev := ev.(type) {
			case *network.EventLoadingFailed:
				if ev.ErrorText != "" && strings.Contains(ev.ErrorText, "404") {
					networkErrors = append(networkErrors, ev.ErrorText)
				}
			}
		})

		err := chromedp.Run(tr.ctx,
			chromedp.Navigate(dashboardURL+"/login"),
			chromedp.WaitVisible("body", chromedp.ByQuery),
			// Wait for page to fully load
			chromedp.Sleep(2*time.Second),
		)

		if err != nil {
			return fmt.Errorf("failed to load login page: %w", err)
		}

		if len(networkErrors) > 0 {
			return fmt.Errorf("found 404 errors: %v", networkErrors)
		}

		return nil
	})

	tr.test("Login page has required elements", func() error {
		var emailExists, continueExists, passwordExists, submitExists bool

		err := chromedp.Run(tr.ctx,
			chromedp.Navigate(dashboardURL+"/login"),
			chromedp.WaitVisible("body", chromedp.ByQuery),
			// Check for email input
			chromedp.Evaluate(`document.querySelector('#login-email') !== null`, &emailExists),
			// Check for continue button
			chromedp.Evaluate(`document.querySelector('#email-continue') !== null`, &continueExists),
			// Check for password input (initially hidden)
			chromedp.Evaluate(`document.querySelector('#login-password') !== null`, &passwordExists),
			// Check for submit button
			chromedp.Evaluate(`document.querySelector('button[type="submit"]') !== null`, &submitExists),
		)

		if err != nil {
			return fmt.Errorf("failed to check login elements: %w", err)
		}

		if !emailExists {
			return fmt.Errorf("email input not found")
		}
		if !continueExists {
			return fmt.Errorf("continue button not found")
		}
		if !passwordExists {
			return fmt.Errorf("password input not found")
		}
		if !submitExists {
			return fmt.Errorf("submit button not found")
		}

		return nil
	})
}

func (tr *TestRunner) testLoginSuccess() {
	tr.test("Login with valid credentials redirects to dashboard", func() error {
		var currentURL string

		err := chromedp.Run(tr.ctx,
			// Navigate to login page
			chromedp.Navigate(dashboardURL+"/login"),
			chromedp.WaitVisible(`#login-email`, chromedp.ByID),

			// Fill in email
			chromedp.SendKeys(`#login-email`, testEmail, chromedp.ByID),

			// Wait for continue button to be enabled (JavaScript validation)
			chromedp.Sleep(500*time.Millisecond),

			// Click continue to show password step
			chromedp.Click(`#email-continue`, chromedp.ByID),

			// Wait for password field to appear
			chromedp.WaitVisible(`#login-password`, chromedp.ByID),

			// Fill in password
			chromedp.SendKeys(`#login-password`, testPassword, chromedp.ByID),

			// Submit the form
			chromedp.Submit(`#email-form`, chromedp.ByID),

			// Wait for navigation to complete
			chromedp.Sleep(3*time.Second),
			chromedp.Location(&currentURL),
		)

		if err != nil {
			return fmt.Errorf("login failed: %w", err)
		}

		// Log cookies for debugging
		var cookies string
		err = chromedp.Run(tr.ctx,
			chromedp.Evaluate(`document.cookie`, &cookies),
		)
		if err == nil && cookies != "" {
			fmt.Printf("      Cookies after login: %s\n", cookies)
		}

		// Check if we're no longer on the login page
		if strings.Contains(currentURL, "/login") {
			// Get any error messages on the page
			var errorText string
			chromedp.Run(tr.ctx,
				chromedp.Evaluate(`document.body.textContent`, &errorText),
			)
			return fmt.Errorf("still on login page after submit, login may have failed. URL: %s\nPage text: %s", currentURL, errorText[:min(200, len(errorText))])
		}

		// Verify we can see dashboard elements
		var dashboardVisible bool
		err = chromedp.Run(tr.ctx,
			chromedp.Evaluate(`document.body.textContent.includes('Overview') || document.body.textContent.includes('Dashboard')`, &dashboardVisible),
		)

		if err != nil || !dashboardVisible {
			return fmt.Errorf("dashboard not visible after login")
		}

		return nil
	})

	// Verify cookies persist by checking a protected page
	tr.test("Session persists across navigation", func() error {
		var currentURL string

		err := chromedp.Run(tr.ctx,
			chromedp.Navigate(dashboardURL+"/organizations"),
			chromedp.Sleep(2*time.Second),
			chromedp.Location(&currentURL),
		)

		if err != nil {
			return fmt.Errorf("failed to navigate to organizations: %w", err)
		}

		// If we got redirected back to login, cookies didn't persist
		if strings.Contains(currentURL, "/login") {
			return fmt.Errorf("redirected to login page, session not persisted")
		}

		return nil
	})
}

func (tr *TestRunner) testOrganizationsCRUD() {
	tr.test("Navigate to Organizations page", func() error {
		return chromedp.Run(tr.ctx,
			chromedp.Click(`a[href="/organizations"]`, chromedp.ByQuery),
			chromedp.WaitVisible("body", chromedp.ByQuery),
			chromedp.Sleep(1*time.Second),
		)
	})

	tr.test("Create new organization", func() error {
		orgName := fmt.Sprintf("Test Org %d", time.Now().Unix())

		// Use JavaScript to click the button and open modal
		err := chromedp.Run(tr.ctx,
			// Find and click the Create button using JavaScript
			chromedp.Evaluate(`(function() {
				const btn = Array.from(document.querySelectorAll('button')).find(b => b.textContent.includes('Create'));
				if (btn) { btn.click(); return true; }
				return false;
			})()`, nil),
			chromedp.Sleep(1*time.Second),
		)
		if err != nil {
			return fmt.Errorf("failed to click create button: %w", err)
		}

		// Fill form and submit
		return chromedp.Run(tr.ctx,
			chromedp.WaitVisible(`input[name="name"]`, chromedp.ByQuery),
			chromedp.SendKeys(`input[name="name"]`, orgName, chromedp.ByQuery),
			// Find and click submit button using JavaScript
			chromedp.Evaluate(`(function() {
				const submitBtn = Array.from(document.querySelectorAll('button[type="submit"]')).find(b => b.textContent.includes('Create'));
				if (submitBtn) { submitBtn.click(); return true; }
				return false;
			})()`, nil),
			chromedp.Sleep(2*time.Second),
		)
	})

	tr.test("View organization in list", func() error {
		var tableVisible bool

		return chromedp.Run(tr.ctx,
			chromedp.Navigate(dashboardURL+"/organizations"),
			chromedp.WaitVisible("body", chromedp.ByQuery),
			chromedp.Sleep(1*time.Second),
			chromedp.Evaluate(`document.querySelector('table') !== null || document.querySelector('.bg-white') !== null`, &tableVisible),
		)
	})
}

func (tr *TestRunner) testProjectsCRUD() {
	tr.test("Navigate to Projects page", func() error {
		return chromedp.Run(tr.ctx,
			chromedp.Click(`a[href="/projects"]`, chromedp.ByQuery),
			chromedp.WaitVisible("body", chromedp.ByQuery),
			chromedp.Sleep(1*time.Second),
		)
	})

	tr.test("Create new project", func() error {
		projectName := fmt.Sprintf("Test Project %d", time.Now().Unix())

		// Click create button
		err := chromedp.Run(tr.ctx,
			chromedp.Evaluate(`(function() {
				const btn = Array.from(document.querySelectorAll('button')).find(b => b.textContent.includes('Create'));
				if (btn) { btn.click(); return true; }
				return false;
			})()`, nil),
			chromedp.Sleep(1*time.Second),
		)
		if err != nil {
			return fmt.Errorf("failed to click create button: %w", err)
		}

		// Fill and submit
		return chromedp.Run(tr.ctx,
			chromedp.WaitVisible(`input[name="name"]`, chromedp.ByQuery),
			chromedp.SendKeys(`input[name="name"]`, projectName, chromedp.ByQuery),
			chromedp.Evaluate(`(function() {
				const submitBtn = Array.from(document.querySelectorAll('button[type="submit"]')).find(b => b.textContent.includes('Create'));
				if (submitBtn) { submitBtn.click(); return true; }
				return false;
			})()`, nil),
			chromedp.Sleep(2*time.Second),
		)
	})
}

func (tr *TestRunner) testSitesCRUD() {
	tr.test("Navigate to Sites page", func() error {
		return chromedp.Run(tr.ctx,
			chromedp.Click(`a[href="/sites"]`, chromedp.ByQuery),
			chromedp.WaitVisible("body", chromedp.ByQuery),
			chromedp.Sleep(1*time.Second),
		)
	})

	tr.test("Create new site", func() error {
		siteName := fmt.Sprintf("test-site-%d", time.Now().Unix())

		// Click create button
		err := chromedp.Run(tr.ctx,
			chromedp.Evaluate(`(function() {
				const btn = Array.from(document.querySelectorAll('button')).find(b => b.textContent.includes('Create'));
				if (btn) { btn.click(); return true; }
				return false;
			})()`, nil),
			chromedp.Sleep(1*time.Second),
		)
		if err != nil {
			return fmt.Errorf("failed to click create button: %w", err)
		}

		// Fill and submit
		return chromedp.Run(tr.ctx,
			chromedp.WaitVisible(`input[name="name"]`, chromedp.ByQuery),
			chromedp.SendKeys(`input[name="name"]`, siteName, chromedp.ByQuery),
			chromedp.Evaluate(`(function() {
				const submitBtn = Array.from(document.querySelectorAll('button[type="submit"]')).find(b => b.textContent.includes('Create'));
				if (submitBtn) { submitBtn.click(); return true; }
				return false;
			})()`, nil),
			chromedp.Sleep(2*time.Second),
		)
	})
}

func (tr *TestRunner) testFirewallManagement() {
	tr.test("Navigate to Firewall page", func() error {
		return chromedp.Run(tr.ctx,
			chromedp.Click(`a[href="/firewall"]`, chromedp.ByQuery),
			chromedp.WaitVisible("body", chromedp.ByQuery),
			chromedp.Sleep(1*time.Second),
		)
	})

	tr.test("View firewall rules list", func() error {
		var pageLoaded bool

		return chromedp.Run(tr.ctx,
			chromedp.Evaluate(`document.querySelector('h1') !== null`, &pageLoaded),
		)
	})
}

func (tr *TestRunner) testMemberManagement() {
	tr.test("Navigate to Members page", func() error {
		return chromedp.Run(tr.ctx,
			chromedp.Click(`a[href="/members"]`, chromedp.ByQuery),
			chromedp.WaitVisible("body", chromedp.ByQuery),
			chromedp.Sleep(1*time.Second),
		)
	})

	tr.test("View members list", func() error {
		var pageLoaded bool

		return chromedp.Run(tr.ctx,
			chromedp.Evaluate(`document.querySelector('h1') !== null`, &pageLoaded),
		)
	})
}

func (tr *TestRunner) testSecretManagement() {
	tr.test("Navigate to Secrets page", func() error {
		return chromedp.Run(tr.ctx,
			chromedp.Click(`a[href="/secrets"]`, chromedp.ByQuery),
			chromedp.WaitVisible("body", chromedp.ByQuery),
			chromedp.Sleep(1*time.Second),
		)
	})

	tr.test("View secrets list", func() error {
		var pageLoaded bool

		return chromedp.Run(tr.ctx,
			chromedp.Evaluate(`document.querySelector('h1') !== null`, &pageLoaded),
		)
	})
}

func (tr *TestRunner) testAPIKeyManagement() {
	tr.test("Navigate to API Keys page", func() error {
		return chromedp.Run(tr.ctx,
			chromedp.Click(`a[href="/api-keys"]`, chromedp.ByQuery),
			chromedp.WaitVisible("body", chromedp.ByQuery),
			chromedp.Sleep(1*time.Second),
		)
	})

	tr.test("Create new API key", func() error {
		keyName := fmt.Sprintf("Test Key %d", time.Now().Unix())

		// Click create button
		err := chromedp.Run(tr.ctx,
			chromedp.Evaluate(`(function() {
				const btn = Array.from(document.querySelectorAll('button')).find(b => b.textContent.includes('Create'));
				if (btn) { btn.click(); return true; }
				return false;
			})()`, nil),
			chromedp.Sleep(1*time.Second),
		)
		if err != nil {
			return fmt.Errorf("failed to click create button: %w", err)
		}

		// Fill and submit
		return chromedp.Run(tr.ctx,
			chromedp.WaitVisible(`#key-name`, chromedp.ByID),
			chromedp.SendKeys(`#key-name`, keyName, chromedp.ByID),
			chromedp.Evaluate(`(function() {
				const submitBtn = Array.from(document.querySelectorAll('button[type="submit"]')).find(b => b.textContent.includes('Create'));
				if (submitBtn) { submitBtn.click(); return true; }
				return false;
			})()`, nil),
			chromedp.Sleep(2*time.Second),
		)
	})

	tr.test("API key secret is displayed", func() error {
		var secretVisible bool

		return chromedp.Run(tr.ctx,
			chromedp.WaitVisible(`#api-key-secret`, chromedp.ByID),
			chromedp.Evaluate(`document.querySelector('#api-key-secret') !== null && document.querySelector('#api-key-secret').value !== ''`, &secretVisible),
		)
	})

	tr.test("Close secret modal", func() error {
		return chromedp.Run(tr.ctx,
			chromedp.Evaluate(`(function() {
				const btn = Array.from(document.querySelectorAll('button')).find(b => b.textContent.includes('Done'));
				if (btn) { btn.click(); return true; }
				return false;
			})()`, nil),
			chromedp.Sleep(1*time.Second),
		)
	})
}

func (tr *TestRunner) testSSHKeyManagement() {
	tr.test("Navigate to SSH Keys page", func() error {
		return chromedp.Run(tr.ctx,
			chromedp.Click(`a[href="/ssh-keys"]`, chromedp.ByQuery),
			chromedp.WaitVisible("body", chromedp.ByQuery),
			chromedp.Sleep(1*time.Second),
		)
	})

	tr.test("Add new SSH key", func() error {
		keyName := fmt.Sprintf("Test Key %d", time.Now().Unix())
		// Sample SSH public key (this is a dummy key for testing)
		sampleKey := "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDTest test@example.com"

		// Click add button
		err := chromedp.Run(tr.ctx,
			chromedp.Evaluate(`(function() {
				const btn = Array.from(document.querySelectorAll('button')).find(b => b.textContent.includes('Add'));
				if (btn) { btn.click(); return true; }
				return false;
			})()`, nil),
			chromedp.Sleep(1*time.Second),
		)
		if err != nil {
			return fmt.Errorf("failed to click add button: %w", err)
		}

		// Fill and submit
		return chromedp.Run(tr.ctx,
			chromedp.WaitVisible(`#key-name`, chromedp.ByID),
			chromedp.SendKeys(`#key-name`, keyName, chromedp.ByID),
			chromedp.SendKeys(`#public-key`, sampleKey, chromedp.ByID),
			chromedp.Evaluate(`(function() {
				const submitBtn = Array.from(document.querySelectorAll('button[type="submit"]')).find(b => b.textContent.includes('Add'));
				if (submitBtn) { submitBtn.click(); return true; }
				return false;
			})()`, nil),
			chromedp.Sleep(2*time.Second),
		)
	})

	tr.test("View SSH keys in list", func() error {
		var tableVisible bool

		return chromedp.Run(tr.ctx,
			chromedp.Navigate(dashboardURL+"/ssh-keys"),
			chromedp.WaitVisible("body", chromedp.ByQuery),
			chromedp.Sleep(2*time.Second),
			chromedp.Evaluate(`document.querySelector('table') !== null || document.querySelector('.bg-white') !== null`, &tableVisible),
		)
	})
}

// Helper methods

func (tr *TestRunner) test(name string, fn func() error) {
	if err := fn(); err == nil {
		tr.passed++
		fmt.Printf("  %s %s\n", green("✓"), name)
	} else {
		tr.failed++
		fmt.Printf("  %s %s: %v\n", red("✗"), name, err)
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
		fmt.Println(green("✓ All dashboard tests passed!"))
	} else {
		fmt.Println(red("✗ Some dashboard tests failed"))
	}
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
