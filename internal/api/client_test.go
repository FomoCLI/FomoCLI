package api_test

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/FomoCLI/FomoCLI/internal/api"
	"github.com/FomoCLI/FomoCLI/internal/cache"
	"github.com/FomoCLI/FomoCLI/internal/mock"
)

func newClient(t *testing.T, key string) (*api.Client, *mock.Server) {
	t.Helper()
	srv := mock.Start()
	t.Cleanup(srv.Close)
	c := api.New(srv.URL, key)
	c.Cache = &cache.Store{Dir: t.TempDir()}
	c.Retries = 0
	return c, srv
}

func TestLeaderboardAndCredits(t *testing.T) {
	c, _ := newClient(t, "k")
	res, err := c.Leaderboard(context.Background(), "24h", 5)
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Data.Get("traders").Index(0).Get("handle").Str(); got != "CryptoKaleo" {
		t.Errorf("handle = %q", got)
	}
	if res.CreditsCost != 250 || res.CreditsRemaining != 249750 {
		t.Errorf("credits = %d / %d", res.CreditsCost, res.CreditsRemaining)
	}
	if lc := c.LastCredits(); lc == nil || lc.Remaining != 249750 {
		t.Errorf("last credits not persisted: %+v", lc)
	}
}

func TestErrors(t *testing.T) {
	c, _ := newClient(t, "")
	_, err := c.Leaderboard(context.Background(), "24h", 0)
	var ae *api.APIError
	if !errors.As(err, &ae) || ae.Status != 401 || ae.Hint() == "" {
		t.Fatalf("want 401 with hint, got %v", err)
	}

	c.Key = "broke"
	_, err = c.User(context.Background(), "x")
	if !errors.As(err, &ae) || ae.Status != 402 || ae.UpgradeURL == "" {
		t.Fatalf("want 402, got %v", err)
	}

	c.Key = "k"
	_, err = c.User(context.Background(), "nobody")
	if !errors.As(err, &ae) || ae.Status != 404 {
		t.Fatalf("want 404, got %v", err)
	}
}

func TestCacheModes(t *testing.T) {
	c, srv := newClient(t, "k")
	ctx := context.Background()

	// Offline with an empty cache is a clean miss.
	c.Mode = api.ModeOffline
	if _, err := c.Status(ctx); !errors.Is(err, api.ErrNotCached) {
		t.Fatalf("want ErrNotCached, got %v", err)
	}

	// Auto: first call hits the network, second is served from cache.
	c.Mode = api.ModeAuto
	c.TTL = time.Minute
	if _, err := c.Status(ctx); err != nil {
		t.Fatal(err)
	}
	before := srv.Calls.Load()
	res, err := c.Status(ctx)
	if err != nil || !res.FromCache || srv.Calls.Load() != before {
		t.Fatalf("expected cache hit, fromCache=%v calls %d→%d err=%v", res.FromCache, before, srv.Calls.Load(), err)
	}

	// Fresh bypasses the cache.
	c.Mode = api.ModeFresh
	if res, _ := c.Status(ctx); res.FromCache {
		t.Fatal("fresh mode served from cache")
	}

	// Network down: fall back to the cached copy.
	srv.Close()
	res, err = c.Status(ctx)
	if err != nil || !res.Fallback {
		t.Fatalf("expected fallback, got %v %+v", err, res)
	}

	// Offline: served from disk.
	c.Mode = api.ModeOffline
	if res, err := c.Status(ctx); err != nil || !res.FromCache {
		t.Fatalf("offline read failed: %v", err)
	}

	// Network down and nothing cached: a network error.
	c.Mode = api.ModeFresh
	_, err = c.Leaderboard(ctx, "7d", 0)
	var ne *api.ErrNetwork
	if !errors.As(err, &ne) {
		t.Fatalf("want ErrNetwork, got %v", err)
	}
}

func TestRequestPath(t *testing.T) {
	q := url.Values{"b": {"2"}, "a": {"1"}, "empty": {""}}
	if got := api.RequestPath("v2/x", q); got != "/v2/x?a=1&b=2" {
		t.Errorf("RequestPath = %q", got)
	}
}

func TestWSURL(t *testing.T) {
	c := api.New("https://api.fomoapi.io", "KEY")
	got := c.WSURL("/ws/alerts", url.Values{"chain": {"solana"}})
	if got != "wss://api.fomoapi.io/ws/alerts?chain=solana&key=KEY" {
		t.Errorf("WSURL = %q", got)
	}
}

func TestStream(t *testing.T) {
	c, srv := newClient(t, "k")
	srv.StreamInterval = 20 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var welcome bool
	alerts := 0
	err := c.Stream(ctx, "/ws/alerts", url.Values{"chain": {"robinhood"}}, func(ev api.StreamEvent) {
		if ev.Msg.IsNull() {
			return
		}
		switch ev.Msg.Get("type").Str() {
		case "welcome":
			welcome = ev.Msg.Get("realtime").Bool()
		case "alert":
			if ev.Msg.Get("chain").Str() != "robinhood" {
				t.Errorf("filter leaked chain %q", ev.Msg.Get("chain").Str())
			}
			if alerts++; alerts == 5 {
				cancel()
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if !welcome || alerts < 5 {
		t.Fatalf("welcome=%v alerts=%d", welcome, alerts)
	}
}

func TestStreamPolicyViolationIsFatal(t *testing.T) {
	c, _ := newClient(t, "starter")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := c.Stream(ctx, "/ws/trades", nil, func(api.StreamEvent) {})
	var ae *api.APIError
	if !errors.As(err, &ae) || ae.Status != 403 {
		t.Fatalf("want fatal 403, got %v", err)
	}
}
