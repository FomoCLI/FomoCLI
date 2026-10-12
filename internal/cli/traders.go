package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/FomoCLI/FomoCLI/internal/api"
	"github.com/FomoCLI/FomoCLI/internal/jx"
	"github.com/FomoCLI/FomoCLI/internal/ui"
)

var traderViews = []string{"profile", "positions", "swaps", "balances", "following", "followers", "spotlight", "theses"}

type traderOpts struct {
	limit  int
	chain  string
	deep   int
	all    bool
	pages  int
	status string
	sortBy string
	cursor string
}

func traderCmd() *cobra.Command {
	var o traderOpts
	cmd := &cobra.Command{
		Use:     "trader <handle|userId> [view]",
		Aliases: []string{"t", "user", "u"},
		Short:   "Everything about one trader: profile, positions, portfolio, network",
		Long: `Inspect a fomo.family trader. Views:

  profile    wallets, PnL by window, stats, account age   (default · 2,500 credits)
  positions  open & closed positions with entry/exit/PnL   (250 credits/page)
  swaps      individual fills
  balances   multi-chain portfolio with allocation
  following  who they watch - annotated with each account's PnL & volume
  followers  who watches them (FOMO caps this at 200)
  spotlight  FOMO's own pick of their best trades and theses
  theses     everything they wrote, newest first

The handle is case-insensitive and may start with @. A userId works everywhere
except 'profile' (it is resolved via 'fomocli whois' automatically).`,
		Example: `  fomocli trader CryptoKaleo
  fomocli t @frankdegods positions --status open
  fomocli t frankdegods positions --all --pages 4      # walk older history
  fomocli t frankdegods balances --chain robinhood
  fomocli t frankdegods following --sort pnl
  fomocli t frankdegods theses --sort likes`,
		Args: cobra.RangeArgs(1, 2),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 1 {
				return traderViews, cobra.ShellCompDirectiveNoFileComp
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			who := cleanHandle(args[0])
			view := "profile"
			if len(args) == 2 {
				view = strings.ToLower(args[1])
			}
			return runTrader(cmd, who, view, o)
		},
	}
	f := cmd.Flags()
	f.IntVarP(&o.limit, "limit", "n", 0, "max rows")
	f.StringVarP(&o.chain, "chain", "c", "", "balances/theses: robinhood | solana | base | bsc | eth | monad | arc | hyperliquid")
	f.IntVar(&o.deep, "deep", 0, "positions: fan out across chains & sort orders (1 = full, N = N calls)")
	f.BoolVarP(&o.all, "all", "a", false, "positions/swaps: walk older pages with the cursor")
	f.IntVar(&o.pages, "pages", 4, "with --all: max pages to walk (250 credits each)")
	f.StringVar(&o.status, "status", "", "positions: open | closed")
	f.StringVarP(&o.sortBy, "sort", "s", "", "positions: pnl | recent | value · following/followers: pnl | volume | followers | trades · theses: likes | recent")
	f.StringVar(&o.cursor, "cursor", "", "positions/swaps: start from this cursor")
	return cmd
}

