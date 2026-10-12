package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/FomoCLI/FomoCLI/internal/mock"
	"github.com/FomoCLI/FomoCLI/internal/ui"
)

// run executes the CLI in-process against a mock API and returns stdout,
// stderr and the exit code.
func run(t *testing.T, srv *mock.Server, args ...string) (string, string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	prevOut, prevErr := ui.Out, ui.Err
	ui.Out, ui.Err = &out, &errb
	defer func() { ui.Out, ui.Err = prevOut, prevErr }()

	root := NewRoot()
	base := []string{"--no-color"}
	if srv != nil {
		base = append(base, "--base-url", srv.URL)
	}
	root.SetArgs(append(base, args...))
	root.SetOut(&out)
	root.SetErr(&errb)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	code := ExitOK
	if err := root.ExecuteContext(ctx); err != nil {
		code = reportError(err)
	}
	return out.String(), errb.String(), code
}

func setupEnv(t *testing.T, key string) *mock.Server {
	t.Helper()
	t.Setenv("FOMO_CONFIG_DIR", t.TempDir())
	t.Setenv("FOMO_CACHE_DIR", t.TempDir())
	t.Setenv("FOMO_API_KEY", key)
	t.Setenv("FOMO_BASE_URL", "")
	t.Setenv("FOMO_OFFLINE", "")
	srv := mock.Start()
	t.Cleanup(srv.Close)
	return srv
}

func TestCommandsRender(t *testing.T) {
	srv := setupEnv(t, "test-key")
	cases := []struct {
		args []string
		want []string
	}{
		{[]string{"lb"}, []string{"Leaderboard · 24H", "@CryptoKaleo", "+$151.4K", "@ansem", "-$3.1K"}},
		{[]string{"lb", "7d", "--sort", "volume"}, []string{"Leaderboard · 7D", "@frankdegods"}},
		{[]string{"tokens", "graduated"}, []string{"Tokens · graduated", "$PONS", "▲ 38.4%"}},
		{[]string{"trader", "@CryptoKaleo"}, []string{"@CryptoKaleo", "K A L E O", "5AhfPStn66hRYoNNDfJHSDgCH7fBbwMQZUECRrhTo62F", "Ender", "+$54.1K"}},
		{[]string{"t", "CryptoKaleo", "positions"}, []string{"Positions · @CryptoKaleo", "WIN RATE", "50%", "$WIF", "recv'd", "983"}},
		{[]string{"t", "CryptoKaleo", "positions", "--status", "open"}, []string{"$PONS", "$AIRDROP"}},
		{[]string{"t", "CryptoKaleo", "balances"}, []string{"Portfolio", "$184.2K", "robinhood", "52.2%"}},
		{[]string{"t", "CryptoKaleo", "following", "--sort", "pnl"}, []string{"follows", "@frankdegods", "complete"}},
		{[]string{"t", "CryptoKaleo", "spotlight"}, []string{"Best trades", "Best theses", "Dog coins"}},
		{[]string{"t", "CryptoKaleo", "theses"}, []string{"Theses by @CryptoKaleo", "♥ 214", "DEV"}},
		{[]string{"whois", "1f08e6ab-5c73-5443-9225-bfc496cde51f"}, []string{"@CryptoKaleo"}},
		{[]string{"token", "0x39db"}, []string{"Token flow", "31.4%", "+$13K", "2.33"}},
		{[]string{"token", "0x51fb", "devs"}, []string{"Devs & insiders", "DEV", "Not selling"}},
		{[]string{"token", "0x39db", "holders"}, []string{"Smart-money holders", "@starcatcher444"}},
		{[]string{"token", "0x39db", "theses", "--sort", "likes"}, []string{"Theses on"}},
		{[]string{"trade", "a323b5ca-c769-4b07-a422-833c4cafbe1f"}, []string{"$PONS", "Swaps (2)", "$812K"}},
		{[]string{"trade", "a323", "comments"}, []string{"2 comments", "agreed, holder base is clean"}},
		{[]string{"activity"}, []string{"Multi-trader activity", "stale", "@frankdegods"}},
		{[]string{"search", "ansem"}, []string{"Traders", "Tokens", "Fu2oZoGxFtCDp29NKA4A89xcn255khq9xbxG7Mmtpump"}},
		{[]string{"theses"}, []string{"Latest theses", "312 on FOMO"}},
		{[]string{"alerts"}, []string{"Alerts · 3", "BUY", "@frankdegods", "$40K"}},
		{[]string{"status"}, []string{"OPERATIONAL", "98% success"}},
		{[]string{"credits"}, []string{"FREE", "243,500"}},
		{[]string{"api", "/v2/leaderboard/24h"}, []string{`"handle": "CryptoKaleo"`}},
	}
	for _, c := range cases {
		t.Run(strings.Join(c.args, "_"), func(t *testing.T) {
			out, errOut, code := run(t, srv, c.args...)
			if code != 0 {
				t.Fatalf("exit %d\nstderr: %s", code, errOut)
			}
			for _, w := range c.want {
				if !strings.Contains(out, w) {
					t.Errorf("output missing %q:\n%s", w, out)
				}
			}
		})
	}
}

