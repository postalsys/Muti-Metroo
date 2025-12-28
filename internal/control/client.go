package control

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"
)

// Client is a control socket client.
type Client struct {
	socketPath string
	httpClient *http.Client
}

// NewClient creates a new control client.
func NewClient(socketPath string) *Client {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socketPath)
		},
	}

	return &Client{
		socketPath: socketPath,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   10 * time.Second,
		},
	}
}

// Status retrieves the agent status.
func (c *Client) Status(ctx context.Context) (*StatusResponse, error) {
	resp, err := c.get(ctx, "/status")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var status StatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &status, nil
}

// Peers retrieves the list of connected peers.
func (c *Client) Peers(ctx context.Context) (*PeersResponse, error) {
	resp, err := c.get(ctx, "/peers")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var peers PeersResponse
	if err := json.NewDecoder(resp.Body).Decode(&peers); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &peers, nil
}

// Routes retrieves the routing table.
func (c *Client) Routes(ctx context.Context) (*RoutesResponse, error) {
	resp, err := c.get(ctx, "/routes")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var routes RoutesResponse
	if err := json.NewDecoder(resp.Body).Decode(&routes); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &routes, nil
}

// get performs a GET request to the control socket.
func (c *Client) get(ctx context.Context, path string) (*http.Response, error) {
	// Use a dummy host since we're connecting via Unix socket
	url := "http://localhost" + path

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	return resp, nil
}

// Close closes the client.
func (c *Client) Close() error {
	c.httpClient.CloseIdleConnections()
	return nil
}
