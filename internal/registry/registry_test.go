package registry

import (
	"testing"
	"time"
)

func TestEmbeddedRegistryLoads(t *testing.T) {
	reg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reg.Models[reg.Default]; !ok {
		t.Fatalf("default %q is not in the model table", reg.Default)
	}
	for alias, m := range reg.Models {
		if m.ID == "" || m.Provider != "gemini" {
			t.Errorf("%s: id=%q provider=%q", alias, m.ID, m.Provider)
		}
		if m.Assume != "" {
			if _, ok := reg.Models[m.Assume]; !ok {
				t.Errorf("%s: assume points at unknown alias %q", alias, m.Assume)
			}
		}
	}
	if reg.Video.TokensPerFrame["default"] == 0 || reg.Video.AudioTokensPerSec == 0 {
		t.Error("measured video rates missing")
	}
}

func TestLookup(t *testing.T) {
	reg, _ := Load("")

	alias, _, err := reg.Lookup("")
	if err != nil || alias != reg.Default {
		t.Fatalf("empty alias must resolve to the default: %v %v", alias, err)
	}
	if _, m, _ := reg.Lookup("3.7-flash"); m.ID != "gemini-3.7-flash" {
		t.Errorf("alias lookup = %q", m.ID)
	}
	if _, m, _ := reg.Lookup("gemini-3.7-flash"); m.ID != "gemini-3.7-flash" {
		t.Errorf("raw id lookup = %q", m.ID)
	}
	// a model released after this build must still be callable, just unpriced
	_, m, _ := reg.Lookup("gemini-9-flash-preview")
	if m.ID != "gemini-9-flash-preview" {
		t.Errorf("unknown id passthrough = %+v", m)
	}
	if _, ok := m.PricesAt(time.Now(), 0); ok {
		t.Error("an unknown model must report no price, never a guessed one")
	}
}

func TestPricesAtHonoursIntroExpiry(t *testing.T) {
	m := Model{
		PriceInPer1M: 0.75, PriceOutPer1M: 3.75,
		PriceUntil: "2026-12-31",
		After:      &Prices{In: 1.50, Out: 7.50},
	}
	during, ok := m.PricesAt(time.Date(2026, 8, 23, 0, 0, 0, 0, time.UTC), 0)
	if !ok || during.In != 0.75 {
		t.Errorf("intro window = %+v %v", during, ok)
	}
	after, ok := m.PricesAt(time.Date(2027, 1, 2, 0, 0, 0, 0, time.UTC), 0)
	if !ok || after.In != 1.50 || after.Out != 7.50 {
		t.Errorf("after expiry = %+v %v", after, ok)
	}

	// an expired intro rate with no recorded successor must go unknown, not stale
	lapsed := Model{PriceInPer1M: 0.75, PriceOutPer1M: 3.75, PriceUntil: "2026-12-31"}
	if _, ok := lapsed.PricesAt(time.Date(2027, 6, 1, 0, 0, 0, 0, time.UTC), 0); ok {
		t.Error("a lapsed rate with no successor must report unknown")
	}
}

func TestPricesAtLongContextTier(t *testing.T) {
	m := Model{
		PriceInPer1M: 2, PriceOutPer1M: 12,
		LongThreshold: 200_000,
		Long:          &Prices{In: 4, Out: 18},
	}
	if p, _ := m.PricesAt(time.Now(), 199_000); p.In != 2 {
		t.Errorf("under threshold = %+v", p)
	}
	if p, _ := m.PricesAt(time.Now(), 500_000); p.In != 4 || p.Out != 18 {
		t.Errorf("over threshold = %+v", p)
	}
}

func TestCostAt(t *testing.T) {
	m := Model{PriceInPer1M: 0.75, PriceOutPer1M: 3.75}
	c, ok := m.CostAt(time.Now(), 1_000_000, 100_000)
	if !ok || c < 1.124 || c > 1.126 {
		t.Errorf("CostAt = %v %v, want 1.125", c, ok)
	}
	if _, ok := (Model{}).CostAt(time.Now(), 1000, 1000); ok {
		t.Error("a model with no published price must report cost unknown, never 0")
	}
}

func TestBillingPrefersTheModelThatServed(t *testing.T) {
	reg, _ := Load("")
	alias, m, _ := reg.Lookup("flash")

	// before the call: only the alias's assumption is available, flagged inexact
	pre, exact := reg.Billing(alias, m, "")
	if exact {
		t.Error("a -latest alias must not report its assumed rate as exact")
	}
	if _, ok := pre.PricesAt(time.Now(), 0); !ok {
		t.Error("the assumption should still produce an estimate")
	}

	// after the call: the served modelVersion decides
	post, exact := reg.Billing(alias, m, "gemini-3.1-flash-lite")
	if !exact || post.ID != "gemini-3.1-flash-lite" {
		t.Errorf("served model must win: %+v exact=%v", post.ID, exact)
	}

	// served by something the registry has never heard of → unknown
	if _, ok := reg.Billing("x", Model{ID: "x"}, "gemini-99"); ok {
		t.Error("unknown served model must report no price")
	}
}

// The numbers below are live measurements, not arithmetic: see registry.yaml.
func TestEstimateVideoTokensMatchesMeasurements(t *testing.T) {
	reg, _ := Load("")
	cases := []struct {
		name   string
		window float64
		fps    float64
		res    string
		audio  bool
		want   int
	}{
		{"4s silent @1fps", 4, 1, "", false, 264},
		{"4s with audio @1fps", 4, 1, "", true, 364},
		{"4s silent @2fps", 4, 2, "", false, 528},
		{"10s silent @1fps", 10, 1, "", false, 660},
		{"10s silent @1fps high", 10, 1, "high", false, 2640},
	}
	for _, c := range cases {
		if got := reg.EstimateVideoTokens(c.window, c.fps, c.res, c.audio); got != c.want {
			t.Errorf("%s: estimate = %d, want the measured %d", c.name, got, c.want)
		}
	}
	if got := reg.EstimateVideoTokens(0, 1, "", false); got != 0 {
		t.Errorf("empty window = %d, want 0", got)
	}
}
