package scan

import (
	"testing"

	"github.com/FomoCLI/FomoCLI/internal/jx"
)

func parse(t *testing.T, s string) jx.V {
	t.Helper()
	v, err := jx.Parse([]byte(s))
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestRuggyTokenScoresHigh(t *testing.T) {
	r := Score(Inputs{
		Address: "0xbad",
		Stats: parse(t, `{"holders":60,"top10HoldersPercent":72,"windows":{
			"5m":{"uniqueBuyers":1,"uniqueSellers":9},
			"1h":{"netVolumeUsd":-40000,"buySellRatio":0.4},
			"24h":{"netVolumeUsd":-90000}}}`),
		Devs:    parse(t, `{"devs":[{"handle":"dev","isDev":true,"realizedPnlUsd":50000,"valueUsd":800,"costBasisUsd":1000}]}`),
		Holders: parse(t, `{"holders":[]}`),
	})
	if r.Verdict != "HIGH" || r.Risk != 100 {
		t.Fatalf("want HIGH/100, got %s/%d: %+v", r.Verdict, r.Risk, r.Findings)
	}
	var devBad bool
	for _, f := range r.Findings {
		devBad = devBad || (f.Area == "dev" && f.Level == Bad)
	}
	if !devBad {
		t.Error("dev dumping must be flagged")
	}
}

func TestHealthyTokenScoresLow(t *testing.T) {
	r := Score(Inputs{
		Stats:   parse(t, `{"holders":5000,"top10HoldersPercent":18,"windows":{"1h":{"netVolumeUsd":20000,"buySellRatio":1.6}}}`),
		Devs:    parse(t, `{"devs":[{"handle":"dev","isDev":true,"realizedPnlUsd":0,"valueUsd":9000,"costBasisUsd":8000}]}`),
		Holders: parse(t, `{"holders":[{"valueUsd":1},{"valueUsd":2},{"valueUsd":3}]}`),
	})
	if r.Verdict != "LOW" {
		t.Fatalf("want LOW, got %s/%d: %+v", r.Verdict, r.Risk, r.Findings)
	}
}

func TestMissingInputs(t *testing.T) {
	r := Score(Inputs{Stats: parse(t, `{"holders":300}`)})
	if len(r.Missing) != 2 || r.Verdict == "" {
		t.Fatalf("missing inputs not reported: %+v", r)
	}
	if r := Score(Inputs{Devs: parse(t, `{"devs":[]}`)}); len(r.Findings) != 1 || r.Findings[0].Level != Info {
		t.Errorf("empty devs should be an info note: %+v", r.Findings)
	}
}
