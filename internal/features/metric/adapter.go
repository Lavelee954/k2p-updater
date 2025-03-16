package metric

import (
	"context"
	"fmt"
	"k2p-updater/cmd/app"
	domainExporter "k2p-updater/internal/features/exporter/domain"
	domainMetric "k2p-updater/internal/features/metric/domain"
	"k2p-updater/internal/features/metric/service"
	"log"

	"k8s.io/client-go/kubernetes"
)

// NewProvider creates and initializes a new metrics provider
func NewProvider(
	ctx context.Context,
	config *app.MetricsConfig,
	kubeClient kubernetes.Interface,
	exporterService domainExporter.Provider,
) (domainMetric.Provider, error) {
	log.Println("Initializing metrics service...")

	// Validate inputs
	if config == nil {
		return nil, fmt.Errorf("metrics config cannot be nil")
	}
	if kubeClient == nil {
		return nil, fmt.Errorf("kubernetes client cannot be nil")
	}
	if exporterService == nil {
		return nil, fmt.Errorf("exporter service cannot be nil")
	}

	// Create metrics service
	metricsService := service.NewMetricsService(
		kubeClient,
		*config,
		exporterService,
	)

	// Make sure the exporter service is initialized before starting metrics collection
	if err := exporterService.WaitForInitialization(ctx); err != nil {
		return nil, fmt.Errorf("failed to wait for exporter service initialization: %w", err)
	}

	// Create a cancellable context for metrics collection
	metricsCtx, cancelFunc := context.WithCancel(context.Background())

	// Set the cancelFunc using the exported method
	metricsService.SetCancelFunc(cancelFunc)

	// Start metrics collection in background
	go func() {
		if err := metricsService.Start(metricsCtx); err != nil {
			log.Printf("Error starting metrics service: %v", err)
		}
	}()

	log.Println("Metrics service initialized")
	return metricsService, nil
}
