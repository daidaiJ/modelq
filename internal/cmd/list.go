package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/daidaiJ/openrouter-cli/internal/api"
	"github.com/daidaiJ/openrouter-cli/internal/config"
)

// clientFromFlags resolves config and builds an API client.
func clientFromFlags() (*api.Client, error) {
	cfg, err := config.Resolve(flagBaseURL)
	if err != nil {
		return nil, err
	}
	return api.NewClient(cfg.BaseURL), nil
}

func newListCmd() *cobra.Command {
	var (
		sortKey    string
		desc       bool
		limit      int
		freeOnly   bool
		minContext int64
		modality   string
	)
	c := &cobra.Command{
		Use:   "list",
		Short: "List all models with context, max output and pricing",
		Example: `  orx list                                 all models, sorted by id
  orx list --sort output --desc --limit 20  priciest output tokens first
  orx list --free --min-ctx 100000          free models with 100K+ context
  orx list --modality image --json          image-in models, machine-readable`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cl, err := clientFromFlags()
			if err != nil {
				return err
			}
			models, err := fetchModels(cmd.Context(), cl)
			if err != nil {
				return err
			}
			total := len(models)
			models = filterModels(models, filterOpts{FreeOnly: freeOnly, MinContext: minContext, Modality: modality})
			sortModels(models, sortKey, desc)
			if flagJSON {
				return printJSON(models)
			}
			if len(models) == 0 {
				fmt.Fprintln(os.Stderr, "no models matched the filters")
				return nil
			}
			fmt.Println(renderList(models, limit))
			fmt.Printf("\n%d of %d model(s)\n", shown(len(models), limit), total)
			return nil
		},
	}
	c.Flags().StringVar(&sortKey, "sort", "", "sort by: id, name, input, output, ctx, maxout")
	c.Flags().BoolVar(&desc, "desc", false, "sort descending")
	c.Flags().IntVar(&limit, "limit", 0, "show at most N rows (0 = all)")
	c.Flags().BoolVar(&freeOnly, "free", false, "only free models")
	c.Flags().Int64Var(&minContext, "min-ctx", 0, "only models with at least this many context tokens")
	c.Flags().StringVar(&modality, "modality", "", "filter by modality substring, e.g. image or audio")
	return c
}

func newSearchCmd() *cobra.Command {
	var (
		sortKey string
		desc    bool
		limit   int
	)
	c := &cobra.Command{
		Use:   "search <query...>",
		Short: "Search models by id or name",
		Example: `  orx search gemini flash            terms are ANDed on id and name
  orx search glm --sort ctx --desc   largest-context GLM models first
  orx search sonnet --json`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := clientFromFlags()
			if err != nil {
				return err
			}
			models, err := fetchModels(cmd.Context(), cl)
			if err != nil {
				return err
			}
			query := strings.Join(args, " ")
			models = searchModels(models, query)
			sortModels(models, sortKey, desc)
			if flagJSON {
				return printJSON(models)
			}
			if len(models) == 0 {
				return fmt.Errorf("no model matches %q", query)
			}
			fmt.Println(renderList(models, limit))
			fmt.Printf("\n%d match(es)\n", shown(len(models), limit))
			return nil
		},
	}
	c.Flags().StringVar(&sortKey, "sort", "", "sort by: id, name, input, output, ctx, maxout")
	c.Flags().BoolVar(&desc, "desc", false, "sort descending")
	c.Flags().IntVar(&limit, "limit", 0, "show at most N rows (0 = all)")
	return c
}

// shown reports how many rows the table will print after the limit is applied.
func shown(n, limit int) int {
	if limit > 0 && limit < n {
		return limit
	}
	return n
}
