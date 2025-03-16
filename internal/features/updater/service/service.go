package service

import (
	"context"
	"errors"
	"fmt"
	"k2p-updater/cmd/app"
	domainMetric "k2p-updater/internal/features/metric/domain"
	domainUpdater "k2p-updater/internal/features/updater/domain"
	"k2p-updater/pkg/resource"
	"log"
	"strings"
	"sync"
	"time"

	"k8s.io/client-go/kubernetes"
)

// UpdaterService implements the updater.Provider interface
type UpdaterService struct {
	config           domainUpdater.UpdaterConfig
	stateMachine     domainUpdater.StateMachine
	metricsService   domainMetric.Provider
	backendClient    domainUpdater.BackendClient
	healthVerifier   domainUpdater.HealthVerifier
	resourceFactory  *resource.Factory
	nodeEventService *NodeEventService

	// Additional required fields
	coordinationManager *CoordinationManager
	nodeDiscoverer      *NodeDiscoverer
	kubeClient          kubernetes.Interface
	recoveryManager     *RecoveryManager
	metricsComponent    MetricsComponent

	// Control structures
	startOnce  sync.Once
	stopChan   chan struct{}
	nodes      []string
	nodesMutex sync.RWMutex
	cancelFunc context.CancelFunc
}

// NewUpdaterService creates a new updater service
func NewUpdaterService(
	config *app.UpdaterConfig,
	metricsService domainMetric.Provider,
	backendClient domainUpdater.BackendClient,
	healthVerifier domainUpdater.HealthVerifier,
	resourceFactory *resource.Factory,
	kubeClient kubernetes.Interface,
) domainUpdater.Provider {
	// Convert app config to domain config
	domainConfig := domainUpdater.UpdaterConfig{
		ScaleThreshold:         config.ScaleThreshold,
		ScaleUpStep:            config.ScaleUpStep,
		CooldownPeriod:         config.CooldownPeriod,
		MonitoringInterval:     time.Minute, // Default to 1 minute
		CooldownUpdateInterval: time.Minute, // Default to 1 minute
	}

	// Create metrics component
	metricsConfig := &app.MetricsConfig{
		WindowSize:     10 * time.Minute, // Default values, should come from config
		SlidingSize:    1 * time.Minute,
		CooldownPeriod: config.CooldownPeriod,
		ScaleTrigger:   config.ScaleThreshold,
	}

	// Create state machine
	stateMachine := NewStateMachine(resourceFactory)

	// Initialize metrics component
	metricsComponent := NewMetricsComponent(
		metricsService,
		stateMachine,
		metricsConfig,
	)

	// Create the updater service
	service := &UpdaterService{
		config:           domainConfig,
		stateMachine:     stateMachine,
		metricsService:   metricsService,
		backendClient:    backendClient,
		healthVerifier:   healthVerifier,
		resourceFactory:  resourceFactory,
		kubeClient:       kubeClient,
		metricsComponent: metricsComponent,
		stopChan:         make(chan struct{}),
		nodes:            []string{},
	}

	// Initialize the node event service
	service.nodeEventService = NewNodeEventService(
		stateMachine,
		metricsComponent,
		resourceFactory,
	)

	return service
}

