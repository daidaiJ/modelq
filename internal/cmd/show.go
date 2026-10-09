package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/daidaiJ/modelq/internal/api"
	"github.com/daidaiJ/modelq/internal/format"
	"github.com/daidaiJ/modelq/internal/fx"
	"github.com/daidaiJ/modelq/internal/locale"
	"github.com/daidaiJ/modelq/internal/modelsdev"
)

func newShowCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "show <model-id>...",
		Short: locale.T("Show full details for one model (id may be a unique prefix)", "显示模型完整详情（id 支持唯一前缀）"),
		Long: locale.T(`Show full details for one or more models.

The id may be exact, a unique prefix, or a substring; ambiguous ids list the
candidates. By default both catalogs are consulted: an OpenRouter hit prints
the OpenRouter detail plus a "models.dev reference" section (vendor reference
pricing, capability parameters); an id that only exists on models.dev prints
the models.dev detail. -s/--source restricts resolution to one catalog.

Pass "-" to read ids from stdin, one per line.`, `显示一个或多个模型的完整详情。

id 支持精确、唯一前缀或子串匹配；有歧义时列出候选。默认查询两个目录：
命中 OpenRouter 时输出 OpenRouter 详情并附 "models.dev 参考"区块（厂商参考
价格、能力参数）；仅存在于 models.dev 的 id 直接输出 models.dev 详情。
-s/--source 可限定单一目录。

传入 "-" 可从标准输入逐行读取 id。`),
		Example: locale.T(`  mqx show anthropic/claude-sonnet-4.5     exact id
  mqx show sonnet-4.5                      unique prefix match
  mqx show glm-5.3-flash                   models.dev fallback for non-OpenRouter ids
  mqx show gpt-5.2 --json                  machine-readable
  mqx show zhipuai/glm-5.3-flash -s modelsdev   one catalog only
  printf 'a\nb\n' | mqx show -             read ids from stdin`, `  mqx show anthropic/claude-sonnet-4.5     精确 id
  mqx show sonnet-4.5                      唯一前缀匹配
  mqx show glm-5.3-flash                   非 OpenRouter id 回退 models.dev
  mqx show gpt-5.2 --json                  机器可读
  mqx show zhipuai/glm-5.3-flash -s modelsdev   限定单一目录
  printf 'a\nb\n' | mqx show -             从标准输入读取 id`),
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			sel, err := parseSource(flagSource)
			if err != nil {
				return err
			}
			ids, fromStdin, err := showIDs(args)
			if err != nil {
				return err
			}

			var orModels []api.Model
			if sel.openrouter {
				cl, err := clientFromFlags()
				if err != nil {
					return err
				}
				orModels, err = fetchModels(cmd.Context(), cl)
				if err != nil {
					if !sel.modelsdev {
						return err
					}
					unavail := fmt.Sprintf("openrouter unavailable (%v); falling back to models.dev", err)
					fmt.Fprintln(os.Stderr, locale.T(unavail, "OpenRouter 不可用（"+err.Error()+"）；回退到 models.dev"))
					orModels = nil
				}
			}
			var cat *modelsdev.Catalog
			loadCat := func() (*modelsdev.Catalog, error) {
				if cat == nil {
					c, cerr := loadModelsDev(cmd.Context(), flagRefresh)
					if cerr != nil {
						return nil, cerr
					}
					cat = c
				}
				return cat, nil
			}

			fxr := fxRate(cmd.Context(), flagRefresh)
			out := make([]modelHit, 0, len(ids))
			var text strings.Builder
			wrote := false
			failed := 0
			for _, id := range ids {
				hit, part, rerr := resolveShowEntry(orModels, loadCat, sel, id, fxr)
				if rerr != nil {
					if fromStdin {
						fmt.Fprintf(os.Stderr, "%s: %v\n", id, rerr)
						failed++
						continue
					}
					return rerr
				}
				out = append(out, hit)
				if !flagJSON {
					if wrote {
						text.WriteString("\n")
					}
					wrote = true
					text.WriteString(part)
				}
			}
			if fromStdin && failed > 0 && failed == len(ids) {
				return errors.New(locale.T("no id from stdin matched a model", "标准输入中的 id 均未匹配到模型"))
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
	c.Flags().StringVarP(&flagSource, "source", "s", "", locale.T("data source: all, openrouter, modelsdev (default all)", "数据来源：all、openrouter、modelsdev（默认 all）"))
	c.Flags().BoolVar(&flagRefresh, "refresh", false, locale.T("re-download the models.dev catalog and the exchange rate, ignoring caches", "忽略缓存，重新下载 models.dev 目录与汇率"))
	return c
}

