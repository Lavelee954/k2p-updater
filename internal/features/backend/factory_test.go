package backend

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/kubernetes/fake"
)

func TestNewBackendServices(t *testing.T) {
	clientset := fake.NewSimpleClientset()

	// Test with minimal config
	config := Config{
		Namespace:       "test-namespace",
		SecretName:      "test-secret",
		APIPath:         "auth/token",
		SourceComponent: "test-component",
		BaseURLKey:      "baseUrl",
	}

	services, err := NewBackendServices(clientset, config)
	require.NoError(t, err, "NewBackendServices should not fail with valid config")
	assert.NotNil(t, services, "Services should not be nil")
	assert.NotNil(t, services.CredentialProvider, "CredentialProvider should not be nil")
	assert.NotNil(t, services.WebhookProvider, "WebhookProvider should not be nil")

	// Test with custom secret keys
	configWithKeys := Config{
		Namespace:          "test-namespace",
		SecretName:         "test-secret",
		APIPath:            "auth/token",
		SourceComponent:    "test-component",
		BaseURLKey:         "baseUrl",
		RequiredSecretKeys: []string{"customKey1", "customKey2"},
	}

	services, err = NewBackendServices(clientset, configWithKeys)
	require.NoError(t, err, "NewBackendServices should not fail with custom secret keys")
	assert.NotNil(t, services, "Services should not be nil")

	// Test with nil client
	_, err = NewBackendServices(nil, config)
	assert.Error(t, err, "NewBackendServices should fail with nil clientset")
	assert.Contains(t, err.Error(), "cannot be nil", "Error should mention nil client")
}
