package updater

import (
	"context"
	"fmt"
	"k2p-updater/cmd/app"
	exporterDomain "k2p-updater/internal/features/exporter/domain"
	metricDomain "k2p-updater/internal/features/metric/domain"
	updaterDomain "k2p-updater/internal/features/updater/domain"
	"k2p-updater/internal/features/updater/service"
	"k2p-updater/pkg/resource"

	"k8s.io/client-go/kubernetes"
)

// NewProvider creates and initializes a new updater provider
func NewProvider(
	ctx context.Context,
	config *app.Config,
	metricsService metricDomain.Provider,
	exporterService exporterDomain.Provider,
	resourceFactory *resource.Factory,
	kubeClient kubernetes.Interface,
) (updaterDomain.Provider, error) {
	// Create backend client
	backendClient := service.NewBackendClient(&config.Backend)

	// Create health verifier
	healthVerifier := service.NewHealthVerifier(exporterService)

	// Create updater service
	updaterService := service.NewUpdaterService(
		&config.Updater,
		metricsService,
		backendClient,
		healthVerifier,
		resourceFactory,
		kubeClient,
	)

	// Start the service
	if err := updaterService.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start updater service: %w", err)
	}

	return updaterService, nil
}
