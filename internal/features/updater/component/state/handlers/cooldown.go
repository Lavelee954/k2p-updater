package handlers

import (
	"context"
	"fmt"
	"k2p-updater/internal/common"
	"k2p-updater/internal/features/updater/domain/interfaces"
	"k2p-updater/internal/features/updater/domain/models"
	"k2p-updater/pkg/resource"
	"log"
	"sync"
	"time"
)

// coolDownHandler handles the CoolDown state
type coolDownHandler struct {
	*BaseStateHandler
	lastEventTime  map[string]time.Time
	lastStatusTime map[string]time.Time
	eventMutex     sync.RWMutex
	eventInterval  time.Duration // Interval between events
	statusInterval time.Duration // Interval between CR status updates
}

// NewCoolDownHandler creates a new handler for CoolDown state
func NewCoolDownHandler(resourceFactory *resource.Factory) interfaces.StateHandler {
	// Define supported events for this state
	supportedEvents := []models.Event{
		models.EventInitialize,
		models.EventCooldownStatus,
		models.EventCooldownEnded,
	}

	return &coolDownHandler{
		BaseStateHandler: NewBaseStateHandler(resourceFactory, supportedEvents),
		lastEventTime:    make(map[string]time.Time),
		lastStatusTime:   make(map[string]time.Time),
		eventInterval:    5 * time.Minute,
		statusInterval:   1 * time.Minute,
	}
}

// Handle processes events for the CoolDown state
func (h *coolDownHandler) Handle(ctx context.Context, status *models.ControlPlaneStatus,
	event models.Event, data map[string]interface{}) (*models.ControlPlaneStatus, error) {

	// Check context first
	if err := common.CheckContextWithOp(ctx, "handling cooldown state event"); err != nil {
		return nil, err
	}

	// Verify the event is supported
	if !h.IsEventSupported(event) {
		return nil, fmt.Errorf("event %s is not supported in CoolDown state", event)
	}

	// Create a copy of the status to work with
	newStatus := *status

	switch event {
	case models.EventInitialize:
		// Stay in the same state, just update the message
		if !newStatus.CoolDownEndTime.IsZero() {
			remaining := time.Until(newStatus.CoolDownEndTime)
			if remaining < 0 {
				remaining = 0
			}
			newStatus.Message = fmt.Sprintf("In cooldown period, %.1f minutes remaining",
				remaining.Minutes())
		} else {
			newStatus.Message = "In cooldown period"
		}

		// Update CPU metrics if available
		if data != nil {
			if cpu, ok := data["cpuUtilization"].(float64); ok {
				newStatus.CPUUtilization = cpu
			}
			if windowAvg, ok := data["windowAverageUtilization"].(float64); ok {
				newStatus.WindowAverageUtilization = windowAvg
			}
		}

	case models.EventCooldownStatus:
		// Update cooldown remaining message
		if !newStatus.CoolDownEndTime.IsZero() {
			remaining := time.Until(newStatus.CoolDownEndTime)
			if remaining < 0 {
				remaining = 0
			}
			newStatus.Message = fmt.Sprintf("In cooldown period, %.1f minutes remaining",
				remaining.Minutes())
		}

		// Update CPU metrics if available
		if data != nil {
			if cpu, ok := data["cpuUtilization"].(float64); ok {
				newStatus.CPUUtilization = cpu
			}
			if windowAvg, ok := data["windowAverageUtilization"].(float64); ok {
				newStatus.WindowAverageUtilization = windowAvg
			}
		}

		// Check if it's time to update CR status (every 1 minute)
		h.eventMutex.RLock()
		lastStatusUpdate, statusExists := h.lastStatusTime[status.NodeName]
		h.eventMutex.RUnlock()

		now := time.Now()
		if !statusExists || now.Sub(lastStatusUpdate) > h.statusInterval {
			// Update the last transition time to ensure the CR message gets updated
			newStatus.LastTransitionTime = now

			// Update the last status update time
			h.eventMutex.Lock()
			h.lastStatusTime[status.NodeName] = now
			h.eventMutex.Unlock()

			log.Printf("Updating CR status for node %s (1-minute interval): %.1f minutes cooldown remaining",
				status.NodeName, time.Until(newStatus.CoolDownEndTime).Minutes())
		}

		// Log the status update
		log.Printf("COOLDOWN STATUS: Node %s cooldown status updated: %.1f minutes remaining, CPU: %.2f%%, Avg: %.2f%%",
			status.NodeName,
			time.Until(newStatus.CoolDownEndTime).Minutes(),
			newStatus.CPUUtilization,
			newStatus.WindowAverageUtilization)

	case models.EventCooldownEnded:
		// Transition to Monitoring state when cooldown ends
		newStatus.CurrentState = models.StateMonitoring
		newStatus.Message = "Cooldown period ended, now monitoring CPU utilization"

		// Record CPU metrics if available
		if data != nil {
			if cpu, ok := data["cpuUtilization"].(float64); ok {
				newStatus.CPUUtilization = cpu
			}
			if windowAvg, ok := data["windowAverageUtilization"].(float64); ok {
				newStatus.WindowAverageUtilization = windowAvg
			}
		}

		log.Printf("COOLDOWN ENDED: Node %s transitioning from CoolDown to Monitoring",
			status.NodeName)
	}

	// Final context check
	if err := common.CheckContextWithOp(ctx, "completing cooldown handler"); err != nil {
		return nil, err
	}

	return &newStatus, nil
}

