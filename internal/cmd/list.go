package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/daidaiJ/modelq/internal/api"
	"github.com/daidaiJ/modelq/internal/config"
	"github.com/daidaiJ/modelq/internal/format"
	"github.com/daidaiJ/modelq/internal/locale"
	"github.com/daidaiJ/modelq/internal/modelsdev"
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
		Short: locale.T("List all models with context, max output and pricing", "列出全部模型的上下文、最大输出与价格"),
		Example: locale.T(`  mqx list                                 all models, sorted by id
  mqx list --sort output --desc --limit 20  priciest output tokens first
  mqx list --free --min-ctx 100000          free models with 100K+ context
  mqx list --modality image --json          image-in models, machine-readable`, `  mqx list                                 全部模型，按 id 排序
  mqx list --sort output --desc --limit 20  输出价格最高的前 20 个
  mqx list --free --min-ctx 100000          100K+ 上下文的免费模型
  mqx list --modality image --json          图像输入模型，机器可读`),
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
				fmt.Fprintln(os.Stderr, locale.T("no models matched the filters", "没有模型符合过滤条件"))
				return nil
			}
			fmt.Println(renderList(models, limit))
			count := fmt.Sprintf("%d of %d model(s)", shown(len(models), limit), total)
			fmt.Println(locale.T(count, fmt.Sprintf("%d / %d 个模型", shown(len(models), limit), total)))
			return nil
		},
	}
	c.Flags().StringVar(&sortKey, "sort", "", locale.T("sort by: id, name, input, output, ctx, maxout", "排序键：id、name、input、output、ctx、maxout"))
	c.Flags().BoolVar(&desc, "desc", false, locale.T("sort descending", "降序排列"))
	c.Flags().IntVar(&limit, "limit", 0, locale.T("show at most N rows (0 = all)", "最多显示 N 行（0 = 全部）"))
	c.Flags().BoolVar(&freeOnly, "free", false, locale.T("only free models", "仅免费模型"))
	c.Flags().Int64Var(&minContext, "min-ctx", 0, locale.T("only models with at least this many context tokens", "仅上下文不少于该 token 数的模型"))
	c.Flags().StringVar(&modality, "modality", "", locale.T("filter by modality substring, e.g. image or audio", "按模态子串过滤，如 image 或 audio"))
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
		Short: locale.T("Search models by id or name across OpenRouter and models.dev", "跨 OpenRouter 与 models.dev 按 id 或名称搜索模型"),
		Long: locale.T(`Search models by id or name across both catalogs.

By default both sources are queried and results carry a SOURCE column;
-s/--source restricts the search to one catalog (openrouter or modelsdev).
--sort applies to OpenRouter rows. Use mqx show <id> for full details.`, `跨两个目录按 id 或名称搜索模型。

默认同时查询两个来源，结果表带 SOURCE 列；-s/--source 可限定单一来源
（openrouter 或 modelsdev）。--sort 仅对 OpenRouter 行生效。详情用
mqx show <id>。`),
		Example: locale.T(`  mqx search gemini flash            terms are ANDed on id and name
  mqx search glm-5.3-flash -s modelsdev   one catalog only
  mqx search glm --sort ctx --desc   largest-context GLM models first
  mqx search sonnet --json`, `  mqx search gemini flash            多词按 AND 匹配 id 与名称
  mqx search glm-5.3-flash -s modelsdev   限定单一来源
  mqx search glm --sort ctx --desc   上下文最大的 GLM 模型优先
  mqx search sonnet --json`),
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sel, err := parseSource(flagSource)
			if err != nil {
				return err
			}
			query := strings.Join(args, " ")
			hits, err := runSearch(cmd.Context(), sel, query, sortKey, desc)
			if err != nil {
				return err
			}
			if len(hits) == 0 {
				return fmt.Errorf("%s", locale.T(
					fmt.Sprintf("no model matches %q", query),
					fmt.Sprintf("没有模型匹配 %q", query)))
			}
			if flagJSON {
				return printJSON(hits)
			}
			fmt.Println(renderSearch(hits, limit))
			count := fmt.Sprintf("%d match(es)", shown(len(hits), limit))
			fmt.Println(locale.T(count, fmt.Sprintf("%d 个匹配", shown(len(hits), limit))))
			return nil
		},
	}
	c.Flags().StringVarP(&flagSource, "source", "s", "", locale.T("data source: all, openrouter, modelsdev (default all)", "数据来源：all、openrouter、modelsdev（默认 all）"))
	c.Flags().BoolVar(&flagRefresh, "refresh", false, locale.T("re-download the models.dev catalog, ignoring the cache", "忽略缓存，重新下载 models.dev 目录"))
	c.Flags().StringVar(&sortKey, "sort", "", locale.T("sort openrouter rows by: id, name, input, output, ctx, maxout", "OpenRouter 行排序键：id、name、input、output、ctx、maxout"))
	c.Flags().BoolVar(&desc, "desc", false, locale.T("sort descending", "降序排列"))
	c.Flags().IntVar(&limit, "limit", 0, locale.T("show at most N rows (0 = all)", "最多显示 N 行（0 = 全部）"))
	return c
}