func runTrader(cmd *cobra.Command, who, view string, o traderOpts) error {
	switch view {
	case "profile", "p", "info":
		if looksLikeID(who) {
			return show(cmd, "resolving trader", func(ctx context.Context) (*api.Response, error) {
				return client.UserByID(ctx, who)
			}, renderProfile)
		}
		return show(cmd, "resolving @"+who, func(ctx context.Context) (*api.Response, error) {
			return client.User(ctx, who)
		}, renderProfile)
	case "positions", "pos", "trades":
		return traderPaged(cmd, who, "positions", o)
	case "swaps", "fills":
		return traderPaged(cmd, who, "swaps", o)
	case "balances", "bal", "portfolio", "holdings":
		chain := o.chain
		if chain == "" {
			chain = cfg.DefaultChain
		}
		return show(cmd, "loading portfolio", func(ctx context.Context) (*api.Response, error) {
			return client.Balances(ctx, who, chain)
		}, func(v jx.V) { renderBalances(v, who) })
	case "following":
		return show(cmd, "walking following list", func(ctx context.Context) (*api.Response, error) {
			return client.Following(ctx, who, o.limit)
		}, func(v jx.V) { renderPeople(v, "@"+who+" follows", o.sortBy) })
	case "followers":
		return show(cmd, "loading followers", func(ctx context.Context) (*api.Response, error) {
			return client.Followers(ctx, who, o.limit)
		}, func(v jx.V) { renderPeople(v, "Followers of @"+who, o.sortBy) })
	case "spotlight", "best":
		return show(cmd, "loading spotlight", func(ctx context.Context) (*api.Response, error) {
			return client.Spotlight(ctx, who)
		}, func(v jx.V) { renderSpotlight(v, who) })
	case "theses", "thesis":
		params := api.Params{"sort": o.sortBy, "chain": o.chain, "limit": api.Itoa(o.limit)}
		return show(cmd, "reading theses", func(ctx context.Context) (*api.Response, error) {
			return client.UserTheses(ctx, who, "", params)
		}, func(v jx.V) { renderTheses(v, "Theses by @"+who) })
	}
	return usagef("unknown view %q (use %s)", view, strings.Join(traderViews, ", "))
}

// ---------------------------------------------------------------- profile

func renderProfile(v jx.V) {
	handle := v.First("handle", "userHandle").Str()
	head := traderName(handle, v.Get("verified").Bool())
	if dn := v.Get("displayName").Str(); dn != "" {
		head += "  " + ui.Bold.Render(dn)
	}
	var tags []string
	if c := v.Get("clan", "name").Str(); c != "" {
		tags = append(tags, ui.Accent.Render("⚑ "+c))
	}
	if v.Get("private").Bool() {
		tags = append(tags, ui.Gold.Render("private"))
	}
	if v.Get("isRestricted").Bool() {
		tags = append(tags, ui.Down.Render("restricted"))
	}
	if age := v.First("accountAgeDays").Num(); age > 0 {
		tag := fmt.Sprintf("%.0fd on FOMO", age)
		if age < 30 {
			tags = append(tags, ui.Gold.Render("🆕 "+tag))
		} else {
			tags = append(tags, ui.Muted.Render(tag))
		}
	}
	lines := []string{head}
	if len(tags) > 0 {
		lines = append(lines, strings.Join(tags, ui.Muted.Render(" · ")))
	}
	if bio := ui.OneLine(v.Get("description").Str()); bio != "" {
		lines = append(lines, ui.Muted.Render(ui.Truncate(bio, 80)))
	}
	p()
	p(indent(ui.Box.Render(strings.Join(lines, "\n")), 2))

	// PnL by window with a mini sparkline.
	pnl := v.Get("pnl")
	var series []float64
	var tiles []string
	for _, w := range []string{"24h", "7d", "30d", "all"} {
		x := pnl.Get(w)
		if x.Exists() {
			series = append(series, x.Num())
		}
		tiles = append(tiles, ui.Stat("pnl "+w, ui.Signed(x)))
	}
	if !pnl.Exists() {
		tiles = []string{ui.Stat("pnl", ui.Signed(v.Get("pnlUsd")))}
	}
	p()
	p(indent(ui.Tiles(tiles...), 2))
	p()
	hold := ui.Dash
	if s := v.First("averageHoldTimeSeconds").Num(); s > 0 {
		hold = ui.Dur(time.Duration(s) * time.Second)
	}
	p(indent(ui.Tiles(
		ui.Stat("volume", ui.USD(v.First("totalVolume", "volumeUsd"))),
		ui.Stat("trades", ui.Int(v.First("numTrades", "trades"))),
		ui.Stat("swaps", ui.Int(v.Get("swapCount"))),
		ui.Stat("holdings", ui.Int(v.Get("holdings"))),
		ui.Stat("followers", ui.Int(v.First("followers"))),
		ui.Stat("following", ui.Int(v.Get("following"))),
		ui.Stat("avg hold", hold),
	), 2))
	if len(series) > 1 {
		p("\n  " + ui.Muted.Render("pnl 24h→all ") + ui.Accent.Render(ui.Spark(series)))
	}

	p(ui.Section("Wallets"))
	p()
	p(indent(ui.KV(
		[2]string{"solana", ui.Bold.Render(v.Get("wallets", "solana").Str())},
		[2]string{"evm", ui.Bold.Render(v.Get("wallets", "evm").Str())},
		[2]string{"userId", ui.Muted.Render(v.First("userId").Str())},
	), 2))
	if v.Get("wallets").Len() == 0 {
		p("  " + ui.Muted.Render("no wallets resolved"))
	}

	if tt := v.Get("topTokens").Array(); len(tt) > 0 {
		var s []string
		for _, t := range tt {
			if t.IsObject() {
				s = append(s, tokenLabel(t))
			} else {
				s = append(s, ui.Muted.Render(ui.Short(t.Str())))
			}
		}
		p(ui.Section("Top tokens"))
		p()
		p("  " + strings.Join(s, "  "))
	}

	who := handle
	if who == "" {
		who = v.First("userId").Str()
	}
	p("\n  " + ui.Muted.Render("Next: ") + strings.Join([]string{
		ui.Bold.Render("fomocli t " + who + " positions"),
		ui.Bold.Render("balances"), ui.Bold.Render("following"), ui.Bold.Render("spotlight"),
	}, ui.Muted.Render(" · ")) + ui.Muted.Render("  ·  live: ") + ui.Bold.Render("fomocli watch "+who))
}

