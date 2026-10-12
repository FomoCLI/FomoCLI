package cli

import (
	"bufio"
	"context"
	"fmt"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/FomoCLI/FomoCLI/internal/api"
	"github.com/FomoCLI/FomoCLI/internal/config"
	"github.com/FomoCLI/FomoCLI/internal/jx"
	"github.com/FomoCLI/FomoCLI/internal/ui"
)

func maskKey(k string) string { return config.Mask(k) }

// ---------------------------------------------------------------- auth

func authCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Log in with your FOMO API key",
	}

	var keyFlag string
	var skipVerify bool
	login := &cobra.Command{
		Use:   "login",
		Short: "Save an API key (get a free one at fomoapi.io/dashboard)",
		Example: `  fomo auth login
  fomo auth login --with-key sk_live_xxx
  echo $KEY | fomo auth login --with-key -`,
		RunE: func(cmd *cobra.Command, args []string) error {
			key := strings.TrimSpace(keyFlag)
			if key == "-" {
				line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
				key = strings.TrimSpace(line)
			}
			if key == "" {
				p(ui.Banner(Version))
				p("  Get a free key in seconds at " + ui.Cyan.Render("https://fomoapi.io/dashboard") + ui.Muted.Render(" (email code, no password)"))
				fmt.Fprint(ui.Err, "\n  "+ui.Accent.Render("›")+" Paste your API key: ")
				if ui.IsTTY(os.Stdin) {
					b, err := term.ReadPassword(int(os.Stdin.Fd()))
					fmt.Fprintln(ui.Err)
					if err != nil {
						return err
					}
					key = strings.TrimSpace(string(b))
				} else {
					line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
					key = strings.TrimSpace(line)
				}
			}
			if key == "" {
				return usagef("no key given")
			}

			if !skipVerify {
				c := *client
				c.Key = key
				c.Mode = api.ModeFresh
				c.Cache = nil
				sp := ui.Spin("verifying key")
				// /v2/me reports the balance; fall back to the cheapest keyed
				// call (/v2/alerts?limit=1, 125 credits) where it is missing.
				res, err := c.Me(cmd.Context())
				if ae, ok := err.(*api.APIError); ok && ae.Status == 404 {
					res, err = c.Alerts(cmd.Context(), api.Params{"limit": "1"})
				}
				sp.Stop()
				if err != nil {
					if ae, ok := err.(*api.APIError); ok && (ae.Status == 401 || ae.Status == 403) {
						return fmt.Errorf("key rejected by the API: %s", ae.Error())
					}
					fmt.Fprintln(ui.Err, ui.BadgeW.Render("WARN")+" could not verify key ("+err.Error()+"); saving anyway")
				} else if res.CreditsRemaining >= 0 {
					fmt.Fprintln(ui.Err, "  "+ui.Muted.Render("credits left: ")+ui.Bold.Render(ui.Commas(float64(res.CreditsRemaining), 0)))
				}
			}

			cfg.APIKey = key
			if err := cfg.Save(); err != nil {
				return err
			}
			path, _ := config.Path()
			p("  " + ui.Badge.Render("LOGGED IN") + "  key " + ui.Muted.Render(maskKey(key)) + " saved to " + ui.Muted.Render(path))
			p("  Next: " + ui.Bold.Render("fomo lb") + ui.Muted.Render("  or  ") + ui.Bold.Render("fomo dash"))
			return nil
		},
	}
	login.Flags().StringVar(&keyFlag, "with-key", "", "key to save ('-' reads stdin)")
	login.Flags().BoolVar(&skipVerify, "no-verify", false, "save without a test call")

	status := &cobra.Command{
		Use:   "status",
		Short: "Show which key is active",
		RunE: func(cmd *cobra.Command, args []string) error {
			key, src := cfg.ResolvedKey()
			if g.key != "" {
				key, src = g.key, "--key flag"
			}
			if key == "" {
				p("  " + ui.BadgeW.Render("LOGGED OUT") + "  run " + ui.Bold.Render("fomo auth login"))
				return nil
			}
			path, _ := config.Path()
			pairs := [][2]string{
				{"key", maskKey(key)},
				{"source", src},
				{"config", path},
				{"api", client.BaseURL},
			}
			if lc := client.LastCredits(); lc != nil {
				pairs = append(pairs, [2]string{"credits", ui.Commas(float64(lc.Remaining), 0) + ui.Muted.Render(" (as of "+ui.AgoT(lc.At)+")")})
			}
			p("  " + ui.Badge.Render("LOGGED IN"))
			p(indent(ui.KV(pairs...), 2))
			return nil
		},
	}

	logout := &cobra.Command{
		Use:   "logout",
		Short: "Remove the saved key",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.APIKey = ""
			if err := cfg.Save(); err != nil {
				return err
			}
			p("  " + ui.Muted.Render("Key removed from config."))
			if os.Getenv("FOMO_API_KEY") != "" {
				p("  " + ui.Gold.Render("Note: FOMO_API_KEY is still set in your environment."))
			}
			return nil
		},
	}
	cmd.AddCommand(login, status, logout)
	return cmd
}

