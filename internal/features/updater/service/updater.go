package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"k2p-updater/internal/features/updater/domain/interfaces"
	"k2p-updater/internal/features/updater/domain/models"
)

// UpdaterService implements the interfaces.Provider interface
type UpdaterService struct {
	config           models.UpdaterConfig
	stateMachine     interfaces.StateMachine
	backendClient    interfaces.BackendClient
	healthVerifier   interfaces.HealthVerifier
	metricsComponent interfaces.MetricsProvider
	nodeDiscoverer   interfaces.NodeDiscoverer
	coordinator      interfaces.Coordinator
	recoveryManager  interfaces.RecoveryManager
	resourceFactory  interfaces.ResourceFactory

	// Control structures
	startOnce  sync.Once
	stopChan   chan struct{}
	nodes      []string
	nodesMutex sync.RWMutex
	cancelFunc context.CancelFunc
}

// NewUpdaterService creates a new updater service with dependencies injected
func NewUpdaterService(
	config models.UpdaterConfig,
	stateMachine interfaces.StateMachine,
	backendClient interfaces.BackendClient,
	healthVerifier interfaces.HealthVerifier,
	metricsComponent interfaces.MetricsProvider,
	nodeDiscoverer interfaces.NodeDiscoverer,
	coordinator interfaces.Coordinator,
	recoveryManager interfaces.RecoveryManager,
	resourceFactory interfaces.ResourceFactory,
) interfaces.Provider {
	return &UpdaterService{
		config:           config,
		stateMachine:     stateMachine,
		backendClient:    backendClient,
		healthVerifier:   healthVerifier,
		metricsComponent: metricsComponent,
		nodeDiscoverer:   nodeDiscoverer,
		coordinator:      coordinator,
		recoveryManager:  recoveryManager,
		resourceFactory:  resourceFactory,
		stopChan:         make(chan struct{}),
		nodes:            []string{},
	}
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

		// Record startup event
		if err := s.recordStartupEvent(ctx); err != nil {
			log.Printf("Warning: Failed to record application startup event: %v", err)
		}

		// Discover control plane nodes
		if err := s.discoverNodes(ctx); err != nil {
			if errors.Is(err, context.Canceled) {
				startErr = ctx.Err()
				return
			}
			startErr = fmt.Errorf("failed to discover nodes: %w", err)
			return
		}

		// Record nodes discovered event
		if len(s.nodes) > 0 {
			if err := s.recordNodesDiscoveredEvent(ctx); err != nil {
				log.Printf("Warning: Failed to record nodes discovered event: %v", err)
			}
		}

		// Create a new independent background context
		serviceCtx, cancel := context.WithCancel(context.Background())
		s.cancelFunc = cancel

		// Initialize all nodes
		s.initializeAllNodes(serviceCtx)

		// Start background monitoring
		log.Println("Starting background monitoring loops...")
		go s.runMonitoringLoop(serviceCtx)
		go s.runCooldownLoop(serviceCtx)
		go s.runRecoveryLoop(serviceCtx)
		log.Println("All background monitoring loops initialized")
	})

	return startErr
}

// recordStartupEvent adds a startup event
func (s *UpdaterService) recordStartupEvent(ctx context.Context) error {
	event := s.resourceFactory.Event().NormalRecordWithNode(
		ctx,
		models.UpdateKey,
		models.ResourceName,
		string(models.StatePendingVmSpecUp),
		"K2P-Updater service starting with %d control plane nodes configured",
		len(s.nodes),
	)

	if event != nil {
		return event
	}

	log.Printf("Successfully recorded application startup event")
	return nil
}

// recordNodesDiscoveredEvent records the nodes discovery event
func (s *UpdaterService) recordNodesDiscoveredEvent(ctx context.Context) error {
	event := s.resourceFactory.Event().NormalRecordWithNode(
		ctx,
		models.UpdateKey,
		models.ResourceName,
		string(models.StatePendingVmSpecUp),
		"Discovered %d control plane nodes: %s",
		len(s.nodes),
		strings.Join(s.nodes, ", "),
	)

	if event != nil {
		return event
	}

	log.Printf("Successfully recorded nodes discovered event")
	return nil
}

