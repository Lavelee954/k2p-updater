package resource

import (
	"context"
	"fmt"
	"k2p-updater/cmd/app"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Manager is responsible for handling resource operations
type Manager struct {
	Event      Event
	Status     Status
	Templates  *Template
	KubeClient *app.KubeClients
	Config     *app.Config
}

// NewManager creates a new resource manager
func NewManager(cfg *app.Config, clients *app.KubeClients) (*Manager, error) {
	// Create template map from configuration
	template, err := buildTemplateFromConfig(cfg)
	if err != nil {
		return nil, err
	}

	// Create event and status handlers
	eventHandler := &EventInfo{
		Template:   template,
		KubeClient: clients.ClientSet,
	}

	statusHandler := &StatusInfo{
		Template:      template,
		DynamicClient: clients.DynamicClient,
	}

	return &Manager{
		Event:      eventHandler,
		Status:     statusHandler,
		Templates:  template,
		KubeClient: clients,
		Config:     cfg,
	}, nil
}

// buildTemplateFromConfig converts configuration into the Template structure
func buildTemplateFromConfig(cfg *app.Config) (*Template, error) {
	template := &Template{
		Key: make(map[string]Resource),
	}

	// For each definition in the configuration, create a Resource
	for key, defConfig := range cfg.Resources.Definitions {
		// Create the definition first with all fields
		def := Definition{
			NameFormat: defConfig.NameFormat,
			Resource:   defConfig.Resource,
			Kind:       defConfig.Kind,
			CRName:     defConfig.CRName,
		}

		// Handle the StatusField which can be a string or a map
		if statusStr, ok := defConfig.GetStatusFieldString(); ok {
			def.StatusField = map[interface{}]interface{}{
				"field": statusStr,
			}
		} else if statusMap, ok := defConfig.StatusField.(map[string]interface{}); ok {
			def.StatusField = statusMapToInterface(statusMap)
		}

		// Now create the resource with the fully populated definition
		resource := Resource{
			namespace: cfg.Resources.Namespace,
			group:     cfg.Resources.Group,
			version:   cfg.Resources.Version,
			definition: map[string]Definition{
				key: def,
			},
		}

		template.Key[key] = resource
	}

	return template, nil
}

// statusMapToInterface converts a map[string]interface{} to map[interface{}]interface{}
func statusMapToInterface(in map[string]interface{}) map[interface{}]interface{} {
	result := make(map[interface{}]interface{})
	for k, v := range in {
		result[k] = v
	}
	return result
}

// GetResource retrieves a resource by key and name
func (m *Manager) GetResource(ctx context.Context, resourceKey, name string) (*unstructured.Unstructured, error) {
	resource, exists := m.Templates.Key[resourceKey]
	if !exists {
		return nil, fmt.Errorf("resource with key %s not found", resourceKey)
	}

	var definition Definition
	for _, def := range resource.definition {
		definition = def
		break
	}

	gvr := schema.GroupVersionResource{
		Group:    resource.group,
		Version:  resource.version,
		Resource: definition.Resource,
	}

	return m.KubeClient.DynamicClient.Resource(gvr).
		Namespace(resource.namespace).
		Get(ctx, name, metav1.GetOptions{})
}

// CreateResource creates a new custom resource
func (m *Manager) CreateResource(ctx context.Context, resourceKey string, spec map[string]interface{}) error {
	resource, exists := m.Templates.Key[resourceKey]
	if !exists {
		return fmt.Errorf("resource with key %s not found", resourceKey)
	}

	var definition Definition
	defKey := ""
	for k, def := range resource.definition {
		definition = def
		defKey = k
		break
	}

	resourceName := fmt.Sprintf(definition.NameFormat, defKey)

	gvr := schema.GroupVersionResource{
		Group:    resource.group,
		Version:  resource.version,
		Resource: definition.Resource,
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": fmt.Sprintf("%s/%s", resource.group, resource.version),
			"kind":       definition.Kind,
			"metadata": map[string]interface{}{
				"name":      resourceName,
				"namespace": resource.namespace,
			},
			"spec": spec,
		},
	}

	_, err := m.KubeClient.DynamicClient.Resource(gvr).
		Namespace(resource.namespace).
		Create(ctx, obj, metav1.CreateOptions{})
	return err
}
