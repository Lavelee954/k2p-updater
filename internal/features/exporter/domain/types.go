package domain

import (
	"time"
)

// Status represents the status of a node-exporter
type Status string

// Exporter status constants
const (
	StatusRunning Status = "Running"
	StatusFailed  Status = "Failed"
	StatusUnknown Status = "Unknown"
)

// NodeExporter represents a node-exporter pod that provides metrics
type NodeExporter struct {
	// Name is the pod name
	Name string
	// IP is the pod IP address
	IP string
	// NodeName is the Kubernetes node name where the exporter is running
	NodeName string
	// LastSeen is when the exporter was last known to be healthy
	LastSeen time.Time
	// Status indicates the current exporter status
	Status Status
	// RetryCount tracks how much consecutive health check failures occurred
	RetryCount int
}