// ---------------------------------------------------------------- positions / swaps

func traderPaged(cmd *cobra.Command, who, kind string, o traderOpts) error {
	fetchPage := func(ctx context.Context, cursor string) (*api.Response, error) {
		params := api.Params{"limit": api.Itoa(o.limit), "cursor": cursor}
		if kind == "positions" {
			params["deep"] = api.Itoa(o.deep)
			return client.Positions(ctx, who, params)
		}
		return client.Swaps(ctx, who, params)
	}
	listKeys := []string{"positions", "trades", "swaps", "items", "data"}

	if !o.all {
		return show(cmd, "loading "+kind, func(ctx context.Context) (*api.Response, error) {
			return fetchPage(ctx, o.cursor)
		}, func(v jx.V) { renderTradeList(v, v.List(listKeys...), who, kind, o) })
	}

	// Walk pages with the cursor, merging rows and de-duplicating by id.
	cursor := o.cursor
	if cursor == "" {
		cursor = "start"
	}
	var merged []any
	seen := map[string]bool{}
	var last *api.Response
	var spent int64
	for page := 1; page <= o.pages && cursor != ""; page++ {
		res, err := fetch(cmd, fmt.Sprintf("loading %s page %d/%d", kind, page, o.pages), func(ctx context.Context) (*api.Response, error) {
			return fetchPage(ctx, cursor)
		})
		if err != nil {
			if last == nil {
				return err
			}
			fmt.Fprintln(ui.Err, ui.BadgeW.Render("WARN")+" stopped at page "+fmt.Sprint(page)+": "+err.Error())
			break
		}
		last = res
		if res.CreditsCost > 0 {
			spent += res.CreditsCost
		}
		for _, r := range res.Data.List(listKeys...) {
			id := r.First("tradeId", "swapId", "id", "txHash").Str()
			if id != "" && seen[id] {
				continue
			}
			seen[id] = true
			merged = append(merged, r.Raw())
		}
		cursor = res.Data.Get("nextCursor").Str()
	}
	if last == nil {
		return nil
	}
	rows := jx.Wrap(merged).Array()
	if g.json {
		p(jx.Wrap(map[string]any{kind: merged, "count": len(merged), "nextCursor": cursor}).JSON())
	} else {
		renderTradeList(last.Data, rows, who, kind, o)
		if cursor != "" {
			p("\n  " + ui.Muted.Render("more history: ") + ui.Bold.Render(fmt.Sprintf("fomocli t %s %s --all --cursor %s", who, kind, cursor)))
		}
	}
	if spent > 0 {
		last.CreditsCost = spent
	}
	footer(last)
	return nil
}

