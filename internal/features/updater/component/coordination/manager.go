package coordination

import (
	"context"
	"fmt"
	errorhandling "k2p-updater/internal/common"
	"k2p-updater/internal/features/updater/domain/interfaces"
	"k2p-updater/internal/features/updater/domain/models"
	"log"
	"sync"
	"time"
)

// Manager CoordinationManager handles coordination between control plane nodes
type Manager struct {
	stateMachine  interfaces.StateMachine
	statusCache   map[string]models.State
	lastUpdate    time.Time
	cacheLifetime time.Duration
	mu            sync.RWMutex
}

// NewCoordinationManager creates a new coordination manager
func NewCoordinationManager(stateMachine interfaces.StateMachine) interfaces.Coordinator {
	return &Manager{
		stateMachine:  stateMachine,
		statusCache:   make(map[string]models.State),
		cacheLifetime: 10 * time.Second,
	}
}

// IsAnyNodeSpecingUp checks if any node is currently being spec'd up
func (c *Manager) IsAnyNodeSpecingUp(ctx context.Context, nodes []string) (bool, string, error) {
	if err := errorhandling.HandleContextError(ctx, "checking nodes spec status"); err != nil {
		return false, "", err
	}

	// Lock for the entire operation to prevent concurrent checks/updates
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if cache is still valid
	if time.Since(c.lastUpdate) < c.cacheLifetime {
		// Use cached data
		for nodeName, state := range c.statusCache {
			if state == models.StateInProgressVmSpecUp {
				log.Printf("Found node %s in InProgressVmSpecUp state (cached)", nodeName)
				return true, nodeName, nil
			}
		}
		log.Printf("No nodes currently spec'ing up (cached data)")
		return false, "", nil
	}

	// Cache is stale, need to refresh
	newCache := make(map[string]models.State)
	log.Printf("Refreshing state cache for %d nodes", len(nodes))

	// Check all nodes atomically
	for _, nodeName := range nodes {
		if err := errorhandling.HandleContextError(ctx, "checking node status"); err != nil {
			return false, "", err
		}

		state, err := c.stateMachine.GetCurrentState(ctx, nodeName)
		if err != nil {
			if errorhandling.IsContextCanceled(err) {
				return false, "", errorhandling.WrapError(err, "context canceled during node status check")
			}

			log.Printf("Warning: Failed to get state for node %s: %v", nodeName, err)

			// Use cached state if available
			if prevState, exists := c.statusCache[nodeName]; exists {
				newCache[nodeName] = prevState
				if prevState == models.StateInProgressVmSpecUp {
					// Update cache and return result
					c.statusCache = newCache
					c.lastUpdate = time.Now()
					log.Printf("Using cached state: node %s is in InProgressVmSpecUp", nodeName)
					return true, nodeName, nil
				}
			}
			continue
		}

		// Store state in cache
		newCache[nodeName] = state

		// If this node is currently being spec'd up, return immediately
		if state == models.StateInProgressVmSpecUp {
			// Update cache and return result
			c.statusCache = newCache
			c.lastUpdate = time.Now()
			log.Printf("Node %s is currently in InProgressVmSpecUp state", nodeName)
			return true, nodeName, nil
		}
	}

	// No node is being spec'd up
	c.statusCache = newCache
	c.lastUpdate = time.Now()
	return false, "", nil
}

// refreshAndCheck refreshes the state cache and checks for spec up
func (c *Manager) refreshAndCheck(ctx context.Context, nodes []string) (bool, string, error) {
	if err := errorhandling.HandleContextError(ctx, "refreshing node cache"); err != nil {
		return false, "", err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Create new cache
	newCache := make(map[string]models.State)

	// Add debug logging
	log.Printf("Refreshing state cache for %d nodes", len(nodes))

	// First pass: look specifically for any node in InProgressVmSpecUp state
	for _, nodeName := range nodes {
		if err := errorhandling.HandleContextError(ctx, "checking node status"); err != nil {
			return false, "", err
		}

		state, err := c.stateMachine.GetCurrentState(ctx, nodeName)
		if err != nil {
			// Check if this is due to context cancellation
			if errorhandling.IsContextCanceled(err) {
				return false, "", errorhandling.WrapError(err, "context canceled during node status check")
			}

			log.Printf("Warning: Failed to get state for node %s: %v", nodeName, err)

			// Use cached state if available
			if prevState, exists := c.statusCache[nodeName]; exists {
				newCache[nodeName] = prevState

				// If cached state shows node in progress, return early
				if prevState == models.StateInProgressVmSpecUp {
					log.Printf("Using cached state: node %s is in InProgressVmSpecUp", nodeName)
					c.statusCache = newCache
					c.lastUpdate = time.Now()
					return true, nodeName, nil
				}
			}
			continue
		}

		// Store state in cache
		newCache[nodeName] = state

		// If this node is currently being spec'd up, return immediately
		if state == models.StateInProgressVmSpecUp {
			log.Printf("Node %s is currently in InProgressVmSpecUp state", nodeName)
			c.statusCache = newCache
			c.lastUpdate = time.Now()
			return true, nodeName, nil
		}
	}

	// No node is being spec'd up
	c.statusCache = newCache
	c.lastUpdate = time.Now()
	return false, "", nil
}

// NextEligibleNode finds the next node eligible for spec up
func (c *Manager) NextEligibleNode(ctx context.Context, nodes []string) (string, error) {
	// Check if any node is already spec'ing up
	isSpecingUp, specingUpNode, err := c.IsAnyNodeSpecingUp(ctx, nodes)
	if err != nil {
		return "", errorhandling.WrapError(err, "failed to check ongoing spec up")
	}

	if isSpecingUp {
		return "", fmt.Errorf("node %s is already being spec'd up", specingUpNode)
	}

	// Find the first node that's in Monitoring state and has threshold exceeded
	for _, nodeName := range nodes {
		if err := errorhandling.HandleContextError(ctx, "checking node eligibility"); err != nil {
			return "", err
		}

		status, err := c.stateMachine.GetStatus(ctx, nodeName)
		if err != nil {
			log.Printf("Warning: Failed to get status for node %s: %v", nodeName, err)
			continue
		}

		if status.CurrentState == models.StateMonitoring &&
			status.WindowAverageUtilization > 0 { // You might want a proper threshold here
			return nodeName, nil
		}
	}

	return "", fmt.Errorf("no eligible nodes for spec up")
}
