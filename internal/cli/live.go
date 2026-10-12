package cli

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/FomoCLI/FomoCLI/internal/api"
	"github.com/FomoCLI/FomoCLI/internal/card"
	"github.com/FomoCLI/FomoCLI/internal/jx"
	"github.com/FomoCLI/FomoCLI/internal/ui"
)

type liveOpts struct {
	trader, chain, token, typ, source, side string
	minUSD                                  float64
	onchain, bell, noReplay                 bool
	limit                                   int
	notify                                  []string
}

func liveCmd() *cobra.Command {
	var o liveOpts
	cmd := &cobra.Command{
		Use:     "live",
		Aliases: []string{"feed", "tail", "stream"},
		Short:   "Stream trades in realtime (WebSocket, 0 credits)",
		Long: `A live ticker of FOMO trades. Two sources:

  app feed  (default)   wss /ws/alerts - exactly what the FOMO app shows: buys,
                        sells, theses. Realtime on paid keys; a free key is
                        realtime for 7 days then 15s delayed; no key = 60s delay.
  on-chain  (--onchain) wss /ws/trades - every fill read straight off the chain,
                        ~3.5s before the app. Growth & Scale plans only.

Streams cost zero credits. Every event is also recorded locally, so
'fomocli live --offline' replays the most recent ones without a connection.`,
		Example: `  fomocli live
  fomocli live --chain robinhood --min-usd 5000 --bell
  fomocli live --trader frankdegods
  fomocli live --type thesis
  fomocli live --onchain --chain solana --side buy
  fomocli live --json | jq -c 'select(.usdValue > 10000)'
  fomocli live --offline -n 50          # replay recorded events, no internet`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error { return runLive(cmd, o) },
	}
	f := cmd.Flags()
	f.StringVarP(&o.trader, "trader", "t", "", "only this trader (handle; wallet with --onchain)")
	f.StringVarP(&o.chain, "chain", "c", "", "robinhood | solana | base | bsc | eth | arc | monad | hyperliquid | <id>")
	f.StringVar(&o.token, "token", "", "only this token (symbol or contract)")
	f.StringVar(&o.typ, "type", "", "app feed: buy | sell | thesis | whale | price | trade")
	f.StringVar(&o.source, "source", "", "app feed: feed | push")
	f.StringVar(&o.side, "side", "", "on-chain: buy | sell")
	f.Float64Var(&o.minUSD, "min-usd", 0, "hide trades smaller than this many USD")
	f.BoolVar(&o.onchain, "onchain", false, "use the on-chain stream (Growth+)")
	f.BoolVar(&o.bell, "bell", false, "ring the terminal bell on every shown trade")
	f.BoolVar(&o.noReplay, "no-replay", false, "skip the replay of recent events on connect")
	f.IntVarP(&o.limit, "limit", "n", 100, "--offline: how many recorded events to replay")
	f.StringSliceVar(&o.notify, "notify", nil, "forward every shown trade to telegram, discord (pair with --min-usd)")
	return cmd
}

func watchCmd() *cobra.Command {
	var o liveOpts
	cmd := &cobra.Command{
		Use:     "watch <handle>",
		Aliases: []string{"follow", "copy"},
		Short:   "Follow one trader's buys, sells and theses live",
		Example: "  fomocli watch frankdegods\n  fomocli watch frankdegods --chain robinhood --bell",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o.trader = cleanHandle(args[0])
			return runLive(cmd, o)
		},
	}
	f := cmd.Flags()
	f.StringVarP(&o.chain, "chain", "c", "", "only this chain")
	f.Float64Var(&o.minUSD, "min-usd", 0, "hide trades smaller than this many USD")
	f.BoolVar(&o.bell, "bell", false, "ring the terminal bell on every trade")
	f.BoolVar(&o.onchain, "onchain", false, "use the on-chain stream (Growth+)")
	f.IntVarP(&o.limit, "limit", "n", 100, "--offline: how many recorded events to replay")
	f.StringSliceVar(&o.notify, "notify", nil, "forward their trades to telegram, discord")
	return cmd
}

