package service

import (
	domainMetric "k2p-updater/internal/features/metric/domain"
	"log"
)

// CPUUtilizationCalculator implements domain.Calculator
type CPUUtilizationCalculator struct{}

// CalculateUtilization calculates CPU usage percentages
func (c *CPUUtilizationCalculator) CalculateUtilization(
	current domainMetric.CPUStats,
	cpuID string,
	nodeName string,
	previousStats map[string]map[string]domainMetric.CPUStats,
) (user, system, idle float64) {
	if prev, exists := previousStats[nodeName]; exists {
		if prevStats, exists := prev[cpuID]; exists {
			timeDelta := current.Timestamp.Sub(prevStats.Timestamp).Seconds()

			// Calculate CPU time deltas
			userDelta := current.User - prevStats.User
			systemDelta := current.System - prevStats.System
			idleDelta := current.Idle - prevStats.Idle
			totalDelta := userDelta + systemDelta + idleDelta

			if totalDelta > 0 && timeDelta > 0 {
				// Calculate percentages
				user = (userDelta / totalDelta) * 100
				system = (systemDelta / totalDelta) * 100
				idle = (idleDelta / totalDelta) * 100

				log.Printf("Node: %s, CPU: %s - Usage: %.2f%% (User: %.2f%%, System: %.2f%%, Idle: %.2f%%)",
					nodeName, cpuID, user+system, user, system, idle)
			}
		}
	}

	// Store current values for next calculation
	if _, exists := previousStats[nodeName]; !exists {
		previousStats[nodeName] = make(map[string]domainMetric.CPUStats)
	}
	previousStats[nodeName][cpuID] = current

	return
}
