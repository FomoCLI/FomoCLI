package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveLoadAndPermissions(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FOMO_CONFIG_DIR", dir)
	t.Setenv("FOMO_API_KEY", "")

	c := &Config{}
	if err := c.Set("api_key", "  secret-key  "); err != nil {
		t.Fatal(err)
	}
	if err := c.Set("cache_ttl", "5m"); err != nil {
		t.Fatal(err)
	}
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("config perms = %v, want 0600", fi.Mode().Perm())
	}
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if k, src := got.ResolvedKey(); k != "secret-key" || src != "config file" {
		t.Errorf("key = %q from %q", k, src)
	}
	if got.ResolvedTTL() != 5*time.Minute {
		t.Errorf("ttl = %v", got.ResolvedTTL())
	}

	t.Setenv("FOMO_API_KEY", "env-key")
	if k, src := got.ResolvedKey(); k != "env-key" || src != "env FOMO_API_KEY" {
		t.Errorf("env override failed: %q %q", k, src)
	}
}

func TestValidation(t *testing.T) {
	c := &Config{}
	if c.Set("default_window", "1y") == nil {
		t.Error("accepted bad window")
	}
	if c.Set("cache_ttl", "soon") == nil {
		t.Error("accepted bad ttl")
	}
	if c.Set("nope", "x") == nil {
		t.Error("accepted unknown key")
	}
	for in, want := range map[string]time.Duration{"60": time.Minute, "90s": 90 * time.Second, "7d": 7 * 24 * time.Hour, "0": 0} {
		if got, err := ParseTTL(in); err != nil || got != want {
			t.Errorf("ParseTTL(%q) = %v, %v", in, got, err)
		}
	}
}

func TestMask(t *testing.T) {
	if got := Mask("fomo_live_abcdefghijklmnop"); got != "fomo••••••••mnop" {
		t.Errorf("Mask = %q", got)
	}
}
