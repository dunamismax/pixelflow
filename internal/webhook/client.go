package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	HeaderSignature = "X-Pixelflow-Signature"
	HeaderTimestamp = "X-Pixelflow-Timestamp"
	HeaderEvent     = "X-Pixelflow-Event"
)

type Config struct {
	SigningSecret  string
	Timeout        time.Duration
	MaxAttempts    int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
}

type Client struct {
	httpClient     *http.Client
	signingSecret  string
	maxAttempts    int
	initialBackoff time.Duration
	maxBackoff     time.Duration
}

func NewClient(cfg Config) *Client {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	maxAttempts := cfg.MaxAttempts
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	initialBackoff := cfg.InitialBackoff
	if initialBackoff <= 0 {
		initialBackoff = 1 * time.Second
	}

	maxBackoff := cfg.MaxBackoff
	if maxBackoff < initialBackoff {
		maxBackoff = initialBackoff
	}

	return &Client{
		httpClient: &http.Client{
			Timeout: timeout,
		},
		signingSecret:  cfg.SigningSecret,
		maxAttempts:    maxAttempts,
		initialBackoff: initialBackoff,
		maxBackoff:     maxBackoff,
	}
}

func (c *Client) Send(ctx context.Context, endpoint, event string, payload any) error {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return nil
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal webhook payload: %w", err)
	}

	timestamp := strconv.FormatInt(time.Now().UTC().Unix(), 10)
	signature := c.sign(timestamp, body)

	backoff := c.initialBackoff
	var lastErr error
	for attempt := 1; attempt <= c.maxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("build webhook request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(HeaderTimestamp, timestamp)
		req.Header.Set(HeaderSignature, signature)
		req.Header.Set(HeaderEvent, event)

		resp, err := c.httpClient.Do(req)
		if err == nil && resp != nil {
			resp.Body.Close()
		}

		if err == nil && resp != nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}

		lastErr = classifyWebhookError(err, resp)
		if attempt == c.maxAttempts {
			break
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}

		backoff = minDuration(backoff*2, c.maxBackoff)
	}

	return fmt.Errorf("webhook delivery failed after %d attempts: %w", c.maxAttempts, lastErr)
}

func (c *Client) sign(timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(c.signingSecret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func classifyWebhookError(err error, resp *http.Response) error {
	if err != nil {
		return err
	}
	if resp == nil {
		return fmt.Errorf("webhook request failed: no response")
	}
	return fmt.Errorf("webhook returned status=%d", resp.StatusCode)
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
