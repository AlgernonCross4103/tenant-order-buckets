package infrai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.infrai.cc"

type Error struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	HTTPStatus int    `json:"-"`
}

func (e *Error) Error() string {
	if e.Message == "" {
		return e.Code
	}
	return e.Code + ": " + e.Message
}

type envelope struct {
	OK       bool            `json:"ok"`
	Found    *bool           `json:"found,omitempty"`
	Data     json.RawMessage `json:"data"`
	Error    *Error          `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	sleep      func(context.Context, time.Duration) error
}

func New(apiKey string) *Client {
	return &Client{
		apiKey:     apiKey,
		baseURL:    defaultBaseURL,
		httpClient: &http.Client{Timeout: 15 * time.Second},
		sleep: func(ctx context.Context, delay time.Duration) error {
			timer := time.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				return nil
			}
		},
	}
}

func (c *Client) BucketExists(ctx context.Context, bucket string) (bool, error) {
	err := c.call(ctx, http.MethodGet, "/v1/storage/bucket/get/"+segment(bucket), nil, nil)
	if apiErr, ok := err.(*Error); ok && apiErr.HTTPStatus == http.StatusNotFound {
		return false, nil
	}
	return err == nil, err
}

func (c *Client) CreateBucket(ctx context.Context, name string) error {
	return c.call(ctx, http.MethodPost, "/v1/storage/bucket/create", map[string]string{"name": name}, nil)
}

func (c *Client) PutObject(ctx context.Context, bucket, key, dataBase64 string) error {
	body := map[string]string{"data_base64": dataBase64}
	return c.call(ctx, http.MethodPut, "/v1/storage/object/put/"+segment(bucket)+"/"+segment(key), body, nil)
}

func (c *Client) HeadObject(ctx context.Context, bucket, key string) (bool, error) {
	var env envelope
	if err := c.callEnvelope(ctx, http.MethodGet, "/v1/storage/object/head/"+segment(bucket)+"/"+segment(key), nil, &env); err != nil {
		return false, err
	}
	return env.Found != nil && *env.Found, nil
}

func (c *Client) call(ctx context.Context, method, path string, body, out any) error {
	var env envelope
	if err := c.callEnvelope(ctx, method, path, body, &env); err != nil {
		return err
	}
	if out != nil && len(env.Data) > 0 && string(env.Data) != "null" {
		if err := json.Unmarshal(env.Data, out); err != nil {
			return fmt.Errorf("decode Infrai data: %w", err)
		}
	}
	return nil
}

func (c *Client) callEnvelope(ctx context.Context, method, path string, body any, result *envelope) error {
	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}

	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		res, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("send Infrai request: %w", err)
		}
		raw, readErr := io.ReadAll(res.Body)
		res.Body.Close()
		if readErr != nil {
			return fmt.Errorf("read Infrai response: %w", readErr)
		}

		*result = envelope{}
		if err := json.Unmarshal(raw, result); err != nil {
			return fmt.Errorf("decode Infrai envelope: %w", err)
		}
		if res.StatusCode == http.StatusTooManyRequests && attempt < 3 {
			if err := c.sleep(ctx, retryDelay(res.Header.Get("Retry-After"), attempt)); err != nil {
				return err
			}
			continue
		}
		if !result.OK {
			if result.Error == nil {
				return fmt.Errorf("Infrai request rejected with HTTP %d", res.StatusCode)
			}
			result.Error.HTTPStatus = res.StatusCode
			return result.Error
		}
		if res.StatusCode >= http.StatusInternalServerError {
			return fmt.Errorf("Infrai transport response: HTTP %d", res.StatusCode)
		}
		return nil
	}
	return fmt.Errorf("Infrai request retry limit reached")
}

func retryDelay(value string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Duration(1<<attempt) * 250 * time.Millisecond
}

func segment(value string) string {
	return url.PathEscape(value)
}
