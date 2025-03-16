package interfaces

import (
	"context"
	"time"
)

// LifecycleManager defines methods for managing service lifecycle
type LifecycleManager interface {
	// Start begins the updater service processing
	Start(ctx context.Context) error

	// Stop performs a clean shutdown
	Stop()
}

// NodeOperator defines operations that can be performed on nodes
type NodeOperator interface {
	// RequestSpecUp requests a spec up for a node
	RequestSpecUp(ctx context.Context, nodeName string) error

	// VerifySpecUpHealth verifies the health of a node after spec up
	VerifySpecUpHealth(ctx context.Context, nodeName string) (bool, error)

	// GetNodeCPUUtilization gets the current CPU utilization for a node
	GetNodeCPUUtilization(ctx context.Context, nodeName string) (float64, float64, error)

	// IsCooldownActive checks if a node is in cooldown period
	IsCooldownActive(ctx context.Context, nodeName string) (bool, time.Duration, error)
}

// StateAccessor provides access to the state machine
type StateAccessor interface {
	// GetStateMachine returns the state machine instance
	GetStateMachine() StateMachine
}

// Provider defines the interface for the updater service through composition
// of more focused interfaces following the Interface Segregation Principle
type Provider interface {
	LifecycleManager
	NodeOperator
	StateAccessor
}
