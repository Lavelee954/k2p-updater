package handlers

import (
	"context"
	"fmt"
	"k2p-updater/internal/features/updater/domain/models"
	"log"
	"sync"
	"time"
)

// testMonitoringHandler is a specialized version for testing
type testMonitoringHandler struct {
	lastEventTime   map[string]time.Time
	eventMutex      sync.RWMutex
	eventInterval   time.Duration
	mockFactory     *mockResourceFactory
	supportedEvents []models.Event
}

// NewTestMonitoringHandler creates a handler specifically for testing
func NewTestMonitoringHandler() *testMonitoringHandler {
	supportedEvents := []models.Event{
		models.EventThresholdExceeded,
		models.EventMonitoringStatus,
		models.EventInitialize,
	}

	return &testMonitoringHandler{
		lastEventTime:   make(map[string]time.Time),
		eventInterval:   10 * time.Minute,
		mockFactory:     createMockResourceFactory(),
		supportedEvents: supportedEvents,
	}
}

// SupportedEvents returns the list of supported events
func (h *testMonitoringHandler) SupportedEvents() []models.Event {
	return h.supportedEvents
}

// IsEventSupported checks if an event is supported
func (h *testMonitoringHandler) IsEventSupported(event models.Event) bool {
	for _, e := range h.supportedEvents {
		if e == event {
			return true
		}
	}
	return false
}

// Handle handles events for testing
func (h *testMonitoringHandler) Handle(ctx context.Context, status *models.ControlPlaneStatus,
	event models.Event, data map[string]interface{}) (*models.ControlPlaneStatus, error) {

	// Create a copy of the status to work with
	newStatus := *status

	log.Printf("MONITORING HANDLER: Processing event %s for node %s", event, status.NodeName)

	switch event {
	case models.EventThresholdExceeded:
		log.Printf("MONITORING HANDLER: CPU threshold exceeded for node %s, transitioning to InProgressVmSpecUp",
			status.NodeName)
		newStatus.CurrentState = models.StateInProgressVmSpecUp
		newStatus.Message = "CPU threshold exceeded, initiating VM spec up"

	case models.EventMonitoringStatus:
		log.Printf("MONITORING HANDLER: Status update event for node %s", status.NodeName)
		if data != nil {
			if cpu, ok := data["cpuUtilization"].(float64); ok {
				newStatus.CPUUtilization = cpu
				log.Printf("MONITORING HANDLER: CPU utilization for node %s: %.2f%%", status.NodeName, cpu)
			}
			if windowAvg, ok := data["windowAverageUtilization"].(float64); ok {
				newStatus.WindowAverageUtilization = windowAvg
				log.Printf("MONITORING HANDLER: Window average for node %s: %.2f%%", status.NodeName, windowAvg)
			}
		}

		newStatus.Message = fmt.Sprintf("Monitoring CPU utilization: current %.2f%%, window avg %.2f%%",
			newStatus.CPUUtilization,
			newStatus.WindowAverageUtilization)

		log.Printf("MONITORING STATUS: Node %s monitoring status updated: CPU: %.2f%%, Avg: %.2f%%",
			status.NodeName,
			newStatus.CPUUtilization,
			newStatus.WindowAverageUtilization)

	case models.EventInitialize:
		if data != nil {
			if cpu, ok := data["cpuUtilization"].(float64); ok {
				newStatus.CPUUtilization = cpu
			}
			if windowAvg, ok := data["windowAverageUtilization"].(float64); ok {
				newStatus.WindowAverageUtilization = windowAvg
			}
		}
	}

	return &newStatus, nil
}

// OnEnter implements the test version
func (h *testMonitoringHandler) OnEnter(ctx context.Context, status *models.ControlPlaneStatus) (*models.ControlPlaneStatus, error) {
	newStatus := *status
	newStatus.Message = "Entered monitoring state"
	return &newStatus, nil
}

// OnExit implements the test version
func (h *testMonitoringHandler) OnExit(ctx context.Context, status *models.ControlPlaneStatus) (*models.ControlPlaneStatus, error) {
	return status, nil
}
