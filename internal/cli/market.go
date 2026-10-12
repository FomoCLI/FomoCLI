package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/FomoCLI/FomoCLI/internal/api"
	"github.com/FomoCLI/FomoCLI/internal/config"
	"github.com/FomoCLI/FomoCLI/internal/jx"
	"github.com/FomoCLI/FomoCLI/internal/ui"
)

// ---------------------------------------------------------------- leaderboard

func leaderboardCmd() *cobra.Command {
	var limit int
	var sortBy string
	var verified bool
	cmd := &cobra.Command{
		Use:     "leaderboard [24h|7d|30d|all]",
		Aliases: []string{"lb", "top"},
		Short:   "Top traders ranked by PnL",
		Example: `  fomocli lb               # 24h board (or your default_window)
  fomocli lb 7d -n 20
  fomocli lb all --sort volume
  fomocli lb --json | jq '.traders[] | {handle, pnlUsd}'`,
		Args:      cobra.MaximumNArgs(1),
		ValidArgs: []string{"24h", "7d", "30d", "all"},
		RunE: func(cmd *cobra.Command, args []string) error {
			window := cfg.DefaultWindow
			if window == "" {
				window = "24h"
			}
			if len(args) == 1 {
				window = args[0]
			}
			if err := config.ValidWindow(window); err != nil {
				return usagef("%s", err)
			}
			return show(cmd, "loading "+window+" leaderboard", func(ctx context.Context) (*api.Response, error) {
				return client.Leaderboard(ctx, window, limit)
			}, func(v jx.V) { renderLeaderboard(v, window, sortBy, verified) })
		},
	}
	cmd.Flags().IntVarP(&limit, "limit", "n", 25, "rows to show (max 150)")
	cmd.Flags().StringVarP(&sortBy, "sort", "s", "pnl", "sort by pnl | volume | trades | followers")
	cmd.Flags().BoolVar(&verified, "verified", false, "only verified traders")
	return cmd
}

func renderLeaderboard(v jx.V, window, sortBy string, verifiedOnly bool) {
	rows := v.List("traders")
	if verifiedOnly {
		var f []jx.V
		for _, r := range rows {
			if r.Get("verified").Bool() {
				f = append(f, r)
			}
		}
		rows = f
	}
	field := map[string]string{"pnl": "pnlUsd", "volume": "volumeUsd", "trades": "trades", "followers": "followers"}[sortBy]
	if field != "" && sortBy != "pnl" {
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].Get(field).Num() > rows[j].Get(field).Num() })
	}

	var maxPnl, totalPnl, totalVol float64
	for _, r := range rows {
		pnl := r.Get("pnlUsd").Num()
		if a := abs(pnl); a > maxPnl {
			maxPnl = a
		}
		totalPnl += pnl
		totalVol += r.Get("volumeUsd").Num()
	}

	title := fmt.Sprintf("Leaderboard · %s", strings.ToUpper(window))
	p(ui.Section(title) + "  " + ui.Muted.Render(captured(v)))
	p()
	p(indent(ui.Tiles(
		ui.Stat("traders", fmt.Sprint(len(rows))),
		ui.Stat("combined pnl", ui.SignedF(totalPnl)),
		ui.Stat("combined volume", ui.USDf(totalVol)),
	), 2))
	p()

	t := ui.NewTable(
		ui.Col{Title: "#", Align: ui.Right},
		ui.Col{Title: "trader", Flex: true, Max: 34},
		ui.Col{Title: "pnl", Align: ui.Right},
		ui.Col{Title: "", Max: 14},
		ui.Col{Title: "volume", Align: ui.Right},
		ui.Col{Title: "trades", Align: ui.Right},
		ui.Col{Title: "followers", Align: ui.Right},
		ui.Col{Title: "holds", Align: ui.Right},
	)
	for i, r := range rows {
		rank := r.Get("rank").Int()
		if rank == 0 || sortBy != "pnl" {
			rank = int64(i + 1)
		}
		name := traderName(r.Get("handle").Str(), r.Get("verified").Bool())
		if dn := r.Get("displayName").Str(); dn != "" && !strings.EqualFold(dn, r.Get("handle").Str()) {
			name += " " + ui.Muted.Render(ui.Truncate(dn, 14))
		}
		t.Add(ui.Rank(rank), name, ui.Signed(r.Get("pnlUsd")), ui.Bar(r.Get("pnlUsd").Num(), maxPnl, 12),
			ui.USD(r.Get("volumeUsd")), ui.Int(r.Get("trades")), ui.Compact(r.Get("followers").Num()), ui.Int(r.Get("holdings")))
	}
	pf("%s", t.Render())
	p("\n  " + ui.Muted.Render("Drill in: ") + ui.Bold.Render("fomocli trader <handle>") + ui.Muted.Render("  ·  positions: ") + ui.Bold.Render("fomocli trader <handle> positions"))
}

