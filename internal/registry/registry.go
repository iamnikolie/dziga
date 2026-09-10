package registry

import (
	_ "embed"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"time"

	"gopkg.in/yaml.v3"
)

//go:embed registry.yaml
var embedded []byte

// Prices is one USD-per-1M-tokens pair.
type Prices struct {
	In  float64 `yaml:"price_in_per_1m" json:"in_per_1m"`
	Out float64 `yaml:"price_out_per_1m" json:"out_per_1m"`
}

// Model is one alias in the registry.
type Model struct {
	ID            string  `yaml:"id" json:"id"`
	Provider      string  `yaml:"provider" json:"provider"`
	Note          string  `yaml:"note,omitempty" json:"note,omitempty"`
	Assume        string  `yaml:"assume,omitempty" json:"assume,omitempty"`
	PriceInPer1M  float64 `yaml:"price_in_per_1m,omitempty" json:"price_in_per_1m,omitempty"`
	PriceOutPer1M float64 `yaml:"price_out_per_1m,omitempty" json:"price_out_per_1m,omitempty"`
	PriceUntil    string  `yaml:"price_until,omitempty" json:"price_until,omitempty"`
	After         *Prices `yaml:"after,omitempty" json:"after,omitempty"`
	LongThreshold int     `yaml:"long_threshold,omitempty" json:"long_threshold,omitempty"`
	Long          *Prices `yaml:"long,omitempty" json:"long,omitempty"`
}

// PricesAt returns the rates in force at t for a prompt of inTokens, and whether
// they are known at all. An expired introductory rate with no recorded successor
// is reported as unknown — a stale price is worse than an honest question mark.
func (m Model) PricesAt(t time.Time, inTokens int) (Prices, bool) {
	base := Prices{In: m.PriceInPer1M, Out: m.PriceOutPer1M}
	if m.LongThreshold > 0 && inTokens > m.LongThreshold && m.Long != nil {
		base = *m.Long
	}
	if m.PriceUntil != "" {
		until, err := time.Parse("2006-01-02", m.PriceUntil)
		if err == nil && t.After(until.Add(24*time.Hour)) {
			if m.After == nil {
				return Prices{}, false
			}
			base = *m.After
		}
	}
	if base.In <= 0 && base.Out <= 0 {
		return Prices{}, false
	}
	return base, true
}

// CostAt returns USD for a token split at time t, and whether it is real.
func (m Model) CostAt(t time.Time, in, out int) (float64, bool) {
	p, ok := m.PricesAt(t, in)
	if !ok {
		return 0, false
	}
	return float64(in)/1e6*p.In + float64(out)/1e6*p.Out, true
}

// VideoRates are the measured token costs of video input (see registry.yaml).
type VideoRates struct {
	TokensPerFrame    map[string]int `yaml:"tokens_per_frame" json:"tokens_per_frame"`
	AudioTokensPerSec float64        `yaml:"audio_tokens_per_sec" json:"audio_tokens_per_sec"`
}

// Registry is the embedded model table, optionally overridden per profile.
type Registry struct {
	Default string           `yaml:"default" json:"default"`
	Video   VideoRates       `yaml:"video" json:"video"`
	Models  map[string]Model `yaml:"models" json:"models"`
}

// Load parses the embedded registry, then overlays <profileDir>/registry.yaml
// when present (adding a model stays a data edit).
func Load(profileDir string) (*Registry, error) {
	var reg Registry
	if err := yaml.Unmarshal(embedded, &reg); err != nil {
		return nil, fmt.Errorf("registry.Load: embedded: %w", err)
	}
	if profileDir != "" {
		data, err := os.ReadFile(filepath.Join(profileDir, "registry.yaml"))
		if err == nil {
			var over Registry
			if err := yaml.Unmarshal(data, &over); err != nil {
				return nil, fmt.Errorf("registry.Load: override: %w", err)
			}
			if over.Default != "" {
				reg.Default = over.Default
			}
			if over.Video.AudioTokensPerSec > 0 {
				reg.Video.AudioTokensPerSec = over.Video.AudioTokensPerSec
			}
			for k, v := range over.Video.TokensPerFrame {
				reg.Video.TokensPerFrame[k] = v
			}
			for k, v := range over.Models {
				reg.Models[k] = v
			}
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("registry.Load: override: %w", err)
		}
	}
	return &reg, nil
}

// Lookup resolves an alias (or a raw API id) to a Model.
func (r *Registry) Lookup(alias string) (string, Model, error) {
	if alias == "" {
		alias = r.Default
	}
	if m, ok := r.Models[alias]; ok {
		return alias, m, nil
	}
	for name, m := range r.Models {
		if m.ID == alias {
			return name, m, nil
		}
	}
	// a model released after this build still works; it just has no price
	return alias, Model{ID: alias, Provider: "gemini"}, nil
}

// Billing returns the model whose prices should be applied. The "-latest" aliases
// carry no rates of their own: before a call the registry's `assume` hint is used
// (flagged as an assumption), and afterwards the modelVersion the API reported wins.
func (r *Registry) Billing(alias string, m Model, servedID string) (Model, bool) {
	if servedID != "" {
		for _, cand := range r.Models {
			if cand.ID == servedID && (cand.PriceInPer1M > 0 || cand.PriceOutPer1M > 0) {
				return cand, true
			}
		}
	}
	if m.PriceInPer1M > 0 || m.PriceOutPer1M > 0 {
		return m, true
	}
	if m.Assume != "" {
		if cand, ok := r.Models[m.Assume]; ok {
			return cand, false
		}
	}
	return Model{}, false
}

// EstimateVideoTokens predicts the video-token bill for a window, from the rates
// measured in registry.yaml. It is an estimate on purpose: the real number comes
// back in usageMetadata and is what the status line reports after a call.
func (r *Registry) EstimateVideoTokens(windowSec, fps float64, res string, hasAudio bool) int {
	if windowSec <= 0 {
		return 0
	}
	if fps <= 0 {
		fps = 1
	}
	if res == "" {
		res = "default"
	}
	perFrame, ok := r.Video.TokensPerFrame[res]
	if !ok {
		perFrame = r.Video.TokensPerFrame["default"]
	}
	if perFrame <= 0 {
		return 0
	}
	frames := int(math.Round(windowSec * fps))
	if frames < 1 {
		frames = 1
	}
	total := frames * perFrame
	if hasAudio {
		total += int(math.Round(windowSec * r.Video.AudioTokensPerSec))
	}
	return total
}

// Aliases returns the alias names, sorted.
func (r *Registry) Aliases() []string {
	out := make([]string, 0, len(r.Models))
	for k := range r.Models {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
