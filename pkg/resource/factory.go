package resource

import (
	"context"
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

// FactoryDefinition is a structure that holds a resource definition configuration
type FactoryDefinition struct {
	Resource    string
	NameFormat  string
	StatusField interface{}
	Kind        string
	CRName      string
}

// GetStatusFieldString returns the StatusField as a string
func (rd *FactoryDefinition) GetStatusFieldString() (string, bool) {
	if field, ok := rd.StatusField.(string); ok {
		return field, true
	}
	return "", false
}

// Factory creates and manages resource handler
type Factory struct {
	namespace      string
	group          string
	version        string
	definitions    map[string]FactoryDefinition
	dynamicClient  dynamic.Interface
	clientSet      KubeClientInterface
	sharedTemplate *Template
	eventHandler   Event
	statusHandler  Status
	helpers        *Helpers
}

// NewFactory creates a factory for the resource handler
func NewFactory(namespace, group, version string, definitions map[string]FactoryDefinition, dynamicClient dynamic.Interface, clientSet KubeClientInterface) (*Factory, error) {
	// Create a shared template only once
	template, err := convertToTemplate(namespace, group, version, definitions)
	if err != nil {
		return nil, err
	}

	// Create a helper
	helpers := &Helpers{
		DynamicClient: dynamicClient,
		Template:      template,
	}

	// Create an event handler
	eventHandler := &EventInfo{
		Template:      template,
		KubeClient:    clientSet,
		DynamicClient: dynamicClient,
	}

	// Create a state handler
	statusHandler := &StatusInfo{
		Template:      template,
		DynamicClient: dynamicClient,
	}

	return &Factory{
		namespace:      namespace,
		group:          group,
		version:        version,
		definitions:    definitions,
		dynamicClient:  dynamicClient,
		clientSet:      clientSet,
		sharedTemplate: template,
		eventHandler:   eventHandler,
		statusHandler:  statusHandler,
		helpers:        helpers,
	}, nil
}

// Event returns an event handler
func (f *Factory) Event() Event {
	return f.eventHandler
}

// Status returns a status handler
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

	return f.dynamicClient.Resource(gvr).
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

	_, err = f.dynamicClient.Resource(gvr).
		Namespace(resource.namespace).
		Create(ctx, obj, metav1.CreateOptions{})
	return err
}

// convertToTemplate converts a direct parameter to a resource template
func convertToTemplate(namespace, group, version string, definitions map[string]FactoryDefinition) (*Template, error) {
	template := &Template{
		Key: make(map[string]Resource),
	}

	// Process each resource definition
	for key, definition := range definitions {
		res := Resource{
			namespace:  namespace,
			group:      group,
			version:    version,
			definition: map[string]Definition{},
		}

		// Create a definition
		def := Definition{
			NameFormat:  definition.NameFormat,
			Resource:    definition.Resource,
			Kind:        definition.Kind,
			CRName:      definition.CRName,
			StatusField: make(map[interface{}]interface{}),
		}

		// StatusField can be a string or map
		if statusFieldStr, ok := definition.GetStatusFieldString(); ok {
			def.StatusField = map[interface{}]interface{}{
				"field": statusFieldStr,
			}
		} else if statusMap, ok := definition.StatusField.(map[string]interface{}); ok {
			// Convert a string map to an interface map
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