func captured(v jx.V) string {
	if t := v.Get("capturedAt"); t.Exists() {
		return "captured " + ui.Ago(t)
	}
	return ""
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

// ---------------------------------------------------------------- token boards

var boards = []string{"trending", "most-held", "graduated"}

func tokensCmd() *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:     "tokens [trending|most-held|graduated]",
		Aliases: []string{"board", "boards"},
		Short:   "Token boards: what FOMO users are trading",
		Long: `FOMO's token boards, live (5-minute cache upstream):

  trending   what FOMO users are actively trading right now
  most-held  tokens the most users hold (dominated by large caps)
  graduated  early-stage graduated tokens (typically < $1M mcap)`,
		Example:   "  fomocli tokens\n  fomocli tokens graduated -n 10",
		Args:      cobra.MaximumNArgs(1),
		ValidArgs: boards,
		RunE: func(cmd *cobra.Command, args []string) error {
			board := "trending"
			if len(args) == 1 {
				board = args[0]
			}
			ok := false
			for _, b := range boards {
				ok = ok || b == board
			}
			if !ok {
				return usagef("unknown board %q (use %s)", board, strings.Join(boards, ", "))
			}
			return show(cmd, "loading "+board+" tokens", func(ctx context.Context) (*api.Response, error) {
				return client.TokenBoard(ctx, board, limit)
			}, func(v jx.V) { renderTokenBoard(v, board) })
		},
	}
	cmd.Flags().IntVarP(&limit, "limit", "n", 25, "rows to show")
	return cmd
}

func renderTokenBoard(v jx.V, board string) {
	src := v.Get("source").Str()
	meta := captured(v)
	if src != "" {
		meta += " · " + src
	}
	if v.Get("stale").Bool() {
		meta += " · " + ui.Gold.Render("stale")
	}
	p(ui.Section("Tokens · "+board) + "  " + ui.Muted.Render(meta))
	p()
	t := ui.NewTable(
		ui.Col{Title: "#", Align: ui.Right},
		ui.Col{Title: "token", Flex: true, Max: 30},
		ui.Col{Title: "price", Align: ui.Right},
		ui.Col{Title: "24h", Align: ui.Right},
		ui.Col{Title: "mcap", Align: ui.Right},
		ui.Col{Title: "holders", Align: ui.Right},
		ui.Col{Title: "chain"},
		ui.Col{Title: "address"},
	)
	for i, r := range v.List("tokens") {
		tok := r.Get("token")
		rank := r.Get("rank").Int()
		if rank == 0 {
			rank = int64(i + 1)
		}
		name := tokenLabel(tok)
		if n := tok.Get("name").Str(); n != "" && !strings.EqualFold(n, tok.Get("symbol").Str()) {
			name += " " + ui.Muted.Render(ui.Truncate(n, 16))
		}
		holders := ui.Muted.Render(ui.Dash)
		if h := r.Get("holders"); h.Exists() {
			holders = ui.Int(h)
		}
		chain := r.Get("network").Str()
		if chain == "" {
			chain = chainOf(r)
		}
		t.Add(ui.Muted.Render(fmt.Sprint(rank)), name, ui.Price(r.Get("priceUsd")), ui.Pct(r.Get("change24h")),
			ui.USD(r.Get("marketCapUsd")), holders, ui.Chain(ui.ChainName(chain)), ui.Muted.Render(ui.Short(tok.Get("address").Str())))
	}
	pf("%s", t.Render())
	p("\n  " + ui.Muted.Render("Analyze one: ") + ui.Bold.Render("fomocli token <address>"))
}

