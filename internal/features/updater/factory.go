package updater

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"k2p-updater/cmd/app"
	exporterDomain "k2p-updater/internal/features/exporter/domain"
	metricDomain "k2p-updater/internal/features/metric/domain"
	"k2p-updater/internal/features/updater/backend"
	"k2p-updater/internal/features/updater/coordination"
	"k2p-updater/internal/features/updater/domain/interfaces"
	"k2p-updater/internal/features/updater/domain/models"
	"k2p-updater/internal/features/updater/health"
	"k2p-updater/internal/features/updater/metrics"
	"k2p-updater/internal/features/updater/recovery"
	resourceUpdater "k2p-updater/internal/features/updater/resource"
	"k2p-updater/internal/features/updater/service"
	"k2p-updater/internal/features/updater/state"
	"k2p-updater/pkg/resource"

	"k8s.io/client-go/kubernetes"
)

// NewProvider creates and initializes a new updater provider with dependencies
func NewProvider(
	ctx context.Context,
	config *app.Config,
	metricsService metricDomain.Provider,
	exporterService exporterDomain.Provider,
	resourceFactory *resource.Factory,
	kubeClient kubernetes.Interface,
	// Optional dependencies for testing
	options ...ProviderOption,
) (interfaces.Provider, error) {
	// Initialize provider options with defaults
	opts := &providerOptions{}
	for _, option := range options {
		option(opts)
	}

	// Create domain config from app config
	domainConfig := models.UpdaterConfig{
		ScaleThreshold:         config.Updater.ScaleThreshold,
		ScaleUpStep:            config.Updater.ScaleUpStep,
		CooldownPeriod:         config.Updater.CooldownPeriod,
		MonitoringInterval:     time.Minute, // Default to 1 minute
		CooldownUpdateInterval: time.Minute, // Default to 1 minute
		Namespace:              config.Kubernetes.Namespace,
	}

	// Create resource factory adapter for the domain interface
	resourceFactoryAdapter := resourceUpdater.NewResourceFactoryAdapter(resourceFactory)

	// Create dependencies if not provided (for backwards compatibility)
	backendClient := opts.backendClient
	if backendClient == nil {
		httpClient := &http.Client{Timeout: config.Backend.Timeout}
		backendClient = backend.NewBackendClient(&config.Backend, httpClient)
	}

	healthVerifier := opts.healthVerifier
	if healthVerifier == nil {
		healthVerifier = health.NewHealthVerifier(exporterService)
	}

	stateMachine := opts.stateMachine
	if stateMachine == nil {
		stateMachine = state.NewStateMachineWithDefaultHandlers(resourceFactory)
	}

	metricsComponent := opts.metricsComponent
	if metricsComponent == nil {
		metricsConfig := &app.MetricsConfig{
			WindowSize:     10 * time.Minute,
			SlidingSize:    1 * time.Minute,
			CooldownPeriod: config.Updater.CooldownPeriod,
			ScaleTrigger:   config.Updater.ScaleThreshold,
		}

		// Use the metrics factory to create the component
		metricsComponent = metrics.NewMetricsComponent(
			metricsService,
			stateMachine, // as StateUpdater
			metricsConfig,
		)
	}

	nodeDiscoverer := opts.nodeDiscoverer
	if nodeDiscoverer == nil {
		nodeDiscoverer = coordination.NewNodeDiscoverer(kubeClient, config.Kubernetes.Namespace)
	}

	coordinator := opts.coordinator
	if coordinator == nil {
		coordinator = coordination.NewCoordinationManager(stateMachine)
	}

	recoveryManager := opts.recoveryManager
	if recoveryManager == nil {
		recoveryManager = recovery.NewRecoveryManager(stateMachine)
	}

	// Create updater service with all dependencies injected
	updaterService := service.NewUpdaterService(
		domainConfig,
		stateMachine,
		backendClient,
		healthVerifier,
		metricsComponent,
		nodeDiscoverer,
		coordinator,
		recoveryManager,
		resourceFactoryAdapter,
	)

	// Start the service
	if err := updaterService.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start updater service: %w", err)
	}

	return updaterService, nil
}

// providerOptions holds optional dependencies for the provider
type providerOptions struct {
	backendClient    interfaces.BackendClient
	healthVerifier   interfaces.HealthVerifier
	stateMachine     interfaces.StateMachine
	metricsComponent interfaces.MetricsProvider
	nodeDiscoverer   interfaces.NodeDiscoverer
	coordinator      interfaces.Coordinator
	recoveryManager  interfaces.RecoveryManager
}

// ProviderOption defines a functional option for configuring the provider
type ProviderOption func(*providerOptions)

// WithBackendClient sets a custom backend client
func WithBackendClient(client interfaces.BackendClient) ProviderOption {
	return func(o *providerOptions) {
		o.backendClient = client
	}
}

// WithHealthVerifier sets a custom health verifier
func WithHealthVerifier(verifier interfaces.HealthVerifier) ProviderOption {
	return func(o *providerOptions) {
		o.healthVerifier = verifier
	}
}

// WithStateMachine sets a custom state machine
func WithStateMachine(sm interfaces.StateMachine) ProviderOption {
	return func(o *providerOptions) {
		o.stateMachine = sm
	}
}

// WithMetricsComponent sets a custom metrics component
func WithMetricsComponent(comp interfaces.MetricsProvider) ProviderOption {
	return func(o *providerOptions) {
		o.metricsComponent = comp
	}
}

// WithNodeDiscoverer sets a custom node discoverer
func WithNodeDiscoverer(discoverer interfaces.NodeDiscoverer) ProviderOption {
	return func(o *providerOptions) {
		o.nodeDiscoverer = discoverer
	}
}

// WithCoordinator sets a custom coordinator
func WithCoordinator(coordinator interfaces.Coordinator) ProviderOption {
	return func(o *providerOptions) {
		o.coordinator = coordinator
	}
}

// WithRecoveryManager sets a custom recovery manager
func WithRecoveryManager(recoveryManager interfaces.RecoveryManager) ProviderOption {
	return func(o *providerOptions) {
		o.recoveryManager = recoveryManager
	}
}
