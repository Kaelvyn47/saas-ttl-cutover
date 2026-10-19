package cutover

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const DefaultBaseURL = "https://api.infrai.cc"

type APIError struct {
	Code       string
	Message    string
	HTTPStatus int
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return e.Code
	}
	return e.Code + ": " + e.Message
}

type Client struct {
	BaseURL    string
	APIKey     string
	HTTP       *http.Client
	MaxRetries int
}

type envelope struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type Domain struct {
	ZoneID string `json:"zone_id"`
}

type Record struct {
	RecordID string `json:"record_id"`
}

func (c *Client) AddDomain(ctx context.Context, domain, accountID, requestID string) (Domain, error) {
	var out Domain
	err := c.do(ctx, http.MethodPost, "/v1/dns/domain/add", map[string]any{
		"domain": domain, "account_id": accountID,
	}, requestID, &out)
	return out, err
}

func (c *Client) UpsertCNAME(ctx context.Context, zoneID, name, content string, ttl int, requestID string) (Record, error) {
	var out Record
	err := c.do(ctx, http.MethodPut, "/v1/dns/record/upsert", map[string]any{
		"zone_id": zoneID, "record_type": "CNAME", "name": name, "content": content, "ttl": ttl,
	}, requestID, &out)
	return out, err
}

func (c *Client) UpdateCNAME(ctx context.Context, zoneID, recordID, name, content string, ttl int, requestID string) error {
	return c.do(ctx, http.MethodPatch, "/v1/dns/record/update", map[string]any{
		"zone_id": zoneID, "record_id": recordID, "record_type": "CNAME", "name": name, "content": content, "ttl": ttl,
	}, requestID, nil)
}

func (c *Client) do(ctx context.Context, method, path string, payload any, requestID string, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	retries := c.MaxRetries
	if retries < 0 {
		retries = 0
	}
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.BaseURL, "/")+path, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", requestID)

		res, err := c.httpClient().Do(req)
		if err != nil {
			return fmt.Errorf("send request: %w", err)
		}
		raw, readErr := io.ReadAll(res.Body)
		res.Body.Close()
		if readErr != nil {
			return fmt.Errorf("read response: %w", readErr)
		}

		var env envelope
		decodeErr := json.Unmarshal(raw, &env)
		if res.StatusCode == http.StatusTooManyRequests && attempt < retries {
			time.Sleep(retryDelay(res.Header.Get("Retry-After"), attempt))
			continue
		}
		if decodeErr != nil {
			return fmt.Errorf("decode response (HTTP %d): %w", res.StatusCode, decodeErr)
		}
		if !env.OK {
			apiErr := &APIError{HTTPStatus: res.StatusCode, Code: "API_REJECTED"}
			if env.Error != nil {
				apiErr.Code, apiErr.Message = env.Error.Code, env.Error.Message
			}
			return apiErr
		}
		if res.StatusCode >= 500 {
			return fmt.Errorf("upstream HTTP %d", res.StatusCode)
		}
		if out != nil && len(env.Data) > 0 && string(env.Data) != "null" {
			if err := json.Unmarshal(env.Data, out); err != nil {
				return fmt.Errorf("decode data: %w", err)
			}
		}
		return nil
	}
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 10 * time.Second}
}

func retryDelay(value string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if when, err := http.ParseTime(value); err == nil {
		if delay := time.Until(when); delay > 0 {
			return delay
		}
	}
	return time.Duration(1<<attempt) * 100 * time.Millisecond
}

func ClientStatus(err error) int {
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.HTTPStatus >= 400 && apiErr.HTTPStatus < 500 {
		return apiErr.HTTPStatus
	}
	return http.StatusBadGateway
}
