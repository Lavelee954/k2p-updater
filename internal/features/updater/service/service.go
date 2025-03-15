package service

import (
	"context"
	"fmt"
	"k2p-updater/cmd/app"
	metricDomain "k2p-updater/internal/features/metric/domain"
	updaterDomain "k2p-updater/internal/features/updater/domian"
	"k2p-updater/pkg/resource"
	"log"
	"sync"
	"time"
)

// UpdaterService implements the updater.Provider interface
type UpdaterService struct {
	config          updaterDomain.UpdaterConfig
	stateMachine    updaterDomain.StateMachine
	metricsService  metricDomain.Provider
	backendClient   updaterDomain.BackendClient
	healthVerifier  updaterDomain.HealthVerifier
	resourceFactory *resource.Factory

	// Control structures
	startOnce  sync.Once
	stopChan   chan struct{}
	nodes      []string
	nodesMutex sync.RWMutex
}

// NewUpdaterService creates a new updater service
func NewUpdaterService(
	config *app.UpdaterConfig,
	metricsService metricDomain.Provider,
	backendClient updaterDomain.BackendClient,
	healthVerifier updaterDomain.HealthVerifier,
	resourceFactory *resource.Factory,
) updaterDomain.Provider {
	// Convert app config to domain config
	domainConfig := updaterDomain.UpdaterConfig{
		ScaleThreshold:         config.ScaleThreshold,
		ScaleUpStep:            config.ScaleUpStep,
		CooldownPeriod:         config.CooldownPeriod,
		MonitoringInterval:     time.Minute, // Default to 1 minute
		CooldownUpdateInterval: time.Minute, // Default to 1 minute
	}

	// Create state machine
	stateMachine := NewStateMachine(resourceFactory)

	return &UpdaterService{
		config:          domainConfig,
		stateMachine:    stateMachine,
		metricsService:  metricsService,
		backendClient:   backendClient,
		healthVerifier:  healthVerifier,
		resourceFactory: resourceFactory,
		stopChan:        make(chan struct{}),
		nodes:           []string{},
	}
}

// Start begins the updater service processing
func (s *UpdaterService) Start(ctx context.Context) error {
	var startErr error

	s.startOnce.Do(func() {
		log.Println("Starting updater service...")

		// Get the list of nodes to monitor
		if err := s.discoverNodes(ctx); err != nil {
			startErr = fmt.Errorf("failed to discover nodes: %w", err)
			return
		}

		// Initialize all nodes to the initial state
		for _, nodeName := range s.nodes {
			if err := s.initializeNode(ctx, nodeName); err != nil {
				log.Printf("Failed to initialize node %s: %v", nodeName, err)
				// Continue with other nodes
			}
		}

		// Start background monitoring
		go s.monitoringLoop(ctx)
		go s.cooldownLoop(ctx)
	})

	return startErr
}

// GetStateMachine returns the state machine instance
func (s *UpdaterService) GetStateMachine() updaterDomain.StateMachine {
	return s.stateMachine
}

// RequestSpecUp requests a spec up for a node
func (s *UpdaterService) RequestSpecUp(ctx context.Context, nodeName string) error {
	// Get current node status
	status, err := s.stateMachine.GetStatus(ctx, nodeName)
	if err != nil {
		return fmt.Errorf("failed to get node status: %w", err)
	}

	// Only allow spec up from appropriate states
	if status.CurrentState != updaterDomain.StateInProgressVmSpecUp &&
		status.CurrentState != updaterDomain.StateMonitoring {
		return fmt.Errorf("node %s is in %s state, cannot request spec up",
			nodeName, status.CurrentState)
	}

	// Get current CPU utilization
	currentCPU, _, err := s.GetNodeCPUUtilization(ctx, nodeName)
	if err != nil {
		return fmt.Errorf("failed to get CPU utilization: %w", err)
	}

	// Request VM spec up from backend
	success, err := s.backendClient.RequestVMSpecUp(ctx, nodeName, currentCPU)
	if err != nil {
		// Trigger failure event
		data := map[string]interface{}{
			"error": err.Error(),
		}
		if err2 := s.stateMachine.HandleEvent(ctx, nodeName, updaterDomain.EventSpecUpFailed, data); err2 != nil {
			log.Printf("Failed to handle spec up failure event: %v", err2)
		}
		return fmt.Errorf("backend request failed: %w", err)
	}

	if !success {
		// Trigger failure event
		data := map[string]interface{}{
			"error": "Backend returned unsuccessful response",
		}
		if err := s.stateMachine.HandleEvent(ctx, nodeName, updaterDomain.EventSpecUpFailed, data); err != nil {
			log.Printf("Failed to handle spec up failure event: %v", err)
		}
		return fmt.Errorf("backend returned unsuccessful response")
	}

	// Trigger success event
	data := map[string]interface{}{
		"cpuUtilization": currentCPU,
	}
	if err := s.stateMachine.HandleEvent(ctx, nodeName, updaterDomain.EventSpecUpRequested, data); err != nil {
		log.Printf("Failed to handle spec up requested event: %v", err)
		return fmt.Errorf("failed to update state: %w", err)
	}

	return nil
}

