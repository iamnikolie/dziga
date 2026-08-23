package cmd

import "github.com/langgerone/dziga/internal/config"

// homeDir is the dziga root (~/.dziga), where cross-profile state like the
// upload cache lives.
func homeDir() (string, error) {
	return config.Home()
}
