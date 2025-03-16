package resource

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/cenkalti/backoff/v4"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

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
	// Check for context cancellation first
	if ctx.Err() != nil {
		return fmt.Errorf("context canceled before recording normal event: %w", ctx.Err())
	}
	return e.normalRecordInternal(ctx, resourceKey, "", reason, message, args...)
}

// NormalRecordWithNode creates a normal event for the specified resource with a node name
func (e EventInfo) normalRecordInternal(ctx context.Context, resourceKey, nodeName, reason, message string, args ...interface{}) error {
	// Check for context cancellation first
	if ctx.Err() != nil {
		return fmt.Errorf("context canceled before recording internal normal event: %w", ctx.Err())
	}

	// Add backoff/retry logic
	b := backoff.NewExponentialBackOff()
	b.MaxElapsedTime = 15 * time.Second

	var finalErr error

	// Operation to retry
	operation := func() error {
		// Check context cancellation before each attempt
		if ctx.Err() != nil {
			return backoff.Permanent(fmt.Errorf("context canceled during event creation: %w", ctx.Err()))
		}

		// Log input parameters
		log.Printf("Creating normal event: resourceKey=%s, nodeName=%s, reason=%s", resourceKey, nodeName, reason)

		// Get resource definition
		resource, exists := e.Template.Key[resourceKey]
		if !exists {
			return backoff.Permanent(fmt.Errorf("resource with key %s not found in template", resourceKey))
		}

		// Get the definition
		var definition Definition
		for _, def := range resource.definition {
			definition = def
			break
		}

		// Always use "master" for updater resource
		var resourceName string
		if resourceKey == "updater" {
			resourceName = fmt.Sprintf(definition.NameFormat, "master")
			log.Printf("Using master resource name: %s", resourceName)
		} else if definition.CRName == "%s" && nodeName != "" {
			resourceName = fmt.Sprintf(definition.NameFormat, nodeName)
		} else if definition.CRName != "" {
			resourceName = fmt.Sprintf(definition.NameFormat, definition.CRName)
		} else {
			resourceName = fmt.Sprintf(definition.NameFormat, resourceKey)
		}

		// Get the resource
		gvr := schema.GroupVersionResource{
			Group:    resource.group,
			Version:  resource.version,
			Resource: definition.Resource,
		}

		obj, err := e.DynamicClient.Resource(gvr).Namespace(resource.namespace).Get(ctx, resourceName, metav1.GetOptions{})
		if err != nil {
			log.Printf("Retry: Failed to get resource %s: %v", resourceName, err)
			return err // Retry this error
		}

		// Create the event
		formattedMessage := fmt.Sprintf(message, args...)
		eventName := fmt.Sprintf("%s-%s-%s", resourceName, reason, time.Now().Format("20060102-150405.000"))

		event := &corev1.Event{
			ObjectMeta: metav1.ObjectMeta{
				Name:      eventName,
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
			Reason:         reason,
			Message:        formattedMessage,
			Type:           corev1.EventTypeNormal,
			FirstTimestamp: metav1.Now(),
			LastTimestamp:  metav1.Now(),
			Count:          1,
			Source: corev1.EventSource{
				Component: "k2p-updater",
			},
		}

		_, err = e.KubeClient.CoreV1().Events(resource.namespace).Create(ctx, event, metav1.CreateOptions{})
		if err != nil {
			log.Printf("Retry: Failed to create event: %v", err)
			return err // Retry this error
		}

		log.Printf("Successfully created event for node %s", nodeName)
		return nil
	}

	// Execute with backoff
	err := backoff.Retry(operation, b)
	if err != nil {
		finalErr = err
		log.Printf("ERROR: Failed to create event after retries: %v", err)
	}

	return finalErr
}

// WarningRecord creates a warning event for the specified resource
func (e EventInfo) WarningRecord(ctx context.Context, resourceKey, reason, message string, args ...interface{}) error {
	// Check for context cancellation first
	if ctx.Err() != nil {
		return fmt.Errorf("context canceled before recording warning event: %w", ctx.Err())
	}
	return e.warningRecordInternal(ctx, resourceKey, "", reason, message, args...)
}

// NormalRecordWithNode method implementation
func (e EventInfo) NormalRecordWithNode(ctx context.Context, resourceKey, nodeName, reason, message string, args ...interface{}) error {
	// Check for context cancellation first
	if ctx.Err() != nil {
		return fmt.Errorf("context canceled before recording normal event with node: %w", ctx.Err())
	}
	return e.normalRecordInternal(ctx, resourceKey, nodeName, reason, message, args...)
}

// WarningRecordWithNode method implementation
func (e EventInfo) WarningRecordWithNode(ctx context.Context, resourceKey, nodeName, reason, message string, args ...interface{}) error {
	// Check for context cancellation first
	if ctx.Err() != nil {
		return fmt.Errorf("context canceled before recording warning event with node: %w", ctx.Err())
	}
	return e.warningRecordInternal(ctx, resourceKey, nodeName, reason, message, args...)
}

// warningRecordInternal implements the logic for creating warning events
func (e EventInfo) warningRecordInternal(ctx context.Context, resourceKey, nodeName, reason, message string, args ...interface{}) error {
	// Check for context cancellation first
	if ctx.Err() != nil {
		return fmt.Errorf("context canceled before internal warning event recording: %w", ctx.Err())
	}

	// Log input parameters at the beginning
	log.Printf("Creating warning event: resourceKey=%s, nodeName=%s, reason=%s", resourceKey, nodeName, reason)

	// 1. Get the information registered in the template by resourceKey
	resource, exists := e.Template.Key[resourceKey]
	if !exists {
		err := fmt.Errorf("resource with key %s not found in template", resourceKey)
		log.Printf("EVENT ERROR: %v", err)
		return err
	}

	// Log successful resource lookup
	log.Printf("Found resource definition: resourceKey=%s, namespace=%s, group=%s, version=%s",
		resourceKey, resource.namespace, resource.group, resource.version)

	// Get the definition for this resource
	var definition Definition
	definitionFound := false
	for _, def := range resource.definition {
		definition = def
		definitionFound = true
		break
	}

	if !definitionFound {
		err := fmt.Errorf("no definition found for resource with key %s", resourceKey)
		log.Printf("EVENT ERROR: %v", err)
		return err
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

	log.Printf("Resolved resource name: %s", resourceName)

	// Create GVR to fetch the resource
	gvr := schema.GroupVersionResource{
		Group:    resource.group,
		Version:  resource.version,
		Resource: definition.Resource,
	}

	log.Printf("Fetching resource using GVR: group=%s, version=%s, resource=%s, namespace=%s, name=%s",
		gvr.Group, gvr.Version, gvr.Resource, resource.namespace, resourceName)

	// Get the actual CR to access its UID and ResourceVersion
	obj, err := e.DynamicClient.Resource(gvr).Namespace(resource.namespace).Get(ctx, resourceName, metav1.GetOptions{})
	if err != nil {
		log.Printf("EVENT ERROR: Failed to get resource %s: %v", resourceName, err)
		return fmt.Errorf("failed to get resource %s: %w", resourceName, err)
	}

	log.Printf("Successfully fetched resource: %s (UID: %s, ResourceVersion: %s)",
		resourceName, obj.GetUID(), obj.GetResourceVersion())

	// 3. Register a normal event with proper resource linking
	formattedMessage := fmt.Sprintf(message, args...)

	// Create unique event name with timestamp to prevent overwrites
	eventName := fmt.Sprintf("%s-%s-%s", resourceName, reason, time.Now().Format(time.RFC3339))

	event := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      eventName,
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
		// Add required fields for event v1 API
		FirstTimestamp: metav1.Now(),
		LastTimestamp:  metav1.Now(),
		Count:          1,
		Source: corev1.EventSource{
			Component: "k2p-updater",
		},
	}

	log.Printf("Creating event with name: %s, reason: %s, message: %s", eventName, reason, formattedMessage)

	_, err = e.KubeClient.CoreV1().Events(resource.namespace).Create(ctx, event, metav1.CreateOptions{})
	if err != nil {
		log.Printf("EVENT ERROR: Failed to create event for resource %s: %v", resourceName, err)
		return err
	}

	log.Printf("Successfully created normal event for resource %s", resourceName)
	return nil
}
