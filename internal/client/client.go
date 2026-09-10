package client

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

// Client is a thin HTTP wrapper: 429/5xx backoff, verbose dumps with API keys
// redacted, and response headers exposed (the Files API hands back the upload
// URL in one). Timeouts come from the context, not the client, because a large
// upload legitimately runs for minutes.
type Client struct {
	http      *http.Client
	Verbose   bool
	RetryWait time.Duration
}

// Response is a completed HTTP exchange.
type Response struct {
	Status int
	Header http.Header
	Body   []byte
}

// New returns a Client with no client-side timeout (use ctx deadlines).
func New() *Client {
	return &Client{http: &http.Client{}, RetryWait: time.Second}
}

var (
	bearerRE = regexp.MustCompile(`(?i)(Authorization:\s*Bearer\s+)(\S+)`)
	apiKeyRE = regexp.MustCompile(`(?i)(x-goog-api-key:\s*)(\S+)`)
	skRE     = regexp.MustCompile(`\b(sk-[A-Za-z0-9_-]{8,})\b`)
	aizaRE   = regexp.MustCompile(`\b(AIza[A-Za-z0-9_-]{8,})\b`)
)

// Redact masks API keys in verbose output.
func Redact(s string) string {
	s = bearerRE.ReplaceAllString(s, `${1}***`)
	s = apiKeyRE.ReplaceAllString(s, `${1}***`)
	s = skRE.ReplaceAllString(s, "sk-***")
	s = aizaRE.ReplaceAllString(s, "AIza***")
	return s
}

// Do sends an in-memory body.
func (c *Client) Do(ctx context.Context, method, url string, body []byte, headers map[string]string) (*Response, error) {
	var fn func() (io.ReadCloser, int64, error)
	if body != nil {
		fn = func() (io.ReadCloser, int64, error) {
			return io.NopCloser(bytes.NewReader(body)), int64(len(body)), nil
		}
	}
	return c.do(ctx, method, url, fn, headers, body != nil, false)
}

// DoBody streams a body built fresh for each attempt (so retries can rewind a file).
func (c *Client) DoBody(ctx context.Context, method, url string, bodyFn func() (io.ReadCloser, int64, error), headers map[string]string) (*Response, error) {
	return c.do(ctx, method, url, bodyFn, headers, false, true)
}

func (c *Client) do(ctx context.Context, method, url string, bodyFn func() (io.ReadCloser, int64, error), headers map[string]string, dumpBody, opaqueBody bool) (*Response, error) {
	const maxRetries = 3
	wait := c.RetryWait
	if wait <= 0 {
		wait = time.Second
	}

	for attempt := 0; ; attempt++ {
		var (
			rc     io.ReadCloser
			length int64
			err    error
		)
		if bodyFn != nil {
			rc, length, err = bodyFn()
			if err != nil {
				return nil, err
			}
		}
		req, err := http.NewRequestWithContext(ctx, method, url, rc)
		if err != nil {
			return nil, err
		}
		if rc != nil {
			req.ContentLength = length
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}

		if c.Verbose {
			fmt.Fprintf(os.Stderr, "→ %s %s\n", method, url)
			for k, v := range headers {
				fmt.Fprintf(os.Stderr, "  %s: %s\n", k, Redact(v))
			}
			if dumpBody && bodyFn != nil {
				b, _, _ := bodyFn()
				raw, _ := io.ReadAll(b)
				fmt.Fprintf(os.Stderr, "  %s\n", Redact(string(raw)))
			} else if opaqueBody {
				fmt.Fprintf(os.Stderr, "  (%d bytes of payload)\n", length)
			}
		}

		resp, err := c.http.Do(req)
		if err != nil {
			return nil, err
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if c.Verbose {
			fmt.Fprintf(os.Stderr, "← HTTP %d\n%s\n", resp.StatusCode, Redact(truncate(string(b), 4000)))
		}

		out := &Response{Status: resp.StatusCode, Header: resp.Header, Body: b}
		retryable := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		if retryable && attempt < maxRetries {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
			}
			wait *= 2
			continue
		}
		if resp.StatusCode >= 400 {
			return out, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(truncate(string(b), 2000)))
		}
		return out, nil
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
