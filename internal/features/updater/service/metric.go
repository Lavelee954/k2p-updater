package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"k2p-updater/cmd/app"
	"k2p-updater/internal/common"
	domainMetric "k2p-updater/internal/features/metric/domain"
	"k2p-updater/internal/features/updater/domain"
)

// MetricsState represents the current state of metrics collection for a node
type MetricsState string

const (
	// MetricsInitializing indicates node registration but no data yet
	MetricsInitializing MetricsState = "Initializing"

	// MetricsCollecting indicates data collection in progress but not enough for decisions
	MetricsCollecting MetricsState = "Collecting"

	// MetricsReady indicates sufficient data for making scaling decisions
	MetricsReady MetricsState = "Ready"

	// MetricsError indicates a persistent problem with metrics collection
	MetricsError MetricsState = "Error"
)

// NodeMetricsStatus tracks metrics collection status for a single node
type NodeMetricsStatus struct {
	State         MetricsState
	LastUpdated   time.Time
	LastCheckTime time.Time
	RetryCount    int
	Message       string
}

// MetricsComponent handles all metrics-related operations
type MetricsComponent interface {
	GetNodeCPUMetrics(nodeName string) (float64, float64, error)
	IsWindowReadyForScaling(ctx context.Context, nodeName string) (bool, error)
	CheckCPUThresholdExceeded(ctx context.Context, nodeName string) (bool, float64, float64, error)
	StartMonitoring(ctx context.Context) error
	GetMetricsState(nodeName string) MetricsState
}

// DefaultMetricsComponent implements the MetricsComponent interface
type DefaultMetricsComponent struct {
	metricsService domainMetric.Provider
	stateMachine   domain.StateMachine
	config         *app.MetricsConfig

	// Status tracking
	nodeStatus  map[string]*NodeMetricsStatus
	statusMutex sync.RWMutex
}

// NewMetricsComponent creates a new DefaultMetricsComponent
func NewMetricsComponent(
	metricsService domainMetric.Provider,
	stateMachine domain.StateMachine,
	config *app.MetricsConfig,
) MetricsComponent {
	return &DefaultMetricsComponent{
		metricsService: metricsService,
		stateMachine:   stateMachine,
		config:         config,
		nodeStatus:     make(map[string]*NodeMetricsStatus),
	}
}

// StartMonitoring begins background monitoring of metrics states
func (c *DefaultMetricsComponent) StartMonitoring(ctx context.Context) error {
	go c.monitorNodesReadiness(ctx)
	return nil
}

// monitorNodesReadiness periodically checks if nodes have metrics available
func (c *DefaultMetricsComponent) monitorNodesReadiness(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("Metrics monitoring stopped due to context cancellation: %v", ctx.Err())
			return
		case <-ticker.C:
			if err := c.checkAllNodesMetricsReadiness(ctx); err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("Error checking metrics readiness: %v", err)
			}
		}
	}
}

// checkAllNodesMetricsReadiness verifies metrics readiness for all nodes
func (c *DefaultMetricsComponent) checkAllNodesMetricsReadiness(ctx context.Context) error {
	// Add a check for context cancellation at the beginning
	if ctx.Err() != nil {
		log.Printf("Skipping metrics readiness check due to context cancellation: %v", ctx.Err())
		return nil
	}

	c.statusMutex.RLock()
	nodes := make([]string, 0, len(c.nodeStatus))
	for node := range c.nodeStatus {
		nodes = append(nodes, node)
	}
	c.statusMutex.RUnlock()

	for _, nodeName := range nodes {
		// Check context cancellation during iteration
		if ctx.Err() != nil {
			log.Printf("Stopping metrics readiness check due to context cancellation: %v", ctx.Err())
			return nil
		}

		// Call the method directly as in the original code
		c.checkNodeMetricsReadiness(ctx, nodeName)
	}

	return nil
}

// checkNodeMetricsReadiness checks if metrics are available for a specific node
func (c *DefaultMetricsComponent) checkNodeMetricsReadiness(ctx context.Context, nodeName string) {
	c.statusMutex.Lock()
	status, exists := c.nodeStatus[nodeName]
	if !exists {
		status = &NodeMetricsStatus{
			State:       MetricsInitializing,
			LastUpdated: time.Now(),
			Message:     "Initializing metrics collection",
		}
		c.nodeStatus[nodeName] = status
	}
	status.LastCheckTime = time.Now()
	c.statusMutex.Unlock()

	// Skip checks for nodes already in Ready state
	if status.State == MetricsReady {
		return
	}

	// Check if metrics are available
	_, err := c.metricsService.GetNodeCPUUsage(nodeName)
	if err != nil {
		c.statusMutex.Lock()
		status.RetryCount++
		if status.RetryCount > 10 {
			status.State = MetricsError
			status.Message = fmt.Sprintf("Failed to get metrics after %d attempts: %v", status.RetryCount, err)
		} else {
			status.Message = fmt.Sprintf("Waiting for metrics to become available (attempt %d)", status.RetryCount)
		}
		c.statusMutex.Unlock()
		return
	}

	// Metrics are available, now check window
	window, exists := c.metricsService.GetWindow(nodeName)
	if !exists {
		c.statusMutex.Lock()
		status.State = MetricsCollecting
		status.Message = "Window not yet available, metrics collection starting"
		c.statusMutex.Unlock()
		return
	}

	// Check window data sufficiency
	values := window.GetWindowValues()
	windowAge := time.Since(window.CreationTime)
	minRequiredAge := window.WindowSize / 2

	c.statusMutex.Lock()
	if windowAge < minRequiredAge || len(values) < window.MinSamples {
		status.State = MetricsCollecting
		status.Message = fmt.Sprintf("Collecting data: %d/%d samples, age: %v/%v",
			len(values), window.MinSamples, windowAge.Round(time.Second), minRequiredAge)
	} else {
		if status.State != MetricsReady {
			log.Printf("Node %s metrics collection is now READY (samples: %d, age: %v)",
				nodeName, len(values), windowAge.Round(time.Second))
		}
		status.State = MetricsReady
		status.Message = "Metrics collection complete, ready for scaling decisions"
	}
	status.LastUpdated = time.Now()
	c.statusMutex.Unlock()

	// Update node status in state machine if needed
	c.updateNodeStatusInStateMachine(ctx, nodeName)
}

