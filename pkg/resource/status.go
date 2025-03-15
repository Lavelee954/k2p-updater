package resource

import (
	"context"
	"fmt"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

type Status interface {
	Create(ctx context.Context, resourceKey, message, status string) error
	Read(ctx context.Context, resourceKey, message, status string) error
	Update(ctx context.Context, resourceKey, message, status string) error
	UpdateGeneric(ctx context.Context, resourceKey string, newStatusData interface{}) error
	UpdateGenericWithNode(ctx context.Context, resourceKey string, nodeName string, newStatusData interface{}) error
}

type StatusInfo struct {
	Template      *Template
	DynamicClient dynamic.Interface
}

func (t StatusInfo) Create(ctx context.Context, resourceKey, message, status string) error {
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
	formattedMessage := fmt.Sprintf(message)

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

func (t StatusInfo) Read(ctx context.Context, resourceKey, message, status string) error {
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

func (t StatusInfo) Update(ctx context.Context, resourceKey, message, status string) error {
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
	formattedMessage := fmt.Sprintf(message)

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

// UpdateGeneric updates the status of a resource while preserving specific fields
func (t StatusInfo) UpdateGeneric(ctx context.Context, resourceKey string, newStatusData interface{}) error {
	return t.updateGenericInternal(ctx, resourceKey, "", newStatusData)
}

// UpdateGenericWithNode updates the status using a specific node name for formatting
func (t StatusInfo) UpdateGenericWithNode(ctx context.Context, resourceKey string, nodeName string, newStatusData interface{}) error {
	return t.updateGenericInternal(ctx, resourceKey, nodeName, newStatusData)
}

// updateGenericInternal implements the status update logic with nodeName handling
func (t StatusInfo) updateGenericInternal(ctx context.Context, resourceKey string, nodeName string, newStatusData interface{}) error {
	// Validate resource key
	if resourceKey == "" {
		return fmt.Errorf("resource key cannot be empty")
	}

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

	// 2. Set the required name for the resource based on cr_name config
	var resourceName string
	if definition.CRName == "%s" && nodeName != "" {
		// When cr_name is "%s" and nodeName is provided, use nodeName
		resourceName = fmt.Sprintf(definition.NameFormat, nodeName)
	} else if definition.CRName != "" {
		// When cr_name is a specific value, use that value
		resourceName = fmt.Sprintf(definition.NameFormat, definition.CRName)
	} else {
		// Fall back to the original behavior using resourceKey
		resourceName = fmt.Sprintf(definition.NameFormat, resourceKey)
	}

	// 3. Setting up gvr for Kubernetes CR status updates with templates
	gvr := schema.GroupVersionResource{
		Group:    resource.group,
		Version:  resource.version,
		Resource: definition.Resource,
	}

	// Get the current resource
	obj, err := t.DynamicClient.
		Resource(gvr).
		Namespace(resource.namespace).
		Get(ctx, resourceName, metav1.GetOptions{})

	if err != nil {
		return fmt.Errorf("failed to get resource %s: %w", resourceName, err)
	}

	// Initialize status if it doesn't exist
	if obj.Object["status"] == nil {
		obj.Object["status"] = make(map[string]interface{})
	}

	statusObj := obj.Object["status"].(map[string]interface{})

	// Only for the updater resource with the nested updates structure
	if resourceKey == "updater" {
		// Get or create the updates array
		var updates []interface{}
		if updatesArr, exists := statusObj["updates"]; exists {
			updates, _ = updatesArr.([]interface{})
		}

		if updates == nil {
			updates = []interface{}{}
		}

		// Convert new status data to map
		newDetailsMap := make(map[string]interface{})

		// Handle different input types
		if detailsMap, ok := newStatusData.(map[string]interface{}); ok {
			// It's already a map
			newDetailsMap = detailsMap
		} else {
			// Try to extract fields from whatever type was passed
			if m, ok := newStatusData.(map[interface{}]interface{}); ok {
				for k, v := range m {
					if keyStr, ok := k.(string); ok {
						newDetailsMap[keyStr] = v
					}
				}
			}
		}

		// Find the update for this node or create a new one
		var nodeUpdate map[string]interface{}
		nodeIndex := -1

		for i, update := range updates {
			updateMap, ok := update.(map[string]interface{})
			if !ok {
				continue
			}

			detailsMap, ok := updateMap["details"].(map[string]interface{})
			if !ok {
				continue
			}

			nodeName, ok := detailsMap["controlPlaneNodeName"].(string)
			if !ok {
				continue
			}

			if nodeName == newDetailsMap["controlPlaneNodeName"] {
				nodeUpdate = updateMap
				nodeIndex = i
				break
			}
		}

		// If not found, create a new update entry
		if nodeIndex == -1 {
			nodeUpdate = map[string]interface{}{
				"details": make(map[string]interface{}),
			}
			updates = append(updates, nodeUpdate)
			nodeIndex = len(updates) - 1
		}

		// Get current details to preserve important fields
		detailsMap, ok := nodeUpdate["details"].(map[string]interface{})
		if !ok {
			detailsMap = make(map[string]interface{})
		}

		// CRITICAL FIX: Always ensure updateStatus is present
		if existingStatus, exists := detailsMap["updateStatus"]; exists {
			// Preserve existing updateStatus if not explicitly specified in new data
			if _, specified := newDetailsMap["updateStatus"]; !specified {
				newDetailsMap["updateStatus"] = existingStatus
			}
		} else {
			// If updateStatus is missing, default to "Pending"
			if _, specified := newDetailsMap["updateStatus"]; !specified {
				newDetailsMap["updateStatus"] = "Pending"
			}
		}

		// Update the details with new values
		for k, v := range newDetailsMap {
			detailsMap[k] = v
		}

		nodeUpdate["details"] = detailsMap
		updates[nodeIndex] = nodeUpdate

		// Update the status
		statusObj["updates"] = updates
	} else {
		// For other types of resources (non-updater)
		// Just set the status directly
		if statusMap, ok := newStatusData.(map[string]interface{}); ok {
			for k, v := range statusMap {
				statusObj[k] = v
			}
		}
	}

	// Update the resource status
	_, err = t.DynamicClient.
		Resource(gvr).
		Namespace(resource.namespace).
		UpdateStatus(ctx, obj, metav1.UpdateOptions{})

	if err != nil {
		return fmt.Errorf("failed to update status for %s: %w", resourceName, err)
	}

	return nil
}
