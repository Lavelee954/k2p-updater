package resource

import (
	"testing"

	"k2p-updater/cmd/app"
)

func TestConvertToTemplate(t *testing.T) {
	// Create a test config
	config := &app.Config{
		Resources: app.ResourcesConfig{
			Namespace: "test-namespace",
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

	// Call the function being tested
	template, err := ConvertToTemplate(config)

	// Check that there was no error
	if err != nil {
		t.Errorf("Expected no error but got: %v", err)
	}

	// Check that the template has the expected resources
	if len(template.Key) != 2 {
		t.Errorf("Expected 2 resources but got %d", len(template.Key))
	}

	// Check the updater resource
	updater, exists := template.Key["updater"]
	if !exists {
		t.Errorf("Expected 'updater' resource to exist")
	} else {
		if updater.namespace != "test-namespace" {
			t.Errorf("Expected namespace 'test-namespace' but got '%s'", updater.namespace)
		}
		if updater.group != "k2p.cloud.kt.com" {
			t.Errorf("Expected group 'k2p.cloud.kt.com' but got '%s'", updater.group)
		}
		if updater.version != "v1beta1" {
			t.Errorf("Expected version 'v1beta1' but got '%s'", updater.version)
		}

		// Check the definition
		def, exists := updater.definition["updater"]
		if !exists {
			t.Errorf("Expected 'updater' definition to exist")
		} else {
			if def.NameFormat != "k2pupdater-%s" {
				t.Errorf("Expected NameFormat 'k2pupdater-%%s' but got '%s'", def.NameFormat)
			}
			if def.Resource != "k2pupdaters" {
				t.Errorf("Expected Resource 'k2pupdaters' but got '%s'", def.Resource)
			}
			if def.Kind != "K2pUpdater" {
				t.Errorf("Expected Kind 'K2pUpdater' but got '%s'", def.Kind)
			}

			// Check the status field
			field, exists := def.StatusField["field"]
			if !exists {
				t.Errorf("Expected 'field' in StatusField to exist")
			} else if field != "updates" {
				t.Errorf("Expected StatusField['field'] to be 'updates' but got '%s'", field)
			}
		}
	}
}

func TestTemplateGetResourceRef(t *testing.T) {
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
		},
	}

	// Test cases
	testCases := []struct {
		name         string
		resourceKey  string
		expectError  bool
		errorMessage string
		expectedRef  map[string]string
	}{
		{
			name:        "Valid resource key",
			resourceKey: "updater",
			expectError: false,
			expectedRef: map[string]string{
				"APIVersion": "k2p.cloud.kt.com/v1beta1",
				"Kind":       "K2pUpdater",
				"Namespace":  "default",
				"Name":       "k2pupdater-updater",
			},
		},
		{
			name:         "Invalid resource key",
			resourceKey:  "nonexistent",
			expectError:  true,
			errorMessage: "resource with key nonexistent not found in template",
		},
	}

	// Run the tests
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Call the function being tested
			ref, err := template.GetResourceRef(tc.resourceKey)

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

				// Check the ref fields
				if ref.APIVersion != tc.expectedRef["APIVersion"] {
					t.Errorf("Expected APIVersion '%s' but got '%s'", tc.expectedRef["APIVersion"], ref.APIVersion)
				}
				if ref.Kind != tc.expectedRef["Kind"] {
					t.Errorf("Expected Kind '%s' but got '%s'", tc.expectedRef["Kind"], ref.Kind)
				}
				if ref.Namespace != tc.expectedRef["Namespace"] {
					t.Errorf("Expected Namespace '%s' but got '%s'", tc.expectedRef["Namespace"], ref.Namespace)
				}
				if ref.Name != tc.expectedRef["Name"] {
					t.Errorf("Expected Name '%s' but got '%s'", tc.expectedRef["Name"], ref.Name)
				}
			}
		})
	}
}
