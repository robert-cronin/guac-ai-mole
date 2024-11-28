package guac

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/Khan/genqlient/graphql"
)

type Client struct {
	client graphql.Client
}

func NewClient(endpoint string) (*Client, error) {
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
	req := graphql.Request{
		Query:     query,
		Variables: variables,
	}
	var resp *graphql.Response

	err := c.client.MakeRequest(ctx, &req, resp)
	if err != nil {
		return nil, err
	}

	return resp, nil
}
