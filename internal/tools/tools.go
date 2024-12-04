package tools

import (
	"github.com/openai/openai-go"
)

// Define GUAC functions that can be called by the LLM
var Specs = []openai.ChatCompletionToolParam{
	{
		Type: openai.F(openai.ChatCompletionToolTypeFunction),
		Function: openai.F(openai.FunctionDefinitionParam{
			Name:        openai.String("get_package_dependencies"),
			Description: openai.String("Get the dependencies for a specified package"),
			Parameters: openai.F(openai.FunctionParameters{
				"type": "object",
				"properties": map[string]interface{}{
					"package_type": map[string]string{
						"type":        "string",
						"description": "The type of package (e.g., npm, maven)",
					},
					"package_name": map[string]string{
						"type":        "string",
						"description": "The name of the package",
					},
				},
				"required": []string{"package_type", "package_name"},
			}),
		}),
	},
	{
		Type: openai.F(openai.ChatCompletionToolTypeFunction),
		Function: openai.F(openai.FunctionDefinitionParam{
			Name:        openai.String("get_vulnerabilities"),
			Description: openai.String("Get vulnerabilities for a specific vulnerability ID"),
			Parameters: openai.F(openai.FunctionParameters{
				"type": "object",
				"properties": map[string]interface{}{
					"vulnerability_id": map[string]string{
						"type":        "string",
						"description": "The ID of the vulnerability (e.g., CVE-2023-1234)",
					},
				},
				"required": []string{"vulnerability_id"},
			}),
		}),
	},
	{
		Type: openai.F(openai.ChatCompletionToolTypeFunction),
		Function: openai.F(openai.FunctionDefinitionParam{
			Name:        openai.String("get_source_scorecard"),
			Description: openai.String("Get OpenSSF Scorecard data for a source repository"),
			Parameters: openai.F(openai.FunctionParameters{
				"type": "object",
				"properties": map[string]interface{}{
					"source_type": map[string]string{
						"type":        "string",
						"description": "The type of source (e.g., git)",
					},
					"namespace": map[string]string{
						"type":        "string",
						"description": "The namespace (e.g., github.com/owner)",
					},
					"name": map[string]string{
						"type":        "string",
						"description": "The repository name",
					},
				},
				"required": []string{"source_type", "namespace", "name"},
			}),
		}),
	},
	{
		Type: openai.F(openai.ChatCompletionToolTypeFunction),
		Function: openai.F(openai.FunctionDefinitionParam{
			Name:        openai.String("get_sbom_attestations"),
			Description: openai.String("Get SBOM attestations for a package"),
			Parameters: openai.F(openai.FunctionParameters{
				"type": "object",
				"properties": map[string]interface{}{
					"package_type": map[string]string{
						"type":        "string",
						"description": "The type of package (e.g., npm, maven)",
					},
					"package_namespace": map[string]string{
						"type":        "string",
						"description": "The package namespace",
					},
					"package_name": map[string]string{
						"type":        "string",
						"description": "The name of the package",
					},
				},
				"required": []string{"package_type", "package_namespace", "package_name"},
			}),
		}),
	},
	{
		Type: openai.F(openai.ChatCompletionToolTypeFunction),
		Function: openai.F(openai.FunctionDefinitionParam{
			Name:        openai.String("get_package_source"),
			Description: openai.String("Get source repository information for a package"),
			Parameters: openai.F(openai.FunctionParameters{
				"type": "object",
				"properties": map[string]interface{}{
					"package_type": map[string]string{
						"type":        "string",
						"description": "The type of package (e.g., npm, maven)",
					},
					"package_namespace": map[string]string{
						"type":        "string",
						"description": "The package namespace",
					},
					"package_name": map[string]string{
						"type":        "string",
						"description": "The name of the package",
					},
				},
				"required": []string{"package_type", "package_namespace", "package_name"},
			}),
		}),
	},
	{
		Type: openai.F(openai.ChatCompletionToolTypeFunction),
		Function: openai.F(openai.FunctionDefinitionParam{
			Name:        openai.String("get_vex_statements"),
			Description: openai.String("Get VEX statements for a vulnerability"),
			Parameters: openai.F(openai.FunctionParameters{
				"type": "object",
				"properties": map[string]interface{}{
					"vuln_type": map[string]string{
						"type":        "string",
						"description": "The type of vulnerability (cve, ghsa, or osv)",
					},
					"vuln_id": map[string]string{
						"type":        "string",
						"description": "The vulnerability identifier",
					},
				},
				"required": []string{"vuln_type", "vuln_id"},
			}),
		}),
	},
	{
		Type: openai.F(openai.ChatCompletionToolTypeFunction),
		Function: openai.F(openai.FunctionDefinitionParam{
			Name:        openai.String("get_package_equality"),
			Description: openai.String("Get package equality assertions"),
			Parameters: openai.F(openai.FunctionParameters{
				"type": "object",
				"properties": map[string]interface{}{
					"package_type": map[string]string{
						"type":        "string",
						"description": "The type of package (e.g., npm, maven)",
					},
					"package_namespace": map[string]string{
						"type":        "string",
						"description": "The package namespace",
					},
					"package_name": map[string]string{
						"type":        "string",
						"description": "The name of the package",
					},
				},
				"required": []string{"package_type", "package_namespace", "package_name"},
			}),
		}),
	},
}
