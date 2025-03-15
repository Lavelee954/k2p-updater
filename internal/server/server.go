package server

import (
	"context"
	"fmt"
	"k2p-updater/cmd/app"
	"k2p-updater/pkg/resource"
	"log"
	"time"
)

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
		return
	}

	ctx, _ := context.WithCancel(context.Background())

	statusMsg := "Pending"

	statusData := map[string]interface{}{
		"controlPlaneNodeName": "cp-k8s",
		"message":              statusMsg,
		"lastUpdateTime":       time.Now().Format(time.RFC3339),
		"coolDown":             true,
		"updateStatus":         "Pending",
	}

	err = cr.Status.UpdateGeneric(ctx, "updater", statusData)
	if err != nil {
		fmt.Println(err)
		return
	}
	err = cr.Event.NormalRecord(ctx, "updater", "VmSpecUp", "suceeceeeeeee")
	if err != nil {
		return
	}

}
