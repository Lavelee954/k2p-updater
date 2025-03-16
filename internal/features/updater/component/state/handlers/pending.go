package handlers

import (
	"context"
	"fmt"
	"k2p-updater/internal/common"
	"k2p-updater/internal/features/updater/domain/interfaces"
	"k2p-updater/internal/features/updater/domain/models"
	"k2p-updater/pkg/resource"
	"log"
	"time"
)

// pendingHandler handles the PendingVmSpecUp state
type pendingHandler struct {
	*BaseStateHandler
}

// NewPendingHandler creates a new handler for PendingVmSpecUp state
func NewPendingHandler(resourceFactory *resource.Factory) interfaces.StateHandler {
	// Define supported events for this state
	supportedEvents := []models.Event{
		models.EventInitialize,
		models.EventCooldownEnded,
	}

	return &pendingHandler{
		BaseStateHandler: NewBaseStateHandler(resourceFactory, supportedEvents),
	}
}

// Handle processes events for the PendingVmSpecUp state
func (h *pendingHandler) Handle(ctx context.Context, status *models.ControlPlaneStatus,
	event models.Event, data map[string]interface{}) (*models.ControlPlaneStatus, error) {

	// Check context first
	if err := common.CheckContextWithOp(ctx, "handling pending state event"); err != nil {
		return nil, err
	}

	// Verify the event is supported
	if !h.IsEventSupported(event) {
		return nil, fmt.Errorf("event %s is not supported in PendingVmSpecUp state", event)
	}

	// Create a copy of the status to work with
	newStatus := *status

	switch event {
	case models.EventInitialize:
		// Stay in the same state, just update the message
		newStatus.Message = "Pending VM spec up, in cooldown period"

		// Update CPU metrics if available
		if data != nil {
			if cpu, ok := data["cpuUtilization"].(float64); ok {
				newStatus.CPUUtilization = cpu
			}
			if windowAvg, ok := data["windowAverageUtilization"].(float64); ok {
				newStatus.WindowAverageUtilization = windowAvg
			}
		}

	case models.EventCooldownEnded:
		// Transition to Monitoring state when cooldown ends
		newStatus.CurrentState = models.StateMonitoring
		newStatus.Message = "Monitoring CPU utilization"

		// Record CPU metrics if available
		if data != nil {
			if cpu, ok := data["cpuUtilization"].(float64); ok {
				newStatus.CPUUtilization = cpu
			}
			if windowAvg, ok := data["windowAverageUtilization"].(float64); ok {
				newStatus.WindowAverageUtilization = windowAvg
			}
		}
	}

	// Final context check
	if err := common.CheckContextWithOp(ctx, "completing pending handler"); err != nil {
		return nil, err
	}

	return &newStatus, nil
}

// OnEnter is called when entering the PendingVmSpecUp state
func (h *pendingHandler) OnEnter(ctx context.Context, status *models.ControlPlaneStatus) (*models.ControlPlaneStatus, error) {
	// Call base method first for context check
	if _, err := h.BaseStateHandler.OnEnter(ctx, status); err != nil {
		return nil, err
	}

	newStatus := *status

	// Record the event
	err := h.resourceFactory.Event().NormalRecordWithNode(
		ctx,
		models.UpdateKey,
		status.NodeName,
		"PendingVmSpecUp",
		"Node %s is in cooldown period, pending VM spec up",
		status.NodeName,
	)

	if err != nil {
		log.Printf("Failed to record event for node %s: %v", status.NodeName, err)
		// Don't return error as we don't want to prevent state transition
	} else {
		log.Printf("Successfully recorded event for node %s", status.NodeName)
	}

	// If this is the first run and CoolDown is true (as per requirements)
	// Set CoolDown to true on initial startup of the application
	if newStatus.CoolDownEndTime.IsZero() {
		// Default cooldown period of 10 minutes
		cooldownPeriod := 10 * time.Minute
		newStatus.CoolDownEndTime = time.Now().Add(cooldownPeriod)
		newStatus.Message = fmt.Sprintf("Initial startup cooldown period for %.1f minutes",
			cooldownPeriod.Minutes())
	}

	return &newStatus, nil
}

// OnExit is called when exiting the PendingVmSpecUp state
func (h *pendingHandler) OnExit(ctx context.Context, status *models.ControlPlaneStatus) (*models.ControlPlaneStatus, error) {
	// Call base method first for context check
	if _, err := h.BaseStateHandler.OnExit(ctx, status); err != nil {
		return nil, err
	}

	// Record transition event
	err := h.resourceFactory.Event().NormalRecordWithNode(
		ctx,
		models.UpdateKey,
		status.NodeName,
		string(models.StatePendingVmSpecUp),
		"Node %s exiting pending state to begin active operations",
		status.NodeName,
	)

	if err != nil {
		log.Printf("Failed to record exit event for node %s: %v", status.NodeName, err)
		// Don't return error as we don't want to prevent state transition
	}

	return status, nil
}
