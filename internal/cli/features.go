package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/FomoCLI/FomoCLI/internal/api"
	"github.com/FomoCLI/FomoCLI/internal/card"
	"github.com/FomoCLI/FomoCLI/internal/jx"
	"github.com/FomoCLI/FomoCLI/internal/notify"
	"github.com/FomoCLI/FomoCLI/internal/radar"
	"github.com/FomoCLI/FomoCLI/internal/scan"
	"github.com/FomoCLI/FomoCLI/internal/ui"
)

// ================================================================ notify

// dispatcher builds a notification dispatcher for the requested targets.
func dispatcher(targets []string) (*notify.Dispatcher, error) {
	if len(targets) == 0 {
		return nil, nil
	}
	tgToken, tgChat, discord := cfg.Notify()
	d := &notify.Dispatcher{OnError: func(name string, err error) {
		fmt.Fprintln(ui.Err, "  "+ui.BadgeW.Render("NOTIFY")+" "+name+": "+err.Error())
	}}
	for _, t := range targets {
		switch strings.ToLower(strings.TrimSpace(t)) {
		case "telegram", "tg":
			if tgToken == "" || tgChat == "" {
				return nil, usagef("telegram is not set up: run `fomocli notify setup`")
			}
			d.Senders = append(d.Senders, &notify.Telegram{Token: tgToken, ChatID: tgChat})
		case "discord":
			if discord == "" {
				return nil, usagef("discord is not set up: run `fomocli notify setup`")
			}
			d.Senders = append(d.Senders, &notify.Discord{WebhookURL: discord})
		case "":
		default:
			return nil, usagef("unknown notify target %q (use telegram, discord)", t)
		}
	}
	return d, nil
}

func notifyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "notify",
		Short: "Send alerts to Telegram or Discord",
		Long: `Push FomoCLI alerts to a chat. Works with radar, live, watch, scan and card:

  fomocli radar --notify telegram
  fomocli watch frankdegods --notify discord
  fomocli live --min-usd 25000 --notify telegram,discord
  fomocli card CryptoKaleo --send telegram`,
	}
	setup := &cobra.Command{
		Use:   "setup",
		Short: "How to connect Telegram and Discord",
		Run: func(cmd *cobra.Command, args []string) {
			p(ui.Section("Telegram"))
			p()
			p("  1. Open Telegram, talk to " + ui.Bold.Render("@BotFather") + ", send /newbot and copy the token.")
			p("  2. Add the bot to your group/channel (as admin for channels) and send it a message.")
			p("  3. Get the chat id: open https://api.telegram.org/bot<TOKEN>/getUpdates and copy chat.id")
			p("  4. " + ui.Bold.Render("fomocli config set telegram_token <TOKEN>"))
			p("     " + ui.Bold.Render("fomocli config set telegram_chat_id <CHAT_ID>"))
			p(ui.Section("Discord"))
			p()
			p("  1. Channel settings → Integrations → Webhooks → New Webhook → Copy Webhook URL")
			p("  2. " + ui.Bold.Render("fomocli config set discord_webhook <URL>"))
			p()
			p("  " + ui.Muted.Render("Or use env vars: FOMO_TELEGRAM_TOKEN, FOMO_TELEGRAM_CHAT_ID, FOMO_DISCORD_WEBHOOK"))
			p("  " + ui.Muted.Render("Then check it: ") + ui.Bold.Render("fomocli notify test --to telegram"))
		},
	}
	var to []string
	test := &cobra.Command{
		Use:   "test",
		Short: "Send a test message",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(to) == 0 {
				tg, chat, dc := cfg.Notify()
				if tg != "" && chat != "" {
					to = append(to, "telegram")
				}
				if dc != "" {
					to = append(to, "discord")
				}
				if len(to) == 0 {
					return usagef("nothing configured yet: run `fomocli notify setup`")
				}
			}
			d, err := dispatcher(to)
			if err != nil {
				return err
			}
			for _, s := range d.Senders {
				sp := ui.Spin("sending to " + s.Name())
				err := s.Send(cmd.Context(), "✅ FomoCLI is connected. Alerts will arrive here.")
				sp.Stop()
				if err != nil {
					p("  " + ui.Down.Render("✖ "+s.Name()+": "+err.Error()))
					continue
				}
				p("  " + ui.Up.Render("✔ "+s.Name()+" works"))
			}
			return nil
		},
	}
	test.Flags().StringSliceVar(&to, "to", nil, "telegram, discord (default: everything configured)")
	cmd.AddCommand(setup, test)
	return cmd
}

