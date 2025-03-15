package resource

import (
	"context"
	"fmt"
	"k2p-updater/cmd/app"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Manager is responsible for handling resource operations
type Manager struct {
	event   Event
	status  Status
	helpers *Helpers
	config  *app.Config
}

// NewManager creates a new resource manager
func NewManager(factory *Factory) *Manager {
	return &Manager{
		event:   factory.CreateEventHandler(),
		status:  factory.CreateStatusHandler(),
		helpers: factory.CreateResourceHelpers(),
		config:  factory.config,
	}
}

// Event returns the event handler
func (m *Manager) Event() Event {
	return m.event
}

// Status returns the status handler
func (m *Manager) Status() Status {
	return m.status
}

// GetResource retrieves a resource by key and name
func (m *Manager) GetResource(ctx context.Context, resourceKey, name string) (*unstructured.Unstructured, error) {
	gvr, err := m.helpers.GetGVR(resourceKey)
	if err != nil {
		return nil, err
	}

	resource := m.helpers.Template.Key[resourceKey]

	return m.helpers.DynamicClient.Resource(gvr).
		Namespace(resource.namespace).
		Get(ctx, name, metav1.GetOptions{})
}

// CreateResource creates a new custom resource
func (m *Manager) CreateResource(ctx context.Context, resourceKey string, spec map[string]interface{}) error {
	gvr, err := m.helpers.GetGVR(resourceKey)
	if err != nil {
		return err
	}

	resourceName, err := m.helpers.GetResourceName(resourceKey)
	if err != nil {
		return err
	}

	resource := m.helpers.Template.Key[resourceKey]
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

	_, err = m.helpers.DynamicClient.Resource(gvr).
		Namespace(resource.namespace).
		Create(ctx, obj, metav1.CreateOptions{})
	return err
}
