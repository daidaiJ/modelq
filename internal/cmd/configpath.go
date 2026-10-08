package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/daidaiJ/modelq/internal/config"
	"github.com/daidaiJ/modelq/internal/locale"
)

// newConfigPathCmd prints where the config file is expected, for troubleshooting.
func newConfigPathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "config-path",
		Short: locale.T("Print the config file path the CLI looks for", "输出 CLI 查找的配置文件路径"),
		Example: `  mqx config-path
  mqx config-path --json`,
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
