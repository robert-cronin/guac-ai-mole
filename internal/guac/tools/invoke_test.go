package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Khan/genqlient/graphql"
	model "github.com/guacsec/guac/pkg/assembler/clients/generated"
	"github.com/sozercan/guac-ai-mole/internal/helpers"
	"github.com/stretchr/testify/assert"
)

// We'll reuse the allowedOperations from toolspecs.go, but let's redefine it here for clarity in tests.
// This init sets up Dependencies as the allowed operation.
func init() {
	allowedOperations = []allowedOperationType{
		{
			Operation:   model.Packages,
			Description: `For testing`,
		},
	}
}

func TestInvokeGUACOperation(t *testing.T) {
	// Mock GraphQL response. Adjust fields as necessary.
	// This is a minimal GraphQL response that might match what the Dependencies query returns.
	mockResponse := `
{
  "data": {
    "packages": [
      {
        "id": "37",
        "type": "maven",
        "namespaces": [
          {
            "id": "2995",
            "namespace": "org.apache.logging.log4j",
            "names": [
              {
                "id": "2996",
                "name": "log4j-core",
                "versions": [
                  {
                    "id": "2997",
                    "version": "2.8.1",
                    "qualifiers": [],
                    "subpath": ""
                  }
                ]
              }
            ]
          }
        ]
      }
    ]
  }
}`

	// Create a test server that always responds with mockResponse
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Optionally check the incoming request's query or variables here if needed.
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(mockResponse))
	}))
	defer ts.Close()

	// Create a genqlient GraphQL client pointing to our mock server
	client := graphql.NewClient(ts.URL, http.DefaultClient)

	// The function name for Dependencies as known at runtime
	fnName := GetRuntimeFuncName(model.Packages)

	// Mock JSON arguments for Is
	// pkg:maven/org.apache.logging.log4j%3Alog4j-api@2.8.1
	filter := model.PkgSpec{
		Namespace: helpers.Ptr("org.apache.logging.log4j"),
		Name:      helpers.Ptr("log4j-core"),
		Version:   helpers.Ptr("2.8.1"),
	}
	// marshal the filter
	llmJSON, err := json.Marshal(filter)
	assert.NoError(t, err, "failed to marshal filter")

	// Invoke the operation
	ctx := context.Background()
	result, err := InvokeGUACOperation(ctx, client, fnName, []byte(llmJSON))
	assert.NoError(t, err, "expected no error invoking operation")

	resultBytes, err := json.Marshal(result)
	assert.NoError(t, err, "failed to marshal result")

	// Unmarshal into a generic map to verify the content
	var genericRes map[string]interface{}
	err = json.Unmarshal(resultBytes, &genericRes)
	assert.NoError(t, err, "failed to unmarshal result into generic map")

	// Check that the mock response made it through
	dataVal, ok := genericRes["packages"]
	assert.True(t, ok, "expected 'packages' field in response")
	pkgs, ok := dataVal.([]interface{})
	assert.True(t, ok, "packages should be a list")
	assert.Len(t, pkgs, 1, "expected one package in mock response")

	pkgMap, ok := pkgs[0].(map[string]interface{})
	assert.True(t, ok, "expected package to be a map")
	assert.Equal(t, "maven", pkgMap["type"], "expected package type to match")
	assert.Contains(t, pkgMap, "namespaces", "expected namespaces in package")
	assert.Len(t, pkgMap["namespaces"].([]interface{}), 1, "expected one namespace in package")
}
