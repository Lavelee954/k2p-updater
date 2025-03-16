package usecase

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"k2p-updater/internal/features/backend/domain"
	"k2p-updater/internal/features/backend/domain/mocks"
)

func TestNewWebhookService(t *testing.T) {
	mockCredProvider := new(mocks.MockCredentialProvider)
	mockSecretProvider := new(mocks.MockSecretProvider)
	mockHTTPClient := new(mocks.MockHTTPClient)
	config := WebhookServiceConfig{
		BaseURLKey: "baseUrl",
	}

	// Test successful creation
	service := NewWebhookService(config, mockCredProvider, mockSecretProvider, mockHTTPClient)
	assert.NotNil(t, service, "WebhookService should not be nil")

	// Test with nil dependencies
	assert.Panics(t, func() {
		NewWebhookService(config, nil, mockSecretProvider, mockHTTPClient)
	}, "Should panic when credential provider is nil")

	assert.Panics(t, func() {
		NewWebhookService(config, mockCredProvider, nil, mockHTTPClient)
	}, "Should panic when secret provider is nil")

	assert.Panics(t, func() {
		NewWebhookService(config, mockCredProvider, mockSecretProvider, nil)
	}, "Should panic when HTTP client is nil")
}

func TestCallAPI(t *testing.T) {
	mockCredProvider := new(mocks.MockCredentialProvider)
	mockSecretProvider := new(mocks.MockSecretProvider)
	mockHTTPClient := new(mocks.MockHTTPClient)
	config := WebhookServiceConfig{
		BaseURLKey: "baseUrl",
	}

	service := &WebhookService{
		config:             config,
		credentialProvider: mockCredProvider,
		secretProvider:     mockSecretProvider,
		httpClient:         mockHTTPClient,
	}

	secretName := "test-secret"
	apiPath := "api/webhook"
	requestBody := map[string]string{"key": "value"}

	// Test successful API call without auth
	secretData := map[string]string{
		"baseUrl": "https://api.example.com",
	}
	mockSecretProvider.On("GetSecretData", secretName, []string{"baseUrl"}).
		Return(secretData, nil).Once()

	mockResp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"result":"success"}`)),
	}

	mockHTTPClient.On("Request",
		http.MethodPost,
		"https://api.example.com/api/webhook",
		mock.Anything,
		mock.Anything).Return(mockResp, nil).Once()

	resp, err := service.CallAPI(secretName, apiPath, requestBody, http.MethodPost, false)

	require.NoError(t, err, "CallAPI should succeed without auth")
	assert.Equal(t, mockResp, resp, "Response should match mock response")

	// Test successful API call with auth
	mockSecretProvider.On("GetSecretData", secretName, []string{"baseUrl"}).
		Return(secretData, nil).Once()

	tokenInfo := domain.TokenInfo{
		AccessToken: "test-token",
		Roles:       []string{"role1"},
		ExpiresAt:   time.Now().Add(1 * time.Hour),
	}
	mockCredProvider.On("GetToken").Return(tokenInfo, nil).Once()

	mockHTTPClient.On("Request",
		http.MethodPost,
		"https://api.example.com/api/webhook",
		mock.Anything,
		// Verify Authorization header is set
		mock.MatchedBy(func(headers map[string]string) bool {
			authHeader, exists := headers["Authorization"]
			return exists && authHeader == "Bearer test-token"
		})).Return(mockResp, nil).Once()

	resp, err = service.CallAPI(secretName, apiPath, requestBody, http.MethodPost, true)

	require.NoError(t, err, "CallAPI should succeed with auth")
	assert.Equal(t, mockResp, resp, "Response should match mock response")

	// Test with empty secret name
	_, err = service.CallAPI("", apiPath, requestBody, http.MethodPost, false)
	assert.Error(t, err, "CallAPI should fail with empty secret name")
	assert.Contains(t, err.Error(), "secret name cannot be empty", "Error should mention empty secret name")

	// Test with empty API path
	_, err = service.CallAPI(secretName, "", requestBody, http.MethodPost, false)
	assert.Error(t, err, "CallAPI should fail with empty API path")
	assert.Contains(t, err.Error(), "API path cannot be empty", "Error should mention empty API path")

	// Test error getting secret data
	mockSecretProvider.On("GetSecretData", secretName, []string{"baseUrl"}).
		Return(nil, assert.AnError).Once()

	_, err = service.CallAPI(secretName, apiPath, requestBody, http.MethodPost, false)
	assert.Error(t, err, "CallAPI should fail when secret not found")
	assert.Contains(t, err.Error(), "failed to get webhook URL", "Error should mention webhook URL failure")

	// Test missing base URL in secret
	mockSecretProvider.On("GetSecretData", secretName, []string{"baseUrl"}).
		Return(map[string]string{}, nil).Once()

	_, err = service.CallAPI(secretName, apiPath, requestBody, http.MethodPost, false)
	assert.Error(t, err, "CallAPI should fail when baseUrl not in secret")
	assert.Contains(t, err.Error(), "webhook URL key", "Error should mention missing key")

	// Test error getting token
	mockSecretProvider.On("GetSecretData", secretName, []string{"baseUrl"}).
		Return(secretData, nil).Once()

	mockCredProvider.On("GetToken").Return(domain.TokenInfo{}, assert.AnError).Once()

	_, err = service.CallAPI(secretName, apiPath, requestBody, http.MethodPost, true)
	assert.Error(t, err, "CallAPI should fail when token retrieval fails")
	assert.Contains(t, err.Error(), "failed to get auth token", "Error should mention token retrieval")

	mockSecretProvider.AssertExpectations(t)
	mockCredProvider.AssertExpectations(t)
	mockHTTPClient.AssertExpectations(t)
}

func TestCallAPIAndParseResponse(t *testing.T) {
	mockCredProvider := new(mocks.MockCredentialProvider)
	mockSecretProvider := new(mocks.MockSecretProvider)
	mockHTTPClient := new(mocks.MockHTTPClient)
	config := WebhookServiceConfig{
		BaseURLKey: "baseUrl",
	}

	service := &WebhookService{
		config:             config,
		credentialProvider: mockCredProvider,
		secretProvider:     mockSecretProvider,
		httpClient:         mockHTTPClient,
	}

	secretName := "test-secret"
	apiPath := "api/webhook"
	requestBody := map[string]string{"key": "value"}

	// Test successful call and parse
	secretData := map[string]string{
		"baseUrl": "https://api.example.com",
	}
	mockSecretProvider.On("GetSecretData", secretName, []string{"baseUrl"}).
		Return(secretData, nil).Once()

	mockResp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"name":"test","value":123}`)),
	}

	mockHTTPClient.On("Request",
		http.MethodGet,
		"https://api.example.com/api/webhook",
		mock.Anything,
		mock.Anything).Return(mockResp, nil).Once()

	mockHTTPClient.On("ReadResponseBody", mockResp).
		Return([]byte(`{"name":"test","value":123}`), nil).Once()

	// Parse into this struct
	var result struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	err := service.CallAPIAndParseResponse(secretName, apiPath, requestBody, http.MethodGet, false, &result)

	require.NoError(t, err, "CallAPIAndParseResponse should succeed")
	assert.Equal(t, "test", result.Name, "Name should be parsed correctly")
	assert.Equal(t, 123, result.Value, "Value should be parsed correctly")

	// Test nil result
	err = service.CallAPIAndParseResponse(secretName, apiPath, requestBody, http.MethodGet, false, nil)
	assert.Error(t, err, "CallAPIAndParseResponse should fail with nil result")
	assert.Contains(t, err.Error(), "result cannot be nil", "Error should mention nil result")

	// Test non-success response
	mockSecretProvider.On("GetSecretData", secretName, []string{"baseUrl"}).
		Return(secretData, nil).Once()

	errorResp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Body:       io.NopCloser(strings.NewReader(`{"error":"invalid_request"}`)),
	}

	mockHTTPClient.On("Request",
		http.MethodGet,
		"https://api.example.com/api/webhook",
		mock.Anything,
		mock.Anything).Return(errorResp, nil).Once()

	mockHTTPClient.On("ReadResponseBody", errorResp).
		Return([]byte(`{"error":"invalid_request"}`), nil).Once()

	err = service.CallAPIAndParseResponse(secretName, apiPath, requestBody, http.MethodGet, false, &result)
	assert.Error(t, err, "CallAPIAndParseResponse should fail with non-success status")
	assert.Contains(t, err.Error(), "non-success status", "Error should mention non-success status")

	// Test JSON parse error
	mockSecretProvider.On("GetSecretData", secretName, []string{"baseUrl"}).
		Return(secretData, nil).Once()

	invalidJSONResp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"name":"test",invalid_json`)),
	}

	mockHTTPClient.On("Request",
		http.MethodGet,
		"https://api.example.com/api/webhook",
		mock.Anything,
		mock.Anything).Return(invalidJSONResp, nil).Once()

	mockHTTPClient.On("ReadResponseBody", invalidJSONResp).
		Return([]byte(`{"name":"test",invalid_json`), nil).Once()

	err = service.CallAPIAndParseResponse(secretName, apiPath, requestBody, http.MethodGet, false, &result)
	assert.Error(t, err, "CallAPIAndParseResponse should fail with invalid JSON")
	assert.Contains(t, err.Error(), "failed to parse response body", "Error should mention parse failure")

	mockSecretProvider.AssertExpectations(t)
	mockCredProvider.AssertExpectations(t)
	mockHTTPClient.AssertExpectations(t)
}
