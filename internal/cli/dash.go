package cli

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/FomoCLI/FomoCLI/internal/api"
	"github.com/FomoCLI/FomoCLI/internal/jx"
	"github.com/FomoCLI/FomoCLI/internal/ui"
)

func dashCmd() *cobra.Command {
	var chain string
	cmd := &cobra.Command{
		Use:     "dash",
		Aliases: []string{"ui", "tui", "d"},
		Short:   "Full-screen interactive dashboard",
		Long: `A full-screen terminal dashboard with four tabs:

  1 Leaderboard   top traders; enter = positions of the selected trader
  2 Tokens        trending / most-held / graduated boards
  3 Live          streaming app feed (0 credits)
  4 Theses        latest theses across all coins

Keys: 1-4 or tab switch · ↑/↓ j/k move · w window · b board · enter open ·
esc back · r refresh · q quit. Works with --offline using cached data.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if g.json {
				return usagef("dash is interactive; use the individual commands with --json")
			}
			if chain == "" {
				chain = cfg.DefaultChain
			}
			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()
			m := newDash(ctx, chain)
			prog := tea.NewProgram(m, tea.WithAltScreen(), tea.WithContext(ctx))
			if !g.offline {
				go streamToDash(ctx, prog, chain)
			}
			_, err := prog.Run()
			if err == tea.ErrProgramKilled {
				return nil
			}
			return err
		},
	}
	cmd.Flags().StringVarP(&chain, "chain", "c", "", "live tab: only this chain")
	return cmd
}

// ---------------------------------------------------------------- messages

type dataMsg struct {
	key string
	res *api.Response
	err error
}

type liveMsg struct{ v jx.V }
type liveStatusMsg string
type tickMsg time.Time

// ---------------------------------------------------------------- model

type tab int

const (
	tabLB tab = iota
	tabTokens
	tabLive
	tabTheses
)

var tabNames = []string{"Leaderboard", "Tokens", "Live", "Theses"}
var windows = []string{"24h", "7d", "30d", "all"}

type dash struct {
	ctx      context.Context
	chain    string
	w, h     int
	tab      tab
	window   int
	board    int
	cursor   [4]int
	data     map[string]*api.Response
	errs     map[string]error
	loading  map[string]bool
	live     []jx.V
	liveStat string
	detail   string // handle whose positions are open, "" = none
	credits  int64
	flash    string
}

func newDash(ctx context.Context, chain string) *dash {
	d := &dash{
		ctx: ctx, chain: chain,
		data: map[string]*api.Response{}, errs: map[string]error{}, loading: map[string]bool{},
		credits: -1, liveStat: "connecting…",
	}
	if lc := client.LastCredits(); lc != nil {
		d.credits = lc.Remaining
	}
	if g.offline {
		d.liveStat = "offline replay"
		for _, l := range readLines(newRecorder(false).path) {
			if v, err := jx.Parse([]byte(l)); err == nil && matchLocal(v, liveOpts{chain: chain}) {
				d.live = append([]jx.V{v}, d.live...)
			}
		}
		if len(d.live) > 300 {
			d.live = d.live[:300]
		}
	}
	return d
}

func (d *dash) Init() tea.Cmd {
	return tea.Batch(d.load(d.lbKey(), false), tick())
}

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (d *dash) lbKey() string     { return "lb:" + windows[d.window] }
func (d *dash) boardKey() string  { return "tokens:" + boards[d.board] }
func (d *dash) detailKey() string { return "pos:" + d.detail }

func (d *dash) currentKey() string {
	switch d.tab {
	case tabLB:
		if d.detail != "" {
			return d.detailKey()
		}
		return d.lbKey()
	case tabTokens:
		return d.boardKey()
	case tabTheses:
		return "theses"
	}
	return ""
}

// load fetches the data behind a key unless it is already present.
func (d *dash) load(key string, force bool) tea.Cmd {
	if key == "" || d.loading[key] || (!force && d.data[key] != nil) {
		return nil
	}
	d.loading[key] = true
	ctx := d.ctx
	c := *client
	if force && c.Mode != api.ModeOffline {
		c.Mode = api.ModeFresh
	}
	return func() tea.Msg {
		var res *api.Response
		var err error
		kind, arg, _ := strings.Cut(key, ":")
		switch kind {
		case "lb":
			res, err = c.Leaderboard(ctx, arg, 100)
		case "tokens":
			res, err = c.TokenBoard(ctx, arg, 50)
		case "theses":
			res, err = c.Theses(ctx, api.Params{"limit": "40"})
		case "pos":
			res, err = c.Positions(ctx, arg, nil)
		}
		return dataMsg{key: key, res: res, err: err}
	}
}

func streamToDash(ctx context.Context, prog *tea.Program, chain string) {
	q := url.Values{}
	set(q, "chain", chain)
	rec := newRecorder(false)
	defer rec.close()
	err := client.Stream(ctx, "/ws/alerts", q, func(ev api.StreamEvent) {
		switch {
		case ev.Err != nil:
			prog.Send(liveStatusMsg("reconnecting…"))
		case ev.Info != "":
			prog.Send(liveStatusMsg(ev.Info))
		default:
			typ := ev.Msg.Get("type").Str()
			if typ == "welcome" {
				if ev.Msg.Get("realtime").Bool() {
					prog.Send(liveStatusMsg("realtime"))
				} else {
					prog.Send(liveStatusMsg("delayed " + ev.Msg.Get("delaySeconds").Str() + "s"))
				}
				return
			}
			if typ == "alert" || typ == "trade" {
				rec.write(ev.Raw)
				prog.Send(liveMsg{ev.Msg})
			}
		}
	})
	if err != nil {
		prog.Send(liveStatusMsg("stream error: " + err.Error()))
	}
}

// ---------------------------------------------------------------- update

func (d *dash) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		d.w, d.h = m.Width, m.Height
	case tickMsg:
		return d, tick()
	case dataMsg:
		d.loading[m.key] = false
		if m.err != nil {
			d.errs[m.key] = m.err
		} else {
			delete(d.errs, m.key)
			d.data[m.key] = m.res
			if m.res.CreditsRemaining >= 0 {
				d.credits = m.res.CreditsRemaining
			}
		}
	case liveMsg:
		d.live = append([]jx.V{m.v}, d.live...)
		if len(d.live) > 300 {
			d.live = d.live[:300]
		}
		if d.tab == tabLive && d.cursor[tabLive] > 0 {
			d.cursor[tabLive]++ // keep the selected row steady
		}
	case liveStatusMsg:
		d.liveStat = string(m)
	case tea.KeyMsg:
		return d, d.key(m)
	}
	return d, nil
}

func (d *dash) key(k tea.KeyMsg) tea.Cmd {
	d.flash = ""
	switch k.String() {
	case "q", "ctrl+c":
		return tea.Quit
	case "esc", "backspace":
		d.detail = ""
	case "1", "2", "3", "4":
		d.tab = tab(k.String()[0] - '1')
		d.detail = ""
	case "tab", "right", "l":
		d.tab = (d.tab + 1) % 4
		d.detail = ""
	case "shift+tab", "left", "h":
		d.tab = (d.tab + 3) % 4
		d.detail = ""
	case "up", "k":
		if d.cursor[d.tab] > 0 {
			d.cursor[d.tab]--
		}
	case "down", "j":
		if d.cursor[d.tab] < d.rowCount()-1 {
			d.cursor[d.tab]++
		}
	case "pgdown", "ctrl+d":
		d.cursor[d.tab] = min(d.cursor[d.tab]+10, max(0, d.rowCount()-1))
	case "pgup", "ctrl+u":
		d.cursor[d.tab] = max(d.cursor[d.tab]-10, 0)
	case "g", "home":
		d.cursor[d.tab] = 0
	case "G", "end":
		d.cursor[d.tab] = max(0, d.rowCount()-1)
	case "w":
		if d.tab == tabLB {
			d.window = (d.window + 1) % len(windows)
			d.cursor[tabLB], d.detail = 0, ""
		}
	case "b":
		if d.tab == tabTokens {
			d.board = (d.board + 1) % len(boards)
			d.cursor[tabTokens] = 0
		}
	case "r":
		d.flash = "refreshing…"
		return d.load(d.currentKey(), true)
	case "enter":
		if d.tab == tabLB && d.detail == "" {
			if rows := d.rows(); d.cursor[tabLB] < len(rows) {
				r := rows[d.cursor[tabLB]]
				id := r.Get("userId").Str()
				if id == "" {
					id = r.Get("handle").Str()
				}
				d.detail = id
			}
		}
	}
	return d.load(d.currentKey(), false)
}

func (d *dash) rows() []jx.V {
	switch d.tab {
	case tabLive:
		return d.live
	}
	res := d.data[d.currentKey()]
	if res == nil {
		return nil
	}
	return res.Data.List("traders", "tokens", "theses", "positions", "trades", "items")
}

func (d *dash) rowCount() int { return len(d.rows()) }

// ---------------------------------------------------------------- view

var (
	tabOn  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#05060f")).Background(ui.ColIce).Padding(0, 1)
	tabOff = lipgloss.NewStyle().Foreground(ui.ColMuted).Padding(0, 1)
	selRow = lipgloss.NewStyle().Bold(true).Foreground(ui.ColIce).Background(lipgloss.AdaptiveColor{Light: "#dfe3f5", Dark: "#1f2438"})
)

func (d *dash) View() string {
	if d.w == 0 {
		return ""
	}
	// header
	var tabs []string
	for i, n := range tabNames {
		label := fmt.Sprintf("%d %s", i+1, n)
		if tab(i) == d.tab {
			tabs = append(tabs, tabOn.Render(label))
		} else {
			tabs = append(tabs, tabOff.Render(label))
		}
	}
	left := " " + ui.Logo() + "  " + strings.Join(tabs, " ")
	right := d.statusRight()
	gap := max(1, d.w-lipgloss.Width(left)-lipgloss.Width(right)-1)
	header := left + strings.Repeat(" ", gap) + right + " "

	bodyH := max(3, d.h-4)
	body := d.body(bodyH)

	help := ui.Muted.Render(" ↑↓ move · 1-4 tabs · " + d.contextHelp() + "r refresh · q quit")
	if d.flash != "" {
		help = " " + ui.Gold.Render(d.flash)
	}
	rule := ui.Muted.Render(strings.Repeat("─", d.w))
	return header + "\n" + rule + "\n" + body + "\n" + help
}

func (d *dash) statusRight() string {
	var parts []string
	if g.offline {
		parts = append(parts, ui.Gold.Render("OFFLINE"))
	}
	if d.credits >= 0 {
		parts = append(parts, ui.Gold.Render("⚡")+ui.Muted.Render(ui.Compact(float64(d.credits))+" credits"))
	}
	if res := d.data[d.currentKey()]; res != nil {
		parts = append(parts, ui.Muted.Render("updated "+ui.AgoT(res.FetchedAt)))
	}
	return strings.Join(parts, ui.Muted.Render(" · "))
}

func (d *dash) contextHelp() string {
	switch d.tab {
	case tabLB:
		if d.detail != "" {
			return "esc back · "
		}
		return "w window (" + windows[d.window] + ") · enter positions · "
	case tabTokens:
		return "b board (" + boards[d.board] + ") · "
	}
	return ""
}

func (d *dash) body(h int) string {
	key := d.currentKey()
	if d.tab != tabLive {
		if err := d.errs[key]; err != nil && d.data[key] == nil {
			msg := ui.Down.Render("  ✖ " + err.Error())
			if ae, ok := err.(*api.APIError); ok && ae.Hint() != "" {
				msg += "\n  " + ui.Muted.Render(ae.Hint())
			}
			return fill(msg, h)
		}
		if d.data[key] == nil {
			return fill(ui.Muted.Render("  loading…"), h)
		}
	}

	var head string
	var lines []string
	cur := d.cursor[d.tab]
	rows := d.rows()
	switch d.tab {
	case tabLB:
		if d.detail != "" {
			head, lines = d.positionsView(rows)
		} else {
			head, lines = d.leaderboardView(rows)
		}
	case tabTokens:
		head, lines = d.tokensView(rows)
	case tabLive:
		head = ui.Header.Render(fmt.Sprintf("  LIVE APP FEED · %s · %d events", d.liveStat, len(rows)))
		if d.chain != "" {
			head += ui.Muted.Render(" · chain " + d.chain)
		}
		for _, r := range rows {
			lines = append(lines, liveLine(r))
		}
		if len(rows) == 0 {
			lines = append(lines, ui.Muted.Render("  waiting for trades…"))
		}
	case tabTheses:
		head, lines = d.thesesView(rows)
	}

	// scroll so the cursor stays visible
	visible := h - 1
	start := 0
	if cur >= visible {
		start = cur - visible + 1
	}
	var out []string
	out = append(out, head)
	for i := start; i < len(lines) && len(out) < h; i++ {
		l := ui.Truncate(lines[i], d.w)
		if i == cur && len(rows) > 0 {
			// Render the selected row as plain text on one background: inner
			// color resets would otherwise punch holes in the highlight.
			l = selRow.Render(padR("▌"+strings.TrimPrefix(ui.Plain(l), " "), d.w))
		}
		out = append(out, l)
	}
	return fill(strings.Join(out, "\n"), h)
}

func fill(s string, h int) string {
	n := strings.Count(s, "\n") + 1
	if n < h {
		s += strings.Repeat("\n", h-n)
	}
	return s
}

func col(s string, w int, right bool) string {
	s = ui.Truncate(s, w)
	pad := strings.Repeat(" ", max(0, w-lipgloss.Width(s)))
	if right {
		return pad + s
	}
	return s + pad
}

func (d *dash) leaderboardView(rows []jx.V) (string, []string) {
	head := ui.Header.Render(fmt.Sprintf("  %4s  %-26s %11s %-14s %10s %7s %9s", "#", "TRADER · "+strings.ToUpper(windows[d.window]), "PNL", "", "VOLUME", "TRADES", "FOLLOWERS"))
	var maxP float64
	for _, r := range rows {
		maxP = max(maxP, abs(r.Get("pnlUsd").Num()))
	}
	var lines []string
	for i, r := range rows {
		rank := r.Get("rank").Int()
		if rank == 0 {
			rank = int64(i + 1)
		}
		name := traderName(r.Get("handle").Str(), r.Get("verified").Bool())
		lines = append(lines, "  "+col(ui.Rank(rank), 4, true)+"  "+col(name, 26, false)+" "+col(ui.Signed(r.Get("pnlUsd")), 11, true)+" "+
			col(ui.Bar(r.Get("pnlUsd").Num(), maxP, 14), 14, false)+" "+col(ui.USD(r.Get("volumeUsd")), 10, true)+" "+
			col(ui.Int(r.Get("trades")), 7, true)+" "+col(ui.Compact(r.Get("followers").Num()), 9, true))
	}
	return head, lines
}

func (d *dash) positionsView(rows []jx.V) (string, []string) {
	var realized, unrealized float64
	for _, r := range rows {
		realized += r.Get("realizedPnlUsd").Num()
		unrealized += r.Get("unrealizedPnlUsd").Num()
	}
	who := d.detail
	if res := d.data[d.lbKey()]; res != nil {
		for _, r := range res.Data.List("traders") {
			if r.Get("userId").Str() == d.detail {
				who = r.Get("handle").Str()
			}
		}
	}
	head := ui.Header.Render(fmt.Sprintf("  POSITIONS · @%s · %d rows · realized ", who, len(rows))) + ui.SignedF(realized) +
		ui.Header.Render(" · unrealized ") + ui.SignedF(unrealized)
	var lines []string
	for _, r := range rows {
		st := ui.Up.Render("●")
		exit := ui.Price(r.Get("priceUsd"))
		if r.Get("status").Str() != "open" {
			st = ui.Muted.Render("○")
			exit = ui.Price(r.Get("avgExitPrice"))
		}
		lines = append(lines, "  "+st+" "+col(tokenLabel(r.Get("token")), 12, false)+" "+col(ui.Chain(chainOf(r)), 10, false)+" "+
			col(ui.Price(r.Get("avgEntryPrice")), 12, true)+" → "+col(exit, 12, false)+" "+
			col(ui.Signed(r.Get("realizedPnlUsd")), 10, true)+" "+col(ui.Signed(r.Get("unrealizedPnlUsd")), 10, true)+"  "+
			ui.Muted.Render(ui.Ago(r.First("createdAt", "openedAt"))))
	}
	if len(rows) == 0 {
		lines = append(lines, ui.Muted.Render("  no positions available"))
	}
	return head, lines
}

func (d *dash) tokensView(rows []jx.V) (string, []string) {
	head := ui.Header.Render(fmt.Sprintf("  %3s  %-24s %13s %9s %9s %8s  %-10s", "#", "TOKEN · "+strings.ToUpper(boards[d.board]), "PRICE", "24H", "MCAP", "HOLDERS", "CHAIN"))
	var lines []string
	for i, r := range rows {
		tok := r.Get("token")
		holders := ui.Dash
		if h := r.Get("holders"); h.Exists() {
			holders = ui.Int(h)
		}
		name := tokenLabel(tok) + " " + ui.Muted.Render(tok.Get("name").Str())
		lines = append(lines, "  "+col(ui.Muted.Render(fmt.Sprint(i+1)), 3, true)+"  "+col(name, 24, false)+" "+
			col(ui.Price(r.Get("priceUsd")), 13, true)+" "+col(ui.Pct(r.Get("change24h")), 9, true)+" "+
			col(ui.USD(r.Get("marketCapUsd")), 9, true)+" "+col(holders, 8, true)+"  "+col(ui.Chain(ui.ChainName(r.Get("network").Str())), 10, false))
	}
	return head, lines
}

func (d *dash) thesesView(rows []jx.V) (string, []string) {
	head := ui.Header.Render(fmt.Sprintf("  LATEST THESES · %d", len(rows)))
	var lines []string
	for _, r := range rows {
		tok := r.Get("token")
		label := tokenLabel(tok)
		if !tok.Exists() {
			label = ui.Bold.Render("$" + r.First("symbol", "tokenSymbol").Str())
		}
		prefix := "  " + col(ui.Handle.Render("@"+r.First("handle", "trader").Str()), 18, false) + " " + col(label, 10, false) + " " +
			col(ui.USD(r.Get("equity")), 8, true) + "  "
		text := ui.OneLine(r.First("text", "thesis").Str())
		lines = append(lines, prefix+ui.Truncate(text, max(10, d.w-lipgloss.Width(prefix)-2)))
	}
	return head, lines
}
