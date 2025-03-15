package resource

import (
	"fmt"
	"k2p-updater/cmd/app"
)

// Factory creates resource handlers with proper dependencies
type Factory struct {
	config         *app.Config
	kubeClients    *app.KubeClients
	sharedTemplate *Template
}

// NewFactory creates a factory for resource handlers
func NewFactory(config *app.Config, clients *app.KubeClients) (*Factory, error) {
	// Create the shared template once
	template, err := convertToTemplate(config)
	if err != nil {
		return nil, err
	}

	return &Factory{
		config:         config,
		kubeClients:    clients,
		sharedTemplate: template,
	}, nil
}

// CreateEventHandler creates a new EventInfo instance
func (f *Factory) CreateEventHandler() Event {
	return &EventInfo{
		Template:      f.sharedTemplate,
		KubeClient:    f.kubeClients.ClientSet,
		DynamicClient: f.kubeClients.DynamicClient,
	}
}

// CreateStatusHandler creates a new StatusInfo instance
func (f *Factory) CreateStatusHandler() Status {
	return &StatusInfo{
		Template:      f.sharedTemplate,
		DynamicClient: f.kubeClients.DynamicClient,
	}
}

// CreateResourceHelpers creates utility helpers for resources
func (f *Factory) CreateResourceHelpers() *Helpers {
	return &Helpers{
		DynamicClient: f.kubeClients.DynamicClient,
		Template:      f.sharedTemplate,
	}
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
