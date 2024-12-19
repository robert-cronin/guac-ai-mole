package guac

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"reflect"
	"runtime"
	"strings"

	"github.com/Khan/genqlient/graphql"
	model "github.com/guacsec/guac/pkg/assembler/clients/generated"
	"github.com/openai/openai-go"
)

type ToolFunctionType = func(context.Context, ...interface{}) (interface{}, error)

type DefinitionType struct {
	Spec     openai.ChatCompletionToolParam
	Function ToolFunctionType
}

type allowedOperationType struct {
	Operation   interface{}
	Description string
}

// GUACTools holds a GUAC GraphQL client and tool definitions.
// It lets us dynamically create and register tools (both GUAC and local).
type GUACTools struct {
	client            graphql.Client
	allowedOperations []allowedOperationType
	definitions       []DefinitionType
}

func NewGUACTools(endpoint string) (*GUACTools, error) {
	if endpoint == "" {
		return nil, fmt.Errorf("GUAC endpoint cannot be empty")
	}

	httpClient := defaultHTTPClientWithTimeout(30)
	gqlClient := graphql.NewClient(endpoint, httpClient)

	// to add more operations, add them to the list below with a useful description
	g := &GUACTools{
		client: gqlClient,
		allowedOperations: []allowedOperationType{
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
and it has an empty string for vulnerabilityID. Setting it to false filters out all results that are "novuln".
`,
			},
			{
				Operation: model.Packages,
				Description: `
PkgSpec allows filtering the list of packages.
Each field matches a qualifier from pURL. Use null to match on all values.
For example, to get all packages in GUAC backend, use a PkgSpec where every field is null.
`,
			},
		},
		definitions: []DefinitionType{},
	}

	// Register GUAC-based operations
	if err := g.registerGQLOperations(); err != nil {
		return nil, err
	}

	// Register KnownQuery local tool
	g.registerKnownQueryTool()

	return g, nil
}

func (g *GUACTools) GetDefinitions() []DefinitionType {
	return g.definitions
}

func (g *GUACTools) registerKnownQueryTool() {
	toolParam := openai.ChatCompletionToolParam{
		Type: openai.F(openai.ChatCompletionToolTypeFunction),
		Function: openai.F(openai.FunctionDefinitionParam{
			Name:        openai.String("KnownQuery"),
			Description: openai.String("query all known info about a package, source, or artifact, just like guacone query known (local tool)"),
			Parameters: openai.F(openai.FunctionParameters(map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"subjectType": map[string]interface{}{
						"type":        "string",
						"description": "one of: package, source, artifact",
					},
					"subject": map[string]interface{}{
						"type":        "string",
						"description": `for package: purl, for source: vcs_tool+transport, for artifact: algorithm:digest`,
					},
				},
				"required": []string{"subjectType", "subject"},
			})),
		}),
	}

	knownQueryFn := func(ctx context.Context, args ...interface{}) (interface{}, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("expected 1 argument, got %d", len(args))
		}

		rawArgs, ok := args[0].(json.RawMessage)
		if !ok {
			return nil, fmt.Errorf("expected arguments to be json.RawMessage, got %T", args[0])
		}

		var input struct {
			SubjectType string `json:"subjectType"`
			Subject     string `json:"subject"`
		}
		if err := json.Unmarshal(rawArgs, &input); err != nil {
			return nil, fmt.Errorf("invalid arguments for KnownQuery: %w", err)
		}

		// Call KnownQuery logic
		return KnownQuery(ctx, g.client, input.SubjectType, input.Subject)
	}

	g.definitions = append(g.definitions, DefinitionType{
		Spec:     toolParam,
		Function: knownQueryFn,
	})
}

// registerGQLOperations generates tool definitions for GUAC operations and registers them.
func (g *GUACTools) registerGQLOperations() error {
	for _, ao := range g.allowedOperations {
		fnVal := reflect.ValueOf(ao.Operation)
		if fnVal.Kind() != reflect.Func {
			slog.Error("allowedOperations entry is not a function type", "type", fnVal.Type().String())
			continue
		}

		fnType := fnVal.Type()
		ptr := fnVal.Pointer()
		rf := runtime.FuncForPC(ptr)
		if rf == nil {
			slog.Error("failed to get runtime function for function", "function", fnType.String())
			continue
		}

		toolName := g.getRuntimeFuncName(ao.Operation)

		numIn := fnType.NumIn()
		if numIn < 3 {
			slog.Error("function doesn't have expected # of arguments", "function", toolName)
			continue
		}

		filterType := fnType.In(numIn - 1)
		filterSchema, err := typeToJSONSchema(filterType)
		if err != nil {
			slog.Error("failed to generate JSON schema", "type", filterType.String(), "error", err)
			continue
		}

		toolParam := openai.ChatCompletionToolParam{
			Type: openai.F(openai.ChatCompletionToolTypeFunction),
			Function: openai.F(openai.FunctionDefinitionParam{
				Name:        openai.String(toolName),
				Description: openai.String(strings.TrimSpace(ao.Description)),
				Parameters:  openai.F(openai.FunctionParameters(filterSchema)),
			}),
		}

		fn := func(ctx context.Context, args ...interface{}) (interface{}, error) {
			if len(args) != 1 {
				return nil, fmt.Errorf("expected 1 argument (json), got %d", len(args))
			}
			rawArgs, ok := args[0].(json.RawMessage)
			if !ok {
				return nil, fmt.Errorf("expected arguments to be json.RawMessage, got %T", args[0])
			}

			filterVal, err := g.unmarshalFilterFromJSON(ao.Operation, rawArgs)
			if err != nil {
				return nil, err
			}

			return g.callOperation(ctx, ao.Operation, filterVal)
		}

		g.definitions = append(g.definitions, DefinitionType{
			Spec:     toolParam,
			Function: fn,
		})
	}

	return nil
}

func (g *GUACTools) getRuntimeFuncName(fn interface{}) string {
	fnVal := reflect.ValueOf(fn)
	ptr := fnVal.Pointer()
	rf := runtime.FuncForPC(ptr)
	if rf == nil {
		return ""
	}
	name := rf.Name()
	lastDot := strings.LastIndex(name, ".")
	if lastDot == -1 {
		return name
	}
	return name[lastDot+1:]
}

// IsGuacGQLOperation checks if the function name corresponds to a known GUAC GQL operation.
func (g *GUACTools) IsGuacGQLOperation(fnName string) bool {
	for _, ao := range g.allowedOperations {
		if g.getRuntimeFuncName(ao.Operation) == fnName {
			return true
		}
	}
	return false
}

func (g *GUACTools) unmarshalFilterFromJSON(fn interface{}, data []byte) (interface{}, error) {
	fnVal := reflect.ValueOf(fn)
	fnType := fnVal.Type()

	if fnVal.Kind() != reflect.Func {
		return nil, fmt.Errorf("provided operation is not a function")
	}

	numIn := fnType.NumIn()
	if numIn < 3 {
		return nil, fmt.Errorf("operation does not have expected number of arguments")
	}

	filterType := fnType.In(numIn - 1)

	isPtr := filterType.Kind() == reflect.Ptr
	var filterVal reflect.Value
	if isPtr {
		filterVal = reflect.New(filterType.Elem())
	} else {
		filterVal = reflect.New(filterType)
	}

	if err := json.Unmarshal(data, filterVal.Interface()); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON into filter type %s: %w", filterType.String(), err)
	}

	if !isPtr {
		return filterVal.Elem().Interface(), nil
	}
	return filterVal.Interface(), nil
}

func (g *GUACTools) callOperation(ctx context.Context, fn interface{}, filter interface{}) (interface{}, error) {
	fnVal := reflect.ValueOf(fn)
	fnType := fnVal.Type()

	if fnVal.Kind() != reflect.Func {
		return nil, fmt.Errorf("provided operation is not a function")
	}

	numIn := fnType.NumIn()
	if numIn < 3 {
		return nil, fmt.Errorf("operation does not have the expected number of arguments")
	}

	args := []reflect.Value{
		reflect.ValueOf(ctx),
		reflect.ValueOf(g.client),
		reflect.ValueOf(filter),
	}

	resVals := fnVal.Call(args)
	if len(resVals) != 2 {
		return nil, fmt.Errorf("operation does not return the expected number of values (expected 2)")
	}

	resVal := resVals[0]
	errVal := resVals[1]

	if !errVal.IsNil() {
		errInterface := errVal.Interface().(error)
		slog.Error("operation call returned error", "error", errInterface)
		return nil, errInterface
	}

	return resVal.Interface(), nil
}

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

func defaultHTTPClientWithTimeout(seconds int) *http.Client {
	return &http.Client{
		Timeout: 0, // in real code, set a timeout if needed
	}
}
