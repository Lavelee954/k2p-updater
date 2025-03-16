package service

import (
	"k2p-updater/internal/features/metric/domain"
	"time"
)

// newCPUUtilizationCalculator creates a new CPU utilization calculator
func newCPUUtilizationCalculator() domain.Calculator {
	return &CPUUtilizationCalculator{}
}

// newNodeMetricsFetcher creates a new metrics fetcher with the specified timeout
func newNodeMetricsFetcher(timeout time.Duration) domain.Fetcher {
	return &NodeMetricsFetcher{
		timeout: timeout,
	}
}

// newCPUMetricsParser creates a new CPU metrics parser
func newCPUMetricsParser() domain.Parser {
	return &CPUMetricsParser{}
}
