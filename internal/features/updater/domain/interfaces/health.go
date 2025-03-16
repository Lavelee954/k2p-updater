package interfaces

import (
	"context"
)

// HealthVerifier defines the interface for verifying node health
type HealthVerifier interface {
	// VerifyNodeHealth checks if a node is healthy after spec up
	VerifyNodeHealth(ctx context.Context, nodeName string) (bool, error)
}
