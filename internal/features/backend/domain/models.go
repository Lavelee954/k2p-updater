package domain

import "time"

// TokenInfo contains authentication token data
type TokenInfo struct {
	AccessToken string
	Roles       []string
	ExpiresAt   time.Time
}

// CredentialRequest represents a request for authentication credentials
type CredentialRequest struct {
	ClusterUUID string `json:"clusterUuid"`
	AccessKey   string `json:"accessKey"`
	SecretKey   string `json:"secretKey"`
	ClusterName string `json:"clusterName"`
	Operator    string `json:"operator"`
}

// CredentialResponse represents the response from an authentication request
type CredentialResponse struct {
	AccessToken        string   `json:"accessToken"`
	Roles              []string `json:"roles"`
	ExpirationDateInMs int64    `json:"expirationDateInMs"`
}
