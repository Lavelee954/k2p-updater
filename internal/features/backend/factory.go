package backend

import (
	"fmt"
	"k2p-updater/internal/features/backend/adapter/http"
	ks "k2p-updater/internal/features/backend/adapter/kubernetes"
	"k2p-updater/internal/features/backend/domain"
	"k2p-updater/internal/features/backend/usecase"
	"k8s.io/client-go/kubernetes"
)

// Config holds the configuration for the backend package
type Config struct {
	Namespace          string
	SecretName         string
	APIPath            string
	SourceComponent    string
	BaseURLKey         string
	RequiredSecretKeys []string
}

// Services contains all the services provided by the backend package
type Services struct {
	CredentialProvider domain.CredentialProvider
	WebhookProvider    domain.WebhookProvider
}

// NewBackendServices creates and initializes all backend services
func NewBackendServices(clientset kubernetes.Interface, config Config) (*Services, error) {
	if clientset == nil {
		return nil, fmt.Errorf("kubernetes client cannot be nil")
	}

	// Create HTTP client
	httpClientConfig := http.DefaultClientConfig()
	httpClient, err := http.NewClient(httpClientConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP client: %w", err)
	}

	// Create secret provider
	secretProvider := ks.NewSecretProvider(clientset, config.Namespace)

	// Prepare required keys with a sensible default if not provided
	requiredKeys := config.RequiredSecretKeys
	if len(requiredKeys) == 0 {
		requiredKeys = []string{"clusterUuid", "accessKey", "secretKey", "clusterName", config.BaseURLKey}
	}

	// Create credential service
	credConfig := usecase.CredentialServiceConfig{
		SecretName:      config.SecretName,
		APIPath:         config.APIPath,
		SourceComponent: config.SourceComponent,
		BaseURLKey:      config.BaseURLKey,
		RequiredKeys:    requiredKeys,
	}

	credService := usecase.NewCredentialService(credConfig, secretProvider, httpClient)

	// Create webhook service
	webhookConfig := usecase.WebhookServiceConfig{
		BaseURLKey: config.BaseURLKey,
	}

	webhookService := usecase.NewWebhookService(webhookConfig, credService, secretProvider, httpClient)

	return &Services{
		CredentialProvider: credService,
		WebhookProvider:    webhookService,
	}, nil
}
