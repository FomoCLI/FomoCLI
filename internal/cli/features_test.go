package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FomoCLI/FomoCLI/internal/ui"
)

func TestScanCommand(t *testing.T) {
	srv := setupEnv(t, "test-key")
	out, e, code := run(t, srv, "scan", "0x39dbed3a77c1b2e0f11e6c0b8c7b9e0a1f2d3c4b")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, e)
	}
	for _, w := range []string{"RUG SCAN", "RISK", "LOW RISK", "SMART MONEY", "3 tracked traders"} {
		if !strings.Contains(out, w) {
			t.Errorf("scan output missing %q:\n%s", w, out)
		}
	}
	if !strings.Contains(e, "750 credits") {
		t.Errorf("scan should report the 3 calls' cost: %q", e)
	}
	out, _, _ = run(t, srv, "scan", "0x39db", "--json")
	var r map[string]any
	if err := json.Unmarshal([]byte(out), &r); err != nil || r["verdict"] == nil {
		t.Fatalf("scan --json: %v\n%s", err, out)
	}
	if _, _, code := run(t, srv, "scan", "0xuncached", "--key", "broke"); code != ExitCredits {
		t.Errorf("out of credits should stop the scan: %d", code)
	}
}

func TestCardCommand(t *testing.T) {
	srv := setupEnv(t, "test-key")
	file := filepath.Join(t.TempDir(), "kaleo.png")
	out, e, code := run(t, srv, "card", "@CryptoKaleo", "-O", file)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, e)
	}
	if !strings.Contains(out, "CARD") || !strings.Contains(out, "+$151.4K") {
		t.Errorf("card output: %s", out)
	}
	f, err := os.Open(file)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 1200 || b.Dy() != 675 {
		t.Errorf("card size %v", b)
	}
	if _, _, code := run(t, srv, "card", "x", "--window", "1y"); code != ExitUsage {
		t.Errorf("bad window accepted: %d", code)
	}
}

// fakeDiscord records webhook posts.
func fakeDiscord(t *testing.T) (*httptest.Server, func() []string) {
	var mu sync.Mutex
	var got []string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		got = append(got, string(b))
		mu.Unlock()
		w.WriteHeader(204)
	}))
	t.Cleanup(s.Close)
	return s, func() []string { mu.Lock(); defer mu.Unlock(); return append([]string(nil), got...) }
}

func TestRadarStreamsSwarmsAndNotifies(t *testing.T) {
	srv := setupEnv(t, "test-key")
	srv.StreamInterval = 15 * time.Millisecond
	hook, posts := fakeDiscord(t)
	t.Setenv("FOMO_DISCORD_WEBHOOK", hook.URL)

	var out, errb bytes.Buffer
	prevOut, prevErr := ui.Out, ui.Err
	ui.Out, ui.Err = &out, &errb
	root := NewRoot()
	root.SetArgs([]string{"--no-color", "--base-url", srv.URL, "radar", "--json", "--min-traders", "3", "--notify", "discord"})
	ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
	defer cancel()
	err := root.ExecuteContext(ctx)
	ui.Out, ui.Err = prevOut, prevErr
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	var sig struct {
		Symbol  string
		Side    string
		Traders []struct{ Trader string }
	}
	if err := json.Unmarshal([]byte(lines[0]), &sig); err != nil || len(sig.Traders) < 3 || sig.Side != "buy" {
		t.Fatalf("no swarm signal: %v\n%s\n%s", err, out.String(), errb.String())
	}
	seen := map[string]bool{}
	for _, tr := range sig.Traders {
		if seen[strings.ToLower(tr.Trader)] {
			t.Errorf("trader %s counted twice", tr.Trader)
		}
		seen[strings.ToLower(tr.Trader)] = true
	}
	if p := posts(); len(p) == 0 || !strings.Contains(p[0], "Swarm") {
		t.Errorf("discord did not receive the alert: %v", p)
	}

	// The same events, recorded, replay offline.
	srv.Close()
	replay, e, code := run(t, srv, "radar", "--offline")
	if code != 0 || !strings.Contains(replay, "SWARM") {
		t.Fatalf("offline radar (%d):\n%s\n%s", code, replay, e)
	}
}

func TestNotifyConfig(t *testing.T) {
	srv := setupEnv(t, "test-key")
	if _, _, code := run(t, srv, "radar", "--notify", "telegram"); code != ExitUsage {
		t.Errorf("unconfigured telegram should be a usage error: %d", code)
	}
	if _, _, code := run(t, srv, "config", "set", "discord_webhook", "http://insecure"); code != ExitUsage {
		t.Errorf("non-https webhook accepted")
	}
	hook, posts := fakeDiscord(t)
	t.Setenv("FOMO_DISCORD_WEBHOOK", hook.URL)
	out, e, code := run(t, srv, "notify", "test")
	if code != 0 || !strings.Contains(out, "discord works") || len(posts()) != 1 {
		t.Fatalf("notify test (%d): %s %s", code, out, e)
	}
	run(t, srv, "config", "set", "telegram_token", "123:SECRETTOKEN")
	out, _, _ = run(t, srv, "config", "list")
	if strings.Contains(out, "SECRETTOKEN") {
		t.Error("telegram token shown unmasked in config list")
	}
}
