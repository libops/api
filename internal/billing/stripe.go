package billing

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/libops/api/internal/db"
	"github.com/stripe/stripe-go/v84"
	"github.com/stripe/stripe-go/v84/checkout/session"
	"github.com/stripe/stripe-go/v84/subscriptionitem"
)

const (
	// TrialPeriodDays is the number of days for the trial period
	TrialPeriodDays = 7
)

// StripeManager handles Stripe subscription operations
type StripeManager struct {
	db              db.Querier
	webhookSecret   string
	stripeKey       string
	stripeSecretKey string
}

// NewStripeManager creates a new Stripe manager
func NewStripeManager(querier db.Querier) *StripeManager {
	return &StripeManager{
		db: querier,
	}
}

// NewStripeManagerWithWebhook creates a new Stripe manager with webhook support
func NewStripeManagerWithWebhook(querier db.Querier, webhookSecret, stripeKey string) *StripeManager {
	return &StripeManager{
		db:              querier,
		webhookSecret:   webhookSecret,
		stripeKey:       stripeKey,
		stripeSecretKey: stripeKey,
	}
}

// GetMachineTypePriceID gets the Stripe price ID for a machine type from database
func (sm *StripeManager) GetMachineTypePriceID(ctx context.Context, machineType string) (string, error) {
	mt, err := sm.db.GetMachineType(ctx, machineType)
	if err != nil {
		return "", fmt.Errorf("machine type not found: %w", err)
	}
	return mt.StripePriceID, nil
}

// GetStoragePriceID gets the Stripe price ID for disk storage from database
func (sm *StripeManager) GetStoragePriceID(ctx context.Context) (string, error) {
	config, err := sm.db.GetStorageConfig(ctx)
	if err != nil {
		return "", fmt.Errorf("storage config not found: %w", err)
	}
	return config.StripePriceID, nil
}

// ValidateMachineType checks if a machine type exists and is active
func (sm *StripeManager) ValidateMachineType(ctx context.Context, machineType string) error {
	_, err := sm.db.GetMachineType(ctx, machineType)
	if err != nil {
		return fmt.Errorf("invalid or inactive machine type: %s", machineType)
	}
	return nil
}

// ValidateDiskSize checks if disk size is within allowed limits
func (sm *StripeManager) ValidateDiskSize(ctx context.Context, diskSizeGB int) error {
	config, err := sm.db.GetStorageConfig(ctx)
	if err != nil {
		return fmt.Errorf("storage config not found: %w", err)
	}

	if diskSizeGB < int(config.MinSizeGb) || diskSizeGB > int(config.MaxSizeGb) {
		return fmt.Errorf("disk size must be between %d and %d GB", config.MinSizeGb, config.MaxSizeGb)
	}
	return nil
}

// AddProjectToSubscription adds a project's machine and disk to an organization's Stripe subscription
// Each project has its own machine + disk allocation
// Returns the machine subscription item ID that should be stored with the project
func (sm *StripeManager) AddProjectToSubscription(ctx context.Context, organizationID int64, projectName, machineType string, diskSizeGB int) (machineItemID string, err error) {
	// Get the organization's subscription
	subscription, err := sm.db.GetStripeSubscriptionByOrganizationID(ctx, organizationID)
	if err != nil {
		return "", fmt.Errorf("failed to get subscription: %w", err)
	}

	// Get machine price ID from database
	machinePriceID, err := sm.GetMachineTypePriceID(ctx, machineType)
	if err != nil {
		return "", fmt.Errorf("failed to get machine price ID: %w", err)
	}

	// Add machine subscription item
	machineParams := &stripe.SubscriptionItemParams{
		Subscription: stripe.String(subscription.StripeSubscriptionID),
		Price:        stripe.String(machinePriceID),
		Quantity:     stripe.Int64(1),
		Metadata: map[string]string{
			"project_name": projectName,
			"type":         "machine",
			"machine_type": machineType,
		},
	}
	machineParams.ProrationBehavior = stripe.String("create_prorations")

	machineItem, err := subscriptionitem.New(machineParams)
	if err != nil {
		return "", fmt.Errorf("failed to create machine subscription item: %w", err)
	}

	// Update disk storage quantity to add this project's disk
	// Find the existing disk subscription item
	diskItemID, err := sm.findDiskSubscriptionItem(ctx, subscription.StripeSubscriptionID)
	if err != nil {
		// Rollback machine item if disk update fails
		_, _ = subscriptionitem.Del(machineItem.ID, nil)
		return "", fmt.Errorf("failed to find disk subscription item: %w", err)
	}

	// Get current disk quantity and add new disk
	currentDisk, err := subscriptionitem.Get(diskItemID, nil)
	if err != nil {
		_, _ = subscriptionitem.Del(machineItem.ID, nil)
		return "", fmt.Errorf("failed to get current disk quantity: %w", err)
	}

	newDiskQuantity := currentDisk.Quantity + int64(diskSizeGB)
	diskParams := &stripe.SubscriptionItemParams{
		Quantity:          stripe.Int64(newDiskQuantity),
		ProrationBehavior: stripe.String("create_prorations"),
	}

	_, err = subscriptionitem.Update(diskItemID, diskParams)
	if err != nil {
		// Rollback machine item if disk update fails
		_, _ = subscriptionitem.Del(machineItem.ID, nil)
		return "", fmt.Errorf("failed to update disk quantity: %w", err)
	}

	return machineItem.ID, nil
}

