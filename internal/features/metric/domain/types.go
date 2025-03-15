package domain

import (
	"container/ring"
	"sync"
	"time"
)

// CPUStats represents CPU statistics for a single measurement
type CPUStats struct {
	// User is the CPU time spent in user mode
	User float64
	// System is the CPU time spent in system mode
	System float64
	// Idle is the CPU time spent idle
	Idle float64
	// Timestamp is when the measurement was taken
	Timestamp time.Time
}

// NodeCPUMetrics represents CPU metrics for an entire node
type NodeCPUMetrics struct {
	// NodeName is the name of the node
	NodeName string
	// CPUs is a map of CPU ID to CPU stats
	CPUs map[string]CPUStats
}

// CPUAlert indicates a high CPU utilization alert
type CPUAlert struct {
	NodeName      string
	CurrentUsage  float64
	WindowAverage float64
	Timestamp     time.Time
}

// CPUMeasurement represents a single CPU measurement point
type CPUMeasurement struct {
	// Timestamp is when the measurement was taken
	Timestamp time.Time
	// Usage is the CPU usage percentage
	Usage float64
	// Valid indicates if this is a valid measurement
	Valid bool
}

// SlidingWindow manages CPU measurements with sliding window logic
type SlidingWindow struct {
	// WindowSize is the total time window to consider
	WindowSize time.Duration
	// SlidingSize is the interval between measurements
	SlidingSize time.Duration
	// Measurements is the ring buffer of measurements
	Measurements *ring.Ring
	// Mu protects concurrent access to the window
	Mu sync.RWMutex
	// LastScaleTime is when scaling was last performed
	LastScaleTime time.Time
	// CooldownPeriod is the waiting period after scaling
	CooldownPeriod time.Duration
	// Initialized tracks if the window has been initialized
	Initialized bool
	// CreationTime is when the window was created
	CreationTime time.Time
	// MinSamples is the minimum number of samples required for valid analysis
	MinSamples int
}

// MinuteAverage represents rolling minute-based averages
type MinuteAverage struct {
	// Sum is the sum of measurements
	Sum float64
	// Count is the number of measurements
	Count int
	// StartTime is when the averaging period started
	StartTime time.Time
}
