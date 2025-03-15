package resource

import (
	"context"
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

type Status interface {
	Create(ctx context.Context, resourceKey, message, status string, args ...interface{}) error
	Read(ctx context.Context, resourceKey, message, status string, args ...interface{}) error
	Update(ctx context.Context, resourceKey, message, status string, args ...interface{}) error
}

type StatusInfo struct {
	Template      *Template
	DynamicClient dynamic.Interface
}

func (t StatusInfo) Create(ctx context.Context, resourceKey, message, status string, args ...interface{}) error {
	// 1. Extract ResourceName registered in a template by resourceKey
	resource, exists := t.Template.Key[resourceKey]
	if !exists {
		return fmt.Errorf("resource with key %s not found in template", resourceKey)
	}

	// Get the definition
	var definition Definition
	for _, def := range resource.definition {
		definition = def
		break
	}

	// Format the resource name
	resourceName := fmt.Sprintf(definition.NameFormat, resourceKey)

	// 2. Setting up gvr for Kubernetes CR status updates with templates
	gvr := schema.GroupVersionResource{
		Group:    resource.group,
		Version:  resource.version,
		Resource: definition.Resource,
	}

	// 3. Create a state
	formattedMessage := fmt.Sprintf(message, args...)

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": fmt.Sprintf("%s/%s", resource.group, resource.version),
			"kind":       definition.Kind,
			"metadata": map[string]interface{}{
				"name":      resourceName,
				"namespace": resource.namespace,
			},
			"status": map[string]interface{}{
				status: formattedMessage,
			},
		},
	}

	_, err := t.DynamicClient.Resource(gvr).Namespace(resource.namespace).Create(ctx, obj, metav1.CreateOptions{})
	return err
}

func (t StatusInfo) Read(ctx context.Context, resourceKey, message, status string, args ...interface{}) error {
	// 1. Extract ResourceName registered in a template by resourceKey
	resource, exists := t.Template.Key[resourceKey]
	if !exists {
		return fmt.Errorf("resource with key %s not found in template", resourceKey)
	}

	// Get the definition
	var definition Definition
	for _, def := range resource.definition {
		definition = def
		break
	}

	// Format the resource name
	resourceName := fmt.Sprintf(definition.NameFormat, resourceKey)

	// 2. Setting up gvr for Kubernetes CR status updates with templates
	gvr := schema.GroupVersionResource{
		Group:    resource.group,
		Version:  resource.version,
		Resource: definition.Resource,
	}

	// 3. Read status
	obj, err := t.DynamicClient.Resource(gvr).Namespace(resource.namespace).Get(ctx, resourceName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	statusValue, exists, err := unstructured.NestedString(obj.Object, "status", status)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("status field %s not found", status)
	}

	// Format and output the message with the status value
	formattedMessage := fmt.Sprintf(message, statusValue)
	fmt.Println(formattedMessage)

	return nil
}

func (t StatusInfo) Update(ctx context.Context, resourceKey, message, status string, args ...interface{}) error {
	// 1. Extract ResourceName registered in a template by resourceKey
	resource, exists := t.Template.Key[resourceKey]
	if !exists {
		return fmt.Errorf("resource with key %s not found in template", resourceKey)
	}

	// Get the definition
	var definition Definition
	for _, def := range resource.definition {
		definition = def
		break
	}

	// Format the resource name
	resourceName := fmt.Sprintf(definition.NameFormat, resourceKey)

	// 2. Setting up gvr for Kubernetes CR status updates with templates
	gvr := schema.GroupVersionResource{
		Group:    resource.group,
		Version:  resource.version,
		Resource: definition.Resource,
	}

	// 3. Status updates
	formattedMessage := fmt.Sprintf(message, args...)

	// Get the current object
	obj, err := t.DynamicClient.Resource(gvr).Namespace(resource.namespace).Get(ctx, resourceName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	// Update the status field
	if err := unstructured.SetNestedField(obj.Object, formattedMessage, "status", status); err != nil {
		return err
	}

	// Update the object status
	_, err = t.DynamicClient.Resource(gvr).Namespace(resource.namespace).UpdateStatus(ctx, obj, metav1.UpdateOptions{})
	return err
}