// initializeAllNodes initializes all discovered nodes
func (s *UpdaterService) initializeAllNodes(ctx context.Context) {
	for _, nodeName := range s.nodes {
		if err := s.initializeNode(ctx, nodeName); err != nil {
			log.Printf("Failed to initialize node %s: %v", nodeName, err)
			// Continue with other nodes
		}
	}
}

// Stop stops the updater service
func (s *UpdaterService) Stop() {
	log.Println("Stopping updater service...")

	// Cancel the service context if it exists
	if s.cancelFunc != nil {
		s.cancelFunc()
	}

	// Close the stop channel to signal any goroutines
	close(s.stopChan)

	// Allow time for graceful shutdown
	time.Sleep(100 * time.Millisecond)

	log.Println("Updater service stopped")
}

// GetStateMachine returns the state machine instance
func (s *UpdaterService) GetStateMachine() interfaces.StateMachine {
	return s.stateMachine
}

// RequestSpecUp requests a spec up for a node
func (s *UpdaterService) RequestSpecUp(ctx context.Context, nodeName string) error {
	// Check context cancellation
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Get current node status
	status, err := s.stateMachine.GetStatus(ctx, nodeName)
	if err != nil {
		return fmt.Errorf("failed to get node status: %w", err)
	}

	// Validate state is appropriate for spec up
	if status.CurrentState != models.StateInProgressVmSpecUp &&
		status.CurrentState != models.StateMonitoring {
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
		// Handle failure
		if err := s.handleSpecUpFailure(ctx, nodeName, err.Error()); err != nil {
			log.Printf("Failed to handle spec up failure event: %v", err)
		}
		return fmt.Errorf("backend request failed: %w", err)
	}

	if !success {
		// Handle unsuccessful response
		if err := s.handleSpecUpFailure(ctx, nodeName, "Backend returned unsuccessful response"); err != nil {
			log.Printf("Failed to handle spec up failure event: %v", err)
		}
		return fmt.Errorf("backend returned unsuccessful response")
	}

	// Handle success
	return s.handleSpecUpSuccess(ctx, nodeName, currentCPU)
}

// handleSpecUpFailure handles spec up failure events
func (s *UpdaterService) handleSpecUpFailure(ctx context.Context, nodeName, errorMsg string) error {
	data := map[string]interface{}{
		"error": errorMsg,
	}
	return s.stateMachine.HandleEvent(ctx, nodeName, models.EventSpecUpFailed, data)
}

// handleSpecUpSuccess handles successful spec up requests
func (s *UpdaterService) handleSpecUpSuccess(ctx context.Context, nodeName string, currentCPU float64) error {
	data := map[string]interface{}{
		"cpuUtilization": currentCPU,
	}
	if err := s.stateMachine.HandleEvent(ctx, nodeName, models.EventSpecUpRequested, data); err != nil {
		log.Printf("Failed to handle spec up requested event: %v", err)
		return fmt.Errorf("failed to update state: %w", err)
	}
	return nil
}

