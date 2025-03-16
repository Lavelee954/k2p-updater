package usecase

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"k2p-updater/internal/features/backend/domain"
)

// WebhookServiceConfig holds the configuration for the webhook service
type WebhookServiceConfig struct {
	BaseURLKey string
}

// WebhookService provides functionality for calling webhook APIs
type WebhookService struct {
	config             WebhookServiceConfig
	credentialProvider domain.CredentialProvider
	secretProvider     domain.SecretProvider
	httpClient         domain.HTTPClientInterface
}

// NewWebhookService creates a new webhook service
func NewWebhookService(
	config WebhookServiceConfig,
	credentialProvider domain.CredentialProvider,
	secretProvider domain.SecretProvider,
	httpClient domain.HTTPClientInterface,
) domain.WebhookProvider {
	if credentialProvider == nil {
		panic("credential provider cannot be nil")
	}
	if secretProvider == nil {
		panic("secret provider cannot be nil")
	}
	if httpClient == nil {
		panic("HTTP client cannot be nil")
	}

	return &WebhookService{
		config:             config,
		credentialProvider: credentialProvider,
		secretProvider:     secretProvider,
		httpClient:         httpClient,
	}
}

// CallAPI makes a webhook API call
func (s *WebhookService) CallAPI(
	secretName string,
	apiPath string,
	requestBody interface{},
	method string,
	requireAuth bool,
) (*http.Response, error) {
	if secretName == "" {
		return nil, fmt.Errorf("secret name cannot be empty")
	}
	if apiPath == "" {
		return nil, fmt.Errorf("API path cannot be empty")
	}

	// Get base URL from secrets
	secretData, err := s.secretProvider.GetSecretData(secretName, []string{s.config.BaseURLKey})
	if err != nil {
		return nil, fmt.Errorf("failed to get webhook URL from secret: %w", err)
	}

	baseURL, exists := secretData[s.config.BaseURLKey]
	if !exists {
		return nil, fmt.Errorf("webhook URL key '%s' not found in secret", s.config.BaseURLKey)
	}

	// Build full URL
	webhookURL, err := url.JoinPath(strings.TrimSpace(baseURL), strings.TrimSpace(apiPath))
	if err != nil {
		return nil, fmt.Errorf("failed to build webhook URL: %w", err)
	}

	// Marshal request body if provided
	var bodyBytes []byte
	if requestBody != nil {
		bodyBytes, err = json.Marshal(requestBody)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
	}

	// Set up headers
	headers := map[string]string{
		"Content-Type": "application/json",
	}

	// Add auth token if required
	if requireAuth {
		tokenInfo, err := s.credentialProvider.GetToken()
		if err != nil {
			return nil, fmt.Errorf("failed to get auth token: %w", err)
		}
		headers["Authorization"] = "Bearer " + tokenInfo.AccessToken
	}

	// Make the request
	resp, err := s.httpClient.Request(method, webhookURL, bodyBytes, headers)
	if err != nil {
		return nil, fmt.Errorf("webhook request failed: %w", err)
	}

	return resp, nil
}

// CallAPIAndParseResponse calls an API and parses the response into the provided result struct
func (s *WebhookService) CallAPIAndParseResponse(
	secretName string,
	apiPath string,
	requestBody interface{},
	method string,
	requireAuth bool,
	result interface{},
) error {
	if result == nil {
		return fmt.Errorf("result cannot be nil")
	}

	resp, err := s.CallAPI(secretName, apiPath, requestBody, method, requireAuth)
	if err != nil {
		return err
	}

	// Check response status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := s.httpClient.ReadResponseBody(resp)
		return fmt.Errorf(
			"webhook API returned non-success status: %d, body: %s",
			resp.StatusCode,
			string(body),
		)
	}

	// Parse response into result
	body, err := s.httpClient.ReadResponseBody(resp)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if err := json.Unmarshal(body, result); err != nil {
		return fmt.Errorf("failed to parse response body: %w", err)
	}

	return nil
}
