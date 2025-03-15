package resource

import (
	"fmt"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

type Template struct {
	Key map[string]Resource
}

type Resource struct {
	namespace  string
	group      string
	version    string
	definition map[string]Definition
}

// Definition represents a resource definition
type Definition struct {
	NameFormat  string
	Resource    string
	StatusField map[interface{}]interface{}
	Kind        string
	CRName      string
}

// Helpers contains utility functions for working with resources
type Helpers struct {
	DynamicClient dynamic.Interface
	Template      *Template
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