// VerifySpecUpHealth verifies the health of a node after spec up
func (s *UpdaterService) VerifySpecUpHealth(ctx context.Context, nodeName string) (bool, error) {
	// Get current node status
	status, err := s.stateMachine.GetStatus(ctx, nodeName)
	if err != nil {
		return false, fmt.Errorf("failed to get node status: %w", err)
	}

	// Check if spec up is completed with backend
	completed, err := s.backendClient.GetVMSpecUpStatus(ctx, nodeName)
	if err != nil {
		return false, fmt.Errorf("failed to get spec up status: %w", err)
	}

	if !completed {
		return false, nil
	}

	// If completed, check health
	healthy, err := s.healthVerifier.VerifyNodeHealth(ctx, nodeName)
	if err != nil {
		return false, fmt.Errorf("health verification failed: %w", err)
	}

	// Trigger appropriate event based on health check
	data := map[string]interface{}{
		"specUpCompleted":   true,
		"healthCheckPassed": healthy,
	}

	if healthy {
		if err := s.stateMachine.HandleEvent(ctx, nodeName, updaterDomain.EventHealthCheckPassed, data); err != nil {
			log.Printf("Failed to handle health check passed event: %v", err)
		}
	} else {
		if err := s.stateMachine.HandleEvent(ctx, nodeName, updaterDomain.EventHealthCheckFailed, data); err != nil {
			log.Printf("Failed to handle health check failed event: %v", err)
		}
	}

	return healthy, nil
}

// GetNodeCPUUtilization gets the current CPU utilization for a node
func (s *UpdaterService) GetNodeCPUUtilization(ctx context.Context, nodeName string) (float64, float64, error) {
	// Get current CPU
	currentCPU, err := s.metricsService.GetNodeCPUUsage(nodeName)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get current CPU usage: %w", err)
	}

	// Get window average
	windowAvg, err := s.metricsService.GetWindowAverageCPU(nodeName)
	if err != nil {
		// If we can't get window average, just use current
		windowAvg = currentCPU
	}

	return currentCPU, windowAvg, nil
}

// IsCooldownActive checks if a node is in cooldown period
func (s *UpdaterService) IsCooldownActive(ctx context.Context, nodeName string) (bool, time.Duration, error) {
	// Get current node status
	status, err := s.stateMachine.GetStatus(ctx, nodeName)
	if err != nil {
		return false, 0, fmt.Errorf("failed to get node status: %w", err)
	}

	// Check if in cooldown state or pending state
	inCooldown := status.CurrentState == updaterDomain.StateCoolDown ||
		status.CurrentState == updaterDomain.StatePendingVmSpecUp

	// Calculate remaining cooldown time
	var remaining time.Duration
	if !status.CoolDownEndTime.IsZero() {
		remaining = time.Until(status.CoolDownEndTime)
		if remaining < 0 {
			remaining = 0
		}
	}

	return inCooldown, remaining, nil
}

// Private helper methods

// discoverNodes discovers control plane nodes to monitor
func (s *UpdaterService) discoverNodes(ctx context.Context) error {
	// In a real implementation, this would discover nodes from Kubernetes API
	// For simplicity, we'll just use a placeholder
	s.nodesMutex.Lock()
	defer s.nodesMutex.Unlock()

	// TODO: Implement proper node discovery
	s.nodes = []string{"master-1", "master-2", "master-3"}
	log.Printf("Discovered %d control plane nodes", len(s.nodes))

	return nil
}

// initializeNode initializes a node in the state machine
func (s *UpdaterService) initializeNode(ctx context.Context, nodeName string) error {
	// Start in cooldown state
	cooldownEnd := time.Now().Add(s.config.CooldownPeriod)

	// Get current CPU utilization
	currentCPU, windowAvg, err := s.GetNodeCPUUtilization(ctx, nodeName)
	if err != nil {
		log.Printf("Warning: Failed to get initial CPU utilization for %s: %v", nodeName, err)
		// Continue with zero values
	}

	// Initialize with data
	data := map[string]interface{}{
		"cpuUtilization":           currentCPU,
		"windowAverageUtilization": windowAvg,
		"coolDownEndTime":          cooldownEnd,
	}

	// Initialize the node in the state machine
	if err := s.stateMachine.HandleEvent(ctx, nodeName, updaterDomain.EventInitialize, data); err != nil {
		return fmt.Errorf("failed to initialize node: %w", err)
	}

	return nil
}

