package service

import (
	"context"
	"fmt"
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
	c.mu.RLock()
	if time.Since(c.lastUpdate) < c.cacheLifetime {
		// Use cached data if recent
		for nodeName, state := range c.statusCache {
			if state == domain.StateInProgressVmSpecUp {
				c.mu.RUnlock()
				return true, nodeName, nil
			}
		}
		c.mu.RUnlock()
		return false, "", nil
	}
	c.mu.RUnlock()

	// Cache is stale, refresh it
	return c.refreshAndCheck(ctx, nodes)
}

// refreshAndCheck refreshes the state cache and checks for spec up
func (c *CoordinationManager) refreshAndCheck(ctx context.Context, nodes []string) (bool, string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Clear existing cache
	c.statusCache = make(map[string]domain.State)

	// Refresh cache with current states
	for _, nodeName := range nodes {
		state, err := c.stateMachine.GetCurrentState(ctx, nodeName)
		if err != nil {
			log.Printf("Warning: Failed to get state for node %s: %v", nodeName, err)
			continue
		}

		c.statusCache[nodeName] = state

		if state == domain.StateInProgressVmSpecUp {
			c.lastUpdate = time.Now()
			return true, nodeName, nil
		}
	}

	c.lastUpdate = time.Now()
	return false, "", nil
}

// NextEligibleNode finds the next node eligible for spec up
func (c *CoordinationManager) NextEligibleNode(ctx context.Context, nodes []string) (string, error) {
	// Check if any node is already spec'ing up
	isSpecingUp, specingUpNode, err := c.IsAnyNodeSpecingUp(ctx, nodes)
	if err != nil {
		return "", fmt.Errorf("failed to check ongoing spec up: %w", err)
	}

	if isSpecingUp {
		return "", fmt.Errorf("node %s is already being spec'd up", specingUpNode)
	}

	// Find the first node that's in Monitoring state and has threshold exceeded
	for _, nodeName := range nodes {
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
