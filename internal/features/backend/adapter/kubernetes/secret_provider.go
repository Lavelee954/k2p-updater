package kubernetes

import (
	"context"
	"fmt"
	"k2p-updater/internal/features/backend/domain"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// SecretProvider implements domain.SecretProvider using Kubernetes secrets
type SecretProvider struct {
	client    kubernetes.Interface
	namespace string
}

// NewSecretProvider creates a new Kubernetes secret provider
func NewSecretProvider(clientset kubernetes.Interface, namespace string) domain.SecretProvider {
	return &SecretProvider{
		client:    clientset,
		namespace: namespace,
	}
}

// GetSecretData retrieves specified keys from a Kubernetes secret
func (p *SecretProvider) GetSecretData(secretName string, keys []string) (map[string]string, error) {
	if secretName == "" {
		return nil, fmt.Errorf("secret name cannot be empty")
	}

	ctx := context.Background()
	secret, err := p.client.CoreV1().Secrets(p.namespace).Get(
		ctx,
		secretName,
		metav1.GetOptions{},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve secret %s: %w", secretName, err)
	}

	selectedData := make(map[string]string)
	for _, key := range keys {
		if value, exists := secret.Data[key]; exists {
			selectedData[key] = string(value)
		}
	}

	return selectedData, nil
}