func renderTradeList(v jx.V, rows []jx.V, who, kind string, o traderOpts) {
	if o.status != "" {
		var f []jx.V
		for _, r := range rows {
			if strings.EqualFold(r.Get("status").Str(), o.status) {
				f = append(f, r)
			}
		}
		rows = f
	}
	switch o.sortBy {
	case "pnl":
		sort.SliceStable(rows, func(i, j int) bool { return totalPnl(rows[i]) > totalPnl(rows[j]) })
	case "value":
		sort.SliceStable(rows, func(i, j int) bool { return posValue(rows[i]) > posValue(rows[j]) })
	case "", "recent":
		sort.SliceStable(rows, func(i, j int) bool { return rowTime(rows[i]).After(rowTime(rows[j])) })
	}
	if kind == "swaps" {
		renderSwaps(rows, who)
		return
	}

	var open, closed, wins int
	var realized, unrealized float64
	for _, r := range rows {
		realized += r.Get("realizedPnlUsd").Num()
		unrealized += r.Get("unrealizedPnlUsd").Num()
		if r.Get("status").Str() == "open" {
			open++
		} else {
			closed++
			if r.Get("realizedPnlUsd").Num() > 0 {
				wins++
			}
		}
	}
	winRate := ui.Dash
	if closed > 0 {
		winRate = fmt.Sprintf("%.0f%%", float64(wins)/float64(closed)*100)
	}
	p(ui.Section("Positions · @" + who))
	p()
	p(indent(ui.Tiles(
		ui.Stat("open", fmt.Sprint(open)),
		ui.Stat("closed", fmt.Sprint(closed)),
		ui.Stat("win rate", winRate),
		ui.Stat("realized", ui.SignedF(realized)),
		ui.Stat("unrealized", ui.SignedF(unrealized)),
	), 2))
	if n := v.Get("closedTotalOnFomo"); n.Exists() {
		p("  " + ui.Muted.Render(fmt.Sprintf("holding %d of %s closed trades FOMO reports", closed, ui.Int(n))))
	}
	p()
	t := ui.NewTable(
		ui.Col{Title: ""}, ui.Col{Title: "token", Max: 14}, ui.Col{Title: "chain"},
		ui.Col{Title: "amount", Align: ui.Right}, ui.Col{Title: "entry", Align: ui.Right}, ui.Col{Title: "exit/now", Align: ui.Right},
		ui.Col{Title: "cost", Align: ui.Right}, ui.Col{Title: "realized", Align: ui.Right}, ui.Col{Title: "unrealized", Align: ui.Right},
		ui.Col{Title: "opened", Align: ui.Right}, ui.Col{Title: "trade id"},
	)
	for _, r := range rows {
		st := ui.Up.Render("●")
		exit := ui.Price(r.Get("priceUsd"))
		if r.Get("status").Str() != "open" {
			st = ui.Muted.Render("○")
			exit = ui.Price(r.Get("avgExitPrice"))
		}
		entry := ui.Price(r.Get("avgEntryPrice"))
		if !r.Get("avgEntryPrice").Exists() && r.Get("transferredInAmount").Num() > 0 {
			entry = ui.Muted.Render("recv'd")
		}
		t.Add(st, tokenLabel(r.Get("token")), ui.Chain(chainOf(r)), ui.Amount(r.Get("amount")), entry, exit,
			ui.USD(r.Get("costBasisUsd")), ui.Signed(r.Get("realizedPnlUsd")), ui.Signed(r.Get("unrealizedPnlUsd")),
			ui.Muted.Render(ui.Ago(r.First("createdAt", "openedAt"))), ui.Muted.Render(shortID(r.Get("tradeId").Str())))
	}
	pf("%s", t.Render())
	if v.Get("partial").Bool() {
		p("\n  " + ui.Gold.Render("partial: some chains did not answer in time - ask again for the rest"))
	}
	note(v)
	p("\n  " + ui.Muted.Render("● open  ○ closed  ·  detail: ") + ui.Bold.Render("fomocli trade <trade id>"))
}

