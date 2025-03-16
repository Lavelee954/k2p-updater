package service

import (
	"context"
	"errors"
	"fmt"
	"k2p-updater/internal/common"
	"log"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// NodeDiscoverer handles discovery of control plane nodes
type NodeDiscoverer struct {
	kubeClient    kubernetes.Interface
	namespace     string
	labelSelector string
}

// NewNodeDiscoverer creates a new node discoverer
func NewNodeDiscoverer(kubeClient kubernetes.Interface, namespace string) *NodeDiscoverer {
	return &NodeDiscoverer{
		kubeClient:    kubeClient,
		namespace:     namespace,
		labelSelector: "node-role.kubernetes.io/control-plane=",
	}
}

// DiscoverControlPlaneNodes discovers control plane nodes from Kubernetes API
func (d *NodeDiscoverer) DiscoverControlPlaneNodes(ctx context.Context) ([]string, error) {
	// Check for context cancellation
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	if d.kubeClient == nil {
		return nil, common.NotInitializedError("Kubernetes client not initialized")
	}

	// List nodes with control plane role label
	nodes, err := d.kubeClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{
		LabelSelector: d.labelSelector,
	})

	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("failed to list control plane nodes: %w", err)
	}

	if len(nodes.Items) == 0 {
		log.Printf("Warning: No control plane nodes found with label selector: %s", d.labelSelector)
		return []string{}, nil
	}

	// Extract node names
	var nodeNames []string
	for _, node := range nodes.Items {
		nodeNames = append(nodeNames, node.Name)

		// Log node details for debugging
		readyCondition := "Unknown"
		for _, condition := range node.Status.Conditions {
			if condition.Type == "Ready" {
				readyCondition = string(condition.Status)
				break
			}
		}

		log.Printf("Discovered control plane node: %s (Ready: %s)",
			node.Name, readyCondition)
	}

	log.Printf("Total control plane nodes discovered: %d", len(nodeNames))
	return nodeNames, nil
}
