package cmd

import (
	"fmt"

	"github.com/iamnikolie/dziga/internal/media"
	"github.com/spf13/cobra"
)

var probeCmd = &cobra.Command{
	Use:   "probe <file|url>",
	Short: "Print what the clip is: duration, size, fps, codecs, audio",
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
		if jsonOutput {
			return emitJSON(info)
		}
		fmt.Fprintln(stdout, describeInfo(info))
		return nil
	},
}

// describeInfo is the one-line clip summary reused by probe and context.
func describeInfo(i *media.Info) string {
	w, h := i.DisplaySize()
	audio := "no audio"
	if i.HasAudio {
		audio = "audio " + i.AudioCodec
	}
	rot := ""
	if i.Rotation != 0 {
		rot = fmt.Sprintf(" rot=%d", i.Rotation)
	}
	return fmt.Sprintf("%s · %dx%d%s · %.3g fps · %s · %s · %s",
		media.FormatTS(i.Duration), w, h, rot, i.FPS, i.VideoCodec, audio, humanBytes(i.SizeBytes))
}

func init() {
	rootCmd.AddCommand(probeCmd)
}
