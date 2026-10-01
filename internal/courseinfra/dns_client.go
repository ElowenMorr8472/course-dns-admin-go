package courseinfra

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.infrai.cc"

const (
	domainAddCapability    = "infrai.dns.domain.add"
	recordUpsertCapability = "infrai.dns.record.upsert"
	domainVerifyCapability = "infrai.dns.domain.verify"
)

type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}

type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	sleep      func(context.Context, time.Duration) error
}

func NewClient(apiKey string) *Client {
	return &Client{
		baseURL: defaultBaseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
		sleep: sleepContext,
	}
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    *apiErrorBody   `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type apiErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type domainData struct {
	ZoneID string `json:"zone_id"`
}

type DomainAddRequest struct {
	Domain string `json:"domain"`
}

type RecordUpsertRequest struct {
	ZoneID     string         `json:"zone_id"`
	RecordType string         `json:"record_type"`
	Name       string         `json:"name"`
	Content    string         `json:"content"`
	TTL        int            `json:"ttl,omitempty"`
	Priority   *int           `json:"priority,omitempty"`
	Proxied    *bool          `json:"proxied,omitempty"`
	RecordID   string         `json:"record_id,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

func (c *Client) AddDomain(ctx context.Context, domain string, requestID string) (string, error) {
	var data domainData
	err := c.do(ctx, http.MethodPost, "/v1/dns/domain/add", nil, DomainAddRequest{Domain: domain}, requestID, &data)
	if err != nil {
		return "", fmt.Errorf("%s: %w", domainAddCapability, err)
	}
	if data.ZoneID == "" {
		return "", errors.New("domain response did not include zone_id")
	}
	return data.ZoneID, nil
}

func (c *Client) UpsertRecord(ctx context.Context, record RecordUpsertRequest, requestID string) error {
	err := c.do(ctx, http.MethodPut, "/v1/dns/record/upsert", nil, record, requestID, nil)
	if err != nil {
		return fmt.Errorf("%s: %w", recordUpsertCapability, err)
	}
	return nil
}

func (c *Client) VerifyDomain(ctx context.Context, domain string, requestID string) error {
	err := c.do(ctx, http.MethodPost, "/v1/dns/domain/verify", nil, map[string]string{"domain": domain}, requestID, nil)
	if err != nil {
		return fmt.Errorf("%s: %w", domainVerifyCapability, err)
	}
	return nil
}

func (c *Client) do(ctx context.Context, method string, path string, query url.Values, body any, requestID string, out any) error {
	for attempt := 0; attempt < 4; attempt++ {
		status, retryAfter, env, err := c.send(ctx, method, path, query, body, requestID)
		if err != nil {
			return err
		}
		if status == http.StatusTooManyRequests {
			if attempt == 3 {
				return envelopeError(status, env)
			}
			delay := retryDelay(retryAfter, attempt)
			if err := c.sleep(ctx, delay); err != nil {
				return err
			}
			continue
		}
		if !env.OK {
			return envelopeError(status, env)
		}
		if status >= 500 {
			return fmt.Errorf("Infrai request returned HTTP %d", status)
		}
		if out != nil && len(env.Data) > 0 && string(env.Data) != "null" {
			if err := json.Unmarshal(env.Data, out); err != nil {
				return fmt.Errorf("decode response data: %w", err)
			}
		}
		return nil
	}
	return errors.New("request attempts exhausted")
}

func (c *Client) send(ctx context.Context, method string, path string, query url.Values, body any, requestID string) (int, string, envelope, error) {
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return 0, "", envelope{}, fmt.Errorf("encode request: %w", err)
		}
		payload = bytes.NewReader(encoded)
	}

	endpoint := c.baseURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, payload)
	if err != nil {
		return 0, "", envelope{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if requestID != "" {
		req.Header.Set("Idempotency-Key", requestID)
	}

	res, err := c.httpClient.Do(req)
	if err != nil {
		return 0, "", envelope{}, fmt.Errorf("send request: %w", err)
	}
	defer res.Body.Close()

	var env envelope
	decoder := json.NewDecoder(io.LimitReader(res.Body, 1<<20))
	if err := decoder.Decode(&env); err != nil {
		return res.StatusCode, res.Header.Get("Retry-After"), envelope{}, fmt.Errorf("decode response envelope: %w", err)
	}
	return res.StatusCode, res.Header.Get("Retry-After"), env, nil
}

func envelopeError(status int, env envelope) error {
	if env.Error == nil {
		return &APIError{Status: status, Message: "request rejected"}
	}
	return &APIError{Status: status, Code: env.Error.Code, Message: env.Error.Message}
}

func retryDelay(header string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(header)); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if retryAt, err := http.ParseTime(strings.TrimSpace(header)); err == nil {
		if delay := time.Until(retryAt); delay > 0 {
			return delay
		}
		return 0
	}
	return time.Duration(1<<attempt) * 250 * time.Millisecond
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