// Start begins the updater service processing
func (s *UpdaterService) Start(ctx context.Context) error {
	var startErr error

	s.startOnce.Do(func() {
		// Check for context cancellation at the beginning
		if ctx.Err() != nil {
			startErr = ctx.Err()
			return
		}

		// After node initialization
		log.Println("Verifying resource configurations...")

		// Add startup event
		startupEvent := s.resourceFactory.Event().NormalRecordWithNode(
			ctx,
			domainUpdater.UpdateKey,
			domainUpdater.ResourceName,
			string(domainUpdater.StatePendingVmSpecUp),
			"K2P-Updater service starting with %d control plane nodes configured",
			len(s.nodes),
		)

		if startupEvent != nil {
			log.Printf("Warning: Failed to record application startup event: %v", startupEvent)
		} else {
			log.Printf("Successfully recorded application startup event")
		}

		// After node initialization
		log.Println("Verifying resource configurations...")

		// Existing code continues...
		// Get the list of nodes to monitor
		if err := s.discoverNodes(ctx); err != nil {
			if errors.Is(err, context.Canceled) {
				startErr = ctx.Err()
				return
			}
			startErr = fmt.Errorf("failed to discover nodes: %w", err)
			return
		}

		// Emit another event after node discovery to report actual node count
		if len(s.nodes) > 0 {
			nodesDiscoveredEvent := s.resourceFactory.Event().NormalRecordWithNode(
				ctx,
				domainUpdater.UpdateKey,
				domainUpdater.ResourceName,
				string(domainUpdater.StatePendingVmSpecUp),
				"Discovered %d control plane nodes: %s",
				len(s.nodes),
				strings.Join(s.nodes, ", "),
			)

			if nodesDiscoveredEvent != nil {
				log.Printf("Warning: Failed to record nodes discovered event: %v", nodesDiscoveredEvent)
			} else {
				log.Printf("Successfully recorded nodes discovered event")
			}
		}

		// Get the list of nodes to monitor
		if err := s.discoverNodes(ctx); err != nil {
			if errors.Is(err, context.Canceled) {
				startErr = ctx.Err()
				return
			}
			startErr = fmt.Errorf("failed to discover nodes: %w", err)
			return
		}

		// Check context again before proceeding
		if ctx.Err() != nil {
			startErr = ctx.Err()
			return
		}

		// Update the node event service with discovered nodes
		s.nodeEventService.UpdateNodes(s.nodes)

		// Create a new independent background context that won't be affected by the parent
		// This ensures our background processes continue running
		serviceCtx, cancel := context.WithCancel(context.Background())
		s.cancelFunc = cancel

		// Initialize all nodes before starting background services
		for _, nodeName := range s.nodes {
			if err := s.initializeNode(serviceCtx, nodeName); err != nil {
				log.Printf("Failed to initialize node %s: %v", nodeName, err)
				// Continue with other nodes
			}
		}

		// Start the node event service with the service context
		if err := s.nodeEventService.Start(serviceCtx); err != nil {
			log.Printf("Warning: Failed to start node event service: %v", err)
		}

		// Start background monitoring with explicit logging
		log.Println("Starting background monitoring loops...")

		// Launch monitoring loops with the service context
		go s.monitoringLoop(serviceCtx)
		go s.cooldownLoop(serviceCtx)
		go s.recoveryLoop(serviceCtx)

		log.Println("All background monitoring loops initialized")
	})

	return startErr
}

func (s *UpdaterService) Stop() {
	log.Println("Stopping updater service...")

	// Cancel the service context if it exists
	if s.cancelFunc != nil {
		s.cancelFunc()
	}

	// Close the stop channel to signal any goroutines using it
	close(s.stopChan)

	// Wait a brief moment for goroutines to terminate
	time.Sleep(100 * time.Millisecond)

	log.Println("Updater service stopped")
}

