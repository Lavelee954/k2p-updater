package interfaces

import (
	"context"
)

// Coordinator defines the interface for coordinating spec up operations
type Coordinator interface {
	// IsAnyNodeSpecingUp checks if any node is currently being spec'd up
	IsAnyNodeSpecingUp(ctx context.Context, nodes []string) (bool, string, error)

	// NextEligibleNode finds the next node eligible for spec up
	NextEligibleNode(ctx context.Context, nodes []string) (string, error)
}
