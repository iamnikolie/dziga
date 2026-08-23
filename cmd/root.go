package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"time"

	"github.com/langgerone/dziga/internal/client"
	"github.com/langgerone/dziga/internal/config"
	"github.com/langgerone/dziga/internal/registry"
	"github.com/spf13/cobra"
)

// version is the base CLI version; overridden via ldflags -X cmd.version=.
var version = "0.1.0"

func buildVersion() string {
	v := version
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" {
				rev := s.Value
				if len(rev) > 12 {
					rev = rev[:12]
				}
				v += " (" + rev + ")"
			}
		}
	}
	return v
}

var (
	profile     string
	outDir      string
	jsonOutput  bool
	verbose     bool
	timeoutFlag time.Duration

	cfg  *config.Config
	reg  *registry.Registry
	http *client.Client
)

var stderr io.Writer = os.Stderr
var stdout io.Writer = os.Stdout

var rootCmd = &cobra.Command{
	Use:   "dziga",
	Short: "Turn video into context an LLM can actually read",
	Long: `dziga — the kino-eye for an agent that cannot watch video.

Local commands (probe, scenes, sheet, frames) are pure ffmpeg: no key, no network,
no cost. They write labelled stills to disk and print their absolute paths, which
the agent then Reads. 'ask' delegates to Gemini's native video understanding.

Run 'dziga skill' for the full agent reference.`,
	SilenceUsage:  true,
	SilenceErrors: true, // Execute() prints the error once, on stderr
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		var err error
		cfg, err = config.Load(profile)
		if err != nil {
			return err
		}
		dir, err := config.ProfileDir(profile)
		if err != nil {
			return err
		}
		reg, err = registry.Load(dir)
		if err != nil {
			return err
		}
		http = client.New()
		http.Verbose = verbose
		return nil
	},
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.ExecuteContext(context.Background()); err != nil {
		fmt.Fprintln(stderr, "error:", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&profile, "config", os.Getenv("DZIGA_CONFIG"), "profile under ~/.dziga/ (optional; env keys work without one)")
	rootCmd.PersistentFlags().StringVar(&outDir, "out", "", "output directory (default: a per-video work dir under ~/.dziga/work/)")
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "emit one JSON object on stdout instead of the plain form")
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "dump ffmpeg/HTTP detail to stderr (keys redacted)")
	rootCmd.PersistentFlags().DurationVar(&timeoutFlag, "timeout", 10*time.Minute, "overall deadline for the command")
	rootCmd.Version = buildVersion()
}

// ctxWithTimeout applies --timeout to the command context.
func ctxWithTimeout(cmd *cobra.Command) (context.Context, context.CancelFunc) {
	if timeoutFlag <= 0 {
		return context.WithCancel(cmd.Context())
	}
	return context.WithTimeout(cmd.Context(), timeoutFlag)
}

func statusf(format string, a ...any) {
	fmt.Fprintf(stderr, format+"\n", a...)
}

func emitJSON(v any) error {
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// humanBytes renders a size the way a status line should read.
func humanBytes(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1fMB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0fKB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%dB", n)
}

// humanTokens renders a token estimate compactly (1600 → "1.6k").
func humanTokens(n int) string {
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}