func renderSwaps(rows []jx.V, who string) {
	p(ui.Section("Swaps · @" + who))
	p()
	t := ui.NewTable(ui.Col{Title: "side"}, ui.Col{Title: "token", Max: 14}, ui.Col{Title: "chain"},
		ui.Col{Title: "amount", Align: ui.Right}, ui.Col{Title: "usd", Align: ui.Right}, ui.Col{Title: "price", Align: ui.Right},
		ui.Col{Title: "when", Align: ui.Right}, ui.Col{Title: "tx"})
	for _, r := range rows {
		side := r.First("side", "type", "direction").Str()
		t.Add(ui.Side(side), tokenLabel(r.Get("token")), ui.Chain(chainOf(r)), ui.Amount(r.First("amount", "amountToken")),
			ui.USD(r.First("usdValue", "valueUsd", "amountUsd")), ui.Price(r.First("priceUsd", "price")),
			ui.Muted.Render(ui.Ago(r.First("ts", "timestamp", "createdAt", "blockTs"))), ui.Muted.Render(ui.Short(r.First("txHash", "signature").Str())))
	}
	pf("%s", t.Render())
}

func totalPnl(r jx.V) float64 { return r.Get("realizedPnlUsd").Num() + r.Get("unrealizedPnlUsd").Num() }
func posValue(r jx.V) float64 { return r.Get("amount").Num() * r.Get("priceUsd").Num() }

func rowTime(r jx.V) time.Time {
	for _, k := range []string{"closedAt", "createdAt", "openedAt", "ts", "timestamp"} {
		if t, ok := ui.Time(r.Get(k)); ok {
			return t
		}
	}
	return time.Time{}
}

func shortID(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}

// ---------------------------------------------------------------- balances

