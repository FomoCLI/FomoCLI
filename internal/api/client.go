// Package api is a small, dependency-free client for the FOMO API
// (https://fomoapi.io/docs), with credit tracking and an offline cache.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/FomoCLI/FomoCLI/internal/cache"
	"github.com/FomoCLI/FomoCLI/internal/jx"
)

// Mode controls how the client uses the offline cache.
type Mode int

const (
	// ModeAuto reuses a cached response younger than the TTL, otherwise hits
	// the network and falls back to the cache if the network is unreachable.
	ModeAuto Mode = iota
	// ModeFresh always hits the network (still falls back when offline).
	ModeFresh
	// ModeOffline never touches the network: cache only.
	ModeOffline
)

// Client talks to the FOMO API.
type Client struct {
	BaseURL   string
	Key       string
	HTTP      *http.Client
	Cache     *cache.Store // nil disables caching
	TTL       time.Duration
	Mode      Mode
	UserAgent string
	Retries   int
}

// Response is a decoded API response plus metadata.
type Response struct {
	Path             string
	Data             jx.V
	Raw              []byte
	FromCache        bool
	Fallback         bool // served from cache because the network failed
	FetchedAt        time.Time
	CreditsCost      int64 // -1 when unknown
	CreditsRemaining int64 // -1 when unknown
	Deprecated       bool
}

// APIError is a non-2xx answer from the API.
type APIError struct {
	Status     int
	Code       string
	Message    string
	UpgradeURL string
	Retryable  bool
}

func (e *APIError) Error() string {
	msg := e.Message
	if msg == "" {
		msg = e.Code
	}
	if msg == "" {
		msg = http.StatusText(e.Status)
	}
	return fmt.Sprintf("%d %s", e.Status, msg)
}

// Hint returns a human-friendly next step for common errors.
func (e *APIError) Hint() string {
	switch e.Status {
	case 401:
		return "Set your key with `fomocli auth login` (free key: https://fomoapi.io/dashboard)."
	case 402:
		u := e.UpgradeURL
		if u == "" {
			u = "https://fomoapi.io/pricing"
		}
		return "Out of credits. Top up or upgrade at " + u + " - or use --offline to browse cached data."
	case 404:
		return "Not found - check the handle, id or address. Try `fomocli search <name>`."
	case 429:
		return "Rate limited on a keyless endpoint. Wait a minute or add an API key."
	case 503:
		return "FOMO's live read timed out. Retrying in a few seconds usually works. See `fomocli status`."
	}
	return ""
}

// ErrNotCached is returned in offline mode when nothing is cached for a request.
var ErrNotCached = errors.New("not available offline: this request has never been cached")

// ErrNetwork wraps transport failures with no cached fallback.
type ErrNetwork struct{ Err error }

func (e *ErrNetwork) Error() string { return "network unreachable: " + e.Err.Error() }
func (e *ErrNetwork) Unwrap() error { return e.Err }

// New returns a client with sensible defaults.
func New(baseURL, key string) *Client {
	return &Client{
		BaseURL:   strings.TrimRight(baseURL, "/"),
		Key:       key,
		HTTP:      &http.Client{Timeout: 30 * time.Second},
		TTL:       60 * time.Second,
		UserAgent: "fomocli",
		Retries:   1,
	}
}

// RequestPath builds the canonical path+query used for requests and cache keys.
func RequestPath(path string, q url.Values) string {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if len(q) == 0 {
		return path
	}
	// Drop empty values so "--chain ''" does not change the cache key.
	clean := url.Values{}
	for k, vs := range q {
		for _, v := range vs {
			if v != "" {
				clean.Add(k, v)
			}
		}
	}
	if len(clean) == 0 {
		return path
	}
	return path + "?" + clean.Encode()
}

// Get performs a GET request, honoring the cache mode.
func (c *Client) Get(ctx context.Context, path string, q url.Values) (*Response, error) {
	rp := RequestPath(path, q)

	var cached *cache.Entry
	if c.Cache != nil {
		cached, _ = c.Cache.Get(rp)
	}

	switch {
	case c.Mode == ModeOffline:
		if cached == nil {
			return nil, ErrNotCached
		}
		return fromEntry(rp, cached, false)
	case c.Mode == ModeAuto && cached != nil && c.TTL > 0 && cached.Age() < c.TTL:
		return fromEntry(rp, cached, false)
	}

	resp, err := c.fetch(ctx, rp)
	if err != nil {
		var ne *ErrNetwork
		if errors.As(err, &ne) && cached != nil {
			return fromEntry(rp, cached, true)
		}
		return nil, err
	}
	if c.Cache != nil {
		_ = c.Cache.Put(rp, resp.Raw)
		if resp.CreditsRemaining >= 0 {
			c.saveCredits(resp.CreditsRemaining)
		}
	}
	return resp, nil
}

