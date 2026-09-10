package cmd

import (
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/iamnikolie/dziga/internal/registry"
	"github.com/spf13/cobra"
)

var modelsCmd = &cobra.Command{
	Use:   "models",
	Short: "Print the resolved model registry, with the rates in force today",
	RunE: func(cmd *cobra.Command, args []string) error {
		if jsonOutput {
			return emitJSON(reg)
		}
		now := time.Now()
		w := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ALIAS\tAPI ID\tIN $/1M\tOUT $/1M\tNOTE")
		for _, alias := range reg.Aliases() {
			m := reg.Models[alias]
			billing, exact := reg.Billing(alias, m, "")
			in, out := "?", "?"
			if p, ok := billing.PricesAt(now, 0); ok {
				mark := ""
				if !exact {
					mark = "~" // resolved through a "-latest" alias assumption
				}
				in = fmt.Sprintf("%s%.2f", mark, p.In)
				out = fmt.Sprintf("%s%.2f", mark, p.Out)
			}
			star := ""
			if alias == reg.Default {
				star = " (default)"
			}
			fmt.Fprintf(w, "%s%s\t%s\t%s\t%s\t%s\n", alias, star, m.ID, in, out, note(m))
		}
		if err := w.Flush(); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "\nvideo input, measured: %d tokens/frame (--res high: %d), audio %.0f tokens/s\n",
			reg.Video.TokensPerFrame["default"], reg.Video.TokensPerFrame["high"], reg.Video.AudioTokensPerSec)
		fmt.Fprintln(stdout, "~ = rate assumed from the model a \"-latest\" alias served last; the real one is billed by modelVersion")
		return nil
	},
}

func note(m registry.Model) string {
	if m.Note != "" {
		return m.Note
	}
	if m.PriceUntil != "" {
		return "intro rate until " + m.PriceUntil
	}
	return ""
}

func init() {
	rootCmd.AddCommand(modelsCmd)
}
