package billing

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/libops/api/internal/db"
	"github.com/stripe/stripe-go/v84"
	"github.com/stripe/stripe-go/v84/subscriptionitem"
	"github.com/stripe/stripe-go/v84/webhook"
)

// HandleStripeWebhook handles Stripe webhook events
func (sm *StripeManager) HandleStripeWebhook(w http.ResponseWriter, r *http.Request) {
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Error("Failed to read webhook payload", "error", err)
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	// Verify webhook signature
	event, err := webhook.ConstructEvent(payload, r.Header.Get("Stripe-Signature"), sm.webhookSecret)
	if err != nil {
		slog.Error("Failed to verify webhook signature", "error", err)
		http.Error(w, "Invalid signature", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Handle the event
	switch event.Type {
	case "checkout.session.completed":
		var session stripe.CheckoutSession
		if err := json.Unmarshal(event.Data.Raw, &session); err != nil {
			slog.Error("Failed to parse checkout session", "error", err)
			http.Error(w, "Failed to parse event", http.StatusBadRequest)
			return
		}

		if err := sm.handleCheckoutSessionCompleted(ctx, &session); err != nil {
			slog.Error("Failed to handle checkout.session.completed", "error", err, "session_id", session.ID)
			http.Error(w, "Failed to process webhook", http.StatusInternalServerError)
			return
		}

	case "checkout.session.expired":
		var session stripe.CheckoutSession
		if err := json.Unmarshal(event.Data.Raw, &session); err != nil {
			slog.Error("Failed to parse checkout session", "error", err)
			http.Error(w, "Failed to parse event", http.StatusBadRequest)
			return
		}

		if err := sm.handleCheckoutSessionExpired(ctx, &session); err != nil {
			slog.Error("Failed to handle checkout.session.expired", "error", err, "session_id", session.ID)
			http.Error(w, "Failed to process webhook", http.StatusInternalServerError)
			return
		}

	case "customer.subscription.updated":
		slog.Info("Received customer.subscription.updated event")
		// Handle subscription updates if needed

	case "customer.subscription.deleted":
		slog.Info("Received customer.subscription.deleted event")
		// Handle subscription cancellation if needed

	default:
		slog.Warn("Unhandled webhook event type", "type", event.Type)
	}

	w.WriteHeader(http.StatusOK)
}

// handleCheckoutSessionCompleted adds subscription details to the organization after successful payment
// Note: Organization is now created in Step 1, so this only adds billing/subscription info
func (sm *StripeManager) handleCheckoutSessionCompleted(ctx context.Context, session *stripe.CheckoutSession) error {
	// Get onboarding session by client reference ID (onboarding session public ID)
	onboardSession, err := sm.db.GetOnboardingSession(ctx, session.ClientReferenceID)
	if err != nil {
		return fmt.Errorf("failed to get onboarding session: %w", err)
	}

	// Organization should already exist from Step 1
	if !onboardSession.OrganizationID.Valid {
		return fmt.Errorf("organization not created yet for session %s", onboardSession.PublicID)
	}

	// Get the existing organization
	org, err := sm.db.GetOrganizationByID(ctx, onboardSession.OrganizationID.Int64)
	if err != nil {
		return fmt.Errorf("failed to get organization: %w", err)
	}

	// Create Stripe subscription record
	subscriptionID := session.Subscription.ID
	customerID := session.Customer.ID

	var trialStart, trialEnd sql.NullTime
	if session.Subscription != nil {
		if session.Subscription.TrialStart != 0 {
			trialStart = sql.NullTime{Time: time.Unix(session.Subscription.TrialStart, 0), Valid: true}
		}
		if session.Subscription.TrialEnd != 0 {
			trialEnd = sql.NullTime{Time: time.Unix(session.Subscription.TrialEnd, 0), Valid: true}
		}
	}

	// Find the machine subscription item ID created during checkout
	// This is needed for the project record so we can manage billing later
	machineItemID, err := sm.findMachineSubscriptionItemByMachineType(ctx, subscriptionID, onboardSession.MachineType.String)
	if err != nil {
		slog.Error("Failed to find machine subscription item in webhook", "error", err, "subscription_id", subscriptionID)
		// Continue anyway - we'll try again in step 7
		machineItemID = ""
	}

	_, err = sm.db.CreateStripeSubscription(ctx, db.CreateStripeSubscriptionParams{
		OrganizationID:          org.ID,
		StripeSubscriptionID:    subscriptionID,
		StripeCustomerID:        customerID,
		StripeCheckoutSessionID: sql.NullString{String: session.ID, Valid: true},
		Status:                  db.StripeSubscriptionsStatusTrialing,
		CurrentPeriodStart:      sql.NullTime{Valid: false},
		CurrentPeriodEnd:        sql.NullTime{Valid: false},
		TrialStart:              trialStart,
		TrialEnd:                trialEnd,
		CancelAtPeriodEnd:       sql.NullBool{Bool: false, Valid: true},
		CanceledAt:              sql.NullTime{Valid: false},
		MachineType:             onboardSession.MachineType,
		DiskSizeGb:              onboardSession.DiskSizeGb,
	})

	if err != nil {
		return fmt.Errorf("failed to create stripe subscription: %w", err)
	}

	// Update onboarding session with organization ID, machine item ID, and advance to step 4
	machinePriceID := onboardSession.MachinePriceID
	if machineItemID != "" {
		// Store the machine subscription item ID for later use in step 7
		machinePriceID = sql.NullString{String: machineItemID, Valid: true}
	}

	err = sm.db.UpdateOnboardingSession(ctx, db.UpdateOnboardingSessionParams{
		OrgName:                 onboardSession.OrgName,
		MachineType:             onboardSession.MachineType,
		MachinePriceID:          machinePriceID, // Store machine item ID here (reusing this field)
		DiskSizeGb:              onboardSession.DiskSizeGb,
		StripeCheckoutSessionID: onboardSession.StripeCheckoutSessionID,
		StripeCheckoutUrl:       onboardSession.StripeCheckoutUrl,
		StripeSubscriptionID:    sql.NullString{String: subscriptionID, Valid: true},
		OrganizationID:          sql.NullInt64{Int64: org.ID, Valid: true},
		ProjectName:             onboardSession.ProjectName,
		GcpCountry:              onboardSession.GcpCountry,
		GcpRegion:               onboardSession.GcpRegion,
		SiteName:                onboardSession.SiteName,
		GithubRepoUrl:           onboardSession.GithubRepoUrl,
		Port:                    onboardSession.Port,
		FirewallIp:              onboardSession.FirewallIp,
		CurrentStep:             sql.NullInt32{Int32: 4, Valid: true}, // Advance to step 4 after payment
		Completed:               onboardSession.Completed,
		ID:                      onboardSession.ID,
	})

	if err != nil {
		slog.Error("Failed to update onboarding session", "error", err)
		// Don't fail the webhook - organization was created successfully
	}

	slog.Info("Organization created successfully from Stripe checkout",
		"organization_id", org.PublicID,
		"subscription_id", subscriptionID,
		"account_id", onboardSession.AccountID)

	return nil
}

// handleCheckoutSessionExpired resets the onboarding session to step 2 when checkout expires
func (sm *StripeManager) handleCheckoutSessionExpired(ctx context.Context, session *stripe.CheckoutSession) error {
	// Get onboarding session by Stripe checkout session ID
	onboardSession, err := sm.db.GetOnboardingSessionByStripeCheckoutID(ctx, sql.NullString{String: session.ID, Valid: true})
	if err != nil {
		if err == sql.ErrNoRows {
			slog.Warn("Onboarding session not found for expired checkout", "checkout_session_id", session.ID)
			return nil // Not an error - session may have already been cleaned up
		}
		return fmt.Errorf("failed to get onboarding session: %w", err)
	}

	// Check if organization was already created (checkout completed before expiring)
	if onboardSession.OrganizationID.Valid {
		slog.Info("Checkout expired but organization already created", "session_id", onboardSession.PublicID)
		return nil
	}

	// Reset onboarding session to step 2 and clear checkout session ID
	err = sm.db.UpdateOnboardingSession(ctx, db.UpdateOnboardingSessionParams{
		OrgName:                 onboardSession.OrgName,
		MachineType:             onboardSession.MachineType,
		MachinePriceID:          onboardSession.MachinePriceID,
		DiskSizeGb:              onboardSession.DiskSizeGb,
		StripeCheckoutSessionID: sql.NullString{Valid: false}, // Clear expired checkout session
		StripeCheckoutUrl:       sql.NullString{Valid: false}, // Clear expired checkout URL
		StripeSubscriptionID:    onboardSession.StripeSubscriptionID,
		OrganizationID:          onboardSession.OrganizationID,
		ProjectName:             onboardSession.ProjectName,
		GcpCountry:              onboardSession.GcpCountry,
		GcpRegion:               onboardSession.GcpRegion,
		SiteName:                onboardSession.SiteName,
		GithubRepoUrl:           onboardSession.GithubRepoUrl,
		Port:                    onboardSession.Port,
		FirewallIp:              onboardSession.FirewallIp,
		CurrentStep:             sql.NullInt32{Int32: 2, Valid: true}, // Reset to step 2
		Completed:               onboardSession.Completed,
		ID:                      onboardSession.ID,
	})

	if err != nil {
		return fmt.Errorf("failed to update onboarding session: %w", err)
	}

	slog.Info("Onboarding session reset to step 2 due to expired checkout",
		"session_id", onboardSession.PublicID,
		"account_id", onboardSession.AccountID,
		"checkout_session_id", session.ID)

	return nil
}

// findMachineSubscriptionItemByMachineType finds the machine subscription item by querying
// the subscription and matching the price ID based on machine type
func (sm *StripeManager) findMachineSubscriptionItemByMachineType(ctx context.Context, subscriptionID, machineType string) (string, error) {
	// Get the expected machine price ID from database
	machinePriceID, err := sm.GetMachineTypePriceID(ctx, machineType)
	if err != nil {
		return "", fmt.Errorf("failed to get machine price ID: %w", err)
	}

	// Query Stripe for subscription items
	params := &stripe.SubscriptionItemListParams{
		Subscription: stripe.String(subscriptionID),
	}

	iter := subscriptionitem.List(params)
	for iter.Next() {
		item := iter.SubscriptionItem()
		// Check if this item's price matches the machine price
		if item.Price.ID == machinePriceID {
			slog.Info("Found machine subscription item",
				"item_id", item.ID,
				"machine_type", machineType,
				"price_id", machinePriceID)
			return item.ID, nil
		}
	}

	if err := iter.Err(); err != nil {
		return "", fmt.Errorf("error listing subscription items: %w", err)
	}

	return "", fmt.Errorf("machine subscription item not found for machine type: %s", machineType)
}
