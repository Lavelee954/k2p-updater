package updater

import (
	"context"
	"fmt"
	"k2p-updater/internal/features/updater/component/backend"
	"k2p-updater/internal/features/updater/component/coordination"
	"k2p-updater/internal/features/updater/component/health"
	"k2p-updater/internal/features/updater/component/metrics"
	"k2p-updater/internal/features/updater/component/recovery"
	resourceUpdater "k2p-updater/internal/features/updater/component/resource"
	"k2p-updater/internal/features/updater/component/state"
	"net/http"
	"time"

	"k2p-updater/cmd/app"
	domainExporter "k2p-updater/internal/features/exporter/domain"
	domainMetric "k2p-updater/internal/features/metric/domain"
	"k2p-updater/internal/features/updater/domain/interfaces"
	"k2p-updater/internal/features/updater/domain/models"
	"k2p-updater/internal/features/updater/service"
	"k2p-updater/pkg/resource"

	"k8s.io/client-go/kubernetes"
)

// ComponentFactory defines an interface for creating updater components
type ComponentFactory interface {
	CreateBackendClient(config *app.BackendConfig) interfaces.BackendClient
	CreateHealthVerifier(exporterService domainExporter.Provider) interfaces.HealthVerifier
	CreateStateMachine(resourceFactory *resource.Factory) interfaces.StateMachine
	CreateMetricsComponent(metricsService domainMetric.Provider, stateMachine interfaces.StateMachine, config *app.MetricsConfig) interfaces.MetricsProvider
	CreateNodeDiscoverer(kubeClient kubernetes.Interface, namespace string) interfaces.NodeDiscoverer
	CreateCoordinator(stateMachine interfaces.StateMachine) interfaces.Coordinator
	CreateRecoveryManager(stateMachine interfaces.StateMachine) interfaces.RecoveryManager
	CreateResourceFactoryAdapter(resourceFactory *resource.Factory) interfaces.ResourceFactory
}

// DefaultComponentFactory is the default implementation of ComponentFactory
type DefaultComponentFactory struct{}

// NewDefaultComponentFactory creates a new default component factory
func NewDefaultComponentFactory() ComponentFactory {
	return &DefaultComponentFactory{}
}

// CreateBackendClient creates a new backend client
func (f *DefaultComponentFactory) CreateBackendClient(config *app.BackendConfig) interfaces.BackendClient {
	httpClient := &http.Client{Timeout: config.Timeout}
	return backend.NewBackendClient(config, httpClient)
}

// CreateHealthVerifier creates a new health verifier
func (f *DefaultComponentFactory) CreateHealthVerifier(exporterService domainExporter.Provider) interfaces.HealthVerifier {
	return health.NewHealthVerifier(exporterService)
}

// CreateStateMachine creates a new state machine
func (f *DefaultComponentFactory) CreateStateMachine(resourceFactory *resource.Factory) interfaces.StateMachine {
	return state.NewStateMachineWithDefaultHandlers(resourceFactory)
}

// CreateMetricsComponent creates a new metrics component
func (f *DefaultComponentFactory) CreateMetricsComponent(metricsService domainMetric.Provider, stateMachine interfaces.StateMachine, config *app.MetricsConfig) interfaces.MetricsProvider {
	return metrics.NewMetricsComponent(metricsService, stateMachine, config)
}

// CreateNodeDiscoverer creates a new node discoverer
func (f *DefaultComponentFactory) CreateNodeDiscoverer(kubeClient kubernetes.Interface, namespace string) interfaces.NodeDiscoverer {
	return coordination.NewNodeDiscoverer(kubeClient, namespace)
}

// CreateCoordinator creates a new coordinator
func (f *DefaultComponentFactory) CreateCoordinator(stateMachine interfaces.StateMachine) interfaces.Coordinator {
	return coordination.NewCoordinationManager(stateMachine)
}

// CreateRecoveryManager creates a new recovery manager
func (f *DefaultComponentFactory) CreateRecoveryManager(stateMachine interfaces.StateMachine) interfaces.RecoveryManager {
	return recovery.NewRecoveryManager(stateMachine)
}

// CreateResourceFactoryAdapter creates a new resource factory adapter
func (f *DefaultComponentFactory) CreateResourceFactoryAdapter(resourceFactory *resource.Factory) interfaces.ResourceFactory {
	return resourceUpdater.NewResourceFactoryAdapter(resourceFactory)
}

// NewProvider creates and initializes a new updater provider with dependencies
func NewProvider(
	ctx context.Context,
	config *app.Config,
	metricsService domainMetric.Provider,
	exporterService domainExporter.Provider,
	resourceFactory *resource.Factory,
	kubeClient kubernetes.Interface,
	componentFactory ComponentFactory,
	options ...ProviderOption,
) (interfaces.Provider, error) {
	// Use default component factory if not provided
	if componentFactory == nil {
		componentFactory = NewDefaultComponentFactory()
	}

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

	// Create resource factory adapter
	resourceFactoryAdapter := opts.resourceFactory
	if resourceFactoryAdapter == nil {
		resourceFactoryAdapter = componentFactory.CreateResourceFactoryAdapter(resourceFactory)
	}

	// Create dependencies if not provided (for backwards compatibility)
	backendClient := opts.backendClient
	if backendClient == nil {
		backendClient = componentFactory.CreateBackendClient(&config.Backend)
	}

	healthVerifier := opts.healthVerifier
	if healthVerifier == nil {
		healthVerifier = componentFactory.CreateHealthVerifier(exporterService)
	}

	stateMachine := opts.stateMachine
	if stateMachine == nil {
		stateMachine = componentFactory.CreateStateMachine(resourceFactory)
	}

	metricsComponent := opts.metricsComponent
	if metricsComponent == nil {
		metricsConfig := &app.MetricsConfig{
			WindowSize:     10 * time.Minute,
			SlidingSize:    1 * time.Minute,
			CooldownPeriod: config.Updater.CooldownPeriod,
			ScaleTrigger:   config.Updater.ScaleThreshold,
		}

		// Use the component factory to create the component
		metricsComponent = componentFactory.CreateMetricsComponent(
			metricsService,
			stateMachine,
			metricsConfig,
		)
	}

	nodeDiscoverer := opts.nodeDiscoverer
	if nodeDiscoverer == nil {
		nodeDiscoverer = componentFactory.CreateNodeDiscoverer(kubeClient, config.Kubernetes.Namespace)
	}

	coordinator := opts.coordinator
	if coordinator == nil {
		coordinator = componentFactory.CreateCoordinator(stateMachine)
	}

	recoveryManager := opts.recoveryManager
	if recoveryManager == nil {
		recoveryManager = componentFactory.CreateRecoveryManager(stateMachine)
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

// ProviderOption defines a functional option for configuring the provider
type ProviderOption func(*providerOptions)

// WithResourceFactory new option for resource factory
func WithResourceFactory(factory interfaces.ResourceFactory) ProviderOption {
	return func(o *providerOptions) {
		o.resourceFactory = factory
	}
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
	resourceFactory  interfaces.ResourceFactory // Added this field
}

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