func renderBalances(v jx.V, who string) {
	rows := v.List("holdings", "balances", "items")
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Get("valueUsd").Num() > rows[j].Get("valueUsd").Num() })
	total := v.Get("totalValueUsd").Num()
	if total == 0 {
		for _, r := range rows {
			total += r.Get("valueUsd").Num()
		}
	}
	p(ui.Section("Portfolio · @" + who))
	p()
	tiles := []string{ui.Stat("total value", ui.USDf(total)), ui.Stat("positions", fmt.Sprint(len(rows)))}
	if x := v.Get("livePerpPnl"); x.Exists() {
		val := x
		if x.IsObject() {
			val = x.First("totalPnl", "pnl", "unrealizedPnl", "total")
		}
		tiles = append(tiles, ui.Stat("perp pnl", ui.Signed(val)))
	}
	p(indent(ui.Tiles(tiles...), 2))

	if bc := v.Get("byChain"); bc.Len() > 0 {
		type kv struct {
			k string
			v float64
			n int64
		}
		var chains []kv
		for _, k := range bc.Keys() {
			chains = append(chains, kv{k, bc.Get(k, "valueUsd").Num(), bc.Get(k, "holdings").Int()})
		}
		sort.Slice(chains, func(i, j int) bool { return chains[i].v > chains[j].v })
		p()
		for _, c := range chains {
			share := 0.0
			if total > 0 {
				share = c.v / total * 100
			}
			unit := "tokens"
			if c.n == 1 {
				unit = "token"
			}
			pf("  %s %s %s  %s\n", padR(ui.Chain(c.k), 12), padR(ui.Bar(c.v, total, 24), 25),
				lipgloss.NewStyle().Width(8).Align(lipgloss.Right).Render(ui.USDf(c.v)), ui.Muted.Render(fmt.Sprintf("%5.1f%% · %d %s", share, c.n, unit)))
		}
	}
	p()
	t := ui.NewTable(ui.Col{Title: "token", Max: 16}, ui.Col{Title: "chain"}, ui.Col{Title: "amount", Align: ui.Right},
		ui.Col{Title: "price", Align: ui.Right}, ui.Col{Title: "value", Align: ui.Right}, ui.Col{Title: "24h", Align: ui.Right},
		ui.Col{Title: "alloc", Align: ui.Right}, ui.Col{Title: ""})
	for _, r := range rows {
		val := r.Get("valueUsd").Num()
		alloc := ui.Dash
		if total > 0 {
			alloc = fmt.Sprintf("%.1f%%", val/total*100)
		}
		t.Add(tokenLabel(r.Get("token")), ui.Chain(chainOf(r)), ui.Amount(r.Get("amount")), ui.Price(r.Get("priceUsd")),
			ui.USD(r.Get("valueUsd")), ui.Pct(r.Get("change24h")), alloc, ui.Bar(val, total, 10))
	}
	pf("%s", t.Render())
	note(v)
}

// ---------------------------------------------------------------- following / followers

func renderPeople(v jx.V, title, sortBy string) {
	rows := v.List("following", "followers", "users", "items")
	field := map[string]string{"pnl": "pnl24h", "volume": "volumeUsd", "followers": "followers", "trades": "trades"}[sortBy]
	if field != "" {
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].Get(field).Num() > rows[j].Get(field).Num() })
	}
	var meta []string
	meta = append(meta, fmt.Sprintf("%d people", len(rows)))
	if v.Get("truncated").Bool() || v.Get("sourceCapped").Bool() {
		meta = append(meta, ui.Gold.Render("capped by FOMO"))
	}
	if v.Get("complete").Bool() {
		meta = append(meta, ui.Up.Render("complete"))
	}
	p(ui.Section(title) + "  " + ui.Muted.Render(strings.Join(meta, " · ")))
	p()
	t := ui.NewTable(ui.Col{Title: "trader", Flex: true, Max: 32}, ui.Col{Title: "pnl 24h", Align: ui.Right}, ui.Col{Title: "volume", Align: ui.Right},
		ui.Col{Title: "trades", Align: ui.Right}, ui.Col{Title: "followers", Align: ui.Right}, ui.Col{Title: "age", Align: ui.Right}, ui.Col{Title: "x"})
	for _, r := range rows {
		name := traderName(r.Get("handle").Str(), r.Get("verified").Bool())
		if r.Get("private").Bool() {
			name += ui.Muted.Render(" 🔒")
		}
		age := ui.Dash
		if d := r.Get("accountAgeDays").Num(); d > 0 {
			age = fmt.Sprintf("%.0fd", d)
		}
		x := r.Get("twitter").Str()
		if x != "" && !strings.HasPrefix(x, "@") && !strings.Contains(x, "/") {
			x = "@" + x
		}
		t.Add(name, ui.Signed(r.Get("pnl24h")), ui.USD(r.Get("volumeUsd")), ui.Int(r.Get("trades")),
			ui.Compact(r.Get("followers").Num()), ui.Muted.Render(age), ui.Muted.Render(x))
	}
	pf("%s", t.Render())
	if v.Get("partial").Bool() {
		p("\n  " + ui.Gold.Render("partial list - run again in a few seconds and the walk resumes where it stopped"))
	}
	note(v)
}