// VerifySpecUpHealth verifies the health of a node after spec up
func (s *UpdaterService) VerifySpecUpHealth(ctx context.Context, nodeName string) (bool, error) {
	// Check context cancellation
	if ctx.Err() != nil {
		return false, ctx.Err()
	}

	// Get current node status
	status, err := s.stateMachine.GetStatus(ctx, nodeName)
	if err != nil {
		return false, fmt.Errorf("failed to get node status: %w", err)
	}

	// Validate state is appropriate for health check
	if status.CurrentState != models.StateInProgressVmSpecUp {
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

	eventType := models.EventHealthCheckPassed
	if !healthy {
		eventType = models.EventHealthCheckFailed
	}

	if err := s.stateMachine.HandleEvent(ctx, nodeName, eventType, data); err != nil {
		log.Printf("Failed to handle health check event: %v", err)
	}

	return healthy, nil
}

// GetNodeCPUUtilization gets the current CPU utilization for a node
func (s *UpdaterService) GetNodeCPUUtilization(ctx context.Context, nodeName string) (float64, float64, error) {
	// Check for context cancellation
	if ctx.Err() != nil {
		return 0, 0, ctx.Err()
	}

	// Call the metrics component
	currentCPU, windowAvg, err := s.metricsComponent.GetNodeCPUMetrics(nodeName)

	// Check context again in case the operation took time
	if ctx.Err() != nil {
		return 0, 0, ctx.Err()
	}

	return currentCPU, windowAvg, err
}

// IsCooldownActive checks if a node is in cooldown period
func (s *UpdaterService) IsCooldownActive(ctx context.Context, nodeName string) (bool, time.Duration, error) {
	if ctx.Err() != nil {
		return false, 0, ctx.Err()
	}

	// Get current node status
	status, err := s.stateMachine.GetStatus(ctx, nodeName)
	if err != nil {
		return false, 0, fmt.Errorf("failed to get node status: %w", err)
	}

	// Check if in cooldown state or pending state
	inCooldown := status.CurrentState == models.StateCoolDown ||
		status.CurrentState == models.StatePendingVmSpecUp

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

	// Discover control plane nodes
	discoveredNodes, err := s.nodeDiscoverer.DiscoverControlPlaneNodes(ctx)
	if err != nil {
		return fmt.Errorf("failed to discover control plane nodes: %w", err)
	}

	if len(discoveredNodes) == 0 {
		log.Printf("Warning: No control plane nodes found. Using nodes from configuration if available.")
	}

	// Update the nodes list
	s.nodes = discoveredNodes
	log.Printf("Updated nodes list with %d control plane nodes", len(s.nodes))

	return nil
}

// initializeNode initializes a node in the state machine
func (s *UpdaterService) initializeNode(ctx context.Context, nodeName string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Start in cooldown state
	cooldownEnd := time.Now().Add(s.config.CooldownPeriod)

	// Get current CPU utilization
	currentCPU, windowAvg, err := s.GetNodeCPUUtilization(ctx, nodeName)
	if err != nil {
		log.Printf("Warning: Failed to get initial CPU utilization for %s: %v", nodeName, err)
		// Continue with zero values
	}

	// Create initial status
	initialStatus := &models.ControlPlaneStatus{
		NodeName:                 nodeName,
		CurrentState:             models.StateCoolDown,
		LastTransitionTime:       time.Now(),
		Message:                  fmt.Sprintf("Initial startup cooldown period for %.1f minutes", s.config.CooldownPeriod.Minutes()),
		CPUUtilization:           currentCPU,
		WindowAverageUtilization: windowAvg,
		CoolDownEndTime:          cooldownEnd,
	}

	// Set initial state directly
	if err := s.stateMachine.UpdateStatus(ctx, nodeName, initialStatus); err != nil {
		log.Printf("Warning: Failed to directly set initial state to CoolDown for %s: %v", nodeName, err)
	}

	// Then trigger initialize event for additional setup
	data := map[string]interface{}{
		"cpuUtilization":           currentCPU,
		"windowAverageUtilization": windowAvg,
		"coolDownEndTime":          cooldownEnd,
		"initialState":             models.StateCoolDown,
	}

	if err := s.stateMachine.HandleEvent(ctx, nodeName, models.EventInitialize, data); err != nil {
		return fmt.Errorf("failed to initialize node: %w", err)
	}

	log.Printf("Successfully initialized node %s in CoolDown state until %s",
		nodeName, cooldownEnd.Format(time.RFC3339))

	return nil
}

// runMonitoringLoop runs the main monitoring loop for CPU utilization
func (s *UpdaterService) runMonitoringLoop(ctx context.Context) {
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
			if ctx.Err() == nil {
				log.Println("Monitoring ticker triggered, updating node metrics")
				s.updateAllNodeMetrics(ctx)
			}
		case <-statusUpdateTicker.C:
			if ctx.Err() == nil {
				log.Println("Status update ticker triggered, updating node status")
				s.updateAllNodeStatus(ctx)
			}
		}
	}
}

// runCooldownLoop runs the cooldown monitoring loop
func (s *UpdaterService) runCooldownLoop(ctx context.Context) {
	log.Println("Starting cooldown loop")

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
			if ctx.Err() == nil {
				s.updateAllCooldowns(ctx)
			}
		}
	}
}

// runRecoveryLoop runs the recovery monitoring loop
func (s *UpdaterService) runRecoveryLoop(ctx context.Context) {
	log.Println("Starting recovery loop")

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
			if ctx.Err() == nil {
				s.nodesMutex.RLock()
				nodesToCheck := make([]string, len(s.nodes))
				copy(nodesToCheck, s.nodes)
				s.nodesMutex.RUnlock()

				s.recoveryManager.CheckAllNodes(ctx, nodesToCheck)
			}
		}
	}
}

