package service

import (
	domainMetric "k2p-updater/internal/features/metric/domain"
	"time"
)

// newCPUUtilizationCalculator creates a new CPU utilization calculator
func newCPUUtilizationCalculator() domainMetric.Calculator {
	return &CPUUtilizationCalculator{}
}

// newNodeMetricsFetcher creates a new metrics fetcher with the specified timeout
func newNodeMetricsFetcher(timeout time.Duration) domainMetric.Fetcher {
	return &NodeMetricsFetcher{
		timeout: timeout,
	}
}

// newCPUMetricsParser creates a new CPU metrics parser
func newCPUMetricsParser() domainMetric.Parser {
	return &CPUMetricsParser{}
}
