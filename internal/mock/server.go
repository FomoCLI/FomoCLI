// Package mock is an in-process stand-in for FOMO API, serving sample payloads
// shaped like the examples in https://fomoapi.io/docs. It backs the test suite
// and `fomocli --demo`, so the CLI can be tried without a key or a connection.
package mock

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

//go:embed fixtures/*.json
var fixtures embed.FS

// Server is a running mock API.
type Server struct {
	*httptest.Server
	Calls   atomic.Int64
	credits atomic.Int64
	// StreamInterval controls how often /ws/alerts emits a synthetic alert.
	StreamInterval time.Duration
}

// Start launches the mock on a random local port.
func Start() *Server {
	s := &Server{StreamInterval: 700 * time.Millisecond}
	s.credits.Store(250000)
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	return s
}

var capturedRe = regexp.MustCompile(`"(capturedAt|updatedAt)":"[^"]*"`)

// fixture loads a sample payload, re-stamping snapshot times so demo data
// always looks a couple of minutes old.
func fixture(name string) []byte {
	b, err := fixtures.ReadFile("fixtures/" + name + ".json")
	if err != nil {
		panic(err)
	}
	now := time.Now().UTC().Add(-2 * time.Minute).Format(time.RFC3339)
	return capturedRe.ReplaceAll(b, []byte(`"$1":"`+now+`"`))
}

type route struct {
	match func(parts []string) bool
	file  string
	cost  int64
}

func seg(parts []string, n int, vals ...string) bool {
	if len(parts) != n {
		return false
	}
	for i, v := range vals {
		if v != "*" && parts[i] != v {
			return false
		}
	}
	return true
}

var routes = []route{
	{func(p []string) bool { return seg(p, 4, "v2", "leaderboard", "tokens", "*") }, "tokens", 250},
	{func(p []string) bool { return seg(p, 3, "v2", "leaderboard", "*") }, "leaderboard", 250},
	{func(p []string) bool { return seg(p, 4, "v2", "users", "id", "*") }, "user", 2500},
	{func(p []string) bool { return seg(p, 3, "v2", "users", "*") }, "user", 2500},
	{func(p []string) bool {
		return seg(p, 4, "v2", "users", "*", "positions") || seg(p, 4, "v2", "users", "*", "trades")
	}, "positions", 250},
	{func(p []string) bool { return seg(p, 4, "v2", "users", "*", "swaps") }, "alerts", 250},
	{func(p []string) bool { return seg(p, 4, "v2", "users", "*", "balances") }, "balances", 250},
	{func(p []string) bool { return seg(p, 4, "v2", "users", "*", "following") }, "following", 250},
	{func(p []string) bool { return seg(p, 4, "v2", "users", "*", "followers") }, "following", 250},
	{func(p []string) bool { return seg(p, 4, "v2", "users", "*", "spotlight") }, "spotlight", 250},
	{func(p []string) bool { return seg(p, 3, "v2", "trades", "*") }, "trade", 250},
	{func(p []string) bool { return seg(p, 4, "v2", "trades", "*", "comments") }, "comments", 250},
	{func(p []string) bool { return seg(p, 4, "v2", "token", "*", "stats") }, "stats", 250},
	{func(p []string) bool { return seg(p, 4, "v2", "token", "*", "devs") }, "devs", 250},
	{func(p []string) bool { return seg(p, 3, "token", "*", "holders") }, "holders", 250},
	{func(p []string) bool { return seg(p, 3, "v2", "tokens", "activity") }, "activity", 250},
	{func(p []string) bool {
		return seg(p, 2, "v2", "thesis") || (len(p) >= 3 && p[0] == "v2" && p[1] == "thesis")
	}, "theses", 1250},
	{func(p []string) bool { return seg(p, 2, "v2", "search") || seg(p, 3, "v2", "tokens", "search") }, "search", 250},
	{func(p []string) bool { return seg(p, 2, "v2", "alerts") }, "alerts", 125},
	{func(p []string) bool { return seg(p, 2, "v2", "me") }, "me", 0},
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	s.Calls.Add(1)
	path := strings.Trim(r.URL.Path, "/")
	parts := strings.Split(path, "/")

	switch path {
	case "status", "v2/status":
		writeJSON(w, 200, fixture("status"))
		return
	case "health":
		writeJSON(w, 200, []byte(`{"ok":true,"traders":6,"uptime":1834.2}`))
		return
	case "v1":
		writeJSON(w, 200, []byte(`{"name":"FOMO API (mock)","version":"mock","tier":"free"}`))
		return
	case "ws/alerts", "ws/trades":
		s.stream(w, r, path == "ws/trades")
		return
	}

	key := keyOf(r)
	if key == "" {
		writeJSON(w, 401, []byte(`{"error":"API key required","message":"This endpoint requires an API key."}`))
		return
	}
	if key == "broke" {
		writeJSON(w, 402, []byte(`{"error":"credits_exhausted","plan":"free","remaining":0,"message":"Out of credits.","upgradeUrl":"https://fomoapi.io/pricing"}`))
		return
	}

	for _, rt := range routes {
		if rt.file == "" || !rt.match(parts) {
			continue
		}
		if len(parts) >= 2 && parts[0] == "v2" && parts[1] == "users" && strings.EqualFold(parts[len(parts)-1], "nobody") {
			break
		}
		body := fixture(rt.file)
		if rt.file == "positions" && r.URL.Query().Get("cursor") == "c2" {
			body = fixture("positions2")
		}
		left := s.credits.Add(-rt.cost)
		w.Header().Set("x-credits-cost", fmt.Sprint(rt.cost))
		w.Header().Set("x-credits-remaining", fmt.Sprint(left))
		writeJSON(w, 200, body)
		return
	}
	writeJSON(w, 404, []byte(`{"error":"trader not found"}`))
}