// GetStateMachine returns the state machine instance
func (s *UpdaterService) GetStateMachine() domainUpdater.StateMachine {
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
	if status.CurrentState != domainUpdater.StateInProgressVmSpecUp &&
		status.CurrentState != domainUpdater.StateMonitoring {
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
		if err2 := s.stateMachine.HandleEvent(ctx, nodeName, domainUpdater.EventSpecUpFailed, data); err2 != nil {
			log.Printf("Failed to handle spec up failure event: %v", err2)
		}
		return fmt.Errorf("backend request failed: %w", err)
	}

	if !success {
		// Trigger failure event
		data := map[string]interface{}{
			"error": "Backend returned unsuccessful response",
		}
		if err := s.stateMachine.HandleEvent(ctx, nodeName, domainUpdater.EventSpecUpFailed, data); err != nil {
			log.Printf("Failed to handle spec up failure event: %v", err)
		}
		return fmt.Errorf("backend returned unsuccessful response")
	}

	// Trigger success event
	data := map[string]interface{}{
		"cpuUtilization": currentCPU,
	}
	if err := s.stateMachine.HandleEvent(ctx, nodeName, domainUpdater.EventSpecUpRequested, data); err != nil {
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

	// Validate that we're in an appropriate state to check health
	if status.CurrentState != domainUpdater.StateInProgressVmSpecUp {
		return false, fmt.Errorf("cannot verify health for node %s in %s state",
			nodeName, status.CurrentState)
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
		if err := s.stateMachine.HandleEvent(ctx, nodeName, domainUpdater.EventHealthCheckPassed, data); err != nil {
			log.Printf("Failed to handle health check passed event: %v", err)
		}
	} else {
		if err := s.stateMachine.HandleEvent(ctx, nodeName, domainUpdater.EventHealthCheckFailed, data); err != nil {
			log.Printf("Failed to handle health check failed event: %v", err)
		}
	}

	return healthy, nil
}

// GetNodeCPUUtilization gets the current CPU utilization for a node
func (s *UpdaterService) GetNodeCPUUtilization(ctx context.Context, nodeName string) (float64, float64, error) {
	return s.metricsComponent.GetNodeCPUMetrics(nodeName)
}

// IsCooldownActive checks if a node is in cooldown period
func (s *UpdaterService) IsCooldownActive(ctx context.Context, nodeName string) (bool, time.Duration, error) {
	// Get current node status
	status, err := s.stateMachine.GetStatus(ctx, nodeName)
	if err != nil {
		return false, 0, fmt.Errorf("failed to get node status: %w", err)
	}

	// Check if in cooldown state or pending state
	inCooldown := status.CurrentState == domainUpdater.StateCoolDown ||
		status.CurrentState == domainUpdater.StatePendingVmSpecUp

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

// discoverNodes discovers control plane nodes to monitor
func (s *UpdaterService) discoverNodes(ctx context.Context) error {
	s.nodesMutex.Lock()
	defer s.nodesMutex.Unlock()

	// Create node discoverer if not already available
	if s.nodeDiscoverer == nil {
		s.nodeDiscoverer = NewNodeDiscoverer(s.kubeClient, s.config.Namespace)
	}

	// Discover control plane nodes
	discoveredNodes, err := s.nodeDiscoverer.DiscoverControlPlaneNodes(ctx)
	if err != nil {
		return fmt.Errorf("failed to discover control plane nodes: %w", err)
	}

	if len(discoveredNodes) == 0 {
		log.Printf("Warning: No control plane nodes found. Using nodes from configuration if available.")
		// You could add a fallback mechanism here to use nodes defined in configuration
		// This would require adding a list of node names to your configuration
	}

	// Update the nodes list
	s.nodes = discoveredNodes
	log.Printf("Updated nodes list with %d control plane nodes", len(s.nodes))

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
		// Continue with zero values but ensure metrics service knows about this node

		// This is the key addition - register the node in metrics service if it doesn't exist
		if s.metricsService != nil {
			// Check if this is a "node not found" error which indicates the metrics service
			// doesn't know about this node yet
			if strings.Contains(err.Error(), "node not found") {
				// Add node to metrics service if it implements the NodeRegistration interface
				if registrar, ok := s.metricsService.(domainMetric.NodeRegistration); ok {
					if registerErr := registrar.RegisterNode(ctx, nodeName); registerErr != nil {
						log.Printf("Failed to register node %s in metrics service: %v", nodeName, registerErr)
					} else {
						log.Printf("Successfully registered node %s in metrics service", nodeName)
					}
				}
			}
		}
	}

	// Create initial status with CoolDown state explicitly
	initialStatus := &domainUpdater.ControlPlaneStatus{
		NodeName:                 nodeName,
		CurrentState:             domainUpdater.StateCoolDown,
		LastTransitionTime:       time.Now(),
		Message:                  fmt.Sprintf("Initial startup cooldown period for %.1f minutes", s.config.CooldownPeriod.Minutes()),
		CPUUtilization:           currentCPU,
		WindowAverageUtilization: windowAvg,
		CoolDownEndTime:          cooldownEnd,
	}

	// First update the status directly to ensure it starts in the CoolDown state
	if err := s.stateMachine.UpdateStatus(ctx, nodeName, initialStatus); err != nil {
		log.Printf("Warning: Failed to directly set initial state to CoolDown for %s: %v", nodeName, err)
	}

	// Then trigger the initialize event to handle any additional setup
	data := map[string]interface{}{
		"cpuUtilization":           currentCPU,
		"windowAverageUtilization": windowAvg,
		"coolDownEndTime":          cooldownEnd,
		"initialState":             domainUpdater.StateCoolDown,
	}

	// Initialize the node in the state machine
	if err := s.stateMachine.HandleEvent(ctx, nodeName, domainUpdater.EventInitialize, data); err != nil {
		return fmt.Errorf("failed to initialize node: %w", err)
	}

	log.Printf("Successfully initialized node %s in CoolDown state until %s",
		nodeName, cooldownEnd.Format(time.RFC3339))

	return nil
}

// monitoringLoop periodically checks CPU utilization and triggers events
func (s *UpdaterService) monitoringLoop(ctx context.Context) {
	log.Println("Starting monitoring loop execution")

	monitoringTicker := time.NewTicker(s.config.MonitoringInterval)
	statusUpdateTicker := time.NewTicker(1 * time.Minute)
	defer monitoringTicker.Stop()
	defer statusUpdateTicker.Stop()

	log.Println("Monitoring loop initialized and waiting for first tick")

	for {
		select {
		case <-ctx.Done():
			log.Println("Monitoring loop context canceled")
			return
		case <-monitoringTicker.C:
			// Skip operations if context is done
			if ctx.Err() != nil {
				continue
			}

			log.Println("Monitoring ticker triggered, updating node metrics")
			s.updateAllNodeMetrics(ctx)

		case <-statusUpdateTicker.C:
			// Skip operations if context is done
			if ctx.Err() != nil {
				continue
			}

			log.Println("Status update ticker triggered, updating node status")
			s.updateAllNodeStatus(ctx)
		}
	}
}

// cooldownLoop periodically updates cooldown status
func (s *UpdaterService) cooldownLoop(ctx context.Context) {
	log.Println("Starting cooldown loop")

	// Change to 1 minute interval to align with our CR update frequency
	cooldownTicker := time.NewTicker(1 * time.Minute)
	defer cooldownTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Cooldown loop context canceled")
			return
		case <-s.stopChan:
			log.Println("Cooldown loop received stop signal")
			return
		case <-cooldownTicker.C:
			// Check context before performing operations
			if ctx.Err() != nil {
				log.Println("Skipping cooldown update due to canceled context")
				return
			}

			s.updateAllCooldowns(ctx)
		}
	}
}

