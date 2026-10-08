package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/daidaiJ/openrouter-cli/internal/config"
)

// newConfigPathCmd prints where the config file is expected, for troubleshooting.
func newConfigPathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "config-path",
		Short: "Print the config file path the CLI looks for",
		Example: `  orx config-path
  orx config-path --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			p := config.ConfigPath()
			if flagJSON {
				return printJSON(map[string]string{"config_path": p})
			}
			fmt.Println(p)
			return nil
		},
	}
}
