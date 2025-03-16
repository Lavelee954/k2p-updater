package domain

import "net/http"

// CredentialProvider defines the interface for obtaining authentication credentials
type CredentialProvider interface {
	// GetToken retrieves a valid access token
	GetToken() (TokenInfo, error)

	// RefreshToken forces a token refresh
	RefreshToken() (TokenInfo, error)
}

// SecretProvider defines the interface for retrieving secrets
type SecretProvider interface {
	// GetSecretData retrieves specific keys from a secret
	GetSecretData(secretName string, keys []string) (map[string]string, error)
}

// HTTPClientInterface defines the contract for HTTP clients
type HTTPClientInterface interface {
	// Request makes an HTTP request with the specified method, URL, body, and headers
	Request(method, url string, body []byte, headers map[string]string) (*http.Response, error)

	// ReadResponseBody reads and closes the response body
	ReadResponseBody(resp *http.Response) ([]byte, error)
}

// WebhookProvider defines the interface for making webhook API calls
type WebhookProvider interface {
	// CallAPI makes a webhook API call and returns the raw response
	CallAPI(
		secretName string,
		apiPath string,
		requestBody interface{},
		method string,
		requireAuth bool,
	) (*http.Response, error)

	// CallAPIAndParseResponse calls an API and parses the response into the provided result struct
	CallAPIAndParseResponse(
		secretName string,
		apiPath string,
		requestBody interface{},
		method string,
		requireAuth bool,
		result interface{},
	) error
}
