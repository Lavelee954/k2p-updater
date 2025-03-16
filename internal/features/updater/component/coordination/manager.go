package coordination

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"k2p-updater/internal/common"
	"k2p-updater/internal/features/updater/domain/interfaces"
	"k2p-updater/internal/features/updater/domain/models"
)

// Manager implements interfaces.Coordinator for coordinating spec up operations
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
	if err := ctx.Err(); err != nil {
		return false, "", err
	}

	// Use cache if still valid
	if c.isCacheValid() {
		specingNode := c.findSpecingUpNodeFromCache()
		if specingNode != "" {
			return true, specingNode, nil
		}
		return false, "", nil
	}

	// Cache is stale, need to refresh
	return c.refreshAndCheckSpecUp(ctx, nodes)
}

// isCacheValid checks if the cache is still valid
func (c *Manager) isCacheValid() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return time.Since(c.lastUpdate) < c.cacheLifetime
}

// findSpecingUpNodeFromCache finds a node in spec-up state from cache
func (c *Manager) findSpecingUpNodeFromCache() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for nodeName, state := range c.statusCache {
		if state == models.StateInProgressVmSpecUp {
			log.Printf("Found node %s in InProgressVmSpecUp state (cached)", nodeName)
			return nodeName
		}
	}

	return ""
}

// refreshAndCheckSpecUp refreshes the state cache and checks for spec up
func (c *Manager) refreshAndCheckSpecUp(ctx context.Context, nodes []string) (bool, string, error) {
	if err := ctx.Err(); err != nil {
		return false, "", err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Create new cache
	newCache := make(map[string]models.State)
	log.Printf("Refreshing state cache for %d nodes", len(nodes))

	// Check all nodes for spec-up state
	for _, nodeName := range nodes {
		if err := ctx.Err(); err != nil {
			return false, "", err
		}

		state, err := c.stateMachine.GetCurrentState(ctx, nodeName)
		if err != nil {
			// Check for context cancellation
			if common.IsContextCanceled(err) {
				return false, "", err
			}

			log.Printf("Warning: Failed to get state for node %s: %v", nodeName, err)

			// Use cached state if available
			if prevState, exists := c.statusCache[nodeName]; exists {
				newCache[nodeName] = prevState

				if prevState == models.StateInProgressVmSpecUp {
					log.Printf("Using cached state: node %s is in InProgressVmSpecUp", nodeName)
					c.updateCache(newCache)
					return true, nodeName, nil
				}
			}
			continue
		}

		// Store state in new cache
		newCache[nodeName] = state

		// If a node is being spec'd up, return immediately
		if state == models.StateInProgressVmSpecUp {
			log.Printf("Node %s is currently in InProgressVmSpecUp state", nodeName)
			c.updateCache(newCache)
			return true, nodeName, nil
		}
	}

	// No node is being spec'd up
	c.updateCache(newCache)
	return false, "", nil
}

// updateCache updates the state cache
func (c *Manager) updateCache(newCache map[string]models.State) {
	c.statusCache = newCache
	c.lastUpdate = time.Now()
}

// NextEligibleNode finds the next node eligible for spec up
func (c *Manager) NextEligibleNode(ctx context.Context, nodes []string) (string, error) {
	// Check if any node is already spec'ing up
	isSpecingUp, specingUpNode, err := c.IsAnyNodeSpecingUp(ctx, nodes)
	if err != nil {
		return "", fmt.Errorf("failed to check ongoing spec up: %w", err)
	}

	if isSpecingUp {
		return "", fmt.Errorf("node %s is already being spec'd up", specingUpNode)
	}

	// Find the first eligible node in Monitoring state
	for _, nodeName := range nodes {
		if err := ctx.Err(); err != nil {
			return "", err
		}

		status, err := c.stateMachine.GetStatus(ctx, nodeName)
		if err != nil {
			log.Printf("Warning: Failed to get status for node %s: %v", nodeName, err)
			continue
		}

		// Check if node is in monitoring state and has sufficient CPU utilization
		if status.CurrentState == models.StateMonitoring && status.WindowAverageUtilization > 0 {
			return nodeName, nil
		}
	}

	return "", fmt.Errorf("no eligible nodes for spec up")
}
