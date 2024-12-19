package analyzer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/openai/openai-go"
	"github.com/sozercan/guac-ai-mole/apimodels"
	"github.com/sozercan/guac-ai-mole/internal/guac"
	"github.com/sozercan/guac-ai-mole/internal/llm"
)

const (
	MaxSteps = 5
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
Do not conclude 'no dependencies' if any IsDependency results show packages or files. Instead, list them and accurately describe them.

A purl is a URL composed of seven components:

scheme:type/namespace/name@version?qualifiers#subpath
Components are separated by a specific character for unambiguous parsing.

The definition for each components is:

scheme: this is the URL scheme with the constant value of "pkg". One of the primary reason for this single scheme is to facilitate the future official registration of the "pkg" scheme for package URLs. Required.
type: the package "type" or package "protocol" such as maven, npm, nuget, gem, pypi, etc. Required.
namespace: some name prefix such as a Maven groupid, a Docker image owner, a GitHub user or organization. Optional and type-specific.
name: the name of the package. Required.
version: the version of the package. Optional.
qualifiers: extra qualifying data for a package such as an OS, architecture, a distro, etc. Optional and type-specific.
subpath: extra subpath within a package, relative to the package root. Optional.
`

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
	llmProvider llm.Provider
	toolParams  []openai.ChatCompletionToolParam
	toolMap     map[string]guac.ToolFunctionType
}

func New(llmProvider llm.Provider, toolDefs []guac.DefinitionType) *Analyzer {
	// decompose toolDefs into toolParams and toolMap
	defs := make([]openai.ChatCompletionToolParam, 0, len(toolDefs))
	toolMap := make(map[string]guac.ToolFunctionType)
	for _, d := range toolDefs {
		defs = append(defs, d.Spec)
		fname := d.Spec.Function.Value.Name.Value
		toolMap[fname] = d.Function
	}
	a := &Analyzer{
		llmProvider: llmProvider,
		toolParams:  defs,
		toolMap:     toolMap,
	}
	return a
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
			stepData, err := a.dispatchTool(ctx, action.Function, action.Arguments)
			if err != nil {
				state.GatheredData = append(state.GatheredData, StepData{
					StepNumber:   state.Steps + 1,
					FunctionName: action.Function,
					Arguments:    action.Arguments,
					Data:         nil,
					Findings:     fmt.Sprintf("Step %d: %s failed: %v", state.Steps+1, action.Function, err),
				})
			} else {
				state.GatheredData = append(state.GatheredData, StepData{
					StepNumber:   state.Steps + 1,
					FunctionName: action.Function,
					Arguments:    action.Arguments,
					Data:         stepData,
					Findings:     fmt.Sprintf("Step %d: %s returned %+v", state.Steps+1, action.Function, stepData),
				})
			}
			state.Steps++
		case "final_response":
			return a.handleFinalResponse(startTime, req, state, llmUsage, action.Message)
		default:
			slog.Error("Unknown agent action", "action", action.Action)
			return nil, fmt.Errorf("unknown agent action: %s", action.Action)
		}
	}

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
			o.Tools = a.toolParams
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
			// No tools needed for final summary
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

func (a *Analyzer) dispatchTool(ctx context.Context, functionName string, arguments json.RawMessage) (string, error) {
	slog.Info("Dispatching tool call", "functionName", functionName)

	// Check if this is a GUAC operation or a local tool.
	// The analyzer just needs to call the appropriate tool from the definitions we passed in.
	// We'll assume the analyzer only has the openai.Function definitions. We need a way to map functionName back to a tool function.

	// Since we passed toolParams to LLM from GUACTools.GetDefinitions(),
	// we need a reference to the actual GUACTools or a mapping from functionName to its handler.
	// One solution: Keep a map from functionName to DefinitionType (fn) in Analyzer as well.

	// For simplicity here, let's assume we have a map or we changed Analyzer signature to also store the GUACTools or a map.
	// Let's say we store tool name -> function in a map: a.toolMap[functionName] = (fn)
	// The code snippet below is illustrative. You will need to pass in or build this map at Analyzer construction time.

	// For example:
	// In New(), after receiving toolDefinitions (from guacTools.GetDefinitions()), we build a map:
	// a.toolMap = make(map[string]ToolFunctionType)
	// For each definition in toolDefinitions:
	//   fname := definition.Spec.Function.Value.Name.Value
	//   a.toolMap[fname] = definition.Function (the actual function that we got from GUACTools)
	// We'll just assume we did that.

	toolFn, ok := a.toolMap[functionName]
	if !ok {
		return "", fmt.Errorf("unknown functionName: %s", functionName)
	}

	// Call the tool function with arguments.
	// The tool function expects raw JSON arguments as a single parameter in this design.
	resultIface, err := toolFn(ctx, arguments)
	if err != nil {
		return "", fmt.Errorf("tool function failed: %w", err)
	}

	// Marshal result to string.
	jsonBytes, err := json.Marshal(resultIface)
	if err != nil {
		slog.Error("Failed to marshal tool result to JSON", "error", err)
		return "error: failed to parse tool output", nil
	}

	out := string(jsonBytes)
	if len(out) > 5000 {
		out = out[:5000] + "\n[truncated]"
	}
	return out, nil
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