// ---------------------------------------------------------------- spotlight

func renderSpotlight(v jx.V, who string) {
	section := func(title string, rows []jx.V) {
		p(ui.Section(title))
		if len(rows) == 0 {
			p("  " + ui.Muted.Render("none"))
			return
		}
		for _, r := range rows {
			state := ui.Muted.Render("closed " + ui.Ago(r.Get("closedAt")))
			if !r.Get("closedAt").Exists() {
				state = ui.Up.Render("open")
			}
			pf("\n  %s  %s  entry %s → %s  realized %s  unrealized %s  %s\n", tokenLabel(r.Get("token")), ui.Chain(chainOf(r)),
				ui.Price(r.Get("avgEntryPrice")), ui.Price(r.Get("avgExitPrice")), ui.Signed(r.Get("realizedPnlUsd")),
				ui.Signed(r.Get("unrealizedPnlUsd")), state)
			if th := r.Get("thesis").Str(); th != "" {
				p(quote(th, 4))
				if l := r.Get("thesisLikes"); l.Num() > 0 {
					p("    " + ui.Pink.Render("♥ "+ui.Int(l)))
				}
			}
			if id := r.Get("tradeId").Str(); id != "" {
				p("    " + ui.Muted.Render("fomocli trade "+id))
			}
		}
	}
	p(ui.Section("Spotlight · @"+who) + "  " + ui.Muted.Render("FOMO's own picks"))
	section("Best trades", v.Get("bestTrades").Array())
	section("Best theses", v.Get("bestTheses").Array())
}

// ---------------------------------------------------------------- whois

func whoisCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "whois <userId>",
		Short:   "Turn a FOMO userId back into a trader (2,500 credits)",
		Example: "  fomocli whois 1f08e6ab-5c73-5443-9225-bfc496cde51f",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return show(cmd, "resolving userId", func(ctx context.Context) (*api.Response, error) {
				return client.UserByID(ctx, args[0])
			}, renderProfile)
		},
	}
}

// ---------------------------------------------------------------- trade

func tradeCmd() *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "trade <tradeId> [comments]",
		Short: "One trade in full: swaps, entry/exit, PnL, thesis thread",
		Example: `  fomocli trade a323b5ca-c769-4b07-a422-833c4cafbe1f
  fomocli trade a323b5ca-c769-4b07-a422-833c4cafbe1f comments`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			if len(args) == 2 {
				if args[1] != "comments" && args[1] != "thread" {
					return usagef("unknown view %q (use comments)", args[1])
				}
				return show(cmd, "loading thread", func(ctx context.Context) (*api.Response, error) {
					return client.TradeComments(ctx, id, limit)
				}, renderComments)
			}
			return show(cmd, "loading trade", func(ctx context.Context) (*api.Response, error) {
				return client.Trade(ctx, id)
			}, renderTrade)
		},
	}
	cmd.Flags().IntVarP(&limit, "limit", "n", 0, "comments: max (up to 200)")
	return cmd
}

