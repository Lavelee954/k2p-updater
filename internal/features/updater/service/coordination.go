package service

import (
	"context"
	"fmt"
	errorhandling "k2p-updater/internal/common"
	"k2p-updater/internal/features/updater/domain"
	"log"
	"sync"
	"time"
)

// CoordinationManager handles coordination between control plane nodes
type CoordinationManager struct {
	stateMachine  domain.StateMachine
	statusCache   map[string]domain.State
	lastUpdate    time.Time
	cacheLifetime time.Duration
	mu            sync.RWMutex
}

// NewCoordinationManager creates a new coordination manager
func NewCoordinationManager(stateMachine domain.StateMachine) *CoordinationManager {
	return &CoordinationManager{
		stateMachine:  stateMachine,
		statusCache:   make(map[string]domain.State),
		cacheLifetime: 10 * time.Second,
	}
}

// IsAnyNodeSpecingUp checks if any node is currently being spec'd up
func (c *CoordinationManager) IsAnyNodeSpecingUp(ctx context.Context, nodes []string) (bool, string, error) {
	if err := errorhandling.HandleContextError(ctx, "checking nodes spec status"); err != nil {
		return false, "", err
	}

	// Add debug logging
	log.Printf("Checking if any of %d nodes is currently spec'ing up", len(nodes))

	c.mu.RLock()
	if time.Since(c.lastUpdate) < c.cacheLifetime {
		// Use cached data if recent
		for nodeName, state := range c.statusCache {
			if state == domain.StateInProgressVmSpecUp {
				c.mu.RUnlock()
				log.Printf("Found node %s in InProgressVmSpecUp state (cached)", nodeName)
				return true, nodeName, nil
			}
		}
		c.mu.RUnlock()
		log.Printf("No nodes currently spec'ing up (cached data)")
		return false, "", nil
	}
	c.mu.RUnlock()

	// Cache is stale, refresh it
	// We can't use CheckContextAndExecute here because we need to return multiple values
	if err := errorhandling.HandleContextError(ctx, "refreshing node status"); err != nil {
		return false, "", err
	}

	result, node, err := c.refreshAndCheck(ctx, nodes)
	if err != nil {
		if errorhandling.IsContextCanceled(err) {
			log.Printf("Context canceled during spec up check")
			return false, "", err
		}
		log.Printf("Error checking nodes status: %v", err)
	} else if result {
		log.Printf("Found node %s in InProgressVmSpecUp state (refreshed)", node)
	} else {
		log.Printf("No nodes currently spec'ing up (refreshed data)")
	}

	return result, node, err
}

// refreshAndCheck refreshes the state cache and checks for spec up
func (c *CoordinationManager) refreshAndCheck(ctx context.Context, nodes []string) (bool, string, error) {
	if err := errorhandling.HandleContextError(ctx, "refreshing node cache"); err != nil {
		return false, "", err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Create new cache
	newCache := make(map[string]domain.State)

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
				if prevState == domain.StateInProgressVmSpecUp {
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
		if state == domain.StateInProgressVmSpecUp {
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
func (c *CoordinationManager) NextEligibleNode(ctx context.Context, nodes []string) (string, error) {
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

		if status.CurrentState == domain.StateMonitoring &&
			status.WindowAverageUtilization > 0 { // You might want a proper threshold here
			return nodeName, nil
		}
	}

	return "", fmt.Errorf("no eligible nodes for spec up")
}
