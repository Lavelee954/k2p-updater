package service

import (
	"context"
	"fmt"
	"k2p-updater/cmd/app"
	"k2p-updater/internal/features/exporter/domain"
	"log"
	"sync"
	"time"

	v1 "k8s.io/api/core/v1"
)

// Service implements the domain.Provider interface.
type Service struct {
	podFetcher      domain.PodFetcher
	healthChecker   domain.HealthChecker
	exporters       map[string]*domain.NodeExporter
	config          *app.ExporterConfig
	mu              sync.RWMutex
	healthCheckChan chan string
	initialized     chan struct{}
	startOnce       sync.Once
}

// NewService creates a new exporter service with default dependencies.
func NewService(client domain.KubernetesClient, config *app.ExporterConfig) *Service {
	return &Service{
		podFetcher:      newPodFetcher(client),
		healthChecker:   newHealthChecker(config.HealthCheckTimeout),
		exporters:       make(map[string]*domain.NodeExporter),
		config:          config,
		healthCheckChan: make(chan string, 100),
		initialized:     make(chan struct{}),
	}
}

// NewWithDependencies creates a new exporter service with custom dependencies.
func NewWithDependencies(
	podFetcher domain.PodFetcher,
	healthChecker domain.HealthChecker,
	config *app.ExporterConfig,
) *Service {
	return &Service{
		podFetcher:      podFetcher,
		healthChecker:   healthChecker,
		exporters:       make(map[string]*domain.NodeExporter),
		config:          config,
		healthCheckChan: make(chan string, 100),
		initialized:     make(chan struct{}),
	}
}

// Start initializes and starts the exporter service.
func (s *Service) Start(ctx context.Context) error {
	var startErr error
	s.startOnce.Do(func() {
		exporters, err := s.fetchNodeExporters(ctx)
		if err != nil {
			startErr = fmt.Errorf("initial exporter fetch failed: %w", err)
			return
		}

		s.mu.Lock()
		for _, exporter := range exporters {
			s.exporters[exporter.NodeName] = &domain.NodeExporter{
				Name:     exporter.Name,
				IP:       exporter.IP,
				NodeName: exporter.NodeName,
				LastSeen: time.Now(),
				Status:   domain.StatusRunning,
			}
		}
		s.mu.Unlock()

		close(s.initialized)

		go s.runHealthCheck(ctx)
		go s.processHealthChecks(ctx)
	})

	return startErr
}

// GetExporter returns an exporter for the specified node if it exists.
func (s *Service) GetExporter(nodeName string) (domain.NodeExporter, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	exporter, exists := s.exporters[nodeName]
	if !exists {
		return domain.NodeExporter{}, false
	}
	return *exporter, exists
}

// GetHealthyExporters returns a list of all healthy exporters.
func (s *Service) GetHealthyExporters() []domain.NodeExporter {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var healthy []domain.NodeExporter
	for _, exporter := range s.exporters {
		if exporter.Status == domain.StatusRunning {
			healthy = append(healthy, *exporter)
		}
	}
	return healthy
}

// WaitForInitialization waits for the service to be initialized or context cancellation.
func (s *Service) WaitForInitialization(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.initialized:
		return nil
	}
}

// runHealthCheck periodically checks all exporters.
func (s *Service) runHealthCheck(ctx context.Context) {
	ticker := time.NewTicker(s.config.UpdateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.checkAllExporters(ctx)
		}
	}
}

// checkAllExporters verifies all node exporters.
func (s *Service) checkAllExporters(ctx context.Context) {
	exporters, err := s.fetchNodeExporters(ctx)
	if err != nil {
		log.Printf("Failed to fetch node exporters: %v", err)
		return
	}

	s.updateExporterStatuses(exporters)
}

