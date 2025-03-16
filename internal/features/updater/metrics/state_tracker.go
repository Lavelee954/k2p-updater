package metrics

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	domainMetric "k2p-updater/internal/features/metric/domain"
	"k2p-updater/internal/features/updater/domain/interfaces"
	"k2p-updater/internal/features/updater/domain/models"
)

// NodeMetricsStatus tracks metrics collection status for a single node
type NodeMetricsStatus struct {
	State         models.MetricsState
	LastUpdated   time.Time
	LastCheckTime time.Time
	RetryCount    int
	Message       string
}

// StateTracker is responsible for tracking metrics collection state
type StateTracker interface {
	GetMetricsState(nodeName string) models.MetricsState
	UpdateMetricsState(nodeName string, state models.MetricsState, message string)
	StartMonitoring(ctx context.Context) error
}

// DefaultMetricsStateTracker implements the StateTracker interface
type DefaultMetricsStateTracker struct {
	metricsService domainMetric.Provider
	stateUpdater   interfaces.StateUpdater
	nodeStatus     map[string]*NodeMetricsStatus
	statusMutex    sync.RWMutex
}

// NewMetricsStateTracker creates a new metrics state tracker
func NewMetricsStateTracker(
	metricsService domainMetric.Provider,
	stateUpdater interfaces.StateUpdater,
) *DefaultMetricsStateTracker {
	return &DefaultMetricsStateTracker{
		metricsService: metricsService,
		stateUpdater:   stateUpdater,
		nodeStatus:     make(map[string]*NodeMetricsStatus),
	}
}

// GetMetricsState returns the current metrics collection state for a node
func (s *DefaultMetricsStateTracker) GetMetricsState(nodeName string) models.MetricsState {
	s.statusMutex.RLock()
	defer s.statusMutex.RUnlock()

	status, exists := s.nodeStatus[nodeName]
	if !exists {
		return models.MetricsInitializing
	}
	return status.State
}

// UpdateMetricsState updates the metrics state for a node
func (s *DefaultMetricsStateTracker) UpdateMetricsState(nodeName string, state models.MetricsState, message string) {
	s.statusMutex.Lock()
	defer s.statusMutex.Unlock()

	status, exists := s.nodeStatus[nodeName]
	if !exists {
		status = &NodeMetricsStatus{
			State:       models.MetricsInitializing,
			LastUpdated: time.Now(),
		}
		s.nodeStatus[nodeName] = status
	}

	status.State = state
	status.Message = message
	status.LastUpdated = time.Now()
}

// StartMonitoring begins background monitoring of metrics states
func (s *DefaultMetricsStateTracker) StartMonitoring(ctx context.Context) error {
	go s.monitorNodesReadiness(ctx)
	return nil
}

// monitorNodesReadiness periodically checks if nodes have metrics available
func (s *DefaultMetricsStateTracker) monitorNodesReadiness(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("Metrics monitoring stopped due to context cancellation: %v", ctx.Err())
			return
		case <-ticker.C:
			if ctx.Err() != nil {
				log.Printf("Skipping metrics readiness check due to context cancellation: %v", ctx.Err())
				return
			}

			if err := s.checkAllNodesMetricsReadiness(ctx); err != nil {
				if errors.Is(err, context.Canceled) {
					log.Printf("Metrics readiness check canceled: %v", err)
					return
				}
				log.Printf("Error checking metrics readiness: %v", err)
			}
		}
	}
}

func (s *DefaultMetricsStateTracker) checkAllNodesMetricsReadiness(ctx context.Context) error {
	if ctx.Err() != nil {
		log.Printf("Skipping metrics readiness check due to context cancellation: %v", ctx.Err())
		return ctx.Err()
	}

	s.statusMutex.RLock()
	nodes := make([]string, 0, len(s.nodeStatus))
	for node := range s.nodeStatus {
		nodes = append(nodes, node)
	}
	s.statusMutex.RUnlock()

	for _, nodeName := range nodes {
		if ctx.Err() != nil {
			log.Printf("Stopping metrics readiness check due to context cancellation: %v", ctx.Err())
			return ctx.Err()
		}

		s.checkNodeMetricsReadiness(ctx, nodeName)
	}

	return nil
}

