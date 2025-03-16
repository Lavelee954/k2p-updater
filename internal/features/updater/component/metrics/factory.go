package metrics

import (
	"context"
	"k2p-updater/cmd/app"
	domainMetric "k2p-updater/internal/features/metric/domain"
	"k2p-updater/internal/features/updater/domain/interfaces"
	"k2p-updater/internal/features/updater/domain/models"
)

// Component is the main interface for metrics operations
type Component interface {
	// GetNodeCPUMetrics returns the current and window average CPU metrics for a node
	GetNodeCPUMetrics(nodeName string) (float64, float64, error)

	// IsWindowReadyForScaling checks if the window is mature enough for scaling decisions
	IsWindowReadyForScaling(ctx context.Context, nodeName string) (bool, error)

	// CheckCPUThresholdExceeded determines if a node's CPU utilization exceeds the threshold
	CheckCPUThresholdExceeded(ctx context.Context, nodeName string) (bool, float64, float64, error)

	// StartMonitoring begins background monitoring of metrics states
	StartMonitoring(ctx context.Context) error

	// GetMetricsState returns the current metrics collection state for a node
	GetMetricsState(nodeName string) models.MetricsState
}

// NewMetricsComponent creates a fully configured metrics component with all dependencies
func NewMetricsComponent(
	metricsService domainMetric.Provider,
	stateUpdater interfaces.StateUpdater,
	config *app.MetricsConfig,
) Component {
	// Create each component with proper dependencies
	collector := NewMetricsCollector(metricsService)
	analyzer := NewMetricsAnalyzer(metricsService, config)
	stateTracker := NewMetricsStateTracker(metricsService, stateUpdater)

	// Create the manager that orchestrates all components
	return NewMetricsManager(
		collector,
		analyzer,
		stateTracker,
		stateUpdater,
		metricsService,
	)
}

// WithCustomCollector allows replacing the default collector
func WithCustomCollector(
	metricsService domainMetric.Provider,
	stateUpdater interfaces.StateUpdater,
	config *app.MetricsConfig,
	customCollector Collector,
) Component {
	analyzer := NewMetricsAnalyzer(metricsService, config)
	stateTracker := NewMetricsStateTracker(metricsService, stateUpdater)

	return NewMetricsManager(
		customCollector,
		analyzer,
		stateTracker,
		stateUpdater,
		metricsService,
	)
}

// WithCustomAnalyzer allows replacing the default analyzer
func WithCustomAnalyzer(
	metricsService domainMetric.Provider,
	stateUpdater interfaces.StateUpdater,
	config *app.MetricsConfig,
	customAnalyzer Analyzer,
) Component {
	collector := NewMetricsCollector(metricsService)
	stateTracker := NewMetricsStateTracker(metricsService, stateUpdater)

	return NewMetricsManager(
		collector,
		customAnalyzer,
		stateTracker,
		stateUpdater,
		metricsService,
	)
}

// WithCustomStateTracker allows replacing the default state tracker
func WithCustomStateTracker(
	metricsService domainMetric.Provider,
	stateUpdater interfaces.StateUpdater,
	config *app.MetricsConfig,
	customStateTracker StateTracker,
) Component {
	collector := NewMetricsCollector(metricsService)
	analyzer := NewMetricsAnalyzer(metricsService, config)

	return NewMetricsManager(
		collector,
		analyzer,
		customStateTracker,
		stateUpdater,
		metricsService,
	)
}
