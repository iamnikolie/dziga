package cmd

import (
	"fmt"

	"github.com/langgerone/dziga/internal/media"
	"github.com/spf13/cobra"
)

var (
	scenesThreshold float64
	scenesScale     int
)

var scenesCmd = &cobra.Command{
	Use:   "scenes <file|url>",
	Short: "List detected shot changes (seconds)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := ctxWithTimeout(cmd)
		defer cancel()

		src, err := resolveSource(ctx, args[0])
		if err != nil {
			return err
		}
		info, err := media.Probe(ctx, src.Path)
		if err != nil {
			return err
		}
		cuts, err := media.DetectScenes(ctx, src.Path, scenesThreshold, scenesScale)
		if err != nil {
			return err
		}
		statusf("scenes=%d threshold=%.2f duration=%s", len(cuts), scenesThreshold, media.FormatTS(info.Duration))

		if jsonOutput {
			return emitJSON(map[string]any{
				"path":         src.Path,
				"duration_sec": info.Duration,
				"threshold":    scenesThreshold,
				"cuts_sec":     cuts,
			})
		}
		for _, c := range cuts {
			fmt.Fprintf(stdout, "%.3f\t%s\n", c, media.FormatTS(c))
		}
		return nil
	},
}

func init() {
	scenesCmd.Flags().Float64Var(&scenesThreshold, "threshold", 0.3, "scene-change sensitivity 0..1 (lower = more cuts)")
	scenesCmd.Flags().IntVar(&scenesScale, "scale", 320, "detector working width in px (a cut is a global change; full res buys nothing)")
	rootCmd.AddCommand(scenesCmd)
}
