package cmd

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/daidaiJ/openrouter-cli/internal/api"
	"github.com/daidaiJ/openrouter-cli/internal/format"
	"github.com/daidaiJ/openrouter-cli/internal/modelsdev"
)

// showOutput is the --json envelope: the OpenRouter model plus the enriched
// models.dev reference when a confident match exists.
type showOutput struct {
	Model     *api.Model       `json:"model"`
	ModelsDev *modelsdev.Match `json:"models_dev,omitempty"`
}

func newShowCmd() *cobra.Command {
	var refresh bool
	c := &cobra.Command{
		Use:   "show <model-id>...",
		Short: "Show full details for one model (id may be a unique prefix)",
		Long: `Show full details for one or more OpenRouter models.

The id may be exact, a unique prefix, or a substring; ambiguous prefixes list
the candidates. OpenRouter data is enriched with a "models.dev reference"
section (vendor reference pricing, capability parameters, release info)
whenever models.dev has a confident match for the id; that section is skipped
when models.dev is unreachable or the id is too fuzzy to match.

Pass "-" to read ids from stdin, one per line.`,
		Example: `  orx show anthropic/claude-sonnet-4.5     exact id
  orx show sonnet-4.5                      unique prefix match
  orx show gpt-5.2 --json                  machine-readable, includes models_dev
  orx show --refresh deepseek/deepseek-v4.1-flash   also refresh the models.dev cache
  printf 'a\nb\n' | orx show -             read ids from stdin`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ids, fromStdin, err := showIDs(args)
			if err != nil {
				return err
			}
			cl, err := clientFromFlags()
			if err != nil {
				return err
			}
			models, err := fetchModels(cmd.Context(), cl)
			if err != nil {
				return err
			}
			var cat *modelsdev.Catalog
			cat, catErr := loadModelsDev(cmd.Context(), refresh)
			if catErr != nil {
				fmt.Fprintf(os.Stderr, "models.dev unavailable (%v); showing OpenRouter data only\n", catErr)
			}

			out := make([]showOutput, 0, len(ids))
			var text strings.Builder
			wrote := false
			failed := 0
			for _, id := range ids {
				m, err := matchModel(models, id)
				if err != nil {
					if fromStdin {
						fmt.Fprintf(os.Stderr, "%s: %v\n", id, err)
						failed++
						continue
					}
					return err
				}
				ref := modelsDevRef(cat, m.ID)
				out = append(out, showOutput{Model: m, ModelsDev: ref})
				if !flagJSON {
					if wrote {
						text.WriteString("\n")
					}
					wrote = true
					text.WriteString(renderDetail(m))
					if ref != nil {
						text.WriteString(renderMDRef(cat, *ref))
					}
				}
			}
			if fromStdin && failed > 0 && failed == len(ids) {
				return fmt.Errorf("no id from stdin matched a model")
			}

			if flagJSON {
				if fromStdin || len(ids) > 1 {
					return printJSON(out)
				}
				return printJSON(out[0])
			}
			fmt.Print(text.String())
			return nil
		},
	}
	c.Flags().BoolVar(&refresh, "refresh", false, "re-download the models.dev catalog before enriching")
	return c
}

// showIDs normalizes arguments into ids, reading stdin when the only
// argument is "-" or when no arguments are given and stdin is piped.
func showIDs(args []string) (ids []string, fromStdin bool, err error) {
	rest := args
	if len(rest) == 1 && rest[0] == "-" {
		rest = nil
	}
	if len(rest) == 0 {
		stat, statErr := os.Stdin.Stat()
		if statErr == nil && stat.Mode()&os.ModeCharDevice != 0 {
			return nil, false, fmt.Errorf("no model id given\n  orx show <model-id>\n  printf '<id>\\n' | orx show -")
		}
		sc := bufio.NewScanner(os.Stdin)
		sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
		for sc.Scan() {
			for _, f := range strings.Fields(sc.Text()) {
				ids = append(ids, f)
			}
		}
		if err := sc.Err(); err != nil {
			return nil, true, fmt.Errorf("read stdin: %w", err)
		}
		if len(ids) == 0 {
			return nil, true, fmt.Errorf("stdin was empty\n  orx show <model-id>\n  printf '<id>\\n' | orx show -")
		}
		return ids, true, nil
	}
	return rest, false, nil
}

