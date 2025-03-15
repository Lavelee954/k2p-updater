package service

import (
	"context"
	"k2p-updater/internal/features/exporter/domain"

	v1 "k8s.io/api/core/v1"
)

// podFetcher는 domain.PodFetcher 인터페이스를 구현합니다.
type podFetcher struct {
	client domain.KubernetesClient
}

// newPodFetcher는 새로운 PodFetcher를 생성합니다.
func newPodFetcher(client domain.KubernetesClient) domain.PodFetcher {
	return &podFetcher{client: client}
}

// FetchPods는 domain.PodFetcher.FetchPods를 구현합니다.
func (f *podFetcher) FetchPods(ctx context.Context, namespace, labelSelector string) ([]v1.Pod, error) {
	return f.client.GetPods(ctx, namespace, labelSelector)
}
