// Package http provides the HTTP replay client adapter.
package http //nolint:revive // package name matches directory convention

import (
	"bytes"
	"context"
	"fmt"
	stdhttp "net/http"
	"time"
)

// ReplayClient POSTs original payloads back through EventSource webhooks.
type ReplayClient struct {
	client *stdhttp.Client
}

// NewReplayClient constructs a ReplayClient. Pass nil to use a default client
// with 30s timeout.
func NewReplayClient(c *stdhttp.Client) *ReplayClient {
	if c == nil {
		c = &stdhttp.Client{Timeout: 30 * time.Second}
	}
	return &ReplayClient{client: c}
}

// Replay implements port.EventReplayClient.
func (c *ReplayClient) Replay(ctx context.Context, url string, payload []byte, headers map[string][]string) error {
	req, err := stdhttp.NewRequestWithContext(ctx, stdhttp.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	if len(payload) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, vv := range headers {
		for _, v := range vv {
			req.Header.Add(k, v)
		}
	}

	resp, err := c.client.Do(req) //nolint:gosec // URL comes from stored DLQ record written by operator-controlled sensor config, not user input
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("replay received non-2xx status: %d", resp.StatusCode)
	}
	return nil
}
