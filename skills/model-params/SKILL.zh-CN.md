---
name: model-params
description: >
  仅用 curl + jq 查询两个公开目录（OpenRouter /models 与 models.dev）确认
  LLM 的能力参数与参考价格，无需安装任何工具、无需 API key。当 agent 需要
  核实模型的上下文长度、tool call、structured output、reasoning/effort 档
  位、模态或每 1M token 价格，或把价格按实时汇率折算为人民币时使用。
---

# 用公开目录确认模型参数（curl + jq）

> English version: [SKILL.md](SKILL.md)

两个公开 JSON 数据源覆盖本任务，均无需任何 API key：

- OpenRouter 目录 — `https://openrouter.ai/api/v1/models`：规范的
  OpenRouter id（`vendor/model`）、每 token 价格字符串、`supported_parameters`
  列表。
- models.dev — `https://models.dev/api.json`：约 9000 个跨提供商模型，能力
  开关更全，且带厂商参考价格。

每个会话下载一次 models.dev 即可（几 MB）：

```bash
curl -s https://models.dev/api.json -o /tmp/models-dev.json
```

## 1. 已知 OpenRouter 风格 id

```bash
curl -s https://openrouter.ai/api/v1/models \
  | jq '.data[] | select(.id=="anthropic/claude-sonnet-4.5")'
```

看：`context_length`、`architecture`（模态、分词器）、
`supported_parameters`（tools、structured_outputs、reasoning……）、
`pricing.prompt` / `pricing.completion` — 美元 **每 token**，乘以 1e6
换算成每 1M。

## 2. models.dev 独有的能力开关

```bash
jq '.openrouter.models["anthropic/claude-sonnet-4.5"]' /tmp/models-dev.json
```

看：`tool_call`、`structured_output`、`temperature`、`reasoning` +
`reasoning_options`（effort 档位 / budget_tokens 上下限）、`limit.context`
/ `limit.output`、`cost.input` / `cost.output` / `cost.cache_read` — 已经
是美元每 1M — 另有 `status`、`release_date`、`knowledge`，以及映射到
OpenRouter 风格 id 的 `canonical_model_id`。

如果该 id 不在 openrouter 提供商里，搜索厂商提供商（它们用自己的原生 id）：

```bash
jq 'to_entries[] | .key as $p | .value.models | to_entries[]
    | select(.key | test("glm-5.3-flash"; "i"))
    | {provider: $p, id: .key, cost: .value.cost, limit: .value.limit}' \
  /tmp/models-dev.json
```

## 3. id 只是近似的

- 查找前先去掉 OpenRouter 变体后缀：
  `deepseek/deepseek-chat-v3.1:free` → 取 `:` 前的部分。
- 两个目录分隔符习惯不同：OpenRouter 版本号用 `.`（`claude-sonnet-4.5`），
  厂商条目常用 `-`（`claude-sonnet-4-5`），`_` 也会出现。两种都试。
- 厂商别名：`z-ai` ~ `zai`、`moonshot-ai` ~ `moonshotai`。

## 4. 人民币折算（中文输出）

每个会话取一次 USD→CNY 汇率，任选下方免 key 源即可；四个源都权威且国内
外访问友好，按序尝试直到成功：

```bash
# ExchangeRate-API 开放端点（exchangerate-api.com）
curl -s https://open.er-api.com/v6/latest/USD | jq '.rates.CNY'
# Frankfurter — 欧洲央行参考汇率
curl -s 'https://api.frankfurter.dev/v1/latest?base=USD&symbols=CNY' | jq '.rates.CNY'
# fawazahmed0 currency-api（jsDelivr CDN 镜像）
curl -s https://cdn.jsdelivr.net/npm/@fawazahmed0/currency-api@latest/v1/currencies/usd.json | jq '.usd.cny'
# fawazahmed0 currency-api（Cloudflare Pages 镜像）
curl -s https://latest.currency-api.pages.dev/v1/currencies/usd.json | jq '.usd.cny'
```

美元价格乘以汇率得到人民币，并注明汇率与来源，例如
`input $3/M（¥20.1/M，1 USD = 6.71 CNY，open.er-api.com）`。models.dev 的
`cost.*` 已是美元每 1M，直接乘；OpenRouter 的 `pricing.*` 是每 token，
先乘 1e6 再乘汇率。对结果做合理性检查：明显超出 1–20 CNY/USD 区间的值
视为响应异常，换下一个源。

## 数据解读

- 价格：models.dev 的 `cost.*` 是美元每 1M tokens；OpenRouter 的
  `pricing.*` 字符串是美元每 token。`-`/缺省表示未知，`0` 表示免费。
- 布尔字段缺省表示目录未声明该能力；`status: "deprecated"` 标记旧条目，
  优先选更新的匹配。
- `reasoning_options` 类型：`toggle`（开/关）、`effort`（允许档位，
  如 low/medium/high）、`budget_tokens`（思考预算上下限）。