func runLive(cmd *cobra.Command, o liveOpts) error {
	if o.chain == "" {
		o.chain = cfg.DefaultChain
	}
	rec := newRecorder(o.onchain)
	if g.offline {
		return replayRecorded(rec, o)
	}

	disp, err := dispatcher(o.notify)
	if err != nil {
		return err
	}
	if disp != nil {
		disp.Start(cmd.Context())
		defer disp.Close(5 * time.Second)
	}

	q := url.Values{}
	path := "/ws/alerts"
	if o.onchain {
		path = "/ws/trades"
		set(q, "side", o.side)
		if o.minUSD > 0 {
			q.Set("minUsd", fmt.Sprint(o.minUSD))
		}
	} else {
		set(q, "type", o.typ)
		set(q, "source", o.source)
	}
	set(q, "trader", o.trader)
	set(q, "chain", o.chain)
	set(q, "token", o.token)

	if !g.json {
		src := "app feed"
		if o.onchain {
			src = "on-chain"
		}
		var filt []string
		for _, kv := range [][2]string{{"trader", o.trader}, {"chain", o.chain}, {"token", o.token}, {"type", o.typ}, {"side", o.side}} {
			if kv[1] != "" {
				filt = append(filt, kv[0]+"="+kv[1])
			}
		}
		if o.minUSD > 0 {
			filt = append(filt, fmt.Sprintf("≥ %s", ui.USDf(o.minUSD)))
		}
		fs := "all trades"
		if len(filt) > 0 {
			fs = strings.Join(filt, " ")
		}
		p("\n  " + ui.Logo() + " " + ui.Bold.Render("live") + ui.Muted.Render(" · "+src+" · "+fs+" · ctrl+c to stop"))
		if client.Key == "" {
			p("  " + ui.Gold.Render("no API key: the app feed is 60s delayed. `fomocli auth login` for realtime."))
		}
		p()
	}

	var count int
	err = client.Stream(cmd.Context(), path, q, func(ev api.StreamEvent) {
		if ev.Err != nil || ev.Info != "" {
			if !g.json {
				msg := ev.Info
				if ev.Err != nil {
					msg = ui.Down.Render("disconnected: "+ev.Err.Error()) + ui.Muted.Render(" · "+ev.Info)
				} else {
					msg = ui.Up.Render("● ") + ui.Muted.Render(msg)
				}
				fmt.Fprintln(ui.Err, "  "+msg)
			}
			return
		}
		m := ev.Msg
		switch m.Get("type").Str() {
		case "welcome":
			if !g.json {
				mode := ui.Up.Render("realtime")
				if !m.Get("realtime").Bool() {
					mode = ui.Gold.Render(fmt.Sprintf("delayed %ss", m.Get("delaySeconds").Str()))
				}
				fmt.Fprintln(ui.Err, "  "+ui.Muted.Render("stream is ")+mode)
			}
			return
		case "subscribed", "pong", "ping":
			return
		case "retract":
			if !g.json {
				p("  " + ui.Gold.Render("↺ retracted ") + ui.Muted.Render(m.Get("id").Str()+" (not a FOMO trade after all)"))
			}
			return
		case "alert", "trade":
		default:
			if !m.Get("trader").Exists() {
				return
			}
		}
		if o.noReplay && m.Get("replay").Bool() {
			return
		}
		if o.minUSD > 0 && m.Get("usdValue").Num() < o.minUSD && m.Get("alertType").Str() != "thesis" {
			return
		}
		rec.write(ev.Raw)
		count++
		if g.json {
			fmt.Fprintln(ui.Out, string(ev.Raw))
			return
		}
		p(liveLine(m))
		if !m.Get("replay").Bool() {
			disp.Push(liveText(m))
		}
		if o.bell {
			fmt.Fprint(ui.Err, "\a")
		}
	})
	rec.close()
	if !g.json && err == nil {
		fmt.Fprintln(ui.Err, "\n  "+ui.Muted.Render(fmt.Sprintf("%d events · recorded for offline replay", count)))
	}
	return err
}

