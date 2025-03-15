package service

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"k2p-updater/internal/features/exporter/domain"
)

// healthChecker implements the domain.HealthChecker interface.
type healthChecker struct {
	httpClient *http.Client
}

// newHealthChecker creates a new health checker.
func newHealthChecker(timeout time.Duration) domain.HealthChecker {
	return &healthChecker{
		httpClient: &http.Client{Timeout: timeout},
	}
}

// CheckHealth implements domain.HealthChecker.CheckHealth.
func (c *healthChecker) CheckHealth(ctx context.Context, ip string, port int) (bool, error) {
	url := fmt.Sprintf("http://%s:%d/metrics", ip, port)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("failed to perform health check: %w", err)
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK, nil
}
