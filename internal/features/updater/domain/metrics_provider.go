package domain

import (
	"context"
)

// MetricsProvider defines an abstraction for retrieving node metrics
// This interface is used to break circular dependencies
type MetricsProvider interface {
	// GetNodeCPUMetrics returns the current and window average CPU metrics for a node
	GetNodeCPUMetrics(nodeName string) (float64, float64, error)

	// CheckCPUThresholdExceeded checks if CPU utilization exceeds the threshold
	CheckCPUThresholdExceeded(ctx context.Context, nodeName string) (bool, float64, float64, error)

	// IsWindowReadyForScaling checks if the window is mature enough for scaling decisions
	IsWindowReadyForScaling(ctx context.Context, nodeName string) (bool, error)

	// GetMetricsState returns the current metrics collection state for a node
	GetMetricsState(nodeName string) MetricsState
}

// MetricsState represents the state of metrics collection
type MetricsState string

const (
	MetricsInitializing MetricsState = "Initializing"
	MetricsCollecting   MetricsState = "Collecting"
	MetricsReady        MetricsState = "Ready"
	MetricsError        MetricsState = "Error"
)
