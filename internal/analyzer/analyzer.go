package analyzer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/sozercan/guac-ai-mole/api/models"
	"github.com/sozercan/guac-ai-mole/internal/guac"
	"github.com/sozercan/guac-ai-mole/internal/llm"
	"github.com/sozercan/guac-ai-mole/internal/tools"
)

const (
	MaxSteps = 5
	// TODO: we should programatically insert the available functions here with signatures
	SystemPrompt = `You are an AI agent analyzing software supply chain data. You can:
1. Query for more information using available functions
2. Return a final response to the user

For each query, consider what additional context might be helpful:
- For dependencies, check if sources have good scorecard ratings
- For vulnerabilities, look for VEX statements that might affect applicability
- For packages, check for equivalent packages and their attestations

After each step, you must either:
1. Call another function to gather more context
2. Return final results with {"action": "final_response", "message": "your detailed findings..."}

Consider what information you need, make function calls to gather it, and provide a clear final response.
Organize information logically and highlight key findings.

IMPORTANT:
- You have a maximum of 5 steps
- Each function call counts as one step
- Make the best use of your steps
- Keep track of what you've learned
- If you have enough information or reach max steps, return a final response

Current step: %d/%d
Previous findings: %s

User Query: %s`
)

// AgentState tracks the agent's analysis progress
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

// AgentAction represents the agent's decision for next step
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

