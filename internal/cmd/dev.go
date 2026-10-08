package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/daidaiJ/openrouter-cli/internal/format"
	"github.com/daidaiJ/openrouter-cli/internal/modelsdev"
)

func newDevCmd() *cobra.Command {
	var (
		refresh bool
		limit   int
	)
	c := &cobra.Command{
		Use:     "dev <query...>",
		Aliases: []string{"mdev"},
		Short:   "Look up model parameters and reference prices on models.dev",
		Long: `Look up model parameters and reference prices on models.dev.

models.dev is a community-maintained catalog covering hundreds of providers.
This command resolves an approximate model id (exact id, vendor/model, or a
fragment) against it and prints the entry's parameters, token limits, vendor
reference pricing, and provider integration details. Everything it needs is
public: no OpenRouter API key and no authentication of any kind.

Matching confidence, highest first: exact id in the openrouter provider,
exact id anywhere, canonical_model_id hit, vendor/model with provider aliasing
("z-ai" matches "zai"), then substring matches. An exact hit (or a single
match) prints full details; otherwise a comparison table lists the
candidates.

The catalog is cached on disk for 24 hours and is served from the cache when
models.dev is unreachable. --refresh forces a re-download. Set
MODELSDEV_API_URL to point at a different endpoint.

Pass "-" (or no arguments with piped input) to read queries from stdin, one
per line; each is resolved independently and a failed query does not abort
the batch.`,
		Example: `  orx dev claude-sonnet-4.5              exact OpenRouter-style id
  orx dev zhipuai/glm-5.3-flash          vendor/model id
  orx dev kimi-k3                        fuzzy: table of matches across providers
  orx dev sonnet 4.5                     multi-word query
  orx dev gpt-5.2 --json                 machine-readable output
  orx dev --refresh gpt-5.2              bypass the 24h cache
  echo kimi-k3 | orx dev -               read queries from stdin`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			queries, fromStdin, err := devQueries(args)
			if err != nil {
				return err
			}
			cat, err := loadModelsDev(cmd.Context(), refresh)
			if err != nil {
				return err
			}
			if flagJSON {
				return printJSON(devJSONResults(queries, cat))
			}
			return runDevText(queries, cat, limit, fromStdin)
		},
	}
	c.Flags().BoolVar(&refresh, "refresh", false, "re-download the models.dev catalog, ignoring the cache")
	c.Flags().IntVar(&limit, "limit", 20, "show at most N rows when several models match")
	return c
}

// devQueries normalizes arguments into queries, reading stdin when the only
// argument is "-" or when no arguments are given and stdin is piped.
func devQueries(args []string) (queries []string, fromStdin bool, err error) {
	rest := args
	if len(rest) == 1 && rest[0] == "-" {
		rest = nil
	}
	if len(rest) == 0 {
		stat, statErr := os.Stdin.Stat()
		if statErr == nil && stat.Mode()&os.ModeCharDevice != 0 {
			return nil, false, fmt.Errorf("no query given\n  orx dev <model-id>\n  orx dev <terms...>\n  echo <id> | orx dev -")
		}
		sc := bufio.NewScanner(os.Stdin)
		sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
		for sc.Scan() {
			for _, f := range strings.Fields(sc.Text()) {
				queries = append(queries, f)
			}
		}
		if err := sc.Err(); err != nil {
			return nil, true, fmt.Errorf("read stdin: %w", err)
		}
		if len(queries) == 0 {
			return nil, true, fmt.Errorf("stdin was empty\n  orx dev <model-id>\n  echo <id> | orx dev -")
		}
		return queries, true, nil
	}
	// Multi-word terms form one query: "sonnet 4.5" -> "sonnet-4.5".
	return []string{strings.Join(rest, "-")}, false, nil
}

func devJSONResults(queries []string, cat *modelsdev.Catalog) []map[string]any {
	out := make([]map[string]any, 0, len(queries))
	for _, q := range queries {
		matches := cat.Match(q)
		entries := make([]map[string]any, 0, len(matches))
		for _, m := range matches {
			entries = append(entries, map[string]any{
				"provider": cat.Info(m.ProviderID),
				"model":    m.Model,
				"source":   m.Source,
				"score":    m.Score,
			})
		}
		out = append(out, map[string]any{"query": q, "matches": entries})
	}
	return out
}

func runDevText(queries []string, cat *modelsdev.Catalog, limit int, fromStdin bool) error {
	failed := 0
	for i, q := range queries {
		if i > 0 {
			fmt.Println()
		}
		if fromStdin {
			fmt.Printf("query: %s\n", q)
		}
		matches := cat.Match(q)
		switch {
		case len(matches) == 0:
			fmt.Fprintf(os.Stderr, "no models.dev entry matches %q (try a broader term, e.g. orx dev sonnet)\n", q)
			failed++
		case matches[0].Score >= 95 || len(matches) == 1:
			// Exact OpenRouter-style hit (or the only match): straight to details.
			fmt.Print(renderDevDetail(cat, matches[0]))
		default:
			fmt.Print(renderDevTable(matches, limit))
			if top := matches[0].Model.ID; !fromStdin && !strings.EqualFold(top, q) {
				fmt.Printf("\n%d match(es); run 'orx dev %s' for details on the closest one\n", len(matches), top)
			}
		}
	}
	if failed > 0 && failed == len(queries) {
		return fmt.Errorf("no models.dev entry matched any query")
	}
	return nil
}

// ptrPrice maps a nil price to -1 so format.Price renders "-".
func ptrPrice(p *float64) float64 {
	if p == nil {
		return -1
	}
	return *p
}

