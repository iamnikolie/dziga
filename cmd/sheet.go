package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/langgerone/dziga/internal/media"
	"github.com/langgerone/dziga/internal/sheet"
	"github.com/spf13/cobra"
)

var (
	sheetN       int
	sheetFrom    string
	sheetTo      string
	sheetScenes  bool
	sheetThresh  float64
	sheetCols    int
	sheetMaxEdge int
	sheetPNG     bool
	sheetQuality int
	sheetName    string
	sheetKeep    bool
)

var sheetCmd = &cobra.Command{
	Use:   "sheet <file|url>",
	Short: "Contact sheet of timecoded frames — the cheap way to see a clip",
	Long: `Build one labelled contact sheet from the clip and print its path.

Every cell carries its index and timestamp, so after looking at the sheet you can
ask for any moment full-size: dziga frames <src> --at 12.4`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := ctxWithTimeout(cmd)
		defer cancel()
		start := time.Now()

		src, info, dir, err := prepare(ctx, args[0])
		if err != nil {
			return err
		}
		times, mode, err := resolveTimes(ctx, src, info, sampleOpts{
			n: sheetN, from: sheetFrom, to: sheetTo,
			useScenes: sheetScenes, threshold: sheetThresh,
		})
		if err != nil {
			return err
		}
		if len(times) == 0 {
			return fmt.Errorf("no frames to sample")
		}

		// Solve the grid first: the cell width is what the frames get scaled to,
		// so the sheet lands right under the model's downscale cap.
		lay := sheet.Solve(len(times), info.Aspect(), sheetCols, sheetMaxEdge, 4)
		if lay.CellW < 120 {
			statusf("warn: %d cells → %dpx wide each; detail and burned timecodes get hard to read (use --n fewer, or --max-edge bigger)", len(times), lay.CellW)
		}

		tmp := filepath.Join(dir, ".cells")
		frames, err := media.ExtractFrames(ctx, src.Path, times, media.ExtractOpts{
			Dir: tmp, Prefix: "cell", Width: lay.CellW, PNG: sheetPNG,
		})
		if err != nil {
			return err
		}
		if !sheetKeep {
			defer os.RemoveAll(tmp)
		}

		cells := make([]sheet.Cell, len(frames))
		for i, f := range frames {
			cells[i] = sheet.Cell{Path: f.Path, Label: fmt.Sprintf("%02d %s", f.Index, media.FormatTS(f.T))}
		}

		ext := "jpg"
		if sheetPNG {
			ext = "png"
		}
		name := sheetName
		if name == "" {
			name = fmt.Sprintf("sheet-%s-%d", mode, len(cells))
		}
		outPath := filepath.Join(dir, name+"."+ext)

		res, err := sheet.Compose(cells, info.Aspect(), outPath, sheet.Opts{
			MaxEdge: sheetMaxEdge, Cols: sheetCols, PNG: sheetPNG, Quality: sheetQuality,
		})
		if err != nil {
			return err
		}

		statusf("sheet=%d frames mode=%s grid=%dx%d %dx%d %s ~%s img-tokens ms=%d",
			res.Cells, mode, res.Cols, res.Rows, res.Width, res.Height,
			humanBytes(res.Bytes), humanTokens(res.Tokens), time.Since(start).Milliseconds())

		if jsonOutput {
			return emitJSON(map[string]any{
				"sheet":  res,
				"mode":   mode,
				"source": src.Path,
				"frames": frameTable(frames),
			})
		}
		fmt.Fprintln(stdout, res.Path)
		return nil
	},
}

// prepare resolves the source, probes it and picks the output directory —
// the opening move of every artifact-producing command.
func prepare(ctx context.Context, arg string) (*Source, *media.Info, string, error) {
	src, err := resolveSource(ctx, arg)
	if err != nil {
		return nil, nil, "", err
	}
	info, err := media.Probe(ctx, src.Path)
	if err != nil {
		return nil, nil, "", err
	}
	dir, err := workDir(src)
	if err != nil {
		return nil, nil, "", err
	}
	return src, info, dir, nil
}

func frameTable(frames []media.Frame) []map[string]any {
	out := make([]map[string]any, len(frames))
	for i, f := range frames {
		out[i] = map[string]any{"index": f.Index, "t_sec": f.T, "ts": media.FormatTS(f.T)}
	}
	return out
}

func init() {
	sheetCmd.Flags().IntVarP(&sheetN, "n", "n", 16, "number of frames on the sheet")
	sheetCmd.Flags().StringVar(&sheetFrom, "from", "", "window start (e.g. 12, 1:30, 90s)")
	sheetCmd.Flags().StringVar(&sheetTo, "to", "", "window end")
	sheetCmd.Flags().BoolVar(&sheetScenes, "scenes", false, "sample one frame per detected cut instead of evenly")
	sheetCmd.Flags().Float64Var(&sheetThresh, "threshold", 0.3, "scene sensitivity when --scenes is set")
	sheetCmd.Flags().IntVar(&sheetCols, "cols", 0, "force column count (default: square-ish for the source aspect)")
	sheetCmd.Flags().IntVar(&sheetMaxEdge, "max-edge", 1568, "long-edge cap in px; 1568 is where Claude stops downscaling")
	sheetCmd.Flags().BoolVar(&sheetPNG, "png", false, "PNG instead of JPEG (sharper for text/UI heavy video)")
	sheetCmd.Flags().IntVar(&sheetQuality, "quality", 88, "JPEG quality 1..100")
	sheetCmd.Flags().StringVar(&sheetName, "name", "", "output basename (no extension)")
	sheetCmd.Flags().BoolVar(&sheetKeep, "keep-cells", false, "keep the individual scaled cell images")
	rootCmd.AddCommand(sheetCmd)
}
