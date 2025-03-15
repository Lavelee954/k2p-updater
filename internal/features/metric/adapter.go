package metric

import (
	"context"
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

	// Create metrics service
	metricsService := service.NewMetricsService(
		kubeClient,
		*config,
		exporterService,
	)

	// Start metrics collection in background
	go func() {
		if err := metricsService.Start(ctx); err != nil {
			log.Printf("Error starting metrics service: %v", err)
		}
	}()

	log.Println("Metrics service initialized")
	return metricsService, nil
}