// ================================================================ radar

type radarOpts struct {
	window     time.Duration
	minTraders int
	minUSD     float64
	side       string
	chain      string
	onchain    bool
	bell       bool
	notify     []string
	limit      int
}

func radarCmd() *cobra.Command {
	var o radarOpts
	cmd := &cobra.Command{
		Use:     "radar",
		Aliases: []string{"swarm"},
		Short:   "Swarm radar: alarm when several traders pile into one token (0 credits)",
		Long: `Watches the live feed and fires when several different traders buy (or
sell) the same token inside a short window - the coordinated move you would
otherwise only notice afterwards. Uses the free WebSocket stream: 0 credits.

An alert fires when a swarm forms, and again each time another trader joins.
Add --notify to forward alerts to Telegram or Discord.`,
		Example: `  fomocli radar
  fomocli radar --min-traders 4 --window 5m --min-usd 2000
  fomocli radar --side sell                    # coordinated dumps
  fomocli radar --chain robinhood --bell --notify telegram
  fomocli radar --offline                      # replay recorded events`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error { return runRadar(cmd, o) },
	}
	f := cmd.Flags()
	f.DurationVarP(&o.window, "window", "w", 10*time.Minute, "how close together the trades must be")
	f.IntVarP(&o.minTraders, "min-traders", "m", 3, "distinct traders needed to fire")
	f.Float64Var(&o.minUSD, "min-usd", 0, "ignore trades smaller than this")
	f.StringVar(&o.side, "side", "buy", "buy | sell | both")
	f.StringVarP(&o.chain, "chain", "c", "", "only this chain")
	f.BoolVar(&o.onchain, "onchain", false, "use the on-chain stream: every fill, ~3.5s earlier (Growth+)")
	f.BoolVar(&o.bell, "bell", false, "ring the terminal bell on every alert")
	f.StringSliceVar(&o.notify, "notify", nil, "forward alerts: telegram, discord")
	f.IntVarP(&o.limit, "limit", "n", 0, "--offline: only replay the last N recorded events")
	return cmd
}

// tradeFromMsg converts an app-feed alert or on-chain trade into a radar trade.
func tradeFromMsg(m jx.V) (radar.Trade, bool) {
	t := radar.Trade{Side: strings.ToLower(m.First("alertType", "side").Str()), Chain: m.Get("chain").Str()}
	if t.Side != "buy" && t.Side != "sell" {
		return t, false
	}
	tr := m.Get("trader")
	if tr.IsObject() {
		t.Trader = tr.Get("handle").Str()
		t.TraderID = tr.Get("wallet").Str()
		if t.Trader == "" {
			t.Trader = ui.Short(t.TraderID)
		}
	} else {
		// Key on the handle: it is on every feed event, while userId is
		// missing from some (push-sourced) alerts.
		t.Trader = tr.Str()
		t.TraderID = strings.ToLower(t.Trader)
		if t.TraderID == "" {
			t.TraderID = m.Get("userId").Str()
		}
	}
	tok := m.Get("token")
	if tok.IsObject() {
		t.Symbol, t.Address = tok.Get("symbol").Str(), tok.Get("address").Str()
	} else {
		t.Symbol, t.Address = tok.Str(), m.Get("tokenAddress").Str()
	}
	if t.Chain == "" {
		t.Chain = ui.ChainName(m.Get("chainId").Str())
	}
	t.USD = m.Get("usdValue").Num()
	if at, ok := ui.Time(m.First("ts", "blockTs", "seenAt")); ok {
		t.At = at
	} else {
		t.At = time.Now()
	}
	return t, t.Trader != ""
}

