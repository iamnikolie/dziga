package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/langgerone/dziga/internal/media"
	"github.com/langgerone/dziga/internal/provider"
	"github.com/langgerone/dziga/internal/registry"
	"github.com/langgerone/dziga/internal/uploads"
	"github.com/spf13/cobra"
)

var (
	askModel  string
	askFrom   string
	askTo     string
	askFPS    float64
	askRes    string
	askSchema string
	askMax    int
	askTemp   float64
	askFresh  bool
	askEst    bool
)

const defaultAskPrompt = `Describe this video shot by shot with timestamps (MM:SS).
For each shot: what is on screen, what moves, any text or UI visible verbatim.
Be concrete and literal. Do not speculate about intent.`

var askCmd = &cobra.Command{
	Use:   "ask <file|url> [question]",
	Short: "Ask Gemini about the clip (native video, timestamps, motion)",
	Long: `Send the clip to Gemini's native video understanding and print the answer.

Use this for what stills cannot show: motion, ordering, long clips, or a specific
question. For "what does this look like", a sheet is free and you look yourself.

The upload is cached by content hash for the 48h the API keeps the file, so follow-up
questions about the same clip do not re-upload it.`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := ctxWithTimeout(cmd)
		defer cancel()
		start := time.Now()

		prompt := defaultAskPrompt
		if len(args) == 2 && strings.TrimSpace(args[1]) != "" {
			prompt = args[1]
		}

		key := cfg.APIKey("gemini")
		if key == "" {
			return fmt.Errorf("no Gemini API key: set GEMINI_API_KEY, or run 'dziga config init --config <profile>'")
		}
		alias, model, err := reg.Lookup(askModel)
		if err != nil {
			return err
		}

		src, err := resolveSource(ctx, args[0])
		if err != nil {
			return err
		}
		info, err := media.Probe(ctx, src.Path)
		if err != nil {
			return err
		}

		// What this run will cost, before it is spent. Video tokens dominate and
		// are predictable from the window, the sampling rate and the audio track;
		// the real number comes back in usageMetadata afterwards.
		window := watchWindow(info, askFrom, askTo)
		estTokens := reg.EstimateVideoTokens(window, askFPS, askRes, info.HasAudio)
		estBilling, estExact := reg.Billing(alias, model, "")
		statusf("estimate: window=%s fps=%.3g res=%s video_tokens≈%d %s",
			media.FormatTS(window), effectiveFPS(askFPS), orDefault(askRes),
			estTokens, estimateCost(estBilling, estExact, estTokens))

		if askEst {
			if jsonOutput {
				return emitJSON(map[string]any{
					"estimate":         true,
					"model":            alias,
					"api_id":           model.ID,
					"window_sec":       window,
					"fps":              effectiveFPS(askFPS),
					"res":              orDefault(askRes),
					"has_audio":        info.HasAudio,
					"video_tokens_est": estTokens,
					"cost_usd_est":     estCostValue(estBilling, estTokens),
					"prices_assumed":   !estExact,
				})
			}
			return nil
		}

		gem := provider.NewGemini(key, http)
		file, reused, err := ensureUploaded(ctx, gem, src)
		if err != nil {
			return err
		}

		req := provider.AskRequest{
			FileURI:   file.URI,
			MimeType:  file.MimeType,
			Prompt:    prompt,
			FPS:       askFPS,
			MediaRes:  askRes,
			MaxTokens: askMax,
		}
		if askFrom != "" {
			v, err := media.ParseTime(askFrom)
			if err != nil {
				return err
			}
			req.StartOffset = &v
		}
		if askTo != "" {
			v, err := media.ParseTime(askTo)
			if err != nil {
				return err
			}
			req.EndOffset = &v
		}
		if askTemp >= 0 {
			req.Temperature = &askTemp
		}
		// --json changes this CLI's stdout, not the model's job: the answer stays
		// prose inside .answer unless a schema says what shape it should have.
		if askSchema != "" {
			raw, err := loadSchema(askSchema)
			if err != nil {
				return err
			}
			req.Schema = raw
		}

		res, err := gem.Ask(ctx, model.ID, req)
		if err != nil {
			return err
		}

		billing, exact := reg.Billing(alias, model, res.ModelVersion)
		statusf("model=%s id=%s served=%s upload=%s video_tokens=%d in=%d out=%d think=%d %s ms=%d",
			alias, model.ID, orDefault(res.ModelVersion), uploadState(reused), res.Usage.Video,
			res.Usage.Prompt, res.Usage.Output, res.Usage.Thoughts,
			costLabel(billing, exact, res.Usage), time.Since(start).Milliseconds())

		if jsonOutput {
			return emitJSON(map[string]any{
				"answer":           res.Text,
				"model":            alias,
				"api_id":           model.ID,
				"model_version":    res.ModelVersion,
				"finish_reason":    res.FinishReason,
				"usage":            res.Usage,
				"source":           src.Path,
				"duration_sec":     info.Duration,
				"file_uri":         file.URI,
				"reused_upload":    reused,
				"cost_usd":         costValue(billing, res.Usage),
				"prices_assumed":   !exact,
				"video_tokens_est": estTokens,
			})
		}
		fmt.Fprintln(stdout, strings.TrimRight(res.Text, "\n"))
		return nil
	},
}

