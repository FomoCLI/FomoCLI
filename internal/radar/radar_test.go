package radar

import (
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)

func buy(trader, sym string, usd float64, at time.Duration) Trade {
	return Trade{Trader: trader, Symbol: sym, Address: "0x" + sym, Chain: "robinhood", Side: "buy", USD: usd, At: t0.Add(at)}
}

func TestSwarmFormsAndGrows(t *testing.T) {
	d := New(Config{Window: 10 * time.Minute, MinTraders: 3})
	if d.Add(buy("a", "PONS", 1000, 0)) != nil || d.Add(buy("a", "PONS", 500, time.Minute)) != nil {
		t.Fatal("one trader is not a swarm")
	}
	if d.Add(buy("b", "PONS", 2000, 2*time.Minute)) != nil {
		t.Fatal("two traders is not a swarm")
	}
	s := d.Add(buy("c", "PONS", 4000, 3*time.Minute))
	if s == nil || !s.First || s.Count() != 3 || s.Trades != 4 || s.TotalUSD != 7500 || s.Span != 3*time.Minute {
		t.Fatalf("swarm not detected correctly: %+v", s)
	}
	if s.Traders[0].Trader != "c" {
		t.Errorf("biggest participant first, got %s", s.Traders[0].Trader)
	}
	if d.Add(buy("c", "PONS", 100, 4*time.Minute)) != nil {
		t.Error("same traders again must not re-alert")
	}
	g := d.Add(buy("d", "PONS", 100, 5*time.Minute))
	if g == nil || g.First || g.Count() != 4 {
		t.Fatalf("growth not signalled: %+v", g)
	}
}

func TestWindowAndFilters(t *testing.T) {
	d := New(Config{Window: 5 * time.Minute, MinTraders: 3, MinUSD: 500})
	d.Add(buy("a", "W", 1000, 0))
	d.Add(buy("b", "W", 1000, time.Minute))
	if d.Add(buy("c", "W", 1000, 7*time.Minute)) != nil {
		t.Error("trades outside the window must not count")
	}
	if d.Add(buy("d", "W", 100, 7*time.Minute)) != nil {
		t.Error("trades under MinUSD must be ignored")
	}
	if d.Add(buy("e", "OTHER", 1000, 7*time.Minute)) != nil {
		t.Error("different tokens must not mix")
	}
	sell := buy("f", "W", 1000, 7*time.Minute)
	sell.Side = "sell"
	if d.Add(sell) != nil {
		t.Error("sells ignored when Side=buy")
	}
	// Inside the window now: c (7m) and g (8m) - still only two.
	if d.Add(buy("g", "W", 1000, 8*time.Minute)) != nil {
		t.Error("a and b slid out; two traders must not fire")
	}
	if s := d.Add(buy("h", "W", 1000, 9*time.Minute)); s == nil || s.Count() != 3 {
		t.Errorf("c, g, h should form a swarm; got %+v", s)
	}
}

func TestSellSwarmAndRealert(t *testing.T) {
	d := New(Config{Window: time.Minute, MinTraders: 2, Side: "sell"})
	s1 := Trade{Trader: "a", Symbol: "X", Side: "sell", USD: -300, At: t0}
	s2 := Trade{Trader: "b", Symbol: "X", Side: "sell", USD: -200, At: t0.Add(10 * time.Second)}
	d.Add(s1)
	sig := d.Add(s2)
	if sig == nil || sig.TotalUSD != 500 {
		t.Fatalf("sell swarm with negative usd: %+v", sig)
	}
	// After the swarm dissolves it can fire again.
	s3, s4 := s1, s2
	s3.At, s4.At = t0.Add(5*time.Minute), t0.Add(5*time.Minute+time.Second)
	d.Add(s3)
	if again := d.Add(s4); again == nil || !again.First {
		t.Fatalf("expected a fresh alert after the window: %+v", again)
	}
	if n, _ := d.Stats(); n != 4 {
		t.Errorf("seen = %d", n)
	}
}
