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

	cr, err := resource.NewManager(cfg, kcfg)
	if err != nil {
		log.Fatalf("Failed to create resource manager: %v", err)
		return
	}

	ctx := context.Background()

	// Check if the key exists in the template
	if _, ok := cr.Templates.Key["updater"]; !ok {
		log.Fatalf("Resource key 'updater' not found in template")
		return
	}

	statusMsg := "정상적으로 등록되었습니다."

	err = cr.Event.NormalRecord(ctx, "updater", "VmSpecUp", statusMsg)
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

	// Use UpdateGeneric with the correct key
	err = cr.Status.UpdateGeneric(ctx, "updater", statusData)
	if err != nil {
		log.Fatalf("Failed to update status: %v", err)
		return
	}
}
