package http

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"time"

	"golang.org/x/net/http2"
)

// ClientConfig holds configuration for the HTTP client
type ClientConfig struct {
	Timeout            time.Duration
	InsecureSkipVerify bool
	EnableHTTP2        bool
}

// DefaultClientConfig returns the default HTTP client configuration
func DefaultClientConfig() ClientConfig {
	return ClientConfig{
		Timeout:            10 * time.Second,
		InsecureSkipVerify: false, // Only set to true in development
		EnableHTTP2:        true,
	}
}

// Client provides a wrapper around http.Client with improved error handling
type Client struct {
	client *http.Client
	config ClientConfig
}

// NewClient creates a new HTTP client
func NewClient(config ClientConfig) (*Client, error) {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: config.InsecureSkipVerify,
		},
	}

	if config.EnableHTTP2 {
		if err := http2.ConfigureTransport(transport); err != nil {
			return nil, fmt.Errorf("failed to configure HTTP/2 transport: %w", err)
		}
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   config.Timeout,
	}

	return &Client{
		client: client,
		config: config,
	}, nil
}

// Request makes an HTTP request and returns the response
func (c *Client) Request(method, url string, body []byte, headers map[string]string) (*http.Response, error) {
	req, err := http.NewRequest(method, url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set default Content-Type if not provided
	if _, exists := headers["Content-Type"]; !exists {
		req.Header.Set("Content-Type", "application/json")
	}

	// Set headers
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send HTTP request: %w", err)
	}

	return resp, nil
}

// ReadResponseBody reads and closes the response body
func (c *Client) ReadResponseBody(resp *http.Response) ([]byte, error) {
	if resp == nil {
		return nil, fmt.Errorf("response is nil")
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return body, nil
}
