package service

import (
	"context"
	"log"
	"sync"
	"time"

	"k2p-updater/internal/features/updater/domain"
	"k2p-updater/pkg/resource"
)

// NodeEventService handles periodic creation of node events
type NodeEventService struct {
	stateMachine     domain.StateMachine
	metricsComponent MetricsComponent
	resourceFactory  *resource.Factory
	nodes            []string
	nodesMutex       sync.RWMutex
	stopChan         chan struct{}
	startOnce        sync.Once
	serviceCtx       context.Context
	cancelFunc       context.CancelFunc
}

// NewNodeEventService creates a new node event service
func NewNodeEventService(
	stateMachine domain.StateMachine,
	metricsComponent MetricsComponent,
	resourceFactory *resource.Factory,
) *NodeEventService {
	return &NodeEventService{
		stateMachine:     stateMachine,
		metricsComponent: metricsComponent,
		resourceFactory:  resourceFactory,
		nodes:            []string{},
		stopChan:         make(chan struct{}),
	}
}

// UpdateNodes updates the list of nodes to monitor
func (s *NodeEventService) UpdateNodes(nodes []string) {
	s.nodesMutex.Lock()
	defer s.nodesMutex.Unlock()

	s.nodes = make([]string, len(nodes))
	copy(s.nodes, nodes)

	log.Printf("Node event service updated with %d nodes", len(s.nodes))
}

// Start begins periodic event creation
func (s *NodeEventService) Start(ctx context.Context) error {
	var startErr error

	s.startOnce.Do(func() {
		log.Println("Starting node event service...")

		// Derive a new context from the parent context
		serviceCtx, cancel := context.WithCancel(ctx)
		s.serviceCtx = serviceCtx
		s.cancelFunc = cancel

		// Start background goroutine with the derived context
		go s.eventCreationLoop(serviceCtx)

		// Add a monitoring goroutine to handle parent context cancellation
		go func() {
			<-ctx.Done()
			log.Println("Parent context canceled, stopping node event service...")
			s.Stop()
		}()

		log.Println("Node event service background loop started")
	})

	return startErr
}

// Stop stops the event service
func (s *NodeEventService) Stop() {
	log.Println("Stopping node event service...")

	// Cancel the internal context if it exists
	if s.cancelFunc != nil {
		s.cancelFunc()
	}

	// Close the stop channel to ensure all goroutines know to terminate
	close(s.stopChan)

	log.Println("Node event service stopped")
}

// eventCreationLoop periodically creates events for all nodes
func (s *NodeEventService) eventCreationLoop(ctx context.Context) {
	log.Println("Starting node event creation loop")

	// Setup heartbeat ticker (every 30 seconds)
	heartbeatTicker := time.NewTicker(30 * time.Second)
	defer heartbeatTicker.Stop()

	// Setup event creation ticker (every 2 minutes)
	eventTicker := time.NewTicker(2 * time.Minute)
	defer eventTicker.Stop()

	// Create initial events after a brief delay
	initialEventTimer := time.NewTimer(30 * time.Second)
	defer initialEventTimer.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Context cancelled, stopping node event service")
			return
		case <-s.stopChan:
			log.Println("Stop signal received, stopping node event service")
			return
		case <-heartbeatTicker.C:
			log.Println("NODE EVENT SERVICE HEARTBEAT: Service is running")
		case <-eventTicker.C:
			log.Println("Ticker triggered, creating periodic events for all nodes")
			s.createEventsForAllNodes(ctx)
		case <-initialEventTimer.C:
			log.Println("Creating initial events for all nodes")
			s.createEventsForAllNodes(ctx)
		}
	}
}

func (s *NodeEventService) createEventsForAllNodes(ctx context.Context) {
	s.nodesMutex.RLock()
	nodes := make([]string, len(s.nodes))
	copy(nodes, s.nodes)
	s.nodesMutex.RUnlock()

	log.Printf("Creating events for %d nodes", len(nodes))

	// Track last event time per node to prevent too frequent updates
	lastEventTimes := make(map[string]time.Time)

	for _, nodeName := range nodes {
		// Limit event frequency - only create events every 60 seconds per node
		now := time.Now()
		if lastTime, exists := lastEventTimes[nodeName]; exists {
			if now.Sub(lastTime) < 60*time.Second {
				log.Printf("Skipping event for node %s - too soon since last event", nodeName)
				continue
			}
		}

		// Update the last event time
		lastEventTimes[nodeName] = now

		// Get current node state with error handling
		state, err := s.stateMachine.GetCurrentState(ctx, nodeName)
		if err != nil {
			log.Printf("Failed to get state for node %s: %v", nodeName, err)
			continue
		}

		// Get current CPU metrics
		currentCPU, windowAvg, err := s.metricsComponent.GetNodeCPUMetrics(nodeName)
		if err != nil {
			log.Printf("Failed to get CPU metrics for node %s: %v", nodeName, err)
			currentCPU = 0
			windowAvg = 0
		}

		// Create a single status event for this node
		log.Printf("Creating status event for node %s (state=%s, CPU=%.2f%%, window=%.2f%%)",
			nodeName, state, currentCPU, windowAvg)
	}
}

func (s *NodeEventService) verifyEventCreation(ctx context.Context) {
	log.Println("Verifying event creation capability...")

	for _, nodeName := range s.nodes {
		// Create a test verification event
		err := s.resourceFactory.Event().NormalRecordWithNode(
			ctx,
			"updater",
			nodeName,
			"EventVerification",
			"Verification event for node %s at %s",
			nodeName, time.Now().Format(time.RFC3339),
		)

		if err != nil {
			log.Printf("EVENT VERIFICATION FAILED for node %s: %v", nodeName, err)
		} else {
			log.Printf("EVENT VERIFICATION SUCCEEDED for node %s", nodeName)
		}
	}
}

func (s *NodeEventService) updateNodeStatusInCR(ctx context.Context, nodeName string) {
	log.Printf("Updating CR status for node %s", nodeName)

	// Get current node state
	status, err := s.stateMachine.GetStatus(ctx, nodeName)
	if err != nil {
		log.Printf("Failed to get status for node %s: %v", nodeName, err)
		return
	}

	// Get current CPU metrics
	currentCPU, windowAvg, err := s.metricsComponent.GetNodeCPUMetrics(nodeName)
	if err != nil {
		log.Printf("Failed to get CPU metrics for node %s: %v", nodeName, err)
		// Continue with previous values
	} else {
		// Update status with new CPU values
		status.CPUUtilization = currentCPU
		status.WindowAverageUtilization = windowAvg
	}

	// Directly update the CR status
	statusData := map[string]interface{}{
		"controlPlaneNodeName": nodeName,
		"cpuWinUsage":          status.WindowAverageUtilization,
		"coolDown":             status.CurrentState == domain.StateCoolDown,
		"updateStatus":         string(status.CurrentState),
		"message":              status.Message,
		"lastUpdateTime":       time.Now().Format(time.RFC3339),
	}

	log.Printf("Updating CR status for node %s with CPU: %.2f%%, window: %.2f%%",
		nodeName, status.CPUUtilization, status.WindowAverageUtilization)

	// Always use "master" as the node name for updater resource
	err = s.resourceFactory.Status().UpdateGenericWithNode(ctx, "updater", "master", statusData)
	if err != nil {
		log.Printf("Failed to update CR status for node %s: %v", nodeName, err)
	} else {
		log.Printf("Successfully updated CR status for node %s", nodeName)
	}
}
