package metrics

import (
	"context"
	"k2p-updater/cmd/app"
	domainMetric "k2p-updater/internal/features/metric/domain"
)

// Analyzer is responsible for analyzing metrics data
type Analyzer interface {
	IsWindowReadyForScaling(ctx context.Context, nodeName string) (bool, error)
	CheckCPUThresholdExceeded(ctx context.Context, nodeName string, currentCPU, windowAvg float64) (bool, error)
}

// DefaultMetricsAnalyzer implements the Analyzer interface
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
