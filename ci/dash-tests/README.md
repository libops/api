# Dashboard E2E Tests

Headless Chrome tests for the LibOps dashboard using chromedp.

## Overview

This test suite runs automated browser tests against the LibOps dashboard to verify:

1. **Login Flow** - Ensures the login page loads without errors and authentication works
2. **Organizations CRUD** - Create, Read, Update, Delete operations for organizations
3. **Projects CRUD** - Create, Read, Update, Delete operations for projects
4. **Sites CRUD** - Create, Read, Update, Delete operations for sites
5. **Firewall Management** - Access and manage firewall rules
6. **Member Management** - Access and manage organization/project/site members
7. **Secret Management** - Access and manage secrets
8. **API Key Management** - Create and manage API keys through the dashboard

## Test Flow

The tests run in sequence after the integration tests complete successfully:

1. Integration tests seed the database with test data (admin@libops.io user with password123)
2. Dashboard E2E tests launch headless Chrome
3. Tests authenticate using the seeded admin credentials
4. Tests perform CRUD operations and verify dashboard functionality

## Running Locally

From the `ci/` directory:

```bash
# Run all tests (integration + dashboard)
./run-tests.sh

# Run with build
./run-tests.sh --build

# Run with cleanup
./run-tests.sh --clean
```

## Running Just Dashboard Tests

```bash
# From ci/ directory
docker compose up --build dash-tests
```

## Environment Variables

- `DASHBOARD_URL` - Base URL of the dashboard (default: `http://api:8080`)
- `HEADLESS` - Run in headless mode (default: `true`)

## Architecture

- **Chromedp** - Go library for driving Chrome via DevTools Protocol
- **Headless Chrome** - Browser automation without GUI
- **Test Sequencing** - Tests run after integration tests to ensure data is seeded

## Test Credentials

The tests use credentials created by the integration test seed data:
- Email: `admin@libops.io`
- Password: `password123`

## Adding New Tests

To add new dashboard tests:

1. Add a new test function in `main.go`
2. Call it from `RunAllTests()` in the appropriate phase
3. Use the `tr.test()` helper method to track pass/fail status

Example:

```go
func (tr *TestRunner) testMyNewFeature() {
    tr.test("My new feature works", func() error {
        return chromedp.Run(tr.ctx,
            chromedp.Navigate(dashboardURL+"/my-feature"),
            chromedp.WaitVisible("h1", chromedp.ByQuery),
            // ... more actions
        )
    })
}
```

## Debugging

To see the browser in action, set `HEADLESS=false`:

```bash
HEADLESS=false docker compose up dash-tests
```

Note: This requires running locally with a display server.
