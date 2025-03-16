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
	domainUpdater "k2p-updater/internal/features/updater/domain"
)

// MetricsState represents the current state of metrics collection for a node
type MetricsState string

// NodeMetricsStatus tracks metrics collection status for a single node
type NodeMetricsStatus struct {
	State         domainUpdater.MetricsState
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
	GetMetricsState(nodeName string) domainUpdater.MetricsState
}

// MetricsCollector is responsible for collecting metrics data
type MetricsCollector interface {
	GetNodeCPUMetrics(nodeName string) (float64, float64, error)
}

// MetricsAnalyzer is responsible for analyzing metrics data
type MetricsAnalyzer interface {
	IsWindowReadyForScaling(ctx context.Context, nodeName string) (bool, error)
	CheckCPUThresholdExceeded(ctx context.Context, nodeName string, currentCPU, windowAvg float64) (bool, error)
}

// MetricsStateTracker is responsible for tracking metrics collection state
type MetricsStateTracker interface {
	GetMetricsState(nodeName string) domainUpdater.MetricsState
	UpdateMetricsState(nodeName string, state domainUpdater.MetricsState, message string)
	StartMonitoring(ctx context.Context) error
}

// MetricsManager orchestrates the metrics components
type MetricsManager struct {
	collector      MetricsCollector
	analyzer       MetricsAnalyzer
	stateTracker   MetricsStateTracker
	stateUpdater   domainUpdater.StateUpdater
	metricsService domainMetric.Provider
}

// NewMetricsManager creates a new metrics manager
func NewMetricsManager(
	collector MetricsCollector,
	analyzer MetricsAnalyzer,
	stateTracker MetricsStateTracker,
	stateUpdater domainUpdater.StateUpdater,
	metricsService domainMetric.Provider,
) *MetricsManager {
	return &MetricsManager{
		collector:      collector,
		analyzer:       analyzer,
		stateTracker:   stateTracker,
		stateUpdater:   stateUpdater,
		metricsService: metricsService,
	}
}

// GetNodeCPUMetrics returns the current and window average CPU metrics for a node
func (m *MetricsManager) GetNodeCPUMetrics(nodeName string) (float64, float64, error) {
	return m.collector.GetNodeCPUMetrics(nodeName)
}

// IsWindowReadyForScaling checks if the window is mature enough for scaling decisions
func (m *MetricsManager) IsWindowReadyForScaling(ctx context.Context, nodeName string) (bool, error) {
	if ctx.Err() != nil {
		return false, ctx.Err()
	}

	metricsState := m.stateTracker.GetMetricsState(nodeName)
	if metricsState != domainUpdater.MetricsReady {
		return false, nil
	}

	return m.analyzer.IsWindowReadyForScaling(ctx, nodeName)
}

// CheckCPUThresholdExceeded determines if a node's CPU utilization exceeds the threshold
func (m *MetricsManager) CheckCPUThresholdExceeded(ctx context.Context, nodeName string) (bool, float64, float64, error) {
	if ctx.Err() != nil {
		return false, 0, 0, ctx.Err()
	}

	metricsState := m.stateTracker.GetMetricsState(nodeName)
	if metricsState != domainUpdater.MetricsReady {
		currentCPU, windowAvg, _ := m.GetNodeCPUMetrics(nodeName)
		return false, currentCPU, windowAvg, nil
	}

	currentCPU, windowAvg, err := m.GetNodeCPUMetrics(nodeName)
	if err != nil {
		if ctx.Err() != nil {
			return false, 0, 0, ctx.Err()
		}
		return false, 0, 0, fmt.Errorf("failed to get CPU metrics: %w", err)
	}

	thresholdExceeded, err := m.analyzer.CheckCPUThresholdExceeded(ctx, nodeName, currentCPU, windowAvg)
	if err != nil {
		if ctx.Err() != nil {
			return false, 0, 0, ctx.Err()
		}
		return false, currentCPU, windowAvg, err
	}

	if thresholdExceeded {
		log.Printf("CPU threshold exceeded for node %s: %.2f%%", nodeName, windowAvg)
	}

	return thresholdExceeded, currentCPU, windowAvg, nil
}

