// Package config loads and saves the user's fomo CLI settings.
//
// Settings live in $XDG_CONFIG_HOME/fomo/config.json (or the OS equivalent),
// written with 0600 permissions because it holds the API key. Environment
// variables always win over the file so the CLI is easy to use in CI and bots.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultBaseURL  = "https://api.fomoapi.io"
	DefaultCacheTTL = 60 * time.Second
)

// Config is the persisted configuration.
type Config struct {
	APIKey        string `json:"api_key,omitempty"`
	BaseURL       string `json:"base_url,omitempty"`
	CacheTTL      string `json:"cache_ttl,omitempty"`      // e.g. "60s", "5m"; "0" disables reuse
	DefaultWindow string `json:"default_window,omitempty"` // leaderboard window
	DefaultChain  string `json:"default_chain,omitempty"`
	Theme         string `json:"theme,omitempty"` // "auto" | "none"
}

// Keys that `fomo config set` accepts, with a short description.
var Keys = map[string]string{
	"api_key":        "API key (prefer `fomo auth login`)",
	"base_url":       "API base URL (default " + DefaultBaseURL + ")",
	"cache_ttl":      "reuse cached responses this long, e.g. 60s, 5m, 0 to disable",
	"default_window": "leaderboard window: 24h | 7d | 30d | all",
	"default_chain":  "default chain filter, e.g. solana, robinhood, base",
	"theme":          "auto | none (disable colors)",
}

// SortedKeys returns the config keys in stable order.
func SortedKeys() []string {
	out := make([]string, 0, len(Keys))
	for k := range Keys {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Dir returns the configuration directory, honoring FOMO_CONFIG_DIR.
func Dir() (string, error) {
	if d := os.Getenv("FOMO_CONFIG_DIR"); d != "" {
		return d, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "fomo"), nil
}

// Path returns the full path of config.json.
func Path() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "config.json"), nil
}

// Load reads the config file. A missing file is not an error.
func Load() (*Config, error) {
	c := &Config{}
	p, err := Path()
	if err != nil {
		return c, err
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(b, c); err != nil {
		return c, fmt.Errorf("parse %s: %w", p, err)
	}
	return c, nil
}

// Save writes the config file atomically with 0600 permissions.
func (c *Config) Save() error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// Get returns a config value by key.
func (c *Config) Get(key string) (string, error) {
	switch key {
	case "api_key":
		return c.APIKey, nil
	case "base_url":
		return c.BaseURL, nil
	case "cache_ttl":
		return c.CacheTTL, nil
	case "default_window":
		return c.DefaultWindow, nil
	case "default_chain":
		return c.DefaultChain, nil
	case "theme":
		return c.Theme, nil
	}
	return "", fmt.Errorf("unknown key %q (valid: %s)", key, strings.Join(SortedKeys(), ", "))
}

// Set validates and stores a config value by key.
func (c *Config) Set(key, val string) error {
	switch key {
	case "api_key":
		c.APIKey = strings.TrimSpace(val)
	case "base_url":
		c.BaseURL = strings.TrimRight(strings.TrimSpace(val), "/")
	case "cache_ttl":
		if _, err := ParseTTL(val); err != nil {
			return err
		}
		c.CacheTTL = val
	case "default_window":
		if err := ValidWindow(val); err != nil {
			return err
		}
		c.DefaultWindow = val
	case "default_chain":
		c.DefaultChain = val
	case "theme":
		if val != "auto" && val != "none" && val != "" {
			return fmt.Errorf("theme must be auto or none")
		}
		c.Theme = val
	default:
		_, err := c.Get(key)
		return err
	}
	return nil
}

// ResolvedKey returns the API key and where it came from.
func (c *Config) ResolvedKey() (key, source string) {
	if k := strings.TrimSpace(os.Getenv("FOMO_API_KEY")); k != "" {
		return k, "env FOMO_API_KEY"
	}
	if c.APIKey != "" {
		return c.APIKey, "config file"
	}
	return "", ""
}

// ResolvedBaseURL returns the API base URL.
func (c *Config) ResolvedBaseURL() string {
	if u := os.Getenv("FOMO_BASE_URL"); u != "" {
		return strings.TrimRight(u, "/")
	}
	if c.BaseURL != "" {
		return c.BaseURL
	}
	return DefaultBaseURL
}

// ResolvedTTL returns how long cached responses may be reused.
func (c *Config) ResolvedTTL() time.Duration {
	if c.CacheTTL == "" {
		return DefaultCacheTTL
	}
	d, err := ParseTTL(c.CacheTTL)
	if err != nil {
		return DefaultCacheTTL
	}
	return d
}

// ParseTTL accepts Go durations ("90s", "5m"), days ("7d") or plain seconds ("60").
func ParseTTL(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if n, err := strconv.Atoi(s); err == nil {
		return time.Duration(n) * time.Second, nil
	}
	if days, ok := strings.CutSuffix(s, "d"); ok {
		if n, err := strconv.Atoi(days); err == nil && n >= 0 {
			return time.Duration(n) * 24 * time.Hour, nil
		}
	}
	d, err := time.ParseDuration(s)
	if err != nil || d < 0 {
		return 0, fmt.Errorf("invalid duration %q (try 60s, 5m or 0)", s)
	}
	return d, nil
}

// ValidWindow checks a leaderboard window.
func ValidWindow(w string) error {
	switch w {
	case "24h", "7d", "30d", "all":
		return nil
	}
	return fmt.Errorf("invalid window %q (use 24h, 7d, 30d or all)", w)
}

// Mask hides most of an API key for display.
func Mask(k string) string {
	if len(k) <= 8 {
		return strings.Repeat("•", len(k))
	}
	return k[:4] + strings.Repeat("•", 8) + k[len(k)-4:]
}