// ---------------------------------------------------------------- config

func configCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "config", Short: "View or change settings"}
	list := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "Show all settings",
		RunE: func(cmd *cobra.Command, args []string) error {
			var pairs [][2]string
			for _, k := range config.SortedKeys() {
				v, _ := cfg.Get(k)
				if k == "api_key" && v != "" {
					v = maskKey(v)
				}
				if v == "" {
					v = ui.Muted.Render("(default)")
				}
				pairs = append(pairs, [2]string{k, v + "  " + ui.Muted.Render("# "+config.Keys[k])})
			}
			p(indent(ui.KV(pairs...), 2))
			return nil
		},
	}
	get := &cobra.Command{
		Use:       "get <key>",
		Short:     "Print one setting",
		Args:      cobra.ExactArgs(1),
		ValidArgs: config.SortedKeys(),
		RunE: func(cmd *cobra.Command, args []string) error {
			v, err := cfg.Get(args[0])
			if err != nil {
				return usagef("%s", err)
			}
			p(v)
			return nil
		},
	}
	set := &cobra.Command{
		Use:       "set <key> <value>",
		Short:     "Change one setting",
		Example:   "  fomo config set cache_ttl 5m\n  fomo config set default_window 7d",
		Args:      cobra.ExactArgs(2),
		ValidArgs: config.SortedKeys(),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := cfg.Set(args[0], args[1]); err != nil {
				return usagef("%s", err)
			}
			if err := cfg.Save(); err != nil {
				return err
			}
			p("  " + ui.Up.Render("✔") + " " + args[0] + " updated")
			return nil
		},
	}
	path := &cobra.Command{
		Use:   "path",
		Short: "Print the config file location",
		RunE: func(cmd *cobra.Command, args []string) error {
			pth, err := config.Path()
			if err != nil {
				return err
			}
			p(pth)
			return nil
		},
	}
	cmd.AddCommand(list, get, set, path)
	return cmd
}

// ---------------------------------------------------------------- status

func statusCmd() *cobra.Command {
	var watch bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Is FOMO API healthy? (keyless, free)",
		RunE: func(cmd *cobra.Command, args []string) error {
			once := func() error {
				c := *client
				if !g.offline {
					c.Mode = api.ModeFresh
				}
				return show(cmd, "checking status", func(ctx context.Context) (*api.Response, error) {
					return c.Status(ctx)
				}, renderStatus)
			}
			if !watch {
				return once()
			}
			for {
				fmt.Fprint(ui.Out, "\x1b[H\x1b[2J")
				if err := once(); err != nil {
					fmt.Fprintln(ui.Err, ui.Down.Render(err.Error()))
				}
				p(ui.Muted.Render("\n  refreshing every 15s · ctrl+c to quit"))
				select {
				case <-cmd.Context().Done():
					return nil
				case <-time.After(15 * time.Second):
				}
			}
		},
	}
	cmd.Flags().BoolVarP(&watch, "watch", "w", false, "refresh every 15 seconds")
	return cmd
}

