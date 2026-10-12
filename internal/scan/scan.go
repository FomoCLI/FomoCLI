// Package scan turns a token's flow, dev positions and smart-money holders
// into one risk score with the reasons behind it. It is a heuristic for
// triage, not a safety guarantee: the reasons matter more than the number.
package scan

import (
	"fmt"
	"math"
	"strings"

	"github.com/FomoCLI/FomoCLI/internal/jx"
)

// Level of a finding.
type Level string

const (
	Good Level = "good"
	Warn Level = "warn"
	Bad  Level = "bad"
	Info Level = "info"
)

// Finding is one reason that moved the score.
type Finding struct {
	Level  Level  `json:"level"`
	Area   string `json:"area"`
	Text   string `json:"text"`
	Points int    `json:"points"`
}

// Result is the full verdict.
type Result struct {
	Address  string    `json:"address"`
	Risk     int       `json:"risk"`    // 0 (calm) … 100 (run)
	Verdict  string    `json:"verdict"` // LOW | MEDIUM | HIGH
	Findings []Finding `json:"findings"`
	Missing  []string  `json:"missing,omitempty"` // inputs that could not be loaded
}

// Inputs are the raw API payloads; any of them may be null.
type Inputs struct {
	Address string
	Stats   jx.V // /v2/token/{address}/stats
	Devs    jx.V // /v2/token/{address}/devs
	Holders jx.V // /token/{address}/holders
}

const base = 25

// Score evaluates the inputs.
func Score(in Inputs) Result {
	r := Result{Address: in.Address}
	add := func(l Level, area string, pts int, format string, a ...any) {
		r.Findings = append(r.Findings, Finding{Level: l, Area: area, Points: pts, Text: fmt.Sprintf(format, a...)})
	}

	// ---- concentration & holder base
	if in.Stats.IsNull() {
		r.Missing = append(r.Missing, "stats")
	} else {
		if top, ok := in.Stats.Get("top10HoldersPercent").Float(); ok {
			switch {
			case top >= 60:
				add(Bad, "supply", 30, "Top 10 wallets hold %.0f%% of supply", top)
			case top >= 40:
				add(Warn, "supply", 15, "Top 10 wallets hold %.0f%% of supply", top)
			case top < 25:
				add(Good, "supply", -5, "Supply is spread out: top 10 hold %.0f%%", top)
			default:
				add(Info, "supply", 0, "Top 10 wallets hold %.0f%%", top)
			}
		}
		if h, ok := in.Stats.Get("holders").Float(); ok {
			switch {
			case h < 100:
				add(Bad, "holders", 15, "Only %.0f holders", h)
			case h < 500:
				add(Warn, "holders", 6, "Thin holder base: %.0f holders", h)
			case h >= 2000:
				add(Good, "holders", -4, "%s holders", commas(h))
			}
		}

		// ---- flow
		w1h, w24, w5 := in.Stats.Get("windows", "1h"), in.Stats.Get("windows", "24h"), in.Stats.Get("windows", "5m")
		if w1h.Exists() {
			net := w1h.Get("netVolumeUsd").Num()
			ratio, hasRatio := w1h.Get("buySellRatio").Float()
			switch {
			case net < 0 && hasRatio && ratio < 0.8:
				add(Bad, "flow", 15, "Sell pressure in the last hour: net %s, %.2f buys per sell", usd(net), ratio)
			case net < 0:
				add(Warn, "flow", 7, "More sold than bought in the last hour (net %s)", usd(net))
			case net > 0:
				add(Good, "flow", -5, "Net buying in the last hour (%s)", usd(net))
			}
		}
		if w24.Exists() && w24.Get("netVolumeUsd").Num() < 0 {
			add(Warn, "flow", 6, "Net selling over 24h (%s)", usd(w24.Get("netVolumeUsd").Num()))
		}
		if w5.Exists() {
			b, s := w5.Get("uniqueBuyers").Num(), w5.Get("uniqueSellers").Num()
			if s >= 5 && s > 2*b {
				add(Bad, "flow", 10, "Exit rush: %.0f sellers vs %.0f buyers in 5 minutes", s, b)
			}
		}
	}

	// ---- devs
	if in.Devs.IsNull() {
		r.Missing = append(r.Missing, "devs")
	} else {
		devs := in.Devs.List("devs", "holders", "items")
		found := false
		for _, d := range devs {
			if d.Get("isDev").Exists() && !d.Get("isDev").Bool() {
				continue
			}
			found = true
			who := "@" + d.Get("handle").Str()
			realized := d.Get("realizedPnlUsd").Num()
			value := d.Get("valueUsd").Num()
			cost := d.Get("costBasisUsd").Num()
			switch {
			case realized > 0 && value < realized:
				add(Bad, "dev", 25, "Dev %s has taken %s off the table and holds only %s", who, usd(realized), usd(value))
			case realized > 0:
				add(Warn, "dev", 10, "Dev %s has realized %s but still holds %s", who, usd(realized), usd(value))
			case value > 0 && cost > 0 && value > 3*cost:
				add(Warn, "dev", 5, "Dev %s sits on %s from a %s entry - a large unrealized bag", who, usd(value), usd(cost))
			case value > 0:
				add(Good, "dev", -5, "Dev %s is still holding (%s) and has not sold", who, usd(value))
			}
		}
		if !found {
			add(Info, "dev", 0, "No dev holding known to FOMO - not a safety signal on its own")
		}
	}

	// ---- smart money
	if in.Holders.IsNull() {
		r.Missing = append(r.Missing, "holders")
	} else {
		hs := in.Holders.List("holders", "items")
		var total float64
		for _, h := range hs {
			total += h.Get("valueUsd").Num()
		}
		switch {
		case len(hs) >= 3:
			add(Good, "smart money", -10, "%d tracked traders hold it (%s)", len(hs), usd(total))
		case len(hs) > 0:
			add(Info, "smart money", -3, "%d tracked trader(s) hold it (%s)", len(hs), usd(total))
		default:
			add(Warn, "smart money", 5, "No tracked traders hold it")
		}
	}

	risk := base
	for _, f := range r.Findings {
		risk += f.Points
	}
	r.Risk = int(math.Max(0, math.Min(100, float64(risk))))
	switch {
	case r.Risk >= 60:
		r.Verdict = "HIGH"
	case r.Risk >= 35:
		r.Verdict = "MEDIUM"
	default:
		r.Verdict = "LOW"
	}
	return r
}

func usd(f float64) string {
	sign := ""
	if f < 0 {
		sign, f = "-", -f
	}
	switch {
	case f >= 1e9:
		return fmt.Sprintf("%s$%.1fB", sign, f/1e9)
	case f >= 1e6:
		return fmt.Sprintf("%s$%.1fM", sign, f/1e6)
	case f >= 1e3:
		return fmt.Sprintf("%s$%.1fK", sign, f/1e3)
	}
	return fmt.Sprintf("%s$%.0f", sign, f)
}

func commas(f float64) string {
	s := fmt.Sprintf("%.0f", f)
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	return b.String()
}
