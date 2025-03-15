package resource

import (
	"fmt"
	"k2p-updater/cmd/app"
	corev1 "k8s.io/api/core/v1"
)

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
