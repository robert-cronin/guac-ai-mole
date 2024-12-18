package apimodels

type AnalysisResponse struct {
	// The analyzed result/answer
	Result string `json:"result"`

	// Any supporting data used in analysis
	SupportingData *SupportingData `json:"supportingData,omitempty"`

	// Metadata about the analysis
	Metadata AnalysisMetadata `json:"metadata"`
}

type SupportingData struct {
	// GraphQL queries executed
	Queries []string `json:"queries,omitempty"`

	// Raw GUAC data retrieved
	GuacData interface{} `json:"guacData,omitempty"`
}

type AnalysisMetadata struct {
	// Time taken for analysis
	Duration string `json:"duration"`

	// Model used for analysis
	Model string `json:"model"`

	// Tokens used in analysis
	TokensUsed int64 `json:"tokensUsed"`

	// Tracks agent steps
	Steps int `json:"steps"`
}
