package guac

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/Khan/genqlient/graphql"
	"github.com/sozercan/guac-ai-mole/internal/guac/tools"
)

type Client struct {
	client graphql.Client
}

func NewClient(endpoint string) (*Client, error) {
	slog.Info("Creating GUAC client", "endpoint", endpoint)
	if endpoint == "" {
		return nil, fmt.Errorf("GUAC endpoint cannot be empty")
	}

	httpClient := defaultHTTPClient()
	client := graphql.NewClient(endpoint, httpClient)

	return &Client{
		client: client,
	}, nil
}

// CallGUACOperation calls a GUAC operation by name with the given JSON arguments.
// fnName: e.g. "github.com/guacsec/guac/pkg/assembler/clients/generated.Dependencies"
// arguments: JSON that matches the filter schema of the operation.
func (c *Client) CallGUACOperation(ctx context.Context, fnName string, arguments json.RawMessage) (interface{}, error) {
	slog.Info("Calling GUAC operation", "fnName", fnName, "arguments", string(arguments))

	// Directly invoke the GUAC operation using the tools.InvokeGUACOperation function
	result, err := tools.InvokeGUACOperation(ctx, c.client, fnName, arguments)
	if err != nil {
		slog.Error("GUAC operation invocation failed", "error", err)
		return nil, err
	}

	slog.Info("GUAC operation invoked successfully")
	return result, nil
}

func defaultHTTPClient() *http.Client {
	return defaultHTTPClientWithTimeout(30)
}

func defaultHTTPClientWithTimeout(seconds int) *http.Client {
	return &http.Client{
		Timeout: time.Duration(seconds) * time.Second,
	}
}
