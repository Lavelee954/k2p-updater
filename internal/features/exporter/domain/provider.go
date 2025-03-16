package domain

import (
	"context"
	"time"

	v1 "k8s.io/api/core/v1"
)

// Provider defines the interface for node exporter operations
type Provider interface {
	ExporterManager
	ExporterQuery
	WaitForInitialization(ctx context.Context) error
	Start(ctx context.Context) error
	Stop()
}

// ExporterManager defines operations for managing exporters
type ExporterManager interface {
	// GetExporter returns information about a specific exporter
	GetExporter(nodeName string) (NodeExporter, bool)
}

// ExporterQuery defines operations for querying exporters
type ExporterQuery interface {
	// GetHealthyExporters returns all healthy exporters
	GetHealthyExporters() []NodeExporter
}

// PodFetcher defines the interface for retrieving pod information
type PodFetcher interface {
	// FetchPods retrieves pods matching criteria
	FetchPods(ctx context.Context, namespace, labelSelector string) ([]v1.Pod, error)
}

// HealthChecker defines the interface for checking exporter health
type HealthChecker interface {
	// CheckHealth verifies if the exporter is healthy
	CheckHealth(ctx context.Context, ip string, port int) (bool, error)
}

// VMHealthVerifier defines operations for verifying VM health
type VMHealthVerifier interface {
	// IsVMHealthy checks if a VM is fully operational after a spec upgrade
	IsVMHealthy(ctx context.Context, nodeName string, upgradeTime time.Time) (bool, error)
}
