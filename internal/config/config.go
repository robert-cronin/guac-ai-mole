package config

import (
	"log/slog"
	"time"

	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	Server ServerConfig
	OpenAI OpenAIConfig
	GUAC   GUACConfig
}

type ServerConfig struct {
	Port         string        `envconfig:"SERVER_PORT" default:"8000"`
	Host         string        `envconfig:"SERVER_HOST" default:"0.0.0.0"`
	ReadTimeout  time.Duration `envconfig:"SERVER_READ_TIMEOUT" default:"30s"`
	WriteTimeout time.Duration `envconfig:"SERVER_WRITE_TIMEOUT" default:"30s"`
}

type OpenAIConfig struct {
	Provider       string `envconfig:"OPENAI_PROVIDER" default:"openai"`
	APIKey         string `envconfig:"OPENAI_API_KEY" required:"true"`
	APIEndpoint    string `envconfig:"OPENAI_ENDPOINT" default:"https://api.openai.com/v1"`
	Model          string `envconfig:"OPENAI_MODEL" default:"gpt-4o-mini"`
	DeploymentName string `envconfig:"OPENAI_DEPLOYMENT" default:"gpt-4o"`
	APIVersion     string `envconfig:"OPENAI_API_VERSION" default:"2023-05-15"`
}

type GUACConfig struct {
	GraphQLEndpoint string        `envconfig:"GUAC_GRAPHQL_ENDPOINT" default:"http://localhost:8080/query"`
	Timeout         time.Duration `envconfig:"GUAC_TIMEOUT" default:"30s"`
}

func LoadConfig() (*Config, error) {
	var cfg Config
	err := envconfig.Process("", &cfg)
	if err != nil {
		return nil, err
	}
	slog.Info("configuration loaded successfully")
	return &cfg, nil
}
