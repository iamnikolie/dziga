package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/langgerone/dziga/internal/media"
	"github.com/langgerone/dziga/internal/provider"
	"github.com/langgerone/dziga/internal/sheet"
	"github.com/spf13/cobra"
)

var (
	ctxN        int
	ctxScenes   bool
	ctxNoScenes bool
	ctxThresh   float64
	ctxAsk      string
	ctxMaxScan  time.Duration
	ctxPNG      bool
)

var contextCmd = &cobra.Command{
	Use:   "context <file|url>",
	Short: "One-shot digest: metadata + cuts + labelled sheet (markdown)",
	Long: `The default way to hand a clip to an agent.

Probes the file, detects cuts, builds one labelled contact sheet, and prints a
markdown digest whose last line is the sheet path — Read that path to see the clip.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := ctxWithTimeout(cmd)
		defer cancel()
		start := time.Now()

		src, info, dir, err := prepare(ctx, args[0])
		if err != nil {
			return err
		}

		var cuts []float64
		scanned := false
		switch {
		case ctxNoScenes:
		case ctxMaxScan > 0 && info.Duration > ctxMaxScan.Seconds():
			statusf("scenes: skipped, clip is %s (> --max-scan %s); pass --max-scan 0 to force",
				media.FormatTS(info.Duration), ctxMaxScan)
		default:
			cuts, err = media.DetectScenes(ctx, src.Path, ctxThresh, 320)
			if err != nil {
				return err
			}
			scanned = true
		}

		times, mode, err := resolveTimes(ctx, src, info, sampleOpts{
			n: ctxN, useScenes: ctxScenes, threshold: ctxThresh,
		})
		if err != nil {
			return err
		}

		lay := sheet.Solve(len(times), info.Aspect(), 0, 1568, 4)
		tmp := filepath.Join(dir, ".cells")
		frames, err := media.ExtractFrames(ctx, src.Path, times, media.ExtractOpts{
			Dir: tmp, Prefix: "cell", Width: lay.CellW, PNG: ctxPNG,
		})
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmp)

		cells := make([]sheet.Cell, len(frames))
		for i, f := range frames {
			cells[i] = sheet.Cell{Path: f.Path, Label: fmt.Sprintf("%02d %s", f.Index, media.FormatTS(f.T))}
		}
		ext := "jpg"
		if ctxPNG {
			ext = "png"
		}
		res, err := sheet.Compose(cells, info.Aspect(), filepath.Join(dir, "context."+ext), sheet.Opts{PNG: ctxPNG})
		if err != nil {
			return err
		}

		var answer string
		var usage provider.Usage
		if ctxAsk != "" {
			key := cfg.APIKey("gemini")
			if key == "" {
				return fmt.Errorf("--ask needs a Gemini API key: set GEMINI_API_KEY, or run 'dziga config init --config <profile>'")
			}
			_, model, _ := reg.Lookup(askModel)
			gem := provider.NewGemini(key, http)
			file, _, err := ensureUploaded(ctx, gem, src)
			if err != nil {
				return err
			}
			out, err := gem.Ask(ctx, model.ID, provider.AskRequest{
				FileURI: file.URI, MimeType: file.MimeType, Prompt: ctxAsk,
			})
			if err != nil {
				return err
			}
			answer, usage = out.Text, out.Usage
		}

		statusf("context: %d frames mode=%s cuts=%s sheet=%dx%d ~%s img-tokens ms=%d",
			len(frames), mode, cutCount(scanned, cuts), res.Width, res.Height,
			humanTokens(res.Tokens), time.Since(start).Milliseconds())

		if jsonOutput {
			return emitJSON(map[string]any{
				"source":         src.Path,
				"info":           info,
				"cuts_sec":       cuts,
				"scenes_scanned": scanned,
				"sheet":          res,
				"frames":         frameTable(frames),
				"answer":         answer,
				"usage":          usage,
			})
		}

		var b strings.Builder
		fmt.Fprintf(&b, "# %s\n\n", filepath.Base(src.Path))
		fmt.Fprintf(&b, "%s\n", describeInfo(info))
		fmt.Fprintf(&b, "path: %s\n\n", src.Path)

		if scanned {
			fmt.Fprintf(&b, "## Cuts (%d)\n\n", len(cuts))
			if len(cuts) == 0 {
				fmt.Fprintf(&b, "none above threshold %.2f — single continuous shot\n\n", ctxThresh)
			} else {
				fmt.Fprintf(&b, "%s\n\n", strings.Join(formatTimes(cuts), ", "))
			}
		}

		fmt.Fprintf(&b, "## Frames on the sheet (%s sampling)\n\n", mode)
		fmt.Fprintf(&b, "%s\n\n", strings.Join(labelTimes(frames), ", "))

		if answer != "" {
			fmt.Fprintf(&b, "## Gemini\n\n%s\n\n", strings.TrimRight(answer, "\n"))
		}

		fmt.Fprintf(&b, "## Look\n\n")
		fmt.Fprintf(&b, "Read the sheet below (%dx%d, ~%s tokens). Then:\n", res.Width, res.Height, humanTokens(res.Tokens))
		fmt.Fprintf(&b, "  dziga frames %q --at <sec>   # any moment, full size\n", src.Arg)
		fmt.Fprintf(&b, "  dziga ask %q \"<question>\"    # motion, ordering, long clips\n\n", src.Arg)
		fmt.Fprintln(&b, res.Path)

		fmt.Fprint(stdout, b.String())
		return nil
	},
}

func cutCount(scanned bool, cuts []float64) string {
	if !scanned {
		return "skipped"
	}
	return fmt.Sprintf("%d", len(cuts))
}

func formatTimes(ts []float64) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = media.FormatTS(t)
	}
	return out
}

func labelTimes(frames []media.Frame) []string {
	out := make([]string, len(frames))
	for i, f := range frames {
		out[i] = fmt.Sprintf("%02d→%s", f.Index, media.FormatTS(f.T))
	}
	return out
}

func init() {
	contextCmd.Flags().IntVarP(&ctxN, "n", "n", 16, "frames on the sheet")
	contextCmd.Flags().BoolVar(&ctxScenes, "scenes", false, "sample the sheet at cuts instead of evenly")
	contextCmd.Flags().BoolVar(&ctxNoScenes, "no-scenes", false, "skip cut detection entirely")
	contextCmd.Flags().Float64Var(&ctxThresh, "threshold", 0.3, "scene sensitivity")
	contextCmd.Flags().StringVar(&ctxAsk, "ask", "", "also ask Gemini this question about the clip")
	contextCmd.Flags().DurationVar(&ctxMaxScan, "max-scan", 20*time.Minute, "skip cut detection above this duration (0 = never skip)")
	contextCmd.Flags().BoolVar(&ctxPNG, "png", false, "PNG sheet instead of JPEG")
	rootCmd.AddCommand(contextCmd)
}