// monitoringLoop periodically checks CPU utilization and triggers events
func (s *UpdaterService) monitoringLoop(ctx context.Context) {
	ticker := time.NewTicker(s.config.MonitoringInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.updateAllNodeMetrics(ctx)
		}
	}
}

// cooldownLoop periodically updates cooldown status
func (s *UpdaterService) cooldownLoop(ctx context.Context) {
	ticker := time.NewTicker(s.config.CooldownUpdateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.updateAllCooldowns(ctx)
		}
	}
}

// updateAllNodeMetrics updates CPU metrics for all nodes
func (s *UpdaterService) updateAllNodeMetrics(ctx context.Context) {
	s.nodesMutex.RLock()
	nodesToUpdate := make([]string, len(s.nodes))
	copy(nodesToUpdate, s.nodes)
	s.nodesMutex.RUnlock()

	for _, nodeName := range nodesToUpdate {
		// Get current state
		state, err := s.stateMachine.GetCurrentState(ctx, nodeName)
		if err != nil {
			log.Printf("Failed to get state for node %s: %v", nodeName, err)
			continue
		}

		// Only update metrics for nodes in Monitoring state
		if state != updaterDomain.StateMonitoring {
			continue
		}

		// Get current CPU utilization
		currentCPU, windowAvg, err := s.GetNodeCPUUtilization(ctx, nodeName)
		if err != nil {
			log.Printf("Failed to get CPU utilization for %s: %v", nodeName, err)
			continue
		}

		// Prepare event data
		data := map[string]interface{}{
			"cpuUtilization":           currentCPU,
			"windowAverageUtilization": windowAvg,
		}

		// Check if threshold exceeded
		if windowAvg > s.config.ScaleThreshold {
			// Trigger threshold exceeded event
			if err := s.stateMachine.HandleEvent(ctx, nodeName, updaterDomain.EventThresholdExceeded, data); err != nil {
				log.Printf("Failed to handle threshold exceeded event: %v", err)
			}
		} else {
			// Just update the metrics
			if err := s.stateMachine.HandleEvent(ctx, nodeName, updaterDomain.EventInitialize, data); err != nil {
				log.Printf("Failed to update metrics: %v", err)
			}
		}
	}
}

// updateAllCooldowns updates cooldown status for all nodes
func (s *UpdaterService) updateAllCooldowns(ctx context.Context) {
	s.nodesMutex.RLock()
	nodesToUpdate := make([]string, len(s.nodes))
	copy(nodesToUpdate, s.nodes)
	s.nodesMutex.RUnlock()

	for _, nodeName := range nodesToUpdate {
		// Get current state
		state, err := s.stateMachine.GetCurrentState(ctx, nodeName)
		if err != nil {
			log.Printf("Failed to get state for node %s: %v", nodeName, err)
			continue
		}

		// Only update for nodes in cooldown or pending states
		if state != updaterDomain.StateCoolDown && state != updaterDomain.StatePendingVmSpecUp {
			continue
		}

		// Get current status
		status, err := s.stateMachine.GetStatus(ctx, nodeName)
		if err != nil {
			log.Printf("Failed to get status for node %s: %v", nodeName, err)
			continue
		}

		// Check if cooldown has ended
		if !status.CoolDownEndTime.IsZero() && time.Now().After(status.CoolDownEndTime) {
			// Get current CPU utilization for the transition
			currentCPU, windowAvg, err := s.GetNodeCPUUtilization(ctx, nodeName)
			if err != nil {
				log.Printf("Failed to get CPU utilization for %s: %v", nodeName, err)
				// Continue with zero values
			}

			// Trigger cooldown ended event
			data := map[string]interface{}{
				"cpuUtilization":           currentCPU,
				"windowAverageUtilization": windowAvg,
			}

			if err := s.stateMachine.HandleEvent(ctx, nodeName, updaterDomain.EventCooldownEnded, data); err != nil {
				log.Printf("Failed to handle cooldown ended event: %v", err)
			}
		} else {
			// Update the cooldown message with remaining time
			remaining := time.Until(status.CoolDownEndTime)
			if remaining < 0 {
				remaining = 0
			}

			data := map[string]interface{}{
				"message": fmt.Sprintf("In cooldown period, %.1f minutes remaining",
					remaining.Minutes()),
			}

			// Just update the message
			if err := s.stateMachine.HandleEvent(ctx, nodeName, updaterDomain.EventInitialize, data); err != nil {
				log.Printf("Failed to update cooldown message: %v", err)
			}
		}
	}
}
