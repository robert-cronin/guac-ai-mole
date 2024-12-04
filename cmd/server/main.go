package main

import (
    "log"
    "log/slog"

    "github.com/sozercan/guac-ai-mole/api/server"
    "github.com/sozercan/guac-ai-mole/internal/config"
)

func main() {
    slog.Debug("Loading configuration")
    // Load configuration
    cfg, err := config.LoadConfig()
    if err != nil {
        log.Fatalf("Failed to load configuration: %v", err)
    }

    slog.Debug("Creating server")
    // Create server
    srv, err := server.New(*cfg)
    if err != nil {
        log.Fatalf("Failed to create server: %v", err)
    }

    slog.Info("Starting server", "host", cfg.Server.Host, "port", cfg.Server.Port)
    if err := srv.Run(); err != nil {
        log.Fatalf("Server failed: %v", err)
    }
}