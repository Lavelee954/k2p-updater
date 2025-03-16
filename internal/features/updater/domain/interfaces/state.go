package interfaces

import (
	"context"

	"k2p-updater/internal/features/updater/domain/models"
)

// StateMachine defines the interface for the VM spec up state machine
type StateMachine interface {
	// GetCurrentState returns the current state of the state machine
	GetCurrentState(ctx context.Context, nodeName string) (models.State, error)

	// HandleEvent processes an event and performs the appropriate state transition
	HandleEvent(ctx context.Context, nodeName string, event models.Event, data map[string]interface{}) error

	// GetStatus returns the full status of a control plane node
	GetStatus(ctx context.Context, nodeName string) (*models.ControlPlaneStatus, error)

	// UpdateStatus updates the status in the custom resource
	UpdateStatus(ctx context.Context, nodeName string, status *models.ControlPlaneStatus) error

	// Close shuts down the state machine gracefully
	Close()
}

// StateHandler defines the interface for individual state handlers
type StateHandler interface {
	// Handle processes events for a specific state
	Handle(ctx context.Context, status *models.ControlPlaneStatus, event models.Event, data map[string]interface{}) (*models.ControlPlaneStatus, error)

	// OnEnter is called when entering this state
	OnEnter(ctx context.Context, status *models.ControlPlaneStatus) (*models.ControlPlaneStatus, error)

	// OnExit is called when exiting this state
	OnExit(ctx context.Context, status *models.ControlPlaneStatus) (*models.ControlPlaneStatus, error)
}

// StateUpdater defines an abstraction for updating node states
type StateUpdater interface {
	// GetCurrentState returns the current state for a node
	GetCurrentState(ctx context.Context, nodeName string) (models.State, error)

	// GetStatus retrieves the current status of a node
	GetStatus(ctx context.Context, nodeName string) (*models.ControlPlaneStatus, error)

	// HandleEvent processes an event for a node and potentially updates its state
	HandleEvent(ctx context.Context, nodeName string, event models.Event, data map[string]interface{}) error

	// UpdateStatus updates the status for a node directly
	UpdateStatus(ctx context.Context, nodeName string, status *models.ControlPlaneStatus) error
}
