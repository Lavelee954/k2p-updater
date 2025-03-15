package service

import (
	"context"
	"fmt"
	"k2p-updater/internal/features/exporter/domain"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// podFetcher implements the domain.PodFetcher interface.
type podFetcher struct {
	client kubernetes.Interface
}

// FetchPods implements domain.PodFetcher.FetchPods.
func (f *podFetcher) FetchPods(ctx context.Context, namespace, labelSelector string) ([]v1.Pod, error) {
	pods, err := f.client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}
	return pods.Items, nil
}

// newPodFetcher creates a new pod fetcher.
func newPodFetcher(client kubernetes.Interface) domain.PodFetcher {
	return &podFetcher{client: client}
}
