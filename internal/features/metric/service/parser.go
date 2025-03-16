package service

import (
	"fmt"
	"k2p-updater/internal/features/metric/domain"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// CPUMetricsParser implements domain.Parser
type CPUMetricsParser struct{}

// ParseCPUMetrics parses raw metrics data
func (p *CPUMetricsParser) ParseCPUMetrics(metrics string, nodeName string) (*domain.NodeCPUMetrics, error) {
	result := &domain.NodeCPUMetrics{
		NodeName: nodeName,
		CPUs:     make(map[string]domain.CPUStats),
	}

	lines := strings.Split(metrics, "\n")

	for _, line := range lines {
		if !strings.Contains(line, "node_cpu_seconds_total") {
			continue
		}

		cpuNum := p.extractLabel(line, "cpu")
		mode := p.extractLabel(line, "mode")
		value := p.extractValue(line)

		stats := result.CPUs[cpuNum]
		stats.Timestamp = time.Now()

		switch mode {
		case "user":
			stats.User = value
		case "system":
			stats.System = value
		case "idle":
			stats.Idle = value
		}
		result.CPUs[cpuNum] = stats
	}

	return result, nil
}

// extractLabel extracts a label value from a metrics line
func (p *CPUMetricsParser) extractLabel(line, label string) string {
	pattern := fmt.Sprintf(`%s="([^"]+)"`, label)
	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(line)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// extractValue extracts the numeric value from a metrics line
func (p *CPUMetricsParser) extractValue(line string) float64 {
	parts := strings.Split(line, " ")
	if len(parts) != 2 {
		return 0
	}
	val, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return 0
	}
	return val
}
