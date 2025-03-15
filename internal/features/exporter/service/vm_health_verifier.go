// internal/features/exporter/service/vm_health_verifier.go
package service

import (
	"context"
	"fmt"
	"log"
	"time"

	"k2p-updater/cmd/app"
	"k2p-updater/internal/features/exporter/domain"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// VMHealthVerifierConfig holds configuration for VM health verification
type VMHealthVerifierConfig struct {
	NodeExporterPort            int
	RequiredPodsReadyPercentage int
	CriticalNamespaces          []string
}

// NewVMHealthVerifierConfig creates a configuration for VM health verifier
func NewVMHealthVerifierConfig(exporterConfig *app.ExporterConfig) VMHealthVerifierConfig {
	return VMHealthVerifierConfig{
		NodeExporterPort:            exporterConfig.MetricsPort,
		RequiredPodsReadyPercentage: 70, // Default value
		CriticalNamespaces:          []string{"kube-system", "monitoring"},
	}
}

// VMHealthVerifier implements the domain.VMHealthVerifier interface
type VMHealthVerifier struct {
	kubeClient      kubernetes.Interface
	exporterService domain.Provider
	config          VMHealthVerifierConfig
}

// NewVMHealthVerifier creates a new VM health verifier
func NewVMHealthVerifier(
	kubeClient kubernetes.Interface,
	exporterService domain.Provider,
	config VMHealthVerifierConfig,
) domain.VMHealthVerifier {
	return &VMHealthVerifier{
		kubeClient:      kubeClient,
		exporterService: exporterService,
		config:          config,
	}
}

// IsVMHealthy checks if a VM is fully operational after a spec upgrade
func (v *VMHealthVerifier) IsVMHealthy(ctx context.Context, nodeName string, upgradeTime time.Time) (bool, error) {
	log.Printf("Verifying health for node %s after upgrade at %s", nodeName, upgradeTime.Format(time.RFC3339))

	// 1. Check Kubernetes node Ready status
	nodeReady, err := v.isNodeReady(ctx, nodeName)
	if err != nil || !nodeReady {
		log.Printf("Node %s not ready according to Kubernetes API: %v", nodeName, err)
		return false, err
	}

	// 2. Check if node-exporter is healthy through the exporter service
	exporter, exists := v.exporterService.GetExporter(nodeName)
	if !exists {
		log.Printf("No exporter found for node %s", nodeName)
		return false, fmt.Errorf("no exporter found for node %s", nodeName)
	}

	if exporter.Status != domain.StatusRunning {
		log.Printf("Exporter for node %s is in %s status", nodeName, exporter.Status)
		return false, nil
	}

	// 3. Check if critical pods are actually running on the node
	podsRunning, err := v.areCriticalPodsRunning(ctx, nodeName, upgradeTime)
	if err != nil || !podsRunning {
		log.Printf("Critical pods not running correctly on node %s: %v", nodeName, err)
		return false, err
	}

	log.Printf("Node %s passed all health checks successfully", nodeName)
	return true, nil
}

// isNodeReady checks if a node is in the Ready condition according to Kubernetes
func (v *VMHealthVerifier) isNodeReady(ctx context.Context, nodeName string) (bool, error) {
	node, err := v.kubeClient.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return false, fmt.Errorf("failed to get node %s: %w", nodeName, err)
	}

	for _, condition := range node.Status.Conditions {
		if condition.Type == v1.NodeReady {
			return condition.Status == v1.ConditionTrue, nil
		}
	}

	return false, nil
}

// areCriticalPodsRunning checks if critical system pods are actually running
func (v *VMHealthVerifier) areCriticalPodsRunning(ctx context.Context, nodeName string, upgradeTime time.Time) (bool, error) {
	// Get pods on this node
	fieldSelector := fmt.Sprintf("spec.nodeName=%s", nodeName)
	pods, err := v.kubeClient.CoreV1().Pods("").List(ctx, metav1.ListOptions{
		FieldSelector: fieldSelector,
	})
	if err != nil {
		return false, fmt.Errorf("failed to list pods: %w", err)
	}

	criticalPods := 0
	healthyPods := 0

	for _, pod := range pods.Items {
		// Check if pod is in a critical namespace
		isCritical := false
		for _, ns := range v.config.CriticalNamespaces {
			if pod.Namespace == ns {
				isCritical = true
				break
			}
		}

		if !isCritical {
			continue
		}

		criticalPods++

		// Pod must be running
		if pod.Status.Phase != v1.PodRunning {
			continue
		}

		// Pod should have been created/restarted after the VM upgrade
		if pod.CreationTimestamp.Time.After(upgradeTime) {
			healthyPods++
			continue
		}

		// Check container start times if pod existed before upgrade
		for _, containerStatus := range pod.Status.ContainerStatuses {
			if containerStatus.State.Running != nil &&
				containerStatus.State.Running.StartedAt.Time.After(upgradeTime) {
				healthyPods++
				break
			}
		}
	}

	// If no critical pods were found, something is wrong
	if criticalPods == 0 {
		return false, fmt.Errorf("no critical pods found on node %s", nodeName)
	}

	// Calculate the percentage of healthy pods
	healthyPercentage := (healthyPods * 100) / criticalPods

	log.Printf("Node %s health check: %d/%d critical pods healthy (%d%%)",
		nodeName, healthyPods, criticalPods, healthyPercentage)

	return healthyPercentage >= v.config.RequiredPodsReadyPercentage, nil
}
