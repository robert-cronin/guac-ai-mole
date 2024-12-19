package analyzer

import (
	"context"
	"encoding/json"
	"errors"
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

var (
	errGuacUnreachable = errors.New("guac unreachable after retry")
	maxGUACRetries     = 2
)

var SystemPrompt = `You are an AI agent analyzing software supply chain data.
You have access to various functions (tools) that can help you gather additional information.
Your goal is to analyze the user's query by possibly calling functions to gather context,
and then provide a final, well-reasoned answer.
When you need more information, call a function instead of making assumptions.
Note: in GUAC, ID fields are numerical identifiers of the nodes in the graph so only use them for referencing specific known nodes.
After you've gathered enough information, provide a concise final answer to the user.

!!!IMPORTANT NOTE!!!: Do not repeat function calls with the same arguments if the results are already known.
If you attempt to call a function with the same arguments again, you will receive no new data.
Thus, do not waste steps by repeating the same call. If no new information is available, proceed to final answer.

If 'top-level package GUAC heuristic' or similar references appear, it indicates some form of dependency or related component was found.
Do not conclude 'no dependencies' if any IsDependency results show packages or files. Instead, list them and accurately describe them.`

type AgentState struct {
	Steps         int
	GatheredData  []StepData
	CurrentQuery  string
	OriginalQuery string
}

type StepData struct {
	StepNumber   int
	FunctionName string
	Arguments    json.RawMessage
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

	for state.Steps < MaxSteps {
		action, llmUsage, err := a.getNextAgentAction(ctx, req, state)
		if err != nil {
			return nil, err
		}

		switch action.Action {
		case "function_call":
			err := a.handleFunctionCall(ctx, state, action)
			if err != nil {
				if errors.Is(err, errGuacUnreachable) {
					state.GatheredData = append(state.GatheredData, StepData{
						StepNumber:   state.Steps + 1,
						FunctionName: action.Function,
						Arguments:    action.Arguments,
						Data:         "Failed to reach GUAC after multiple attempts.",
						Findings:     "GUAC unreachable after multiple attempts.",
					})
					state.Steps++
					return a.generateGUACFailureExplanation(ctx, req, startTime, state)
				}
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

	// build a reminder of previously used function calls to discourage repetition
	historyReminder := a.buildHistoryReminder(state)

	systemContent := fmt.Sprintf(
		"%s\n\nCurrent step: %d/%d\nPrevious findings:\n%s\n\n%s",
		SystemPrompt, state.Steps+1, MaxSteps, findings, historyReminder,
	)
	userContent := state.CurrentQuery

	llmResp, err := a.llmProvider.Analyze(
		[]string{systemContent},
		[]string{userContent},
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

func (a *Analyzer) buildHistoryReminder(state *AgentState) string {
	if len(state.GatheredData) == 0 {
		return "No previous function calls have been made."
	}

	reminder := "Previously called functions (do not repeat these exact calls):\n"
	seen := make(map[string]bool)
	for _, sd := range state.GatheredData {
		key := sd.FunctionName + string(sd.Arguments)
		if !seen[key] {
			reminder += fmt.Sprintf("- Function: %s Arguments: %s\n", sd.FunctionName, string(sd.Arguments))
			seen[key] = true
		}
	}
	return reminder
}

func (a *Analyzer) handleFunctionCall(ctx context.Context, state *AgentState, action AgentAction) error {
	slog.Info("Executing function call", "function", action.Function)

	// check if this exact call was previously made
	for _, sd := range state.GatheredData {
		if sd.FunctionName == action.Function && jsonEqual(sd.Arguments, action.Arguments) {
			findings := fmt.Sprintf("Step %d: %s called again with same arguments, reusing results from step %d",
				state.Steps+1, action.Function, sd.StepNumber)

			state.GatheredData = append(state.GatheredData, StepData{
				StepNumber:   state.Steps + 1,
				FunctionName: action.Function,
				Arguments:    action.Arguments,
				Data:         sd.Data,
				Findings:     findings,
			})
			state.Steps++
			return nil
		}
	}

	stepData, err := a.executeFunction(ctx, action.Function, action.Arguments)
	if err != nil {
		slog.Error("Function execution failed", "error", err)
		return fmt.Errorf("function execution failed: %w", err)
	}

	state.GatheredData = append(state.GatheredData, StepData{
		StepNumber:   state.Steps + 1,
		FunctionName: action.Function,
		Arguments:    action.Arguments,
		Data:         stepData,
		Findings:     fmt.Sprintf("Step %d: %s returned %+v", state.Steps+1, action.Function, stepData),
	})
	state.Steps++
	slog.Debug("Recorded step data", "stepData", stepData)
	return nil
}

func (a *Analyzer) handleFinalResponse(startTime time.Time, req apimodels.AnalysisRequest, state *AgentState, usage llm.Usage, message string) (*apimodels.AnalysisResponse, error) {
	slog.Info("Returning final response")
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

func (a *Analyzer) generateFinalSummary(ctx context.Context, req apimodels.AnalysisRequest, startTime time.Time, state *AgentState) (*apimodels.AnalysisResponse, error) {
	systemContent := fmt.Sprintf(`
You have reached the maximum steps (%d). Please provide a final summary.
Original Query: %s

Previous findings:
%s

In your summary provide a truthful and concise final answer that reflects all the data discovered.
`, MaxSteps, state.OriginalQuery, summarizeFindings(state.GatheredData))

	userContent := ""

	finalResp, err := a.llmProvider.Analyze(
		[]string{systemContent},
		[]string{userContent},
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

func (a *Analyzer) executeFunction(ctx context.Context, functionName string, arguments json.RawMessage) (string, error) {
	slog.Info("Executing function", "functionName", functionName)

	result, err := a.callGUACWithRetry(ctx, functionName, arguments, maxGUACRetries)
	if err != nil {
		return "", err
	}

	jsonBytes, err := json.Marshal(result)
	if err != nil {
		slog.Error("Failed to marshal GUAC result to JSON", "error", err)
		return "error: failed to parse tool output", nil
	}

	out := string(jsonBytes)
	if len(out) > 5000 {
		out = out[:5000] + "\n[truncated]"
	}

	return out, nil
}

func (a *Analyzer) callGUACWithRetry(ctx context.Context, functionName string, arguments json.RawMessage, maxAttempts int) (interface{}, error) {
	var lastErr error
	for i := 0; i < maxAttempts; i++ {
		slog.Info("Calling GUAC operation", "function", functionName, "attempt", i+1)
		result, err := a.guacClient.CallGUACOperation(ctx, functionName, arguments)
		if err == nil {
			return result, nil
		}
		lastErr = err
		slog.Warn("Failed to call GUAC operation", "attempt", i+1, "function", functionName, "error", err)
	}
	return nil, fmt.Errorf("%w: %v", errGuacUnreachable, lastErr)
}

func (a *Analyzer) generateGUACFailureExplanation(ctx context.Context, req apimodels.AnalysisRequest, startTime time.Time, state *AgentState) (*apimodels.AnalysisResponse, error) {
	slog.Info("Generating GUAC failure explanation")

	systemContent := fmt.Sprintf(`You attempted to use GUAC tools multiple times but they failed.
Now provide a concise, friendly message to the user explaining that you cannot reach the GUAC service 
and thus cannot complete their request. Apologize briefly and ask them to try again later.

Original query: %s

Previous findings:
%s
`, state.OriginalQuery, summarizeFindings(state.GatheredData))

	userContent := ""

	finalResp, err := a.llmProvider.Analyze(
		[]string{systemContent},
		[]string{userContent},
		llm.Option(func(o *llm.Options) {
			if req.Options.Model != "" {
				o.Model = req.Options.Model
			}
			o.Tools = nil
		}),
	)
	if err != nil {
		slog.Error("Failed to generate GUAC failure explanation", "error", err)
		return nil, fmt.Errorf("failed to generate GUAC failure explanation: %w", err)
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

func summarizeFindings(data []StepData) string {
	if len(data) == 0 {
		return "No previous findings."
	}
	summary := ""
	for _, step := range data {
		argsStr := string(step.Arguments)
		summary += fmt.Sprintf("Step %d:\n  Function: %s\n  Arguments: %s\n  Data: %v\n  Findings: %s\n\n",
			step.StepNumber, step.FunctionName, argsStr, step.Data, step.Findings)
	}
	return summary
}

func getFunctionCalls(data []StepData) []interface{} {
	calls := make([]interface{}, len(data))
	for i, step := range data {
		calls[i] = map[string]interface{}{
			"function":  step.FunctionName,
			"arguments": json.RawMessage(step.Arguments),
		}
	}
	return calls
}

func truncateString(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen] + "\n[truncated]"
	}
	return s
}

func jsonEqual(a, b json.RawMessage) bool {
	var ja, jb interface{}
	_ = json.Unmarshal(a, &ja)
	_ = json.Unmarshal(b, &jb)
	return fmt.Sprintf("%v", ja) == fmt.Sprintf("%v", jb)
}
