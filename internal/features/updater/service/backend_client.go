package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"k2p-updater/cmd/app"
	"k2p-updater/internal/common"
	"k2p-updater/internal/features/updater/domain"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
)

// BackendClient implements the domain.BackendClient interface with retry and circuit breaker
type BackendClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	timeout    time.Duration

	// Circuit breaker fields
	circuitOpen      bool
	failureCount     int
	failureThreshold int
	lastFailure      time.Time
	resetTimeout     time.Duration
	mu               sync.RWMutex
}

// BackendClientOption defines functional options for BackendClient
type BackendClientOption func(*BackendClient)

// WithTimeout sets the request timeout
func WithTimeout(timeout time.Duration) BackendClientOption {
	return func(bc *BackendClient) {
		bc.timeout = timeout
	}
}

// WithCircuitBreaker configures circuit breaker parameters
func WithCircuitBreaker(threshold int, resetTimeout time.Duration) BackendClientOption {
	return func(bc *BackendClient) {
		bc.failureThreshold = threshold
		bc.resetTimeout = resetTimeout
	}
}

// NewBackendClient creates a new backend client with retry logic and circuit breaker
func NewBackendClient(config *app.BackendConfig, options ...BackendClientOption) domain.BackendClient {
	client := &BackendClient{
		baseURL:          config.BaseURL,
		apiKey:           config.APIKey,
		timeout:          config.Timeout,
		failureThreshold: 5,               // Default threshold
		resetTimeout:     1 * time.Minute, // Default reset timeout
	}

	// Apply options
	for _, option := range options {
		option(client)
	}

	client.httpClient = &http.Client{
		Timeout: client.timeout,
	}

	return client
}

// isCircuitOpen checks if the circuit breaker is open
func (c *BackendClient) isCircuitOpen() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// If circuit is open but enough time has passed, we'll try again (half-open state)
	if c.circuitOpen && time.Since(c.lastFailure) > c.resetTimeout {
		return false
	}

	return c.circuitOpen
}

// recordSuccess resets failure count on successful call
func (c *BackendClient) recordSuccess() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.failureCount = 0
	c.circuitOpen = false
}

// recordFailure increments failure count and potentially opens circuit
func (c *BackendClient) recordFailure() {
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

// RequestVMSpecUp sends a spec up request to the backend with retry logic
func (c *BackendClient) RequestVMSpecUp(ctx context.Context, nodeName string, currentCPU float64) (bool, error) {
	// Check circuit breaker
	if c.isCircuitOpen() {
		return false, common.UnavailableError("circuit breaker is open")
	}

	// Create exponential backoff
	expBackoff := backoff.NewExponentialBackOff()
	expBackoff.MaxElapsedTime = 30 * time.Second

	// Prepare request payload
	reqBody := fmt.Sprintf(`{"nodeName": "%s", "currentCPU": %.2f}`, nodeName, currentCPU)

	// Use backoff retry
	var responseSuccess bool
	var responseError error

	operation := func() error {
		// Check context before each retry
		if ctx.Err() != nil {
			return backoff.Permanent(ctx.Err())
		}

		// Create new request for each retry
		url := fmt.Sprintf("%s/nodes/%s/specup", c.baseURL, nodeName)
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
			// Check if this is a context cancellation error
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return backoff.Permanent(err)
			}
			// Transient network errors should be retried
			return fmt.Errorf("request failed: %w", err)
		}
		defer resp.Body.Close()

		// Handle different response statuses
		switch resp.StatusCode {
		case http.StatusOK, http.StatusCreated, http.StatusAccepted:
			// Parse successful response
			var response struct {
				Success bool `json:"success"`
			}

			if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
				return backoff.Permanent(fmt.Errorf("failed to parse response: %w", err))
			}

			responseSuccess = response.Success
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

	// Modify to use backoff.RetryNotify to get visibility into retries
	err := backoff.RetryNotify(
		operation,
		expBackoff,
		func(err error, duration time.Duration) {
			// Check if we're retrying due to context cancellation
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				log.Printf("Not retrying request to backend for node %s due to context cancellation: %v",
					nodeName, err)
				return
			}

			log.Printf("Request to backend for node %s failed: %v, retrying in %.2f seconds",
				nodeName, err, duration.Seconds())
		},
	)

	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			log.Printf("RequestVMSpecUp for node %s canceled due to context: %v", nodeName, err)
			return false, err
		}
		c.recordFailure()
		responseError = err
	} else {
		c.recordSuccess()
	}

	return responseSuccess, responseError
}

// GetVMSpecUpStatus checks the status of a spec up request with retry logic
func (c *BackendClient) GetVMSpecUpStatus(ctx context.Context, nodeName string) (bool, error) {
	// Check circuit breaker
	if c.isCircuitOpen() {
		return false, common.UnavailableError("circuit breaker is open")
	}

	// Create exponential backoff
	expBackoff := backoff.NewExponentialBackOff()
	expBackoff.MaxElapsedTime = 20 * time.Second

	// Use backoff retry
	var completed bool
	var responseError error

	operation := func() error {
		// Check context before each retry
		if ctx.Err() != nil {
			return backoff.Permanent(ctx.Err())
		}

		// Create new request for each retry
		url := fmt.Sprintf("%s/nodes/%s/specup/status", c.baseURL, nodeName)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return backoff.Permanent(fmt.Errorf("failed to create request: %w", err))
		}

		// Set headers
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))

		// Send request
		resp, err := c.httpClient.Do(req)
		if err != nil {
			// Check if this is a context cancellation error
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return backoff.Permanent(err)
			}
			// Transient network errors should be retried
			return fmt.Errorf("request failed: %w", err)
		}
		defer resp.Body.Close()

		// Handle different response statuses
		switch resp.StatusCode {
		case http.StatusOK:
			// Parse successful response
			var response struct {
				Completed bool `json:"completed"`
			}

			if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
				return backoff.Permanent(fmt.Errorf("failed to parse response: %w", err))
			}

			completed = response.Completed
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

	// Execute with retry and notify
	err := backoff.RetryNotify(
		operation,
		expBackoff,
		func(err error, duration time.Duration) {
			// Check if we're retrying due to context cancellation
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				log.Printf("Not retrying status request for node %s due to context cancellation: %v",
					nodeName, err)
				return
			}

			log.Printf("Status request for node %s failed: %v, retrying in %.2f seconds",
				nodeName, err, duration.Seconds())
		},
	)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			log.Printf("GetVMSpecUpStatus for node %s canceled due to context: %v", nodeName, err)
			return false, err
		}
		c.recordFailure()
		responseError = err
	} else {
		c.recordSuccess()
	}

	return completed, responseError
}
