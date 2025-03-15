package resource

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

type Event interface {
	NormalRecord(ctx context.Context, resourceKey, reason, message string, args ...interface{}) error
	WarningRecord(ctx context.Context, resourceKey, reason, message string, args ...interface{}) error
}

// KubeClientInterface is an interface that defines only the necessary methods of a Kubernetes clientset.
// This interface implements both real clientsets and fake clientsets.
type KubeClientInterface interface {
	CoreV1() typedcorev1.CoreV1Interface
}

type EventInfo struct {
	Template   *Template
	KubeClient KubeClientInterface
}

func (e EventInfo) NormalRecord(ctx context.Context, resourceKey, reason, message string, args ...interface{}) error {
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

	// 2. Set the required GVR for Kubernetes events with template information
	resourceName := fmt.Sprintf(definition.NameFormat, resourceKey)

	// 3. Register a normal event
	formattedMessage := fmt.Sprintf(message, args...)

	event := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", resourceName, time.Now().Format("20060102-150405")),
			Namespace: resource.namespace,
		},
		InvolvedObject: corev1.ObjectReference{
			Kind:       definition.Kind,
			Namespace:  resource.namespace,
			Name:       resourceName,
			APIVersion: fmt.Sprintf("%s/%s", resource.group, resource.version),
		},
		Reason:  reason,
		Message: formattedMessage,
		Type:    corev1.EventTypeNormal,
	}

	_, err := e.KubeClient.CoreV1().Events(resource.namespace).Create(ctx, event, metav1.CreateOptions{})
	return err
}

func (e EventInfo) WarningRecord(ctx context.Context, resourceKey, reason, message string, args ...interface{}) error {
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

	// 2. Set the required GVR for Kubernetes events with template information
	resourceName := fmt.Sprintf(definition.NameFormat, resourceKey)

	// 3. Register an alert event
	formattedMessage := fmt.Sprintf(message, args...)

	event := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", resourceName, time.Now().Format("20060102-150405")),
			Namespace: resource.namespace,
		},
		InvolvedObject: corev1.ObjectReference{
			Kind:       definition.Kind,
			Namespace:  resource.namespace,
			Name:       resourceName,
			APIVersion: fmt.Sprintf("%s/%s", resource.group, resource.version),
		},
		Reason:  reason,
		Message: formattedMessage,
		Type:    corev1.EventTypeWarning,
	}

	_, err := e.KubeClient.CoreV1().Events(resource.namespace).Create(ctx, event, metav1.CreateOptions{})
	return err
}
