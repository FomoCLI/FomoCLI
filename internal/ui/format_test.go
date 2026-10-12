package ui

import (
	"testing"
	"time"

	"github.com/FomoCLI/FomoCLI/internal/jx"
)

func init() { DisableColor() }

func num(f float64) jx.V { return jx.Wrap(f) }

func TestCompact(t *testing.T) {
	cases := map[float64]string{
		0: "0", 12: "12", 999: "999", 1000: "1K", 1500: "1.5K", 151383: "151.4K",
		2_500_000: "2.5M", 1_120_000_000: "1.1B", -3120: "-3.1K", 0.5: "0.5",
	}
	for in, want := range cases {
		if got := Compact(in); got != want {
			t.Errorf("Compact(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestPrice(t *testing.T) {
	cases := map[float64]string{
		0: "$0", 182.4: "$182.40", 1.12: "$1.12", 0.0712: "$0.0712", 0.0031: "$0.0031",
		0.004812: "$0.004812", 0.00000231: "$0.0₅231", 0.00000037: "$0.0₆37", 2500: "$2,500",
	}
	for in, want := range cases {
		if got := Price(num(in)); got != want {
			t.Errorf("Price(%v) = %q, want %q", in, got, want)
		}
	}
	if got := Price(jx.V{}); got != Dash {
		t.Errorf("Price(null) = %q", got)
	}
}

func TestSignedAndPct(t *testing.T) {
	if got := Signed(num(8231)); got != "+$8.2K" {
		t.Errorf("Signed = %q", got)
	}
	if got := Signed(num(-910)); got != "-$910" {
		t.Errorf("Signed = %q", got)
	}
	if got := Pct(num(38.44)); got != "▲ 38.4%" {
		t.Errorf("Pct = %q", got)
	}
	if got := Pct(num(-12.7)); got != "▼ 12.7%" {
		t.Errorf("Pct = %q", got)
	}
}

func TestCommasShortDur(t *testing.T) {
	if got := Commas(1234567, 0); got != "1,234,567" {
		t.Errorf("Commas = %q", got)
	}
	if got := Commas(-1234.5, 1); got != "-1,234.5" {
		t.Errorf("Commas = %q", got)
	}
	if got := Short("5AhfPStn66hRYoNNDfJHSDgCH7fBbwMQZUECRrhTo62F"); got != "5AhfPS…To62F" {
		t.Errorf("Short = %q", got)
	}
	if got := Short("0x7fe995"); got != "0x7fe995" {
		t.Errorf("Short short = %q", got)
	}
	for d, want := range map[time.Duration]string{30 * time.Second: "30s", 5 * time.Minute: "5m", 3 * time.Hour: "3h", 72 * time.Hour: "3d"} {
		if got := Dur(d); got != want {
			t.Errorf("Dur(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestTimeParsing(t *testing.T) {
	iso, ok := Time(jx.Wrap("2026-10-12T06:33:21.199Z"))
	if !ok || iso.Year() != 2026 {
		t.Fatalf("iso parse failed: %v %v", iso, ok)
	}
	ms, ok := Time(num(1788378000000))
	if !ok || ms.Unix() != 1788378000 {
		t.Fatalf("ms parse failed: %v", ms)
	}
	s, ok := Time(num(1788378000))
	if !ok || s.Unix() != 1788378000 {
		t.Fatalf("s parse failed: %v", s)
	}
}

func TestTruncateAndTable(t *testing.T) {
	if got := Truncate("frankdegods", 6); got != "frank…" {
		t.Errorf("Truncate = %q", got)
	}
	tb := NewTable(Col{Title: "a"}, Col{Title: "b", Align: Right})
	tb.Add("x", "1")
	tb.Add("longer", "100")
	out := tb.Render()
	want := "  A         B\n  ───────────\n  x         1\n  longer  100\n"
	if out != want {
		t.Errorf("table:\n%q\nwant\n%q", out, want)
	}
}

func TestChainName(t *testing.T) {
	if ChainName("4663") != "robinhood" || ChainName("1399811149") != "solana" || ChainName("5042") != "arc" {
		t.Error("chain ids not mapped")
	}
}
