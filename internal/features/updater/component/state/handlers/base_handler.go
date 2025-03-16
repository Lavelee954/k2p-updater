// base_handler.go

package handlers

import (
	"context"
	"k2p-updater/internal/common"
	"k2p-updater/internal/features/updater/domain/models"
	"k2p-updater/pkg/resource"
)

// BaseStateHandler provides common functionality for all state handlers
type BaseStateHandler struct {
	resourceFactory *resource.Factory
	supportedEvents []models.Event
}

// NewBaseStateHandler creates a new base handler
func NewBaseStateHandler(resourceFactory *resource.Factory, events []models.Event) *BaseStateHandler {
	return &BaseStateHandler{
		resourceFactory: resourceFactory,
		supportedEvents: events,
	}
}

// SupportedEvents returns the list of events supported by this state
func (h *BaseStateHandler) SupportedEvents() []models.Event {
	return h.supportedEvents
}

// OnEnter default implementation (no-op)
func (h *BaseStateHandler) OnEnter(ctx context.Context, status *models.ControlPlaneStatus) (*models.ControlPlaneStatus, error) {
	// Check context
	if err := common.CheckContextWithOp(ctx, "entering state"); err != nil {
		return nil, err
	}
	return status, nil
}

// OnExit default implementation (no-op)
func (h *BaseStateHandler) OnExit(ctx context.Context, status *models.ControlPlaneStatus) (*models.ControlPlaneStatus, error) {
	// Check context
	if err := common.CheckContextWithOp(ctx, "exiting state"); err != nil {
		return nil, err
	}
	return status, nil
}

// IsEventSupported checks if an event is supported
func (h *BaseStateHandler) IsEventSupported(event models.Event) bool {
	for _, e := range h.supportedEvents {
		if e == event {
			return true
		}
	}
	return false
}
