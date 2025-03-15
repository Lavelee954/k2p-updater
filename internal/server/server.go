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

// Server encapsulates the application server functionality
type Server struct {
	cfg             *app.Config
	kubeClients     *app.KubeClients
	exporterSvc     domain.Provider
	metricsSvc      interface{}
	resourceFactory *resource.Factory
	httpServer      *http.Server
}

// NewServer creates a new server instance
func NewServer(ctx context.Context) (*Server, error) {
	// 1. Load configuration
	cfg, err := app.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

	// 2. Create Kubernetes clients
	kubeClients, err := app.NewKubeClients(&cfg.Kubernetes)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes clients: %w", err)
	}

	return &Server{
		cfg:         cfg,
		kubeClients: kubeClients,
	}, nil
}

// Initialize sets up all required services
func (s *Server) Initialize(ctx context.Context) error {
	// Create a context with timeout for initialization
	initCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	// 1. Initialize exporter service
	log.Println("Initializing exporter service...")
	exporterCtx, exporterCancel := context.WithTimeout(initCtx, 30*time.Second)
	defer exporterCancel()

	exporterProvider, err := exporter.NewProvider(exporterCtx, &s.cfg.Exporter, s.kubeClients.ClientSet)
	if err != nil {
		return fmt.Errorf("failed to initialize exporter service: %w", err)
	}
	log.Println("Exporter service initialized successfully")
	s.exporterSvc = exporterProvider

	// 2. Initialize metrics service - Use FullClientSet instead of ClientSet
	log.Println("Initializing metrics service...")
	metricsCtx, metricsCancel := context.WithTimeout(initCtx, 30*time.Second)
	defer metricsCancel()

	metricsProvider, err := metric.NewProvider(metricsCtx, &s.cfg.Metrics, s.kubeClients.FullClientSet, exporterProvider)
	if err != nil {
		return fmt.Errorf("failed to initialize metrics service: %w", err)
	}
	log.Println("Metrics service initialized successfully")
	s.metricsSvc = metricsProvider

	// 3. Convert resource definitions for the factory
	resourceDefs := convertResourceDefinitions(s.cfg.Resources.Definitions)

	// 4. Create resource factory
	factory, err := resource.NewFactory(
		s.cfg.Resources.Namespace,
		s.cfg.Resources.Group,
		s.cfg.Resources.Version,
		resourceDefs,
		s.kubeClients.DynamicClient,
		s.kubeClients.ClientSet,
	)
	if err != nil {
		return fmt.Errorf("failed to create resource factory: %w", err)
	}
	s.resourceFactory = factory

	return nil
}

// Start begins the server operation and handles graceful shutdown
func (s *Server) Start(ctx context.Context) error {
	// 1. Record initialization event
	statusMsg := fmt.Sprintf(
		"Service initialized. Scale threshold: %.1f%%, Window size: %.1f minutes",
		s.cfg.Metrics.ScaleTrigger,
		s.cfg.Metrics.WindowSize.Minutes(),
	)
	if err := s.resourceFactory.Event().NormalRecord(ctx, "updater", "ServiceStart", statusMsg); err != nil {
		log.Printf("Warning: Failed to record initialization event: %v", err)
	}

	// 2. Update initial status
	statusData := map[string]interface{}{
		"lastUpdateTime": time.Now().Format(time.RFC3339),
		"status":         "Running",
		"message":        "Service initialized successfully",
	}

	if err := s.resourceFactory.Status().UpdateGeneric(ctx, "updater", statusData); err != nil {
		log.Printf("Warning: Failed to update initial status: %v", err)
	}

	// 3. Start HTTP server in background
	serverErrCh := make(chan error, 1)
	go func() {
		log.Printf("Starting HTTP server on %s", s.cfg.Server.Port)
		if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrCh <- fmt.Errorf("HTTP server error: %w", err)
		}
	}()

	// 4. Wait for either server error or context cancellation
	select {
	case err := <-serverErrCh:
		return err
	case <-ctx.Done():
		log.Println("Shutdown initiated, gracefully stopping services...")
	}

	return nil
}

// Shutdown performs graceful shutdown of the server
func (s *Server) Shutdown() error {
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), s.cfg.Server.ShutdownTimeout)
	defer shutdownCancel()

	// Update final status before shutdown
	finalStatusData := map[string]interface{}{
		"lastUpdateTime": time.Now().Format(time.RFC3339),
		"status":         "Shutdown",
		"message":        "Service shutting down gracefully",
	}

	if err := s.resourceFactory.Status().UpdateGeneric(shutdownCtx, "updater", finalStatusData); err != nil {
		log.Printf("Warning: Failed to update final status: %v", err)
	}

	// Record shutdown event
	if err := s.resourceFactory.Event().NormalRecord(shutdownCtx, "updater", "ServiceStop", "Service shutting down"); err != nil {
		log.Printf("Warning: Failed to record shutdown event: %v", err)
	}

	// Shutdown HTTP server
	if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("HTTP server shutdown error: %w", err)
	}

	log.Println("Application shutdown complete")
	return nil
}

// Run starts the application and returns an exit code
func Run() int {
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

	// Initialize the server
	server, err := NewServer(ctx)
	if err != nil {
		log.Printf("Failed to create server: %v", err)
		return 1
	}

	// Initialize services
	if err := server.Initialize(ctx); err != nil {
		log.Printf("Failed to initialize server: %v", err)
		return 1
	}

	// Start the server
	if err := server.Start(ctx); err != nil {
		log.Printf("Server error: %v", err)
		return 1
	}

	// Shutdown gracefully
	if err := server.Shutdown(); err != nil {
		log.Printf("Error during shutdown: %v", err)
		return 1
	}

	return 0
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
