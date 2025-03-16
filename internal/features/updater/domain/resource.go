package domain

import (
	"context"
)

// ResourceFactory defines the interface for accessing resource operations
type ResourceFactory interface {
	// Event returns an interface for event operations
	Event() EventRecorder
	// Status returns an interface for status operations
	Status() StatusUpdater
}

// EventRecorder defines operations for recording events
type EventRecorder interface {
	// NormalRecordWithNode records a normal event for a specific node
	NormalRecordWithNode(ctx context.Context, component, nodeName, reason, messageFmt string, args ...interface{}) error

	// WarningRecordWithNode records a warning event for a specific node
	WarningRecordWithNode(ctx context.Context, component, nodeName, reason, messageFmt string, args ...interface{}) error
}

// StatusUpdater defines operations for updating status
type StatusUpdater interface {
	// UpdateGenericWithNode updates a generic status with node information
	UpdateGenericWithNode(ctx context.Context, component, nodeName string, statusData map[string]interface{}) error
}
