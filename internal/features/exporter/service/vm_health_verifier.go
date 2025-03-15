package service

import (
	"context"
	"fmt"
	"log"
	"time"

	"k2p-updater/cmd/app"
	"k2p-updater/internal/features/exporter/domain"

	v1 "k8s.io/api/core/v1"
)

// VMHealthVerifierConfig contains the VM health verification configuration.
type VMHealthVerifierConfig struct {
	NodeExporterPort            int
	RequiredPodsReadyPercentage int
	CriticalNamespaces          []string
}

// NewVMHealthVerifierConfig creates a VM health verification configuration.
func NewVMHealthVerifierConfig(exporterConfig *app.ExporterConfig) VMHealthVerifierConfig {
	return VMHealthVerifierConfig{
		NodeExporterPort:            exporterConfig.MetricsPort,
		RequiredPodsReadyPercentage: 70, // 기본값
		CriticalNamespaces:          []string{"kube-system", "monitoring"},
	}
}

// VMHealthVerifier implements the domain.VMHealthVerifier interface.
type VMHealthVerifier struct {
	kubeClient      domain.KubernetesClient
	exporterService domain.Provider
	config          VMHealthVerifierConfig
}

// NewVMHealthVerifier creates a new VM health verifier.
func NewVMHealthVerifier(
	kubeClient domain.KubernetesClient,
	exporterService domain.Provider,
	config VMHealthVerifierConfig,
) domain.VMHealthVerifier {
	return &VMHealthVerifier{
		kubeClient:      kubeClient,
		exporterService: exporterService,
		config:          config,
	}
}

// IsVMHealthy ensures that the VM is fully functional after a spec upgrade.
func (v *VMHealthVerifier) IsVMHealthy(ctx context.Context, nodeName string, upgradeTime time.Time) (bool, error) {
	log.Printf("Checking the health of node %s after upgrade (%s)", nodeName, upgradeTime.Format(time.RFC3339))

	// 1. Kubernetes 노드 Ready 상태 확인
	nodeReady, err := v.isNodeReady(ctx, nodeName)
	if err != nil || !nodeReady {
		log.Printf("Node %s is not ready according to the Kubernetes API: %v", nodeName, err)
		return false, err
	}

	// 2. Verify that the node exporter is healthy via the exporter service
	exporter, exists := v.exporterService.GetExporter(nodeName)
	if !exists {
		log.Printf("No exporter found for node %s", nodeName)
		return false, fmt.Errorf("no exporter found for node %s", nodeName)
	}

	if exporter.Status != domain.StatusRunning {
		log.Printf("The exporter on node %s is in %s status", nodeName, exporter.Status)
		return false, nil
	}

	// 3. Verify that the critical Pod is actually running on the node
	podsRunning, err := v.areCriticalPodsRunning(ctx, nodeName, upgradeTime)
	if err != nil || !podsRunning {
		log.Printf("Critical Pod not running correctly on node %s: %v", nodeName, err)
		return false, err
	}

	log.Printf("Node %s successfully passed all health checks", nodeName)
	return true, nil
}

// isNodeReady checks if the node is Ready according to Kubernetes.
func (v *VMHealthVerifier) isNodeReady(ctx context.Context, nodeName string) (bool, error) {
	node, err := v.kubeClient.GetNode(ctx, nodeName)
	if err != nil {
		return false, fmt.Errorf("node %s lookup failed: %w", nodeName, err)
	}

	for _, condition := range node.Status.Conditions {
		if condition.Type == v1.NodeReady {
			return condition.Status == v1.ConditionTrue, nil
		}
	}

	return false, nil
}

// areCriticalPodsRunning checks to see if critical system pods are actually running.
func (v *VMHealthVerifier) areCriticalPodsRunning(ctx context.Context, nodeName string, upgradeTime time.Time) (bool, error) {
	// 이 노드의 파드 가져오기
	pods, err := v.kubeClient.ListPodsInNode(ctx, nodeName)
	if err != nil {
		return false, fmt.Errorf("pod list lookup failed: %w", err)
	}

	criticalPods := 0
	healthyPods := 0

	for _, pod := range pods {
		// Make sure the Pod is in the critical namespace
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

		// Pods must have been created/restarted after the VM upgrade
		if pod.CreationTimestamp.Time.After(upgradeTime) {
			healthyPods++
			continue
		}

		// 파드가 업그레이드 전에 존재했다면 컨테이너 시작 시간 확인
		for _, containerStatus := range pod.Status.ContainerStatuses {
			if containerStatus.State.Running != nil &&
				containerStatus.State.Running.StartedAt.Time.After(upgradeTime) {
				healthyPods++
				break
			}
		}
	}

	// If critical pods are not found, there is a problem
	if criticalPods == 0 {
		return false, fmt.Errorf("critical Pod not found on node %s", nodeName)
	}

	// Calculate the percentage of healthy pods
	healthyPercentage := (healthyPods * 100) / criticalPods

	log.Printf("Node %s health check: %d/%d Critical Pods healthy (%d%%)",
		nodeName, healthyPods, criticalPods, healthyPercentage)

	return healthyPercentage >= v.config.RequiredPodsReadyPercentage, nil
}
