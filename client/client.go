package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Auth holds the credentials sent with every request. A remote MyWant
// deployment usually sits behind an authenticating proxy (the fly.io GUI app
// terminates Basic auth in front of the private backend), so talking to
// anything other than a local server needs these.
//
// Token wins over Username/Password when both are set.
type Auth struct {
	Username string
	Password string
	Token    string
}

// IsZero reports whether no credential at all is configured.
func (a Auth) IsZero() bool {
	return a.Username == "" && a.Password == "" && a.Token == ""
}

// defaultAuth is applied to clients built by NewClient/NewClientWithTimeout.
// The CLI resolves credentials once at startup (from the active context) and
// installs them here, so the ~40 NewClient call sites stay unchanged.
var defaultAuth Auth

// SetDefaultAuth sets the credentials used by subsequently created clients.
func SetDefaultAuth(a Auth) {
	defaultAuth = a
}

// Client is the MyWant API client
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
	Auth       Auth
}

// NewClient creates a new API client
func NewClient(baseURL string) *Client {
	return NewClientWithTimeout(baseURL, 30*time.Second)
}

// NewClientWithTimeout creates a new API client with custom timeout
func NewClientWithTimeout(baseURL string, timeout time.Duration) *Client {
	return &Client{
		BaseURL: baseURL,
		Auth:    defaultAuth,
		HTTPClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// ValidateWantConfig sends a want configuration for validation
func (c *Client) ValidateWantConfig(config Config) (*ValidationResult, error) {
	var result ValidationResult
	err := c.Request("POST", "/api/v1/wants/validate", config, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// Request performs an HTTP request and decodes JSON response
func (c *Client) Request(method, path string, body any, result any) error {
	resp, err := c.doRequest(method, path, body, "application/json")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Decode response if result pointer is provided
	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return nil
}

// RawRequest performs an HTTP request and returns the raw response body
func (c *Client) RawRequest(method, path string, body any, contentType string) ([]byte, error) {
	resp, err := c.doRequest(method, path, body, contentType)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

// doRequest is an internal helper to perform HTTP requests
func (c *Client) doRequest(method, path string, body any, contentType string) (*http.Response, error) {
	u, err := url.Parse(c.BaseURL + path)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	var bodyReader io.Reader
	if body != nil {
		if data, ok := body.([]byte); ok {
			bodyReader = bytes.NewReader(data)
		} else {
			jsonBody, err := json.Marshal(body)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal request body: %w", err)
			}
			bodyReader = bytes.NewReader(jsonBody)
		}
	}

	req, err := http.NewRequest(method, u.String(), bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", contentType)
	req.Header.Set("User-Agent", "mywant/1.0.0")

	switch {
	case c.Auth.Token != "":
		req.Header.Set("Authorization", "Bearer "+c.Auth.Token)
	case c.Auth.Username != "" || c.Auth.Password != "":
		req.SetBasicAuth(c.Auth.Username, c.Auth.Password)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	// Handle error responses
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if (resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden) && c.Auth.IsZero() {
			return nil, fmt.Errorf("API error (status %d): %s\n(no credentials sent — set username/password on the active context: mywant config set-context <name> --username ...)",
				resp.StatusCode, string(respBody))
		}
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	return resp, nil
}
