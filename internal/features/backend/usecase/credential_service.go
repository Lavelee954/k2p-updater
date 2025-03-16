package usecase

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"k2p-updater/internal/features/backend/domain"
)

// CredentialServiceConfig holds the configuration for the credential service
type CredentialServiceConfig struct {
	SecretName      string
	APIPath         string
	SourceComponent string
	BaseURLKey      string
	RequiredKeys    []string
}

// CredentialService implements domain.CredentialProvider
type CredentialService struct {
	config         CredentialServiceConfig
	secretProvider domain.SecretProvider
	httpClient     domain.HTTPClientInterface
	tokenInfo      domain.TokenInfo
	mutex          sync.RWMutex
}

// NewCredentialService creates a new credential service
func NewCredentialService(
	config CredentialServiceConfig,
	secretProvider domain.SecretProvider,
	httpClient domain.HTTPClientInterface,
) domain.CredentialProvider {
	if secretProvider == nil {
		panic("secret provider cannot be nil")
	}
	if httpClient == nil {
		panic("HTTP client cannot be nil")
	}

	return &CredentialService{
		config:         config,
		secretProvider: secretProvider,
		httpClient:     httpClient,
	}
}

// GetToken returns a valid access token, refreshing if necessary
func (s *CredentialService) GetToken() (domain.TokenInfo, error) {
	s.mutex.RLock()
	token := s.tokenInfo
	s.mutex.RUnlock()

	// Check if token exists and is valid
	if token.AccessToken != "" && time.Now().Before(token.ExpiresAt) {
		return token, nil
	}

	// Token doesn't exist or is expired, get a new one
	return s.RefreshToken()
}

// RefreshToken forces a token refresh
func (s *CredentialService) RefreshToken() (domain.TokenInfo, error) {
	// Get credential data from secret
	secretData, err := s.secretProvider.GetSecretData(s.config.SecretName, s.config.RequiredKeys)
	if err != nil {
		return domain.TokenInfo{}, fmt.Errorf("failed to get credential secret data: %w", err)
	}

	// Check if secretData is nil as a defensive measure
	if secretData == nil {
		return domain.TokenInfo{}, fmt.Errorf("no data returned from secret")
	}

	// Validate required keys
	for _, key := range s.config.RequiredKeys {
		if _, exists := secretData[key]; !exists {
			return domain.TokenInfo{}, fmt.Errorf("required key '%s' missing in secret", key)
		}
	}

	// Create request payload
	requestData := domain.CredentialRequest{
		ClusterUUID: secretData["clusterUuid"],
		AccessKey:   secretData["accessKey"],
		SecretKey:   secretData["secretKey"],
		ClusterName: secretData["clusterName"],
		Operator:    s.config.SourceComponent,
	}

	// Build API URL
	baseURL := strings.TrimSpace(secretData[s.config.BaseURLKey])
	apiPath := strings.TrimSpace(s.config.APIPath)
	apiURL, err := url.JoinPath(baseURL, apiPath)
	if err != nil {
		return domain.TokenInfo{}, fmt.Errorf("failed to build API URL: %w", err)
	}

	// Marshal request body
	requestBody, err := json.Marshal(requestData)
	if err != nil {
		return domain.TokenInfo{}, fmt.Errorf("failed to marshal request body: %w", err)
	}

	// Make API request
	resp, err := s.httpClient.Request(
		http.MethodPost,
		apiURL,
		requestBody,
		map[string]string{"Content-Type": "application/json"},
	)
	if err != nil {
		return domain.TokenInfo{}, fmt.Errorf("failed to send credential request: %w", err)
	}

	// Check response status
	if resp.StatusCode != http.StatusOK {
		body, _ := s.httpClient.ReadResponseBody(resp)
		return domain.TokenInfo{}, fmt.Errorf(
			"credential API returned non-200 status: %d, body: %s",
			resp.StatusCode,
			string(body),
		)
	}

	// Parse response
	var credResponse domain.CredentialResponse
	body, err := s.httpClient.ReadResponseBody(resp)
	if err != nil {
		return domain.TokenInfo{}, fmt.Errorf("failed to read response body: %w", err)
	}

	if err := json.Unmarshal(body, &credResponse); err != nil {
		return domain.TokenInfo{}, fmt.Errorf("failed to parse credential response: %w", err)
	}

	// Create token info with proper expiration time
	expiresAt := time.Now().Add(time.Duration(credResponse.ExpirationDateInMs) * time.Millisecond)
	tokenInfo := domain.TokenInfo{
		AccessToken: credResponse.AccessToken,
		Roles:       credResponse.Roles,
		ExpiresAt:   expiresAt,
	}

	// Store token info
	s.mutex.Lock()
	s.tokenInfo = tokenInfo
	s.mutex.Unlock()

	return tokenInfo, nil
}
