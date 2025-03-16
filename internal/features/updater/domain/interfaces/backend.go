package interfaces

import (
	"context"
)

// BackendClient defines the interface for communicating with the backend API
type BackendClient interface {
	// RequestVMSpecUp sends a spec up request to the backend
	RequestVMSpecUp(ctx context.Context, nodeName string, currentCPU float64) (bool, error)

	// GetVMSpecUpStatus checks the status of a spec up request
	GetVMSpecUpStatus(ctx context.Context, nodeName string) (bool, error)
}
