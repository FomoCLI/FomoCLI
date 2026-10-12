// Package radar detects swarms: several distinct traders buying (or selling)
// the same token within a short window. It consumes the free /ws/alerts
// stream, so it costs no credits, and works the same on recorded events.
package radar

import (
	"sort"
	"strings"
	"time"
)

// Trade is one buy or sell seen on the feed.
type Trade struct {
	Trader   string // handle (display)
	TraderID string // userId when available, else handle
	Symbol   string
	Address  string
	Chain    string
	Side     string // "buy" | "sell"
	USD      float64
	At       time.Time
}

// Participant is one trader's contribution to a swarm.
type Participant struct {
	Trader string
	USD    float64
	Trades int
	Last   time.Time
}

// Signal is emitted when a swarm forms or grows.
type Signal struct {
	Symbol, Address, Chain, Side string
	Traders                      []Participant // biggest first
	TotalUSD                     float64
	Trades                       int
	Span                         time.Duration // first → last trade in the window
	First                        bool          // true when the swarm just formed
	At                           time.Time
}

// Count is the number of distinct traders.
func (s *Signal) Count() int { return len(s.Traders) }

// Config tunes the detector.
type Config struct {
	Window     time.Duration // default 10m
	MinTraders int           // default 3
	MinUSD     float64       // ignore trades smaller than this
	Side       string        // "buy" (default), "sell" or "both"
}

// Detector keeps a sliding window of trades per token.
type Detector struct {
	cfg     Config
	trades  map[string][]Trade
	alerted map[string]int // distinct traders at the last signal
	now     time.Time
	seen    int
}

// New returns a detector.
func New(cfg Config) *Detector {
	if cfg.Window <= 0 {
		cfg.Window = 10 * time.Minute
	}
	if cfg.MinTraders < 2 {
		cfg.MinTraders = 3
	}
	if cfg.Side == "" {
		cfg.Side = "buy"
	}
	return &Detector{cfg: cfg, trades: map[string][]Trade{}, alerted: map[string]int{}}
}

func tokenKey(t Trade) string {
	id := strings.ToLower(t.Address)
	if id == "" {
		id = "$" + strings.ToUpper(t.Symbol)
	}
	return t.Side + "|" + strings.ToLower(t.Chain) + "|" + id
}

// Add records a trade and returns a signal when it forms or grows a swarm.
func (d *Detector) Add(t Trade) *Signal {
	side := strings.ToLower(t.Side)
	if side != "buy" && side != "sell" {
		return nil
	}
	if d.cfg.Side != "both" && side != d.cfg.Side {
		return nil
	}
	if t.USD < 0 {
		t.USD = -t.USD
	}
	if t.USD < d.cfg.MinUSD || (t.Address == "" && t.Symbol == "") {
		return nil
	}
	if t.TraderID == "" {
		t.TraderID = strings.ToLower(t.Trader)
	}
	if t.TraderID == "" {
		return nil
	}
	t.Side = side
	d.seen++
	if t.At.After(d.now) {
		d.now = t.At
	}
	d.prune()

	key := tokenKey(t)
	d.trades[key] = append(d.trades[key], t)

	sig := d.build(key)
	n := sig.Count()
	if n < d.cfg.MinTraders || n <= d.alerted[key] {
		return nil
	}
	sig.First = d.alerted[key] == 0
	d.alerted[key] = n
	return sig
}

// prune drops trades that slid out of the window; a swarm that dissolves can
// alert again later.
func (d *Detector) prune() {
	cut := d.now.Add(-d.cfg.Window)
	for k, ts := range d.trades {
		i := 0
		for i < len(ts) && ts[i].At.Before(cut) {
			i++
		}
		ts = ts[i:]
		if len(ts) == 0 {
			delete(d.trades, k)
			delete(d.alerted, k)
			continue
		}
		d.trades[k] = ts
		if distinct(ts) < d.cfg.MinTraders {
			d.alerted[k] = 0
		}
	}
}

func distinct(ts []Trade) int {
	m := map[string]bool{}
	for _, t := range ts {
		m[t.TraderID] = true
	}
	return len(m)
}

func (d *Detector) build(key string) *Signal {
	ts := d.trades[key]
	last := ts[len(ts)-1]
	s := &Signal{Symbol: last.Symbol, Address: last.Address, Chain: last.Chain, Side: last.Side, At: last.At}
	by := map[string]*Participant{}
	var order []string
	first := ts[0].At
	for _, t := range ts {
		p := by[t.TraderID]
		if p == nil {
			p = &Participant{Trader: t.Trader}
			by[t.TraderID] = p
			order = append(order, t.TraderID)
		}
		p.USD += t.USD
		p.Trades++
		p.Last = t.At
		s.TotalUSD += t.USD
		s.Trades++
		if t.Symbol != "" {
			s.Symbol = t.Symbol
		}
	}
	for _, id := range order {
		s.Traders = append(s.Traders, *by[id])
	}
	sort.SliceStable(s.Traders, func(i, j int) bool { return s.Traders[i].USD > s.Traders[j].USD })
	s.Span = last.At.Sub(first)
	return s
}

// Heat is a token's current activity, for the status line.
type Heat struct {
	Symbol  string
	Traders int
	USD     float64
}

// Hottest returns the most active tokens in the window.
func (d *Detector) Hottest(n int) []Heat {
	var out []Heat
	for k := range d.trades {
		s := d.build(k)
		out = append(out, Heat{Symbol: s.Symbol, Traders: s.Count(), USD: s.TotalUSD})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Traders != out[j].Traders {
			return out[i].Traders > out[j].Traders
		}
		return out[i].USD > out[j].USD
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}

// Stats reports how many trades were seen and how many tokens are in the window.
func (d *Detector) Stats() (trades, tokens int) { return d.seen, len(d.trades) }
