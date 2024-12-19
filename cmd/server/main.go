package main

import (
	"log"
	"log/slog"

	"github.com/sozercan/guac-ai-mole/internal/analyzer"
	"github.com/sozercan/guac-ai-mole/internal/config"
	"github.com/sozercan/guac-ai-mole/internal/guac"
	"github.com/sozercan/guac-ai-mole/internal/llm"
	"github.com/sozercan/guac-ai-mole/internal/server"
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("failed to load configuration: %v", err)
	}

	// Initialize GUACTools to get all tool definitions
	guacTools, err := guac.NewGUACTools(cfg.GUAC.GraphQLEndpoint)
	if err != nil {
		log.Fatalf("failed to create GUAC tools: %v", err)
	}

	// Extract the definitions from GUACTools
	definitions := guacTools.GetDefinitions()

	llmProvider, err := llm.NewOpenAI(&cfg.OpenAI)
	if err != nil {
		log.Fatalf("failed to create LLM provider: %v", err)
	}

	// Create the analyzer with GUAC client, LLM provider, and tool definitions
	an := analyzer.New(llmProvider, definitions)

	srv := server.New(*cfg, an)
	slog.Info("starting server", "host", cfg.Server.Host, "port", cfg.Server.Port)
	if err := srv.Run(); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
