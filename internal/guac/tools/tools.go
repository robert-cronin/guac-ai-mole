package tools

import (
	"log/slog"

	"github.com/openai/openai-go"
)

var Definitions = []openai.ChatCompletionToolParam{}

func init() {
	definitions, err := generateDefinitionsFromGUAC()
	if err != nil {
		slog.Error("Failed to generate tool definitions", "error", err)
		return
	}

	definitions = append(definitions, openai.ChatCompletionToolParam{
		Type: openai.F(openai.ChatCompletionToolTypeFunction),
		Function: openai.F(openai.FunctionDefinitionParam{
			Name:        openai.String("KnownQuery"),
			Description: openai.String("query all known info about a package, source, or artifact, just like guacone query known"),
			Parameters: openai.F(openai.FunctionParameters(map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"subjectType": map[string]interface{}{
						"type":        "string",
						"description": "one of: package, source, artifact",
					},
					"subject": map[string]interface{}{
						"type":        "string",
						"description": "for package: purl, for source: vcs_tool+transport, for artifact: algorithm:digest",
					},
				},
				"required": []string{"subjectType", "subject"},
			})),
		}),
	})

	Definitions = definitions
}