// resolveShowEntry resolves one id across the selected sources: OpenRouter
// first (with models.dev enrichment), then a models.dev fallback for ids the
// OpenRouter catalog does not carry. text is the human rendering, empty for
// --json callers. fxr is the resolved USD→CNY rate for zh output; nil skips
// the CNY annotation.
func resolveShowEntry(orModels []api.Model, loadCat func() (*modelsdev.Catalog, error), sel sourceSel, id string, fxr *fx.Rate) (modelHit, string, error) {
	if sel.openrouter && orModels != nil {
		m, ambiguous, found := resolveOpenRouter(orModels, id)
		if found {
			if m == nil {
				return modelHit{}, "", ambiguousError(id, ambiguous)
			}
			hit := modelHit{Source: "openrouter", Model: m, FX: fxr}
			text := renderDetail(m, fxr)
			if sel.modelsdev {
				if cat, cerr := loadCat(); cerr == nil {
					hit.ModelsDev = modelsDevRef(cat, m.ID)
					if hit.ModelsDev != nil {
						text += renderMDRef(cat, *hit.ModelsDev, fxr)
					}
				} else {
					unavail := fmt.Sprintf("models.dev unavailable (%v); showing OpenRouter data only", cerr)
					fmt.Fprintln(os.Stderr, locale.T(unavail, "models.dev 不可用（"+cerr.Error()+"）；仅显示 OpenRouter 数据"))
				}
			}
			return hit, text, nil
		}
		if !sel.modelsdev {
			return modelHit{}, "", fmt.Errorf("%s", locale.T(
				fmt.Sprintf("no model matches %q (try: mqx search %s)", id, id),
				fmt.Sprintf("没有模型匹配 %q（试试：mqx search %s）", id, id)))
		}
	}

	cat, err := loadCat()
	if err != nil {
		return modelHit{}, "", err
	}
	var confident []modelsdev.Match
	for _, m := range cat.Match(id) {
		if m.Score >= 80 {
			confident = append(confident, m)
		}
	}
	if pick := preferredVendor(confident, id); pick != nil {
		return modelHit{Source: "models.dev", ModelsDev: pick, FX: fxr}, renderDevDetail(cat, *pick, fxr), nil
	}
	switch len(confident) {
	case 1:
		return modelHit{Source: "models.dev", ModelsDev: &confident[0], FX: fxr}, renderDevDetail(cat, confident[0], fxr), nil
	case 0:
		return modelHit{}, "", noMatchError(cat, id)
	default:
		return modelHit{}, "", ambiguousDevError(id, confident)
	}
}

// preferredVendor picks the entry whose provider matches the "vendor/" part
// of a provider-qualified query, so "zhipuai/glm-5.3-flash" resolves to the
// vendor's own catalog entry instead of the dozens of resellers declaring
// the same canonical model. nil when no provider matches.
func preferredVendor(matches []modelsdev.Match, id string) *modelsdev.Match {
	vendor, _, ok := strings.Cut(id, "/")
	if !ok {
		return nil
	}
	vn, vc := foldID(vendor), strings.ReplaceAll(foldID(vendor), "-", "")
	for i := range matches {
		pid := matches[i].ProviderID
		if foldID(pid) == vn || strings.ReplaceAll(foldID(pid), "-", "") == vc {
			return &matches[i]
		}
	}
	return nil
}

// noMatchError reports a failed models.dev resolution, suggesting the
// closest substring candidates when any exist.
func noMatchError(cat *modelsdev.Catalog, id string) error {
	matches := cat.Match(id)
	if len(matches) == 0 {
		return fmt.Errorf("%s", locale.T(
			fmt.Sprintf("no model matches %q (try: mqx search %s)", id, id),
			fmt.Sprintf("没有模型匹配 %q（试试：mqx search %s）", id, id)))
	}
	const maxShown = 5
	ids := make([]string, 0, maxShown)
	for _, m := range matches {
		if len(ids) == maxShown {
			break
		}
		ids = append(ids, mdDisplayID(m.ProviderID, m.Model))
	}
	list := strings.Join(ids, "\n  ")
	if len(matches) > maxShown {
		list += locale.T(
			fmt.Sprintf("\n  ... and %d more (mqx search %s)", len(matches)-maxShown, id),
			fmt.Sprintf("\n  ……另有 %d 个（mqx search %s）", len(matches)-maxShown, id))
	}
	return fmt.Errorf("%s", locale.T(
		fmt.Sprintf("no exact match for %q; closest candidates:\n  %s", id, list),
		fmt.Sprintf("没有精确匹配 %q；最接近的候选：\n  %s", id, list)))
}

