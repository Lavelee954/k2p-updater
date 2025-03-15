package main

import (
	"k2p-updater/internal/server"
	"os"
)

func main() {
	os.Exit(server.Run())
}
