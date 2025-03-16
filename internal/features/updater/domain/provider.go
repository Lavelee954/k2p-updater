package domain

import (
	"context"
	"time"
)

// Provider defines the interface for the updater service
type Provider interface {
	// Start begins the updater service processing
	Start(ctx context.Context) error

	// GetStateMachine returns the state machine instance
	GetStateMachine() StateMachine

	// RequestSpecUp requests a spec up for a node
	RequestSpecUp(ctx context.Context, nodeName string) error

	// VerifySpecUpHealth verifies the health of a node after spec up
	VerifySpecUpHealth(ctx context.Context, nodeName string) (bool, error)

	// GetNodeCPUUtilization gets the current CPU utilization for a node
	GetNodeCPUUtilization(ctx context.Context, nodeName string) (float64, float64, error)

	// IsCooldownActive checks if a node is in cooldown period
	IsCooldownActive(ctx context.Context, nodeName string) (bool, time.Duration, error)

	// Stop method for clean shutdown
	Stop()
}

// BackendClient defines the interface for communicating with the backend API
type BackendClient interface {
	// RequestVMSpecUp sends a spec up request to the backend
	RequestVMSpecUp(ctx context.Context, nodeName string, currentCPU float64) (bool, error)

	// GetVMSpecUpStatus checks the status of a spec up request
	GetVMSpecUpStatus(ctx context.Context, nodeName string) (bool, error)
}

// HealthVerifier defines the interface for verifying node health
type HealthVerifier interface {
	// VerifyNodeHealth checks if a node is healthy after spec up
	VerifyNodeHealth(ctx context.Context, nodeName string) (bool, error)
}
