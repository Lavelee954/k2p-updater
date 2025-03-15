package domain

import (
	"context"
	"k2p-updater/internal/features/exporter/domain"
)

// Provider defines the interface for the metrics service
type Provider interface {
	NodeMetricsCollector
	MetricsAnalyzer
	AlertNotifier

	// Start begins metrics collection for all nodes
	Start(ctx context.Context) error
}

// NodeMetricsCollector collects node metrics
type NodeMetricsCollector interface {
	// GetNodeCPUUsage returns the current CPU utilization of a specific node
	GetNodeCPUUsage(nodeName string) (float64, error)

	// GetWindow returns the sliding window of a specific node
	GetWindow(nodeName string) (*SlidingWindow, bool)
}

// MetricsAnalyzer analyzes collected metrics
type MetricsAnalyzer interface {
	// GetWindowAverageCPU returns the average CPU utilization over the window duration
	GetWindowAverageCPU(nodeName string) (float64, error)
}

// AlertNotifier provides notifications about high resource usage
type AlertNotifier interface {
	// SubscribeHighCPUAlerts returns a channel to receive high CPU utilization notifications
	SubscribeHighCPUAlerts() <-chan CPUAlert
}

// Calculator defines operations for CPU utilization calculations
type Calculator interface {
	// CalculateUtilization calculates CPU usage from current and previous readings
	CalculateUtilization(current CPUStats, cpuID string, nodeName string,
		previousStats map[string]map[string]CPUStats) (user, system, idle float64)
}

// Fetcher defines operations for fetching metrics from exporters
type Fetcher interface {
	// FetchNodeMetrics retrieves raw metrics from a node exporter
	FetchNodeMetrics(ctx context.Context, exporter domain.NodeExporter) (string, error)
}

// Parser defines operations for parsing metrics data
type Parser interface {
	// ParseCPUMetrics parses raw metrics into CPU metrics objects
	ParseCPUMetrics(metricsData string, nodeName string) (*NodeCPUMetrics, error)
}

// In domain/provider.go

// NodeRegistration allows manual registration of nodes in the metrics system
type NodeRegistration interface {
	// RegisterNode adds a node to the metrics tracking system
	RegisterNode(ctx context.Context, nodeName string) error
}