func renderTrade(v jx.V) {
	head := tokenLabel(v.Get("token")) + "  " + ui.Muted.Render("by ") + ui.Handle.Render("@"+v.First("trader", "handle").Str())
	if v.Get("isDev").Bool() {
		head += " " + ui.BadgeW.Render("DEV")
	}
	if st := v.Get("status").Str(); st != "" {
		head += "  " + ui.Muted.Render(st)
	}
	p()
	p(indent(ui.Box.Render(head+"\n"+ui.Muted.Render(v.First("tradeId").Str())), 2))
	p()
	p(indent(ui.Tiles(
		ui.Stat("avg entry", ui.Price(v.Get("avgEntryPrice"))),
		ui.Stat("avg exit", ui.Price(v.Get("avgExitPrice"))),
		ui.Stat("realized", ui.Signed(v.Get("realizedPnlUsd"))),
		ui.Stat("unrealized", ui.Signed(v.Get("unrealizedPnlUsd"))),
		ui.Stat("bought", ui.Amount(v.Get("boughtAmount"))),
		ui.Stat("received", ui.Amount(v.Get("transferredInAmount"))),
	), 2))
	if pr := v.Get("priceUsd"); pr.Exists() {
		p("\n  " + ui.Muted.Render("now ") + ui.Price(pr) + ui.Muted.Render("  liquidity ") + ui.USD(v.First("liquidityUsd", "liquidity")))
	}
	if th := v.Get("thesis"); th.Exists() {
		text := th.Str()
		if th.IsObject() {
			text = th.First("text", "thesis").Str()
		}
		p(ui.Section("Thesis"))
		p()
		p(quote(text, 2))
	}
	if sw := v.Get("swaps").Array(); len(sw) > 0 {
		p(ui.Section(fmt.Sprintf("Swaps (%d)", len(sw))))
		p()
		renderLegs(sw)
	}
	if tr := v.Get("transfers").Array(); len(tr) > 0 {
		p(ui.Section(fmt.Sprintf("Transfers (%d)", len(tr))))
		p()
		renderLegs(tr)
	}
	p("\n  " + ui.Muted.Render("thread: ") + ui.Bold.Render("fomocli trade "+v.First("tradeId").Str()+" comments"))
}

func renderLegs(rows []jx.V) {
	t := ui.NewTable(ui.Col{Title: "side"}, ui.Col{Title: "amount", Align: ui.Right}, ui.Col{Title: "usd", Align: ui.Right},
		ui.Col{Title: "price", Align: ui.Right}, ui.Col{Title: "when", Align: ui.Right}, ui.Col{Title: "tx"})
	for _, r := range rows {
		t.Add(ui.Side(r.First("side", "type", "direction").Str()), ui.Amount(r.First("amount", "tokenAmount")),
			ui.USD(r.First("usdValue", "valueUsd", "amountUsd")), ui.Price(r.First("priceUsd", "price")),
			ui.Muted.Render(ui.Ago(r.First("ts", "timestamp", "createdAt"))), ui.Muted.Render(ui.Short(r.First("txHash", "signature", "hash").Str())))
	}
	pf("%s", t.Render())
}

func renderComments(v jx.V) {
	rows := v.List("comments", "items")
	p(ui.Section(fmt.Sprintf("Thesis thread · %d comments", len(rows))))
	if o, n := v.Get("olderThesisCount").Num(), v.Get("newerThesisCount").Num(); o+n > 0 {
		p("  " + ui.Gold.Render(fmt.Sprintf("trader wrote %.0f earlier and %.0f later theses on this position", o, n)))
	}
	children := map[string][]jx.V{}
	var roots []jx.V
	for _, r := range rows {
		if pid := r.Get("parentId").Str(); pid != "" {
			children[pid] = append(children[pid], r)
		} else {
			roots = append(roots, r)
		}
	}
	var walk func(r jx.V, depth int)
	walk = func(r jx.V, depth int) {
		pad := 2 + depth*4
		head := strings.Repeat(" ", pad) + ui.Muted.Render(shortID(r.Get("userId").Str())) + "  " + ui.Pink.Render("♥ "+ui.Int(r.Get("likes"))) + "  " + ui.Muted.Render(ui.Ago(r.First("ts", "createdAt")))
		p("\n" + head)
		p(quote(r.Get("text").Str(), pad))
		for _, c := range children[r.Get("id").Str()] {
			walk(c, depth+1)
		}
	}
	for _, r := range roots {
		walk(r, 0)
	}
	if len(roots) == 0 {
		for _, r := range rows {
			walk(r, 0)
		}
	}
	p("\n  " + ui.Muted.Render("authors are userIds - resolve with ") + ui.Bold.Render("fomocli whois <userId>"))
}
