package health

import (
	"context"
	"fmt"

	domainExporter "k2p-updater/internal/features/exporter/domain"
	"k2p-updater/internal/features/updater/domain/interfaces"
	"time"
)

// HealthVerifier implements the interfaces.HealthVerifier interface
type HealthVerifier struct {
	exporterService domainExporter.Provider
}

// NewHealthVerifier creates a new health verifier
func NewHealthVerifier(exporterService domainExporter.Provider) interfaces.HealthVerifier {
	return &HealthVerifier{
		exporterService: exporterService,
	}
}

// VerifyNodeHealth checks if a node is healthy after spec up
func (v *HealthVerifier) VerifyNodeHealth(ctx context.Context, nodeName string) (bool, error) {
	// Get the exporter for this node
	exporter, exists := v.exporterService.GetExporter(nodeName)
	if !exists {
		return false, fmt.Errorf("no exporter found for node %s", nodeName)
	}

	// Check exporter status
	if exporter.Status != domainExporter.StatusRunning {
		return false, fmt.Errorf("exporter for node %s is in %s state", nodeName, exporter.Status)
	}

	// In a real implementation, we would use the VM health verifier to check:
	// 1. Is the node ready in Kubernetes
	// 2. Are critical pods running
	// 3. Can we collect metrics from the node

	// For simplicity, we'll just check if the exporter was seen recently
	recentlyActive := time.Since(exporter.LastSeen) < 5*time.Minute

	return recentlyActive, nil
}