// ambiguousDevError reports a models.dev id that resolves to several providers.
func ambiguousDevError(id string, matches []modelsdev.Match) error {
	const maxShown = 10
	ids := make([]string, 0, maxShown)
	for _, m := range matches {
		if len(ids) == maxShown {
			break
		}
		ids = append(ids, mdDisplayID(m.ProviderID, m.Model))
	}
	list := strings.Join(ids, "\n  ")
	if len(matches) > maxShown {
		list += locale.T(
			fmt.Sprintf("\n  ... and %d more (mqx search %s)", len(matches)-maxShown, id),
			fmt.Sprintf("\n  ……另有 %d 个（mqx search %s）", len(matches)-maxShown, id))
	}
	return fmt.Errorf("%s", locale.T(
		fmt.Sprintf("%q is ambiguous across providers, candidates:\n  %s", id, list),
		fmt.Sprintf("%q 在多个提供商间有歧义，候选：\n  %s", id, list)))
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
			return nil, false, errors.New(locale.T("no model id given\n  mqx show <model-id>\n  printf '<id>\\n' | mqx show -", "未提供模型 id\n  mqx show <model-id>\n  printf '<id>\\n' | mqx show -"))
		}
		sc := bufio.NewScanner(os.Stdin)
		sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
		for sc.Scan() {
			for _, f := range strings.Fields(sc.Text()) {
				ids = append(ids, f)
			}
		}
		if err := sc.Err(); err != nil {
			return nil, true, fmt.Errorf("%s", locale.T("read stdin: "+err.Error(), "读取标准输入失败: "+err.Error()))
		}
		if len(ids) == 0 {
			return nil, true, errors.New(locale.T("stdin was empty\n  mqx show <model-id>\n  printf '<id>\\n' | mqx show -", "标准输入为空\n  mqx show <model-id>\n  printf '<id>\\n' | mqx show -"))
		}
		return ids, true, nil
	}
	return rest, false, nil
}

// ptrPrice maps a nil price to -1 so format.Price renders "-".
func ptrPrice(p *float64) float64 {
	if p == nil {
		return -1
	}
	return *p
}

// renderDevDetail renders a models.dev entry as aligned label/value lines.
func renderDevDetail(cat *modelsdev.Catalog, m modelsdev.Match, fxr *fx.Rate) string {
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
	row("context", tokensDetail(md.Limit.Context))
	if md.Limit.Input > 0 && md.Limit.Input != md.Limit.Context {
		row("max_input", tokensDetail(md.Limit.Input))
	}
	row("max_output", tokensDetail(md.Limit.Output))
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

	b.WriteString("\n")
	b.WriteString(locale.T("pricing (USD per 1M tokens, vendor reference)\n", "价格（美元 / 1M tokens，厂商参考）\n"))
	if note := fxNote(fxr); note != "" {
		fmt.Fprintf(&b, "  %s\n", note)
	}
	price := func(k string, p *float64) {
		if p == nil {
			return
		}
		fmt.Fprintf(&b, "  %-18s %s\n", k, format.WithCNY(*p, cnyRate(fxr)))
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
			"in " + format.WithCNY(ptrPrice(t.Input), cnyRate(fxr)),
			"out " + format.WithCNY(ptrPrice(t.Output), cnyRate(fxr)),
			"cache " + format.WithCNY(ptrPrice(t.CacheRead), cnyRate(fxr)),
		}
		size := tokens(t.Tier.Size)
		detail := strings.Join(parts, ", ")
		fmt.Fprintf(&b, "  %-18s %s\n", locale.T("tiers", "分段"), locale.T("ctx >= "+size+": "+detail, "上下文 ≥ "+size+": "+detail))
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
	fmt.Fprintf(&b, "%-20s models.dev %s\n", locale.T("source", "来源"), cat.FetchedAt.Format("2006-01-02 15:04 MST"))
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

// renderMDRef renders the models.dev enrichment section: vendor reference
// pricing and parameters the OpenRouter catalog does not carry.
func renderMDRef(cat *modelsdev.Catalog, ref modelsdev.Match, fxr *fx.Rate) string {
	md := ref.Model
	info := cat.Info(ref.ProviderID)

	var b strings.Builder
	b.WriteString(locale.T("\nmodels.dev reference ("+ref.ProviderID+" / "+md.ID, "\nmodels.dev 参考（"+ref.ProviderID+" / "+md.ID))
	if info.Name != "" {
		b.WriteString(locale.T(", "+info.Name, "，"+info.Name))
	}
	b.WriteString(locale.T(")\n", "）\n"))

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
		row("max_output", tokensDetail(md.Limit.Output))
	}
	if md.Limit.Input > 0 {
		row("max_input", tokensDetail(md.Limit.Input))
	}

	price := func(k string, p *float64) {
		if p == nil {
			return
		}
		fmt.Fprintf(&b, "  %-18s %s\n", k, format.WithCNY(*p, cnyRate(fxr)))
	}
	b.WriteString(locale.T("  vendor reference pricing (USD per 1M tokens)\n", "  厂商参考价格（美元 / 1M tokens）\n"))
	if note := fxNote(fxr); note != "" {
		fmt.Fprintf(&b, "  %s\n", note)
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
			"in " + format.WithCNY(ptrPrice(t.Input), cnyRate(fxr)),
			"out " + format.WithCNY(ptrPrice(t.Output), cnyRate(fxr)),
			"cache " + format.WithCNY(ptrPrice(t.CacheRead), cnyRate(fxr)),
		}
		size := tokens(t.Tier.Size)
		detail := strings.Join(parts, ", ")
		fmt.Fprintf(&b, "  %-18s %s\n", locale.T("tiers", "分段"), locale.T("ctx >= "+size+": "+detail, "上下文 ≥ "+size+": "+detail))
	}
	if info.Doc != "" {
		row("docs", info.Doc)
	}
	return b.String()
}

