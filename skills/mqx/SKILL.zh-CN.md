---
name: mqx
description: >
  用 mqx CLI 查询 LLM 模型的能力参数与参考价格，默认综合 OpenRouter 目录与
  models.dev 两个数据源。当 agent 需要确认模型支持什么（上下文长度、tool
  call、structured output、reasoning、temperature、模态）或价格几何、解析
  近似模型 id、对比多个模型时使用。只读、免 key、非交互，支持 --json 与
  -s/--source 选择目录。
---

# mqx — 在终端查询模型参数与价格

mqx 从两个公开来源回答"这个模型能做什么、多少钱"——OpenRouter 模型目录与
models.dev 参考目录——**默认综合两源**。无需 API key，无需任何网络配置。
结果表带 `SOURCE` 列；`-s/--source` 可限定单一目录（`openrouter` 或
`modelsdev`）。

> English version: [SKILL.md](SKILL.md)

## 安装

```bash
go install github.com/daidaiJ/modelq/cmd/mqx@latest
# 或从 https://github.com/daidaiJ/modelq/releases 下载二进制
```

## 选命令

| 任务 | 命令 |
|---|---|
| 单个 id 的完整参数与价格 | `mqx show <model-id>` |
| 按近似 id 或名称找模型 | `mqx search <terms...>` |
| 限定单一目录 | 追加 `-s openrouter` 或 `-s modelsdev` |
| 对比两个或多个模型 | `mqx compare <id1> <id2>` |
| 浏览 / 过滤 OpenRouter 目录 | `mqx list [--free] [--min-ctx N] [--modality image]` |
| 批量查询 / 分页 | `mqx search k3 5.3-flash`（一个参数 = 一个查询）；`--page N`、`--limit N`（默认 5，0 = 全部） |
| 机器可读输出 | 任意命令追加 `--json` |

## 确认模型能力参数（agent 典型流程）

1. **一步解析并检查** — `mqx show <id> --json`：
   - 命中 OpenRouter：`.source == "openrouter"`，`.model.supported_parameters`
     列出请求特性（tools、structured_outputs、reasoning……），`.models_dev`
     补充 OpenRouter 没有的厂商侧能力开关（`tool_call`、
     `structured_output`、`temperature`、`reasoning_options`）。
   - OpenRouter 上没有：`.source == "models.dev"`，`.models_dev.model`
     携带完整能力集与厂商参考价格。
2. **id 不确定** — `mqx search <片段>` 同时搜索两个目录；`SOURCE` 列标明
   每行来自哪个目录；带厂商前缀的查询（`zhipuai/glm-5.3-flash`）会直接
   解析到厂商自己的条目，而不是转售商。
3. **价格核对** — 搜索表列出每个来源的每 1M 输入/输出/缓存价格，可以直接
   对比厂商参考价与 OpenRouter 实际售价。

## 输出单位与约定

- 价格统一为美元 / 1M tokens；`-` 表示未知，`free` 表示免费。
- `--lang zh`（或 `MQX_LANG=zh`）时每个价格同时标注人民币换算：USD→CNY
  汇率依次尝试多个免 key 公共源（ExchangeRate-API、Frankfurter/ECB、
  currency-api 镜像），磁盘缓存 24 小时；输出下方注明汇率、来源与取数
  时间。英文纯文本输出仅显示美元。
- `--json` 输出会在模型字段旁附带同样的汇率（`fx` 对象，含 `usd_cny`、
  `source`、`fetched_at`）——`list`/`compare` 按元素附带，`show`/`search`
  按 hit 附带；汇率不可用时字段整体省略。
- `status: deprecated` 标记旧条目；优先选择非 deprecated 的匹配。
- `reasoning_options` 类型：`toggle`（开/关）、`effort`（允许档位）、
  `budget_tokens`（min-max 预算范围）。
- 所有命令只读且可管道；models.dev 数据与 USD→CNY 汇率均磁盘缓存 24
  小时，`search`/`show` 的 `--refresh` 强制重新下载。任一来源不可达时，
  另一来源独立作答；`MQX_FX_URL` 可覆盖汇率接口。
