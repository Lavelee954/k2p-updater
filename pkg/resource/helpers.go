package resource

import (
	"context"
	"fmt"
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
