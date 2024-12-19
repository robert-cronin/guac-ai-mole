package llm

import (
	"github.com/openai/openai-go"
)

type Provider interface {
	// Analyze takes a prompt and returns a structured response
	Analyze(systemMessages []string, userMessages []string, opts ...Option) (*Response, error)
}

type Usage struct {
	PromptTokens     int64
	CompletionTokens int64
	TotalTokens      int64
}

type Option func(*Options)

type Options struct {
	Model       string
	MaxTokens   int64
	Temperature float64
	Tools       []openai.ChatCompletionToolParam
}

// FunctionResponse represents the structured response from a function call
type FunctionResponse struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Extend the Response struct to include function calling results
type Response struct {
	Content      string
	FunctionCall *FunctionResponse
	Usage        Usage
}
