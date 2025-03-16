package interfaces

import "context"

// NodeDiscoverer defines the interface for discovering control plane nodes
type NodeDiscoverer interface {
	// DiscoverControlPlaneNodes discovers control plane nodes
	DiscoverControlPlaneNodes(ctx context.Context) ([]string, error)
}
