package exporter

import (
	"context"

	"k2p-updater/cmd/app"
	"k2p-updater/internal/features/exporter/domain"
	"k2p-updater/internal/features/exporter/service"
)

// NewProvider creates and initializes a new exporter provider.
func NewProvider(ctx context.Context, config *app.ExporterConfig, kubeClient app.KubeClientInterface) (domain.Provider, error) {
	// Create a Kubernetes client for the domain interface
	domainClient := service.NewKubernetesClient(kubeClient.CoreV1())

	// 서비스 생성 및 시작
	exporterService := service.NewService(domainClient, config)

	if err := exporterService.Start(ctx); err != nil {
		return nil, err
	}

	if err := exporterService.WaitForInitialization(ctx); err != nil {
		return nil, err
	}

	return exporterService, nil
}

// NewVMHealthVerifier creates a new VM health verifier.
func NewVMHealthVerifier(
	kubeClient app.KubeClientInterface,
	exporterService domain.Provider,
	config *app.ExporterConfig,
) domain.VMHealthVerifier {
	// Create a Kubernetes client for the domain interface
	domainClient := service.NewKubernetesClient(kubeClient.CoreV1())

	verifierConfig := service.NewVMHealthVerifierConfig(config)
	return service.NewVMHealthVerifier(domainClient, exporterService, verifierConfig)
}
