package exporter

import (
	"context"

	"k2p-updater/cmd/app"
	"k2p-updater/internal/features/exporter/domain"
	"k2p-updater/internal/features/exporter/service"

	"k8s.io/client-go/kubernetes"
)

// NewProvider creates and initializes a new exporter provider.
func NewProvider(ctx context.Context, config *app.ExporterConfig, kubeClient kubernetes.Interface) (domain.Provider, error) {
	// Create and start the service
	exporterService := service.NewService(kubeClient, config)

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
	kubeClient kubernetes.Interface,
	exporterService domain.Provider,
	config *app.ExporterConfig,
) domain.VMHealthVerifier {
	verifierConfig := service.NewVMHealthVerifierConfig(config)
	return service.NewVMHealthVerifier(kubeClient, exporterService, verifierConfig)
}
