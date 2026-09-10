package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show the dziga version",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Fprintf(stdout, "dziga version %s\n", buildVersion())
		return nil
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