func statusDot(s string) string {
	switch s {
	case "operational":
		return ui.Up.Render("●") + " " + s
	case "degraded":
		return ui.Gold.Render("●") + " " + s
	case "down":
		return ui.Down.Render("●") + " " + s
	}
	return ui.Muted.Render("○ " + s)
}

func renderStatus(v jx.V) {
	overall := v.Get("status").Str()
	badge := ui.Badge
	switch overall {
	case "degraded":
		badge = ui.BadgeW
	case "down":
		badge = ui.BadgeE
	}
	p("\n  " + ui.Logo() + "  " + badge.Render(strings.ToUpper(overall)) + "  " + ui.Muted.Render("updated "+ui.Ago(v.Get("updatedAt"))))
	t := ui.NewTable(ui.Col{Title: "component"}, ui.Col{Title: "status"}, ui.Col{Title: "detail"})
	names := map[string]string{"api": "REST API", "liveData": "Live trader data", "alertStream": "App feed stream", "onchainStream": "On-chain stream"}
	for _, k := range []string{"api", "liveData", "alertStream", "onchainStream"} {
		c := v.Get("components", k)
		if !c.Exists() {
			continue
		}
		var detail []string
		if f, ok := c.Get("successRate").Float(); ok {
			detail = append(detail, fmt.Sprintf("%.0f%% success (5m)", f*100))
		}
		if f, ok := c.Get("lastEventSecondsAgo").Float(); ok {
			detail = append(detail, "last event "+ui.Dur(time.Duration(f)*time.Second)+" ago")
		}
		if pl := c.Get("plans").Array(); len(pl) > 0 {
			var s []string
			for _, x := range pl {
				s = append(s, x.Str())
			}
			detail = append(detail, "plans: "+strings.Join(s, ", "))
		}
		t.Add(names[k], statusDot(c.Get("status").Str()), ui.Muted.Render(strings.Join(detail, " · ")))
	}
	p()
	pf("%s", t.Render())
	note(v)
}

// ---------------------------------------------------------------- credits

func creditsCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "credits",
		Aliases: []string{"me", "balance"},
		Short:   "Show your plan and credit balance",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := *client
			if !g.offline {
				c.Mode = api.ModeFresh
			}
			res, err := fetch(cmd, "checking credits", func(ctx context.Context) (*api.Response, error) { return c.Me(ctx) })
			if err != nil {
				// /v2/me is not in every deployment; fall back to the last header we saw.
				if lc := client.LastCredits(); lc != nil {
					if g.json {
						p(jx.Wrap(map[string]any{"remaining": lc.Remaining, "at": lc.At}).JSON())
						return nil
					}
					p("  " + ui.Stat("credits left", ui.Commas(float64(lc.Remaining), 0)))
					p("  " + ui.Muted.Render("last seen "+ui.AgoT(lc.At)+" (from response headers)"))
					return nil
				}
				return err
			}
			emit(res, func(v jx.V) {
				var tiles []string
				if s := v.First("plan", "tier").Str(); s != "" {
					tiles = append(tiles, ui.Stat("plan", strings.ToUpper(s)))
				}
				if r := v.First("remaining", "creditsRemaining", "credits"); r.Exists() {
					if r.IsObject() {
						r = r.First("remaining", "total")
					}
					tiles = append(tiles, ui.Stat("credits left", ui.Int(r)))
				} else if res.CreditsRemaining >= 0 {
					tiles = append(tiles, ui.Stat("credits left", ui.Commas(float64(res.CreditsRemaining), 0)))
				}
				if s := v.First("renewsAt", "expiresAt", "resetAt").Str(); s != "" {
					tiles = append(tiles, ui.Stat("renews", s[:min(10, len(s))]))
				}
				p()
				if len(tiles) == 0 {
					p(res.Data.JSON())
					return
				}
				p(indent(ui.Tiles(tiles...), 2))
				p("\n  " + ui.Muted.Render("Need more? https://fomoapi.io/pricing"))
			})
			return nil
		},
	}
}

// ---------------------------------------------------------------- cache

func cacheCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cache",
		Short: "Manage the offline cache",
		Long: `Every response fomo receives is saved locally. Within the cache TTL
(default 60s, see 'fomo config set cache_ttl') repeated commands are served
from disk for 0 credits; with --offline any command shows the last data seen.`,
	}
	ls := &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list"},
		Short:   "List cached responses (what works offline)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if client.Cache == nil {
				return fmt.Errorf("cache unavailable")
			}
			items, err := client.Cache.List()
			if err != nil {
				return err
			}
			if g.json {
				var out []map[string]any
				for _, it := range items {
					out = append(out, map[string]any{"path": it.Path, "fetchedAt": it.FetchedAt, "bytes": it.Size})
				}
				p(jx.Wrap(out).JSON())
				return nil
			}
			t := ui.NewTable(ui.Col{Title: "request", Flex: true, Max: 80}, ui.Col{Title: "age", Align: ui.Right}, ui.Col{Title: "size", Align: ui.Right})
			var total int64
			for _, it := range items {
				total += it.Size
				t.Add(it.Path, ui.Dur(time.Since(it.FetchedAt)), ui.Compact(float64(it.Size))+"B")
			}
			p(ui.Section(fmt.Sprintf("Offline cache · %d entries · %sB", len(items), ui.Compact(float64(total)))))
			p()
			pf("%s", t.Render())
			p("\n  " + ui.Muted.Render(client.Cache.Dir))
			return nil
		},
	}
	var older string
	clear := &cobra.Command{
		Use:   "clear",
		Short: "Delete cached responses",
		RunE: func(cmd *cobra.Command, args []string) error {
			var d time.Duration
			if older != "" {
				var err error
				if d, err = config.ParseTTL(older); err != nil {
					return usagef("%s", err)
				}
			}
			n, err := client.Cache.Clear(d)
			if err != nil {
				return err
			}
			p("  " + ui.Up.Render("✔") + fmt.Sprintf(" removed %d cached responses", n))
			return nil
		},
	}
	clear.Flags().StringVar(&older, "older-than", "", "only remove entries older than this, e.g. 12h or 7d")
	path := &cobra.Command{
		Use:   "path",
		Short: "Print the cache directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			p(client.Cache.Dir)
			return nil
		},
	}
	cmd.AddCommand(ls, clear, path)
	return cmd
}

// ---------------------------------------------------------------- api

func apiCmd() *cobra.Command {
	var params []string
	cmd := &cobra.Command{
		Use:   "api <path>",
		Short: "Call any endpoint and print the JSON (escape hatch)",
		Example: `  fomo api /v1
  fomo api /v2/users/CryptoKaleo/swaps -p limit=10
  fomo api /v2/thesis -p sort=equity -p minEquity=100000 | jq '.theses[0]'`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			path := args[0]
			if i := strings.IndexByte(path, '?'); i >= 0 {
				pq, err := url.ParseQuery(path[i+1:])
				if err != nil {
					return usagef("bad query string: %s", err)
				}
				q, path = pq, path[:i]
			}
			for _, kv := range params {
				k, v, ok := strings.Cut(kv, "=")
				if !ok {
					return usagef("param %q must look like key=value", kv)
				}
				q.Add(k, v)
			}
			res, err := fetch(cmd, "GET "+path, func(ctx context.Context) (*api.Response, error) { return client.Raw(ctx, path, q) })
			if err != nil {
				return err
			}
			p(res.Data.JSON())
			footer(res)
			return nil
		},
	}
	cmd.Flags().StringArrayVarP(&params, "param", "p", nil, "query parameter key=value (repeatable)")
	return cmd
}

// ---------------------------------------------------------------- version

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			if g.json {
				p(jx.Wrap(map[string]any{"version": Version, "commit": Commit, "date": Date, "go": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH}).JSON())
				return
			}
			p(ui.Logo() + " " + ui.Bold.Render(Version))
			p(indent(ui.KV(
				[2]string{"commit", Commit},
				[2]string{"built", Date},
				[2]string{"go", runtime.Version()},
				[2]string{"platform", runtime.GOOS + "/" + runtime.GOARCH},
			), 2))
		},
	}
}

func indent(s string, n int) string {
	pad := strings.Repeat(" ", n)
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = pad + l
		}
	}
	return strings.Join(lines, "\n")
}
