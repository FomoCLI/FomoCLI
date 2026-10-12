// Package cli wires fomo's commands together.
package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/FomoCLI/FomoCLI/internal/api"
	"github.com/FomoCLI/FomoCLI/internal/cache"
	"github.com/FomoCLI/FomoCLI/internal/config"
	"github.com/FomoCLI/FomoCLI/internal/mock"
	"github.com/FomoCLI/FomoCLI/internal/ui"
)

// Build information, set via -ldflags at release time.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// Global flags.
type globals struct {
	json    bool
	offline bool
	fresh   bool
	noColor bool
	key     string
	baseURL string
	quiet   bool
	demo    bool
}

var (
	g      globals
	cfg    *config.Config
	client *api.Client
)

// Exit codes, so scripts can branch on failures.
const (
	ExitOK       = 0
	ExitError    = 1
	ExitUsage    = 2
	ExitAuth     = 3
	ExitCredits  = 4
	ExitNotFound = 5
	ExitOffline  = 6
)

// NewRoot builds the command tree.
func NewRoot() *cobra.Command {
	root := &cobra.Command{
		Use:   "fomo",
		Short: "fomo.family in your terminal - traders, PnL, tokens, theses and live trades",
		Long: ui.Banner(Version) + `
Track fomo.family traders from the command line: leaderboards, wallets, PnL,
positions, token flow, theses and a live trade feed - with an offline cache so
the last data you saw is always one command away, even without a connection.

Powered by FOMO API (https://fomoapi.io). Independent and unofficial; not
affiliated with fomo.family.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return setup()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return home()
		},
	}
	root.Version = fmt.Sprintf("%s (commit %s, built %s)", Version, Commit, Date)
	root.SetVersionTemplate("fomo {{.Version}}\n")

	pf := root.PersistentFlags()
	pf.BoolVarP(&g.json, "json", "j", false, "print raw JSON (pipe into jq)")
	pf.BoolVarP(&g.offline, "offline", "o", false, "never touch the network; serve from the local cache")
	pf.BoolVarP(&g.fresh, "fresh", "f", false, "skip the cache and fetch live data")
	pf.BoolVar(&g.noColor, "no-color", false, "disable colors")
	pf.BoolVarP(&g.quiet, "quiet", "q", false, "hide the credits/cache footer")
	pf.StringVar(&g.key, "key", "", "API key (overrides FOMO_API_KEY and config)")
	pf.BoolVar(&g.demo, "demo", false, "use built-in sample data (no key or internet needed)")
	pf.StringVar(&g.baseURL, "base-url", "", "API base URL")
	_ = pf.MarkHidden("base-url")

	root.AddGroup(
		&cobra.Group{ID: "market", Title: "Market:"},
		&cobra.Group{ID: "traders", Title: "Traders & trades:"},
		&cobra.Group{ID: "live", Title: "Realtime:"},
		&cobra.Group{ID: "setup", Title: "Setup & tools:"},
	)

	add := func(group string, cmds ...*cobra.Command) {
		for _, c := range cmds {
			c.GroupID = group
			root.AddCommand(c)
		}
	}
	add("market", leaderboardCmd(), tokensCmd(), tokenCmd(), activityCmd(), searchCmd(), thesesCmd())
	add("traders", traderCmd(), whoisCmd(), tradeCmd())
	add("live", liveCmd(), watchCmd(), alertsCmd(), dashCmd())
	add("setup", authCmd(), configCmd(), statusCmd(), creditsCmd(), cacheCmd(), apiCmd(), versionCmd())
	root.SetHelpCommandGroupID("setup")
	root.SetCompletionCommandGroupID("setup")
	return root
}

func setup() error {
	var err error
	cfg, err = config.Load()
	if err != nil {
		return err
	}
	ui.Setup(g.noColor || cfg.Theme == "none")

	key, _ := cfg.ResolvedKey()
	if g.key != "" {
		key = g.key
	}
	base := cfg.ResolvedBaseURL()
	if g.baseURL != "" {
		base = strings.TrimRight(g.baseURL, "/")
	}
	client = api.New(base, key)
	client.UserAgent = "fomo-cli/" + Version
	client.TTL = cfg.ResolvedTTL()
	if st, err := cache.Default(); err == nil {
		client.Cache = st
	}
	if g.demo {
		srv := mock.Start()
		client.BaseURL, client.Key = srv.URL, "demo"
		client.Cache = &cache.Store{Dir: filepath.Join(os.TempDir(), "fomo-demo-cache")}
		fmt.Fprintln(ui.Err, ui.BadgeW.Render("DEMO")+ui.Muted.Render(" sample data, not live - drop --demo for the real thing"))
	}
	switch {
	case g.offline || os.Getenv("FOMO_OFFLINE") == "1":
		client.Mode = api.ModeOffline
	case g.fresh:
		client.Mode = api.ModeFresh
	}
	return nil
}

// Execute runs the CLI and returns the process exit code.
func Execute() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	root := NewRoot()
	err := root.ExecuteContext(ctx)
	if err == nil {
		return ExitOK
	}
	if errors.Is(err, context.Canceled) {
		return 130
	}
	return reportError(err)
}

func reportError(err error) int {
	code := ExitError
	hint := ""
	var ae *api.APIError
	var ne *api.ErrNetwork
	switch {
	case errors.As(err, &ae):
		hint = ae.Hint()
		switch ae.Status {
		case 401, 403:
			code = ExitAuth
		case 402:
			code = ExitCredits
		case 404:
			code = ExitNotFound
		}
	case errors.Is(err, api.ErrNotCached):
		code = ExitOffline
		hint = "Run the same command once while online and it will be available offline afterwards."
	case errors.As(err, &ne):
		code = ExitOffline
		hint = "No cached copy of this request yet. Check your connection, or try --offline for data you have viewed before."
	case isUsage(err):
		code = ExitUsage
	}
	fmt.Fprintln(ui.Err, ui.BadgeE.Render("ERROR")+" "+err.Error())
	if hint != "" {
		fmt.Fprintln(ui.Err, ui.Muted.Render("      "+hint))
	}
	return code
}

type usageErr struct{ msg string }

func (e usageErr) Error() string { return e.msg }

func usagef(format string, a ...any) error { return usageErr{fmt.Sprintf(format, a...)} }

func isUsage(err error) bool {
	var u usageErr
	if errors.As(err, &u) {
		return true
	}
	s := err.Error()
	return strings.HasPrefix(s, "unknown command") || strings.HasPrefix(s, "unknown flag") ||
		strings.HasPrefix(s, "accepts ") || strings.HasPrefix(s, "requires ") ||
		strings.Contains(s, "invalid argument")
}
