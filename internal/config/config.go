package config

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server ServerConfig `mapstructure:"server"`
	OpenAI OpenAIConfig `mapstructure:"openai"`
	GUAC   GUACConfig   `mapstructure:"guac"`
}

type ServerConfig struct {
	Port         string        `mapstructure:"port"`
	Host         string        `mapstructure:"host"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
}

type OpenAIConfig struct {
	Provider    string `mapstructure:"provider"` // "azure" or "openai"
	APIKey      string `mapstructure:"api_key"`
	APIEndpoint string `mapstructure:"api_endpoint"`
	Model       string `mapstructure:"model"`
	// Azure specific settings
	DeploymentName string `mapstructure:"deployment_name"`
	APIVersion     string `mapstructure:"api_version"`
}

type GUACConfig struct {
	GraphQLEndpoint string        `mapstructure:"graphql_endpoint"`
	Timeout         time.Duration `mapstructure:"timeout"`
}

func LoadConfig() (*Config, error) {
	slog.Info("Loading configuration")
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("./config")

	// Environment variables
	viper.SetEnvPrefix("GUACAIMOLE")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	// Bind environment variables to configuration keys
	err := viper.BindEnv("openai.api_key")
	if err != nil {
		return nil, fmt.Errorf("error binding environment variable: %w", err)
	}

	// Default values
	setDefaults()

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
		// Config file not found - using env vars and defaults
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	if err := validateConfig(&cfg); err != nil {
		slog.Error("Configuration validation failed", "error", err)
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	slog.Info("Configuration loaded successfully")
	return &cfg, nil
}

func setDefaults() {
	viper.SetDefault("server.port", "8000")
	viper.SetDefault("server.host", "0.0.0.0")
	viper.SetDefault("server.read_timeout", time.Second*30)
	viper.SetDefault("server.write_timeout", time.Second*30)

	viper.SetDefault("openai.provider", "openai")
	viper.SetDefault("openai.api_endpoint", "https://api.openai.com/v1")
	viper.SetDefault("openai.model", "gpt-4o-mini")
	viper.SetDefault("openai.api_version", "2023-05-15") // Azure default

	viper.SetDefault("guac.graphql_endpoint", "http://localhost:8080/query")
	viper.SetDefault("guac.timeout", time.Second*30)
}

func validateConfig(cfg *Config) error {
	slog.Info("Validating configuration")
	if cfg.OpenAI.APIKey == "" {
		return fmt.Errorf("OpenAI API key is required")
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