// RemoveProjectFromSubscription removes a project's machine and disk from an organization's Stripe subscription
func (sm *StripeManager) RemoveProjectFromSubscription(ctx context.Context, machineItemID string, diskSizeGB int, organizationID int64) error {
	if machineItemID == "" {
		// Project doesn't have a subscription item (maybe created before billing was set up)
		return nil
	}

	// Get the subscription to find disk item
	subscription, err := sm.db.GetStripeSubscriptionByOrganizationID(ctx, organizationID)
	if err != nil {
		return fmt.Errorf("failed to get subscription: %w", err)
	}

	// Delete the machine subscription item
	machineParams := &stripe.SubscriptionItemParams{
		ProrationBehavior: stripe.String("create_prorations"),
	}
	_, err = subscriptionitem.Del(machineItemID, machineParams)
	if err != nil {
		return fmt.Errorf("failed to delete machine subscription item: %w", err)
	}

	// Update disk storage quantity to remove this project's disk
	diskItemID, err := sm.findDiskSubscriptionItem(ctx, subscription.StripeSubscriptionID)
	if err != nil {
		return fmt.Errorf("failed to find disk subscription item: %w", err)
	}

	currentDisk, err := subscriptionitem.Get(diskItemID, nil)
	if err != nil {
		return fmt.Errorf("failed to get current disk quantity: %w", err)
	}

	newDiskQuantity := currentDisk.Quantity - int64(diskSizeGB)
	if newDiskQuantity < 0 {
		newDiskQuantity = 0
	}

	diskParams := &stripe.SubscriptionItemParams{
		Quantity:          stripe.Int64(newDiskQuantity),
		ProrationBehavior: stripe.String("create_prorations"),
	}

	_, err = subscriptionitem.Update(diskItemID, diskParams)
	if err != nil {
		return fmt.Errorf("failed to update disk quantity: %w", err)
	}

	return nil
}

// CreateCheckoutSession creates a Stripe checkout session for the onboarding flow
// It queries the database for machine pricing and storage configuration
func (sm *StripeManager) CreateCheckoutSession(ctx context.Context, accountEmail, sessionID, machineType string, diskSizeGB int, baseURL string) (*CheckoutSessionResult, error) {
	// Validate machine type and get price ID from database
	if err := sm.ValidateMachineType(ctx, machineType); err != nil {
		return nil, fmt.Errorf("invalid machine type: %w", err)
	}

	machinePriceID, err := sm.GetMachineTypePriceID(ctx, machineType)
	if err != nil {
		return nil, fmt.Errorf("failed to get machine price ID: %w", err)
	}

	// Validate disk size and get storage config from database
	if err := sm.ValidateDiskSize(ctx, diskSizeGB); err != nil {
		return nil, err
	}

	storageConfig, err := sm.db.GetStorageConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get storage config: %w", err)
	}

	diskPriceID := storageConfig.StripePriceID
	minDiskGB := int64(storageConfig.MinSizeGb)
	maxDiskGB := int64(storageConfig.MaxSizeGb)

	params := &stripe.CheckoutSessionParams{
		Mode: stripe.String(string(stripe.CheckoutSessionModeSubscription)),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				Price:    stripe.String(machinePriceID),
				Quantity: stripe.Int64(1),
			},
			{
				Price:    stripe.String(diskPriceID),
				Quantity: stripe.Int64(int64(diskSizeGB)),
				AdjustableQuantity: &stripe.CheckoutSessionLineItemAdjustableQuantityParams{
					Enabled: stripe.Bool(true),
					Minimum: stripe.Int64(minDiskGB),
					Maximum: stripe.Int64(maxDiskGB),
				},
			},
		},
		SuccessURL:        stripe.String(fmt.Sprintf("%s/onboarding/stripe/success?session_id={CHECKOUT_SESSION_ID}", baseURL)),
		CancelURL:         stripe.String(fmt.Sprintf("%s/onboarding/stripe/cancel", baseURL)),
		CustomerEmail:     stripe.String(accountEmail),
		ClientReferenceID: stripe.String(sessionID),
		SubscriptionData: &stripe.CheckoutSessionSubscriptionDataParams{
			TrialPeriodDays: stripe.Int64(TrialPeriodDays),
		},
	}

	s, err := session.New(params)
	if err != nil {
		return nil, fmt.Errorf("failed to create checkout session: %w", err)
	}

	return &CheckoutSessionResult{
		SessionID: s.ID,
		URL:       s.URL,
	}, nil
}

