# mqx — 在终端查询模型参数与价格

[English](README.md) | 简体中文

`mqx` 回答"这个模型能做什么、多少钱"——哪怕你只记得模型 id 的一半。它综合
两个公开目录，**无需 API key**，全程只读：

- **OpenRouter 目录** — 上下文、最大输出、模态、分词器、支持的请求参数，
  以及 OpenRouter 实际售价。
- **[models.dev](https://models.dev)** — 社区维护的跨厂商模型目录，用于
  解析近似模型 id，并展示厂商参考价格与能力开关（`tool_call`、
  `structured_output`、`temperature`、reasoning 档位）——这些是 OpenRouter
  不提供的。

搜索与查询**默认同时命中两个目录**：结果表带 `SOURCE` 列区分来源，
`-s/--source` 可把命令限定到单一目录（`openrouter` 或 `modelsdev`）。

## 安装

```bash
go install github.com/daidaiJ/modelq/cmd/mqx@latest
```

或从 [Releases](https://github.com/daidaiJ/modelq/releases) 下载二进制
（Windows / Linux / macOS，amd64 + arm64，每个文件附 `.sha256` 校验和）。

## 使用

```bash
# 单个模型完整详情（OpenRouter 数据 + models.dev 参考区块）
mqx show anthropic/claude-sonnet-4.5

# OpenRouter 上没有？回退 models.dev，输出厂商参数与价格
mqx show zhipuai/glm-5.3-flash

# 同时搜索两个目录（SOURCE 列区分来源）
mqx search glm-5.3-flash
mqx search kimi-k3 -s modelsdev      # 限定单一目录

# 批量查询与分页（默认每页截断 50 行）
mqx search k3 5.3-flash              # 一次跑两个查询
mqx search "5.3 flash" --page 2      # 引号内空格 = AND 词；--page 翻页

# 并排对比
mqx compare gpt-5-mini gpt-5-nano

# 浏览与过滤 OpenRouter 目录
mqx list --sort output --desc --limit 20
mqx list --free --min-ctx 100000
```

所有命令支持 `--json` 机器可读输出；`show` 支持从标准输入读取 id
（`echo <id> | mqx show -`）。agent 一步确认模型能力参数：

```bash
mqx show anthropic/claude-sonnet-4.5 --json | jq '.models_dev.model |
  {tool_call, structured_output, temperature, reasoning_options, limit, cost}'
```

输出语言跟随 `--lang` 或环境变量 `MQX_LANG`（`en`、`zh`）。价格统一为
美元 / 1M tokens；`-` 表示未知，`free` 表示免费。

## Agent 技能

仓库内置两个可直接使用的技能：

- [`skills/mqx`](skills/mqx/SKILL.md) — 教编码 agent 用 `mqx` CLI 查模型、
  对比与确认能力参数。
- [`skills/model-params`](skills/model-params/SKILL.md) — 独立配方（仅需
  curl + jq，无需安装任何工具），用同样两个公开目录确认模型参数与价格。

两个技能均有中文版（`SKILL.zh-CN.md`，与 `SKILL.md` 同目录）。

## 匹配规则

近似 id 按置信度排序：openrouter 提供商内精确 id → 去掉 `:free` 类变体
后缀 → 任意提供商精确 id → `vendor/model` 且提供商别名折叠（`z-ai` ~
`zai`）→ `canonical_model_id` 命中 → 子串。带厂商前缀的查询会优先选择
厂商自己的目录条目，而不是声明同一 canonical 的转售商。分隔符归一化让
`claude-sonnet-4.5` 与 `claude-sonnet-4-5` 等价。

models.dev 数据在磁盘缓存 24 小时；models.dev 不可达时回退到缓存副本。
`search`/`show` 的 `--refresh` 强制重新下载，`MODELSDEV_API_URL` 可覆盖
接口地址。任一来源不可达时，另一来源独立作答。

## 开发

```bash
go test ./...
go build ./cmd/mqx
```

发布由 tag 驱动：推送 annotated `v*` tag 后，GitHub Actions 自动交叉编译
Windows/Linux/macOS（amd64/arm64）二进制并注入版本号、逐文件附加 SHA-256
校验和，并以 tag 消息作为 release 日志发布。release 日志为中英双语
（中文在前、英文在后），两侧读者都能读懂变更。

## 许可证

[MIT](LICENSE)
