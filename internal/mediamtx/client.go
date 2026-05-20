package mediamtx

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		BaseURL:    baseURL,
		HTTPClient: &http.Client{Timeout: 5 * time.Second},
	}
}

type PathInfo struct {
	Name      string        `json:"name"`
	Ready     bool          `json:"ready"`
	Available bool          `json:"available"`
	Online    bool          `json:"online"`
	Tracks    []string      `json:"tracks"`
	Tracks2   []interface{} `json:"tracks2"`
}

type pathsListResponse struct {
	Items []PathInfo `json:"items"`
}

func (c *Client) GetPath(ctx context.Context, path string) (*PathInfo, error) {
	url := fmt.Sprintf("%s/v3/paths/list", c.BaseURL)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var list pathsListResponse
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, err
	}

	for _, item := range list.Items {
		if item.Name == path {
			return &item, nil
		}
	}

	return nil, fmt.Errorf("path not found: %s", path)
}

func (c *Client) WaitUntilHealthy(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(ctx, "GET", c.BaseURL+"/v3/paths/list", nil)
		if err == nil {
			resp, err := c.HTTPClient.Do(req)
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					return nil
				}
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return fmt.Errorf("mediamtx not healthy after %v", timeout)
}

func (c *Client) WaitForPathReady(ctx context.Context, path string, timeout time.Duration) (*PathInfo, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		info, err := c.GetPath(ctx, path)
		if err == nil && info != nil {
			if info.Ready && info.Available && info.Online && (len(info.Tracks) > 0 || len(info.Tracks2) > 0) {
				return info, nil
			}
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
	return nil, fmt.Errorf("timeout waiting for path ready: %s", path)
}
