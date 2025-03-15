package service

import (
	"context"
	"fmt"
	"k2p-updater/internal/features/updater/domain"
	"log"
	"sync"
	"time"
)

// RecoveryManager handles recovery from failed states
type RecoveryManager struct {
	stateMachine           domain.StateMachine
	recoveryWaitPeriod     time.Duration
	recoveryCooldownPeriod time.Duration
	maxRecoveryAttempts    int
	recoveryAttempts       map[string]int
	mu                     sync.RWMutex
}

// NewRecoveryManager creates a new recovery manager
func NewRecoveryManager(stateMachine domain.StateMachine) *RecoveryManager {
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
	// Get current node status
	status, err := r.stateMachine.GetStatus(ctx, nodeName)
	if err != nil {
		return fmt.Errorf("failed to get node status: %w", err)
	}

	// Only attempt recovery for nodes in failed state
	if status.CurrentState != domain.StateFailedVmSpecUp {
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
	if err := r.stateMachine.HandleEvent(ctx, nodeName, domain.EventRecoveryAttempt, data); err != nil {
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
	for _, nodeName := range nodes {
		if err := r.AttemptRecovery(ctx, nodeName); err != nil {
			log.Printf("Failed to attempt recovery for node %s: %v", nodeName, err)
		}
	}
}
