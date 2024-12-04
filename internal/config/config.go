package config

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"
)

type Config struct {
	Server ServerConfig
	OpenAI OpenAIConfig
	GUAC   GUACConfig
}

type ServerConfig struct {
	Port         string
	Host         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

type OpenAIConfig struct {
	Provider       string
	APIKey         string
	APIEndpoint    string
	Model          string
	DeploymentName string
	APIVersion     string
}

type GUACConfig struct {
	GraphQLEndpoint string
	Timeout         time.Duration
}

func LoadConfig() (*Config, error) {
	slog.Info("Loading configuration")

	cfg := &Config{}

	// Server flags
	flag.StringVar(&cfg.Server.Port, "server-port", "8000", "Server port")
	flag.StringVar(&cfg.Server.Host, "server-host", "0.0.0.0", "Server host")
	flag.DurationVar(&cfg.Server.ReadTimeout, "server-read-timeout", 30*time.Second, "Server read timeout")
	flag.DurationVar(&cfg.Server.WriteTimeout, "server-write-timeout", 30*time.Second, "Server write timeout")

	// OpenAI flags
	flag.StringVar(&cfg.OpenAI.Provider, "openai-provider", "openai", "OpenAI provider (openai or azure)")
	flag.StringVar(&cfg.OpenAI.APIEndpoint, "openai-endpoint", "https://api.openai.com/v1", "OpenAI API endpoint")
	flag.StringVar(&cfg.OpenAI.Model, "openai-model", "gpt-4", "OpenAI model")
	flag.StringVar(&cfg.OpenAI.DeploymentName, "openai-deployment", "gpt-4o-mini", "Azure OpenAI deployment name")
	flag.StringVar(&cfg.OpenAI.APIVersion, "openai-api-version", "2023-05-15", "Azure OpenAI API version")

	// GUAC flags
	flag.StringVar(&cfg.GUAC.GraphQLEndpoint, "guac-endpoint", "http://localhost:8080/query", "GUAC GraphQL endpoint")
	flag.DurationVar(&cfg.GUAC.Timeout, "guac-timeout", 30*time.Second, "GUAC request timeout")

	// Parse flags
	flag.Parse()

	// Get API key from environment variable
	cfg.OpenAI.APIKey = os.Getenv("GUACAIMOLE_OPENAI_API_KEY")

	if err := validateConfig(cfg); err != nil {
		slog.Error("Configuration validation failed", "error", err)
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	slog.Info("Configuration loaded successfully")
	return cfg, nil
}

func validateConfig(cfg *Config) error {
	slog.Info("Validating configuration")
	if cfg.OpenAI.APIKey == "" {
		return fmt.Errorf("OpenAI API key is required (set GUACAIMOLE_OPENAI_API_KEY environment variable)")
	}

	if cfg.OpenAI.Provider == "azure" {
		if cfg.OpenAI.DeploymentName == "" {
			return fmt.Errorf("Azure deployment name is required when using Azure provider")
		}
		if cfg.OpenAI.APIEndpoint == "" {
			return fmt.Errorf("Azure API endpoint is required when using Azure provider")
		}
	}

	if cfg.GUAC.GraphQLEndpoint == "" {
		return fmt.Errorf("GUAC GraphQL endpoint is required")
	}

	return nil
}
