package main

import (
    "log"

    "github.com/sozercan/guac-ai-mole/api/server"
    "github.com/sozercan/guac-ai-mole/internal/config"
)

func main() {
    // Load configuration
    cfg, err := config.LoadConfig()
    if err != nil {
        log.Fatalf("Failed to load configuration: %v", err)
    }

    // Create server
    srv, err := server.New(*cfg)
    if err != nil {
        log.Fatalf("Failed to create server: %v", err)
    }

    log.Printf("Starting server on %s:%s", cfg.Server.Host, cfg.Server.Port)
    if err := srv.Run(); err != nil {
        log.Fatalf("Server failed: %v", err)
    }
}