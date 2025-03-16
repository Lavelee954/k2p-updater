package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"k2p-updater/cmd/app"
	"k2p-updater/internal/api/v1/handler"
	"k2p-updater/internal/api/v1/middleware"
	"k2p-updater/internal/features/exporter"
	exporterDomain "k2p-updater/internal/features/exporter/domain"
	"k2p-updater/internal/features/metric"
	"k2p-updater/internal/features/metric/domain"
	"k2p-updater/internal/features/updater"
	"k2p-updater/internal/features/updater/domain/interfaces"
	"k2p-updater/pkg/resource"

	"github.com/gin-gonic/gin"
)

// Server encapsulates the application server functionality
type Server struct {
	cfg             *app.Config
	kubeClients     *app.KubeClients
	exporterSvc     exporterDomain.Provider
	metricsSvc      domain.Provider
	resourceFactory *resource.Factory
	updaterSvc      interfaces.Provider
	httpServer      *http.Server
	router          *gin.Engine
}

// NewServer creates a new server instance with all dependencies initialized.
func NewServer() (*Server, error) {
	// Load configuration
	cfg, err := app.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

	// Create Kubernetes clients
	kubeClients, err := app.NewKubeClients(&cfg.Kubernetes)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes clients: %w", err)
	}

	// Set up Gin router with appropriate mode
	if cfg.App.LogLevel == "debug" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.Default()

	return &Server{
		cfg:         cfg,
		kubeClients: kubeClients,
		router:      router,
	}, nil
}

// Initialize sets up all required services and routes
func (s *Server) Initialize(ctx context.Context) error {

	log.Println("Initializing server components...")

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

	// 2. Initialize metrics service
	log.Println("Initializing metrics service...")
	metricsCtx, metricsCancel := context.WithTimeout(initCtx, 30*time.Second)
	defer metricsCancel()

	metricsProvider, err := metric.NewProvider(metricsCtx, &s.cfg.Metrics, s.kubeClients.FullClientSet, exporterProvider)
	if err != nil {
		return fmt.Errorf("failed to initialize metrics service: %w", err)
	}
	log.Println("Metrics service initialized successfully")
	s.metricsSvc = metricsProvider

	// 3. Create resource factory
	log.Println("Creating resource factory...")
	resourceDefs := convertResourceDefinitions(s.cfg.Resources.Definitions)
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
	log.Println("Resource factory created successfully")

	// After resource factory creation
	log.Println("Testing direct event creation...")

	// 4. Initialize updater service
	log.Println("Initializing updater service...")
	updaterCtx, updaterCancel := context.WithTimeout(initCtx, 30*time.Second)
	defer updaterCancel()

	// Create component factory
	componentFactory := updater.NewDefaultComponentFactory()

	updaterProvider, err := updater.NewProvider(
		updaterCtx,
		s.cfg,
		s.metricsSvc,
		s.exporterSvc,
		s.resourceFactory,
		s.kubeClients.FullClientSet,
		componentFactory,
	)
	if err != nil {
		return fmt.Errorf("failed to initialize updater service: %w", err)
	}
	log.Println("Updater service initialized successfully")
	s.updaterSvc = updaterProvider

	// 5. Setup HTTP routes and handlers
	if err := s.setupRoutes(); err != nil {
		return fmt.Errorf("failed to setup HTTP routes: %w", err)
	}
	log.Println("HTTP routes configured successfully")

	return nil
}

// setupRoutes configures all HTTP routes and handlers
func (s *Server) setupRoutes() error {
	// Add common middleware
	s.router.Use(middleware.LoggingMiddleware())

	// Create and register health handler
	healthHandler := handler.NewHealthHandler()
	healthHandler.SetupRoutes(s.router)

	// Create and register status handler
	statusHandler := handler.NewStatusHandler(s.updaterSvc)
	statusHandler.SetupRoutes(s.router)

	// Add swagger documentation if in development mode
	if s.cfg.App.LogLevel == "debug" {
		// s.router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	}

	return nil
}

// Start begins the server operation and handles graceful shutdown
func (s *Server) Start(ctx context.Context) error {
	// Log server starting
	log.Printf("Starting service on port %s", s.cfg.Server.Port)

	// Create HTTP server with the Gin router
	s.httpServer = &http.Server{
		Addr:         s.cfg.Server.Port,
		Handler:      s.router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start HTTP server in background
	serverErrCh := make(chan error, 1)
	go func() {
		log.Printf("HTTP server listening on %s", s.cfg.Server.Port)
		if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrCh <- fmt.Errorf("HTTP server error: %w", err)
		}
	}()

	// Start background processing for updater service
	if err := s.updaterSvc.Start(ctx); err != nil {
		return fmt.Errorf("failed to start updater service: %w", err)
	}

	// Now we only return if there's a server error
	// We don't return on context.Done() as that would cause premature shutdown
	select {
	case err := <-serverErrCh:
		return err
	default:
		// Don't block, continue execution
	}

	return nil
}

// Shutdown performs graceful shutdown of the server and all services
func (s *Server) Shutdown() error {
	log.Println("Performing graceful shutdown...")

	// Create a context with timeout for shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), s.cfg.Server.ShutdownTimeout)
	defer shutdownCancel()

	// Shutdown HTTP server
	if s.httpServer != nil {
		log.Println("Shutting down HTTP server...")
		if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("HTTP server shutdown error: %w", err)
		}
	}

	// Stop the updater service if it's running
	if s.updaterSvc != nil {
		log.Println("Stopping updater service...")
		s.updaterSvc.Stop()
	}

	// Stop the metrics service if it's running
	if s.metricsSvc != nil {
		log.Println("Stopping metrics service...")
		s.metricsSvc.Stop()
	}

	// Stop the exporter service if it's running
	if s.exporterSvc != nil {
		log.Println("Stopping exporter service...")
		s.exporterSvc.Stop()
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

	// Initialize the server
	server, err := NewServer()
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

	log.Println("Updater service initialized successfully")

	// Wait for shutdown signal
	sig := <-signals
	log.Printf("Received signal: %v, initiating shutdown", sig)
	cancel() // Cancel the context to signal all background processes to stop

	log.Println("Beginning graceful shutdown...")

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