// ---------------------------------------------------------------- token

var tokenViews = []string{"stats", "devs", "holders", "theses"}

func tokenCmd() *cobra.Command {
	var network string
	var limit int
	var sortBy string
	var threshold int
	var pages int
	cmd := &cobra.Command{
		Use:   "token <address> [stats|devs|holders|theses]",
		Short: "Analyze a token: flow, devs, smart-money holders, theses",
		Long: `Look at one token from four angles:

  stats    buy/sell flow over 5m · 1h · 4h · 24h and top-10 concentration (default)
  devs     deployer & insider positions with their own thesis (rug signal)
  holders  which tracked traders hold it (smart money)
  theses   what traders wrote about it (1,250 credits per page)

Tip: find an address with 'fomocli search <symbol>' or 'fomocli tokens'.`,
		Example: `  fomocli token 0x39dbed3a...
  fomocli token Fu2oZoGxFtCDp29NKA4A89xcn255khq9xbxG7Mmtpump devs --network 1399811149
  fomocli token <mint> theses --sort likes --threshold 1000`,
		Args: cobra.RangeArgs(1, 2),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 1 {
				return tokenViews, cobra.ShellCompDirectiveNoFileComp
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			addr := strings.TrimSpace(args[0])
			view := "stats"
			if len(args) == 2 {
				view = args[1]
			}
			switch view {
			case "stats":
				return show(cmd, "reading token flow", func(ctx context.Context) (*api.Response, error) {
					return client.TokenStats(ctx, addr, network)
				}, func(v jx.V) { renderTokenStats(v, addr) })
			case "devs", "dev":
				return show(cmd, "finding devs", func(ctx context.Context) (*api.Response, error) {
					return client.TokenDevs(ctx, addr, network)
				}, func(v jx.V) { renderDevs(v, addr) })
			case "holders":
				return show(cmd, "finding smart-money holders", func(ctx context.Context) (*api.Response, error) {
					return client.TokenHolders(ctx, addr, limit)
				}, func(v jx.V) { renderHolders(v, addr) })
			case "theses", "thesis":
				params := api.Params{"limit": api.Itoa(limit), "sort": sortBy, "threshold": api.Itoa(threshold), "pages": api.Itoa(pages), "network": network}
				return show(cmd, "reading theses", func(ctx context.Context) (*api.Response, error) {
					return client.TokenTheses(ctx, addr, params)
				}, func(v jx.V) { renderTheses(v, "Theses on "+ui.Short(addr)) })
			}
			return usagef("unknown view %q (use %s)", view, strings.Join(tokenViews, ", "))
		},
	}
	f := cmd.Flags()
	f.StringVar(&network, "network", "", "network id for tokens outside the directory (4663 Robinhood, 1399811149 Solana)")
	f.IntVarP(&limit, "limit", "n", 0, "max rows")
	f.StringVar(&sortBy, "sort", "", "theses: likes | recent | pnl")
	f.IntVar(&threshold, "threshold", 0, "theses: minimum position size in USD")
	f.IntVar(&pages, "pages", 0, "theses: pages of history to walk (1-10, 25 per page)")
	return cmd
}

