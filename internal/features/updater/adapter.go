package updater

import (
	"context"
	"fmt"
	"time"

	"k2p-updater/cmd/app"
	exporterDomain "k2p-updater/internal/features/exporter/domain"
	metricDomain "k2p-updater/internal/features/metric/domain"
	"k2p-updater/internal/features/updater/domain"
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
) (domain.Provider, error) {
	// Create domain config from app config
	domainConfig := domain.UpdaterConfig{
		ScaleThreshold:         config.Updater.ScaleThreshold,
		ScaleUpStep:            config.Updater.ScaleUpStep,
		CooldownPeriod:         config.Updater.CooldownPeriod,
		MonitoringInterval:     time.Minute, // Default to 1 minute
		CooldownUpdateInterval: time.Minute, // Default to 1 minute
		Namespace:              config.Kubernetes.Namespace,
	}

	// Create backend client
	backendClient := service.NewBackendClient(&config.Backend)

	// Create health verifier
	healthVerifier := service.NewHealthVerifier(exporterService)

	// Create resource factory adapter for the domain interface
	resourceFactoryAdapter := service.NewResourceFactoryAdapter(resourceFactory)

	// Create state machine using the factory that accepts concrete types
	stateMachineFactory := service.NewStateMachineFactory(resourceFactory)
	stateMachine := stateMachineFactory.CreateStateMachine()

	// Create metrics component
	metricsConfig := &app.MetricsConfig{
		WindowSize:     10 * time.Minute,
		SlidingSize:    1 * time.Minute,
		CooldownPeriod: config.Updater.CooldownPeriod,
		ScaleTrigger:   config.Updater.ScaleThreshold,
	}
	metricsComponent := service.NewMetricsComponent(
		metricsService,
		stateMachine,
		metricsConfig,
	)

	// Create node discoverer
	nodeDiscoverer := service.NewNodeDiscoverer(kubeClient, config.Kubernetes.Namespace)

	// Create coordination manager
	coordinationManager := service.NewCoordinationManager(stateMachine)

	// Create recovery manager
	recoveryManager := service.NewRecoveryManager(stateMachine)

	// Create updater service with all dependencies injected
	updaterService := service.NewUpdaterService(
		domainConfig,
		stateMachine,
		backendClient,
		healthVerifier,
		metricsComponent,
		nodeDiscoverer,
		coordinationManager,
		recoveryManager,
		resourceFactoryAdapter,
	)

	// Start the service
	if err := updaterService.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start updater service: %w", err)
	}

	return updaterService, nil
}