func runRadar(cmd *cobra.Command, o radarOpts) error {
	if o.chain == "" {
		o.chain = cfg.DefaultChain
	}
	side := strings.ToLower(o.side)
	if side != "buy" && side != "sell" && side != "both" {
		return usagef("--side must be buy, sell or both")
	}
	det := radar.New(radar.Config{Window: o.window, MinTraders: o.minTraders, MinUSD: o.minUSD, Side: side})
	d, err := dispatcher(o.notify)
	if err != nil {
		return err
	}
	if d != nil {
		d.Start(cmd.Context())
		defer d.Close(5 * time.Second)
	}

	alerts := 0
	handle := func(m jx.V) {
		t, ok := tradeFromMsg(m)
		if !ok {
			return
		}
		if o.chain != "" && !strings.EqualFold(t.Chain, o.chain) && ui.ChainName(o.chain) != t.Chain {
			return
		}
		sig := det.Add(t)
		if sig == nil {
			return
		}
		alerts++
		if g.json {
			b, _ := json.Marshal(sig)
			fmt.Fprintln(ui.Out, string(b))
		} else {
			p(radarBox(sig))
		}
		if o.bell {
			fmt.Fprint(ui.Err, "\a")
		}
		d.Push(radarText(sig))
	}

	header := func(src string) {
		if g.json {
			return
		}
		sideTxt := map[string]string{"buy": "buys", "sell": "sells", "both": "buys & sells"}[side]
		p("\n  " + ui.Logo() + " " + ui.Bold.Render("radar") + ui.Muted.Render(fmt.Sprintf(" · %s · ≥%d traders · %s window · %s", src, o.minTraders, ui.Dur(o.window), sideTxt)))
		if o.minUSD > 0 {
			p("  " + ui.Muted.Render("ignoring trades under "+ui.USDf(o.minUSD)))
		}
		if d != nil {
			var names []string
			for _, s := range d.Senders {
				names = append(names, s.Name())
			}
			p("  " + ui.Muted.Render("forwarding alerts to "+strings.Join(names, ", ")))
		}
		p()
	}

	if g.offline {
		rec := newRecorder(o.onchain)
		lines := readLines(rec.path)
		if len(lines) == 0 {
			return fmt.Errorf("%w (no recorded live events yet - run `fomocli live` or `fomocli radar` online first)", api.ErrNotCached)
		}
		if o.limit > 0 && len(lines) > o.limit {
			lines = lines[len(lines)-o.limit:]
		}
		header(fmt.Sprintf("replaying %d recorded events", len(lines)))
		for _, l := range lines {
			if v, err := jx.Parse([]byte(l)); err == nil {
				handle(v)
			}
		}
		if !g.json {
			n, _ := det.Stats()
			p("  " + ui.Muted.Render(fmt.Sprintf("%d trades scanned · %d swarm alerts", n, alerts)))
		}
		return nil
	}

	q := url.Values{}
	path := "/ws/alerts"
	if o.onchain {
		path = "/ws/trades"
		if side != "both" {
			q.Set("side", side)
		}
	}
	set(q, "chain", o.chain)
	src := "app feed"
	if o.onchain {
		src = "on-chain"
	}
	header(src)
	if client.Key == "" && !g.json {
		p("  " + ui.Gold.Render("no API key: the app feed is 60s delayed. `fomocli auth login` for realtime.") + "\n")
	}

	rec := newRecorder(o.onchain)
	defer rec.close()
	lastBeat := time.Now()
	err = client.Stream(cmd.Context(), path, q, func(ev api.StreamEvent) {
		if ev.Err != nil || ev.Info != "" {
			if !g.json {
				msg := ui.Up.Render("● ") + ui.Muted.Render(ev.Info)
				if ev.Err != nil {
					msg = ui.Down.Render("disconnected: "+ev.Err.Error()) + ui.Muted.Render(" · "+ev.Info)
				}
				fmt.Fprintln(ui.Err, "  "+msg)
			}
			return
		}
		typ := ev.Msg.Get("type").Str()
		if typ == "welcome" && !g.json {
			mode := ui.Up.Render("realtime")
			if !ev.Msg.Get("realtime").Bool() {
				mode = ui.Gold.Render("delayed " + ev.Msg.Get("delaySeconds").Str() + "s")
			}
			fmt.Fprintln(ui.Err, "  "+ui.Muted.Render("stream is ")+mode+ui.Muted.Render(" · scanning for swarms…"))
			return
		}
		if typ != "alert" && typ != "trade" {
			return
		}
		rec.write(ev.Raw)
		handle(ev.Msg)
		if !g.json && time.Since(lastBeat) > 30*time.Second {
			lastBeat = time.Now()
			fmt.Fprintln(ui.Err, "  "+heartbeat(det))
		}
	})
	if !g.json && err == nil {
		n, _ := det.Stats()
		fmt.Fprintln(ui.Err, "\n  "+ui.Muted.Render(fmt.Sprintf("%d trades scanned · %d swarm alerts", n, alerts)))
	}
	return err
}