func renderTokenStats(v jx.V, addr string) {
	p(ui.Section("Token flow") + "  " + ui.Muted.Render(addr))
	p()
	p(indent(ui.Tiles(
		ui.Stat("holders", ui.Int(v.Get("holders"))),
		ui.Stat("top 10 hold", pctPlain(v.Get("top10HoldersPercent"))),
	), 2))
	p()
	t := ui.NewTable(
		ui.Col{Title: "window"}, ui.Col{Title: "buys", Align: ui.Right}, ui.Col{Title: "sells", Align: ui.Right},
		ui.Col{Title: "buyers", Align: ui.Right}, ui.Col{Title: "sellers", Align: ui.Right},
		ui.Col{Title: "buy vol", Align: ui.Right}, ui.Col{Title: "sell vol", Align: ui.Right},
		ui.Col{Title: "net flow", Align: ui.Right}, ui.Col{Title: "b/s", Align: ui.Right}, ui.Col{Title: "pressure"},
	)
	for _, w := range []string{"5m", "1h", "4h", "24h"} {
		x := v.Get("windows", w)
		if !x.Exists() {
			continue
		}
		ratio := ui.Muted.Render(ui.Dash)
		if f, ok := x.Get("buySellRatio").Float(); ok {
			ratio = fmt.Sprintf("%.2f", f)
		}
		t.Add(ui.Bold.Render(w), ui.Int(x.Get("buys")), ui.Int(x.Get("sells")), ui.Int(x.Get("uniqueBuyers")), ui.Int(x.Get("uniqueSellers")),
			ui.Up.Render(ui.USD(x.Get("buyVolumeUsd"))), ui.Down.Render(ui.USD(x.Get("sellVolumeUsd"))),
			ui.Signed(x.Get("netVolumeUsd")), ratio, pressure(x.Get("buyVolumeUsd").Num(), x.Get("sellVolumeUsd").Num(), 16))
	}
	pf("%s", t.Render())
	note(v)
}

// pressure renders a buy-vs-sell split bar.
func pressure(buy, sell float64, width int) string {
	total := buy + sell
	if total <= 0 {
		return ui.Muted.Render(strings.Repeat("·", width))
	}
	b := int(buy / total * float64(width))
	return ui.Up.Render(strings.Repeat("█", b)) + ui.Down.Render(strings.Repeat("█", width-b))
}

func pctPlain(v jx.V) string {
	f, ok := v.Float()
	if !ok {
		return ui.Dash
	}
	return fmt.Sprintf("%.1f%%", f)
}

func renderDevs(v jx.V, addr string) {
	p(ui.Section("Devs & insiders") + "  " + ui.Muted.Render(addr))
	rows := v.List("devs", "holders", "items")
	if len(rows) == 0 {
		p("\n  " + ui.Muted.Render("No dev holding known to FOMO. ") + ui.Gold.Render("That is not a clean bill of health - absence of a signal is not safety."))
		note(v)
		return
	}
	for _, r := range rows {
		head := "  " + ui.Handle.Render("@"+r.Get("handle").Str())
		if r.Get("isDev").Bool() {
			head += " " + ui.BadgeW.Render("DEV")
		}
		p("\n" + head)
		p(indent(ui.KV(
			[2]string{"holding", ui.Amount(r.Get("amount")) + "  " + ui.Muted.Render(ui.USD(r.Get("valueUsd")))},
			[2]string{"cost basis", ui.USD(r.Get("costBasisUsd"))},
			[2]string{"avg entry", ui.Price(r.Get("averageEntryPrice"))},
			[2]string{"realized", ui.Signed(r.Get("realizedPnlUsd"))},
			[2]string{"unrealized", ui.Signed(r.Get("unrealizedPnlUsd"))},
			[2]string{"solana", r.Get("wallet", "solana").Str()},
			[2]string{"evm", r.Get("wallet", "evm").Str()},
		), 4))
		if th := r.Get("thesis").Str(); th != "" {
			p(quote(th, 4))
		}
	}
	note(v)
}

