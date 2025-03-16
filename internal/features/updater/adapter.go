package updater

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"k2p-updater/cmd/app"
	exporterDomain "k2p-updater/internal/features/exporter/domain"
	metricDomain "k2p-updater/internal/features/metric/domain"
	"k2p-updater/internal/features/updater/domain"
	"k2p-updater/internal/features/updater/service"
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
) (domain.Provider, error) {
	// Initialize provider options with defaults
	opts := &providerOptions{}
	for _, option := range options {
		option(opts)
	}

	// Create domain config from app config
	domainConfig := domain.UpdaterConfig{
		ScaleThreshold:         config.Updater.ScaleThreshold,
		ScaleUpStep:            config.Updater.ScaleUpStep,
		CooldownPeriod:         config.Updater.CooldownPeriod,
		MonitoringInterval:     time.Minute, // Default to 1 minute
		CooldownUpdateInterval: time.Minute, // Default to 1 minute
		Namespace:              config.Kubernetes.Namespace,
	}

	// Create resource factory adapter for the domain interface
	resourceFactoryAdapter := NewResourceFactoryAdapter(resourceFactory)

	// Create dependencies if not provided (for backwards compatibility)
	backendClient := opts.backendClient
	if backendClient == nil {
		httpClient := &http.Client{Timeout: config.Backend.Timeout}
		backendClient = service.NewBackendClient(&config.Backend, httpClient)
	}

	healthVerifier := opts.healthVerifier
	if healthVerifier == nil {
		healthVerifier = service.NewHealthVerifier(exporterService)
	}

	stateMachine := opts.stateMachine
	if stateMachine == nil {
		stateMachine = service.NewStateMachine(resourceFactory, nil) // Use default handlers
	}

	metricsComponent := opts.metricsComponent
	if metricsComponent == nil {
		metricsConfig := &app.MetricsConfig{
			WindowSize:     10 * time.Minute,
			SlidingSize:    1 * time.Minute,
			CooldownPeriod: config.Updater.CooldownPeriod,
			ScaleTrigger:   config.Updater.ScaleThreshold,
		}

		// Use the new factory method
		metricsComponent = service.NewMetricsComponent(
			metricsService,
			stateMachine,
			metricsConfig,
		)
	}

	nodeDiscoverer := opts.nodeDiscoverer
	if nodeDiscoverer == nil {
		nodeDiscoverer = service.NewNodeDiscoverer(kubeClient, config.Kubernetes.Namespace)
	}

	coordinator := opts.coordinator
	if coordinator == nil {
		coordinator = service.NewCoordinationManager(stateMachine)
	}

	recoveryManager := opts.recoveryManager
	if recoveryManager == nil {
		recoveryManager = service.NewRecoveryManager(stateMachine)
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
	backendClient    domain.BackendClient
	healthVerifier   domain.HealthVerifier
	stateMachine     domain.StateMachine
	metricsComponent service.MetricsComponent
	nodeDiscoverer   domain.NodeDiscoverer
	coordinator      domain.Coordinator
	recoveryManager  domain.RecoveryManager
}

// ProviderOption defines a functional option for configuring the provider
type ProviderOption func(*providerOptions)

// WithBackendClient sets a custom backend client
func WithBackendClient(client domain.BackendClient) ProviderOption {
	return func(o *providerOptions) {
		o.backendClient = client
	}
}

// WithHealthVerifier sets a custom health verifier
func WithHealthVerifier(verifier domain.HealthVerifier) ProviderOption {
	return func(o *providerOptions) {
		o.healthVerifier = verifier
	}
}

// WithStateMachine sets a custom state machine
func WithStateMachine(sm domain.StateMachine) ProviderOption {
	return func(o *providerOptions) {
		o.stateMachine = sm
	}
}

// WithMetricsComponent sets a custom metrics component
func WithMetricsComponent(comp service.MetricsComponent) ProviderOption {
	return func(o *providerOptions) {
		o.metricsComponent = comp
	}
}

// WithNodeDiscoverer sets a custom node discoverer
func WithNodeDiscoverer(discoverer domain.NodeDiscoverer) ProviderOption {
	return func(o *providerOptions) {
		o.nodeDiscoverer = discoverer
	}
}

// WithCoordinator sets a custom coordinator
func WithCoordinator(coordinator domain.Coordinator) ProviderOption {
	return func(o *providerOptions) {
		o.coordinator = coordinator
	}
}

// WithRecoveryManager sets a custom recovery manager
func WithRecoveryManager(recoveryManager domain.RecoveryManager) ProviderOption {
	return func(o *providerOptions) {
		o.recoveryManager = recoveryManager
	}
}
