// event.go
package resource

import (
	"context"
	"fmt"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

type Event interface {
	NormalRecord(ctx context.Context, resourceKey, reason, message string, args ...interface{}) error
	WarningRecord(ctx context.Context, resourceKey, reason, message string, args ...interface{}) error
	NormalRecordWithNode(ctx context.Context, resourceKey, nodeName, reason, message string, args ...interface{}) error
	WarningRecordWithNode(ctx context.Context, resourceKey, nodeName, reason, message string, args ...interface{}) error
}

// KubeClientInterface is an interface that defines only the necessary methods of a Kubernetes clientset.
// This interface implements both real clientsets and fake clientsets.
type KubeClientInterface interface {
	CoreV1() typedcorev1.CoreV1Interface
}

type EventInfo struct {
	Template      *Template
	KubeClient    KubeClientInterface
	DynamicClient dynamic.Interface
}

// NormalRecord creates a normal event for the specified resource
func (e EventInfo) NormalRecord(ctx context.Context, resourceKey, reason, message string, args ...interface{}) error {
	return e.normalRecordInternal(ctx, resourceKey, "", reason, message, args...)
}

// NormalRecordWithNode creates a normal event for the specified resource with a node name
func (e EventInfo) normalRecordInternal(ctx context.Context, resourceKey, nodeName, reason, message string, args ...interface{}) error {
	// 1. Get the information registered in the template by resourceKey
	resource, exists := e.Template.Key[resourceKey]
	if !exists {
		return fmt.Errorf("resource with key %s not found in template", resourceKey)
	}

	// Get the definition for this resource
	var definition Definition
	for _, def := range resource.definition {
		definition = def
		break
	}

	// 2. Set the required resource name based on cr_name configuration
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

	// Create GVR to fetch the resource
	gvr := schema.GroupVersionResource{
		Group:    resource.group,
		Version:  resource.version,
		Resource: definition.Resource,
	}

	// Get the actual CR to access its UID and ResourceVersion
	obj, err := e.DynamicClient.Resource(gvr).Namespace(resource.namespace).Get(ctx, resourceName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get resource %s: %w", resourceName, err)
	}

	// 3. Register a normal event with proper resource linking
	formattedMessage := fmt.Sprintf(message, args...)

	event := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", resourceName, time.Now().Format("20060102-150405")),
			Namespace: resource.namespace,
		},
		InvolvedObject: corev1.ObjectReference{
			Kind:            definition.Kind,
			Namespace:       resource.namespace,
			Name:            resourceName,
			APIVersion:      fmt.Sprintf("%s/%s", resource.group, resource.version),
			UID:             obj.GetUID(),
			ResourceVersion: obj.GetResourceVersion(),
		},
		Reason:  reason,
		Message: formattedMessage,
		Type:    corev1.EventTypeNormal,
	}

	_, err = e.KubeClient.CoreV1().Events(resource.namespace).Create(ctx, event, metav1.CreateOptions{})
	return err
}

// WarningRecord creates a warning event for the specified resource
func (e EventInfo) WarningRecord(ctx context.Context, resourceKey, reason, message string, args ...interface{}) error {
	return e.warningRecordInternal(ctx, resourceKey, "", reason, message, args...)
}

// NormalRecordWithNode method implementation
func (e EventInfo) NormalRecordWithNode(ctx context.Context, resourceKey, nodeName, reason, message string, args ...interface{}) error {
	return e.normalRecordInternal(ctx, resourceKey, nodeName, reason, message, args...)
}

// WarningRecordWithNode method implementation
func (e EventInfo) WarningRecordWithNode(ctx context.Context, resourceKey, nodeName, reason, message string, args ...interface{}) error {
	return e.warningRecordInternal(ctx, resourceKey, nodeName, reason, message, args...)
}

// warningRecordInternal implements the logic for creating warning events
func (e EventInfo) warningRecordInternal(ctx context.Context, resourceKey, nodeName, reason, message string, args ...interface{}) error {
	// 1. Get the information registered in the template by resourceKey
	resource, exists := e.Template.Key[resourceKey]
	if !exists {
		return fmt.Errorf("resource with key %s not found in template", resourceKey)
	}

	// Get the definition for this resource
	var definition Definition
	for _, def := range resource.definition {
		definition = def
		break
	}

	// 2. Set the required resource name based on cr_name configuration
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

	// Create GVR to fetch the resource
	gvr := schema.GroupVersionResource{
		Group:    resource.group,
		Version:  resource.version,
		Resource: definition.Resource,
	}

	// Get the actual CR to access its UID and ResourceVersion
	obj, err := e.DynamicClient.Resource(gvr).Namespace(resource.namespace).Get(ctx, resourceName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get resource %s: %w", resourceName, err)
	}

	// 3. Register a warning event with proper resource linking
	formattedMessage := fmt.Sprintf(message, args...)

	event := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", resourceName, time.Now().Format("20060102-150405")),
			Namespace: resource.namespace,
		},
		InvolvedObject: corev1.ObjectReference{
			Kind:            definition.Kind,
			Namespace:       resource.namespace,
			Name:            resourceName,
			APIVersion:      fmt.Sprintf("%s/%s", resource.group, resource.version),
			UID:             obj.GetUID(),
			ResourceVersion: obj.GetResourceVersion(),
		},
		Reason:  reason,
		Message: formattedMessage,
		Type:    corev1.EventTypeWarning,
	}

	_, err = e.KubeClient.CoreV1().Events(resource.namespace).Create(ctx, event, metav1.CreateOptions{})
	return err
}