func renderHolders(v jx.V, addr string) {
	p(ui.Section("Smart-money holders") + "  " + ui.Muted.Render(addr))
	p()
	rows := v.List("holders", "items")
	var maxV float64
	for _, r := range rows {
		if x := r.Get("valueUsd").Num(); x > maxV {
			maxV = x
		}
	}
	t := ui.NewTable(ui.Col{Title: "#", Align: ui.Right}, ui.Col{Title: "trader", Flex: true}, ui.Col{Title: "amount", Align: ui.Right},
		ui.Col{Title: "value", Align: ui.Right}, ui.Col{Title: ""}, ui.Col{Title: "price", Align: ui.Right})
	for i, r := range rows {
		t.Add(ui.Muted.Render(fmt.Sprint(i+1)), ui.Handle.Render("@"+r.Get("handle").Str()), ui.Amount(r.Get("amount")),
			ui.USD(r.Get("valueUsd")), ui.Bar(r.Get("valueUsd").Num(), maxV, 12), ui.Price(r.Get("priceUsd")))
	}
	pf("%s", t.Render())
}

// ---------------------------------------------------------------- theses

func thesesCmd() *cobra.Command {
	var chain, sortBy string
	var minEquity, limit int
	cmd := &cobra.Command{
		Use:     "theses",
		Aliases: []string{"thesis", "why"},
		Short:   "Recent theses - the reasoning behind trades",
		Long: `The global thesis feed across all coins. For a single coin use
'fomocli token <address> theses', for one trader 'fomocli trader <handle> theses'.`,
		Example: "  fomocli theses\n  fomocli theses --chain robinhood\n  fomocli theses --sort equity --min-equity 100000",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if chain == "" {
				chain = cfg.DefaultChain
			}
			params := api.Params{"chain": chain, "sort": sortBy, "minEquity": api.Itoa(minEquity), "limit": api.Itoa(limit)}
			return show(cmd, "reading theses", func(ctx context.Context) (*api.Response, error) {
				return client.Theses(ctx, params)
			}, func(v jx.V) { renderTheses(v, "Latest theses") })
		},
	}
	cmd.Flags().StringVarP(&chain, "chain", "c", "", "robinhood | solana | base | bsc | eth | arc | <chain id>")
	cmd.Flags().StringVarP(&sortBy, "sort", "s", "", "recent | equity | pnl")
	cmd.Flags().IntVar(&minEquity, "min-equity", 0, "only theses backed by a position of at least this many USD")
	cmd.Flags().IntVarP(&limit, "limit", "n", 20, "max theses")
	return cmd
}

func renderTheses(v jx.V, title string) {
	rows := v.List("theses", "items", "data")
	meta := []string{fmt.Sprintf("%d shown", len(rows))}
	if n := v.Get("totalAvailable"); n.Exists() {
		meta = append(meta, ui.Int(n)+" on FOMO")
	}
	if v.Get("stale").Bool() {
		meta = append(meta, ui.Gold.Render("stale snapshot"))
	}
	p(ui.Section(title) + "  " + ui.Muted.Render(strings.Join(meta, " · ")))
	for _, r := range rows {
		head := "  " + ui.Handle.Render("@"+r.First("handle", "trader").Str())
		if r.Get("isDev").Bool() {
			head += " " + ui.BadgeW.Render("DEV")
		}
		tok := r.Get("token")
		if tok.Exists() {
			head += "  " + tokenLabel(tok)
		} else if s := r.First("symbol", "tokenSymbol").Str(); s != "" {
			head += "  " + ui.Bold.Render("$"+s)
		}
		var bits []string
		if c := chainOf(r); c != "" {
			bits = append(bits, ui.Chain(c))
		}
		if e := r.Get("equity"); e.Exists() {
			bits = append(bits, "pos "+ui.USD(e))
		}
		if pn := r.First("pnlUsd", "pnl"); pn.Exists() && !pn.IsObject() {
			bits = append(bits, ui.Signed(pn))
		}
		if l := r.Get("likes"); l.Exists() {
			bits = append(bits, ui.Pink.Render("♥ "+ui.Int(l)))
		}
		if rp := r.Get("replies"); rp.Num() > 0 {
			bits = append(bits, ui.Muted.Render("↩ "+ui.Int(rp)))
		}
		if ts := r.First("ts", "createdAt"); ts.Exists() {
			bits = append(bits, ui.Muted.Render(ui.Ago(ts)))
		}
		p("\n" + head + "  " + strings.Join(bits, ui.Muted.Render(" · ")))
		p(quote(r.First("text", "thesis").Str(), 4))
	}
	if len(rows) == 0 {
		p("\n  " + ui.Muted.Render("No theses yet."))
	}
	note(v)
}

