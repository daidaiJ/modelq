package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/daidaiJ/modelq/internal/api"
	"github.com/daidaiJ/modelq/internal/config"
	"github.com/daidaiJ/modelq/internal/format"
	"github.com/daidaiJ/modelq/internal/fx"
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
		page       int
		freeOnly   bool
		minContext int64
		modality   string
	)
	c := &cobra.Command{
		Use:   "list",
		Short: locale.T("List all models with context, max output and pricing", "列出全部模型的上下文、最大输出与价格"),
		Example: locale.T(`  mqx list                                 all models, sorted by id
  mqx list --sort output --desc --limit 20  priciest output tokens first
  mqx list --page 2                         second page of the catalog
  mqx list --free --min-ctx 100000          free models with 100K+ context
  mqx list --modality image --json          image-in models, machine-readable`, `  mqx list                                 全部模型，按 id 排序
  mqx list --sort output --desc --limit 20  输出价格最高的前 20 个
  mqx list --page 2                         目录第二页
  mqx list --free --min-ctx 100000          100K+ 上下文的免费模型
  mqx list --modality image --json          图像输入模型，机器可读`),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := checkPageLimit(page, limit); err != nil {
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
			total := len(models)
			models = filterModels(models, filterOpts{FreeOnly: freeOnly, MinContext: minContext, Modality: modality})
			sortModels(models, sortKey, desc)
			fxr := fxRate(cmd.Context(), flagRefresh)
			if flagJSON {
				// --json reflects the same page the table would show;
				// --limit 0 prints everything. Each element inlines the
				// model fields plus the resolved USD→CNY rate.
				return printJSON(withFX(pageSlice(models, page, limit), fxr))
			}
			if len(models) == 0 {
				fmt.Fprintln(os.Stderr, locale.T("no models matched the filters", "没有模型符合过滤条件"))
				return nil
			}
			fmt.Println(renderList(models, page, limit, fxr))
			fmt.Println(listFooter(shown(len(models), limit, page), total, page, limit))
			if note := fxNote(fxr); note != "" {
				fmt.Println(note)
			}
			return nil
		},
	}
	c.Flags().StringVar(&sortKey, "sort", "", locale.T("sort by: id, name, input, output, ctx, maxout", "排序键：id、name、input、output、ctx、maxout"))
	c.Flags().BoolVar(&desc, "desc", false, locale.T("sort descending", "降序排列"))
	c.Flags().IntVar(&limit, "limit", 5, locale.T("max rows to show (default 5; 0 = all), table and --json", "最多显示行数（默认 5；0 = 全部），对表格和 --json 生效"))
	c.Flags().IntVar(&page, "page", 1, locale.T("1-based page of results", "结果页码，从 1 开始"))
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
		page    int
	)
	c := &cobra.Command{
		Use:   "search <query...>",
		Short: locale.T("Search models by id or name across OpenRouter and models.dev", "跨 OpenRouter 与 models.dev 按 id 或名称搜索模型"),
		Long: locale.T(`Search models by id or name across both catalogs.

Each argument is one query and commas split further: mqx search k3
5.3-flash batches two queries, while mqx search "5.3 flash" ANDs the terms
into a single keyword query. Stdin lines work too (mqx search -), one query
per line.

By default both sources are queried and results carry a SOURCE column;
-s/--source restricts the search to one catalog (openrouter or modelsdev).
--sort applies to OpenRouter rows. Results are truncated to --limit rows
(default 5, 0 = all) and --page selects the page, applied per query. Use
mqx show <id> for full details.`, `跨两个目录按 id 或名称搜索模型。

每个参数是一个查询，参数内逗号会再拆分：mqx search k3 5.3-flash 为批量两个
查询；mqx search "5.3 flash" 是把两个词做 AND 的单个关键词查询。也支持从
标准输入逐行读取（mqx search -）。

默认同时查询两个来源，结果表带 SOURCE 列；-s/--source 可限定单一来源
（openrouter 或 modelsdev）。--sort 仅对 OpenRouter 行生效。结果按 --limit
截断（默认 5，0 = 全部），--page 翻页，均按每个查询独立生效。详情用
mqx show <id>。`),
		Example: locale.T(`  mqx search "5.3 flash"                   one keyword query (terms ANDed)
  mqx search k3 5.3-flash                  batch: two queries in one run
  mqx search glm -s modelsdev --page 2     paginate one catalog
  printf 'k3\n5.3-flash\n' | mqx search -  batch from stdin
  mqx search sonnet --json`, `  mqx search "5.3 flash"                   单个关键词查询（多词 AND）
  mqx search k3 5.3-flash                  批量：一次跑两个查询
  mqx search glm -s modelsdev --page 2     单一目录分页
  printf 'k3\n5.3-flash\n' | mqx search -  从标准输入批量读取
  mqx search sonnet --json`),
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			sel, err := parseSource(flagSource)
			if err != nil {
				return err
			}
			if err := checkPageLimit(page, limit); err != nil {
				return err
			}
			queries, fromStdin, err := searchQueries(args)
			if err != nil {
				return err
			}
			fxr := fxRate(cmd.Context(), flagRefresh)

			// queryResult is the --json envelope: one entry per query with
			// pagination metadata alongside the page of hits.
			type queryResult struct {
				Query    string     `json:"query"`
				Page     int        `json:"page"`
				PageSize int        `json:"page_size"`
				Total    int        `json:"total"`
				Hits     []modelHit `json:"hits"`
			}
			results := make([]queryResult, 0, len(queries))
			var text strings.Builder
			batch := len(queries) > 1 || fromStdin
			var lastErr error
			failed := 0
			for i, q := range queries {
				hits, err := runSearch(cmd.Context(), sel, q, sortKey, desc, fxr)
				if err != nil {
					lastErr = err
					if batch {
						fmt.Fprintf(os.Stderr, "%s: %v\n", q, err)
						failed++
						continue
					}
					return err
				}
				res := queryResult{Query: q, Page: page, PageSize: limit, Total: len(hits), Hits: pageSlice(hits, page, limit)}
				results = append(results, res)
				if !flagJSON {
					if i > 0 {
						text.WriteString("\n")
					}
					if batch {
						text.WriteString(locale.T("query: "+q+"\n", "查询: "+q+"\n"))
					}
					if res.Total == 0 {
						text.WriteString(locale.T(
							fmt.Sprintf("no model matches %q\n", q),
							fmt.Sprintf("没有模型匹配 %q\n", q)))
					} else {
						text.WriteString(renderSearch(res.Hits, fxr))
						text.WriteString("\n")
						text.WriteString(searchFooter(res.Total, page, limit))
					}
					text.WriteString("\n")
				}
			}
			if failed > 0 && failed == len(queries) {
				return lastErr
			}

			if flagJSON {
				if batch {
					return printJSON(results)
				}
				return printJSON(results[0])
			}
			out := strings.TrimRight(text.String(), "\n")
			if note := fxNote(fxr); note != "" {
				out += "\n" + note
			}
			fmt.Print(out + "\n")
			return nil
		},
	}
	c.Flags().StringVarP(&flagSource, "source", "s", "", locale.T("data source: all, openrouter, modelsdev (default all)", "数据来源：all、openrouter、modelsdev（默认 all）"))
	c.Flags().BoolVar(&flagRefresh, "refresh", false, locale.T("re-download the models.dev catalog and the exchange rate, ignoring caches", "忽略缓存，重新下载 models.dev 目录与汇率"))
	c.Flags().StringVar(&sortKey, "sort", "", locale.T("sort openrouter rows by: id, name, input, output, ctx, maxout", "OpenRouter 行排序键：id、name、input、output、ctx、maxout"))
	c.Flags().BoolVar(&desc, "desc", false, locale.T("sort descending", "降序排列"))
	c.Flags().IntVar(&limit, "limit", 5, locale.T("max rows per page (default 5; 0 = all), table and --json", "每页最多行数（默认 5；0 = 全部），对表格和 --json 生效"))
	c.Flags().IntVar(&page, "page", 1, locale.T("1-based page of results, per query", "结果页码，从 1 开始，按查询独立生效"))
	return c
}

