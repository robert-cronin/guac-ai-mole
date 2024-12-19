package tools

import (
	"log/slog"
	"reflect"
	"runtime"
	"strings"

	model "github.com/guacsec/guac/pkg/assembler/clients/generated"
	"github.com/openai/openai-go"
)

// maps a type representing an operation to a description that we'll expose as an LLM tool
type allowedOperationType struct {
	Operation   interface{}
	Description string
}

var allowedOperations = []allowedOperationType{
	{
		Operation: model.Dependencies,
		Description: `
IsDependencySpec allows filtering the list of dependencies to return.
To obtain the list of dependency packages, caller must fill in the package
field.
Dependency packages must be defined at PackageVersion.
`,
	},
	{
		Operation: model.Vulnerabilities,
		Description: `
VulnerabilitySpec allows filtering the list of vulnerabilities to return in a query.
Use null to match on all values at that level.
For example, to get all vulnerabilities in GUAC backend, use a VulnSpec
where every field is null.
Setting the noVuln boolean true will ignore the other inputs for type and vulnerabilityID.
Setting noVuln to true means retrieving only nodes where the type of the vulnerability is "novuln"
and the it has an empty string for vulnerabilityID. Setting it to false filters out all results that are "novuln".
Setting one of the other fields and omitting the noVuln means retrieving vulnerabilities for the corresponding
type and vulnerabilityID. Omission of noVuln field will return all vulnerabilities and novuln.
`,
	},
	{
		Operation: model.Packages,
		Description: `
PkgSpec allows filtering the list of sources to return in a query.
Each field matches a qualifier from pURL. Use null to match on all values at
that level. For example, to get all packages in GUAC backend, use a PkgSpec
where every field is null.
The id field can be used to match on a specific node in the trie to match packageTypeID,
packageNamespaceID, packageNameID, or packageVersionID.
Empty string at a field means matching with the empty string. If passing in
qualifiers, all of the values in the list must match. Since we want to return
nodes with any number of qualifiers if no qualifiers are passed in the input,
we must also return the same set of nodes it the qualifiers list is empty. To
match on nodes that don't contain any qualifier, set matchOnlyEmptyQualifiers
to true. If this field is true, then the qualifiers argument is ignored.
`,
	},
}

func generateDefinitionsFromGUAC() ([]openai.ChatCompletionToolParam, error) {
	var definitions []openai.ChatCompletionToolParam

	for _, ao := range allowedOperations {
		fn := ao.Operation
		desc := ao.Description
		fnVal := reflect.ValueOf(fn)
		fnType := fnVal.Type()

		slog.Debug("Generating tool param", "fnName", fnType.Name(), "fnType", fnType.String())

		if fnVal.Kind() != reflect.Func {
			slog.Error("allowedOperations entry is not a function type", "type", fnType.String())
			continue
		}

		ptr := fnVal.Pointer()
		rf := runtime.FuncForPC(ptr)
		if rf == nil {
			slog.Error("failed to get runtime function for function", "function", fnType.String())
			continue
		}

		toolName := GetRuntimeFuncName(fn)

		numIn := fnType.NumIn()
		if numIn < 3 {
			slog.Error("function '%s' doesn't have the expected number of arguments", "function", toolName)
			continue
		}

		// The last argument is the filter type (IsDependencySpec)
		filterType := fnType.In(numIn - 1)

		filterSchema, err := typeToJSONSchema(filterType)
		if err != nil {
			slog.Error("failed to generate JSON schema for filter type", "type", filterType.String(), "error", err)
			continue
		}

		// Instead of wrapping in a "filter" object, we directly use filterSchema as top-level.
		// This ensures required fields within IsDependencySpec are represented naturally.
		// filterSchema should be an object schema with properties and required fields derived from IsDependencySpec.

		definitions = append(definitions, openai.ChatCompletionToolParam{
			Type: openai.F(openai.ChatCompletionToolTypeFunction),
			Function: openai.F(openai.FunctionDefinitionParam{
				Name:        openai.String(toolName),
				Description: openai.String(strings.TrimSpace(desc)),
				Parameters:  openai.F(openai.FunctionParameters(filterSchema)),
			}),
		})
	}

	return definitions, nil
}

// GetRuntimeFuncName returns the runtime name of the given function.
func GetRuntimeFuncName(fn interface{}) string {
	fnVal := reflect.ValueOf(fn)
	ptr := fnVal.Pointer()
	rf := runtime.FuncForPC(ptr)
	if rf == nil {
		return ""
	}
	// name is like :"github.com/guacsec/guac/pkg/assembler/clients/generated.Dependencies"
	// so trim everything before the last . because the suffix is all we  care about
	// but it mikght not necessailry be this exact prefix
	name := rf.Name()
	lastDot := strings.LastIndex(name, ".")
	if lastDot == -1 {
		return name
	}
	return name[lastDot+1:]
}

// typeToJSONSchema remains unchanged
func typeToJSONSchema(t reflect.Type) (map[string]interface{}, error) {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	switch t.Kind() {
	case reflect.Struct:
		props := map[string]interface{}{}
		var requiredFields []string

		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if f.PkgPath != "" {
				continue
			}

			jsonName := jsonFieldName(f)
			if jsonName == "" {
				continue
			}

			fieldSchema, err := typeToJSONSchema(f.Type)
			if err != nil {
				return nil, err
			}

			props[jsonName] = fieldSchema

			// Fields that aren't pointers or slices/maps/interfaces are considered required
			// (This is a heuristic, modify as needed)
			if f.Type.Kind() != reflect.Ptr && f.Type.Kind() != reflect.Slice && f.Type.Kind() != reflect.Map && f.Type.Kind() != reflect.Interface {
				requiredFields = append(requiredFields, jsonName)
			}
		}

		objSchema := map[string]interface{}{
			"type":       "object",
			"properties": props,
		}
		if len(requiredFields) > 0 {
			objSchema["required"] = requiredFields
		}
		return objSchema, nil

	case reflect.String:
		return map[string]interface{}{"type": "string"}, nil
	case reflect.Int, reflect.Int64, reflect.Int32:
		return map[string]interface{}{"type": "integer"}, nil
	case reflect.Float32, reflect.Float64:
		return map[string]interface{}{"type": "number"}, nil
	case reflect.Bool:
		return map[string]interface{}{"type": "boolean"}, nil
	case reflect.Slice, reflect.Array:
		elemSchema, err := typeToJSONSchema(t.Elem())
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{
			"type":  "array",
			"items": elemSchema,
		}, nil
	case reflect.Map:
		valSchema, err := typeToJSONSchema(t.Elem())
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{
			"type":                 "object",
			"additionalProperties": valSchema,
		}, nil
	case reflect.Interface:
		return map[string]interface{}{"type": "object"}, nil
	default:
		return map[string]interface{}{"type": "string"}, nil
	}
}

func jsonFieldName(f reflect.StructField) string {
	tag := f.Tag.Get("json")
	if tag == "-" {
		return ""
	}
	if tag == "" {
		return strings.ToLower(f.Name)
	}
	parts := strings.Split(tag, ",")
	return parts[0]
}
