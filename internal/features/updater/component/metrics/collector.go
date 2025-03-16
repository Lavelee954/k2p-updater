package metrics

import (
	"fmt"
	"k2p-updater/internal/common"
	"k2p-updater/internal/features/metric/domain"
	"log"
)

// Collector is responsible for collecting metrics data
type Collector interface {
	GetNodeCPUMetrics(nodeName string) (float64, float64, error)
}

// DefaultMetricsCollector implements the Collector interface
type DefaultMetricsCollector struct {
	metricsService domain.Provider
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector(metricsService domain.Provider) *DefaultMetricsCollector {
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