// findDiskSubscriptionItem finds the disk storage subscription item in a subscription
func (sm *StripeManager) findDiskSubscriptionItem(ctx context.Context, subscriptionID string) (string, error) {
	// Get disk storage price ID from database
	diskPriceID, err := sm.GetStoragePriceID(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get storage price ID: %w", err)
	}

	params := &stripe.SubscriptionItemListParams{
		Subscription: stripe.String(subscriptionID),
	}

	iter := subscriptionitem.List(params)
	for iter.Next() {
		item := iter.SubscriptionItem()
		// Check if this is the disk storage item
		if item.Price.ID == diskPriceID {
			return item.ID, nil
		}
	}

	if err := iter.Err(); err != nil {
		return "", err
	}

	return "", fmt.Errorf("disk subscription item not found")
}

// UpdateProjectMachine updates the machine type for a project
// This swaps out the old machine subscription item for a new one
func (sm *StripeManager) UpdateProjectMachine(ctx context.Context, oldMachineItemID, newMachineType, projectName string, organizationID int64) (newMachineItemID string, err error) {
	// Get subscription
	subscription, err := sm.db.GetStripeSubscriptionByOrganizationID(ctx, organizationID)
	if err != nil {
		return "", fmt.Errorf("failed to get subscription: %w", err)
	}

	// Get new machine price ID from database
	newMachinePriceID, err := sm.GetMachineTypePriceID(ctx, newMachineType)
	if err != nil {
		return "", fmt.Errorf("failed to get machine price ID: %w", err)
	}

	// Add new machine
	machineParams := &stripe.SubscriptionItemParams{
		Subscription: stripe.String(subscription.StripeSubscriptionID),
		Price:        stripe.String(newMachinePriceID),
		Quantity:     stripe.Int64(1),
		Metadata: map[string]string{
			"project_name": projectName,
			"type":         "machine",
			"machine_type": newMachineType,
		},
		ProrationBehavior: stripe.String("create_prorations"),
	}

	newMachineItem, err := subscriptionitem.New(machineParams)
	if err != nil {
		return "", fmt.Errorf("failed to create new machine subscription item: %w", err)
	}

	// Remove old machine
	delParams := &stripe.SubscriptionItemParams{
		ProrationBehavior: stripe.String("create_prorations"),
	}
	_, err = subscriptionitem.Del(oldMachineItemID, delParams)
	if err != nil {
		// Rollback: delete the new item
		_, _ = subscriptionitem.Del(newMachineItem.ID, nil)
		return "", fmt.Errorf("failed to delete old machine subscription item: %w", err)
	}

	return newMachineItem.ID, nil
}

// UpdateProjectDiskSize updates the disk size for a project
// This adjusts the disk storage quantity in the subscription
func (sm *StripeManager) UpdateProjectDiskSize(ctx context.Context, organizationID int64, oldDiskSizeGB, newDiskSizeGB int) error {
	// Get subscription
	subscription, err := sm.db.GetStripeSubscriptionByOrganizationID(ctx, organizationID)
	if err != nil {
		return fmt.Errorf("failed to get subscription: %w", err)
	}

	// Find disk subscription item
	diskItemID, err := sm.findDiskSubscriptionItem(ctx, subscription.StripeSubscriptionID)
	if err != nil {
		return fmt.Errorf("failed to find disk subscription item: %w", err)
	}

	// Get current disk quantity
	currentDisk, err := subscriptionitem.Get(diskItemID, nil)
	if err != nil {
		return fmt.Errorf("failed to get current disk quantity: %w", err)
	}

	// Calculate new total: remove old project disk, add new project disk
	diskDifference := int64(newDiskSizeGB - oldDiskSizeGB)
	newDiskQuantity := currentDisk.Quantity + diskDifference

	if newDiskQuantity < 0 {
		newDiskQuantity = 0
	}

	// Update disk quantity
	diskParams := &stripe.SubscriptionItemParams{
		Quantity:          stripe.Int64(newDiskQuantity),
		ProrationBehavior: stripe.String("create_prorations"),
	}

	_, err = subscriptionitem.Update(diskItemID, diskParams)
	if err != nil {
		return fmt.Errorf("failed to update disk quantity: %w", err)
	}

	slog.Info("Updated project disk size in Stripe",
		"organization_id", organizationID,
		"old_disk_gb", oldDiskSizeGB,
		"new_disk_gb", newDiskSizeGB,
		"total_disk_gb", newDiskQuantity)

	return nil
}
