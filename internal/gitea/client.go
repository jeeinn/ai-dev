package gitea

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/jeeinn/matea/internal/logging"
)

// Client is a Gitea API client.
type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

// NewClient creates a new Gitea API client.
func NewClient(baseURL, token string) *Client {
	return &Client{
		BaseURL: baseURL,
		Token:   token,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) doWithStatus(method, path string, body interface{}) ([]byte, int, error) {
	var reqBody []byte
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("marshal body: %w", err)
		}
		reqBody = data
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, c.BaseURL+"/api/v1"+path, bodyReader)
	if err != nil {
		return nil, 0, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "token "+c.Token)
	req.Header.Set("Content-Type", "application/json")

	return c.execute(req, reqBody)
}

func (c *Client) do(method, path string, body interface{}) ([]byte, error) {
	respBody, status, err := c.doWithStatus(method, path, body)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("API error %d: %s", status, string(respBody))
	}
	return respBody, nil
}

func (c *Client) execute(req *http.Request, reqBody []byte) ([]byte, int, error) {
	logGiteaRequest(req, reqBody)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		logGiteaTransportError(req, err)
		return nil, 0, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read response: %w", err)
	}

	logGiteaResponse(req, resp.StatusCode, respBody)
	if resp.StatusCode >= 400 {
		logging.Debugf("Gitea API error: %s %s status=%d body=%s",
			req.Method, req.URL.Path, resp.StatusCode, truncateForLog(string(respBody), debugBodyLimit))
	}

	return respBody, resp.StatusCode, nil
}
