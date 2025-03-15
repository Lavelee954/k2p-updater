package server

import (
	"context"
	"k2p-updater/cmd/app"
	"k2p-updater/pkg/resource"
	"log"
	"time"
)

// server.go

func Run() {
	// Load configuration
	cfg, err := app.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	kcfg, err := app.NewKubeClients(&cfg.Kubernetes)
	if err != nil {
		log.Fatalf("Failed to create Kubernetes clients: %v", err)
	}

	// Create a factory first
	factory, err := resource.NewFactory(cfg, kcfg)
	if err != nil {
		log.Fatalf("Failed to create resource factory: %v", err)
		return
	}

	// Create the manager using the factory
	cr := resource.NewManager(factory)

	ctx := context.Background()

	// Resource key validation is handled internally by the methods

	statusMsg := "정상적으로 등록되었습니다."

	// Use the Event() method to get the event handler
	err = cr.Event().NormalRecord(ctx, "updater", "VmSpecUp", statusMsg)
	if err != nil {
		log.Fatalf("Failed to record event: %v", err)
		return
	}

	statusData := map[string]interface{}{
		"controlPlaneNodeName": "cp-k8s",
		"message":              statusMsg,
		"lastUpdateTime":       time.Now().Format(time.RFC3339),
		"coolDown":             true,
		"updateStatus":         "Pending",
	}

	// Use the Status() method to get the status handler
	err = cr.Status().UpdateGeneric(ctx, "updater", statusData)
	if err != nil {
		log.Fatalf("Failed to update status: %v", err)
		return
	}
}
