package models

type AnalysisRequest struct {
	// Query is the natural language query to analyze
	Query string `json:"query"`

	// Optional parameters to control analysis behavior
	Options AnalysisOptions `json:"options,omitempty"`
}

type AnalysisOptions struct {
	// Model specifies which LLM model to use (e.g. "gpt-4")
	Model string `json:"model,omitempty"`

	// MaxTokens limits the LLM response length
	MaxTokens int `json:"maxTokens,omitempty"`

	// Temperature controls randomness (0.0-1.0)
	Temperature float32 `json:"temperature,omitempty"`
}