// ensureUploaded returns a live Files API handle for the clip, reusing the
// cached one when the same bytes were uploaded within the last 48h.
func ensureUploaded(ctx context.Context, gem *provider.Gemini, src *Source) (*provider.File, bool, error) {
	hash, err := uploads.HashFile(src.Path)
	if err != nil {
		return nil, false, err
	}
	dir, err := homeDir()
	if err != nil {
		return nil, false, err
	}
	store, err := uploads.Open(dir)
	if err != nil {
		return nil, false, err
	}

	if !askFresh {
		if e, ok := store.Get(hash); ok {
			return &provider.File{Name: e.Name, URI: e.URI, MimeType: e.MimeType, State: "ACTIVE", ExpirationTime: e.ExpiresAt}, true, nil
		}
	}

	mime := mimeForPath(src.Path)
	st, _ := os.Stat(src.Path)
	statusf("upload %s (%s) → Gemini Files API", filepath.Base(src.Path), humanBytes(sizeOf(st)))

	file, err := gem.Upload(ctx, src.Path, mime, filepath.Base(src.Path))
	if err != nil {
		return nil, false, err
	}
	file, err = gem.WaitActive(ctx, file, 2*time.Second)
	if err != nil {
		return nil, false, err
	}
	_ = store.Put(hash, uploads.Entry{
		Name: file.Name, URI: file.URI, MimeType: file.MimeType,
		Bytes: sizeOf(st), Source: src.Path, ExpiresAt: file.ExpirationTime,
	})
	return file, false, nil
}

func uploadState(reused bool) string {
	if reused {
		return "cached"
	}
	return "new"
}

// costLabel prints a real cost when the registry has a rate in force for the
// model that actually served the request, and an honest cost≈? when it does not.
// exact=false means the rate came from a "-latest" alias's assumption, so the
// number is marked rather than passed off as billed truth.
func costLabel(m registry.Model, exact bool, u provider.Usage) string {
	c, ok := m.CostAt(time.Now(), u.Prompt, u.Output+u.Thoughts)
	if !ok {
		return "cost≈?"
	}
	if !exact {
		return fmt.Sprintf("cost~$%.4f(assumed)", c)
	}
	return fmt.Sprintf("cost≈$%.4f", c)
}

// costValue is costLabel's JSON counterpart: a number, or null when unknown.
func costValue(m registry.Model, u provider.Usage) any {
	if c, ok := m.CostAt(time.Now(), u.Prompt, u.Output+u.Thoughts); ok {
		return c
	}
	return nil
}

// estimateCost prices the input side only — the answer length is not knowable
// before the call.
func estimateCost(m registry.Model, exact bool, inTokens int) string {
	c, ok := m.CostAt(time.Now(), inTokens, 0)
	if !ok {
		return "cost≈? (no published rate for this model)"
	}
	if !exact {
		return fmt.Sprintf("cost~$%.4f in-only (rate assumed from %s)", c, m.ID)
	}
	return fmt.Sprintf("cost≈$%.4f in-only", c)
}

func estCostValue(m registry.Model, inTokens int) any {
	if c, ok := m.CostAt(time.Now(), inTokens, 0); ok {
		return c
	}
	return nil
}

// watchWindow is how much of the clip the model will actually watch.
func watchWindow(info *media.Info, from, to string) float64 {
	start, end := 0.0, info.Duration
	if from != "" {
		if v, err := media.ParseTime(from); err == nil {
			start = v
		}
	}
	if to != "" {
		if v, err := media.ParseTime(to); err == nil && v > 0 && v < end {
			end = v
		}
	}
	if end <= start {
		return 0
	}
	return end - start
}

func effectiveFPS(f float64) float64 {
	if f <= 0 {
		return 1
	}
	return f
}

func orDefault(s string) string {
	if s == "" {
		return "default"
	}
	return s
}

func loadSchema(s string) (json.RawMessage, error) {
	trimmed := strings.TrimSpace(s)
	if strings.HasPrefix(trimmed, "{") {
		return json.RawMessage(trimmed), nil
	}
	data, err := os.ReadFile(trimmed)
	if err != nil {
		return nil, fmt.Errorf("--schema: %w", err)
	}
	return json.RawMessage(data), nil
}

func mimeForPath(p string) string {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".mov":
		return "video/quicktime"
	case ".webm":
		return "video/webm"
	case ".mkv":
		return "video/x-matroska"
	case ".avi":
		return "video/x-msvideo"
	case ".mpeg", ".mpg":
		return "video/mpeg"
	case ".3gp":
		return "video/3gpp"
	case ".wmv":
		return "video/x-ms-wmv"
	case ".flv":
		return "video/x-flv"
	}
	return "video/mp4"
}

func sizeOf(st os.FileInfo) int64 {
	if st == nil {
		return 0
	}
	return st.Size()
}

func init() {
	askCmd.Flags().StringVar(&askModel, "model", "", "registry alias or raw API id (default: registry default)")
	askCmd.Flags().StringVar(&askFrom, "from", "", "only watch from this timestamp")
	askCmd.Flags().StringVar(&askTo, "to", "", "only watch up to this timestamp")
	askCmd.Flags().Float64Var(&askFPS, "fps", 0, "frames per second the model samples (default 1; raise for fast action, lower for long clips)")
	askCmd.Flags().StringVar(&askRes, "res", "", "media resolution: low|medium|high (high costs ~3x, needed for small on-screen text)")
	askCmd.Flags().StringVar(&askSchema, "schema", "", "JSON Schema file or inline object — forces structured JSON output")
	askCmd.Flags().IntVar(&askMax, "max-tokens", 0, "cap the answer length")
	askCmd.Flags().Float64Var(&askTemp, "temperature", -1, "sampling temperature (-1 = model default)")
	askCmd.Flags().BoolVar(&askFresh, "fresh", false, "ignore the upload cache and re-upload")
	askCmd.Flags().BoolVar(&askEst, "estimate", false, "print the predicted token/dollar cost and exit without calling the API")
	rootCmd.AddCommand(askCmd)
}