// updateAllNodeStatus updates the status messages for all nodes
func (s *UpdaterService) updateAllNodeStatus(ctx context.Context) {
	// Check context cancellation at the beginning
	if ctx.Err() != nil {
		log.Printf("Skipping node status update due to context cancellation: %v", ctx.Err())
		return
	}

	log.Printf("Starting periodic status update for all nodes...")
	s.nodesMutex.RLock()
	nodesToUpdate := make([]string, len(s.nodes))
	copy(nodesToUpdate, s.nodes)
	s.nodesMutex.RUnlock()

	for _, nodeName := range nodesToUpdate {
		// Check for context cancellation during iteration
		if ctx.Err() != nil {
			log.Printf("Context canceled during node status update: %v", ctx.Err())
			return
		}

		// Get current state
		state, err := s.stateMachine.GetCurrentState(ctx, nodeName)
		if err != nil {
			log.Printf("Failed to get state for node %s: %v", nodeName, err)
			continue
		}

		// Get current CPU metrics
		currentCPU, windowAvg, err := s.GetNodeCPUUtilization(ctx, nodeName)
		if err != nil {
			log.Printf("Failed to get CPU metrics for status update: %v", err)
			// Try to continue with the current values from status
			status, statErr := s.stateMachine.GetStatus(ctx, nodeName)
			if statErr == nil {
				currentCPU = status.CPUUtilization
				windowAvg = status.WindowAverageUtilization
			} else {
				log.Printf("Could not get current status: %v", statErr)
				continue
			}
		}

		// Add current time for timestamp
		data := map[string]interface{}{
			"cpuUtilization":           currentCPU,
			"windowAverageUtilization": windowAvg,
			"lastUpdateTime":           time.Now(),
		}

		// Update status based on current state
		switch state {
		case domainUpdater.StateMonitoring:
			log.Printf("Updating monitoring status for node %s", nodeName)
			if ctx.Err() != nil {
				log.Printf("Context canceled before updating monitoring status: %v", ctx.Err())
				return
			}
			if err := s.stateMachine.HandleEvent(ctx, nodeName, domainUpdater.EventMonitoringStatus, data); err != nil {
				log.Printf("Failed to update monitoring status: %v", err)
			}
		case domainUpdater.StateCoolDown:
			log.Printf("Updating cooldown status for node %s", nodeName)
			if ctx.Err() != nil {
				log.Printf("Context canceled before updating cooldown status: %v", ctx.Err())
				return
			}
			if err := s.stateMachine.HandleEvent(ctx, nodeName, domainUpdater.EventCooldownStatus, data); err != nil {
				log.Printf("Failed to update cooldown status: %v", err)
			}
		default:
			log.Printf("Node %s is in %s state, no status update needed", nodeName, state)
		}
	}
	log.Printf("Finished periodic status update for all nodes")
}

