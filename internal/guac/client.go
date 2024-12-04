package guac

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/Khan/genqlient/graphql"
)

type Client struct {
	client graphql.Client
}

func NewClient(endpoint string) (*Client, error) {
	slog.Info("Creating GUAC client", "endpoint", endpoint)
	if endpoint == "" {
		return nil, fmt.Errorf("GUAC endpoint cannot be empty")
	}

	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}

	client := graphql.NewClient(endpoint, httpClient)

	return &Client{
		client: client,
	}, nil
}

func (c *Client) ExecuteGraphQL(ctx context.Context, query string, variables interface{}) (*graphql.Response, error) {
	slog.Info("Executing GraphQL query", "query", query, "variables", variables)
	req := graphql.Request{
		Query:     query,
		Variables: variables,
	}
	var resp *graphql.Response

	err := c.client.MakeRequest(ctx, &req, resp)
	if err != nil {
		slog.Error("GraphQL query execution failed", "error", err)
		return nil, err
	}

	slog.Info("GraphQL query executed successfully")
	return resp, nil
}
