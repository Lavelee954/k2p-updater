package kubernetes

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestNewSecretProvider(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	namespace := "test-namespace"

	provider := NewSecretProvider(clientset, namespace)

	assert.NotNil(t, provider, "SecretProvider should not be nil")
}

func TestGetSecretData(t *testing.T) {
	// Create a fake clientset and a test secret
	secretName := "test-secret"
	namespace := "test-namespace"
	keys := []string{"key1", "key2", "missing-key"}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"key1": []byte("value1"),
			"key2": []byte("value2"),
		},
	}

	clientset := fake.NewSimpleClientset(secret)
	provider := NewSecretProvider(clientset, namespace)

	// Test getting existing keys
	data, err := provider.GetSecretData(secretName, keys)

	require.NoError(t, err, "GetSecretData should not return an error for existing secret")
	assert.Equal(t, "value1", data["key1"], "Value for key1 should match")
	assert.Equal(t, "value2", data["key2"], "Value for key2 should match")
	assert.Empty(t, data["missing-key"], "Value for missing-key should be empty")

	// Test with empty secret name
	_, err = provider.GetSecretData("", keys)
	assert.Error(t, err, "GetSecretData should return an error for empty secret name")

	// Test with non-existent secret
	_, err = provider.GetSecretData("non-existent-secret", keys)
	assert.Error(t, err, "GetSecretData should return an error for non-existent secret")
}
