package resource

import (
	"context"
	"fmt"
	"k2p-updater/cmd/app"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// Helpers contains utility functions for working with resources
type Helpers struct {
	DynamicClient dynamic.Interface
	Template      *Template
}

// NewResourceHelpers creates a new ResourceHelpers instance
func NewResourceHelpers(client dynamic.Interface, template *Template) *Helpers {
	return &Helpers{
		DynamicClient: client,
		Template:      template,
	}
}

// GetGVR returns the GroupVersionResource for a resource key
func (r *Helpers) GetGVR(resourceKey string) (schema.GroupVersionResource, error) {
	resource, exists := r.Template.Key[resourceKey]
	if !exists {
		return schema.GroupVersionResource{}, fmt.Errorf("resource with key %s not found", resourceKey)
	}

	var definition Definition
	for _, def := range resource.definition {
		definition = def
		break
	}

	return schema.GroupVersionResource{
		Group:    resource.group,
		Version:  resource.version,
		Resource: definition.Resource,
	}, nil
}

// GetResourceName returns the formatted resource name for a key
func (r *Helpers) GetResourceName(resourceKey string) (string, error) {
	resource, exists := r.Template.Key[resourceKey]
	if !exists {
		return "", fmt.Errorf("resource with key %s not found", resourceKey)
	}

	var definition Definition
	for k, def := range resource.definition {
		definition = def
		return fmt.Sprintf(definition.NameFormat, k), nil
	}

	return "", fmt.Errorf("no definition found for resource key %s", resourceKey)
}

// UpdateResourceStatus updates the status field of a resource
func (r *Helpers) UpdateResourceStatus(ctx context.Context, resourceKey, statusField, value string) error {
	gvr, err := r.GetGVR(resourceKey)
	if err != nil {
		return err
	}

	name, err := r.GetResourceName(resourceKey)
	if err != nil {
		return err
	}

	resource := r.Template.Key[resourceKey]

	// Get the current object
	obj, err := r.DynamicClient.Resource(gvr).Namespace(resource.namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}

	// Update the status field
	if err := unstructured.SetNestedField(obj.Object, value, "status", statusField); err != nil {
		return err
	}

	// Update the object status
	_, err = r.DynamicClient.Resource(gvr).Namespace(resource.namespace).UpdateStatus(ctx, obj, metav1.UpdateOptions{})
	return err
}

// GetResourceStatus retrieves the status field value of a resource
func (r *Helpers) GetResourceStatus(ctx context.Context, resourceKey, statusField string) (string, error) {
	gvr, err := r.GetGVR(resourceKey)
	if err != nil {
		return "", err
	}

	name, err := r.GetResourceName(resourceKey)
	if err != nil {
		return "", err
	}

	resource := r.Template.Key[resourceKey]

	// Get the object
	obj, err := r.DynamicClient.Resource(gvr).Namespace(resource.namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	// Get the status field
	value, found, err := unstructured.NestedString(obj.Object, "status", statusField)
	if err != nil {
		return "", err
	}
	if !found {
		return "", fmt.Errorf("status field %s not found", statusField)
	}

	return value, nil
}

// Factory creates and initializes resource handlers
type Factory struct {
	Config        *app.Config
	ClientSet     KubeClientInterface
	DynamicClient dynamic.Interface
}

// NewFactory creates a new resource factory
func NewFactory(config *app.Config, clients *app.KubeClients) *Factory {
	return &Factory{
		Config:        config,
		ClientSet:     clients.ClientSet,
		DynamicClient: clients.DynamicClient,
	}
}

// CreateEventHandler creates a new EventInfo instance
func (f *Factory) CreateEventHandler() (Event, error) {
	template, err := ConvertToTemplate(f.Config)
	if err != nil {
		return nil, err
	}

	return &EventInfo{
		Template:   template,
		KubeClient: f.ClientSet,
	}, nil
}

// CreateStatusHandler creates a new StatusInfo instance
func (f *Factory) CreateStatusHandler() (Status, error) {
	template, err := ConvertToTemplate(f.Config)
	if err != nil {
		return nil, err
	}

	return &StatusInfo{
		Template:      template,
		DynamicClient: f.DynamicClient,
	}, nil
}

// ConvertToTemplate converts app configuration to a resource template
func ConvertToTemplate(config *app.Config) (*Template, error) {
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
			CRName:      definition.CRName, // Add the CRName field
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

// GetResourceRef returns a Kubernetes ObjectReference for a resource
func (t *Template) GetResourceRef(resourceKey string) (*corev1.ObjectReference, error) {
	resource, exists := t.Key[resourceKey]
	if !exists {
		return nil, fmt.Errorf("resource with key %s not found in template", resourceKey)
	}

	// Get the definition
	var definition Definition
	for k, def := range resource.definition {
		definition = def

		// Create ObjectReference
		return &corev1.ObjectReference{
			APIVersion: fmt.Sprintf("%s/%s", resource.group, resource.version),
			Kind:       definition.Kind,
			Namespace:  resource.namespace,
			Name:       fmt.Sprintf(definition.NameFormat, k),
		}, nil
	}

	return nil, fmt.Errorf("no definition found for resource key %s", resourceKey)
}
