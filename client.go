package rmailer

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

var (
	ErrMissingAccessToken = errors.New("r-mailer: access token is required")
	ErrMissingBaseURL     = errors.New("r-mailer: base URL is required")
	ErrMissingMessageID   = errors.New("r-mailer: message ID is required")
)

type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

func New(baseURL string) *Client {
	return &Client{
		BaseURL:    strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		HTTPClient: http.DefaultClient,
	}
}

type SendTemplateRequest struct {
	From           string         `json:"from"`
	To             []string       `json:"to"`
	Template       string         `json:"template"`
	Locale         string         `json:"locale"`
	Data           map[string]any `json:"data"`
	IdempotencyKey string         `json:"idempotency_key,omitempty"`
}

type Message struct {
	ID                string    `json:"id"`
	ClientID          string    `json:"client_id"`
	FromAddress       string    `json:"from_address"`
	ToAddresses       []string  `json:"to_addresses"`
	TemplateKey       string    `json:"template_key"`
	Locale            string    `json:"locale"`
	Subject           string    `json:"subject"`
	HTMLBody          string    `json:"html_body"`
	TextBody          string    `json:"text_body"`
	Status            string    `json:"status"`
	Provider          string    `json:"provider"`
	ProviderMessageID string    `json:"provider_message_id"`
	IdempotencyKey    string    `json:"idempotency_key,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	SentAt            time.Time `json:"sent_at"`
}

type APIError struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

type HTTPError struct {
	StatusCode int
	APIError   APIError
	Body       string
}

func (e *HTTPError) Error() string {
	if e.APIError.Message != "" {
		return fmt.Sprintf("r-mailer: %s: %s", e.APIError.Error, e.APIError.Message)
	}
	if e.APIError.Error != "" {
		return "r-mailer: " + e.APIError.Error
	}
	return fmt.Sprintf("r-mailer: request failed with status %d", e.StatusCode)
}

func (c *Client) SendTemplate(ctx context.Context, accessToken string, req SendTemplateRequest) (*Message, error) {
	var msg Message
	if err := c.doJSON(ctx, http.MethodPost, accessToken, []string{"v1", "messages", "send-template"}, req, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

func (c *Client) GetMessage(ctx context.Context, accessToken string, messageID string) (*Message, error) {
	if strings.TrimSpace(messageID) == "" {
		return nil, ErrMissingMessageID
	}
	var msg Message
	if err := c.doJSON(ctx, http.MethodGet, accessToken, []string{"v1", "messages", messageID}, nil, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

func (c *Client) doJSON(ctx context.Context, method, accessToken string, path []string, in any, out any) error {
	if strings.TrimSpace(accessToken) == "" {
		return ErrMissingAccessToken
	}
	endpoint, err := c.endpoint(path...)
	if err != nil {
		return err
	}

	var body io.Reader
	if in != nil {
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(in); err != nil {
			return fmt.Errorf("r-mailer: encode request: %w", err)
		}
		body = &buf
	}
	httpReq, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return err
	}
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+accessToken)
	if in != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}

	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	res, err := httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return decodeHTTPError(res.StatusCode, raw)
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("r-mailer: decode response: %w", err)
	}
	return nil
}

func (c *Client) endpoint(parts ...string) (string, error) {
	if strings.TrimSpace(c.BaseURL) == "" {
		return "", ErrMissingBaseURL
	}
	base, err := url.Parse(c.BaseURL)
	if err != nil {
		return "", err
	}
	if base.Scheme == "" || base.Host == "" {
		return "", fmt.Errorf("r-mailer: invalid base URL %q", c.BaseURL)
	}
	return url.JoinPath(base.String(), parts...)
}

func decodeHTTPError(statusCode int, raw []byte) error {
	apiErr := APIError{}
	_ = json.Unmarshal(raw, &apiErr)
	if apiErr.Error == "" && apiErr.Message == "" && len(raw) > 0 {
		apiErr.Message = string(raw)
	}
	return &HTTPError{
		StatusCode: statusCode,
		APIError:   apiErr,
		Body:       string(raw),
	}
}
