package cmd

import (
	"github.com/spf13/cobra"

	"github.com/daidaiJ/modelq/internal/locale"
)

var (
	flagBaseURL string
	flagJSON    bool
	flagLang    string
	flagRefresh bool
	flagSource  string
)

// NewRootCmd builds the mqx command tree; version is shown by --version.
func NewRootCmd(version string) *cobra.Command {
	root := &cobra.Command{
		Use:     "mqx",
		Version: version,
		Short:   locale.T("Query model parameters and pricing (OpenRouter + models.dev)", "查询模型参数与价格（OpenRouter + models.dev）"),
		Long: locale.T(`mqx queries model parameters and pricing, enriched with the
models.dev reference catalog.

Data sources:
  - OpenRouter public /models metadata endpoint. It is fully public:
    every command works without an API key.
  - models.dev (https://models.dev), a community-maintained catalog across
    providers, used to resolve approximate model ids and to show vendor
    reference pricing and capability parameters. Cached on disk for 24h;
    --refresh on 'dev' and 'show' forces a re-download, and the
    MODELSDEV_API_URL environment variable overrides the endpoint.

The base URL, if you need to override it, comes from --base-url,
OPENROUTER_BASE_URL, or base_url in the config file. Output language:
--lang or the MQX_LANG environment variable (en, zh).

Every command is read-only and non-interactive. Use --json for
machine-readable output; tables are for humans.`, `mqx 查询模型参数与价格，并用 models.dev 参考目录补充信息。

数据来源：
  - OpenRouter 公开 /models 元数据接口，完全公开：所有命令无需 API key。
  - models.dev（https://models.dev），社区维护的跨厂商模型目录，用于解析
    近似模型 id、展示厂商参考价格与能力参数。磁盘缓存 24 小时；'dev' 与
    'show' 的 --refresh 强制重新下载；环境变量 MODELSDEV_API_URL 可覆盖
    接口地址。

如需覆盖接口地址：--base-url、环境变量 OPENROUTER_BASE_URL 或配置文件的
base_url。输出语言：--lang 或环境变量 MQX_LANG（en、zh）。

所有命令只读且非交互。--json 输出机器可读格式，表格供人类阅读。`),
		Example: locale.T(`  mqx list                          all models with pricing
  mqx search gemini flash           find models in both catalogs
  mqx show anthropic/claude-sonnet-4.5   one model in detail
  mqx compare gpt-5-mini gpt-5-nano side-by-side comparison
  mqx show glm-5.3-flash            models.dev fallback for non-OpenRouter ids`, `  mqx list                          全部模型及价格
  mqx search gemini flash           跨两个目录搜索模型
  mqx show anthropic/claude-sonnet-4.5   单个模型详情
  mqx compare gpt-5-mini gpt-5-nano 并排对比
  mqx show glm-5.3-flash            非 OpenRouter id 回退 models.dev`),
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.PersistentFlags().StringVar(&flagBaseURL, "base-url", "", locale.T("OpenRouter API base URL (default https://openrouter.ai/api/v1)", "OpenRouter API 基础地址（默认 https://openrouter.ai/api/v1）"))
	root.PersistentFlags().BoolVar(&flagJSON, "json", false, locale.T("emit raw JSON instead of a table", "输出原始 JSON 而非表格"))
	root.PersistentFlags().StringVar(&flagLang, "lang", "", locale.T("output language: en or zh (default en; MQX_LANG env also works)", "输出语言：en 或 zh（默认 en，也可用 MQX_LANG 环境变量）"))

	root.AddCommand(
		newListCmd(),
		newSearchCmd(),
		newShowCmd(),
		newCompareCmd(),
		newConfigPathCmd(),
	)
	return root
}
