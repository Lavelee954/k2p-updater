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

	// Create a factory
	factory, err := resource.NewFactory(cfg, kcfg)
	if err != nil {
		log.Fatalf("Failed to create resource factory: %v", err)
		return
	}

	ctx := context.Background()

	statusMsg := "정상적으로 등록되었습니다."

	// Use factory directly to access Event functionality
	err = factory.Event().NormalRecord(ctx, "updater", "VmSpecUp", statusMsg)
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

	// Use factory directly to access Status functionality
	err = factory.Status().UpdateGeneric(ctx, "updater", statusData)
	if err != nil {
		log.Fatalf("Failed to update status: %v", err)
		return
	}
}
