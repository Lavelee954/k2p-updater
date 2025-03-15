package resource

import (
	"context"
	"fmt"
	"k2p-updater/cmd/app"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Factory creates and manages resource handlers
type Factory struct {
	config         *app.Config
	kubeClients    *app.KubeClients
	sharedTemplate *Template
	eventHandler   Event
	statusHandler  Status
	helpers        *Helpers
}

// NewFactory creates a factory for resource handlers
func NewFactory(config *app.Config, clients *app.KubeClients) (*Factory, error) {
	// Create the shared template once
	template, err := convertToTemplate(config)
	if err != nil {
		return nil, err
	}

	// Create helpers
	helpers := &Helpers{
		DynamicClient: clients.DynamicClient,
		Template:      template,
	}

	// Create event handler
	eventHandler := &EventInfo{
		Template:      template,
		KubeClient:    clients.ClientSet,
		DynamicClient: clients.DynamicClient,
	}

	// Create status handler
	statusHandler := &StatusInfo{
		Template:      template,
		DynamicClient: clients.DynamicClient,
	}

	return &Factory{
		config:         config,
		kubeClients:    clients,
		sharedTemplate: template,
		eventHandler:   eventHandler,
		statusHandler:  statusHandler,
		helpers:        helpers,
	}, nil
}

// Event returns the event handler
func (f *Factory) Event() Event {
	return f.eventHandler
}

// Status returns the status handler
func (f *Factory) Status() Status {
	return f.statusHandler
}

// GetResource retrieves a resource by key and name
func (f *Factory) GetResource(ctx context.Context, resourceKey, name string) (*unstructured.Unstructured, error) {
	gvr, err := f.helpers.GetGVR(resourceKey)
	if err != nil {
		return nil, err
	}

	resource := f.sharedTemplate.Key[resourceKey]

	return f.kubeClients.DynamicClient.Resource(gvr).
		Namespace(resource.namespace).
		Get(ctx, name, metav1.GetOptions{})
}

// CreateResource creates a new custom resource
func (f *Factory) CreateResource(ctx context.Context, resourceKey string, spec map[string]interface{}) error {
	gvr, err := f.helpers.GetGVR(resourceKey)
	if err != nil {
		return err
	}

	resourceName, err := f.helpers.GetResourceName(resourceKey)
	if err != nil {
		return err
	}

	resource := f.sharedTemplate.Key[resourceKey]
	var definition Definition

	for _, def := range resource.definition {
		definition = def
		break
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

	_, err = f.kubeClients.DynamicClient.Resource(gvr).
		Namespace(resource.namespace).
		Create(ctx, obj, metav1.CreateOptions{})
	return err
}

// createTemplate converts app configuration to a resource template
// Private method as it's an implementation detail
func convertToTemplate(config *app.Config) (*Template, error) {
	template := &Template{
		Key: make(map[string]Resource),
	}

	// Process each resource definition
	for key, definition := range config.Resources.Definitions {
		res := Resource{
			namespace:  config.Resources.Namespace,
			group:      config.Resources.Group,
			version:    config.Resources.Version,
			definition: map[string]Definition{},
		}

		// Create the definition
		def := Definition{
			NameFormat:  definition.NameFormat,
			Resource:    definition.Resource,
			Kind:        definition.Kind,
			CRName:      definition.CRName,
			StatusField: make(map[interface{}]interface{}),
		}

		// Handle status field which can be a string or map
		if statusFieldStr, ok := definition.GetStatusFieldString(); ok {
			def.StatusField = map[interface{}]interface{}{
				"field": statusFieldStr,
			}
		} else if statusMap, ok := definition.StatusField.(map[string]interface{}); ok {
			// Convert string map to interface map
			for k, v := range statusMap {
				def.StatusField[k] = v
			}
		} else {
			return nil, fmt.Errorf("invalid status field type for resource %s", key)
		}

		res.definition[key] = def
		template.Key[key] = res
	}

	return template, nil
}
