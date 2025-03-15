package service

import (
	"context"
	"k2p-updater/internal/features/exporter/domain"

	v1 "k8s.io/api/core/v1"
)

// podFetcher implements the domain.PodFetcher interface.
type podFetcher struct {
	client domain.KubernetesClient
}

// newPodFetcher creates a new PodFetcher.
func newPodFetcher(client domain.KubernetesClient) domain.PodFetcher {
	return &podFetcher{client: client}
}

// FetchPods implements domain.PodFetcher.FetchPods.
func (f *podFetcher) FetchPods(ctx context.Context, namespace, labelSelector string) ([]v1.Pod, error) {
	return f.client.GetPods(ctx, namespace, labelSelector)
}
