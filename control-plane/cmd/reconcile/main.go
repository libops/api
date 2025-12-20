package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"cloud.google.com/go/pubsub"
)

// SiteReconciliationRequest matches the structure expected by Event Router
type SiteReconciliationRequest struct {
	SitePublicID    string   `json:"site_public_id"`
	ProjectPublicID string   `json:"project_public_id"`
	OrgPublicID     string   `json:"org_public_id"`
	RequestType     string   `json:"request_type"` // "ssh_keys", "secrets", "firewall", "full"
	EventIDs        []string `json:"event_ids"`
	Timestamp       string   `json:"timestamp"`
}

func main() {
	// CLI flags
	var (
		sitePublicID    = flag.String("site-public-id", "", "Site Public ID (UUID) (required)")
		projectPublicID = flag.String("project-public-id", "", "Project Public ID (UUID) (required)")
		orgPublicID     = flag.String("org-public-id", "", "Organization Public ID (UUID) (required)")
		requestType     = flag.String("type", "full", "Reconciliation type: ssh_keys, secrets, firewall, full")
		gcpProject      = flag.String("gcp-project", "", "GCP project ID where Pub/Sub topic is (required)")
		topic           = flag.String("topic", "libops-control-plane", "Pub/Sub topic name")
		dryRun          = flag.Bool("dry-run", false, "Print message but don't publish")
	)

	flag.Parse()

	// Validate required flags
	if *sitePublicID == "" {
		fmt.Fprintf(os.Stderr, "Error: --site-public-id is required\n")
		flag.Usage()
		os.Exit(1)
	}
	if *projectPublicID == "" {
		fmt.Fprintf(os.Stderr, "Error: --project-public-id is required\n")
		flag.Usage()
		os.Exit(1)
	}
	if *orgPublicID == "" {
		fmt.Fprintf(os.Stderr, "Error: --org-public-id is required\n")
		flag.Usage()
		os.Exit(1)
	}
	if *gcpProject == "" && !*dryRun {
		fmt.Fprintf(os.Stderr, "Error: --gcp-project is required (unless --dry-run)\n")
		flag.Usage()
		os.Exit(1)
	}

	// Validate request type
	validTypes := map[string]bool{
		"ssh_keys": true,
		"secrets":  true,
		"firewall": true,
		"full":     true,
	}
	if !validTypes[*requestType] {
		fmt.Fprintf(os.Stderr, "Error: --type must be one of: ssh_keys, secrets, firewall, full\n")
		os.Exit(1)
	}

	ctx := context.Background()

	// Build reconciliation request
	req := SiteReconciliationRequest{
		SitePublicID:    *sitePublicID,
		ProjectPublicID: *projectPublicID,
		OrgPublicID:     *orgPublicID,
		RequestType:     *requestType,
		EventIDs:        []string{"manual-trigger"},
		Timestamp:       time.Now().UTC().Format(time.RFC3339),
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(req, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to marshal request: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("📋 Reconciliation Request:\n%s\n\n", string(data))

	if *dryRun {
		fmt.Println("✓ Dry-run mode: Message not published")
		fmt.Printf("\nTo publish, run:\n")
		fmt.Printf("  gcloud pubsub topics publish %s \\\n", *topic)
		fmt.Printf("    --project=%s \\\n", *gcpProject)
		fmt.Printf("    --message='%s' \\\n", string(data))
		fmt.Printf("    --attribute=site_public_id=%s \\\n", *sitePublicID)
		fmt.Printf("    --attribute=project_public_id=%s \\\n", *projectPublicID)
		fmt.Printf("    --attribute=org_public_id=%s \\\n", *orgPublicID)
		fmt.Printf("    --attribute=request_type=%s\n", *requestType)
		return
	}

	// Create Pub/Sub client
	client, err := pubsub.NewClient(ctx, *gcpProject)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create pubsub client: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	// Get topic
	t := client.Topic(*topic)
	exists, err := t.Exists(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to check topic existence: %v\n", err)
		os.Exit(1)
	}
	if !exists {
		fmt.Fprintf(os.Stderr, "Error: Topic '%s' does not exist in project '%s'\n", *topic, *gcpProject)
		os.Exit(1)
	}

	// Publish message
	slog.Info("Publishing to Pub/Sub",
		"project", *gcpProject,
		"topic", *topic,
		"site_public_id", *sitePublicID,
		"type", *requestType)

	result := t.Publish(ctx, &pubsub.Message{
		Data: data,
		Attributes: map[string]string{
			"site_public_id":    *sitePublicID,
			"project_public_id": *projectPublicID,
			"org_public_id":     *orgPublicID,
			"request_type":      *requestType,
		},
	})

	// Wait for publish result
	messageID, err := result.Get(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to publish message: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Published reconciliation request to Pub/Sub\n")
	fmt.Printf("  Topic: %s\n", *topic)
	fmt.Printf("  Message ID: %s\n", messageID)
	fmt.Printf("  Site Public ID: %s\n", *sitePublicID)
	fmt.Printf("  Type: %s\n", *requestType)
	fmt.Printf("\n🔄 Event Router will receive this message and notify the site controller\n")
	fmt.Printf("   The site will reconcile: %s\n", *requestType)
}
