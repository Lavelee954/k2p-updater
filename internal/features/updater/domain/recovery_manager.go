package domain

import (
	"context"
)

// RecoveryManager defines the interface for handling node recovery
type RecoveryManager interface {
	// AttemptRecovery attempts to recover a node from a failed state
	AttemptRecovery(ctx context.Context, nodeName string) error

	// CheckAllNodes attempts recovery for all nodes in failed state
	CheckAllNodes(ctx context.Context, nodes []string)

	// ResetRecoveryCounter resets the recovery counter for a node
	ResetRecoveryCounter(nodeName string)
}