func (a *Analyzer) Analyze(ctx context.Context, req models.AnalysisRequest) (*models.AnalysisResponse, error) {
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
		// Get findings summary from gathered data
		findings := summarizeFindings(state.GatheredData)
		slog.Debug("Current findings summary", "findings", findings)

		// Get next action from LLM
		llmResp, err := a.llmProvider.Analyze(
			fmt.Sprintf(SystemPrompt, state.Steps+1, MaxSteps, findings, state.CurrentQuery),
			llm.Option(func(o *llm.Options) {
				o.Tools = tools.Specs
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
			return nil, fmt.Errorf("LLM analysis failed: %w", err)
		}

		// Parse agent's action
		var action AgentAction
		if llmResp.FunctionCall != nil {
			// LLM wants to make a function call
			action = AgentAction{
				Action:    "function_call",
				Function:  llmResp.FunctionCall.Name,
				Arguments: []byte(llmResp.FunctionCall.Arguments),
			}
			slog.Debug("LLM requested function call", "function", action.Function, "arguments", string(action.Arguments))
		} else {
			// LLM provided a direct response, parse it as an action
			action.Action = "final_response"
			action.Message = llmResp.Content
			slog.Debug("LLM provided final response", "message", action.Message)
		}

		// Handle the agent's chosen action
		switch action.Action {
		case "function_call":
			slog.Info("Executing function call", "function", action.Function)
			stepData, err := a.executeFunction(ctx, action.Function, action.Arguments)
			if err != nil {
				slog.Error("Function execution failed", "error", err)
				return nil, fmt.Errorf("function execution failed: %w", err)
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

		case "final_response":
			slog.Info("Returning final response")
			// Agent has decided to return final results
			return &models.AnalysisResponse{
				Result: action.Message,
				SupportingData: &models.SupportingData{
					Queries:  getFunctionCalls(state.GatheredData),
					GuacData: state.GatheredData,
				},
				Metadata: models.AnalysisMetadata{
					Duration:   time.Since(startTime).String(),
					Model:      req.Options.Model,
					TokensUsed: llmResp.Usage.TotalTokens,
					Steps:      state.Steps,
				},
			}, nil

		default:
			slog.Error("Unknown agent action", "action", action.Action)
			return nil, fmt.Errorf("unknown agent action: %s", action.Action)
		}
	}

	// If we've reached max steps, get final summary from LLM
	finalResp, err := a.llmProvider.Analyze(
		fmt.Sprintf("You've reached the maximum steps (%d). Please provide a final summary of all findings.\n\nOriginal Query: %s\n\nGathered Data:\n%s",
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

	return &models.AnalysisResponse{
		Result: finalResp.Content,
		SupportingData: &models.SupportingData{
			Queries:  getFunctionCalls(state.GatheredData),
			GuacData: state.GatheredData,
		},
		Metadata: models.AnalysisMetadata{
			Duration:   time.Since(startTime).String(),
			Model:      req.Options.Model,
			TokensUsed: finalResp.Usage.TotalTokens,
			Steps:      state.Steps,
		},
	}, nil
}

func (a *Analyzer) executeFunction(ctx context.Context, functionName string, arguments json.RawMessage) (interface{}, error) {
	slog.Info("Executing function", "functionName", functionName)
	switch functionName {
	case "graphql_query":
		var args struct {
			PackageType string `json:"package_type"`
			PackageName string `json:"package_name"`
		}
		if err := json.Unmarshal(arguments, &args); err != nil {
			slog.Error("Failed to parse arguments", "error", err)
			return nil, fmt.Errorf("failed to parse arguments: %w", err)
		}
		return a.guacClient.ExecuteGraphQL(ctx, args.PackageType, args.PackageName)

	default:
		slog.Error("Unknown function", "functionName", functionName)
		return nil, fmt.Errorf("unknown function: %s", functionName)
	}
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

// func formatFindings(stepNumber int, functionName string, data interface{}) string {
// 	switch functionName {
// 	case "get_package_dependencies":
// 		if deps, ok := data.(*guac.DependencyQuery); ok {
// 			return fmt.Sprintf("Step %d: Found dependencies for package in namespace %s",
// 				stepNumber, deps.IsDependency.DependencyPackage.Namespaces[0].Namespace)
// 		}

// 	case "get_vulnerabilities":
// 		if vulns, ok := data.(*guac.VulnerabilityQuery); ok {
// 			return fmt.Sprintf("Step %d: Retrieved vulnerability information for packages in %s",
// 				stepNumber, vulns.CertifyVuln.Package.Namespaces[0].Names[0].Name)
// 		}

// 	case "get_source_scorecard":
// 		if score, ok := data.(*guac.ScorecardQuery); ok {
// 			return fmt.Sprintf("Step %d: Retrieved scorecard with aggregate score %.2f for source %s/%s",
// 				stepNumber, score.CertifyScorecard.Scorecard.AggregateScore,
// 				score.CertifyScorecard.Source.Namespaces[0].Namespace,
// 				score.CertifyScorecard.Source.Namespaces[0].Names[0].Name)
// 		}

// 	case "get_sbom_attestations":
// 		if sbom, ok := data.(*guac.HasSBOMQuery); ok {
// 			return fmt.Sprintf("Step %d: Found SBOM attestation created at %s",
// 				stepNumber, sbom.HasSBOM.KnownSince)
// 		}

// 	case "get_package_source":
// 		if src, ok := data.(*guac.HasSourceAtQuery); ok {
// 			return fmt.Sprintf("Step %d: Found source repository for package at %s/%s",
// 				stepNumber,
// 				src.HasSourceAt.Source.Namespaces[0].Namespace,
// 				src.HasSourceAt.Source.Namespaces[0].Names[0].Name)
// 		}

// 	case "get_vex_statements":
// 		if vex, ok := data.(*guac.CertifyVEXQuery); ok {
// 			return fmt.Sprintf("Step %d: Retrieved VEX statement with status %s created at %s",
// 				stepNumber, vex.CertifyVEXStatement.Status, vex.CertifyVEXStatement.KnownSince)
// 		}

// 	case "get_package_equality":
// 		if eq, ok := data.(*guac.PkgEqualQuery); ok {
// 			return fmt.Sprintf("Step %d: Found package equality assertion with justification: %s",
// 				stepNumber, eq.PkgEqual.Justification)
// 		}
// 	}

// 	return fmt.Sprintf("Step %d: %s returned %+v", stepNumber, functionName, data)
// }
