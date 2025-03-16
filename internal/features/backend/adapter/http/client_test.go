package http

import (
	"bytes"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockRoundTripper implements the http.RoundTripper interface for testing
type mockRoundTripper struct {
	response *http.Response
	err      error
}

func (m *mockRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return m.response, m.err
}

func TestDefaultClientConfig(t *testing.T) {
	config := DefaultClientConfig()

	assert.Equal(t, 10*time.Second, config.Timeout, "Default timeout should be 10 seconds")
	assert.False(t, config.InsecureSkipVerify, "InsecureSkipVerify should be false by default")
	assert.True(t, config.EnableHTTP2, "EnableHTTP2 should be true by default")
}

func TestNewClient(t *testing.T) {
	// Test with default config
	config := DefaultClientConfig()
	client, err := NewClient(config)

	require.NoError(t, err, "Creating client with default config should not fail")
	assert.NotNil(t, client, "Client should not be nil")
	assert.NotNil(t, client.client, "HTTP client should not be nil")

	// Test with custom config
	customConfig := ClientConfig{
		Timeout:            5 * time.Second,
		InsecureSkipVerify: true,
		EnableHTTP2:        false,
	}
	customClient, err := NewClient(customConfig)

	require.NoError(t, err, "Creating client with custom config should not fail")
	assert.NotNil(t, customClient, "Client should not be nil")
	assert.Equal(t, customConfig, customClient.config, "Client should store the provided config")
}

func TestRequest(t *testing.T) {
	// Create a mock response
	mockResp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString(`{"status":"success"}`)),
	}

	// Create a client with a mock transport
	httpClient := &http.Client{
		Transport: &mockRoundTripper{response: mockResp, err: nil},
	}

	client := &Client{
		client: httpClient,
		config: DefaultClientConfig(),
	}

	// Test successful request
	resp, err := client.Request(
		http.MethodGet,
		"https://example.com/api",
		[]byte(`{"key":"value"}`),
		map[string]string{"X-API-Key": "test-key"},
	)

	require.NoError(t, err, "Request should not return an error")
	assert.Equal(t, mockResp, resp, "Response should match mock response")

	// Test with default Content-Type
	resp, err = client.Request(
		http.MethodPost,
		"https://example.com/api",
		[]byte(`{"key":"value"}`),
		map[string]string{},
	)

	require.NoError(t, err, "Request with default Content-Type should not fail")
}

func TestReadResponseBody(t *testing.T) {
	// Create a response with a body
	expectedBody := `{"status":"success"}`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString(expectedBody)),
	}

	client := &Client{
		client: &http.Client{},
		config: DefaultClientConfig(),
	}

	// Test reading response body
	body, err := client.ReadResponseBody(resp)

	require.NoError(t, err, "ReadResponseBody should not return an error")
	assert.Equal(t, []byte(expectedBody), body, "Body should match expected content")

	// Test with nil response
	_, err = client.ReadResponseBody(nil)
	assert.Error(t, err, "ReadResponseBody should return an error for nil response")
}