// updateAllCooldowns updates cooldown status for all nodes
func (s *UpdaterService) updateAllNodeMetrics(ctx context.Context) {
	// Check context cancellation at the beginning
	if ctx.Err() != nil {
		log.Printf("Skipping node metrics update due to context cancellation: %v", ctx.Err())
		return
	}

	s.nodesMutex.RLock()
	nodesToUpdate := make([]string, len(s.nodes))
	copy(nodesToUpdate, s.nodes)
	s.nodesMutex.RUnlock()

	if s.coordinationManager == nil {
		s.coordinationManager = NewCoordinationManager(s.stateMachine)
	}

	// Check if any node is currently being spec'd up
	isSpecingUp, specingUpNode, err := s.coordinationManager.IsAnyNodeSpecingUp(ctx, nodesToUpdate)
	if err != nil {
		if ctx.Err() != nil {
			log.Printf("Context canceled during specUp check: %v", ctx.Err())
			return
		}
		log.Printf("Failed to check ongoing spec up: %v", err)
	}

	for _, nodeName := range nodesToUpdate {
		// Check for context cancellation during iteration
		if ctx.Err() != nil {
			log.Printf("Context canceled during node metrics update: %v", ctx.Err())
			return
		}

		// Get current state
		state, err := s.stateMachine.GetCurrentState(ctx, nodeName)
		if err != nil {
			log.Printf("Failed to get state for node %s: %v", nodeName, err)
			continue
		}

		// For nodes in monitoring state, check CPU threshold
		if state == domainUpdater.StateMonitoring {
			thresholdExceeded, currentCPU, windowAvg, err := s.metricsComponent.CheckCPUThresholdExceeded(ctx, nodeName)
			if err != nil {
				log.Printf("Failed to check CPU threshold for node %s: %v", nodeName, err)
				continue
			}

			// Prepare event data
			data := map[string]interface{}{
				"cpuUtilization":           currentCPU,
				"windowAverageUtilization": windowAvg,
			}

			// If threshold exceeded and no other node is spec'ing up, trigger an event
			if thresholdExceeded && !isSpecingUp {
				if ctx.Err() != nil {
					log.Printf("Context canceled before handling threshold event: %v", ctx.Err())
					return
				}
				if err := s.stateMachine.HandleEvent(ctx, nodeName, domainUpdater.EventThresholdExceeded, data); err != nil {
					log.Printf("Failed to handle threshold exceeded event: %v", err)
				} else {
					log.Printf("Node %s triggered for spec up (CPU: %.2f%%)", nodeName, windowAvg)
					isSpecingUp = true
					specingUpNode = nodeName
				}
			} else if thresholdExceeded {
				log.Printf("Node %s exceeded threshold (%.2f%%) but waiting for %s to complete",
					nodeName, windowAvg, specingUpNode)
				// Just update the metrics
				if ctx.Err() != nil {
					log.Printf("Context canceled before updating metrics: %v", ctx.Err())
					return
				}
				if err := s.stateMachine.HandleEvent(ctx, nodeName, domainUpdater.EventInitialize, data); err != nil {
					log.Printf("Failed to update metrics: %v", err)
				}
			} else {
				// Just update the metrics
				if ctx.Err() != nil {
					log.Printf("Context canceled before updating metrics: %v", ctx.Err())
					return
				}
				if err := s.stateMachine.HandleEvent(ctx, nodeName, domainUpdater.EventInitialize, data); err != nil {
					log.Printf("Failed to update metrics: %v", err)
				}
			}
		} else if state == domainUpdater.StatePendingVmSpecUp || state == domainUpdater.StateCoolDown {
			// For nodes in pending or cooldown state, just update their metrics
			currentCPU, windowAvg, _ := s.metricsComponent.GetNodeCPUMetrics(nodeName)

			data := map[string]interface{}{
				"cpuUtilization":           currentCPU,
				"windowAverageUtilization": windowAvg,
			}

			if ctx.Err() != nil {
				log.Printf("Context canceled before updating metrics: %v", ctx.Err())
				return
			}
			if err := s.stateMachine.HandleEvent(ctx, nodeName, domainUpdater.EventInitialize, data); err != nil {
				log.Printf("Failed to update metrics: %v", err)
			}
		}
	}
}

