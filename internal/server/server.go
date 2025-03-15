package server

import (
	"context"
	"k2p-updater/cmd/app"
	"k2p-updater/pkg/resource"
	"log"
	"time"
)

// Run starts the application
func Run() {
	// 1. Load configuration
	cfg, err := app.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// 2. Create Kubernetes clients
	kcfg, err := app.NewKubeClients(&cfg.Kubernetes)
	if err != nil {
		log.Fatalf("Failed to create Kubernetes clients: %v", err)
	}

	// 3. Convert app-specific configuration to resource package configuration
	// This is the key change that breaks the circular dependency
	resourceDefs := convertResourceDefinitions(cfg.Resources.Definitions)

	// 4. Create factory with extracted data instead of app-specific types
	factory, err := resource.NewFactory(
		cfg.Resources.Namespace,
		cfg.Resources.Group,
		cfg.Resources.Version,
		resourceDefs,
		kcfg.DynamicClient,
		kcfg.ClientSet,
	)
	if err != nil {
		log.Fatalf("Failed to create resource factory: %v", err)
		return
	}

	// 5. Use the factory for business logic
	ctx := context.Background()
	statusMsg := "정상적으로 등록되었습니다."

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

	err = factory.Status().UpdateGeneric(ctx, "updater", statusData)
	if err != nil {
		log.Fatalf("Failed to update status: %v", err)
		return
	}
}

// convertResourceDefinitions converts app-specific configuration to resource package format
func convertResourceDefinitions(appDefs map[string]app.ResourceDefinitionConfig) map[string]resource.ResourceDefinition {
	resourceDefs := make(map[string]resource.ResourceDefinition)

	for key, appDef := range appDefs {
		resourceDefs[key] = resource.ResourceDefinition{
			Resource:    appDef.Resource,
			NameFormat:  appDef.NameFormat,
			StatusField: appDef.StatusField,
			Kind:        appDef.Kind,
			CRName:      appDef.CRName,
		}
	}

	return resourceDefs
}
