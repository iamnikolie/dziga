package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/iamnikolie/dziga/internal/config"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage profiles (~/.dziga/<profile>/config.yaml)",
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Write a profile with a Gemini API key",
	RunE: func(cmd *cobra.Command, args []string) error {
		if profile == "" {
			return fmt.Errorf("--config <name> is required for config init")
		}
		existing, err := config.Load(profile)
		if err != nil {
			return err
		}
		key := os.Getenv("GEMINI_API_KEY")
		fmt.Fprint(stderr, "Gemini API key")
		if cur := existing.Providers["gemini"].APIKey; cur != "" {
			fmt.Fprintf(stderr, " [keep existing %s]", mask(cur))
		} else if key != "" {
			fmt.Fprintf(stderr, " [GEMINI_API_KEY %s]", mask(key))
		}
		fmt.Fprint(stderr, ": ")

		in := bufio.NewScanner(os.Stdin)
		entered := ""
		if in.Scan() {
			entered = strings.TrimSpace(in.Text())
		}
		switch {
		case entered != "":
			key = entered
		case existing.Providers["gemini"].APIKey != "":
			key = existing.Providers["gemini"].APIKey
		}
		if key == "" {
			return fmt.Errorf("no key given and GEMINI_API_KEY is unset")
		}

		existing.Providers["gemini"] = config.ProviderKeys{APIKey: key}
		if err := config.Save(existing, profile); err != nil {
			return err
		}
		path, _ := config.Path(profile)
		fmt.Fprintln(stdout, path)
		return nil
	},
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show the resolved profile (keys masked)",
	RunE: func(cmd *cobra.Command, args []string) error {
		path, _ := config.Path(profile)
		key := cfg.APIKey("gemini")
		source := "none"
		if cfg.Providers["gemini"].APIKey != "" {
			source = "config"
		} else if key != "" {
			source = "env"
		}
		home, _ := config.Home()
		if jsonOutput {
			return emitJSON(map[string]any{
				"profile": profile, "config_path": path, "home": home,
				"gemini_key": mask(key), "key_source": source,
				"default_model": reg.Default,
			})
		}
		fmt.Fprintf(stdout, "profile:       %s\n", orNone(profile))
		fmt.Fprintf(stdout, "config:        %s\n", path)
		fmt.Fprintf(stdout, "home:          %s\n", home)
		fmt.Fprintf(stdout, "gemini key:    %s (%s)\n", mask(key), source)
		fmt.Fprintf(stdout, "default model: %s\n", reg.Default)
		return nil
	},
}

func mask(s string) string {
	if s == "" {
		return "-"
	}
	if len(s) <= 8 {
		return "***"
	}
	return s[:4] + "***" + s[len(s)-4:]
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

func init() {
	configCmd.AddCommand(configInitCmd, configShowCmd)
	rootCmd.AddCommand(configCmd)
}
