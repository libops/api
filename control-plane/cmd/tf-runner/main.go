package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "github.com/go-sql-driver/mysql"
	"github.com/libops/api/db"
	"github.com/libops/api/db/types"
)

func main() {
	// CLI flags
	var (
		orgID      = flag.Int64("org-id", 0, "Organization ID (required)")
		projectID  = flag.Int64("project-id", 0, "Project ID (optional, for project-level runs)")
		siteID     = flag.Int64("site-id", 0, "Site ID (optional, for site-level runs)")
		dryRun     = flag.Bool("dry-run", false, "Create run but don't trigger Cloud Run job")
		watch      = flag.Bool("watch", false, "Watch the job execution and tail logs")
		bootstrap  = flag.Bool("bootstrap", false, "Bootstrap organization (create folder, project, and state bucket)")
		gcpProject = flag.String("gcp-project", "", "GCP project ID where Cloud Run job is deployed (required)")
		region     = flag.String("region", "us-central1", "GCP region where Cloud Run job is deployed")
		jobName    = flag.String("job", "libops-terraform-runner", "Cloud Run job name")
	)

	flag.Parse()

	// Validate required flags
	if *orgID == 0 {
		fmt.Fprintf(os.Stderr, "Error: --org-id is required\n")
		flag.Usage()
		os.Exit(1)
	}

	if *bootstrap && (*projectID != 0 || *siteID != 0) {
		fmt.Fprintf(os.Stderr, "Error: --bootstrap cannot be used with --project-id or --site-id\n")
		os.Exit(1)
	}

	if *gcpProject == "" && !*dryRun {
		fmt.Fprintf(os.Stderr, "Error: --gcp-project is required (unless --dry-run)\n")
		flag.Usage()
		os.Exit(1)
	}

	// Get database connection from environment
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		fmt.Fprintf(os.Stderr, "Error: DATABASE_URL environment variable is required\n")
		os.Exit(1)
	}

	ctx := context.Background()

	// Connect to database
	sqlDB, err := sql.Open("mysql", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer sqlDB.Close()

	queries := db.New(sqlDB)

	// Fetch organization to get public_id
	org, err := queries.GetOrganizationByID(ctx, *orgID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to fetch organization: %v\n", err)
		os.Exit(1)
	}

	// Determine scope and modules
	var scope string
	var modules []string
	var projID, sID *int64

	if *bootstrap {
		scope = "bootstrap"
		modules = []string{"organization"}
		slog.Info("Bootstrapping organization", "org_id", *orgID, "public_id", org.PublicID)
	} else if *siteID != 0 {
		scope = "site"
		modules = []string{"site"}
		projID = projectID
		if *projectID != 0 {
			projID = projectID
		}
		sID = siteID
		slog.Info("Running terraform for site", "org_id", *orgID, "project_id", *projectID, "site_id", *siteID)
	} else if *projectID != 0 {
		scope = "project"
		modules = []string{"organization", "project"}
		projID = projectID
		slog.Info("Running terraform for project", "org_id", *orgID, "project_id", *projectID)
	} else {
		scope = "organization"
		modules = []string{"organization"}
		slog.Info("Running terraform for organization", "org_id", *orgID)
	}

	// Determine GCP project for the job
	targetProject := *gcpProject
	if targetProject == "" {
		if *bootstrap {
			targetProject = os.Getenv("LIBOPS_ORCHESTRATOR_PROJECT")
		} else {
			if org.GcpProjectID.Valid {
				targetProject = org.GcpProjectID.String
			}
		}
	}

	if targetProject == "" && !*dryRun {
		if *bootstrap {
			fmt.Fprintf(os.Stderr, "Error: --gcp-project or LIBOPS_ORCHESTRATOR_PROJECT env var is required for bootstrap\n")
		} else {
			fmt.Fprintf(os.Stderr, "Error: --gcp-project is required (organization %d has no gcp_project_id set)\n", *orgID)
		}
		os.Exit(1)
	}

	// Generate run ID
	runID := fmt.Sprintf("manual-%s-%s-%s", scope, time.Now().Format("20060102-150405"), uuid.New().String()[:8])

	// Create reconciliation run in database
	modulesJSON, err := json.Marshal(modules)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to marshal modules: %v\n", err)
		os.Exit(1)
	}

	eventIDsJSON, err := json.Marshal([]string{"manual-trigger"})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to marshal event IDs: %v\n", err)
		os.Exit(1)
	}

	now := time.Now()

	var orgIDParam, projIDParam, sIDParam sql.NullInt64
	orgIDParam = sql.NullInt64{Int64: *orgID, Valid: true}
	if projID != nil {
		projIDParam = sql.NullInt64{Int64: *projID, Valid: true}
	}
	if sID != nil {
		sIDParam = sql.NullInt64{Int64: *sID, Valid: true}
	}

	params := db.CreateReconciliationRunParams{
		RunID:              runID,
		OrganizationID:     orgIDParam,
		ProjectID:          projIDParam,
		SiteID:             sIDParam,
		RunType:            db.ReconciliationsRunTypeTerraform,
		ReconciliationType: db.NullReconciliationsReconciliationType{},
		Modules:            types.RawJSON(modulesJSON),
		TargetSiteIds:      types.RawJSON("[]"),
		EventIds:           eventIDsJSON,
		FirstEventAt:       now,
		LastEventAt:        now,
	}

	slog.Info("Creating reconciliation run", "run_id", runID, "modules", modules)

	if _, err := queries.CreateReconciliationRun(ctx, params); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create reconciliation run: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Created reconciliation run: %s\n", runID)
	fmt.Printf("  Scope: %s\n", scope)
	fmt.Printf("  Modules: %s\n", strings.Join(modules, ", "))

	stateBucket := fmt.Sprintf("libops-org-%s-tfstate", org.PublicID[:8])
	envVars := []string{
		"RUN_ID=" + runID,
		"TERRAFORM_STATE_BUCKET=" + stateBucket,
	}
	if *bootstrap {
		envVars = append(envVars, "BOOTSTRAP=true")
	}

	if *dryRun {
		fmt.Println("\n✓ Dry-run mode: Skipping Cloud Run job execution")
		fmt.Println("\nTo trigger manually, run:")
		fmt.Printf("  gcloud run jobs execute %s \\\n", *jobName)
		fmt.Printf("    --project=%s \\\n", targetProject)
		fmt.Printf("    --region=%s \\\n", *region)
		fmt.Printf("    --set-env-vars=%s\n", strings.Join(envVars, ","))
		return
	}

	// Update status to triggered
	if err := queries.UpdateReconciliationRunTriggered(ctx, runID); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to update run status: %v\n", err)
		os.Exit(1)
	}

	// Trigger Cloud Run job
	fmt.Printf("\n⚡ Triggering Cloud Run job in project %s...\n", targetProject)

	args := []string{
		"run", "jobs", "execute", *jobName,
		"--project=" + targetProject,
		"--region=" + *region,
		"--set-env-vars=" + strings.Join(envVars, ","),
	}

	if *watch {
		args = append(args, "--wait")
	}

	cmd := exec.CommandContext(ctx, "gcloud", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "\nFailed to execute Cloud Run job: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n✓ Cloud Run job triggered successfully\n")
	fmt.Printf("\nRun ID: %s\n", runID)

	if *watch {
		// Poll for final status
		fmt.Println("\n⏳ Checking final status...")
		time.Sleep(2 * time.Second)

		run, err := queries.GetReconciliationRunByID(ctx, runID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to get run status: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Final Status: %s\n", run.Status)
		if run.ErrorMessage.Valid {
			fmt.Printf("Error: %s\n", run.ErrorMessage.String)
		}
	} else {
		fmt.Println("\nMonitor with:")
		fmt.Printf("  gcloud run jobs executions list \\\n")
		fmt.Printf("    --project=%s \\\n", targetProject)
		fmt.Printf("    --region=%s \\\n", *region)
		fmt.Printf("    --job=%s\n", *jobName)
	}
}