// searchQueries turns arguments into queries, reading stdin when the only
// argument is "-" or when no arguments are given and stdin is piped. Each
// stdin line is one query; commas split further.
func searchQueries(args []string) (queries []string, fromStdin bool, err error) {
	rest := args
	if len(rest) == 1 && rest[0] == "-" {
		rest = nil
	}
	if len(rest) == 0 {
		stat, statErr := os.Stdin.Stat()
		if statErr == nil && stat.Mode()&os.ModeCharDevice != 0 {
			return nil, false, errors.New(locale.T(
				"no query given\n  mqx search <terms...>\n  mqx search \"5.3 flash\"            quoted = one keyword query\n  mqx search k3 5.3-flash             multiple args = batch\n  printf 'k3\\n5.3-flash\\n' | mqx search -",
				"未提供查询\n  mqx search <关键词...>\n  mqx search \"5.3 flash\"            引号包裹 = 单个关键词查询\n  mqx search k3 5.3-flash             多个参数 = 批量查询\n  printf 'k3\\n5.3-flash\\n' | mqx search -"))
		}
		sc := bufio.NewScanner(os.Stdin)
		sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
		for sc.Scan() {
			queries = append(queries, splitQueries([]string{sc.Text()})...)
		}
		if err := sc.Err(); err != nil {
			return nil, true, fmt.Errorf("%s", locale.T("read stdin: "+err.Error(), "读取标准输入失败: "+err.Error()))
		}
		if len(queries) == 0 {
			return nil, true, errors.New(locale.T("stdin was empty\n  mqx search <terms...>\n  echo k3 | mqx search -", "标准输入为空\n  mqx search <关键词...>\n  echo k3 | mqx search -"))
		}
		return queries, true, nil
	}
	return splitQueries(rest), false, nil
}