func heartbeat(det *radar.Detector) string {
	n, tokens := det.Stats()
	var hot []string
	for _, h := range det.Hottest(3) {
		if h.Traders < 2 {
			break
		}
		hot = append(hot, fmt.Sprintf("$%s %d👤", h.Symbol, h.Traders))
	}
	s := ui.Muted.Render(fmt.Sprintf("· %d trades · %d tokens in window", n, tokens))
	if len(hot) > 0 {
		s += ui.Muted.Render(" · heating up: ") + ui.Bold.Render(strings.Join(hot, "  "))
	}
	return s
}

func radarBox(s *radar.Signal) string {
	border := ui.ColGreen
	title := "🚨 SWARM BUY"
	if s.Side == "sell" {
		border, title = ui.ColRed, "🚨 SWARM SELL"
	}
	if !s.First {
		title = "▲ SWARM GROWS"
		if s.Side == "sell" {
			title = "▼ DUMP GROWS"
		}
	}
	verb := map[string]string{"buy": "buys", "sell": "sells"}[s.Side]
	head := ui.Bold.Render(title) + "  " + ui.Bold.Render("$"+s.Symbol) + "  " + ui.Chain(s.Chain) +
		ui.Muted.Render("   "+s.At.Local().Format("15:04:05"))
	stats := ui.Bold.Render(fmt.Sprintf("%d traders", s.Count())) + ui.Muted.Render(fmt.Sprintf(" · %d %s · ", s.Trades, verb)) +
		ui.Bold.Render(ui.USDf(s.TotalUSD)) + ui.Muted.Render(" in "+ui.Dur(s.Span))
	var who []string
	for i, t := range s.Traders {
		if i == 5 {
			who = append(who, ui.Muted.Render(fmt.Sprintf("+%d more", len(s.Traders)-5)))
			break
		}
		who = append(who, ui.Handle.Render("@"+t.Trader)+" "+ui.Muted.Render(ui.USDf(t.USD)))
	}
	lines := []string{head, stats, strings.Join(who, ui.Muted.Render(" · "))}
	if s.Address != "" {
		lines = append(lines, ui.Muted.Render(s.Address)+ui.Muted.Render("   check it: ")+ui.Bold.Render("fomocli scan "+ui.Short(s.Address)))
	}
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(border).Padding(0, 1)
	return indent(box.Render(strings.Join(lines, "\n")), 2)
}

func radarText(s *radar.Signal) string {
	title := "🚨 Swarm BUY"
	if s.Side == "sell" {
		title = "🚨 Swarm SELL"
	}
	if !s.First {
		title = "📈 Swarm grows"
	}
	var who []string
	for i, t := range s.Traders {
		if i == 8 {
			break
		}
		who = append(who, "@"+t.Trader+" "+card.CompactUSD(t.USD))
	}
	msg := fmt.Sprintf("%s $%s (%s)\n%d traders · %s in %s\n%s", title, s.Symbol, s.Chain, s.Count(),
		card.CompactUSD(s.TotalUSD), ui.Dur(s.Span), strings.Join(who, ", "))
	if s.Address != "" {
		msg += "\nCA: " + s.Address
	}
	return msg + "\n— FomoCLI radar"
}

// ================================================================ scan

