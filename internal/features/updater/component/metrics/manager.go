package metrics

import (
	"context"
	"fmt"
	"log"

	"k2p-updater/internal/features/metric/domain"
	"k2p-updater/internal/features/updater/domain/interfaces"
	"k2p-updater/internal/features/updater/domain/models"
)

// Manager MetricsManager orchestrates the metrics components
type Manager struct {
	collector      Collector
	analyzer       Analyzer
	stateTracker   StateTracker
	stateUpdater   interfaces.StateUpdater
	metricsService domain.Provider
}

// NewMetricsManager creates a new metrics manager
func NewMetricsManager(
	collector Collector,
	analyzer Analyzer,
	stateTracker StateTracker,
	stateUpdater interfaces.StateUpdater,
	metricsService domain.Provider,
) *Manager {
	return &Manager{
		collector:      collector,
		analyzer:       analyzer,
		stateTracker:   stateTracker,
		stateUpdater:   stateUpdater,
		metricsService: metricsService,
	}
}

// GetNodeCPUMetrics returns the current and window average CPU metrics for a node
func (m *Manager) GetNodeCPUMetrics(nodeName string) (float64, float64, error) {
	return m.collector.GetNodeCPUMetrics(nodeName)
}

// IsWindowReadyForScaling checks if the window is mature enough for scaling decisions
func (m *Manager) IsWindowReadyForScaling(ctx context.Context, nodeName string) (bool, error) {
	if ctx.Err() != nil {
		return false, ctx.Err()
	}

	metricsState := m.stateTracker.GetMetricsState(nodeName)
	if metricsState != models.MetricsReady {
		return false, nil
	}

	return m.analyzer.IsWindowReadyForScaling(ctx, nodeName)
}

// CheckCPUThresholdExceeded determines if a node's CPU utilization exceeds the threshold
func (m *Manager) CheckCPUThresholdExceeded(ctx context.Context, nodeName string) (bool, float64, float64, error) {
	if ctx.Err() != nil {
		return false, 0, 0, ctx.Err()
	}

	metricsState := m.stateTracker.GetMetricsState(nodeName)
	if metricsState != models.MetricsReady {
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
func (m *Manager) StartMonitoring(ctx context.Context) error {
	return m.stateTracker.StartMonitoring(ctx)
}

// GetMetricsState returns the current metrics collection state for a node
func (m *Manager) GetMetricsState(nodeName string) models.MetricsState {
	return m.stateTracker.GetMetricsState(nodeName)
}

// Verify interface implementation
var _ interfaces.MetricsProvider = (*Manager)(nil)
