package cmd

import (
	"fmt"
	"time"

	"github.com/langgerone/dziga/internal/media"
	"github.com/spf13/cobra"
)

var (
	framesAt     string
	framesEvery  string
	framesN      int
	framesFrom   string
	framesTo     string
	framesScenes bool
	framesThresh float64
	framesWidth  int
	framesPNG    bool
)

var framesCmd = &cobra.Command{
	Use:   "frames <file|url>",
	Short: "Write full-size stills at chosen moments and print their paths",
	Long: `Extract stills at exact timestamps.

This is the zoom-in step: read a sheet first, then pull the two or three moments
that matter at full resolution.`,
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
			n: framesN, from: framesFrom, to: framesTo, at: framesAt,
			every: framesEvery, useScenes: framesScenes, threshold: framesThresh,
		})
		if err != nil {
			return err
		}

		frames, err := media.ExtractFrames(ctx, src.Path, times, media.ExtractOpts{
			Dir: dir, Prefix: "frame", Width: framesWidth, PNG: framesPNG,
		})
		if err != nil {
			return err
		}

		w, h := info.DisplaySize()
		if framesWidth > 0 {
			h = h * framesWidth / max(w, 1)
			w = framesWidth
		}
		statusf("frames=%d mode=%s %dx%d ~%s img-tokens each ms=%d",
			len(frames), mode, w, h, humanTokens(w*h/750), time.Since(start).Milliseconds())

		if jsonOutput {
			return emitJSON(map[string]any{"source": src.Path, "mode": mode, "frames": frames})
		}
		for _, f := range frames {
			fmt.Fprintln(stdout, f.Path)
		}
		return nil
	},
}

func init() {
	framesCmd.Flags().StringVar(&framesAt, "at", "", "comma-separated timestamps (e.g. 3.5,12,1:04)")
	framesCmd.Flags().StringVar(&framesEvery, "every", "", "one frame every N (e.g. 5s, 1m)")
	framesCmd.Flags().IntVarP(&framesN, "n", "n", 6, "number of evenly spaced frames when no --at/--every/--scenes")
	framesCmd.Flags().StringVar(&framesFrom, "from", "", "window start")
	framesCmd.Flags().StringVar(&framesTo, "to", "", "window end")
	framesCmd.Flags().BoolVar(&framesScenes, "scenes", false, "one frame per detected cut")
	framesCmd.Flags().Float64Var(&framesThresh, "threshold", 0.3, "scene sensitivity when --scenes is set")
	framesCmd.Flags().IntVar(&framesWidth, "width", 0, "scale width in px (0 = source resolution)")
	framesCmd.Flags().BoolVar(&framesPNG, "png", false, "PNG instead of JPEG")
	rootCmd.AddCommand(framesCmd)
}