// renderMDRef renders the models.dev enrichment section: vendor reference
// pricing and parameters the OpenRouter catalog does not carry.
func renderMDRef(cat *modelsdev.Catalog, ref modelsdev.Match) string {
	md := ref.Model
	info := cat.Info(ref.ProviderID)

	var b strings.Builder
	fmt.Fprintf(&b, "\nmodels.dev reference (%s / %s", ref.ProviderID, md.ID)
	if info.Name != "" {
		fmt.Fprintf(&b, ", %s", info.Name)
	}
	b.WriteString(")\n")

	row := func(k, v string) {
		if v == "" || v == "-" {
			return
		}
		fmt.Fprintf(&b, "  %-18s %s\n", k, v)
	}
	yesNo := func(v *bool) string {
		if v == nil {
			return ""
		}
		return strconv.FormatBool(*v)
	}
	row("status", md.Status)
	row("release_date", md.ReleaseDate)
	row("open_weights", yesNo(md.OpenWeights))
	row("tool_call", yesNo(md.ToolCall))
	row("structured_output", yesNo(md.StructuredOutput))
	row("temperature", yesNo(md.Temperature))
	if rl := reasoningLabel(md); rl != "" {
		row("reasoning", rl)
	}
	if md.Limit.Output > 0 {
		row("max_output", format.TokensExact(md.Limit.Output)+" ("+format.Tokens(md.Limit.Output)+")")
	}
	if md.Limit.Input > 0 {
		row("max_input", format.TokensExact(md.Limit.Input)+" ("+format.Tokens(md.Limit.Input)+")")
	}

	price := func(k string, p *float64) {
		if p == nil {
			return
		}
		fmt.Fprintf(&b, "  %-18s %s\n", k, format.Price(*p))
	}
	b.WriteString("  vendor reference pricing (USD per 1M tokens)\n")
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
	if info.Doc != "" {
		row("docs", info.Doc)
	}
	return b.String()
}

// renderDetail renders one model as aligned label/value lines.
func renderDetail(m *api.Model) string {
	var b strings.Builder
	row := func(k, v string) {
		if v == "" || v == "-" {
			return
		}
		fmt.Fprintf(&b, "%-18s %s\n", k, v)
	}

	row("id", m.ID)
	row("name", m.Name)
	if m.CanonicalSlug != "" && m.CanonicalSlug != m.ID {
		row("canonical_slug", m.CanonicalSlug)
	}
	if m.AliasTarget != nil && m.AliasTarget.Slug != "" {
		row("alias_of", fmt.Sprintf("%s (%s)", m.AliasTarget.Slug, m.AliasTarget.Name))
	}
	row("context_length", fmt.Sprintf("%s (%s)", format.TokensExact(m.ContextLength), format.Tokens(m.ContextLength)))
	row("max_output", fmt.Sprintf("%s (%s)", format.TokensExact(m.TopProvider.MaxCompletionTokens), format.Tokens(m.TopProvider.MaxCompletionTokens)))
	row("modality", m.Architecture.Modality)
	if len(m.Architecture.InputModalities) > 0 {
		row("input", strings.Join(m.Architecture.InputModalities, ", "))
	}
	if len(m.Architecture.OutputModalities) > 0 {
		row("output", strings.Join(m.Architecture.OutputModalities, ", "))
	}
	if m.Architecture.Tokenizer != "" {
		row("tokenizer", m.Architecture.Tokenizer)
	}
	if m.Architecture.InstructType != nil && *m.Architecture.InstructType != "" {
		row("instruct_type", *m.Architecture.InstructType)
	}
	if m.HuggingFaceID != nil && *m.HuggingFaceID != "" {
		row("hugging_face_id", *m.HuggingFaceID)
	}
	if m.KnowledgeCutoff != nil && *m.KnowledgeCutoff != "" {
		row("knowledge_cutoff", *m.KnowledgeCutoff)
	}
	if m.ExpirationDate != nil && *m.ExpirationDate != "" {
		row("expiration_date", *m.ExpirationDate)
	}
	if m.Created > 0 {
		row("created", fmt.Sprintf("%d", m.Created))
	}
	row("moderated", strconv.FormatBool(m.TopProvider.IsModerated))
	if m.Reasoning != nil {
		row("reasoning", fmt.Sprintf("mandatory=%t", m.Reasoning.Mandatory))
	}

	b.WriteString("\npricing (USD per 1M tokens)\n")
	priceRow := func(k string, v float64) {
		if v < 0 {
			return
		}
		fmt.Fprintf(&b, "  %-16s %s\n", k, format.Price(v))
	}
	p := m.Pricing
	priceRow("input", p.InputPerM())
	priceRow("output", p.OutputPerM())
	priceRow("cache_read", p.CacheReadPerM())
	priceRow("cache_write", perM(p.InputCacheWrite))
	priceRow("reasoning", perM(p.InternalReasoning))
	priceRow("web_search", perM(p.WebSearch))
	priceRow("image", perM(p.Image))
	priceRow("audio", perM(p.Audio))

	if len(m.SupportedParams) > 0 {
		params := append([]string(nil), m.SupportedParams...)
		sort.Strings(params)
		fmt.Fprintf(&b, "\n%-18s %s\n", "supported_params", wrapList(params, 62))
	}

	if m.Description != "" {
		fmt.Fprintf(&b, "\n%s\n", wrapText(strings.TrimSpace(m.Description), 78))
	}
	return b.String()
}

func newCompareCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "compare <model-id> <model-id> [model-id...]",
		Short: "Compare two or more models side by side",
		Example: `  orx compare anthropic/claude-sonnet-4.5 openai/gpt-5.2
  orx compare gpt-5-mini gpt-5-nano google/gemini-2.5-flash --json`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := clientFromFlags()
			if err != nil {
				return err
			}
			models, err := fetchModels(cmd.Context(), cl)
			if err != nil {
				return err
			}
			picked := make([]*api.Model, 0, len(args))
			for _, a := range args {
				m, err := matchModel(models, a)
				if err != nil {
					return err
				}
				picked = append(picked, m)
			}
			if flagJSON {
				return printJSON(picked)
			}
			fmt.Print(renderCompare(picked))
			return nil
		},
	}
	return c
}

// renderCompare prints one row per attribute and one column per model.
func renderCompare(models []*api.Model) string {
	// Each row is [label, m1, m2, ...]; the header row is [ATTR, MODEL 1, MODEL 2, ...].
	headers := []string{"ATTR"}
	for i := range models {
		headers = append(headers, fmt.Sprintf("MODEL %d", i+1))
	}

	attr := func(name string, get func(*api.Model) string) []string {
		row := make([]string, 0, len(models)+1)
		row = append(row, name)
		for _, m := range models {
			row = append(row, get(m))
		}
		return row
	}

	rows := [][]string{
		headers,
		attr("id", func(m *api.Model) string { return format.Truncate(m.ID, 34) }),
		attr("context", func(m *api.Model) string { return format.Tokens(m.ContextLength) }),
		attr("max_out", func(m *api.Model) string { return format.Tokens(m.TopProvider.MaxCompletionTokens) }),
		attr("input/M", func(m *api.Model) string { return format.Price(m.Pricing.InputPerM()) }),
		attr("output/M", func(m *api.Model) string { return format.Price(m.Pricing.OutputPerM()) }),
		attr("cache/M", func(m *api.Model) string { return format.Price(m.Pricing.CacheReadPerM()) }),
		attr("modality", func(m *api.Model) string { return format.Truncate(m.Architecture.Modality, 34) }),
		attr("reasoning", func(m *api.Model) string {
			if m.Reasoning == nil {
				return "-"
			}
			return fmt.Sprintf("mandatory=%t", m.Reasoning.Mandatory)
		}),
	}

	widths := make([]int, len(headers))
	for _, r := range rows {
		for i, cell := range r {
			if n := len([]rune(cell)); n > widths[i] {
				widths[i] = n
			}
		}
	}

	var b strings.Builder
	for i := range headers {
		b.WriteString(format.Pad(headers[i], widths[i]))
		if i < len(headers)-1 {
			b.WriteString("  ")
		}
	}
	b.WriteByte('\n')
	for i := range headers {
		b.WriteString(strings.Repeat("-", widths[i]))
		if i < len(headers)-1 {
			b.WriteString("  ")
		}
	}
	for _, r := range rows[1:] {
		b.WriteByte('\n')
		for i := range headers {
			b.WriteString(format.Pad(r[i], widths[i]))
			if i < len(headers)-1 {
				b.WriteString("  ")
			}
		}
	}
	return b.String()
}

func perM(s string) float64 {
	if s == "" {
		return -1
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return -1
	}
	return v * 1_000_000
}

func wrapList(items []string, width int) string {
	var b strings.Builder
	line := ""
	for i, it := range items {
		sep := ""
		if i < len(items)-1 {
			sep = ", "
		}
		if len(line)+len(it)+len(sep) > width && line != "" {
			b.WriteString(line + "\n" + strings.Repeat(" ", 19))
			line = ""
		}
		line += it + sep
	}
	b.WriteString(line)
	return b.String()
}

func wrapText(s string, width int) string {
	var b strings.Builder
	for _, para := range strings.Split(s, "\n") {
		for len(para) > width {
			cut := strings.LastIndex(para[:width], " ")
			if cut <= 0 {
				cut = width
			}
			b.WriteString(strings.TrimSpace(para[:cut]) + "\n")
			para = strings.TrimSpace(para[cut:])
		}
		b.WriteString(para + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}