// StartMonitoring begins background monitoring of metrics states
func (m *MetricsManager) StartMonitoring(ctx context.Context) error {
	return m.stateTracker.StartMonitoring(ctx)
}

// GetMetricsState returns the current metrics collection state for a node
func (m *MetricsManager) GetMetricsState(nodeName string) domainUpdater.MetricsState {
	return m.stateTracker.GetMetricsState(nodeName)
}

// DefaultMetricsCollector implements the MetricsCollector interface
type DefaultMetricsCollector struct {
	metricsService domainMetric.Provider
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector(
	metricsService domainMetric.Provider,
) *DefaultMetricsCollector {
	return &DefaultMetricsCollector{
		metricsService: metricsService,
	}
}

// GetNodeCPUMetrics returns the current and window average CPU metrics for a node
func (c *DefaultMetricsCollector) GetNodeCPUMetrics(nodeName string) (float64, float64, error) {
	currentCPU, err := c.metricsService.GetNodeCPUUsage(nodeName)
	if err != nil {
		if common.IsNodeNotFoundError(err) {
			log.Printf("Node %s registered but metrics not yet available - using default values", nodeName)
			return 0.0, 0.0, nil
		}
		return 0, 0, fmt.Errorf("failed to get current CPU usage: %w", err)
	}

	windowAvg, err := c.metricsService.GetWindowAverageCPU(nodeName)
	if err != nil {
		log.Printf("Window average not yet available for node %s, using current value", nodeName)
		windowAvg = currentCPU
	}

	return currentCPU, windowAvg, nil
}

// DefaultMetricsAnalyzer implements the MetricsAnalyzer interface
type DefaultMetricsAnalyzer struct {
	metricsService domainMetric.Provider
	config         *app.MetricsConfig
}

// NewMetricsAnalyzer creates a new metrics analyzer
func NewMetricsAnalyzer(
	metricsService domainMetric.Provider,
	config *app.MetricsConfig,
) *DefaultMetricsAnalyzer {
	return &DefaultMetricsAnalyzer{
		metricsService: metricsService,
		config:         config,
	}
}

// IsWindowReadyForScaling checks if the window is mature enough for scaling decisions
func (a *DefaultMetricsAnalyzer) IsWindowReadyForScaling(ctx context.Context, nodeName string) (bool, error) {
	if ctx.Err() != nil {
		return false, ctx.Err()
	}

	window, exists := a.metricsService.GetWindow(nodeName)
	if !exists {
		return false, nil
	}

	values := window.GetWindowValues()
	return len(values) >= window.MinSamples, nil
}

// CheckCPUThresholdExceeded determines if a node's CPU utilization exceeds the threshold
func (a *DefaultMetricsAnalyzer) CheckCPUThresholdExceeded(ctx context.Context, nodeName string, currentCPU, windowAvg float64) (bool, error) {
	if ctx.Err() != nil {
		return false, ctx.Err()
	}

	return windowAvg > a.config.ScaleTrigger, nil
}

// DefaultMetricsStateTracker implements the MetricsStateTracker interface
type DefaultMetricsStateTracker struct {
	metricsService domainMetric.Provider
	stateUpdater   domainUpdater.StateUpdater // Changed from stateMachine
	nodeStatus     map[string]*NodeMetricsStatus
	statusMutex    sync.RWMutex
}

// NewMetricsStateTracker creates a new metrics state tracker
func NewMetricsStateTracker(
	metricsService domainMetric.Provider,
	stateUpdater domainUpdater.StateUpdater,
) *DefaultMetricsStateTracker {
	return &DefaultMetricsStateTracker{
		metricsService: metricsService,
		stateUpdater:   stateUpdater,
		nodeStatus:     make(map[string]*NodeMetricsStatus),
	}
}

// GetMetricsState returns the current metrics collection state for a node
func (s *DefaultMetricsStateTracker) GetMetricsState(nodeName string) domainUpdater.MetricsState {
	s.statusMutex.RLock()
	defer s.statusMutex.RUnlock()

	status, exists := s.nodeStatus[nodeName]
	if !exists {
		return domainUpdater.MetricsInitializing
	}
	return status.State
}

// UpdateMetricsState updates the metrics state for a node
func (s *DefaultMetricsStateTracker) UpdateMetricsState(nodeName string, state domainUpdater.MetricsState, message string) {
	// Method body remains the same
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

// checkAllNodesMetricsReadiness verifies metrics readiness for all nodes
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
			State:       domainUpdater.MetricsInitializing,
			LastUpdated: time.Now(),
			Message:     "Initializing metrics collection",
		}
		s.nodeStatus[nodeName] = status
	}
	status.LastCheckTime = time.Now()
	s.statusMutex.Unlock()

	// Skip checks for nodes already in Ready state
	if status.State == domainUpdater.MetricsReady {
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
			status.State = domainUpdater.MetricsError
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
		status.State = domainUpdater.MetricsCollecting
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
		status.State = domainUpdater.MetricsCollecting
		status.Message = fmt.Sprintf("Collecting data: %d/%d samples, age: %v/%v",
			len(values), window.MinSamples, windowAge.Round(time.Second), minRequiredAge)
	} else {
		if status.State != domainUpdater.MetricsReady {
			log.Printf("Node %s metrics collection is now READY (samples: %d, age: %v)",
				nodeName, len(values), windowAge.Round(time.Second))
		}
		status.State = domainUpdater.MetricsReady
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
	if currentStatus.CurrentState != domainUpdater.StateMonitoring &&
		currentStatus.CurrentState != domainUpdater.StatePendingVmSpecUp &&
		currentStatus.CurrentState != domainUpdater.StateCoolDown {
		return
	}

	if ctx.Err() != nil {
		log.Printf("Context canceled during state determination: %v", ctx.Err())
		return
	}

	// Determine appropriate state based on metrics state
	var targetState domainUpdater.State
	switch status.State {
	case domainUpdater.MetricsInitializing, domainUpdater.MetricsCollecting:
		targetState = domainUpdater.StatePendingVmSpecUp
	case domainUpdater.MetricsReady:
		targetState = domainUpdater.StateMonitoring
	case domainUpdater.MetricsError:
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

		if err := s.stateUpdater.HandleEvent(ctx, nodeName, domainUpdater.EventInitialize, data); err != nil {
			log.Printf("Failed to update node state based on metrics state: %v", err)
		}
	}
}

