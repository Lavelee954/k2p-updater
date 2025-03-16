package resource

import (
	"context"

	"k2p-updater/internal/features/updater/domain/interfaces"
	"k2p-updater/pkg/resource"
)

// ResourceFactoryAdapter adapts the pkg/resource.Factory to the domain.ResourceFactory interface
type ResourceFactoryAdapter struct {
	factory *resource.Factory
}

// NewResourceFactoryAdapter creates a new resource factory adapter
func NewResourceFactoryAdapter(factory *resource.Factory) interfaces.ResourceFactory {
	return &ResourceFactoryAdapter{
		factory: factory,
	}
}

// Event returns the event recorder interface
func (r *ResourceFactoryAdapter) Event() interfaces.EventRecorder {
	return &EventRecorderAdapter{
		event: r.factory.Event(),
	}
}

// Status returns the status updater interface
func (r *ResourceFactoryAdapter) Status() interfaces.StatusUpdater {
	return &StatusUpdaterAdapter{
		status: r.factory.Status(),
	}
}

// EventRecorderAdapter adapts the pkg/resource.Event to the domain.EventRecorder interface
type EventRecorderAdapter struct {
	event resource.Event
}

// NormalRecordWithNode records a normal event for a specific node
func (e *EventRecorderAdapter) NormalRecordWithNode(ctx context.Context, component, nodeName, reason, messageFmt string, args ...interface{}) error {
	return e.event.NormalRecordWithNode(ctx, component, nodeName, reason, messageFmt, args...)
}

// WarningRecordWithNode records a warning event for a specific node
func (e *EventRecorderAdapter) WarningRecordWithNode(ctx context.Context, component, nodeName, reason, messageFmt string, args ...interface{}) error {
	return e.event.WarningRecordWithNode(ctx, component, nodeName, reason, messageFmt, args...)
}

// StatusUpdaterAdapter adapts the pkg/resource.Status to the domain.StatusUpdater interface
type StatusUpdaterAdapter struct {
	status resource.Status
}

// UpdateGenericWithNode updates a generic status with node information
func (s *StatusUpdaterAdapter) UpdateGenericWithNode(ctx context.Context, component, nodeName string, statusData map[string]interface{}) error {
	return s.status.UpdateGenericWithNode(ctx, component, nodeName, statusData)
}