// runSearch queries the selected catalogs; OpenRouter rows honor sortKey.
// A failing source degrades to the other one instead of aborting.
func runSearch(ctx context.Context, sel sourceSel, query, sortKey string, desc bool) ([]modelHit, error) {
	var hits []modelHit
	if sel.openrouter {
		cl, err := clientFromFlags()
		if err != nil {
			return nil, err
		}
		models, err := fetchModels(ctx, cl)
		if err != nil {
			if !sel.modelsdev {
				return nil, err
			}
			unavail := fmt.Sprintf("openrouter unavailable (%v); searching models.dev only", err)
			fmt.Fprintln(os.Stderr, locale.T(unavail, "OpenRouter 不可用（"+err.Error()+"）；仅搜索 models.dev"))
		} else {
			matches := searchModels(models, query)
			sortModels(matches, sortKey, desc)
			for _, m := range matches {
				mm := m
				hits = append(hits, modelHit{Source: "openrouter", Model: &mm})
			}
		}
	}
	if sel.modelsdev {
		cat, err := loadModelsDev(ctx, flagRefresh)
		if err != nil {
			if len(hits) == 0 {
				return nil, err
			}
			unavail := fmt.Sprintf("models.dev unavailable (%v); showing openrouter results only", err)
			fmt.Fprintln(os.Stderr, locale.T(unavail, "models.dev 不可用（"+err.Error()+"）；仅显示 openrouter 结果"))
		} else {
			for _, h := range searchModelsDev(cat, query) {
				hits = append(hits, modelHit{
					Source:    "models.dev",
					ModelsDev: &modelsdev.Match{ProviderID: h.ProviderID, Model: h.Model, Source: "search"},
				})
			}
		}
	}
	return hits, nil
}

// renderSearch prints the combined-source search table.
func renderSearch(hits []modelHit, limit int) string {
	if limit > 0 && len(hits) > limit {
		hits = hits[:limit]
	}
	headers := []string{
		locale.T("SOURCE", "来源"),
		locale.T("MODEL", "模型"),
		locale.T("NAME", "名称"),
		locale.T("CTX", "上下文"),
		locale.T("IN/M", "输入/M"),
		locale.T("OUT/M", "输出/M"),
		locale.T("CACHE/M", "缓存/M"),
	}
	rows := make([][]string, 0, len(hits))
	for _, h := range hits {
		switch {
		case h.Model != nil:
			rows = append(rows, []string{
				"openrouter",
				format.Truncate(h.Model.ID, 46),
				format.Truncate(h.Model.Name, 24),
				format.Tokens(h.Model.ContextLength),
				format.Price(h.Model.Pricing.InputPerM()),
				format.Price(h.Model.Pricing.OutputPerM()),
				format.Price(h.Model.Pricing.CacheReadPerM()),
			})
		case h.ModelsDev != nil:
			md := h.ModelsDev.Model
			rows = append(rows, []string{
				"models.dev",
				format.Truncate(mdDisplayID(h.ModelsDev.ProviderID, md), 46),
				format.Truncate(md.Name, 24),
				format.Tokens(md.Limit.Context),
				format.Price(ptrPrice(md.Cost.Input)),
				format.Price(ptrPrice(md.Cost.Output)),
				format.Price(ptrPrice(md.Cost.CacheRead)),
			})
		}
	}
	return format.Table(headers, rows)
}

// shown reports how many rows the table will print after the limit is applied.
func shown(n, limit int) int {
	if limit > 0 && limit < n {
		return limit
	}
	return n
}
