package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	DefaultTimeout       = 5 * time.Second
	maxResponseBodyBytes = 64 << 10
)

var (
	ErrInvalidConfig   = errors.New("invalid client configuration")
	ErrAuth            = errors.New("authentication failed")
	ErrUnavailable     = errors.New("server unavailable")
	ErrInvalidResponse = errors.New("invalid server response")
)

type Client struct {
	endpoint   string
	apiKey     string
	httpClient *http.Client
}

type Options struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

type Notification struct {
	Event   string `json:"event"`
	Agent   string `json:"agent"`
	Message string `json:"message"`
}

type notifyResponse struct {
	OK    bool   `json:"ok"`
	Event string `json:"event,omitempty"`
	Error string `json:"error,omitempty"`
}

func New(opts Options) (*Client, error) {
	endpoint, err := notifyEndpoint(opts.BaseURL)
	if err != nil {
		return nil, err
	}
	if opts.APIKey == "" {
		return nil, fmt.Errorf("%w: missing api key", ErrAuth)
	}

	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: DefaultTimeout}
	}

	return &Client{
		endpoint:   endpoint,
		apiKey:     opts.APIKey,
		httpClient: httpClient,
	}, nil
}

func (c *Client) Notify(ctx context.Context, n Notification) error {
	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(n); err != nil {
		return fmt.Errorf("%w: encode request", ErrInvalidConfig)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, &body)
	if err != nil {
		return fmt.Errorf("%w: build request: %v", ErrInvalidConfig, err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes+1))
	if err != nil {
		return fmt.Errorf("%w: read response: %v", ErrUnavailable, err)
	}
	responseBodyTooLarge := len(responseBody) > maxResponseBodyBytes
	if responseBodyTooLarge {
		responseBody = responseBody[:maxResponseBodyBytes]
	}

	if resp.StatusCode == http.StatusOK {
		if responseBodyTooLarge {
			return fmt.Errorf("%w: response body too large", ErrInvalidResponse)
		}
		var envelope notifyResponse
		if err := json.Unmarshal(responseBody, &envelope); err != nil {
			return fmt.Errorf("%w: decode success response", ErrInvalidResponse)
		}
		if !envelope.OK {
			return fmt.Errorf("%w: success response did not confirm acceptance", ErrInvalidResponse)
		}
		return nil
	}

	message := responseErrorMessage(responseBody)
	if resp.StatusCode == http.StatusUnauthorized {
		return statusError(ErrAuth, resp.StatusCode, message)
	}
	return statusError(ErrUnavailable, resp.StatusCode, message)
}

func notifyEndpoint(rawBase string) (string, error) {
	if rawBase == "" {
		return "", fmt.Errorf("%w: missing server url", ErrInvalidConfig)
	}

	u, err := url.Parse(rawBase)
	if err != nil {
		return "", fmt.Errorf("%w: parse server url: %v", ErrInvalidConfig, err)
	}
	if u.Scheme == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("%w: server url must use http or https", ErrInvalidConfig)
	}
	if u.Host == "" {
		return "", fmt.Errorf("%w: server url must include a host", ErrInvalidConfig)
	}

	u.Path = strings.TrimRight(u.Path, "/") + "/notify"
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

func responseErrorMessage(body []byte) string {
	var envelope notifyResponse
	if err := json.Unmarshal(body, &envelope); err != nil {
		return ""
	}
	return envelope.Error
}

func statusError(category error, status int, message string) error {
	if message == "" {
		return fmt.Errorf("%w: server returned %s", category, http.StatusText(status))
	}
	return fmt.Errorf("%w: server returned %s: %s", category, http.StatusText(status), message)
}
