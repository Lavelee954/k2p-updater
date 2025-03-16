package service

import (
	"k2p-updater/cmd/app"
	domainMetric "k2p-updater/internal/features/metric/domain"
	domainUpdater "k2p-updater/internal/features/updater/domain"
)

// NewMetricsComponent creates a fully configured metrics component with all dependencies
func NewMetricsComponent(
	metricsService domainMetric.Provider,
	stateUpdater domainUpdater.StateUpdater,
	config *app.MetricsConfig,
) MetricsComponent {
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
	stateUpdater domainUpdater.StateUpdater,
	config *app.MetricsConfig,
	customCollector MetricsCollector,
) MetricsComponent {
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
	stateUpdater domainUpdater.StateUpdater,
	config *app.MetricsConfig,
	customAnalyzer MetricsAnalyzer,
) MetricsComponent {
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
	stateUpdater domainUpdater.StateUpdater,
	config *app.MetricsConfig,
	customStateTracker MetricsStateTracker,
) MetricsComponent {
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