// updateAllNodeStatus updates the status messages for all nodes
func (s *UpdaterService) updateAllNodeStatus(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}

	log.Printf("Starting periodic status update for all nodes...")
	s.nodesMutex.RLock()
	nodesToUpdate := make([]string, len(s.nodes))
	copy(nodesToUpdate, s.nodes)
	s.nodesMutex.RUnlock()

	for _, nodeName := range nodesToUpdate {
		if ctx.Err() != nil {
			log.Printf("Context canceled during node status update")
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

		// Prepare data for update
		data := map[string]interface{}{
			"cpuUtilization":           currentCPU,
			"windowAverageUtilization": windowAvg,
			"lastUpdateTime":           time.Now(),
		}

		// Update status based on current state
		var eventType models.Event
		switch state {
		case models.StateMonitoring:
			eventType = models.EventMonitoringStatus
			log.Printf("Updating monitoring status for node %s", nodeName)
		case models.StateCoolDown:
			eventType = models.EventCooldownStatus
			log.Printf("Updating cooldown status for node %s", nodeName)
		default:
			log.Printf("Node %s is in %s state, no status update needed", nodeName, state)
			continue
		}

		if ctx.Err() != nil {
			return
		}

		if err := s.stateMachine.HandleEvent(ctx, nodeName, eventType, data); err != nil {
			log.Printf("Failed to update status: %v", err)
		}
	}
	log.Printf("Finished periodic status update for all nodes")
}

// updateAllNodeMetrics updates metrics for all nodes and checks for threshold exceedance
func (s *UpdaterService) updateAllNodeMetrics(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}

	s.nodesMutex.RLock()
	nodesToUpdate := make([]string, len(s.nodes))
	copy(nodesToUpdate, s.nodes)
	s.nodesMutex.RUnlock()

	// Get information about nodes currently spec'ing up
	specingUp, specingUpNode, err := s.coordinator.IsAnyNodeSpecingUp(ctx, nodesToUpdate)
	if err != nil && ctx.Err() == nil {
		log.Printf("Failed to check ongoing spec up: %v", err)
	}

	// Process each node with the consistent information
	for _, nodeName := range nodesToUpdate {
		if ctx.Err() != nil {
			return
		}

		// Get current state
		state, err := s.stateMachine.GetCurrentState(ctx, nodeName)
		if err != nil {
			log.Printf("Failed to get state for node %s: %v", nodeName, err)
			continue
		}

		// For nodes in monitoring state, check CPU threshold
		if state == models.StateMonitoring {
			s.checkNodeThreshold(ctx, nodeName, specingUp, specingUpNode, nodesToUpdate)
		} else if state == models.StatePendingVmSpecUp || state == models.StateCoolDown {
			// For other states, just update metrics
			s.updateNodeMetricsOnly(ctx, nodeName)
		}
	}
}

// checkNodeThreshold checks if a node's CPU threshold is exceeded and handles accordingly
func (s *UpdaterService) checkNodeThreshold(ctx context.Context, nodeName string,
	specingUp bool, specingUpNode string, allNodes []string) {

	if ctx.Err() != nil {
		return
	}

	thresholdExceeded, currentCPU, windowAvg, err := s.metricsComponent.CheckCPUThresholdExceeded(ctx, nodeName)
	if err != nil {
		log.Printf("Failed to check CPU threshold for node %s: %v", nodeName, err)
		return
	}

	// Prepare data for updates
	data := map[string]interface{}{
		"cpuUtilization":           currentCPU,
		"windowAverageUtilization": windowAvg,
	}

	if thresholdExceeded && !specingUp {
		if ctx.Err() != nil {
			return
		}

		// Double check if any node started a spec-up since our earlier check
		currentSpecingUp, currentSpecingNode, checkErr := s.coordinator.IsAnyNodeSpecingUp(ctx, allNodes)
		if checkErr != nil {
			log.Printf("Failed to perform final spec-up check: %v", checkErr)
			return
		}

		if !currentSpecingUp {
			// No node spec'ing up, we can proceed
			if err := s.stateMachine.HandleEvent(ctx, nodeName, models.EventThresholdExceeded, data); err != nil {
				log.Printf("Failed to handle threshold exceeded event: %v", err)
			} else {
				log.Printf("Node %s triggered for spec up (CPU: %.2f%%)", nodeName, windowAvg)
			}
		} else {
			log.Printf("Node %s threshold exceeded but another node (%s) started spec-up during processing",
				nodeName, currentSpecingNode)
		}
	} else if thresholdExceeded {
		log.Printf("Node %s exceeded threshold (%.2f%%) but waiting for %s to complete",
			nodeName, windowAvg, specingUpNode)
		s.updateNodeData(ctx, nodeName, data)
	} else {
		// Just update the metrics
		s.updateNodeData(ctx, nodeName, data)
	}
}

