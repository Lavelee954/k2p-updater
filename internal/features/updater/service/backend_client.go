package service

import (
	"context"
	"encoding/json"
	"fmt"
	"k2p-updater/cmd/app"
	"k2p-updater/internal/features/updater/domain"
	"net/http"
	"strings"
	"time"
)

// BackendClient implements the domain.BackendClient interface
type BackendClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewBackendClient creates a new backend client
func NewBackendClient(config *app.UpdaterConfig) domain.BackendClient {
	return &BackendClient{
		baseURL: "http://backend-api/v1", // Default, should be configured
		apiKey:  "api-key",               // Default, should be configured
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// RequestVMSpecUp sends a spec up request to the backend
func (c *BackendClient) RequestVMSpecUp(ctx context.Context, nodeName string, currentCPU float64) (bool, error) {
	// Construct request URL
	url := fmt.Sprintf("%s/nodes/%s/specup", c.baseURL, nodeName)

	// Prepare request body
	reqBody := fmt.Sprintf(`{"nodeName": "%s", "currentCPU": %.2f}`, nodeName, currentCPU)

	// Create request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(reqBody))
	if err != nil {
		return false, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))

	// Send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("backend returned status %d", resp.StatusCode)
	}

	// Parse response
	var response struct {
		Success bool `json:"success"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return false, fmt.Errorf("failed to parse response: %w", err)
	}

	return response.Success, nil
}

// GetVMSpecUpStatus checks the status of a spec up request
func (c *BackendClient) GetVMSpecUpStatus(ctx context.Context, nodeName string) (bool, error) {
	// Construct request URL
	url := fmt.Sprintf("%s/nodes/%s/specup/status", c.baseURL, nodeName)

	// Create request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))

	// Send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("backend returned status %d", resp.StatusCode)
	}

	// Parse response
	var response struct {
		Completed bool `json:"completed"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return false, fmt.Errorf("failed to parse response: %w", err)
	}

	return response.Completed, nil
}