// updateExporterStatuses updates the status of exporters.
func (s *Service) updateExporterStatuses(exporters []domain.NodeExporter) {
	s.mu.Lock()
	defer s.mu.Unlock()

	currentExporters := make(map[string]bool)

	// Process existing and new exporters
	for _, exporter := range exporters {
		currentExporters[exporter.NodeName] = true

		existing, exists := s.exporters[exporter.NodeName]
		if !exists {
			s.exporters[exporter.NodeName] = &domain.NodeExporter{
				Name:     exporter.Name,
				IP:       exporter.IP,
				NodeName: exporter.NodeName,
				LastSeen: time.Now(),
				Status:   domain.StatusRunning,
			}
			log.Printf("New exporter detected - Node: %s, IP: %s", exporter.NodeName, exporter.IP)
			continue
		}

		if existing.IP != exporter.IP {
			log.Printf("Exporter IP changed - Node: %s, Old IP: %s, New IP: %s",
				exporter.NodeName, existing.IP, exporter.IP)
			existing.IP = exporter.IP
			existing.LastSeen = time.Now()
			existing.Status = domain.StatusRunning
			existing.RetryCount = 0

			s.queueHealthCheck(exporter.NodeName)
		}
	}

	// Check for missing exporters
	for nodeName, exporter := range s.exporters {
		if !currentExporters[nodeName] {
			log.Printf("Exporter not found - Node: %s, Last IP: %s", nodeName, exporter.IP)
			exporter.Status = domain.StatusUnknown
			s.queueHealthCheck(nodeName)
		}
	}
}

// queueHealthCheck adds a node to the health check queue.
func (s *Service) queueHealthCheck(nodeName string) {
	select {
	case s.healthCheckChan <- nodeName:
		// Successfully queued
	default:
		log.Printf("Health check channel full, skipping check for %s", nodeName)
	}
}

// processHealthChecks processes health check requests.
func (s *Service) processHealthChecks(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case nodeName := <-s.healthCheckChan:
			s.verifyExporterHealth(ctx, nodeName)
		}
	}
}

// verifyExporterHealth checks health of an exporter.
func (s *Service) verifyExporterHealth(ctx context.Context, nodeName string) {
	s.mu.RLock()
	exporter, exists := s.exporters[nodeName]
	s.mu.RUnlock()

	if !exists {
		return
	}

	healthy, err := s.healthChecker.CheckHealth(ctx, exporter.IP, s.config.MetricsPort)
	if err != nil {
		log.Printf("Health check failed for %s: %v", nodeName, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if healthy {
		exporter.Status = domain.StatusRunning
		exporter.RetryCount = 0
		exporter.LastSeen = time.Now()
		return
	}

	exporter.RetryCount++
	if exporter.RetryCount >= s.config.MaxRetries {
		exporter.Status = domain.StatusFailed
		log.Printf("Exporter marked as failed - Node: %s, IP: %s", nodeName, exporter.IP)
	}
}

// fetchNodeExporters retrieves node-exporter pods.
func (s *Service) fetchNodeExporters(ctx context.Context) ([]domain.NodeExporter, error) {
	log.Println("==== Fetching node exporters ====")

	pods, err := s.podFetcher.FetchPods(ctx, s.config.Namespace, s.config.AppLabel)
	if err != nil {
		return nil, fmt.Errorf("failed to list node-exporter pods in namespace %s: %w",
			s.config.Namespace, err)
	}

	log.Printf("Found %d total pods with label %s", len(pods), s.config.AppLabel)

	if len(pods) == 0 {
		return nil, fmt.Errorf("no node-exporter pods found in namespace %s",
			s.config.Namespace)
	}

	return s.processExporterPods(pods)
}

// processExporterPods extracts exporter information from pods.
func (s *Service) processExporterPods(pods []v1.Pod) ([]domain.NodeExporter, error) {
	var exporters []domain.NodeExporter
	var runningCount, nonRunningCount int

	for _, pod := range pods {
		if pod.Status.Phase != v1.PodRunning {
			nonRunningCount++
			log.Printf("Skipping pod %s (status: %s)", pod.Name, pod.Status.Phase)
			continue
		}

		runningCount++
		exporter := domain.NodeExporter{
			Name:     pod.Name,
			IP:       pod.Status.PodIP,
			NodeName: pod.Spec.NodeName,
			LastSeen: time.Now(),
			Status:   domain.StatusRunning,
		}
		exporters = append(exporters, exporter)

		log.Printf("Found running node-exporter: %s on node %s (IP: %s)",
			exporter.Name, exporter.NodeName, exporter.IP)
	}

	log.Printf("Exporter summary: %d running, %d not running", runningCount, nonRunningCount)

	if len(exporters) == 0 {
		return nil, fmt.Errorf("no running node-exporter pods found")
	}

	return exporters, nil
}
