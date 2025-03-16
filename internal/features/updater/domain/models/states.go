package models

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

// Common constants
const (
	UpdateKey    string = "updater"
	ResourceName string = "master"
)

// MetricsState represents the state of metrics collection
type MetricsState string

// Metric collection states
const (
	MetricsInitializing MetricsState = "Initializing"
	MetricsCollecting   MetricsState = "Collecting"
	MetricsReady        MetricsState = "Ready"
	MetricsError        MetricsState = "Error"
)
