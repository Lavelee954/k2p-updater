package resource

import (
	"context"
	"testing"

	"k2p-updater/cmd/app"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"
	kfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

// setupManagerTest creates a test environment for Manager tests
func setupManagerTest() (*Manager, *app.Config) {
	// Create a test config
	config := &app.Config{
		Kubernetes: app.KubernetesConfig{
			Namespace:  "default",
			ConfigPath: "",
			MasterURL:  "",
		},
		Resources: app.ResourcesConfig{
			Namespace: "default",
			Group:     "k2p.cloud.kt.com",
			Version:   "v1beta1",
			Definitions: map[string]app.ResourceDefinitionConfig{
				"updater": {
					Resource:    "k2pupdaters",
					NameFormat:  "k2pupdater-%s",
					StatusField: "updates",
					Kind:        "K2pUpdater",
				},
				"upgrader": {
					Resource:    "k2pupgraders",
					NameFormat:  "k2pupgrader-%s",
					StatusField: "upgradeStatus",
					Kind:        "K2pUpgrader",
				},
			},
		},
	}

	// Create a scheme
	s := runtime.NewScheme()

	// Create fake clients
	fakeKubeClient := kfake.NewSimpleClientset()
	fakeDynamicClient := fake.NewSimpleDynamicClient(s)

	// Create the KubeClients with our fake clients
	clients := &app.KubeClients{
		ClientSet:     fakeKubeClient,
		DynamicClient: fakeDynamicClient,
		Config:        &rest.Config{},
	}

	// Create the test template
	template, _ := buildTemplateFromConfig(config)

	// Create a manager with our mocks
	manager := &Manager{
		Templates:  template,
		KubeClient: clients,
		Config:     config,
		Event:      &EventInfo{Template: template, KubeClient: fakeKubeClient},
		Status:     &StatusInfo{Template: template, DynamicClient: fakeDynamicClient},
	}

	return manager, config
}

func TestGetResource(t *testing.T) {
	// Create a test environment
	manager, _ := setupManagerTest()
	ctx := context.Background()

	// Test cases
	testCases := []struct {
		name         string
		resourceKey  string
		resourceName string
		expectError  bool
		errorMessage string
	}{
		{
			name:         "Invalid resource key",
			resourceKey:  "nonexistent",
			resourceName: "test",
			expectError:  true,
			errorMessage: "resource with key nonexistent not found",
		},
		// The "Valid resource" case would require more setup to mock the response
	}

	// Run the tests
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Call the function being tested
			_, err := manager.GetResource(ctx, tc.resourceKey, tc.resourceName)

			// Check error expectations
			if tc.expectError {
				if err == nil {
					t.Errorf("Expected error but got nil")
				} else if err.Error() != tc.errorMessage {
					t.Errorf("Expected error message '%s', but got '%s'", tc.errorMessage, err.Error())
				}
			}
		})
	}
}
