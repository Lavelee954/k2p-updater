package service

import (
	"context"
	"fmt"
	"k2p-updater/cmd/app"
	"k2p-updater/internal/common"
	domainExporter "k2p-updater/internal/features/exporter/domain"
	domainMetric "k2p-updater/internal/features/metric/domain"
	"log"
	"sync"
	"time"

	"k8s.io/client-go/kubernetes"
)

// Service handles metrics collection and analysis.
type Service struct {
	client           kubernetes.Interface
	exporterService  domainExporter.Provider
	windows          map[string]*domainMetric.SlidingWindow
	previousStats    map[string]map[string]domainMetric.CPUStats
	nodeAverages     map[string]*domainMetric.MinuteAverage
	config           app.MetricsConfig
	mu               sync.RWMutex
	alertSubscribers []chan domainMetric.CPUAlert
	alertMu          sync.RWMutex
	// Components aligned with domain interfaces
	fetcher    domainMetric.Fetcher
	parser     domainMetric.Parser
	calculator domainMetric.Calculator
}

// NewMetricsService creates a new metrics service.
func NewMetricsService(
	client kubernetes.Interface,
	config app.MetricsConfig,
	exporterService domainExporter.Provider,
) *Service {
	// Create new components
	fetcher := newNodeMetricsFetcher(10 * time.Second)
	parser := newCPUMetricsParser()
	calculator := newCPUUtilizationCalculator()

	return &Service{
		client:           client,
		exporterService:  exporterService,
		windows:          make(map[string]*domainMetric.SlidingWindow),
		previousStats:    make(map[string]map[string]domainMetric.CPUStats),
		nodeAverages:     make(map[string]*domainMetric.MinuteAverage),
		config:           config,
		alertSubscribers: []chan domainMetric.CPUAlert{},
		fetcher:          fetcher,
		parser:           parser,
		calculator:       calculator,
	}
}

// Start begins metrics collection for all nodes.
func (s *Service) Start(ctx context.Context) error {
	// Wait for the ExporterService to initialize
	if err := s.exporterService.WaitForInitialization(ctx); err != nil {
		return fmt.Errorf("failed to wait for exporter service initialization: %w", err)
	}

	// Get healthy exporters
	exporters := s.exporterService.GetHealthyExporters()
	if len(exporters) == 0 {
		return fmt.Errorf("no healthy node exporters found after initialization")
	}

	log.Printf("Found %d healthy exporters", len(exporters))
	log.Printf("Initializing windows for %d nodes...", len(exporters))

	// Initialize monitoring windows for each node
	for _, exporter := range exporters {
		s.windows[exporter.NodeName] = domainMetric.NewSlidingWindow(
			s.config.WindowSize,
			s.config.SlidingSize,
			s.config.CooldownPeriod,
		)
		s.previousStats[exporter.NodeName] = make(map[string]domainMetric.CPUStats)
		log.Printf("Window initialized for node: %s", exporter.NodeName)
	}

	log.Println("\nCollecting initial baseline metrics...")
	time.Sleep(2 * time.Second)

	// Collect initial metrics
	for _, exporter := range exporters {
		metrics, err := s.fetcher.FetchNodeMetrics(ctx, exporter)
		if err != nil {
			log.Printf("Error collecting initial metrics from %s: %v", exporter.NodeName, err)
			continue
		}

		cpuMetrics, err := s.parser.ParseCPUMetrics(metrics, exporter.NodeName)
		if err != nil {
			continue
		}

		if _, exists := s.previousStats[exporter.NodeName]; !exists {
			s.previousStats[exporter.NodeName] = make(map[string]domainMetric.CPUStats)
		}
		for cpuNum, stats := range cpuMetrics.CPUs {
			stats.Timestamp = time.Now()
			s.previousStats[exporter.NodeName][cpuNum] = stats
		}
	}

	// Start regular monitoring
	ticker := time.NewTicker(s.config.SlidingSize)
	defer ticker.Stop()

	log.Println("\nStarting regular monitoring...")
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			// Collect metrics from all healthy exporters
			healthyExporters := s.exporterService.GetHealthyExporters()
			s.collectMetrics(ctx, healthyExporters)
		}
	}
}

