// cmd/server/main.go
package main

import (
	"log"
	"log/slog"

	"github.com/sozercan/guac-ai-mole/internal/server"
	"github.com/sozercan/guac-ai-mole/internal/analyzer"
	"github.com/sozercan/guac-ai-mole/internal/config"
	"github.com/sozercan/guac-ai-mole/internal/guac"
	"github.com/sozercan/guac-ai-mole/internal/llm"
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("failed to load configuration: %v", err)
	}

	guacClient, err := guac.NewClient(cfg.GUAC.GraphQLEndpoint)
	if err != nil {
		log.Fatalf("failed to create GUAC client: %v", err)
	}

	llmProvider, err := llm.NewOpenAI(&cfg.OpenAI)
	if err != nil {
		log.Fatalf("failed to create LLM provider: %v", err)
	}

	analyzer := analyzer.New(guacClient, llmProvider)

	srv := server.New(*cfg, analyzer)
	slog.Info("starting server", "host", cfg.Server.Host, "port", cfg.Server.Port)
	if err := srv.Run(); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