// NewMetricsComponent creates a new metrics component with proper dependency injection
func NewMetricsComponent(
	metricsService domainMetric.Provider,
	stateUpdater domainUpdater.StateUpdater,
	config *app.MetricsConfig,
	collector MetricsCollector,
	analyzer MetricsAnalyzer,
	stateTracker MetricsStateTracker,
) MetricsComponent {
	// Use provided dependencies or create default ones
	if collector == nil {
		collector = NewMetricsCollector(metricsService)
	}

	if analyzer == nil {
		analyzer = NewMetricsAnalyzer(metricsService, config)
	}

	if stateTracker == nil {
		stateTracker = NewMetricsStateTracker(metricsService, stateUpdater)
	}

	// Create manager that implements MetricsComponent
	return NewMetricsManager(collector, analyzer, stateTracker, stateUpdater, metricsService)
}

// NewMetricsComponentLegacy preserves backward compatibility
func NewMetricsComponentLegacy(
	metricsService domainMetric.Provider,
	stateUpdater domainUpdater.StateUpdater,
	config *app.MetricsConfig,
) MetricsComponent {
	collector := NewMetricsCollector(metricsService)
	analyzer := NewMetricsAnalyzer(metricsService, config)
	stateTracker := NewMetricsStateTracker(metricsService, stateUpdater)

	return NewMetricsManager(collector, analyzer, stateTracker, stateUpdater, metricsService)
}

// Verify interface implementation
var _ domainUpdater.MetricsProvider = (*MetricsManager)(nil)
