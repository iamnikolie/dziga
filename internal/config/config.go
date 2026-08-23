package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ProviderKeys holds the API key for one provider.
type ProviderKeys struct {
	APIKey string `yaml:"api_key"`
}

// Config is the per-profile dziga configuration. Everything in it is optional:
// the local ffmpeg commands need no config at all, and `ask` falls back to
// GEMINI_API_KEY / GOOGLE_API_KEY when no profile exists.
type Config struct {
	DefaultModel string                  `yaml:"default_model,omitempty"`
	Providers    map[string]ProviderKeys `yaml:"providers,omitempty"`
}

// APIKey resolves a provider key: config file first, then environment.
func (c *Config) APIKey(provider string) string {
	if c != nil && c.Providers != nil {
		if p, ok := c.Providers[provider]; ok && p.APIKey != "" {
			return p.APIKey
		}
	}
	if provider == "gemini" {
		return firstEnv("GEMINI_API_KEY", "GOOGLE_API_KEY")
	}
	return ""
}

func firstEnv(names ...string) string {
	for _, n := range names {
		if v := os.Getenv(n); v != "" {
			return v
		}
	}
	return ""
}

// Home returns the dziga root: $DZIGA_HOME or ~/.dziga.
func Home() (string, error) {
	if h := os.Getenv("DZIGA_HOME"); h != "" {
		return h, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("config.Home: %w", err)
	}
	return filepath.Join(home, ".dziga"), nil
}

// ProfileDir returns the directory for a profile ("" → the root itself).
func ProfileDir(profile string) (string, error) {
	base, err := Home()
	if err != nil {
		return "", err
	}
	if profile != "" {
		return filepath.Join(base, profile), nil
	}
	return base, nil
}

// SubDir returns (and creates) a directory under the dziga root, e.g. work/, cache/.
func SubDir(name string) (string, error) {
	base, err := Home()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("config.SubDir: %w", err)
	}
	return dir, nil
}

// Load reads a profile's config. A missing file is not an error — it returns an
// empty Config so environment keys still work.
func Load(profile string) (*Config, error) {
	dir, err := ProfileDir(profile)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(dir, "config.yaml"))
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{Providers: map[string]ProviderKeys{}}, nil
		}
		return nil, fmt.Errorf("config.Load: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("config.Load: parse: %w", err)
	}
	if cfg.Providers == nil {
		cfg.Providers = map[string]ProviderKeys{}
	}
	return &cfg, nil
}

// Path returns the config file path for a profile.
func Path(profile string) (string, error) {
	dir, err := ProfileDir(profile)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// Save writes the profile config (dir 0700, file 0600).
func Save(cfg *Config, profile string) error {
	dir, err := ProfileDir(profile)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("config.Save: mkdir: %w", err)
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("config.Save: marshal: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), data, 0600); err != nil {
		return fmt.Errorf("config.Save: %w", err)
	}
	return nil
}
