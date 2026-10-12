package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/FomoCLI/FomoCLI/internal/api"
	"github.com/FomoCLI/FomoCLI/internal/jx"
	"github.com/FomoCLI/FomoCLI/internal/ui"
)

// fetch runs one API call behind a spinner.
func fetch(cmd *cobra.Command, msg string, fn func(ctx context.Context) (*api.Response, error)) (*api.Response, error) {
	sp := ui.Spin(msg)
	res, err := fn(cmd.Context())
	sp.Stop()
	return res, err
}

// show fetches and renders a response: raw JSON with --json, otherwise the
// given renderer followed by the credits/cache footer.
func show(cmd *cobra.Command, msg string, fn func(ctx context.Context) (*api.Response, error), render func(v jx.V)) error {
	res, err := fetch(cmd, msg, fn)
	if err != nil {
		return err
	}
	emit(res, render)
	return nil
}

func emit(res *api.Response, render func(v jx.V)) {
	if g.json {
		fmt.Fprintln(ui.Out, res.Data.JSON())
		footer(res)
		return
	}
	if res.Data.Get("available").Exists() && !res.Data.Get("available").Bool() && len(res.Data.List("traders", "tokens", "trades", "positions", "theses", "items", "data")) == 0 {
		fmt.Fprintln(ui.Out, ui.Muted.Render("  Nothing available for this yet - FOMO has no data here (available: false)."))
		footer(res)
		return
	}
	render(res.Data)
	footer(res)
}

// footer prints how a response was obtained: credits spent, or cache age.
// It goes to stderr so stdout stays pipeable.
func footer(res *api.Response) {
	if g.quiet || res == nil {
		return
	}
	var parts []string
	switch {
	case res.Fallback:
		parts = append(parts, ui.Gold.Render("⚠ offline")+ui.Muted.Render(" · showing cached data from "+ui.AgoT(res.FetchedAt)))
	case res.FromCache && client.Mode == api.ModeOffline:
		parts = append(parts, ui.Gold.Render("⚠ offline")+ui.Muted.Render(" · data from "+ui.AgoT(res.FetchedAt)))
	case res.FromCache:
		parts = append(parts, ui.Cyan.Render("◷ cached")+ui.Muted.Render(" "+ui.AgoT(res.FetchedAt)+" · 0 credits · --fresh to refetch"))
	default:
		if res.CreditsCost >= 0 {
			parts = append(parts, ui.Gold.Render("⚡ "+ui.Commas(float64(res.CreditsCost), 0)+" credits"))
		}
		if res.CreditsRemaining >= 0 {
			parts = append(parts, ui.Muted.Render(ui.Commas(float64(res.CreditsRemaining), 0)+" left"))
		}
	}
	if res.Deprecated {
		parts = append(parts, ui.Gold.Render("deprecated endpoint"))
	}
	if len(parts) == 0 {
		return
	}
	fmt.Fprintln(ui.Err, "\n  "+strings.Join(parts, ui.Muted.Render(" · ")))
}

// note prints a muted note line from a payload's "note" field, if any.
func note(v jx.V) {
	if n := v.Get("note").Str(); n != "" {
		fmt.Fprintln(ui.Out, "\n  "+ui.Gold.Render("ℹ ")+ui.Muted.Render(n))
	}
}

func p(a ...any) { fmt.Fprintln(ui.Out, a...) }

func pf(format string, a ...any) { fmt.Fprintf(ui.Out, format, a...) }

// traderName renders "@handle" plus a verified tick when FOMO reports one.
func traderName(handle string, verified bool) string {
	s := ui.Handle.Render("@" + handle)
	if verified {
		s += " " + ui.Check(true)
	}
	return s
}

func cleanHandle(h string) string { return strings.TrimPrefix(strings.TrimSpace(h), "@") }

// looksLikeID reports whether s is a FOMO userId (UUID).
func looksLikeID(s string) bool {
	return len(s) == 36 && strings.Count(s, "-") == 4
}

// tokenLabel renders "$SYMBOL" for a token object or string.
func tokenLabel(v jx.V) string {
	if v.IsObject() {
		if s := v.Get("symbol").Str(); s != "" {
			return ui.Bold.Render("$" + s)
		}
		if s := v.Get("name").Str(); s != "" {
			return ui.Bold.Render(s)
		}
		return ui.Muted.Render(ui.Short(v.Get("address").Str()))
	}
	if s := v.Str(); s != "" {
		return ui.Bold.Render("$" + s)
	}
	return ui.Muted.Render(ui.Dash)
}

func chainOf(v jx.V) string {
	if c := v.First("chain").Str(); c != "" {
		return c
	}
	if id := v.First("networkId", "chainId").Str(); id != "" {
		return ui.ChainName(id)
	}
	if id := v.Get("token", "networkId").Str(); id != "" {
		return ui.ChainName(id)
	}
	return ""
}

func home() error {
	p(ui.Banner(Version))
	key, src := cfg.ResolvedKey()
	if g.key != "" {
		key, src = g.key, "--key flag"
	}
	if key == "" {
		p("  " + ui.BadgeW.Render("NO KEY") + "  Get a free key (250k credits/mo) at " + ui.Cyan.Render("https://fomoapi.io/dashboard"))
		p("          then run " + ui.Bold.Render("fomo auth login"))
	} else {
		p("  " + ui.Badge.Render("READY") + "  key " + ui.Muted.Render(maskKey(key)+" from "+src))
	}
	p(ui.Section("Try"))
	p()
	ex := [][2]string{
		{"fomo lb 24h", "top traders by PnL today"},
		{"fomo trader CryptoKaleo", "profile, wallets & PnL of a trader"},
		{"fomo trader CryptoKaleo positions", "their open & closed positions"},
		{"fomo tokens trending", "what FOMO users trade right now"},
		{"fomo live --chain solana", "streaming trade ticker"},
		{"fomo dash", "full-screen interactive dashboard"},
		{"fomo lb --offline", "last leaderboard you saw, no internet"},
	}
	for _, e := range ex {
		pf("  %s  %s\n", ui.Accent.Render(fmt.Sprintf("%-36s", "$ "+e[0])), ui.Muted.Render(e[1]))
	}
	p("\n  " + ui.Muted.Render("All commands: ") + ui.Bold.Render("fomo --help") + ui.Muted.Render("   ·   Docs: https://fomoapi.io/docs"))
	return nil
}