// liveText is the chat-message form of a feed event.
func liveText(m jx.V) string {
	t, ok := tradeFromMsg(m)
	if !ok {
		return ui.OneLine(m.Get("text").Str()) + "\n— FomoCLI"
	}
	icon := "🟢 BUY"
	if t.Side == "sell" {
		icon = "🔴 SELL"
	}
	msg := fmt.Sprintf("%s @%s $%s %s (%s)", icon, t.Trader, t.Symbol, card.CompactUSD(abs(t.USD)), t.Chain)
	if t.Address != "" {
		msg += "\nCA: " + t.Address
	}
	return msg + "\n— FomoCLI"
}

func set(q url.Values, k, v string) {
	if v != "" {
		q.Set(k, v)
	}
}

// liveLine formats one stream event as a ticker row.
func liveLine(m jx.V) string {
	t, ok := ui.Time(m.First("ts", "blockTs", "seenAt"))
	if !ok {
		t = time.Now()
	}
	ts := ui.Muted.Render(t.Local().Format("15:04:05"))

	side := m.First("alertType", "side").Str()
	trader := m.Get("trader")
	who := trader.Str()
	if trader.IsObject() {
		who = trader.Get("handle").Str()
		if who == "" {
			who = ui.Short(trader.Get("wallet").Str())
		}
	}
	tok := m.Get("token")
	sym := tok.Str()
	addr := m.Get("tokenAddress").Str()
	if tok.IsObject() {
		sym = tok.Get("symbol").Str()
		addr = tok.Get("address").Str()
	}
	usd := m.Get("usdValue").Num()
	size := fmt.Sprintf("%8s", ui.USDf(usd))
	switch {
	case usd >= 25000:
		size = ui.Gold.Bold(true).Render(size) + " 🐋"
	case usd >= 5000:
		size = ui.Gold.Render(size) + "  "
	default:
		size += "  "
	}
	if usd == 0 {
		size = fmt.Sprintf("%8s  ", "")
	}
	line := fmt.Sprintf("  %s  %s  %s  %s %s  %s",
		ts, ui.Side(side), padR(ui.Handle.Render("@"+who), 20), padR(ui.Bold.Render("$"+sym), 12), size, padR(ui.Chain(m.Get("chain").Str()), 11))
	if side == "thesis" {
		if txt := ui.OneLine(m.Get("text").Str()); txt != "" {
			line += ui.Muted.Render(ui.Truncate("“"+txt+"”", max(20, ui.Width()-90)))
		}
	} else if addr != "" {
		line += ui.Muted.Render(ui.Short(addr))
	}
	if lat := m.Get("latencyMs"); lat.Exists() {
		line += ui.Muted.Render(fmt.Sprintf("  +%sms", lat.Str()))
	}
	return line
}

func padR(s string, n int) string {
	return s + strings.Repeat(" ", max(0, n-ui.VisibleWidth(s)))
}

// ---------------------------------------------------------------- recorder

const recordKeep = 2000

type recorder struct {
	path string
	f    *os.File
	n    int
}

func newRecorder(onchain bool) *recorder {
	if client.Cache == nil {
		return &recorder{}
	}
	name := "live-alerts.ndjson"
	if onchain {
		name = "live-trades.ndjson"
	}
	return &recorder{path: filepath.Join(client.Cache.Dir, name)}
}

func (r *recorder) write(b []byte) {
	if r.path == "" {
		return
	}
	if r.f == nil {
		_ = os.MkdirAll(filepath.Dir(r.path), 0o700)
		f, err := os.OpenFile(r.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			r.path = ""
			return
		}
		r.f = f
	}
	_, _ = r.f.Write(append(bytes.TrimSpace(b), '\n'))
	r.n++
}

// close flushes and trims the log to the last recordKeep events.
func (r *recorder) close() {
	if r.f != nil {
		_ = r.f.Close()
	}
	if r.path == "" || r.n == 0 {
		return
	}
	lines := readLines(r.path)
	if len(lines) > recordKeep {
		lines = lines[len(lines)-recordKeep:]
		_ = os.WriteFile(r.path, []byte(strings.Join(lines, "\n")+"\n"), 0o600)
	}
}

func readLines(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 4<<20)
	for sc.Scan() {
		if l := strings.TrimSpace(sc.Text()); l != "" {
			out = append(out, l)
		}
	}
	return out
}

