package service

import (
	"context"
	"fmt"
	"io"
	domainExporter "k2p-updater/internal/features/exporter/domain"
	"log"
	"net/http"
	"time"
)

// NodeMetricsFetcher implements domain.Fetcher
type NodeMetricsFetcher struct {
	timeout time.Duration
}

// FetchNodeMetrics retrieves metrics from a node-exporter
func (f *NodeMetricsFetcher) FetchNodeMetrics(ctx context.Context, exporter domainExporter.NodeExporter) (string, error) {
	client := &http.Client{Timeout: f.timeout}
	url := fmt.Sprintf("http://%s:9100/metrics", exporter.IP)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to get metrics from %s: %w", exporter.IP, err)
	}
	defer func(Body io.ReadCloser) {
		if err := Body.Close(); err != nil {
			log.Printf("failed to close response body: %v", err)
		}
	}(resp.Body)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	return string(body), nil
}
