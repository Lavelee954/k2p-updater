package usecase

import (
	"fmt"
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

func TestNewCredentialService(t *testing.T) {
	mockSecretProvider := new(mocks.MockSecretProvider)
	mockHTTPClient := new(mocks.MockHTTPClient)
	config := CredentialServiceConfig{
		SecretName:      "test-secret",
		APIPath:         "auth/token",
		SourceComponent: "test-component",
		BaseURLKey:      "baseUrl",
		RequiredKeys:    []string{"clusterUuid", "accessKey", "secretKey", "clusterName", "baseUrl"},
	}

	// Test successful creation
	service := NewCredentialService(config, mockSecretProvider, mockHTTPClient)
	assert.NotNil(t, service, "CredentialService should not be nil")

	// Test with nil dependencies
	assert.Panics(t, func() {
		NewCredentialService(config, nil, mockHTTPClient)
	}, "Should panic when secret provider is nil")

	assert.Panics(t, func() {
		NewCredentialService(config, mockSecretProvider, nil)
	}, "Should panic when HTTP client is nil")
}

func TestGetToken(t *testing.T) {
	mockSecretProvider := new(mocks.MockSecretProvider)
	mockHTTPClient := new(mocks.MockHTTPClient)
	config := CredentialServiceConfig{
		SecretName:      "test-secret",
		APIPath:         "auth/token",
		SourceComponent: "test-component",
		BaseURLKey:      "baseUrl",
		RequiredKeys:    []string{"clusterUuid", "accessKey", "secretKey", "clusterName", "baseUrl"},
	}

	service := &CredentialService{
		config:         config,
		secretProvider: mockSecretProvider,
		httpClient:     mockHTTPClient,
	}

	// Test with valid, non-expired token already set
	validToken := domain.TokenInfo{
		AccessToken: "valid-token",
		Roles:       []string{"role1", "role2"},
		ExpiresAt:   time.Now().Add(1 * time.Hour),
	}
	service.tokenInfo = validToken

	token, err := service.GetToken()
	require.NoError(t, err, "GetToken should not fail with valid token")
	assert.Equal(t, validToken, token, "Should return the existing valid token")

	// Test with expired token (should call RefreshToken)
	expiredToken := domain.TokenInfo{
		AccessToken: "expired-token",
		Roles:       []string{"role1", "role2"},
		ExpiresAt:   time.Now().Add(-1 * time.Hour),
	}
	service.tokenInfo = expiredToken

	// Mock the secret provider and HTTP client for token refresh
	secretData := map[string]string{
		"clusterUuid": "test-uuid",
		"accessKey":   "test-access-key",
		"secretKey":   "test-secret-key",
		"clusterName": "test-cluster",
		"baseUrl":     "https://api.example.com",
	}
	mockSecretProvider.On("GetSecretData", config.SecretName, config.RequiredKeys).Return(secretData, nil)

	responseBody := `{
		"accessToken": "new-token",
		"roles": ["role1", "role2"],
		"expirationDateInMs": 3600000
	}`
	mockResp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(responseBody)),
	}

	mockHTTPClient.On("Request",
		http.MethodPost,
		"https://api.example.com/auth/token",
		mock.Anything,
		mock.Anything).Return(mockResp, nil)

	mockHTTPClient.On("ReadResponseBody", mockResp).Return([]byte(responseBody), nil)

	token, err = service.GetToken()
	require.NoError(t, err, "GetToken should refresh when token is expired")
	assert.Equal(t, "new-token", token.AccessToken, "Should return a new token after refresh")
	assert.Equal(t, []string{"role1", "role2"}, token.Roles, "Should have the correct roles")
	assert.True(t, token.ExpiresAt.After(time.Now()), "New token should expire in the future")

	mockSecretProvider.AssertExpectations(t)
	mockHTTPClient.AssertExpectations(t)
}

