package recovery

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"k2p-updater/internal/features/updater/domain/interfaces"
	"k2p-updater/internal/features/updater/domain/models"
)

// Manager implements interfaces.RecoveryManager
type Manager struct {
	stateMachine           interfaces.StateMachine
	recoveryWaitPeriod     time.Duration
	recoveryCooldownPeriod time.Duration
	maxRecoveryAttempts    int
	recoveryAttempts       map[string]int
	mu                     sync.RWMutex
}

// NewRecoveryManager creates a new recovery manager
func NewRecoveryManager(stateMachine interfaces.StateMachine) interfaces.RecoveryManager {
	return &Manager{
		stateMachine:           stateMachine,
		recoveryWaitPeriod:     5 * time.Minute,
		recoveryCooldownPeriod: 10 * time.Minute,
		maxRecoveryAttempts:    3,
		recoveryAttempts:       make(map[string]int),
	}
}

// AttemptRecovery tries to recover a node from a failed state
func (r *Manager) AttemptRecovery(ctx context.Context, nodeName string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	// Get current node status
	status, err := r.stateMachine.GetStatus(ctx, nodeName)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return ctx.Err()
		}
		return fmt.Errorf("failed to get node status: %w", err)
	}

	// Only attempt recovery for nodes in failed state
	if status.CurrentState != models.StateFailedVmSpecUp {
		return nil
	}

	// Check if enough time has passed since failure
	if time.Since(status.LastTransitionTime) < r.recoveryWaitPeriod {
		return nil
	}

	// Check recovery attempts limit
	attempts := r.getRecoveryAttempts(nodeName)
	if attempts >= r.maxRecoveryAttempts {
		log.Printf("Node %s has reached maximum recovery attempts (%d), manual intervention required",
			nodeName, r.maxRecoveryAttempts)
		return nil
	}

	// Increment recovery attempts counter
	attemptsCount := r.incrementRecoveryAttempts(nodeName)
	log.Printf("Attempting recovery for node %s (attempt %d of %d)",
		nodeName, attemptsCount, r.maxRecoveryAttempts)

	// Prepare recovery data
	data := map[string]interface{}{
		"coolDownEndTime": time.Now().Add(r.recoveryCooldownPeriod),
		"recoveryAttempt": attemptsCount,
	}

	// Trigger recovery event
	if err := r.stateMachine.HandleEvent(ctx, nodeName, models.EventRecoveryAttempt, data); err != nil {
		if errors.Is(err, context.Canceled) {
			return ctx.Err()
		}
		return fmt.Errorf("failed to handle recovery event: %w", err)
	}

	log.Printf("Recovery initiated for node %s", nodeName)
	return nil
}

// getRecoveryAttempts gets the current recovery attempts count for a node
func (r *Manager) getRecoveryAttempts(nodeName string) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.recoveryAttempts[nodeName]
}

// incrementRecoveryAttempts increments and returns the recovery attempts count
func (r *Manager) incrementRecoveryAttempts(nodeName string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.recoveryAttempts[nodeName]++
	return r.recoveryAttempts[nodeName]
}

// ResetRecoveryCounter resets the recovery counter for a node
func (r *Manager) ResetRecoveryCounter(nodeName string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.recoveryAttempts, nodeName)
}

// CheckAllNodes attempts recovery for all nodes in failed state
func (r *Manager) CheckAllNodes(ctx context.Context, nodes []string) {
	if err := ctx.Err(); err != nil {
		log.Printf("Skipping recovery check due to context cancellation: %v", err)
		return
	}

	for _, nodeName := range nodes {
		if err := ctx.Err(); err != nil {
			log.Printf("Context canceled during recovery check: %v", err)
			return
		}

		if err := r.AttemptRecovery(ctx, nodeName); err != nil {
			if errors.Is(err, context.Canceled) {
				log.Printf("Context canceled during recovery attempt for node %s", nodeName)
				return
			}
			log.Printf("Failed to attempt recovery for node %s: %v", nodeName, err)
		}
	}
}
