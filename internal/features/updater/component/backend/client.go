package backend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"

	"k2p-updater/cmd/app"
	"k2p-updater/internal/common"
	"k2p-updater/internal/features/updater/domain/interfaces"
)

// Client BackendClient implements interfaces.BackendClient with retry and circuit breaker
type Client struct {
	baseURL          string
	apiKey           string
	httpClient       *http.Client
	timeout          time.Duration
	circuitOpen      bool
	failureCount     int
	failureThreshold int
	lastFailure      time.Time
	resetTimeout     time.Duration
	mu               sync.RWMutex
}

// NewBackendClient creates a new backend client
func NewBackendClient(config *app.BackendConfig, httpClient *http.Client, options ...BackendClientOption) interfaces.BackendClient {
	client := &Client{
		baseURL:          config.BaseURL,
		apiKey:           config.APIKey,
		timeout:          config.Timeout,
		failureThreshold: 5,
		resetTimeout:     1 * time.Minute,
		httpClient:       httpClient,
	}

	// Apply options
	for _, option := range options {
		option(client)
	}

	// Create a default HTTP client if none provided
	if client.httpClient == nil {
		client.httpClient = &http.Client{
			Timeout: client.timeout,
		}
	}

	return client
}

// RequestVMSpecUp sends a spec up request to the backend
func (c *Client) RequestVMSpecUp(ctx context.Context, nodeName string, currentCPU float64) (bool, error) {
	// Check circuit breaker
	if c.isCircuitOpen() {
		return false, common.UnavailableError("circuit breaker is open")
	}

	// Prepare request payload
	reqBody := fmt.Sprintf(`{"nodeName": "%s", "currentCPU": %.2f}`, nodeName, currentCPU)
	url := fmt.Sprintf("%s/nodes/%s/specup", c.baseURL, nodeName)

	// Define the operation to retry
	var responseSuccess bool
	operation := func() error {
		// Check context cancellation
		if err := ctx.Err(); err != nil {
			return backoff.Permanent(err)
		}

		// Create new request
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(reqBody))
		if err != nil {
			return backoff.Permanent(fmt.Errorf("failed to create request: %w", err))
		}

		// Set headers
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))

		// Send request
		resp, err := c.httpClient.Do(req)
		if err != nil {
			// Check for context cancellation
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return backoff.Permanent(err)
			}
			return fmt.Errorf("request failed: %w", err)
		}
		defer resp.Body.Close()

		return c.handleResponse(resp, &responseSuccess)
	}

	// Execute with retry
	err := c.executeWithRetry(ctx, operation, "RequestVMSpecUp", nodeName)
	return responseSuccess, err
}

// GetVMSpecUpStatus checks the status of a spec up request
func (c *Client) GetVMSpecUpStatus(ctx context.Context, nodeName string) (bool, error) {
	// Check circuit breaker
	if c.isCircuitOpen() {
		return false, common.UnavailableError("circuit breaker is open")
	}

	url := fmt.Sprintf("%s/nodes/%s/specup/status", c.baseURL, nodeName)

	// Define the operation to retry
	var completed bool
	operation := func() error {
		// Check context cancellation
		if err := ctx.Err(); err != nil {
			return backoff.Permanent(err)
		}

		// Create new request
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return backoff.Permanent(fmt.Errorf("failed to create request: %w", err))
		}

		// Set headers
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))

		// Send request
		resp, err := c.httpClient.Do(req)
		if err != nil {
			// Check for context cancellation
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return backoff.Permanent(err)
			}
			return fmt.Errorf("request failed: %w", err)
		}
		defer resp.Body.Close()

		return c.handleStatusResponse(resp, &completed)
	}

	// Execute with retry
	err := c.executeWithRetry(ctx, operation, "GetVMSpecUpStatus", nodeName)
	return completed, err
}

// handleResponse processes the HTTP response for spec up requests
func (c *Client) handleResponse(resp *http.Response, success *bool) error {
	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusAccepted:
		var response struct {
			Success bool `json:"success"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			return backoff.Permanent(fmt.Errorf("failed to parse response: %w", err))
		}

		*success = response.Success
		return nil

	case http.StatusTooManyRequests, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		// Retry these errors
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("backend returned status %d: %s", resp.StatusCode, string(body))

	default:
		// Don't retry other errors
		body, _ := io.ReadAll(resp.Body)
		return backoff.Permanent(fmt.Errorf("backend returned status %d: %s", resp.StatusCode, string(body)))
	}
}

// handleStatusResponse processes the HTTP response for status checks
func (c *Client) handleStatusResponse(resp *http.Response, completed *bool) error {
	if resp.StatusCode == http.StatusOK {
		var response struct {
			Completed bool `json:"completed"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			return backoff.Permanent(fmt.Errorf("failed to parse response: %w", err))
		}

		*completed = response.Completed
		return nil
	}

	// Handle non-200 responses
	if resp.StatusCode == http.StatusTooManyRequests ||
		resp.StatusCode == http.StatusServiceUnavailable ||
		resp.StatusCode == http.StatusGatewayTimeout {
		// Retry these errors
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("backend returned status %d: %s", resp.StatusCode, string(body))
	}

	// Don't retry other errors
	body, _ := io.ReadAll(resp.Body)
	return backoff.Permanent(fmt.Errorf("backend returned status %d: %s", resp.StatusCode, string(body)))
}

// executeWithRetry executes an operation with retry logic
func (c *Client) executeWithRetry(ctx context.Context, operation backoff.Operation, operationName, nodeName string) error {
	// Create exponential backoff
	expBackoff := backoff.NewExponentialBackOff()
	expBackoff.MaxElapsedTime = 30 * time.Second

	// Execute with retry and notify
	err := backoff.RetryNotify(
		operation,
		expBackoff,
		func(err error, duration time.Duration) {
			// Skip logging for context cancellation
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				log.Printf("Not retrying %s for node %s due to context cancellation: %v",
					operationName, nodeName, err)
				return
			}

			log.Printf("%s for node %s failed: %v, retrying in %.2f seconds",
				operationName, nodeName, err, duration.Seconds())
		},
	)

	// Handle the result
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			log.Printf("%s for node %s canceled due to context: %v", operationName, nodeName, err)
			return err
		}
		c.recordFailure()
	} else {
		c.recordSuccess()
	}

	return err
}

// isCircuitOpen checks if the circuit breaker is open
func (c *Client) isCircuitOpen() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Allow retry if enough time has passed (half-open state)
	if c.circuitOpen && time.Since(c.lastFailure) > c.resetTimeout {
		return false
	}

	return c.circuitOpen
}

// recordSuccess resets failure count on successful call
func (c *Client) recordSuccess() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.failureCount = 0
	c.circuitOpen = false
}

// recordFailure increments failure count and potentially opens circuit
func (c *Client) recordFailure() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.failureCount++
	c.lastFailure = time.Now()

	if c.failureCount >= c.failureThreshold {
		if !c.circuitOpen {
			log.Printf("Circuit breaker opened after %d consecutive failures", c.failureCount)
		}
		c.circuitOpen = true
	}
}

// BackendClientOption defines functional options for BackendClient
type BackendClientOption func(*Client)

// WithTimeout sets the request timeout
func WithTimeout(timeout time.Duration) BackendClientOption {
	return func(bc *Client) {
		bc.timeout = timeout
	}
}

// WithCircuitBreaker configures circuit breaker parameters
func WithCircuitBreaker(threshold int, resetTimeout time.Duration) BackendClientOption {
	return func(bc *Client) {
		bc.failureThreshold = threshold
		bc.resetTimeout = resetTimeout
	}
}
