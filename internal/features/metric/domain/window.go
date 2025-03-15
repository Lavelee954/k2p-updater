package domain

import (
	"container/ring"
	"k2p-updater/internal/common"
	"log"
	"time"
)

// NewSlidingWindow creates a new sliding window for CPU measurements
func NewSlidingWindow(windowSize, slidingSize, cooldown time.Duration) *SlidingWindow {
	slots := int(windowSize.Minutes() / slidingSize.Minutes())
	log.Printf("Creating sliding window with %d slots", slots)

	r := ring.New(slots)
	minSamples := int(float64(slots) * 0.5) // Need 50% of slots filled

	return &SlidingWindow{
		WindowSize:     windowSize,
		SlidingSize:    slidingSize,
		Measurements:   r,
		CooldownPeriod: cooldown,
		Initialized:    true,
		CreationTime:   time.Now(),
		MinSamples:     minSamples,
	}
}

// AddMeasurement adds a new CPU measurement to the window
func (sw *SlidingWindow) AddMeasurement(usage float64) {
	sw.Mu.Lock()
	defer sw.Mu.Unlock()

	measurement := CPUMeasurement{
		Timestamp: time.Now(),
		Usage:     usage,
		Valid:     true,
	}

	// Initialize if not already done
	if !sw.Initialized {
		for i := 0; i < sw.Measurements.Len(); i++ {
			sw.Measurements.Value = CPUMeasurement{Valid: false}
			sw.Measurements = sw.Measurements.Next()
		}
		sw.Initialized = true
	}

	sw.Measurements.Value = measurement
	sw.Measurements = sw.Measurements.Next()
}

// CheckScaling determines if scaling is needed based on current measurements
func (sw *SlidingWindow) CheckScaling(triggerPoint float64) (bool, float64, error) {
	sw.Mu.RLock()
	defer sw.Mu.RUnlock()

	if time.Since(sw.LastScaleTime) < sw.CooldownPeriod {
		return false, 0, nil
	}

	values := sw.GetWindowValues()
	log.Printf("CheckScaling checking %d values", len(values))

	if len(values) < sw.MinSamples {
		err := common.NewInsufficientDataError("", len(values), sw.MinSamples)
		log.Printf("Not enough samples: %v", err)
		return false, 0, err
	}

	var sum float64
	for _, v := range values {
		sum += v
	}

	avgUsage := sum / float64(len(values))
	needsScaling := avgUsage > triggerPoint

	return needsScaling, avgUsage, nil
}

// RecordScalingEvent records when a scaling operation occurs
func (sw *SlidingWindow) RecordScalingEvent() {
	sw.Mu.Lock()
	sw.LastScaleTime = time.Now()
	sw.Mu.Unlock()
}

// GetWindowValues returns valid values from the sliding window
func (sw *SlidingWindow) GetWindowValues() []float64 {
	sw.Mu.RLock()
	defer sw.Mu.RUnlock()

	var values []float64
	cutoffTime := time.Now().Add(-sw.WindowSize)

	sw.Measurements.Do(func(val interface{}) {
		if val != nil {
			if measurement, ok := val.(CPUMeasurement); ok && measurement.Valid {
				// Only include valid measurements from within the window period
				if measurement.Timestamp.After(cutoffTime) {
					values = append(values, measurement.Usage)
				}
			}
		}
	})

	return values
}
