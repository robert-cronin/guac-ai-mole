package llm

import (
	"context"
	"encoding/json"

	openai "github.com/sashabaranov/go-openai"
	"github.com/sozercan/guac-ai-mole/internal/config"
)

// OpenAI client implementation
type OpenAI struct {
	client *openai.Client
	cfg    *config.OpenAIConfig
}

func NewOpenAI(cfg *config.OpenAIConfig) (*OpenAI, error) {
	var client *openai.Client

	switch cfg.Provider {
	case "azure":
		config := openai.DefaultAzureConfig(cfg.APIKey, cfg.APIEndpoint)
		config.AzureModelMapperFunc = func(model string) string {
			return cfg.DeploymentName
		}
		config.APIVersion = cfg.APIVersion
		client = openai.NewClientWithConfig(config)

	default: // "openai"
		config := openai.DefaultConfig(cfg.APIKey)
		if cfg.APIEndpoint != "" {
			config.BaseURL = cfg.APIEndpoint
		}
		client = openai.NewClientWithConfig(config)
	}

	return &OpenAI{
		client: client,
		cfg:    cfg,
	}, nil
}

func (o *OpenAI) Analyze(prompt string, opts ...Option) (*Response, error) {
	// Apply options
	options := &Options{
		Model:       o.cfg.Model,
		Temperature: 0,
		MaxTokens:   1000,
	}
	for _, opt := range opts {
		opt(options)
	}

	// Convert our function definitions to OpenAI format
	var tools []openai.Tool
	if len(options.Functions) > 0 {
		tools = make([]openai.Tool, len(options.Functions))
		for i, fn := range options.Functions {
			oaiFunc := openai.FunctionDefinition{
				Name:        fn.Name,
				Description: fn.Description,
				Parameters:  rawJSON(fn.Parameters),
			}
			tools[i] = openai.Tool{
				Type:     openai.ToolTypeFunction,
				Function: &oaiFunc,
			}
		}
	}

	resp, err := o.client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: options.Model,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleSystem,
					Content: "You are guac-ai-mole, a helpful AI assistant analyzing software supply chain data.",
				},
				{
					Role:    openai.ChatMessageRoleUser,
					Content: prompt,
				},
			},
			Tools:       tools,
			Temperature: options.Temperature,
			MaxTokens:   options.MaxTokens,
		},
	)
	if err != nil {
		return nil, err
	}

	// Process the response
	response := &Response{
		Usage: Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		},
	}

	// Check for function calls in the response
	if len(resp.Choices) > 0 && resp.Choices[0].Message.ToolCalls != nil && len(resp.Choices[0].Message.ToolCalls) > 0 {
		toolCall := resp.Choices[0].Message.ToolCalls[0]
		response.FunctionCall = &FunctionResponse{
			Name:      toolCall.Function.Name,
			Arguments: toolCall.Function.Arguments,
		}
	} else {
		response.Content = resp.Choices[0].Message.Content
	}

	return response, nil
}

// Helper function to convert our JSONSchema to raw json for the OpenAI API
func rawJSON(schema JSONSchema) json.RawMessage {
	data, _ := json.Marshal(schema)
	return data
}
