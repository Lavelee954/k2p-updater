package mocks

import (
	"net/http"

	"github.com/stretchr/testify/mock"
	"k2p-updater/internal/features/backend/domain"
)

// MockHTTPClient is a mock implementation of domain.HTTPClientInterface
type MockHTTPClient struct {
	mock.Mock
}

// Request mocks the Request method
func (m *MockHTTPClient) Request(method, url string, body []byte, headers map[string]string) (*http.Response, error) {
	args := m.Called(method, url, body, headers)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*http.Response), args.Error(1)
}

// ReadResponseBody mocks the ReadResponseBody method
func (m *MockHTTPClient) ReadResponseBody(resp *http.Response) ([]byte, error) {
	args := m.Called(resp)
	return args.Get(0).([]byte), args.Error(1)
}

// MockSecretProvider is a mock implementation of domain.SecretProvider
type MockSecretProvider struct {
	mock.Mock
}

// GetSecretData mocks the GetSecretData method
func (m *MockSecretProvider) GetSecretData(secretName string, keys []string) (map[string]string, error) {
	args := m.Called(secretName, keys)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]string), args.Error(1)
}

// MockCredentialProvider is a mock implementation of domain.CredentialProvider
type MockCredentialProvider struct {
	mock.Mock
}

// GetToken mocks the GetToken method
func (m *MockCredentialProvider) GetToken() (domain.TokenInfo, error) {
	args := m.Called()
	return args.Get(0).(domain.TokenInfo), args.Error(1)
}

// RefreshToken mocks the RefreshToken method
func (m *MockCredentialProvider) RefreshToken() (domain.TokenInfo, error) {
	args := m.Called()
	return args.Get(0).(domain.TokenInfo), args.Error(1)
}

// MockWebhookProvider is a mock implementation of domain.WebhookProvider
type MockWebhookProvider struct {
	mock.Mock
}

// CallAPI mocks the CallAPI method
func (m *MockWebhookProvider) CallAPI(secretName, apiPath string, requestBody interface{}, method string, requireAuth bool) (*http.Response, error) {
	args := m.Called(secretName, apiPath, requestBody, method, requireAuth)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*http.Response), args.Error(1)
}

// CallAPIAndParseResponse mocks the CallAPIAndParseResponse method
func (m *MockWebhookProvider) CallAPIAndParseResponse(secretName, apiPath string, requestBody interface{}, method string, requireAuth bool, result interface{}) error {
	args := m.Called(secretName, apiPath, requestBody, method, requireAuth, result)
	return args.Error(0)
}
