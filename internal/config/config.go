package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// Config holds resolved settings.
type Config struct {
	BaseURL string
}

// Resolve determines the effective config. The base URL comes from, in
// order of precedence:
//  1. --base-url flag (flagBaseURL)
//  2. OPENROUTER_BASE_URL environment variable
//  3. base_url in the config file
//
// The config file is searched as openrouter-cli.(toml|yaml|json) under
// $XDG_CONFIG_HOME or ~/.config, and alongside the executable.
func Resolve(flagBaseURL string) (*Config, error) {
	v := viper.New()
	v.SetConfigName("openrouter-cli")
	v.SetConfigType("toml")
	v.AddConfigPath(filepath.Join(configHome(), "openrouter-cli"))
	v.AddConfigPath(configHome())
	v.AddConfigPath(".")

	v.SetEnvPrefix("OPENROUTER")
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	v.AutomaticEnv()

	// A missing config file is not an error; the CLI works with env/flags only.
	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !isConfigNotFound(err, &notFound) {
			return nil, fmt.Errorf("read config: %w", err)
		}
	}

	cfg := &Config{BaseURL: flagBaseURL}
	if cfg.BaseURL == "" {
		cfg.BaseURL = v.GetString("base_url")
	}
	return cfg, nil
}

// ConfigPath returns the preferred config file path for documentation purposes.
func ConfigPath() string {
	return filepath.Join(configHome(), "openrouter-cli", "openrouter-cli.toml")
}

func configHome() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return x
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, ".config")
}

func isConfigNotFound(err error, target *viper.ConfigFileNotFoundError) bool {
	for err != nil {
		if e, ok := err.(viper.ConfigFileNotFoundError); ok {
			*target = e
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}