// GetNodeCPUUsage returns the current CPU usage for a node.
func (s *Service) GetNodeCPUUsage(nodeName string) (float64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	avg, exists := s.nodeAverages[nodeName]
	if !exists {
		return 0, common.NewNodeNotFoundError(nodeName)
	}
	if avg.Count == 0 {
		return 0, common.NewMetricsUnavailableError(nodeName, "no measurements collected")
	}

	return avg.Sum / float64(avg.Count), nil
}

// GetWindowAverageCPU returns the window average CPU for a node.
func (s *Service) GetWindowAverageCPU(nodeName string) (float64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	window, exists := s.windows[nodeName]
	if !exists {
		return 0, common.NewNodeNotFoundError(nodeName)
	}

	values := window.GetWindowValues()
	if len(values) == 0 {
		return 0, common.NewInsufficientDataError(nodeName, 0, window.MinSamples)
	}

	var sum float64
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values)), nil
}

// SubscribeHighCPUAlerts returns a channel for CPU alerts.
func (s *Service) SubscribeHighCPUAlerts() <-chan domainMetric.CPUAlert {
	s.alertMu.Lock()
	defer s.alertMu.Unlock()

	ch := make(chan domainMetric.CPUAlert, 10)
	s.alertSubscribers = append(s.alertSubscribers, ch)
	return ch
}

// GetWindow returns the sliding window for a node.
func (s *Service) GetWindow(nodeName string) (*domainMetric.SlidingWindow, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	window, exists := s.windows[nodeName]
	return window, exists
}

// notifyAlertSubscribers sends alerts to all subscribers.
func (s *Service) notifyAlertSubscribers(alert domainMetric.CPUAlert) {
	s.alertMu.RLock()
	defer s.alertMu.RUnlock()

	log.Printf("\n==== Preparing to notify %d alert subscribers ====", len(s.alertSubscribers))

	// Update window average
	windowValues := s.windows[alert.NodeName].GetWindowValues()
	if len(windowValues) > 0 {
		sum := 0.0
		for _, v := range windowValues {
			sum += v
		}
		alert.WindowAverage = sum / float64(len(windowValues))
	}

	for i, ch := range s.alertSubscribers {
		select {
		case ch <- alert:
			log.Printf("Alert #%d: Sent CPU alert for node %s (current: %.2f%%, avg: %.2f%%)",
				i+1, alert.NodeName, alert.CurrentUsage, alert.WindowAverage)
		default:
			log.Printf("Alert #%d: Channel full, skipping notification for node %s",
				i+1, alert.NodeName)
		}
	}

	if len(s.alertSubscribers) == 0 {
		log.Println("No alert subscribers registered, metrics are collected but not being monitored")
	}
}

// collectMetrics gathers metrics from all node exporters.
func (s *Service) collectMetrics(ctx context.Context, exporters []domainExporter.NodeExporter) {
	log.Println("\n==== Collecting metrics from exporters ====")
	log.Printf("Found %d exporters to collect from", len(exporters))

	if len(exporters) == 0 {
		log.Println("No healthy exporters found, cannot collect metrics")
		return
	}

	for _, exporter := range exporters {
		log.Printf("Attempting to collect metrics from %s (Node: %s)", exporter.Name, exporter.NodeName)

		metrics, err := s.fetcher.FetchNodeMetrics(ctx, exporter)
		if err != nil {
			log.Printf("Error collecting metrics from %s: %v", exporter.NodeName, err)
			continue
		}
		log.Printf("Successfully collected metrics from %s", exporter.NodeName)

		cpuMetrics, err := s.parser.ParseCPUMetrics(metrics, exporter.NodeName)
		if err != nil {
			log.Printf("Error parsing metrics from %s: %v", exporter.NodeName, err)
			continue
		}
		log.Printf("Parsed CPU metrics for %s: %d CPU cores found",
			exporter.NodeName, len(cpuMetrics.CPUs))

		s.processNodeMetrics(exporter.NodeName, cpuMetrics)
	}
	log.Println("==== Metrics collection completed ====")
}