// OnEnter is called when entering the CoolDown state
func (h *coolDownHandler) OnEnter(ctx context.Context, status *models.ControlPlaneStatus) (*models.ControlPlaneStatus, error) {
	// Call base method first for context check
	if _, err := h.BaseStateHandler.OnEnter(ctx, status); err != nil {
		return nil, err
	}

	newStatus := *status

	// Make sure CoolDownEndTime is set
	if newStatus.CoolDownEndTime.IsZero() {
		// Default cooldown period (should come from config)
		cooldownPeriod := 5 * time.Minute
		newStatus.CoolDownEndTime = time.Now().Add(cooldownPeriod)
	}

	// Calculate cooldown duration
	cooldownMinutes := time.Until(newStatus.CoolDownEndTime).Minutes()

	// Record the event
	err := h.resourceFactory.Event().NormalRecordWithNode(
		ctx,
		models.UpdateKey,
		status.NodeName,
		"CoolDown",
		"Node %s entered cooldown period for %.1f minutes after VM spec up",
		status.NodeName,
		cooldownMinutes,
	)

	if err != nil {
		log.Printf("Failed to record cooldown entry event for node %s: %v", status.NodeName, err)
		// Don't return error as we don't want to prevent state transition
	} else {
		log.Printf("Successfully recorded cooldown entry event for node %s", status.NodeName)
	}

	newStatus.Message = fmt.Sprintf("In cooldown period, %.1f minutes remaining", cooldownMinutes)

	return &newStatus, nil
}

// OnExit is called when exiting the CoolDown state
func (h *coolDownHandler) OnExit(ctx context.Context, status *models.ControlPlaneStatus) (*models.ControlPlaneStatus, error) {
	// Call base method first for context check
	if _, err := h.BaseStateHandler.OnExit(ctx, status); err != nil {
		return nil, err
	}

	// Record the exit event
	err := h.resourceFactory.Event().NormalRecordWithNode(
		ctx,
		models.UpdateKey,
		status.NodeName,
		"ExitingCooldown",
		"Node %s exiting cooldown period after %.1f minutes",
		status.NodeName,
		time.Since(status.LastTransitionTime).Minutes(),
	)

	if err != nil {
		log.Printf("Failed to record cooldown exit event for node %s: %v", status.NodeName, err)
		// Don't return error as we don't want to prevent state transition
	}

	return status, nil
}
