package queue

import (
	"context"
	"fmt"
	"k2p-updater/internal/common"
	"k2p-updater/internal/features/updater/domain/models"
	"log"
	"sync"
	"time"
)

// Operation represents a state update operation in the queue
type Operation struct {
	NodeName  string
	Event     models.Event
	Data      map[string]interface{}
	Context   context.Context
	ResultCh  chan OperationResult
	CreatedAt time.Time
	Priority  int
}

// OperationResult contains the result of a processed operation
type OperationResult struct {
	Error error
}

// NodeProcessor defines an interface for processing node operations
type NodeProcessor interface {
	ProcessNodeOperation(ctx context.Context, nodeName string, event models.Event, data map[string]interface{}) error
}

// UpdateQueue manages queued operations for each node
type UpdateQueue struct {
	nodeQueues      map[string]chan *Operation
	nodeProcessors  map[string]context.CancelFunc
	processor       NodeProcessor
	mu              sync.RWMutex
	maxQueueSize    int
	maxRetries      int
	processingDelay time.Duration
}

// NewUpdateQueue creates a new update queue with the specified processor
func NewUpdateQueue(processor NodeProcessor, options ...UpdateQueueOption) *UpdateQueue {
	queue := &UpdateQueue{
		nodeQueues:      make(map[string]chan *Operation),
		nodeProcessors:  make(map[string]context.CancelFunc),
		processor:       processor,
		maxQueueSize:    100,
		maxRetries:      3,
		processingDelay: 50 * time.Millisecond,
	}

	// Apply options
	for _, option := range options {
		option(queue)
	}

	return queue
}

// UpdateQueueOption defines options for the update queue
type UpdateQueueOption func(*UpdateQueue)

// WithMaxQueueSize sets the maximum queue size
func WithMaxQueueSize(size int) UpdateQueueOption {
	return func(q *UpdateQueue) {
		q.maxQueueSize = size
	}
}

// WithMaxRetries sets the maximum retry count
func WithMaxRetries(retries int) UpdateQueueOption {
	return func(q *UpdateQueue) {
		q.maxRetries = retries
	}
}

// WithProcessingDelay sets the delay between processing operations
func WithProcessingDelay(delay time.Duration) UpdateQueueOption {
	return func(q *UpdateQueue) {
		q.processingDelay = delay
	}
}

// EnqueueOperation adds an operation to the node's queue
func (q *UpdateQueue) EnqueueOperation(ctx context.Context, nodeName string, event models.Event, data map[string]interface{}) error {
	// Check context first
	if err := common.CheckContext(ctx); err != nil {
		return fmt.Errorf("context error before enqueueing: %w", err)
	}

	// Create result channel
	resultCh := make(chan OperationResult, 1)

	// Create the operation
	op := &Operation{
		NodeName:  nodeName,
		Event:     event,
		Data:      data,
		Context:   ctx,
		ResultCh:  resultCh,
		CreatedAt: time.Now(),
	}

	// Get or create queue for this node
	nodeQueue, err := q.getOrCreateNodeQueue(nodeName)
	if err != nil {
		return fmt.Errorf("failed to get queue for node %s: %w", nodeName, err)
	}

	// Try to enqueue with timeout
	select {
	case nodeQueue <- op:
		// Successfully enqueued
	case <-ctx.Done():
		return fmt.Errorf("failed to enqueue operation: %w", ctx.Err())
	case <-time.After(5 * time.Second):
		return fmt.Errorf("failed to enqueue operation: queue is full")
	}

	// Wait for the result with context timeout
	select {
	case result := <-resultCh:
		return result.Error
	case <-ctx.Done():
		return fmt.Errorf("operation canceled while waiting for result: %w", ctx.Err())
	}
}

// getOrCreateNodeQueue gets or creates a queue for a node
func (q *UpdateQueue) getOrCreateNodeQueue(nodeName string) (chan *Operation, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Check if queue already exists
	if queue, exists := q.nodeQueues[nodeName]; exists {
		return queue, nil
	}

	// Create a new queue
	queue := make(chan *Operation, q.maxQueueSize)
	q.nodeQueues[nodeName] = queue

	// Start a processor for this node
	workerCtx, cancel := context.WithCancel(context.Background())
	q.nodeProcessors[nodeName] = cancel
	go q.processNodeQueue(workerCtx, nodeName, queue)

	log.Printf("Created new queue and processor for node %s", nodeName)
	return queue, nil
}

// processNodeQueue processes operations for a specific node
func (q *UpdateQueue) processNodeQueue(ctx context.Context, nodeName string, queue chan *Operation) {
	log.Printf("Starting operation processor for node %s", nodeName)

	for {
		select {
		case <-ctx.Done():
			log.Printf("Stopping operation processor for node %s: %v", nodeName, ctx.Err())
			return
		case op := <-queue:
			// Process the operation
			q.processOperation(op)

			// Add small delay to prevent CPU hogging
			time.Sleep(q.processingDelay)
		}
	}
}

// processOperation processes a single operation
func (q *UpdateQueue) processOperation(op *Operation) {
	// Check if operation context is still valid
	if op.Context.Err() != nil {
		op.ResultCh <- OperationResult{Error: op.Context.Err()}
		return
	}

	// Process the operation
	err := q.processor.ProcessNodeOperation(op.Context, op.NodeName, op.Event, op.Data)

	// Send the result
	op.ResultCh <- OperationResult{Error: err}
}

// Shutdown gracefully shuts down all processors
func (q *UpdateQueue) Shutdown() {
	q.mu.Lock()
	defer q.mu.Unlock()

	log.Println("Shutting down update queue...")
	for nodeName, cancel := range q.nodeProcessors {
		log.Printf("Stopping processor for node %s", nodeName)
		cancel()
	}

	// Clear maps
	q.nodeProcessors = make(map[string]context.CancelFunc)
	q.nodeQueues = make(map[string]chan *Operation)
}