// renderDetail renders one model as aligned label/value lines.
func renderDetail(m *api.Model, fxr *fx.Rate) string {
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
	row("context_length", tokensDetail(m.ContextLength))
	row("max_output", tokensDetail(m.TopProvider.MaxCompletionTokens))
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

	b.WriteString("\n")
	b.WriteString(locale.T("pricing (USD per 1M tokens)\n", "价格（美元 / 1M tokens）\n"))
	if note := fxNote(fxr); note != "" {
		fmt.Fprintf(&b, "  %s\n", note)
	}
	priceRow := func(k string, v float64) {
		if v < 0 {
			return
		}
		fmt.Fprintf(&b, "  %-16s %s\n", k, format.WithCNY(v, cnyRate(fxr)))
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
		Short: locale.T("Compare two or more models side by side", "并排对比两个或多个模型"),
		Example: locale.T(`  mqx compare anthropic/claude-sonnet-4.5 openai/gpt-5.2
  mqx compare gpt-5-mini gpt-5-nano google/gemini-2.5-flash --json`, `  mqx compare anthropic/claude-sonnet-4.5 openai/gpt-5.2
  mqx compare gpt-5-mini gpt-5-nano google/gemini-2.5-flash --json`),
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
			fxr := fxRate(cmd.Context(), flagRefresh)
			if flagJSON {
				vals := make([]api.Model, len(picked))
				for i, m := range picked {
					vals[i] = *m
				}
				return printJSON(withFX(vals, fxr))
			}
			fmt.Print(renderCompare(picked, fxr))
			return nil
		},
	}
	return c
}

// renderCompare prints one row per attribute and one column per model.
func renderCompare(models []*api.Model, fxr *fx.Rate) string {
	// Each row is [label, m1, m2, ...]; the header row is [ATTR, MODEL 1, MODEL 2, ...].
	headers := []string{locale.T("ATTR", "属性")}
	for i := range models {
		n := strconv.Itoa(i + 1)
		headers = append(headers, locale.T("MODEL "+n, "模型 "+n))
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
		attr(locale.T("context", "上下文"), func(m *api.Model) string { return tokens(m.ContextLength) }),
		attr(locale.T("max_out", "最大输出"), func(m *api.Model) string { return tokens(m.TopProvider.MaxCompletionTokens) }),
		attr(locale.T("input/M", "输入/M"), func(m *api.Model) string { return format.WithCNY(m.Pricing.InputPerM(), cnyRate(fxr)) }),
		attr(locale.T("output/M", "输出/M"), func(m *api.Model) string { return format.WithCNY(m.Pricing.OutputPerM(), cnyRate(fxr)) }),
		attr(locale.T("cache/M", "缓存/M"), func(m *api.Model) string { return format.WithCNY(m.Pricing.CacheReadPerM(), cnyRate(fxr)) }),
		attr(locale.T("modality", "模态"), func(m *api.Model) string { return format.Truncate(m.Architecture.Modality, 34) }),
		attr(locale.T("reasoning", "推理"), func(m *api.Model) string {
			if m.Reasoning == nil {
				return "-"
			}
			return fmt.Sprintf("mandatory=%t", m.Reasoning.Mandatory)
		}),
	}

	widths := make([]int, len(headers))
	for _, r := range rows {
		for i, cell := range r {
			if n := format.Width(cell); n > widths[i] {
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
	if note := fxNote(fxr); note != "" {
		b.WriteString("\n" + note)
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