// checkNodeMetricsReadiness checks if metrics are available for a specific node
func (s *DefaultMetricsStateTracker) checkNodeMetricsReadiness(ctx context.Context, nodeName string) {
	if ctx.Err() != nil {
		log.Printf("Skipping node metrics readiness check due to context cancellation: %v", ctx.Err())
		return
	}

	s.statusMutex.Lock()
	status, exists := s.nodeStatus[nodeName]
	if !exists {
		status = &NodeMetricsStatus{
			State:       models.MetricsInitializing,
			LastUpdated: time.Now(),
			Message:     "Initializing metrics collection",
		}
		s.nodeStatus[nodeName] = status
	}
	status.LastCheckTime = time.Now()
	s.statusMutex.Unlock()

	// Skip checks for nodes already in Ready state
	if status.State == models.MetricsReady {
		return
	}

	if ctx.Err() != nil {
		log.Printf("Context canceled during metrics readiness check: %v", ctx.Err())
		return
	}

	// Check if metrics are available
	_, err := s.metricsService.GetNodeCPUUsage(nodeName)
	if err != nil {
		s.statusMutex.Lock()
		status.RetryCount++
		if status.RetryCount > 10 {
			status.State = models.MetricsError
			status.Message = fmt.Sprintf("Failed to get metrics after %d attempts: %v", status.RetryCount, err)
		} else {
			status.Message = fmt.Sprintf("Waiting for metrics to become available (attempt %d)", status.RetryCount)
		}
		s.statusMutex.Unlock()
		return
	}

	if ctx.Err() != nil {
		log.Printf("Context canceled during metrics window check: %v", ctx.Err())
		return
	}

	// Metrics are available, now check window
	window, exists := s.metricsService.GetWindow(nodeName)
	if !exists {
		s.statusMutex.Lock()
		status.State = models.MetricsCollecting
		status.Message = "Window not yet available, metrics collection starting"
		s.statusMutex.Unlock()
		return
	}

	// Check window data sufficiency
	values := window.GetWindowValues()
	windowAge := time.Since(window.CreationTime)
	minRequiredAge := window.WindowSize / 2

	s.statusMutex.Lock()
	if windowAge < minRequiredAge || len(values) < window.MinSamples {
		status.State = models.MetricsCollecting
		status.Message = fmt.Sprintf("Collecting data: %d/%d samples, age: %v/%v",
			len(values), window.MinSamples, windowAge.Round(time.Second), minRequiredAge)
	} else {
		if status.State != models.MetricsReady {
			log.Printf("Node %s metrics collection is now READY (samples: %d, age: %v)",
				nodeName, len(values), windowAge.Round(time.Second))
		}
		status.State = models.MetricsReady
		status.Message = "Metrics collection complete, ready for scaling decisions"
	}
	status.LastUpdated = time.Now()
	s.statusMutex.Unlock()

	if ctx.Err() != nil {
		log.Printf("Context canceled before updating node status in state machine: %v", ctx.Err())
		return
	}

	// Update node status in state machine if needed
	s.updateNodeStatusInStateMachine(ctx, nodeName)
}

// updateNodeStatusInStateMachine updates status in the state machine based on metrics state
func (s *DefaultMetricsStateTracker) updateNodeStatusInStateMachine(ctx context.Context, nodeName string) {
	if ctx.Err() != nil {
		log.Printf("Skipping state machine update due to context cancellation: %v", ctx.Err())
		return
	}

	s.statusMutex.RLock()
	status := s.nodeStatus[nodeName]
	s.statusMutex.RUnlock()

	currentStatus, err := s.stateUpdater.GetStatus(ctx, nodeName)
	if err != nil {
		log.Printf("Failed to get current status for node %s: %v", nodeName, err)
		return
	}

	// If node is already in an active state (beyond monitoring), don't change it
	if currentStatus.CurrentState != models.StateMonitoring &&
		currentStatus.CurrentState != models.StatePendingVmSpecUp &&
		currentStatus.CurrentState != models.StateCoolDown {
		return
	}

	if ctx.Err() != nil {
		log.Printf("Context canceled during state determination: %v", ctx.Err())
		return
	}

	// Determine appropriate state based on metrics state
	var targetState models.State
	switch status.State {
	case models.MetricsInitializing, models.MetricsCollecting:
		targetState = models.StatePendingVmSpecUp
	case models.MetricsReady:
		targetState = models.StateMonitoring
	case models.MetricsError:
		// Keep current state, just update message
		targetState = currentStatus.CurrentState
	}

	// Only transition if state is different
	if currentStatus.CurrentState != targetState || currentStatus.Message != status.Message {
		data := map[string]interface{}{
			"message": status.Message,
		}

		if ctx.Err() != nil {
			log.Printf("Context canceled before handling state event: %v", ctx.Err())
			return
		}

		if err := s.stateUpdater.HandleEvent(ctx, nodeName, models.EventInitialize, data); err != nil {
			log.Printf("Failed to update node state based on metrics state: %v", err)
		}
	}
}