// recoveryLoop periodically checks for nodes in failed state and attempts recovery
func (s *UpdaterService) recoveryLoop(ctx context.Context) {
	log.Println("Starting recovery loop")

	// Create recovery manager if not already available
	if s.recoveryManager == nil {
		s.recoveryManager = NewRecoveryManager(s.stateMachine)
	}

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Recovery loop context canceled")
			return
		case <-s.stopChan:
			log.Println("Recovery loop received stop signal")
			return
		case <-ticker.C:
			// Check context before performing operations
			if ctx.Err() != nil {
				log.Println("Skipping recovery check due to canceled context")
				return
			}

			s.nodesMutex.RLock()
			nodesToCheck := make([]string, len(s.nodes))
			copy(nodesToCheck, s.nodes)
			s.nodesMutex.RUnlock()

			s.recoveryManager.CheckAllNodes(ctx, nodesToCheck)
		}
	}
}

func (s *UpdaterService) updateAllCooldowns(ctx context.Context) {
	// Check context cancellation at the beginning
	if ctx.Err() != nil {
		log.Printf("Skipping cooldown update due to context cancellation: %v", ctx.Err())
		return
	}

	log.Printf("Starting cooldown update check for all nodes")

	s.nodesMutex.RLock()
	nodesToUpdate := make([]string, len(s.nodes))
	copy(nodesToUpdate, s.nodes)
	s.nodesMutex.RUnlock()

	// Track if we've already processed a node to avoid multiple updates
	processedNodes := make(map[string]bool)

	for _, nodeName := range nodesToUpdate {
		// Check for context cancellation during iteration
		if ctx.Err() != nil {
			log.Printf("Context canceled during cooldown update: %v", ctx.Err())
			return
		}

		// Skip if already processed in this cycle
		if processedNodes[nodeName] {
			log.Printf("Node %s already processed in this cooldown cycle, skipping", nodeName)
			continue
		}

		processedNodes[nodeName] = true

		// Get current state
		state, err := s.stateMachine.GetCurrentState(ctx, nodeName)
		if err != nil {
			log.Printf("Failed to get state for node %s: %v", nodeName, err)
			continue
		}

		// Only update for nodes in cooldown state
		if state != domainUpdater.StateCoolDown {
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
			log.Printf("Cooldown period ended for node %s", nodeName)

			// Get current CPU utilization for the transition
			currentCPU, windowAvg, err := s.GetNodeCPUUtilization(ctx, nodeName)
			if err != nil {
				log.Printf("Failed to get CPU utilization for %s: %v", nodeName, err)
				currentCPU = status.CPUUtilization
				windowAvg = status.WindowAverageUtilization
			}

			// Trigger cooldown ended event
			data := map[string]interface{}{
				"cpuUtilization":           currentCPU,
				"windowAverageUtilization": windowAvg,
			}

			if ctx.Err() != nil {
				log.Printf("Context canceled before handling cooldown ended event: %v", ctx.Err())
				return
			}
			if err := s.stateMachine.HandleEvent(ctx, nodeName, domainUpdater.EventCooldownEnded, data); err != nil {
				log.Printf("Failed to handle cooldown ended event: %v", err)
			} else {
				// Create a single event recording the transition
				if ctx.Err() != nil {
					log.Printf("Context canceled before recording event: %v", ctx.Err())
					return
				}
				s.resourceFactory.Event().NormalRecordWithNode(
					ctx,
					domainUpdater.UpdateKey,
					nodeName,
					string(domainUpdater.StatePendingVmSpecUp),
					"Node %s cooldown period ended, transitioning to monitoring state",
					nodeName,
				)
			}
		} else {
			// Only update message occasionally, not every cycle
			// This helps reduce redundant status updates
			timeRemaining := time.Until(status.CoolDownEndTime)
			if timeRemaining < 0 {
				timeRemaining = 0
			}

			// Only update if more than 30 seconds have passed since last update
			if time.Since(status.LastTransitionTime) > 30*time.Second {
				data := map[string]interface{}{
					"cpuUtilization":           status.CPUUtilization,
					"windowAverageUtilization": status.WindowAverageUtilization,
				}

				if ctx.Err() != nil {
					log.Printf("Context canceled before updating cooldown message: %v", ctx.Err())
					return
				}
				if err := s.stateMachine.HandleEvent(ctx, nodeName, domainUpdater.EventCooldownStatus, data); err != nil {
					log.Printf("Failed to update cooldown message: %v", err)
				}
			}
		}
	}

	log.Printf("Completed cooldown update check for all nodes")
}