func renderDevDetail(cat *modelsdev.Catalog, m modelsdev.Match) string {
	md := m.Model
	info := cat.Info(m.ProviderID)

	var b strings.Builder
	row := func(k, v string) {
		if v == "" || v == "-" {
			return
		}
		fmt.Fprintf(&b, "%-20s %s\n", k, v)
	}
	yesNo := func(v *bool) string {
		if v == nil {
			return ""
		}
		return strconv.FormatBool(*v)
	}

	row("id", md.ID)
	row("provider", fmt.Sprintf("%s (%s)", info.ID, info.Name))
	row("name", md.Name)
	row("status", md.Status)
	row("family", md.Family)
	if md.CanonicalModelID != "" && md.CanonicalModelID != md.ID {
		row("canonical_model_id", md.CanonicalModelID)
	}
	row("context", format.TokensExact(md.Limit.Context)+" ("+format.Tokens(md.Limit.Context)+")")
	if md.Limit.Input > 0 && md.Limit.Input != md.Limit.Context {
		row("max_input", format.TokensExact(md.Limit.Input)+" ("+format.Tokens(md.Limit.Input)+")")
	}
	row("max_output", format.TokensExact(md.Limit.Output)+" ("+format.Tokens(md.Limit.Output)+")")
	if len(md.Modalities.Input) > 0 {
		row("input", strings.Join(md.Modalities.Input, ", "))
	}
	if len(md.Modalities.Output) > 0 {
		row("output", strings.Join(md.Modalities.Output, ", "))
	}
	if md.Attachment {
		row("attachments", "true")
	}
	row("tool_call", yesNo(md.ToolCall))
	row("structured_output", yesNo(md.StructuredOutput))
	row("temperature", yesNo(md.Temperature))
	if opt := reasoningLabel(md); opt != "" {
		row("reasoning", opt)
	}
	if field, ok := md.InterleavedField(); ok {
		row("interleaved", field)
	}
	if modes, ok := md.ExperimentalModes(); ok {
		row("experimental_modes", strings.Join(modes, ", "))
	}
	row("open_weights", yesNo(md.OpenWeights))
	row("release_date", md.ReleaseDate)
	row("last_updated", md.LastUpdated)
	row("knowledge", md.Knowledge)

	b.WriteString("\npricing (USD per 1M tokens, vendor reference)\n")
	price := func(k string, p *float64) {
		if p == nil {
			return
		}
		fmt.Fprintf(&b, "  %-18s %s\n", k, format.Price(*p))
	}
	price("input", md.Cost.Input)
	price("output", md.Cost.Output)
	price("cache_read", md.Cost.CacheRead)
	price("cache_write", md.Cost.CacheWrite)
	price("reasoning", md.Cost.Reasoning)
	price("audio_in", md.Cost.InputAudio)
	price("audio_out", md.Cost.OutputAudio)
	for _, t := range md.Cost.Tiers {
		parts := []string{
			"in " + format.Price(ptrPrice(t.Input)),
			"out " + format.Price(ptrPrice(t.Output)),
			"cache " + format.Price(ptrPrice(t.CacheRead)),
		}
		fmt.Fprintf(&b, "  %-18s ctx >= %s: %s\n", "tiers", format.Tokens(t.Tier.Size), strings.Join(parts, ", "))
	}

	integration := func(k, v string) {
		if v != "" {
			fmt.Fprintf(&b, "%-20s %s\n", k, v)
		}
	}
	integration("npm", info.NPM)
	integration("api", info.API)
	integration("docs", info.Doc)
	if len(info.Env) > 0 {
		integration("env", strings.Join(info.Env, ", "))
	}
	fmt.Fprintf(&b, "%-20s models.dev %s\n", "source", cat.FetchedAt.Format("2006-01-02 15:04 MST"))
	return b.String()
}

// reasoningLabel renders the reasoning capability plus its controls as
// "true (effort: low, high)"; empty when the source lists nothing.
func reasoningLabel(m modelsdev.Model) string {
	opts := reasoningOptions(m.ReasoningOptions)
	switch {
	case m.Reasoning && opts != "":
		return "true (" + opts + ")"
	case m.Reasoning:
		return "true"
	default:
		return opts
	}
}

// reasoningOptions renders the list as "effort: low/high; toggle".
func reasoningOptions(opts []modelsdev.ReasoningOption) string {
	if len(opts) == 0 {
		return ""
	}
	parts := make([]string, 0, len(opts))
	for _, o := range opts {
		switch {
		case len(o.Values) > 0:
			parts = append(parts, o.Type+": "+strings.Join(o.Values, "/"))
		case o.Min != nil || o.Max != nil:
			parts = append(parts, fmt.Sprintf("%s: %s-%s", o.Type,
				formatPriceOpt(o.Min), formatPriceOpt(o.Max)))
		default:
			parts = append(parts, o.Type)
		}
	}
	return strings.Join(parts, "; ")
}

func formatPriceOpt(f *float64) string {
	if f == nil {
		return "?"
	}
	return strconv.FormatFloat(*f, 'f', -1, 64)
}

func renderDevTable(matches []modelsdev.Match, limit int) string {
	if limit > 0 && len(matches) > limit {
		matches = matches[:limit]
	}
	headers := []string{"PROVIDER", "MODEL ID", "NAME", "CTX", "IN/M", "OUT/M", "STATUS"}
	rows := make([][]string, 0, len(matches))
	for _, m := range matches {
		rows = append(rows, []string{
			format.Truncate(m.ProviderID, 22),
			format.Truncate(m.Model.ID, 40),
			format.Truncate(m.Model.Name, 24),
			format.Tokens(m.Model.Limit.Context),
			format.Price(ptrPrice(m.Model.Cost.Input)),
			format.Price(ptrPrice(m.Model.Cost.Output)),
			m.Model.Status,
		})
	}
	return format.Table(headers, rows)
}
