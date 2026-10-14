// Package httpx is the shared JSON-over-HTTP layer for UniFi APIs: API key
// auth, a per-source concurrency limiter, and retries on rate limiting.
package httpx

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

const (
	maxRetries    = 3
	maxRetryDelay = 30 * time.Second
	maxBodyBytes  = 32 << 20
)

// Limiter bounds concurrent requests. Share one across all clients that use
// the same API key.
type Limiter chan struct{}

// NewLimiter returns a Limiter allowing n concurrent requests.
func NewLimiter(n int) Limiter { return make(Limiter, n) }

// Requester performs authenticated JSON GET requests.
type Requester struct {
	http    *http.Client
	apiKey  string
	limiter Limiter
}

// Options configures a Requester.
type Options struct {
	APIKey   string
	Insecure bool // skip TLS verification (self-signed console certificates)
	Timeout  time.Duration
	Limiter  Limiter // optional
}

// New creates a Requester.
func New(o Options) *Requester {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	if o.Insecure {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // opt-in per source
	}
	return &Requester{
		http:    &http.Client{Transport: tr, Timeout: o.Timeout},
		apiKey:  o.APIKey,
		limiter: o.Limiter,
	}
}

// APIError is a non-2xx response. It understands both the Network
// Integration API ({statusCode,statusName,message}) and Site Manager API
// ({code,httpStatusCode,message}) error bodies.
type APIError struct {
	StatusCode int
	StatusName string `json:"statusName"`
	Code       string `json:"code"`
	Message    string `json:"message"`
}

func (e *APIError) Error() string {
	name := e.StatusName
	if name == "" {
		name = e.Code
	}
	if name == "" {
		name = http.StatusText(e.StatusCode)
	}
	if e.Message != "" {
		return fmt.Sprintf("%d %s: %s", e.StatusCode, name, e.Message)
	}
	return fmt.Sprintf("%d %s", e.StatusCode, name)
}

// GetJSON fetches url and decodes the JSON response into out, retrying on
// 429 Too Many Requests.
func (r *Requester) GetJSON(ctx context.Context, url string, out any) error {
	for attempt := 0; ; attempt++ {
		status, header, body, err := r.do(ctx, url)
		if err != nil {
			return err
		}
		if status == http.StatusTooManyRequests && attempt < maxRetries {
			if err := sleep(ctx, retryDelay(header, attempt)); err != nil {
				return err
			}
			continue
		}
		if status < 200 || status > 299 {
			apiErr := &APIError{}
			_ = json.Unmarshal(body, apiErr)
			apiErr.StatusCode = status
			return apiErr
		}
		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
		return nil
	}
}

func (r *Requester) do(ctx context.Context, url string) (int, http.Header, []byte, error) {
	if r.limiter != nil {
		select {
		case r.limiter <- struct{}{}:
			defer func() { <-r.limiter }()
		case <-ctx.Done():
			return 0, nil, nil, ctx.Err()
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, nil, nil, err
	}
	req.Header.Set("X-API-KEY", r.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := r.http.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return 0, nil, nil, err
	}
	return resp.StatusCode, resp.Header, body, nil
}

func retryDelay(h http.Header, attempt int) time.Duration {
	if s, err := strconv.Atoi(h.Get("Retry-After")); err == nil && s >= 0 {
		return min(time.Duration(s)*time.Second, maxRetryDelay)
	}
	return min(time.Second<<attempt, maxRetryDelay)
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