// quote renders wrapped text with a left rule.
func quote(s string, pad int) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	w := ui.Width() - pad - 4
	if w > 96 {
		w = 96
	}
	body := lipgloss.NewStyle().Width(w).Render(s)
	var out []string
	for _, l := range strings.Split(body, "\n") {
		out = append(out, strings.Repeat(" ", pad)+ui.Accent.Render("│ ")+strings.TrimRight(l, " "))
	}
	return strings.Join(out, "\n")
}

// ---------------------------------------------------------------- activity

func activityCmd() *cobra.Command {
	var side string
	var minTraders, limit int
	cmd := &cobra.Command{
		Use:     "activity",
		Aliases: []string{"swarm", "multi"},
		Short:   "Coordinated flow: several traders hitting one coin at once",
		Example: "  fomocli activity\n  fomocli activity --side buy --min-traders 3",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			params := api.Params{"side": side, "minTraders": api.Itoa(minTraders), "limit": api.Itoa(limit)}
			return show(cmd, "reading multi-trader activity", func(ctx context.Context) (*api.Response, error) {
				return client.TokenActivity(ctx, params)
			}, renderActivity)
		},
	}
	cmd.Flags().StringVar(&side, "side", "", "buy | sell")
	cmd.Flags().IntVar(&minTraders, "min-traders", 0, "minimum distinct traders")
	cmd.Flags().IntVarP(&limit, "limit", "n", 25, "max events (up to 100)")
	return cmd
}

func renderActivity(v jx.V) {
	meta := ""
	if a := v.Get("newestEventAgeHours"); a.Exists() {
		meta = fmt.Sprintf("newest event %.1fh ago", a.Num())
	}
	if v.Get("stale").Bool() {
		meta += " · " + ui.Gold.Render("stale")
	}
	p(ui.Section("Multi-trader activity") + "  " + ui.Muted.Render(meta))
	p()
	t := ui.NewTable(ui.Col{Title: "side"}, ui.Col{Title: "token"}, ui.Col{Title: "traders", Align: ui.Right}, ui.Col{Title: "trades", Align: ui.Right},
		ui.Col{Title: "window", Align: ui.Right}, ui.Col{Title: "volume", Align: ui.Right}, ui.Col{Title: "price", Align: ui.Right},
		ui.Col{Title: "who", Flex: true, Max: 48})
	for _, r := range v.List("events", "activity", "items") {
		var who []string
		for _, tr := range r.Get("traders").Array() {
			who = append(who, "@"+tr.Get("handle").Str())
		}
		tok := r.Get("token")
		label := tokenLabel(tok)
		if !tok.Exists() {
			label = ui.Bold.Render("$" + r.First("symbol", "tokenSymbol").Str())
		}
		if r.Get("topTradersInvolved").Bool() {
			label += " " + ui.Gold.Render("★")
		}
		t.Add(ui.Side(r.Get("side").Str()), label, ui.Int(r.Get("uniqueTraders")), ui.Int(r.Get("trades")),
			ui.Int(r.Get("windowMinutes"))+"m", ui.USD(r.Get("volumeUsd")), ui.Pct(r.Get("priceChangePercent")),
			ui.Cyan.Render(strings.Join(who, " ")))
	}
	pf("%s", t.Render())
	note(v)
}