func TestJSONIsPipeable(t *testing.T) {
	srv := setupEnv(t, "test-key")
	out, errOut, code := run(t, srv, "lb", "--json")
	if code != 0 {
		t.Fatal(errOut)
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, out)
	}
	if !strings.Contains(errOut, "credits") {
		t.Errorf("credits footer should go to stderr, got %q", errOut)
	}
}

func TestExitCodes(t *testing.T) {
	srv := setupEnv(t, "")
	if _, e, code := run(t, srv, "lb"); code != ExitAuth || !strings.Contains(e, "auth login") {
		t.Errorf("no key: code %d, stderr %q", code, e)
	}
	if _, _, code := run(t, srv, "lb", "--key", "broke"); code != ExitCredits {
		t.Errorf("broke: code %d", code)
	}
	if _, _, code := run(t, srv, "t", "nobody", "--key", "k"); code != ExitNotFound {
		t.Errorf("404: code %d", code)
	}
	if _, _, code := run(t, srv, "lb", "1y", "--key", "k"); code != ExitUsage {
		t.Errorf("bad window: code %d", code)
	}
	if _, _, code := run(t, srv, "lb", "--offline", "--key", "k"); code != ExitOffline {
		t.Errorf("offline miss: code %d", code)
	}
}

func TestOfflineAfterOnline(t *testing.T) {
	srv := setupEnv(t, "test-key")
	if _, e, code := run(t, srv, "t", "CryptoKaleo", "balances"); code != 0 {
		t.Fatal(e)
	}
	srv.Close()
	out, errOut, code := run(t, srv, "t", "CryptoKaleo", "balances", "--offline")
	if code != 0 || !strings.Contains(out, "$184.2K") || !strings.Contains(errOut, "offline") {
		t.Fatalf("offline replay failed (%d):\n%s\n%s", code, out, errOut)
	}
	// Network gone, --fresh asks for live data and falls back to the cache.
	out, errOut, code = run(t, srv, "t", "CryptoKaleo", "balances", "--fresh")
	if code != 0 || !strings.Contains(errOut, "showing cached data") {
		t.Fatalf("fallback failed (%d):\n%s\n%s", code, out, errOut)
	}
}

func TestAuthLoginAndConfig(t *testing.T) {
	srv := setupEnv(t, "")
	out, e, code := run(t, srv, "auth", "login", "--with-key", "my-key-123456")
	if code != 0 || !strings.Contains(out, "LOGGED IN") {
		t.Fatalf("login failed (%d): %s %s", code, out, e)
	}
	out, _, _ = run(t, srv, "auth", "status")
	if !strings.Contains(out, "my-k") || strings.Contains(out, "my-key-123456") {
		t.Errorf("status should show a masked key: %s", out)
	}
	if _, _, code := run(t, srv, "lb"); code != 0 {
		t.Errorf("saved key not used: %d", code)
	}
	if _, _, code := run(t, srv, "config", "set", "default_window", "7d"); code != 0 {
		t.Fatal("config set failed")
	}
	out, _, _ = run(t, srv, "lb")
	if !strings.Contains(out, "Leaderboard · 7D") {
		t.Errorf("default_window not applied: %s", out)
	}
	if _, _, code := run(t, srv, "config", "set", "default_window", "nope"); code != ExitUsage {
		t.Errorf("bad config value accepted")
	}
}

func TestLiveStreamAndReplay(t *testing.T) {
	srv := setupEnv(t, "test-key")
	srv.StreamInterval = 30 * time.Millisecond

	var out, errb bytes.Buffer
	prevOut, prevErr := ui.Out, ui.Err
	ui.Out, ui.Err = &out, &errb
	root := NewRoot()
	root.SetArgs([]string{"--no-color", "--base-url", srv.URL, "live", "--chain", "robinhood"})
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	if err := root.ExecuteContext(ctx); err != nil {
		t.Fatal(err)
	}
	ui.Out, ui.Err = prevOut, prevErr
	live := out.String()
	if !strings.Contains(live, "BUY") && !strings.Contains(live, "SELL") {
		t.Fatalf("no trades streamed:\n%s\n%s", live, errb.String())
	}
	if strings.Contains(live, "solana") {
		t.Errorf("chain filter leaked:\n%s", live)
	}

	srv.Close()
	replay, e, code := run(t, srv, "live", "--offline", "-n", "5")
	if code != 0 || !strings.Contains(replay, "replay") || strings.Count(replay, "@") != 5 {
		t.Fatalf("offline replay (%d):\n%s\n%s", code, replay, e)
	}
}

func TestHomeAndHelp(t *testing.T) {
	setupEnv(t, "")
	out, _, code := run(t, nil)
	if code != 0 || !strings.Contains(out, "fomocli auth login") {
		t.Errorf("home screen: %s", out)
	}
	out, _, _ = run(t, nil, "--help")
	for _, w := range []string{"Market:", "Traders & trades:", "Realtime:", "leaderboard", "dash"} {
		if !strings.Contains(out, w) {
			t.Errorf("help missing %q", w)
		}
	}
}