// processNodeMetrics processes metrics for a specific node.
func (s *Service) processNodeMetrics(nodeName string, metrics *domainMetric.NodeCPUMetrics) {
	s.mu.Lock()
	defer s.mu.Unlock()

	window := s.windows[nodeName]
	if window == nil {
		log.Printf("Warning: No window initialized for node %s", nodeName)
		return
	}

	var totalCPUUsage float64
	var cpuCount float64

	// Calculate total CPU usage
	for cpuID, stats := range metrics.CPUs {
		user, system, idle := s.calculator.CalculateUtilization(stats, cpuID, nodeName, s.previousStats)
		if user+system+idle > 0 {
			cpuUsage := user + system
			totalCPUUsage += cpuUsage
			cpuCount++
		}
	}

	if cpuCount > 0 {
		currentUsage := totalCPUUsage / cpuCount

		// Calculate minute average
		minuteAvg := s.calculateMinuteAverage(nodeName, currentUsage)

		// Add measurement to window
		window.AddMeasurement(minuteAvg)

		// Get window values and prepare output
		windowValues := window.GetWindowValues()
		windowAge := time.Since(window.CreationTime)
		hasSufficientSamples := len(windowValues) >= window.MinSamples

		// Log basic information
		log.Printf("\nNode: %s at %s", nodeName, time.Now().Format("15:04:05"))
		log.Printf("Current CPU Usage: %.2f%%", currentUsage)
		log.Printf("Current Minute Average: %.2f%%", minuteAvg)
		log.Printf("Current Window Age: %s", windowAge.Round(time.Second))

		// Calculate and display window average
		var windowAvg float64
		if len(windowValues) > 0 {
			sum := 0.0
			for _, v := range windowValues {
				sum += v
			}
			windowAvg = sum / float64(len(windowValues))

			if hasSufficientSamples {
				log.Printf("Window Average (10min): %.2f%%", windowAvg)
			} else {
				log.Printf("Window Average (10min): %.2f%% (collecting: %d/%d samples)",
					windowAvg, len(windowValues), window.MinSamples)
			}
		} else {
			log.Printf("Window Average (10min): Initializing...")
		}

		// Log window values
		log.Printf("\nSliding Window Values (oldest to newest):")
		for i, value := range windowValues {
			log.Printf("  Slot %2d: %.2f%%", i+1, value)
		}

		// Create CPU alert
		alert := domainMetric.CPUAlert{
			NodeName:      nodeName,
			CurrentUsage:  currentUsage,
			WindowAverage: windowAvg,
			Timestamp:     time.Now(),
		}

		// Check scaling threshold and notify
		needsScaling, avgUsage, _ := window.CheckScaling(s.config.ScaleTrigger)
		if needsScaling {
			log.Printf("\n⚠️  ALERT: High CPU usage detected (%.2f%% > %.2f%%)",
				avgUsage, s.config.ScaleTrigger)
			window.RecordScalingEvent()
		}

		// Always send alerts regardless of CPU usage
		s.notifyAlertSubscribers(alert)
	}
}

// calculateMinuteAverage calculates the rolling minute average.
func (s *Service) calculateMinuteAverage(nodeName string, currentUsage float64) float64 {
	avg, exists := s.nodeAverages[nodeName]
	if !exists {
		s.nodeAverages[nodeName] = &domainMetric.MinuteAverage{
			StartTime: time.Now(),
		}
		avg = s.nodeAverages[nodeName]
	}

	// Reset if minute has passed
	if time.Since(avg.StartTime) >= time.Minute {
		avg.Sum = 0
		avg.Count = 0
		avg.StartTime = time.Now()
	}

	avg.Sum += currentUsage
	avg.Count++

	return avg.Sum / float64(avg.Count)
}

// RegisterNode implements domain.NodeRegistration interface
func (s *Service) RegisterNode(ctx context.Context, nodeName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Initialize window for this node if it doesn't exist
	if _, exists := s.windows[nodeName]; !exists {
		s.windows[nodeName] = domainMetric.NewSlidingWindow(
			s.config.WindowSize,
			s.config.SlidingSize,
			s.config.CooldownPeriod,
		)
		log.Printf("Initialized new window for node: %s", nodeName)
	}

	// Initialize stats tracking
	if _, exists := s.previousStats[nodeName]; !exists {
		s.previousStats[nodeName] = make(map[string]domainMetric.CPUStats)
	}

	// Initialize minute average
	if _, exists := s.nodeAverages[nodeName]; !exists {
		s.nodeAverages[nodeName] = &domainMetric.MinuteAverage{
			StartTime: time.Now(),
		}
	}

	return nil
}
