package llm

type Provider interface {
	// Analyze takes a prompt and returns a structured response
	Analyze(prompt string, opts ...Option) (*Response, error)
    
}

type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

type Option func(*Options)

type Options struct {
	Model       string
	MaxTokens   int
	Temperature float32
	Functions   []FunctionDefinition
}

// FunctionDefinition defines an available function that can be called
type FunctionDefinition struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Parameters  JSONSchema `json:"parameters"`
}

// JSONSchema defines the structure for function parameters
type JSONSchema struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties"`
	Required   []string            `json:"required,omitempty"`
}

type Property struct {
	Type        string   `json:"type"`
	Description string   `json:"description,omitempty"`
	Enum        []string `json:"enum,omitempty"`
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
