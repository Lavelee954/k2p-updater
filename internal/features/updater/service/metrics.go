package service

import (
	"k2p-updater/internal/features/updater/domain"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

// MetricsCollector manages Prometheus metrics for the updater
type MetricsCollector struct {
	stateGauge        *prometheus.GaugeVec
	cpuUtilGauge      *prometheus.GaugeVec
	transitionCounter *prometheus.CounterVec
	transitionLatency *prometheus.HistogramVec
	recoveryCounter   *prometheus.CounterVec
	registered        bool
	mu                sync.Mutex
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		stateGauge: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "k2p_node_state",
				Help: "Current state of node in VM spec up process (1=active, 0=inactive)",
			},
			[]string{"node", "state"},
		),
		cpuUtilGauge: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "k2p_node_cpu_utilization",
				Help: "CPU utilization metrics for node",
			},
			[]string{"node", "type"}, // type can be "current" or "window_avg"
		),
		transitionCounter: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "k2p_state_transitions_total",
				Help: "Count of state transitions by node and source/target state",
			},
			[]string{"node", "from_state", "to_state"},
		),
		transitionLatency: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "k2p_state_transition_duration_seconds",
				Help:    "Duration of state transitions",
				Buckets: prometheus.ExponentialBuckets(1, 2, 10), // 1s to ~17m
			},
			[]string{"node", "from_state", "to_state"},
		),
		recoveryCounter: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "k2p_recovery_attempts_total",
				Help: "Count of recovery attempts by node",
			},
			[]string{"node", "success"},
		),
	}
}

// Register registers metrics with Prometheus
func (m *MetricsCollector) Register() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.registered {
		return
	}

	prometheus.MustRegister(m.stateGauge)
	prometheus.MustRegister(m.cpuUtilGauge)
	prometheus.MustRegister(m.transitionCounter)
	prometheus.MustRegister(m.transitionLatency)
	prometheus.MustRegister(m.recoveryCounter)

	m.registered = true
}

// UpdateNodeState updates metrics for node state
func (m *MetricsCollector) UpdateNodeState(nodeName string, oldState, newState domain.State) {
	// Clear all state values for this node
	for _, state := range []domain.State{
		domain.StatePendingVmSpecUp,
		domain.StateMonitoring,
		domain.StateInProgressVmSpecUp,
		domain.StateCompletedVmSpecUp,
		domain.StateFailedVmSpecUp,
		domain.StateCoolDown,
	} {
		m.stateGauge.WithLabelValues(nodeName, string(state)).Set(0)
	}

	// Set the current state to 1
	m.stateGauge.WithLabelValues(nodeName, string(newState)).Set(1)

	// Record the transition if state changed
	if oldState != "" && oldState != newState {
		m.transitionCounter.WithLabelValues(
			nodeName, string(oldState), string(newState)).Inc()
	}
}

// UpdateCPUMetrics updates CPU utilization metrics
func (m *MetricsCollector) UpdateCPUMetrics(nodeName string, current, windowAvg float64) {
	m.cpuUtilGauge.WithLabelValues(nodeName, "current").Set(current)
	m.cpuUtilGauge.WithLabelValues(nodeName, "window_avg").Set(windowAvg)
}

// RecordTransitionLatency records latency of state transition
func (m *MetricsCollector) RecordTransitionLatency(nodeName string, fromState, toState domain.State, durationSeconds float64) {
	m.transitionLatency.WithLabelValues(
		nodeName, string(fromState), string(toState)).Observe(durationSeconds)
}

// RecordRecoveryAttempt records a recovery attempt
func (m *MetricsCollector) RecordRecoveryAttempt(nodeName string, successful bool) {
	successStr := "false"
	if successful {
		successStr = "true"
	}
	m.recoveryCounter.WithLabelValues(nodeName, successStr).Inc()
}
