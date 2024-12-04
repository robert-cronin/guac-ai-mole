package main

import (
	"log"
	"log/slog"

	"github.com/sozercan/guac-ai-mole/api/server"
	"github.com/sozercan/guac-ai-mole/internal/analyzer"
	"github.com/sozercan/guac-ai-mole/internal/config"
	"github.com/sozercan/guac-ai-mole/internal/guac"
	"github.com/sozercan/guac-ai-mole/internal/llm"
)

func main() {
	slog.Debug("Loading configuration")
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	guacClient, err := guac.NewClient(cfg.GUAC.GraphQLEndpoint)
	if err != nil {
		log.Fatalf("Failed to create GUAC client: %v", err)
	}

	llmProvider, err := llm.NewOpenAI(&cfg.OpenAI)
	if err != nil {
		log.Fatalf("Failed to create LLM provider: %v", err)
	}

	analyzer := analyzer.New(guacClient, llmProvider)

	slog.Debug("Creating server")
	srv := server.New(*cfg, analyzer)

	slog.Info("Starting server", "host", cfg.Server.Host, "port", cfg.Server.Port)
	if err := srv.Run(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
