package billing

import "context"

// Manager defines the interface for billing operations used across the application
// This interface is implemented by both StripeManager (production) and NoOpBillingManager (testing/dev)
type Manager interface {
	// Project billing operations
	ValidateMachineType(ctx context.Context, machineType string) error
	ValidateDiskSize(ctx context.Context, diskSizeGB int) error
	AddProjectToSubscription(ctx context.Context, organizationID int64, projectName, machineType string, diskSizeGB int) (machineItemID string, err error)
	RemoveProjectFromSubscription(ctx context.Context, machineItemID string, diskSizeGB int, organizationID int64) error
	UpdateProjectMachine(ctx context.Context, oldMachineItemID, newMachineType, projectName string, organizationID int64) (newMachineItemID string, err error)
	UpdateProjectDiskSize(ctx context.Context, organizationID int64, oldDiskSizeGB, newDiskSizeGB int) error

	// Onboarding operations
	GetMachineTypePriceID(ctx context.Context, machineType string) (string, error)
	CreateCheckoutSession(ctx context.Context, accountEmail, sessionID, machineType string, diskSizeGB int, baseURL string, withTrial bool) (*CheckoutSessionResult, error)
}

// CheckoutSessionResult contains the checkout session ID and URL
type CheckoutSessionResult struct {
	SessionID string
	URL       string // Empty URL means skip redirect (for NoOp billing)
}
