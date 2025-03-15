package exporter

import (
	"context"
	"fmt"

	"k2p-updater/cmd/app"
	"k2p-updater/internal/features/exporter/domain"
	"k2p-updater/internal/features/exporter/service"
)

// NewProvider creates and initializes a new exporter provider.
func NewProvider(ctx context.Context, config *app.ExporterConfig, kubeClient app.KubeClientInterface) (domain.Provider, error) {
	// Create domain-compatible Kubernetes client
	domainClient := service.NewKubernetesClient(kubeClient.CoreV1())

	// Create and start the service
	exporterService := service.NewService(domainClient, config)

	if err := exporterService.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start exporter service: %w", err)
	}

	if err := exporterService.WaitForInitialization(ctx); err != nil {
		return nil, fmt.Errorf("exporter service initialization timed out: %w", err)
	}

	return exporterService, nil
}

// NewVMHealthVerifier creates a new VM health verifier.
func NewVMHealthVerifier(
	kubeClient app.KubeClientInterface,
	exporterService domain.Provider,
	config *app.ExporterConfig,
) domain.VMHealthVerifier {
	// Create domain-compatible Kubernetes client
	domainClient := service.NewKubernetesClient(kubeClient.CoreV1())

	verifierConfig := service.NewVMHealthVerifierConfig(config)
	return service.NewVMHealthVerifier(domainClient, exporterService, verifierConfig)
}