func TestRefreshToken(t *testing.T) {
	// Create mocks
	mockSecretProvider := new(mocks.MockSecretProvider)
	mockHTTPClient := new(mocks.MockHTTPClient)

	// Set up configuration
	config := CredentialServiceConfig{
		SecretName:      "test-secret",
		APIPath:         "auth/token",
		SourceComponent: "test-component",
		BaseURLKey:      "baseUrl",
		RequiredKeys:    []string{"clusterUuid", "accessKey", "secretKey", "clusterName", "baseUrl"},
	}

	// Create service with dependencies
	service := &CredentialService{
		config:         config,
		secretProvider: mockSecretProvider,
		httpClient:     mockHTTPClient,
	}

	// Test 1: Secret provider returns an error
	mockSecretProvider.On("GetSecretData", config.SecretName, config.RequiredKeys).
		Return(map[string]string{}, fmt.Errorf("secret not found")).Once()

	token, err := service.RefreshToken()

	assert.Error(t, err, "RefreshToken should fail when secret provider returns an error")
	assert.Contains(t, err.Error(), "failed to get credential secret data")
	assert.Empty(t, token.AccessToken)

	// Test 2: Secret missing required key
	incompleteData := map[string]string{
		"clusterUuid": "test-uuid",
		"accessKey":   "test-access-key",
		// Missing secretKey
		"clusterName": "test-cluster",
		"baseUrl":     "https://api.example.com",
	}

	mockSecretProvider.On("GetSecretData", config.SecretName, config.RequiredKeys).
		Return(incompleteData, nil).Once()

	token, err = service.RefreshToken()

	assert.Error(t, err, "RefreshToken should fail with missing required key")
	assert.Contains(t, err.Error(), "missing in secret")
	assert.Empty(t, token.AccessToken)

	// Test 3: HTTP request fails
	completeData := map[string]string{
		"clusterUuid": "test-uuid",
		"accessKey":   "test-access-key",
		"secretKey":   "test-secret-key",
		"clusterName": "test-cluster",
		"baseUrl":     "https://api.example.com",
	}

	mockSecretProvider.On("GetSecretData", config.SecretName, config.RequiredKeys).
		Return(completeData, nil).Once()

	mockHTTPClient.On("Request",
		http.MethodPost,
		"https://api.example.com/auth/token",
		mock.Anything,
		mock.Anything).Return(nil, fmt.Errorf("connection error")).Once()

	token, err = service.RefreshToken()

	assert.Error(t, err, "RefreshToken should fail when HTTP request fails")
	assert.Contains(t, err.Error(), "failed to send credential request")
	assert.Empty(t, token.AccessToken)

	// Test 4: Non-200 HTTP response
	mockSecretProvider.On("GetSecretData", config.SecretName, config.RequiredKeys).
		Return(completeData, nil).Once()

	errorResp := &http.Response{
		StatusCode: http.StatusUnauthorized,
		Body:       io.NopCloser(strings.NewReader(`{"error":"invalid_credentials"}`)),
	}

	mockHTTPClient.On("Request",
		http.MethodPost,
		"https://api.example.com/auth/token",
		mock.Anything,
		mock.Anything).Return(errorResp, nil).Once()

	mockHTTPClient.On("ReadResponseBody", errorResp).
		Return([]byte(`{"error":"invalid_credentials"}`), nil).Once()

	token, err = service.RefreshToken()

	assert.Error(t, err, "RefreshToken should fail with non-200 response")
	assert.Contains(t, err.Error(), "non-200 status")
	assert.Empty(t, token.AccessToken)

	// Test 5: Successful token refresh
	mockSecretProvider.On("GetSecretData", config.SecretName, config.RequiredKeys).
		Return(completeData, nil).Once()

	successResp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"accessToken":"new-token","roles":["role1"],"expirationDateInMs":3600000}`)),
	}

	mockHTTPClient.On("Request",
		http.MethodPost,
		"https://api.example.com/auth/token",
		mock.Anything,
		mock.Anything).Return(successResp, nil).Once()

	mockHTTPClient.On("ReadResponseBody", successResp).
		Return([]byte(`{"accessToken":"new-token","roles":["role1"],"expirationDateInMs":3600000}`), nil).Once()

	token, err = service.RefreshToken()

	assert.NoError(t, err, "RefreshToken should succeed with valid response")
	assert.Equal(t, "new-token", token.AccessToken)
	assert.Equal(t, []string{"role1"}, token.Roles)

	// Verify that all expected mock calls were made
	mockSecretProvider.AssertExpectations(t)
	mockHTTPClient.AssertExpectations(t)
}