func replayRecorded(r *recorder, o liveOpts) error {
	lines := readLines(r.path)
	if len(lines) == 0 {
		return fmt.Errorf("%w (no recorded live events yet - run `fomocli live` online first)", api.ErrNotCached)
	}
	var shown []jx.V
	var raws []string
	for _, l := range lines {
		v, err := jx.Parse([]byte(l))
		if err != nil {
			continue
		}
		if !matchLocal(v, o) {
			continue
		}
		shown = append(shown, v)
		raws = append(raws, l)
	}
	if o.limit > 0 && len(shown) > o.limit {
		shown, raws = shown[len(shown)-o.limit:], raws[len(raws)-o.limit:]
	}
	if g.json {
		for _, l := range raws {
			fmt.Fprintln(ui.Out, l)
		}
		return nil
	}
	p("\n  " + ui.Logo() + " " + ui.Bold.Render("replay") + ui.Muted.Render(fmt.Sprintf(" · %d recorded events · offline", len(shown))))
	p()
	for _, v := range shown {
		p(liveLine(v))
	}
	return nil
}

func matchLocal(v jx.V, o liveOpts) bool {
	trader := v.Get("trader")
	who := trader.Str()
	if trader.IsObject() {
		who = trader.Get("handle").Str() + " " + trader.Get("wallet").Str()
	}
	tok := v.Get("token")
	sym := tok.Str() + " " + v.Get("tokenAddress").Str()
	if tok.IsObject() {
		sym = tok.Get("symbol").Str() + " " + tok.Get("address").Str()
	}
	switch {
	case o.trader != "" && !strings.Contains(strings.ToLower(who), strings.ToLower(o.trader)):
		return false
	case o.chain != "" && !strings.EqualFold(v.Get("chain").Str(), o.chain) && v.Get("chainId").Str() != o.chain:
		return false
	case o.token != "" && !strings.Contains(strings.ToLower(sym), strings.ToLower(o.token)):
		return false
	case o.typ != "" && !strings.EqualFold(v.Get("alertType").Str(), o.typ):
		return false
	case o.side != "" && !strings.EqualFold(v.First("side", "alertType").Str(), o.side):
		return false
	case o.minUSD > 0 && v.Get("usdValue").Num() < o.minUSD:
		return false
	}
	return true
}

// ---------------------------------------------------------------- alerts (REST)

func alertsCmd() *cobra.Command {
	var typ, chain, trader, token, since string
	var limit int
	cmd := &cobra.Command{
		Use:   "alerts",
		Short: "Recent buys, sells & theses (REST snapshot of the feed, 125 credits)",
		Example: `  fomocli alerts
  fomocli alerts --type whale --chain robinhood
  fomocli alerts --trader frankdegods -n 50`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if chain == "" {
				chain = cfg.DefaultChain
			}
			params := api.Params{"type": typ, "chain": chain, "trader": cleanHandle(trader), "token": token, "since": since, "limit": api.Itoa(limit)}
			return show(cmd, "loading alerts", func(ctx context.Context) (*api.Response, error) {
				return client.Alerts(ctx, params)
			}, func(v jx.V) {
				rows := v.List("alerts", "items", "events")
				p(ui.Section(fmt.Sprintf("Alerts · %d", len(rows))))
				p()
				// newest first from the API; print oldest→newest like a log
				for i := len(rows) - 1; i >= 0; i-- {
					p(liveLine(rows[i]))
				}
				if len(rows) == 0 {
					p("  " + ui.Muted.Render("(no alerts)"))
				}
				p("\n  " + ui.Muted.Render("stream these live for 0 credits: ") + ui.Bold.Render("fomocli live"))
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&typ, "type", "", "whale | thesis | price | trade | follow | buy | sell")
	f.StringVarP(&chain, "chain", "c", "", "robinhood | solana | base | bsc | eth | arc | monad | hyperliquid")
	f.StringVarP(&trader, "trader", "t", "", "only this trader")
	f.StringVar(&token, "token", "", "only this token")
	f.StringVar(&since, "since", "", "ISO timestamp")
	f.IntVarP(&limit, "limit", "n", 30, "max alerts")
	return cmd
}
