package llm

import (
	"context"
	"log/slog"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/azure"
	"github.com/openai/openai-go/option"
	"github.com/sozercan/guac-ai-mole/internal/config"
	"github.com/sozercan/guac-ai-mole/internal/guac/tools"
)

// OpenAI client implementation
type OpenAI struct {
	client *openai.Client
	cfg    *config.OpenAIConfig
	tools  []openai.ChatCompletionToolParam
}

func NewOpenAI(cfg *config.OpenAIConfig) (*OpenAI, error) {
	slog.Info("Creating OpenAI client", "provider", cfg.Provider)
	var client *openai.Client

	switch cfg.Provider {
	case "azure":
		client = openai.NewClient(
			azure.WithEndpoint(cfg.APIEndpoint, cfg.APIVersion),
			azure.WithAPIKey(cfg.APIKey),
		)
	default: // "openai"
		client = openai.NewClient(
			option.WithAPIKey(cfg.APIKey),
			option.WithBaseURL(cfg.APIEndpoint),
		)
	}

	return &OpenAI{
		client: client,
		cfg:    cfg,
		tools:  tools.Definitions,
	}, nil
}

func (o *OpenAI) Analyze(prompt string, opts ...Option) (*Response, error) {
	slog.Info("Starting analysis", "prompt", prompt)
	// Apply options
	options := &Options{
		Model:       o.cfg.Model,
		Temperature: 0,
		MaxTokens:   1000,
	}
	for _, opt := range opts {
		opt(options)
	}

	resp, err := o.client.Chat.Completions.New(
		context.Background(),
		openai.ChatCompletionNewParams{
			Model: openai.F(options.Model),
			Messages: openai.F([]openai.ChatCompletionMessageParamUnion{
				openai.SystemMessage("You are guac-ai-mole, a helpful AI assistant analyzing software supply chain data."),
				openai.UserMessage(prompt),
			}),
			Tools:       openai.F(options.Tools),
			Temperature: openai.F(options.Temperature),
			MaxTokens:   openai.F(options.MaxTokens),
		},
	)
	if err != nil {
		slog.Error("Analysis failed", "error", err)
		return nil, err
	}
	slog.Info("Analysis completed successfully")

	// Process the response
	response := &Response{
		Usage: Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		},
	}

	// Check for function calls in the response
	if len(resp.Choices) > 0 && len(resp.Choices[0].Message.ToolCalls) > 0 {
		toolCall := resp.Choices[0].Message.ToolCalls[0]
		response.FunctionCall = &FunctionResponse{
			Name:      toolCall.Function.Name,
			Arguments: toolCall.Function.Arguments,
		}
	} else if len(resp.Choices) > 0 {
		response.Content = resp.Choices[0].Message.Content
	}

	return response, nil
}
