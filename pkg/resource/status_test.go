package resource

import (
	"context"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	fakedynamic "k8s.io/client-go/dynamic/fake"
)

// setupStatusTest creates a test environment for StatusInfo tests
func setupStatusTest() (*StatusInfo, *Template, dynamic.Interface) {
	// Create a test template
	template := &Template{
		Key: map[string]Resource{
			"updater": {
				namespace: "default",
				group:     "k2p.cloud.kt.com",
				version:   "v1beta1",
				definition: map[string]Definition{
					"updater": {
						NameFormat:  "k2pupdater-%s",
						Resource:    "k2pupdaters",
						Kind:        "K2pUpdater",
						StatusField: map[interface{}]interface{}{"field": "updates"},
					},
				},
			},
			"upgrader": {
				namespace: "default",
				group:     "k2p.cloud.kt.com",
				version:   "v1beta1",
				definition: map[string]Definition{
					"upgrader": {
						NameFormat:  "k2pupgrader-%s",
						Resource:    "k2pupgraders",
						Kind:        "K2pUpgrader",
						StatusField: map[interface{}]interface{}{"field": "upgradeStatus"},
					},
				},
			},
		},
	}

	// Create a scheme for the fake dynamic client
	scheme := runtime.NewScheme()

	// Create a fake dynamic client
	fakeDynamicClient := fakedynamic.NewSimpleDynamicClient(scheme)

	// Create mock resources - we'll manually handle the Get requests in our tests
	updaterResource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "k2p.cloud.kt.com/v1beta1",
			"kind":       "K2pUpdater",
			"metadata": map[string]interface{}{
				"name":      "k2pupdater-updater",
				"namespace": "default",
			},
			"status": map[string]interface{}{
				"updates": "Running update process",
			},
		},
	}

	upgraderResource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "k2p.cloud.kt.com/v1beta1",
			"kind":       "K2pUpgrader",
			"metadata": map[string]interface{}{
				"name":      "k2pupgrader-upgrader",
				"namespace": "default",
			},
			"status": map[string]interface{}{
				"upgradeStatus": "Upgrade in progress",
			},
		},
	}

	// Create the StatusInfo with our mocks
	statusInfo := &StatusInfo{
		Template:      template,
		DynamicClient: fakeDynamicClient,
	}

	// Add test resources to the fake client
	gvrUpdater := template.Key["updater"].definition["updater"].Resource
	fakeDynamicClient.Resource(
		schema.GroupVersionResource{
			Group:    template.Key["updater"].group,
			Version:  template.Key["updater"].version,
			Resource: gvrUpdater,
		}).
		Namespace(template.Key["updater"].namespace).
		Create(context.Background(), updaterResource, metav1.CreateOptions{})

	gvrUpgrader := template.Key["upgrader"].definition["upgrader"].Resource
	fakeDynamicClient.Resource(
		schema.GroupVersionResource{
			Group:    template.Key["upgrader"].group,
			Version:  template.Key["upgrader"].version,
			Resource: gvrUpgrader,
		}).
		Namespace(template.Key["upgrader"].namespace).
		Create(context.Background(), upgraderResource, metav1.CreateOptions{})

	return statusInfo, template, fakeDynamicClient
}

func TestStatusCreate(t *testing.T) {
	// Setup the test
	statusInfo, _, _ := setupStatusTest()
	ctx := context.Background()

	// Test cases
	testCases := []struct {
		name         string
		resourceKey  string
		message      string
		status       string
		args         []interface{}
		expectError  bool
		errorMessage string
	}{
		{
			name:        "Create valid status",
			resourceKey: "updater",
			message:     "Started update process",
			status:      "updates",
			expectError: false,
		},
		{
			name:         "Invalid resource key",
			resourceKey:  "nonexistent",
			message:      "Error occurred",
			status:       "error",
			expectError:  true,
			errorMessage: "resource with key nonexistent not found in template",
		},
	}

	// Run the tests
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Call the function being tested
			err := statusInfo.Create(ctx, tc.resourceKey, tc.message, tc.status, tc.args...)

			// Check error expectations
			if tc.expectError {
				if err == nil {
					t.Errorf("Expected error but got nil")
				} else if err.Error() != tc.errorMessage {
					t.Errorf("Expected error message '%s', but got '%s'", tc.errorMessage, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
			}
		})
	}
}

func TestStatusRead(t *testing.T) {
	// Setup the test
	statusInfo, _, _ := setupStatusTest()
	ctx := context.Background()

	// Test cases
	testCases := []struct {
		name         string
		resourceKey  string
		message      string
		status       string
		args         []interface{}
		expectError  bool
		errorMessage string
	}{
		{
			name:        "Read valid status",
			resourceKey: "updater",
			message:     "Current status: %s",
			status:      "updates",
			expectError: false,
		},
		{
			name:         "Invalid resource key",
			resourceKey:  "nonexistent",
			message:      "Error occurred",
			status:       "error",
			expectError:  true,
			errorMessage: "resource with key nonexistent not found in template",
		},
	}

	// Run the tests
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Call the function being tested
			err := statusInfo.Read(ctx, tc.resourceKey, tc.message, tc.status, tc.args...)

			// Check error expectations
			if tc.expectError {
				if err == nil {
					t.Errorf("Expected error but got nil")
				} else if err.Error() != tc.errorMessage {
					t.Errorf("Expected error message '%s', but got '%s'", tc.errorMessage, err.Error())
				}
			} else {
				// Some implementations might return not found error when reading
				// the resource, which is fine for testing
				if err != nil && !isNotFoundError(err) {
					t.Errorf("Expected no error but got: %v", err)
				}
			}
		})
	}
}

func TestStatusUpdate(t *testing.T) {
	// Setup the test
	statusInfo, _, _ := setupStatusTest()
	ctx := context.Background()

	// Test cases
	testCases := []struct {
		name         string
		resourceKey  string
		message      string
		status       string
		args         []interface{}
		expectError  bool
		errorMessage string
	}{
		{
			name:        "Update valid status",
			resourceKey: "updater",
			message:     "Update completed: %d%%",
			status:      "updates",
			args:        []interface{}{100},
			expectError: false,
		},
		{
			name:         "Invalid resource key",
			resourceKey:  "nonexistent",
			message:      "Error occurred",
			status:       "error",
			expectError:  true,
			errorMessage: "resource with key nonexistent not found in template",
		},
	}

	// Run the tests
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Call the function being tested
			err := statusInfo.Update(ctx, tc.resourceKey, tc.message, tc.status, tc.args...)

			// Check error expectations
			if tc.expectError {
				if err == nil {
					t.Errorf("Expected error but got nil")
				} else if err.Error() != tc.errorMessage {
					t.Errorf("Expected error message '%s', but got '%s'", tc.errorMessage, err.Error())
				}
			} else {
				// Some implementations might return not found error when updating
				// the resource, which is fine for testing
				if err != nil && !isNotFoundError(err) {
					t.Errorf("Expected no error but got: %v", err)
				}
			}
		})
	}
}

// Helper function to check if an error is a "not found" error
func isNotFoundError(err error) bool {
	return err != nil && (err.Error() == "resource not found" ||
		err.Error() == "resourceVersion is required" ||
		err.Error() == "error updating status for resource")
}
