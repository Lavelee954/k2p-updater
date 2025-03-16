package domain

import (
	"time"
)

// State represents the VM spec up state
type State string

// VM spec up states
const (
	StatePendingVmSpecUp    State = "PendingVmSpecUp"
	StateMonitoring         State = "Monitoring"
	StateInProgressVmSpecUp State = "InProgressVmSpecUp"
	StateCompletedVmSpecUp  State = "CompletedVmSpecUp"
	StateFailedVmSpecUp     State = "FailedVmSpecUp"
	StateCoolDown           State = "CoolDown"
)

const (
	UpdateKey    string = "updater"
	ResourceName string = "master"
)

// ControlPlaneStatus represents the current status of a control plane node
type ControlPlaneStatus struct {
	// NodeName is the Kubernetes node name
	NodeName string `json:"nodeName"`

	// CurrentState is the current state of the VM spec up process
	CurrentState State `json:"currentState"`

	// LastTransitionTime is when the last state transition occurred
	LastTransitionTime time.Time `json:"lastTransitionTime"`

	// Message contains additional information about the current state
	Message string `json:"message"`

	// CPUUtilization is the current CPU utilization percentage
	CPUUtilization float64 `json:"cpuUtilization"`

	// WindowAverageUtilization is the average CPU utilization over the window period
	WindowAverageUtilization float64 `json:"windowAverageUtilization"`

	// CoolDownEndTime is when the cooldown period will end
	CoolDownEndTime time.Time `json:"coolDownEndTime,omitempty"`

	// SpecUpRequested indicates if a spec up has been requested
	SpecUpRequested bool `json:"specUpRequested"`

	// SpecUpCompleted indicates if the spec up has been completed
	SpecUpCompleted bool `json:"specUpCompleted"`

	// HealthCheckPassed indicates if the health check passed after spec up
	HealthCheckPassed bool `json:"healthCheckPassed"`
}

// UpdaterConfig contains configuration for the updater
type UpdaterConfig struct {
	// ScaleThreshold is the CPU threshold for scaling
	ScaleThreshold float64

	// ScaleUpStep is the amount to scale up
	ScaleUpStep int32

	// CooldownPeriod is the cooldown period after scaling
	CooldownPeriod time.Duration

	// MonitoringInterval is how often to update monitoring status
	MonitoringInterval time.Duration

	// CooldownUpdateInterval is how often to update cooldown status
	CooldownUpdateInterval time.Duration

	// Namespace is the Kubernetes namespace to operate in
	Namespace string
}
