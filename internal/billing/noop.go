package billing

import (
	"context"
	"log/slog"
)

// NoOpBillingManager is a no-op billing manager for testing/development
// It satisfies the BillingManager interface but doesn't perform any Stripe operations
type NoOpBillingManager struct{}

// NewNoOpBillingManager creates a new no-op billing manager
func NewNoOpBillingManager() *NoOpBillingManager {
	slog.Warn("Using NoOp billing manager - Stripe billing is disabled")
	return &NoOpBillingManager{}
}

// ValidateMachineType always returns nil (allows any machine type in test mode)
func (n *NoOpBillingManager) ValidateMachineType(ctx context.Context, machineType string) error {
	return nil
}

// ValidateDiskSize always returns nil (allows any disk size in test mode)
func (n *NoOpBillingManager) ValidateDiskSize(ctx context.Context, diskSizeGB int) error {
	return nil
}

// AddProjectToSubscription returns a fake subscription item ID
func (n *NoOpBillingManager) AddProjectToSubscription(ctx context.Context, organizationID int64, projectName, machineType string, diskSizeGB int) (machineItemID string, err error) {
	return "noop_subscription_item", nil
}

// RemoveProjectFromSubscription does nothing
func (n *NoOpBillingManager) RemoveProjectFromSubscription(ctx context.Context, machineItemID string, diskSizeGB int, organizationID int64) error {
	return nil
}

// UpdateProjectMachine returns a fake subscription item ID
func (n *NoOpBillingManager) UpdateProjectMachine(ctx context.Context, oldMachineItemID, newMachineType, projectName string, organizationID int64) (newMachineItemID string, err error) {
	return "noop_subscription_item_updated", nil
}

// UpdateProjectDiskSize does nothing
func (n *NoOpBillingManager) UpdateProjectDiskSize(ctx context.Context, organizationID int64, oldDiskSizeGB, newDiskSizeGB int) error {
	return nil
}

// GetMachineTypePriceID returns a fake price ID
func (n *NoOpBillingManager) GetMachineTypePriceID(ctx context.Context, machineType string) (string, error) {
	return "noop_price_id", nil
}

// CreateCheckoutSession returns a fake checkout session (skips Stripe redirect)
func (n *NoOpBillingManager) CreateCheckoutSession(ctx context.Context, accountEmail, sessionID, machineType string, diskSizeGB int, baseURL string, withTrial bool) (*CheckoutSessionResult, error) {
	// Return empty checkout result - no URL means no redirect needed
	return &CheckoutSessionResult{
		SessionID: "noop_checkout_session",
		URL:       "", // Empty URL signals to skip Stripe redirect
	}, nil
}
