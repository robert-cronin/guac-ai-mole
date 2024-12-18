package analyzer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/sozercan/guac-ai-mole/apimodels"
	"github.com/sozercan/guac-ai-mole/internal/guac"
	"github.com/sozercan/guac-ai-mole/internal/guac/tools"
	"github.com/sozercan/guac-ai-mole/internal/llm"
)

const (
	MaxSteps = 5
)

// The LLM is aware tools are available via function calling, but we don't over-explain them.
// The LLM should reason about what queries to perform and return a final answer.
// We'll rely on passing tools at runtime, no need to specify them in the prompt itself.
var SystemPrompt = `You are an AI agent analyzing software supply chain data. 
You have access to various functions (tools) that can help you gather additional information. 
Your goal is to analyze the user's query, possibly by calling functions to gather context, 
and then provide a final, well-reasoned answer. 
When you need more information, call a function instead of making assumptions. 
After you've gathered enough information, provide a concise final answer to the user.`

type AgentState struct {
	Steps         int
	GatheredData  []StepData
	CurrentQuery  string
	OriginalQuery string
}

type StepData struct {
	StepNumber   int
	FunctionName string
	Data         interface{}
	Findings     string
}

type AgentAction struct {
	Action    string          `json:"action"` // "function_call" or "final_response"
	Function  string          `json:"function,omitempty"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
	Message   string          `json:"message,omitempty"`
}

type Analyzer struct {
	guacClient  *guac.Client
	llmProvider llm.Provider
}

func New(guacClient *guac.Client, llmProvider llm.Provider) *Analyzer {
	return &Analyzer{
		guacClient:  guacClient,
		llmProvider: llmProvider,
	}
}
func (a *Analyzer) Analyze(ctx context.Context, req apimodels.AnalysisRequest) (*apimodels.AnalysisResponse, error) {
	slog.Info("Starting analysis", "query", req.Query)
	startTime := time.Now()

	state := &AgentState{
		Steps:         0,
		OriginalQuery: req.Query,
		CurrentQuery:  req.Query,
		GatheredData:  make([]StepData, 0),
	}

	// Main agent loop
	for state.Steps < MaxSteps {
		action, llmUsage, err := a.getNextAgentAction(ctx, req, state)
		if err != nil {
			return nil, err
		}

		switch action.Action {
		case "function_call":
			if err := a.handleFunctionCall(ctx, state, action); err != nil {
				return nil, err
			}
		case "final_response":
			return a.handleFinalResponse(startTime, req, state, llmUsage, action.Message)
		default:
			slog.Error("Unknown agent action", "action", action.Action)
			return nil, fmt.Errorf("unknown agent action: %s", action.Action)
		}
	}

	// If we've reached max steps, generate final summary
	return a.generateFinalSummary(ctx, req, startTime, state)
}

// getNextAgentAction calls the LLM to get the agent's next action.
func (a *Analyzer) getNextAgentAction(ctx context.Context, req apimodels.AnalysisRequest, state *AgentState) (AgentAction, llm.Usage, error) {
	findings := summarizeFindings(state.GatheredData)
	slog.Debug("Current findings summary", "findings", findings)

	prompt := fmt.Sprintf("%s\n\nCurrent step: %d/%d\nPrevious findings: %s\nUser Query: %s",
		SystemPrompt, state.Steps+1, MaxSteps, findings, state.CurrentQuery)

	llmResp, err := a.llmProvider.Analyze(
		prompt,
		llm.Option(func(o *llm.Options) {
			o.Tools = tools.Definitions
			if req.Options.Model != "" {
				o.Model = req.Options.Model
			}
			if req.Options.MaxTokens != 0 {
				o.MaxTokens = req.Options.MaxTokens
			}
			if req.Options.Temperature != 0 {
				o.Temperature = req.Options.Temperature
			}
		}),
	)
	if err != nil {
		slog.Error("LLM analysis failed", "error", err)
		return AgentAction{}, llm.Usage{}, fmt.Errorf("LLM analysis failed: %w", err)
	}

	var action AgentAction
	if llmResp.FunctionCall != nil {
		action = AgentAction{
			Action:    "function_call",
			Function:  llmResp.FunctionCall.Name,
			Arguments: []byte(llmResp.FunctionCall.Arguments),
		}
		slog.Debug("LLM requested function call", "function", action.Function, "arguments", string(action.Arguments))
	} else {
		action.Action = "final_response"
		action.Message = llmResp.Content
		slog.Debug("LLM provided final response", "message", action.Message)
	}

	return action, llmResp.Usage, nil
}

// handleFunctionCall executes the requested function and stores the data in the state.
func (a *Analyzer) handleFunctionCall(ctx context.Context, state *AgentState, action AgentAction) error {
	slog.Info("Executing function call", "function", action.Function)
	stepData, err := a.executeFunction(ctx, action.Function, action.Arguments)
	if err != nil {
		slog.Error("Function execution failed", "error", err)
		return fmt.Errorf("function execution failed: %w", err)
	}

	// Record step data
	state.GatheredData = append(state.GatheredData, StepData{
		StepNumber:   state.Steps + 1,
		FunctionName: action.Function,
		Data:         stepData,
		Findings:     fmt.Sprintf("Step %d: %s returned %+v", state.Steps+1, action.Function, stepData),
	})
	state.Steps++
	slog.Debug("Recorded step data", "stepData", stepData)
	return nil
}

// handleFinalResponse returns the final response from the LLM to the client.
func (a *Analyzer) handleFinalResponse(startTime time.Time, req apimodels.AnalysisRequest, state *AgentState, usage llm.Usage, message string) (*apimodels.AnalysisResponse, error) {
	slog.Info("Returning final response")

	// Truncate final message if necessary
	finalMessage := truncateString(message, 5000)

	return &apimodels.AnalysisResponse{
		Result: finalMessage,
		SupportingData: &apimodels.SupportingData{
			Queries:  getFunctionCalls(state.GatheredData),
			GuacData: state.GatheredData,
		},
		Metadata: apimodels.AnalysisMetadata{
			Duration:   time.Since(startTime).String(),
			Model:      req.Options.Model,
			TokensUsed: usage.TotalTokens,
			Steps:      state.Steps,
		},
	}, nil
}

// generateFinalSummary calls the LLM one more time after max steps to get a summary and returns it.
func (a *Analyzer) generateFinalSummary(ctx context.Context, req apimodels.AnalysisRequest, startTime time.Time, state *AgentState) (*apimodels.AnalysisResponse, error) {
	finalResp, err := a.llmProvider.Analyze(
		fmt.Sprintf("You've reached the maximum steps (%d). Please provide a final summary.\n\nOriginal Query: %s\n\nFindings:\n%s",
			MaxSteps, state.OriginalQuery, summarizeFindings(state.GatheredData)),
		llm.Option(func(o *llm.Options) {
			if req.Options.Model != "" {
				o.Model = req.Options.Model
			}
		}),
	)
	if err != nil {
		slog.Error("Failed to generate final summary", "error", err)
		return nil, fmt.Errorf("failed to generate final summary: %w", err)
	}

	finalMessage := truncateString(finalResp.Content, 5000)
	return &apimodels.AnalysisResponse{
		Result: finalMessage,
		SupportingData: &apimodels.SupportingData{
			Queries:  getFunctionCalls(state.GatheredData),
			GuacData: state.GatheredData,
		},
		Metadata: apimodels.AnalysisMetadata{
			Duration:   time.Since(startTime).String(),
			Model:      req.Options.Model,
			TokensUsed: finalResp.Usage.TotalTokens,
			Steps:      state.Steps,
		},
	}, nil
}

// truncateLargeData attempts to truncate large textual data if present.
// If stepData is a string, it truncates it. If it's a structure, you may
// need to recursively truncate fields as needed. For simplicity, we just
// handle the common case of a string result.
func truncateLargeData(data interface{}, maxLen int) interface{} {
	switch v := data.(type) {
	case string:
		return truncateString(v, maxLen)
	default:
		// If needed, add logic to handle maps, arrays, etc.
		return data
	}
}

// truncateString ensures a string does not exceed maxLen, appending "[truncated]" if it does.
func truncateString(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen] + "\n[truncated]"
	}
	return s
}

func (a *Analyzer) executeFunction(ctx context.Context, functionName string, arguments json.RawMessage) (string, error) {
	slog.Info("Executing function", "functionName", functionName)

	// Call GUAC operation
	result, err := a.guacClient.CallGUACOperation(ctx, functionName, arguments)
	if err != nil {
		return "", fmt.Errorf("failed to call GUAC operation: %w", err)
	}

	// Marshal result to JSON string, since LLM works best with text-based formats
	jsonBytes, err := json.Marshal(result)
	if err != nil {
		slog.Error("Failed to marshal GUAC result to JSON", "error", err)
		// If marshaling fails, return a simple error message as a string so we still fulfill the text return
		return "error: failed to parse tool output", nil
	}

	// Convert to string
	out := string(jsonBytes)

	// Truncate to 5000 characters
	if len(out) > 5000 {
		out = out[:5000] + "\n[truncated]"
	}

	return out, nil
}

// Helper functions
func summarizeFindings(data []StepData) string {
	if len(data) == 0 {
		return "No previous findings."
	}

	summary := "Previous findings:\n"
	for _, step := range data {
		summary += step.Findings + "\n"
	}
	return summary
}

func getFunctionCalls(data []StepData) []string {
	calls := make([]string, len(data))
	for i, step := range data {
		calls[i] = step.FunctionName
	}
	return calls
}