func keyOf(r *http.Request) string {
	if a := r.Header.Get("authorization"); a != "" {
		return strings.TrimSpace(strings.TrimPrefix(a, "Bearer "))
	}
	if k := r.Header.Get("x-api-key"); k != "" {
		return k
	}
	return r.URL.Query().Get("key")
}

func writeJSON(w http.ResponseWriter, status int, b []byte) {
	w.Header().Set("content-type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(b)
}

// stream emits a welcome, a short replay and then synthetic live alerts.
func (s *Server) stream(w http.ResponseWriter, r *http.Request, onchain bool) {
	q := r.URL.Query()
	if onchain && q.Get("key") != "growth" {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		c.Close(websocket.StatusPolicyViolation, "on-chain stream requires a Growth or Scale key")
		return
	}
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer c.CloseNow()
	ctx := r.Context()
	realtime := q.Get("key") != ""
	_ = writeWS(ctx, c, map[string]any{"type": "welcome", "realtime": realtime, "delaySeconds": map[bool]int{true: 0, false: 60}[realtime]})

	var base struct {
		Alerts []map[string]any `json:"alerts"`
	}
	_ = json.Unmarshal(fixture("alerts"), &base)
	traders := []string{"frankdegods", "CryptoKaleo", "starcatcher444", "theveeman", "laifu", "ansem"}
	tokens := [][3]any{{"PONS", "0x39dbed3a77c1b2e0f11e6c0b8c7b9e0a1f2d3c4b", "robinhood"}, {"CACHE", "0x51fb76aa00bb11cc22dd33ee44ff556677889900", "robinhood"},
		{"WIF", "EKpQGSJtjMFqKZ9KQanSqYXRcF8fBopzLHYxdM65zcjm", "solana"}, {"FARTCOIN", "9BB6NFEcjBCtnNLFko2FqVQBq8HHM13kCyYcdQbgpump", "solana"}, {"DEGEN", "0x4ed4e862860bed51a9570b96d89af5e1b0efefed", "base"}}

	matches := func(a map[string]any) bool {
		if t := q.Get("trader"); t != "" && !strings.EqualFold(fmt.Sprint(a["trader"]), t) {
			return false
		}
		if ch := q.Get("chain"); ch != "" && !strings.EqualFold(fmt.Sprint(a["chain"]), ch) {
			return false
		}
		return true
	}
	for _, a := range base.Alerts {
		a["ts"] = time.Now().Add(-time.Minute).UnixMilli()
		if matches(a) {
			_ = writeWS(ctx, c, a)
		}
	}
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	t := time.NewTicker(s.StreamInterval)
	defer t.Stop()
	for i := 0; ; i++ {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		tok := tokens[rng.Intn(len(tokens))]
		side := []string{"buy", "buy", "sell"}[rng.Intn(3)]
		tr := traders[rng.Intn(len(traders))]
		usd := float64(int(rng.ExpFloat64()*6000) + 3000)
		a := map[string]any{"type": "alert", "alertType": side, "source": "feed", "eventId": fmt.Sprintf("mock-%d", i),
			"trader": tr, "token": tok[0], "tokenAddress": tok[1], "chain": tok[2], "usdValue": usd,
			"text": fmt.Sprintf("%s %s $%s", tr, map[string]string{"buy": "bought", "sell": "sold"}[side], tok[0]), "ts": time.Now().UnixMilli()}
		if !matches(a) {
			continue
		}
		if err := writeWS(ctx, c, a); err != nil {
			return
		}
	}
}

func writeWS(ctx context.Context, c *websocket.Conn, v any) error {
	b, _ := json.Marshal(v)
	return c.Write(ctx, websocket.MessageText, b)
}