// updateNodeMetricsOnly updates only the node metrics without threshold checks
func (s *UpdaterService) updateNodeMetricsOnly(ctx context.Context, nodeName string) {
	if ctx.Err() != nil {
		return
	}

	currentCPU, windowAvg, _ := s.metricsComponent.GetNodeCPUMetrics(nodeName)
	data := map[string]interface{}{
		"cpuUtilization":           currentCPU,
		"windowAverageUtilization": windowAvg,
	}

	s.updateNodeData(ctx, nodeName, data)
}

// updateNodeData handles updating node data with error handling
func (s *UpdaterService) updateNodeData(ctx context.Context, nodeName string, data map[string]interface{}) {
	if ctx.Err() != nil {
		return
	}

	if err := s.stateMachine.HandleEvent(ctx, nodeName, models.EventInitialize, data); err != nil {
		log.Printf("Failed to update metrics for node %s: %v", nodeName, err)
	}
}

// updateAllCooldowns updates cooldown status for all nodes
func (s *UpdaterService) updateAllCooldowns(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}

	log.Printf("Starting cooldown update check for all nodes")

	s.nodesMutex.RLock()
	nodesToUpdate := make([]string, len(s.nodes))
	copy(nodesToUpdate, s.nodes)
	s.nodesMutex.RUnlock()

	// Track processed nodes to avoid duplicates
	processedNodes := make(map[string]bool)

	for _, nodeName := range nodesToUpdate {
		if ctx.Err() != nil {
			return
		}

		// Skip if already processed
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
		if state != models.StateCoolDown {
			continue
		}

		s.processCooldownNode(ctx, nodeName)
	}

	log.Printf("Completed cooldown update check for all nodes")
}

// processCooldownNode processes a single node in cooldown state
func (s *UpdaterService) processCooldownNode(ctx context.Context, nodeName string) {
	if ctx.Err() != nil {
		return
	}

	// Get current status
	status, err := s.stateMachine.GetStatus(ctx, nodeName)
	if err != nil {
		log.Printf("Failed to get status for node %s: %v", nodeName, err)
		return
	}

	// Check if cooldown has ended
	if !status.CoolDownEndTime.IsZero() && time.Now().After(status.CoolDownEndTime) {
		s.handleCooldownEnded(ctx, nodeName, status)
	} else {
		// Only update message if significant time has passed
		if time.Since(status.LastTransitionTime) > 30*time.Second {
			data := map[string]interface{}{
				"cpuUtilization":           status.CPUUtilization,
				"windowAverageUtilization": status.WindowAverageUtilization,
			}

			if ctx.Err() != nil {
				return
			}

			if err := s.stateMachine.HandleEvent(ctx, nodeName, models.EventCooldownStatus, data); err != nil {
				log.Printf("Failed to update cooldown message: %v", err)
			}
		}
	}
}

// handleCooldownEnded handles when a node's cooldown period has ended
func (s *UpdaterService) handleCooldownEnded(ctx context.Context, nodeName string, status *models.ControlPlaneStatus) {
	if ctx.Err() != nil {
		return
	}

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
		return
	}

	if err := s.stateMachine.HandleEvent(ctx, nodeName, models.EventCooldownEnded, data); err != nil {
		log.Printf("Failed to handle cooldown ended event: %v", err)
	} else {
		// Record the transition event
		if ctx.Err() != nil {
			return
		}

		s.resourceFactory.Event().NormalRecordWithNode(
			ctx,
			models.UpdateKey,
			nodeName,
			string(models.StateMonitoring),
			"Node %s cooldown period ended, transitioning to monitoring state",
			nodeName,
		)
	}
}