// searchFooter renders the count line under a search table; the long form
// appears once pagination is in play.
func searchFooter(total, page, limit int) string {
	if limit <= 0 || total <= limit {
		return locale.T(
			fmt.Sprintf("%d match(es)", total),
			fmt.Sprintf("%d 个匹配", total))
	}
	return locale.T(
		fmt.Sprintf("%d match(es), page %d/%d (%d per page)", total, page, pageCount(total, limit), limit),
		fmt.Sprintf("%d 个匹配，第 %d/%d 页（每页 %d）", total, page, pageCount(total, limit), limit))
}

// listFooter renders the count line under the list table.
func listFooter(shown, total, page, limit int) string {
	count := fmt.Sprintf("%d of %d model(s)", shown, total)
	zh := fmt.Sprintf("%d / %d 个模型", shown, total)
	if limit > 0 && total > limit {
		count += fmt.Sprintf(", page %d/%d (%d per page)", page, pageCount(total, limit), limit)
		zh += fmt.Sprintf("，第 %d/%d 页（每页 %d）", page, pageCount(total, limit), limit)
	}
	return locale.T(count, zh)
}

// runSearch queries the selected catalogs; OpenRouter rows honor sortKey.
// A failing source degrades to the other one instead of aborting. fxr rides
// along on every hit for --json output.
func runSearch(ctx context.Context, sel sourceSel, query, sortKey string, desc bool, fxr *fx.Rate) ([]modelHit, error) {
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
				hits = append(hits, modelHit{Source: "openrouter", Model: &mm, FX: fxr})
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
					FX:        fxr,
				})
			}
		}
	}
	return hits, nil
}

// renderSearch prints the combined-source search table.
func renderSearch(hits []modelHit, fxr *fx.Rate) string {
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
				format.WithCNY(h.Model.Pricing.InputPerM(), cnyRate(fxr)),
				format.WithCNY(h.Model.Pricing.OutputPerM(), cnyRate(fxr)),
				format.WithCNY(h.Model.Pricing.CacheReadPerM(), cnyRate(fxr)),
			})
		case h.ModelsDev != nil:
			md := h.ModelsDev.Model
			rows = append(rows, []string{
				"models.dev",
				format.Truncate(mdDisplayID(h.ModelsDev.ProviderID, md), 46),
				format.Truncate(md.Name, 24),
				format.Tokens(md.Limit.Context),
				format.WithCNY(ptrPrice(md.Cost.Input), cnyRate(fxr)),
				format.WithCNY(ptrPrice(md.Cost.Output), cnyRate(fxr)),
				format.WithCNY(ptrPrice(md.Cost.CacheRead), cnyRate(fxr)),
			})
		}
	}
	return format.Table(headers, rows)
}

// renderList prints the compact table used by list (OpenRouter catalog),
// already paginated by the caller.
func renderList(models []api.Model, page, limit int, fxr *fx.Rate) string {
	models = pageSlice(models, page, limit)
	headers := []string{
		locale.T("MODEL", "模型"),
		locale.T("CTX", "上下文"),
		locale.T("MAX OUT", "最大输出"),
		locale.T("INPUT/M", "输入/M"),
		locale.T("OUTPUT/M", "输出/M"),
		locale.T("CACHE/M", "缓存/M"),
	}
	rows := make([][]string, 0, len(models))
	for _, m := range models {
		rows = append(rows, []string{
			format.Truncate(m.ID, 46),
			format.Tokens(m.ContextLength),
			format.Tokens(m.TopProvider.MaxCompletionTokens),
			format.WithCNY(m.Pricing.InputPerM(), cnyRate(fxr)),
			format.WithCNY(m.Pricing.OutputPerM(), cnyRate(fxr)),
			format.WithCNY(m.Pricing.CacheReadPerM(), cnyRate(fxr)),
		})
	}
	return format.Table(headers, rows)
}
