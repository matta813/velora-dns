package update

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

type Status struct {
	State         State     `json:"state"`
	Installed     string    `json:"installed"`
	FromVersion   string    `json:"from_version,omitempty"`
	ToVersion     string    `json:"to_version,omitempty"`
	Channel       string    `json:"channel,omitempty"`
	StartedAt     time.Time `json:"started_at,omitempty"`
	LastCompleted time.Time `json:"last_completed,omitempty"`
	Updating      bool      `json:"updating"`
	Error         string    `json:"error,omitempty"`
	RollbackUsed  bool      `json:"rollback_used,omitempty"`
	ReadinessOK   bool      `json:"readiness_ok,omitempty"`
}

type RequestResponse struct {
	Status  string `json:"status"`
	Version string `json:"version,omitempty"`
	Message string `json:"message,omitempty"`
}

// Client talks to the privileged updater over its fixed Unix socket.
type Client struct {
	SocketPath string
	Timeout    time.Duration
}

func (c Client) Status(ctx context.Context) (Status, error) {
	var result Status
	err := c.do(ctx, http.MethodGet, "/status", nil, &result)
	return result, err
}

func (c Client) History(ctx context.Context) ([]Entry, error) {
	var result []Entry
	err := c.do(ctx, http.MethodGet, "/history", nil, &result)
	if result == nil {
		result = []Entry{}
	}
	return result, err
}

func (c Client) Check(ctx context.Context) (CheckResult, error) {
	var result CheckResult
	err := c.do(ctx, http.MethodGet, "/check", nil, &result)
	return result, err
}

func (c Client) Request(ctx context.Context) (RequestResponse, error) {
	var result RequestResponse
	err := c.do(ctx, http.MethodPost, "/update", map[string]string{"action": "update"}, &result)
	return result, err
}

func (c Client) do(ctx context.Context, method, path string, body any, target any) error {
	if c.SocketPath == "" {
		return fmt.Errorf("updater socket is not configured")
	}
	var payload io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(data)
	}
	timeout := c.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	dialer := net.Dialer{Timeout: timeout}
	httpClient := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, "unix", c.SocketPath)
		}},
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://updater"+path, payload)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("contact updater agent: %w", err)
	}
	defer response.Body.Close()
	limited := io.LimitReader(response.Body, 1<<20)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var failure struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(limited).Decode(&failure)
		if failure.Error == "" {
			failure.Error = http.StatusText(response.StatusCode)
		}
		return &AgentError{StatusCode: response.StatusCode, Message: failure.Error}
	}
	if err := json.NewDecoder(limited).Decode(target); err != nil {
		return fmt.Errorf("decode updater response: %w", err)
	}
	return nil
}

type AgentError struct {
	StatusCode int
	Message    string
}

func (e *AgentError) Error() string { return e.Message }
