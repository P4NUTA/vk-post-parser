package vkapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"
)

const vkAPIBaseURL = "https://api.vk.com/method/"

// Client is a VK API HTTP client with rate limiting and retry logic.
type Client struct {
	token      string
	version    string
	httpClient *http.Client
	limiter    *rateLimiter
	logger     *slog.Logger
}

// NewClient creates a new VK API client.
func NewClient(token, version string, logger *slog.Logger) *Client {
	return &Client{
		token:   token,
		version: version,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		limiter: newRateLimiter(3, time.Second),
		logger:  logger,
	}
}

// request makes a single API request without retry.
func (c *Client) request(ctx context.Context, method string, params url.Values) (json.RawMessage, error) {
	c.limiter.Wait(ctx)

	params.Set("access_token", c.token)
	params.Set("v", c.version)

	reqURL := vkAPIBaseURL + method

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.URL.RawQuery = params.Encode()

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var apiResp APIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if apiResp.Error != nil {
		return nil, apiResp.Error
	}

	return apiResp.Response, nil
}

// RequestWithRetry makes an API request with exponential backoff retry for retryable errors.
func (c *Client) RequestWithRetry(ctx context.Context, method string, params url.Values) (json.RawMessage, error) {
	const maxRetries = 5
	backoff := 500 * time.Millisecond

	for attempt := 0; attempt <= maxRetries; attempt++ {
		raw, err := c.request(ctx, method, params)
		if err == nil {
			return raw, nil
		}

		var apiErr *APIError
		if errors.As(err, &apiErr) {
			if isRetryable(apiErr.ErrorCode) {
				c.logger.Warn("retryable VK API error",
					"code", apiErr.ErrorCode,
					"msg", apiErr.ErrorMsg,
					"attempt", attempt+1,
					"backoff", backoff,
				)

				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(backoff):
				}
				backoff *= 2
				if backoff > 30*time.Second {
					backoff = 30 * time.Second
				}
				continue
			}

			return nil, fmt.Errorf("VK API error (code %d): %w", apiErr.ErrorCode, err)
		}

		return nil, err
	}

	return nil, fmt.Errorf("max retries (%d) exceeded", maxRetries)
}

func isRetryable(code int) bool {
	return code == 6 || code == 9 || code == 10 || code == 29
}

func isAPIErrorCode(err error, code int) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.ErrorCode == code
	}
	return false
}

// rateLimiter is a simple token-bucket rate limiter.
type rateLimiter struct {
	mu        sync.Mutex
	tokens    int
	maxTokens int
	interval  time.Duration
	lastFill  time.Time
}

func newRateLimiter(maxTokens int, interval time.Duration) *rateLimiter {
	return &rateLimiter{
		tokens:    maxTokens,
		maxTokens: maxTokens,
		interval:  interval,
		lastFill:  time.Now(),
	}
}

func (rl *rateLimiter) Wait(ctx context.Context) {
	for {
		rl.mu.Lock()
		now := time.Now()
		elapsed := now.Sub(rl.lastFill)
		if elapsed >= rl.interval {
			rl.tokens = rl.maxTokens
			rl.lastFill = now
		}
		if rl.tokens > 0 {
			rl.tokens--
			rl.mu.Unlock()
			return
		}
		waitTime := rl.interval - elapsed
		rl.mu.Unlock()

		select {
		case <-ctx.Done():
			return
		case <-time.After(waitTime):
		}
	}
}
