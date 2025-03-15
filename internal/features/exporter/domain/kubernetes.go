// internal/features/exporter/domain/kubernetes.go
package domain

import (
	"context"
	v1 "k8s.io/api/core/v1"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

// The KubernetesClient interface defines the minimum Kubernetes client functionality required by the exporter.
type KubernetesClient interface {
	// CoreV1 returns the CoreV1 client interface
	CoreV1() typedcorev1.CoreV1Interface

	// GetPods returns a list of Pods that match a specific namespace and label selector.
	GetPods(ctx context.Context, namespace, labelSelector string) ([]v1.Pod, error)

	// GetNode returns information about a specific node.
	GetNode(ctx context.Context, name string) (*v1.Node, error)

	// ListPodsInNode returns all Pods running on a specific node.
	ListPodsInNode(ctx context.Context, nodeName string) ([]v1.Pod, error)
}