// updateNodeStatusInStateMachine updates status in the state machine based on metrics state
func (c *DefaultMetricsComponent) updateNodeStatusInStateMachine(ctx context.Context, nodeName string) {
	c.statusMutex.RLock()
	status := c.nodeStatus[nodeName]
	c.statusMutex.RUnlock()

	currentStatus, err := c.stateMachine.GetStatus(ctx, nodeName)
	if err != nil {
		log.Printf("Failed to get current status for node %s: %v", nodeName, err)
		return
	}

	// If node is already in an active state (beyond monitoring), don't change it
	if currentStatus.CurrentState != domain.StateMonitoring &&
		currentStatus.CurrentState != domain.StatePendingVmSpecUp &&
		currentStatus.CurrentState != domain.StateCoolDown {
		return
	}

	// Determine appropriate state based on metrics state
	var targetState domain.State
	switch status.State {
	case MetricsInitializing, MetricsCollecting:
		targetState = domain.StatePendingVmSpecUp
	case MetricsReady:
		targetState = domain.StateMonitoring
	case MetricsError:
		// Keep current state, just update message
		targetState = currentStatus.CurrentState
	}

	// Only transition if state is different
	if currentStatus.CurrentState != targetState || currentStatus.Message != status.Message {
		data := map[string]interface{}{
			"message": status.Message,
		}

		if err := c.stateMachine.HandleEvent(ctx, nodeName, domain.EventInitialize, data); err != nil {
			log.Printf("Failed to update node state based on metrics state: %v", err)
		}
	}

}

// GetMetricsState returns the current metrics collection state for a node
func (c *DefaultMetricsComponent) GetMetricsState(nodeName string) MetricsState {
	c.statusMutex.RLock()
	defer c.statusMutex.RUnlock()

	status, exists := c.nodeStatus[nodeName]
	if !exists {
		return MetricsInitializing
	}
	return status.State
}

// GetNodeCPUMetrics returns the current and window average CPU metrics for a node
func (c *DefaultMetricsComponent) GetNodeCPUMetrics(nodeName string) (float64, float64, error) {
	// Register node status if not already registered
	c.statusMutex.RLock()
	_, exists := c.nodeStatus[nodeName]
	c.statusMutex.RUnlock()

	if !exists {
		c.statusMutex.Lock()
		c.nodeStatus[nodeName] = &NodeMetricsStatus{
			State:       MetricsInitializing,
			LastUpdated: time.Now(),
			Message:     "Initializing metrics collection",
		}
		c.statusMutex.Unlock()
	}

	// Get current CPU from metrics service
	currentCPU, err := c.metricsService.GetNodeCPUUsage(nodeName)
	if err != nil {
		if common.IsNodeNotFoundError(err) {
			log.Printf("Node %s registered but metrics not yet available - using default values", nodeName)
			return 0.0, 0.0, nil
		}
		return 0, 0, fmt.Errorf("failed to get current CPU usage: %w", err)
	}

	// Get window average from metrics service
	windowAvg, err := c.metricsService.GetWindowAverageCPU(nodeName)
	if err != nil {
		log.Printf("Window average not yet available for node %s, using current value", nodeName)
		windowAvg = currentCPU
	}

	return currentCPU, windowAvg, nil
}

// IsWindowReadyForScaling checks if the window is mature enough for scaling decisions
func (c *DefaultMetricsComponent) IsWindowReadyForScaling(ctx context.Context, nodeName string) (bool, error) {
	// Check metrics state first
	metricsState := c.GetMetricsState(nodeName)
	if metricsState != MetricsReady {
		return false, nil
	}

	// For nodes in READY state, verify window data
	window, exists := c.metricsService.GetWindow(nodeName)
	if !exists {
		return false, nil
	}

	values := window.GetWindowValues()
	return len(values) >= window.MinSamples, nil
}

// CheckCPUThresholdExceeded determines if a node's CPU utilization exceeds the threshold
func (c *DefaultMetricsComponent) CheckCPUThresholdExceeded(ctx context.Context, nodeName string) (bool, float64, float64, error) {
	// Get metrics state first
	metricsState := c.GetMetricsState(nodeName)
	if metricsState != MetricsReady {
		currentCPU, windowAvg, _ := c.GetNodeCPUMetrics(nodeName)
		return false, currentCPU, windowAvg, nil
	}

	// Get current and window average CPU metrics
	currentCPU, windowAvg, err := c.GetNodeCPUMetrics(nodeName)
	if err != nil {
		return false, 0, 0, fmt.Errorf("failed to get CPU metrics: %w", err)
	}

	// Check if window average exceeds threshold
	thresholdExceeded := windowAvg > c.config.ScaleTrigger

	if thresholdExceeded {
		log.Printf("CPU threshold exceeded for node %s: %.2f%% > %.2f%%",
			nodeName, windowAvg, c.config.ScaleTrigger)
	}

	return thresholdExceeded, currentCPU, windowAvg, nil
}