// ---------------------------------------------------------------- search

func searchCmd() *cobra.Command {
	var typ string
	var limit int
	var tokensOnly bool
	cmd := &cobra.Command{
		Use:     "search <query>",
		Aliases: []string{"s", "find"},
		Short:   "Find traders and tokens by name",
		Example: "  fomocli search ansem\n  fomocli search pons --tokens\n  fomocli search ether --type traders",
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := strings.Join(args, " ")
			if tokensOnly {
				return show(cmd, "searching tokens", func(ctx context.Context) (*api.Response, error) {
					return client.TokenSearch(ctx, q, limit)
				}, func(v jx.V) { renderSearch(v, q) })
			}
			return show(cmd, "searching", func(ctx context.Context) (*api.Response, error) {
				return client.Search(ctx, q, typ, limit)
			}, func(v jx.V) { renderSearch(v, q) })
		},
	}
	cmd.Flags().StringVarP(&typ, "type", "t", "", "traders | tokens (default both)")
	cmd.Flags().BoolVar(&tokensOnly, "tokens", false, "token-only search endpoint")
	cmd.Flags().IntVarP(&limit, "limit", "n", 0, "max results")
	return cmd
}

func renderSearch(v jx.V, q string) {
	all := v.List("results", "items", "tokens", "traders")
	var traders, tokens []jx.V
	for _, r := range all {
		switch r.Get("type").Str() {
		case "trader":
			traders = append(traders, r)
		case "token":
			tokens = append(tokens, r)
		default:
			if r.Get("handle").Exists() {
				traders = append(traders, r)
			} else {
				tokens = append(tokens, r)
			}
		}
	}
	// Some shapes split them into two arrays.
	if len(all) == 0 {
		traders, tokens = v.Get("traders").Array(), v.Get("tokens").Array()
	}
	p(ui.Section(fmt.Sprintf("Search · %q", q)))
	if len(traders) > 0 {
		p("\n  " + ui.Heading.Render("Traders"))
		t := ui.NewTable(ui.Col{Title: "trader", Flex: true}, ui.Col{Title: "pnl", Align: ui.Right}, ui.Col{Title: "volume", Align: ui.Right},
			ui.Col{Title: "followers", Align: ui.Right}, ui.Col{Title: "solana"}, ui.Col{Title: "evm"})
		for _, r := range traders {
			name := traderName(r.Get("handle").Str(), r.Get("verified").Bool())
			if dn := r.Get("displayName").Str(); dn != "" {
				name += " " + ui.Muted.Render(ui.Truncate(dn, 16))
			}
			t.Add(name, ui.Signed(r.Get("pnlUsd")), ui.USD(r.Get("volumeUsd")), ui.Compact(r.Get("followers").Num()),
				ui.Muted.Render(ui.Short(r.Get("wallets", "solana").Str())), ui.Muted.Render(ui.Short(r.Get("wallets", "evm").Str())))
		}
		pf("%s", t.Render())
	}
	if len(tokens) > 0 {
		p("\n  " + ui.Heading.Render("Tokens"))
		t := ui.NewTable(ui.Col{Title: "token", Flex: true}, ui.Col{Title: "mcap", Align: ui.Right}, ui.Col{Title: "address"})
		for _, r := range tokens {
			sym := r.First("symbol").Str()
			name := ui.Bold.Render("$" + sym)
			if n := r.Get("name").Str(); n != "" && !strings.EqualFold(n, sym) {
				name += " " + ui.Muted.Render(ui.Truncate(n, 20))
			}
			t.Add(name, ui.USD(r.Get("marketCapUsd")), r.First("address", "mint").Str())
		}
		pf("%s", t.Render())
	}
	if len(traders)+len(tokens) == 0 {
		p("\n  " + ui.Muted.Render("No matches. Search matches handles, display names, symbols and token names."))
	}
}
