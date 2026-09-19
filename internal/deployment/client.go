package deployment

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"
)

// Client talks only to a local Unix socket. It never accepts a remote URL.
type Client struct{ SocketPath string }

func (c Client) request(ctx context.Context, method string, settings *Settings) (Response, error) {
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", c.SocketPath)
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 15 * time.Second}
	if settings != nil {
		encoded, err := json.Marshal(settings)
		if err != nil {
			return Response{}, err
		}
		request, err := http.NewRequestWithContext(ctx, method, "http://unix/settings", bytes.NewReader(encoded))
		if err != nil {
			return Response{}, err
		}
		request.Header.Set("Content-Type", "application/json")
		return c.do(client, request)
	}
	request, err := http.NewRequestWithContext(ctx, method, "http://unix/settings", nil)
	if err != nil {
		return Response{}, err
	}
	return c.do(client, request)
}

func (c Client) do(client *http.Client, request *http.Request) (Response, error) {
	response, err := client.Do(request)
	if err != nil {
		return Response{}, fmt.Errorf("deployment agent unavailable: %w", err)
	}
	defer response.Body.Close()
	var result Response
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return Response{}, fmt.Errorf("decode deployment agent response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Response{}, fmt.Errorf("deployment agent: %s", response.Status)
	}
	return result, nil
}

func (c Client) Get(ctx context.Context) (Response, error) {
	return c.request(ctx, http.MethodGet, nil)
}
func (c Client) Apply(ctx context.Context, settings Settings) (Response, error) {
	if err := settings.Validate(); err != nil {
		return Response{}, err
	}
	return c.request(ctx, http.MethodPut, &settings)
}
