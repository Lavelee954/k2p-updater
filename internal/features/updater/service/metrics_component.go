package service

import (
	"context"
	"k2p-updater/internal/features/updater/domain"
)

// MetricsComponent is the main interface for metrics operations
type MetricsComponent interface {
	// GetNodeCPUMetrics returns the current and window average CPU metrics for a node
	GetNodeCPUMetrics(nodeName string) (float64, float64, error)

	// IsWindowReadyForScaling checks if the window is mature enough for scaling decisions
	IsWindowReadyForScaling(ctx context.Context, nodeName string) (bool, error)

	// CheckCPUThresholdExceeded determines if a node's CPU utilization exceeds the threshold
	CheckCPUThresholdExceeded(ctx context.Context, nodeName string) (bool, float64, float64, error)

	// StartMonitoring begins background monitoring of metrics states
	StartMonitoring(ctx context.Context) error

	// GetMetricsState returns the current metrics collection state for a node
	GetMetricsState(nodeName string) domain.MetricsState
}
