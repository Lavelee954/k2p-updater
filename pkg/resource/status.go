package resource

import (
	"context"
	"fmt"
	"github.com/cenkalti/backoff/v4"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"log"
	"time"

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
	// Logging to help with debugging
	log.Printf("StatusInfo.updateGenericInternal called with resourceKey=%s, nodeName=%s",
		resourceKey, nodeName)

	// Get the resource name (always use master for updater resource)
	var resourceName string

	resource, exists := t.Template.Key[resourceKey]
	if !exists {
		return fmt.Errorf("resource with key %s not found in template", resourceKey)
	}

	var definition Definition
	for _, def := range resource.definition {
		definition = def
		break
	}

	// Always use "master" for updater resource
	if resourceKey == "updater" {
		resourceName = fmt.Sprintf(definition.NameFormat, "master")
		log.Printf("Using master resource name: %s for status update", resourceName)
	} else {
		// Existing code for other resources...
	}

	// Get the current resource
	gvr := schema.GroupVersionResource{
		Group:    resource.group,
		Version:  resource.version,
		Resource: definition.Resource,
	}

	obj, err := t.DynamicClient.
		Resource(gvr).
		Namespace(resource.namespace).
		Get(ctx, resourceName, metav1.GetOptions{})

	if err != nil {
		log.Printf("ERROR: Failed to get resource %s: %v", resourceName, err)
		return fmt.Errorf("failed to get resource %s: %w", resourceName, err)
	}

	// Initialize status if it doesn't exist
	if obj.Object["status"] == nil {
		obj.Object["status"] = make(map[string]interface{})
		log.Printf("Initialized new status object for resource %s", resourceName)
	}

	statusObj := obj.Object["status"].(map[string]interface{})

	// Log the incoming update data
	statusDataMap, ok := newStatusData.(map[string]interface{})
	if ok {
		log.Printf("Status update data for node %s: CPU=%.2f%%, state=%s",
			nodeName,
			statusDataMap["cpuWinUsage"],
			statusDataMap["updateStatus"])
	}

	// For updater resource with the nested updates structure
	if resourceKey == "updater" {
		// Get or create the updates array
		var updates []interface{}

		if updatesArr, exists := statusObj["updates"]; exists {
			updates, _ = updatesArr.([]interface{})
			log.Printf("Found existing updates array with %d entries", len(updates))
		}

		if updates == nil {
			updates = []interface{}{}
			log.Printf("Initialized new updates array")
		}

		// Convert new status data to map
		newDetailsMap := make(map[string]interface{})

		// Handle different input types
		if detailsMap, ok := newStatusData.(map[string]interface{}); ok {
			newDetailsMap = detailsMap
		} else if m, ok := newStatusData.(map[interface{}]interface{}); ok {
			for k, v := range m {
				if keyStr, ok := k.(string); ok {
					newDetailsMap[keyStr] = v
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
				log.Printf("Found existing node entry at index %d", i)
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
			log.Printf("Created new node entry at index %d", nodeIndex)
		}

		// Get current details to preserve important fields
		detailsMap, ok := nodeUpdate["details"].(map[string]interface{})
		if !ok {
			detailsMap = make(map[string]interface{})
		}

		// Ensure updateStatus is present
		if existingStatus, exists := detailsMap["updateStatus"]; exists {
			if _, specified := newDetailsMap["updateStatus"]; !specified {
				newDetailsMap["updateStatus"] = existingStatus
			}
		} else if _, specified := newDetailsMap["updateStatus"]; !specified {
			newDetailsMap["updateStatus"] = "Pending"
		}

		// Update the details with new values
		for k, v := range newDetailsMap {
			detailsMap[k] = v
		}

		nodeUpdate["details"] = detailsMap
		updates[nodeIndex] = nodeUpdate

		// Update the status
		statusObj["updates"] = updates
		log.Printf("Updated CR status array with %d entries", len(updates))
	} else {
		// For other types of resources (non-updater)
		if statusMap, ok := newStatusData.(map[string]interface{}); ok {
			for k, v := range statusMap {
				statusObj[k] = v
			}
		}
	}

	// Update the resource status with retry logic
	b := backoff.NewExponentialBackOff()
	b.MaxElapsedTime = 15 * time.Second

	return backoff.Retry(func() error {
		_, err = t.DynamicClient.
			Resource(gvr).
			Namespace(resource.namespace).
			UpdateStatus(ctx, obj, metav1.UpdateOptions{})

		if err != nil {
			log.Printf("Retrying status update for %s: %v", resourceName, err)
			return err
		}

		log.Printf("Successfully updated status for %s", resourceName)
		return nil
	}, b)
}