func fromEntry(rp string, e *cache.Entry, fallback bool) (*Response, error) {
	v, err := jx.Parse(e.Body)
	if err != nil {
		return nil, fmt.Errorf("corrupt cache entry for %s: %w", rp, err)
	}
	return &Response{
		Path: rp, Data: v, Raw: e.Body, FromCache: true, Fallback: fallback,
		FetchedAt: e.FetchedAt, CreditsCost: -1, CreditsRemaining: -1,
	}, nil
}

func (c *Client) fetch(ctx context.Context, rp string) (*Response, error) {
	var lastErr error
	for attempt := 0; attempt <= c.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * 1500 * time.Millisecond):
			}
		}
		resp, err := c.do(ctx, rp)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		var ae *APIError
		if errors.As(err, &ae) && !(ae.Status >= 500 || ae.Retryable) {
			return nil, err // 4xx are final
		}
		if errors.Is(err, context.Canceled) {
			return nil, err
		}
	}
	return nil, lastErr
}

func (c *Client) do(ctx context.Context, rp string) (*Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+rp, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("accept", "application/json")
	req.Header.Set("user-agent", c.UserAgent)
	if c.Key != "" {
		req.Header.Set("authorization", "Bearer "+c.Key)
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, &ErrNetwork{Err: simplifyNetErr(err)}
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 64<<20))
	if err != nil {
		return nil, &ErrNetwork{Err: err}
	}

	if res.StatusCode < 200 || res.StatusCode > 299 {
		return nil, parseError(res.StatusCode, body)
	}
	v, err := jx.Parse(body)
	if err != nil {
		return nil, fmt.Errorf("invalid JSON from %s: %w", rp, err)
	}
	return &Response{
		Path: rp, Data: v, Raw: body, FetchedAt: time.Now(),
		CreditsCost:      headerInt(res.Header, "x-credits-cost"),
		CreditsRemaining: headerInt(res.Header, "x-credits-remaining"),
		Deprecated:       res.Header.Get("deprecation") != "",
	}, nil
}

func parseError(status int, body []byte) error {
	e := &APIError{Status: status}
	var m map[string]any
	if json.Unmarshal(body, &m) == nil {
		e.Code, _ = m["error"].(string)
		e.Message, _ = m["message"].(string)
		e.UpgradeURL, _ = m["upgradeUrl"].(string)
		e.Retryable, _ = m["retryable"].(bool)
		if e.Message == "" {
			e.Message = e.Code
		}
	} else if s := strings.TrimSpace(string(body)); s != "" && len(s) < 200 {
		e.Message = s
	}
	return e
}

func headerInt(h http.Header, k string) int64 {
	s := strings.ReplaceAll(h.Get(k), ",", "")
	if s == "" {
		return -1
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		f, ferr := strconv.ParseFloat(s, 64)
		if ferr != nil {
			return -1
		}
		return int64(f)
	}
	return n
}

func simplifyNetErr(err error) error {
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return fmt.Errorf("cannot resolve %s (are you offline?)", dnsErr.Name)
	}
	var ue *url.Error
	if errors.As(err, &ue) && ue.Timeout() {
		return errors.New("request timed out")
	}
	return err
}

// WSURL builds a WebSocket URL for a streaming endpoint.
func (c *Client) WSURL(path string, q url.Values) string {
	base := c.BaseURL
	switch {
	case strings.HasPrefix(base, "https://"):
		base = "wss://" + strings.TrimPrefix(base, "https://")
	case strings.HasPrefix(base, "http://"):
		base = "ws://" + strings.TrimPrefix(base, "http://")
	}
	if q == nil {
		q = url.Values{}
	}
	if c.Key != "" {
		q.Set("key", c.Key)
	}
	return base + RequestPath(path, q)
}

// CreditsState is the last-known credit balance, persisted for offline viewing.
type CreditsState struct {
	Remaining int64     `json:"remaining"`
	At        time.Time `json:"at"`
}

func (c *Client) creditsFile() string { return filepath.Join(c.Cache.Dir, "credits.state") }

func (c *Client) saveCredits(n int64) {
	b, _ := json.Marshal(CreditsState{Remaining: n, At: time.Now().UTC()})
	_ = os.WriteFile(c.creditsFile(), b, 0o600)
}

// LastCredits returns the last credit balance seen in a response header.
func (c *Client) LastCredits() *CreditsState {
	if c.Cache == nil {
		return nil
	}
	b, err := os.ReadFile(c.creditsFile())
	if err != nil {
		return nil
	}
	var s CreditsState
	if json.Unmarshal(b, &s) != nil {
		return nil
	}
	return &s
}
