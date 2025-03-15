package server

import (
	"fmt"
	"k2p-updater/cmd/app"
	"log"
)

func Run() {
	// Load configuration
	cfg, err := app.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	fmt.Println(cfg)

}
