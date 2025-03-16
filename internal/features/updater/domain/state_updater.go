package domain

import (
	"context"
)

// StateUpdater defines an abstraction for updating node states
// This interface is used to break circular dependencies
type StateUpdater interface {
	// GetCurrentState returns the current state for a node
	GetCurrentState(ctx context.Context, nodeName string) (State, error)

	// GetStatus retrieves the current status of a node
	GetStatus(ctx context.Context, nodeName string) (*ControlPlaneStatus, error)

	// HandleEvent processes an event for a node and potentially updates its state
	HandleEvent(ctx context.Context, nodeName string, Event Event, data map[string]interface{}) error

	// UpdateStatus updates the status for a node directly
	UpdateStatus(ctx context.Context, nodeName string, status *ControlPlaneStatus) error
}
