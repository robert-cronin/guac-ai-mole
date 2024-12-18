package tools

import (
	"encoding/json"
	"reflect"
	"testing"

	model "github.com/guacsec/guac/pkg/assembler/clients/generated"
	"github.com/openai/openai-go"
	"github.com/stretchr/testify/assert"
)

func init() {
	// Set allowed operations for testing
	allowedOperations = []allowedOperationType{
		{
			Operation:   model.Dependencies,
			Description: `IsDependencySpec allows filtering the list of dependencies to return.`,
		},
	}
}

func TestGenerateToolDefinitionsFromGUAC(t *testing.T) {
	tools, err := generateDefinitionsFromGUAC()
	assert.NoError(t, err, "expected no error generating toolspecs")

	// With the new approach, we expect exactly one tool: "Dependencies".
	assert.Len(t, tools, 1, "expected one tool")

	tool := tools[0]
	f := tool.Function
	fnName := f.Value.Name.Value
	desc := f.Value.Description.Value
	params := f.Value.Parameters.Value

	expectFnName := "github.com/guacsec/guac/pkg/assembler/clients/generated.Dependencies"
	assert.Equal(t, expectFnName, fnName, "expected function name to match")
	assert.Contains(t, desc, "IsDependencySpec allows filtering the list of dependencies to return", "description should match the Dependencies operation")

	// Now we check the schema against IsDependencySpec
	checkSchemaFormat(t, params, reflect.TypeOf(model.IsDependencySpec{}))
}

func checkSchemaFormat(t *testing.T, params openai.FunctionParameters, typ reflect.Type) {
	data, err := json.Marshal(params)
	assert.NoError(t, err, "failed to marshal parameters to JSON")

	var schema map[string]interface{}
	err = json.Unmarshal(data, &schema)
	assert.NoError(t, err, "failed to unmarshal schema")

	assert.Equal(t, "object", schema["type"], "top-level schema should be type object")

	props, ok := schema["properties"].(map[string]interface{})
	assert.True(t, ok, "properties should be a map")
	assert.NotEmpty(t, props, "expected properties in schema")

	// Resolve pointer types if any
	for typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}

	assert.Equal(t, reflect.Struct, typ.Kind(), "expected a struct type for reflection")

	// Verify that each exported, non-ignored field of the struct is present as a property
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if f.PkgPath != "" {
			// unexported field, skip
			continue
		}
		jsonName := jsonFieldName(f)
		if jsonName == "" {
			// ignored field (json:"-"), skip
			continue
		}
		_, fieldPresent := props[jsonName]
		assert.Truef(t, fieldPresent, "expected field %q in properties", jsonName)
	}
}
