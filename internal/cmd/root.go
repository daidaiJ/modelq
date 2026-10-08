package cmd

import (
	"github.com/spf13/cobra"
)

var (
	flagBaseURL string
	flagJSON    bool
)

// NewRootCmd builds the orx command tree; version is shown by --version.
func NewRootCmd(version string) *cobra.Command {
	root := &cobra.Command{
		Use:     "orx",
		Version: version,
		Short:   "Query OpenRouter model parameters and pricing",
		Long: `orx queries the OpenRouter model catalog for parameters and pricing,
enriched with the models.dev reference catalog.

Data sources:
  - OpenRouter public /models metadata endpoint. It is fully public:
    every command works without an API key.
  - models.dev (https://models.dev), a community-maintained catalog across
    providers, used to resolve approximate model ids and to show vendor
    reference pricing and capability parameters. Cached on disk for 24h;
    --refresh on 'dev' and 'show' forces a re-download, and the
    MODELSDEV_API_URL environment variable overrides the endpoint.

The base URL, if you need to override it, comes from --base-url,
OPENROUTER_BASE_URL, or base_url in the config file.

Every command is read-only and non-interactive. Use --json for
machine-readable output; tables are for humans.`,
		Example: `  orx list                          all models with pricing
  orx search gemini flash           filter by id or name
  orx show anthropic/claude-sonnet-4.5   one model in detail
  orx compare gpt-5-mini gpt-5-nano side-by-side comparison
  orx dev glm-5.3-flash             models.dev lookup for an approximate id`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.PersistentFlags().StringVar(&flagBaseURL, "base-url", "", "OpenRouter API base URL (default https://openrouter.ai/api/v1)")
	root.PersistentFlags().BoolVar(&flagJSON, "json", false, "emit raw JSON instead of a table")

	root.AddCommand(
		newListCmd(),
		newSearchCmd(),
		newShowCmd(),
		newCompareCmd(),
		newDevCmd(),
		newConfigPathCmd(),
	)
	return root
}
