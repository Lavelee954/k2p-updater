package server

import (
	"context"
	"errors"
	"fmt"
	"k2p-updater/cmd/app"
	"k2p-updater/internal/features/exporter"
	"k2p-updater/internal/features/exporter/domain"
	"k2p-updater/internal/features/metric"
	"k2p-updater/pkg/resource"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// Run starts the application
func Run() {
	// Create context with cancellation for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle OS signals for graceful shutdown
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	// Listen for shutdown signals in background
	go func() {
		sig := <-signals
		log.Printf("Received signal: %v, initiating shutdown", sig)
		cancel()
	}()

	// 1. Load configuration
	cfg, err := app.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// 2. Create Kubernetes clients
	kcfg, err := app.NewKubeClients(&cfg.Kubernetes)
	if err != nil {
		log.Fatalf("Failed to create Kubernetes clients: %v", err)
	}

	// 3. Initialize exporter service
	log.Println("Initializing exporter service...")
	exporterProvider, err := exporter.NewProvider(ctx, &cfg.Exporter, kcfg.ClientSet)
	if err != nil {
		log.Fatalf("Failed to initialize exporter service: %v", err)
	}
	log.Println("Exporter service initialized successfully")

	// 4. Initialize metrics service - Use FullClientSet instead of ClientSet
	log.Println("Initializing metrics service...")
	metricsProvider, err := metric.NewProvider(ctx, &cfg.Metrics, kcfg.FullClientSet, exporterProvider)
	if err != nil {
		log.Fatalf("Failed to initialize metrics service: %v", err)
	}
	log.Println("Metrics service initialized successfully")

	// 5. Convert resource definitions for the factory
	resourceDefs := convertResourceDefinitions(cfg.Resources.Definitions)

	// 6. Create resource factory
	factory, err := resource.NewFactory(
		cfg.Resources.Namespace,
		cfg.Resources.Group,
		cfg.Resources.Version,
		resourceDefs,
		kcfg.DynamicClient,
		kcfg.ClientSet,
	)
	if err != nil {
		log.Fatalf("Failed to create resource factory: %v", err)
	}

	// 7. Record initialization event
	statusMsg := fmt.Sprintf(
		"Service initialized. Scale threshold: %.1f%%, Window size: %.1f minutes",
		cfg.Metrics.ScaleTrigger,
		cfg.Metrics.WindowSize.Minutes(),
	)
	err = factory.Event().NormalRecord(ctx, "updater", "ServiceStart", statusMsg)
	if err != nil {
		log.Printf("Warning: Failed to record initialization event: %v", err)
	}

	// 8. Update initial status
	statusData := map[string]interface{}{
		"lastUpdateTime": time.Now().Format(time.RFC3339),
		"status":         "Running",
		"message":        "Service initialized successfully",
	}

	err = factory.Status().UpdateGeneric(ctx, "updater", statusData)
	if err != nil {
		log.Printf("Warning: Failed to update initial status: %v", err)
	}

	// 9. Setup HTTP server
	server := &http.Server{
		Addr:    cfg.Server.Port,
		Handler: setupRoutes(exporterProvider, metricsProvider),
	}

	// 10. Start HTTP server in background
	go func() {
		log.Printf("Starting HTTP server on %s", cfg.Server.Port)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// 11. Wait for context cancellation (shutdown signal)
	<-ctx.Done()
	log.Println("Shutdown initiated, gracefully stopping services...")

	// 12. Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer shutdownCancel()

	// Update final status before shutdown
	finalStatusData := map[string]interface{}{
		"lastUpdateTime": time.Now().Format(time.RFC3339),
		"status":         "Shutdown",
		"message":        "Service shutting down gracefully",
	}

	if err := factory.Status().UpdateGeneric(shutdownCtx, "updater", finalStatusData); err != nil {
		log.Printf("Warning: Failed to update final status: %v", err)
	}

	// Record shutdown event
	if err := factory.Event().NormalRecord(shutdownCtx, "updater", "ServiceStop", "Service shutting down"); err != nil {
		log.Printf("Warning: Failed to record shutdown event: %v", err)
	}

	// Shutdown HTTP server
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}

	log.Println("Application shutdown complete")
}

// convertResourceDefinitions converts app-specific configuration to resource package format
func convertResourceDefinitions(appDefs map[string]app.ResourceDefinitionConfig) map[string]resource.FactoryDefinition {
	resourceDefs := make(map[string]resource.FactoryDefinition)

	for key, appDef := range appDefs {
		resourceDefs[key] = resource.FactoryDefinition{
			Resource:    appDef.Resource,
			NameFormat:  appDef.NameFormat,
			StatusField: appDef.StatusField,
			Kind:        appDef.Kind,
			CRName:      appDef.CRName,
		}
	}

	return resourceDefs
}

// setupRoutes configures the HTTP routes for the application
func setupRoutes(exporterProvider domain.Provider, metricsProvider interface{}) http.Handler {
	mux := http.NewServeMux()

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// Status endpoint - shows exporters info
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		exporters := exporterProvider.GetHealthyExporters()
		w.Header().Set("Content-Type", "application/json")

		fmt.Fprintf(w, "{\n  \"status\": \"running\",\n  \"healthy_exporters\": %d,\n", len(exporters))

		// Add exporters info
		fmt.Fprintf(w, "  \"exporters\": [\n")
		for i, exporter := range exporters {
			fmt.Fprintf(w, "    {\n")
			fmt.Fprintf(w, "      \"name\": \"%s\",\n", exporter.Name)
			fmt.Fprintf(w, "      \"node\": \"%s\",\n", exporter.NodeName)
			fmt.Fprintf(w, "      \"ip\": \"%s\",\n", exporter.IP)
			fmt.Fprintf(w, "      \"status\": \"%s\",\n", exporter.Status)
			fmt.Fprintf(w, "      \"last_seen\": \"%s\"\n", exporter.LastSeen.Format(time.RFC3339))

			if i < len(exporters)-1 {
				fmt.Fprintf(w, "    },\n")
			} else {
				fmt.Fprintf(w, "    }\n")
			}
		}
		fmt.Fprintf(w, "  ]\n}")
	})

	// Metrics endpoint
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("# Metrics endpoint - Prometheus format metrics would be here\n"))
	})

	return mux
}