func scanCmd() *cobra.Command {
	var network string
	var notifyTo []string
	cmd := &cobra.Command{
		Use:     "scan <token address>",
		Aliases: []string{"rug", "check"},
		Short:   "Rug scanner: one risk score from flow, devs and smart money (750 credits)",
		Long: `Pulls a token's buy/sell flow, its dev & insider positions and the tracked
traders holding it, and turns them into a 0-100 risk score with the reasons.

It is a triage heuristic, not a safety guarantee - read the reasons.
Costs 3 calls (750 credits); repeats inside the cache TTL are free.`,
		Example: `  fomocli scan 0x39dbed3a77c1b2e0f11e6c0b8c7b9e0a1f2d3c4b
  fomocli scan <mint> --network 1399811149
  fomocli scan <address> --notify telegram`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			addr := strings.TrimSpace(args[0])
			d, err := dispatcher(notifyTo)
			if err != nil {
				return err
			}
			in := scan.Inputs{Address: addr}
			var spent int64 = -1
			var last *api.Response
			var firstErr error
			step := func(label string, fn func(context.Context) (*api.Response, error)) jx.V {
				res, err := fetch(cmd, label, fn)
				if err != nil {
					var ae *api.APIError
					if errors.As(err, &ae) && (ae.Status == 401 || ae.Status == 402 || ae.Status == 403) {
						firstErr = err
					} else if firstErr == nil {
						firstErr = err
					}
					return jx.V{}
				}
				last = res
				if res.CreditsCost >= 0 {
					spent = max(spent, 0) + res.CreditsCost
				}
				return res.Data
			}
			in.Stats = step("reading flow (1/3)", func(ctx context.Context) (*api.Response, error) { return client.TokenStats(ctx, addr, network) })
			if errors.Is(firstErr, api.ErrNotCached) || isAuthOrCredits(firstErr) {
				return firstErr
			}
			in.Devs = step("checking devs (2/3)", func(ctx context.Context) (*api.Response, error) { return client.TokenDevs(ctx, addr, network) })
			in.Holders = step("finding smart money (3/3)", func(ctx context.Context) (*api.Response, error) { return client.TokenHolders(ctx, addr, 0) })
			if in.Stats.IsNull() && in.Devs.IsNull() && in.Holders.IsNull() {
				return firstErr
			}
			r := scan.Score(in)
			if g.json {
				b, _ := json.MarshalIndent(r, "", "  ")
				p(string(b))
			} else {
				renderScan(r)
			}
			if d != nil {
				d.Start(cmd.Context())
				d.Push(scanText(r))
				d.Close(10 * time.Second)
			}
			if last != nil {
				last.CreditsCost = spent
				footer(last)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&network, "network", "", "network id for tokens outside the directory (4663 Robinhood, 1399811149 Solana)")
	cmd.Flags().StringSliceVar(&notifyTo, "notify", nil, "also send the verdict to telegram, discord")
	return cmd
}

func isAuthOrCredits(err error) bool {
	var ae *api.APIError
	return errors.As(err, &ae) && (ae.Status == 401 || ae.Status == 402 || ae.Status == 403)
}

func renderScan(r scan.Result) {
	col := ui.Up
	switch r.Verdict {
	case "MEDIUM":
		col = ui.Gold
	case "HIGH":
		col = ui.Down
	}
	const width = 30
	filled := r.Risk * width / 100
	gauge := col.Render(strings.Repeat("█", filled)) + ui.Muted.Render(strings.Repeat("░", width-filled))
	badge := map[string]lipgloss.Style{"LOW": ui.Badge, "MEDIUM": ui.BadgeW, "HIGH": ui.BadgeE}[r.Verdict]
	head := ui.Bold.Render("RUG SCAN") + "  " + ui.Muted.Render(ui.Short(r.Address)) + "\n\n" +
		"RISK  " + gauge + "  " + col.Bold(true).Render(fmt.Sprintf("%d", r.Risk)) + ui.Muted.Render("/100  ") + badge.Render(r.Verdict+" RISK")
	p()
	p(indent(ui.Box.Render(head), 2))

	order := map[scan.Level]int{scan.Bad: 0, scan.Warn: 1, scan.Info: 2, scan.Good: 3}
	fs := append([]scan.Finding(nil), r.Findings...)
	sort.SliceStable(fs, func(i, j int) bool { return order[fs[i].Level] < order[fs[j].Level] })
	p()
	for _, f := range fs {
		icon := map[scan.Level]string{scan.Bad: ui.Down.Render("✖"), scan.Warn: ui.Gold.Render("⚠"), scan.Good: ui.Up.Render("✔"), scan.Info: ui.Muted.Render("•")}[f.Level]
		pts := ""
		if f.Points != 0 {
			pts = ui.Muted.Render(fmt.Sprintf("  %+d", f.Points))
		}
		pf("  %s %s  %s%s\n", icon, padR(ui.Muted.Render(strings.ToUpper(f.Area)), 12), f.Text, pts)
	}
	if len(r.Missing) > 0 {
		p("\n  " + ui.Gold.Render("could not load: "+strings.Join(r.Missing, ", ")+" - the score uses what was available"))
	}
	p("\n  " + ui.Muted.Render("Heuristic triage, not financial advice. Drill in: ") + ui.Bold.Render("fomocli token "+ui.Short(r.Address)+" devs"))
}

func scanText(r scan.Result) string {
	icon := map[string]string{"LOW": "🟢", "MEDIUM": "🟡", "HIGH": "🔴"}[r.Verdict]
	var lines []string
	for _, f := range r.Findings {
		if f.Level == scan.Bad || f.Level == scan.Warn || f.Level == scan.Good {
			mark := map[scan.Level]string{scan.Bad: "✖", scan.Warn: "⚠", scan.Good: "✔"}[f.Level]
			lines = append(lines, mark+" "+f.Text)
		}
	}
	return fmt.Sprintf("%s Rug scan: %s risk (%d/100)\n%s\n%s\n— FomoCLI scan", icon, r.Verdict, r.Risk, r.Address, strings.Join(lines, "\n"))
}

// ================================================================ card

func cardCmd() *cobra.Command {
	var window, out string
	var noPositions bool
	var send []string
	cmd := &cobra.Command{
		Use:     "card <handle>",
		Aliases: []string{"share"},
		Short:   "Make a shareable PNG card of a trader's stats",
		Long: `Renders a 1200×675 image of a trader - PnL by window, win rate, volume,
best trade - in the FomoCLI style, ready to post on X or Telegram.

Costs a profile lookup (2,500 credits) plus one positions page (250) for the
win rate and best trade; --no-positions skips the latter.`,
		Example: `  fomocli card CryptoKaleo
  fomocli card frankdegods --window 7d -o frank.png
  fomocli card CryptoKaleo --send telegram`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			who := cleanHandle(args[0])
			if window == "" {
				window = "all"
			}
			if window != "24h" && window != "7d" && window != "30d" && window != "all" {
				return usagef("--window must be 24h, 7d, 30d or all")
			}
			d, err := dispatcher(send)
			if err != nil {
				return err
			}
			prof, err := fetch(cmd, "loading @"+who, func(ctx context.Context) (*api.Response, error) {
				if looksLikeID(who) {
					return client.UserByID(ctx, who)
				}
				return client.User(ctx, who)
			})
			if err != nil {
				return err
			}
			var pos jx.V
			spent := prof.CreditsCost
			last := prof
			if !noPositions {
				if res, err := fetch(cmd, "loading positions", func(ctx context.Context) (*api.Response, error) {
					return client.Positions(ctx, who, nil)
				}); err == nil {
					pos = res.Data
					last = res
					if res.CreditsCost > 0 {
						spent = max(spent, 0) + res.CreditsCost
					}
				} else {
					fmt.Fprintln(ui.Err, "  "+ui.BadgeW.Render("WARN")+" positions unavailable: "+err.Error())
				}
			}
			data := cardData(prof.Data, pos, window)
			img := card.Render(data)
			var buf bytes.Buffer
			if err := card.WritePNG(&buf, img); err != nil {
				return err
			}
			if out == "" {
				out = strings.ToLower(data.Handle) + "-fomocli.png"
			}
			if err := os.WriteFile(out, buf.Bytes(), 0o644); err != nil {
				return err
			}
			abs, _ := filepath.Abs(out)
			if g.json {
				p(jx.Wrap(map[string]any{"file": abs, "width": card.W, "height": card.H}).JSON())
			} else {
				p("\n  " + ui.Badge.Render("CARD") + "  " + ui.Bold.Render(abs))
				p("  " + ui.Muted.Render(fmt.Sprintf("%d×%d PNG · @%s · PnL %s %s", card.W, card.H, data.Handle, data.Headline.Label, card.SignedUSD(data.Headline.PnL))))
			}
			if d != nil {
				caption := fmt.Sprintf("@%s on FOMO · PnL %s %s\n— made with FomoCLI", data.Handle, data.Headline.Label, card.SignedUSD(data.Headline.PnL))
				for _, s := range d.Senders {
					ps, ok := s.(notify.PhotoSender)
					if !ok {
						continue
					}
					sp := ui.Spin("sending to " + s.Name())
					err := ps.SendPhoto(cmd.Context(), caption, buf.Bytes())
					sp.Stop()
					if err != nil {
						p("  " + ui.Down.Render("✖ "+s.Name()+": "+err.Error()))
					} else if !g.json {
						p("  " + ui.Up.Render("✔ posted to "+s.Name()))
					}
				}
			}
			last.CreditsCost = spent
			footer(last)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&window, "window", "all", "headline PnL window: 24h | 7d | 30d | all")
	f.StringVarP(&out, "output", "O", "", "output file (default <handle>-fomocli.png)")
	f.BoolVar(&noPositions, "no-positions", false, "skip win rate / best trade (saves 250 credits)")
	f.StringSliceVar(&send, "send", nil, "also post the image to telegram, discord")
	return cmd
}

func cardData(v, pos jx.V, window string) card.Data {
	d := card.Data{
		Handle:      v.First("handle", "userHandle").Str(),
		DisplayName: v.Get("displayName").Str(),
		Verified:    v.Get("verified").Bool(),
		Demo:        g.demo,
		Footer:      "Data: FOMO API · " + time.Now().Format("2 Jan 2006"),
	}
	if strings.EqualFold(d.DisplayName, d.Handle) {
		d.DisplayName = ""
	}
	var sub []string
	if c := v.Get("clan", "name").Str(); c != "" {
		sub = append(sub, c+" clan")
	}
	if age := v.Get("accountAgeDays").Num(); age > 0 {
		sub = append(sub, fmt.Sprintf("%.0f days on FOMO", age))
	}
	d.Subtitle = strings.Join(sub, " · ")

	labels := map[string]string{"24h": "24 hours", "7d": "7 days", "30d": "30 days", "all": "all time"}
	pnl := v.Get("pnl")
	for _, w := range []string{"24h", "7d", "30d", "all"} {
		x := pnl.Get(w)
		if w == "all" && !x.Exists() {
			x = v.Get("pnlUsd")
		}
		f, ok := x.Float()
		win := card.Window{Label: strings.ToUpper(w), PnL: f, OK: ok}
		d.Windows = append(d.Windows, win)
		if w == window {
			d.Headline = card.Window{Label: labels[w], PnL: f, OK: ok}
		}
	}

	d.Stats = append(d.Stats, card.Stat{Label: "Volume", Value: card.CompactUSD(v.First("totalVolume", "volumeUsd").Num())})
	d.Stats = append(d.Stats, card.Stat{Label: "Trades", Value: ui.Int(v.First("numTrades", "trades"))})
	rows := pos.List("positions", "trades", "items")
	var closed, wins int
	var best jx.V
	for _, r := range rows {
		if r.Get("status").Str() == "open" {
			continue
		}
		closed++
		if r.Get("realizedPnlUsd").Num() > 0 {
			wins++
		}
		if best.IsNull() || r.Get("realizedPnlUsd").Num() > best.Get("realizedPnlUsd").Num() {
			best = r
		}
	}
	if closed > 0 {
		rate := float64(wins) / float64(closed) * 100
		tone := 0
		if rate >= 55 {
			tone = 1
		} else if rate < 40 {
			tone = -1
		}
		d.Stats = append(d.Stats, card.Stat{Label: fmt.Sprintf("Win rate (%d)", closed), Value: fmt.Sprintf("%.0f%%", rate), Tone: tone})
	}
	d.Stats = append(d.Stats, card.Stat{Label: "Followers", Value: ui.Compact(v.Get("followers").Num())})
	if !best.IsNull() && best.Get("realizedPnlUsd").Num() > 0 {
		d.Stats = append(d.Stats, card.Stat{Label: "Best trade", Value: "$" + best.Get("token", "symbol").Str() + " " + card.SignedUSD(best.Get("realizedPnlUsd").Num()), Tone: 1})
	} else if h := v.Get("holdings"); h.Exists() {
		d.Stats = append(d.Stats, card.Stat{Label: "Holdings", Value: ui.Int(h)})
	}
	return d
}
