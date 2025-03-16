package recovery

import (
	"context"
	"errors"
	"fmt"
	"k2p-updater/internal/features/updater/domain/interfaces"
	"k2p-updater/internal/features/updater/domain/models"
	"log"
	"sync"
	"time"
)

// RecoveryManager handles recovery from failed states
type RecoveryManager struct {
	stateMachine           interfaces.StateMachine
	recoveryWaitPeriod     time.Duration
	recoveryCooldownPeriod time.Duration
	maxRecoveryAttempts    int
	recoveryAttempts       map[string]int
	mu                     sync.RWMutex
}

// NewRecoveryManager creates a new recovery manager
func NewRecoveryManager(stateMachine interfaces.StateMachine) interfaces.RecoveryManager {
	return &RecoveryManager{
		stateMachine:           stateMachine,
		recoveryWaitPeriod:     5 * time.Minute,
		recoveryCooldownPeriod: 10 * time.Minute,
		maxRecoveryAttempts:    3,
		recoveryAttempts:       make(map[string]int),
	}
}

// AttemptRecovery tries to recover a node from a failed state
func (r *RecoveryManager) AttemptRecovery(ctx context.Context, nodeName string) error {
	// Check for context cancellation
	if ctx.Err() != nil {
		return ctx.Err()
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
	r.mu.RLock()
	attempts := r.recoveryAttempts[nodeName]
	r.mu.RUnlock()

	if attempts >= r.maxRecoveryAttempts {
		log.Printf("Node %s has reached maximum recovery attempts (%d), manual intervention required",
			nodeName, r.maxRecoveryAttempts)
		return nil
	}

	// Check for context cancellation again
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Increment recovery attempts counter
	r.mu.Lock()
	r.recoveryAttempts[nodeName]++
	attemptsCount := r.recoveryAttempts[nodeName]
	r.mu.Unlock()

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

// ResetRecoveryCounter resets the recovery counter for a node
func (r *RecoveryManager) ResetRecoveryCounter(nodeName string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.recoveryAttempts, nodeName)
}

// CheckAllNodes attempts recovery for all nodes in failed state
func (r *RecoveryManager) CheckAllNodes(ctx context.Context, nodes []string) {
	// Check for context cancellation at the beginning
	if ctx.Err() != nil {
		log.Printf("Skipping recovery check due to context cancellation: %v", ctx.Err())
		return
	}

	for _, nodeName := range nodes {
		// Check for context cancellation during iteration
		if ctx.Err() != nil {
			log.Printf("Context canceled during recovery check: %v", ctx.Err())
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
