package resource

import (
	"context"
	"testing"

	kfake "k8s.io/client-go/kubernetes/fake"
)

func setupEventTest() (*EventInfo, *Template) {
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

	// Create a fake Kubernetes clientset
	fakeClient := kfake.NewSimpleClientset()

	// Create the EventInfo with our mocks
	eventInfo := &EventInfo{
		Template:   template,
		KubeClient: fakeClient, // The fake client implements the same interface as the real client
	}

	return eventInfo, template
}

func TestNormalRecord(t *testing.T) {
	// Setup the test
	eventInfo, _ := setupEventTest()
	ctx := context.Background()

	// Test cases
	testCases := []struct {
		name         string
		resourceKey  string
		reason       string
		eventType    string
		message      string
		args         []interface{}
		expectError  bool
		errorMessage string
	}{
		{
			name:        "Valid normal event",
			resourceKey: "updater",
			reason:      "Created",
			eventType:   "Normal",
			message:     "Resource %s created successfully",
			args:        []interface{}{"test-resource"},
			expectError: false,
		},
		{
			name:         "Invalid resource key",
			resourceKey:  "nonexistent",
			reason:       "Error",
			eventType:    "Normal",
			message:      "Error occurred",
			expectError:  true,
			errorMessage: "resource with key nonexistent not found in template",
		},
	}

	// Run the tests
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Call the function being tested
			err := eventInfo.NormalRecord(ctx, tc.resourceKey, tc.reason, tc.eventType, tc.message)

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

func TestWarningRecord(t *testing.T) {
	// Setup the test
	eventInfo, _ := setupEventTest()
	ctx := context.Background()

	// Test cases
	testCases := []struct {
		name         string
		resourceKey  string
		reason       string
		eventType    string
		message      string
		args         []interface{}
		expectError  bool
		errorMessage string
	}{
		{
			name:        "Valid warning event",
			resourceKey: "upgrader",
			reason:      "Failed",
			eventType:   "Warning",
			message:     "Resource %s failed with error: %s",
			args:        []interface{}{"test-resource", "connection timeout"},
			expectError: false,
		},
		{
			name:         "Invalid resource key",
			resourceKey:  "nonexistent",
			reason:       "Error",
			eventType:    "Warning",
			message:      "Error occurred",
			expectError:  true,
			errorMessage: "resource with key nonexistent not found in template",
		},
	}

	// Run the tests
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Call the function being tested
			err := eventInfo.WarningRecord(ctx, tc.resourceKey, tc.reason, tc.eventType, tc.message)

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
