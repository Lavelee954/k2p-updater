package service

import (
	"context"
	"fmt"
	"k2p-updater/internal/features/exporter/domain"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

// kubernetesClient implements the domain.KubernetesClient interface
type kubernetesClient struct {
	coreClient typedcorev1.CoreV1Interface
}

// NewKubernetesClient creates a new Kubernetes client that implements the domain interface
func NewKubernetesClient(coreClient typedcorev1.CoreV1Interface) domain.KubernetesClient {
	return &kubernetesClient{
		coreClient: coreClient,
	}
}

// CoreV1 returns the CoreV1 client interface
func (k *kubernetesClient) CoreV1() typedcorev1.CoreV1Interface {
	return k.coreClient
}

// GetPods retrieves pods matching specific namespace and label selector
func (k *kubernetesClient) GetPods(ctx context.Context, namespace, labelSelector string) ([]v1.Pod, error) {
	pods, err := k.coreClient.Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}
	return pods.Items, nil
}

// GetNode retrieves information about a specific node
func (k *kubernetesClient) GetNode(ctx context.Context, name string) (*v1.Node, error) {
	return k.coreClient.Nodes().Get(ctx, name, metav1.GetOptions{})
}

// ListPodsInNode lists all pods running on a specific node
func (k *kubernetesClient) ListPodsInNode(ctx context.Context, nodeName string) ([]v1.Pod, error) {
	fieldSelector := fmt.Sprintf("spec.nodeName=%s", nodeName)
	pods, err := k.coreClient.Pods("").List(ctx, metav1.ListOptions{
		FieldSelector: fieldSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods on node %s: %w", nodeName, err)
	}
	return pods.Items, nil
}
