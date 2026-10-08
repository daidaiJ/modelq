---
name: mqx
description: >
  Look up LLM model capability parameters and reference pricing with the mqx
  CLI, which combines the OpenRouter catalog and models.dev by default. Use
  when the agent must confirm what a model supports (context length, tool
  call, structured output, reasoning, temperature, modalities) or what it
  costs, resolve an approximate model id, or compare models. Read-only,
  keyless, non-interactive, --json available, -s/--source picks a catalog.
---

# mqx — model parameters and pricing from the terminal

mqx answers "what can this model do and what does it cost" from two public
sources — the OpenRouter model catalog and the models.dev reference catalog —
**combined by default**. No API key and no network setup needed. Results
carry a `SOURCE` column; `-s/--source` restricts a command to one catalog
(`openrouter` or `modelsdev`).

> 中文版：[SKILL.zh-CN.md](SKILL.zh-CN.md)

## Install

```bash
go install github.com/daidaiJ/modelq/cmd/mqx@latest
# or grab a binary from https://github.com/daidaiJ/modelq/releases
```

## Pick the command

| Task | Command |
|---|---|
| Full params + prices for one id | `mqx show <model-id>` |
| Find models by approximate id or name | `mqx search <terms...>` |
| Restrict to one catalog | append `-s openrouter` or `-s modelsdev` |
| Compare two or more models | `mqx compare <id1> <id2>` |
| Browse / filter the OpenRouter catalog | `mqx list [--free] [--min-ctx N] [--modality image]` |
| Batch queries / paginate | `mqx search k3 5.3-flash` (one arg = one query); `--page N`, `--limit N` (default 50, 0 = all) |
| Machine-readable output | append `--json` to any command |

## Confirm a model's capability parameters (typical agent flow)

1. **Resolve and inspect in one step** — `mqx show <id> --json`:
   - OpenRouter id hit: `.source == "openrouter"`, `.model.supported_parameters`
     lists request features (tools, structured_outputs, reasoning, ...) and
     `.models_dev` adds vendor-side capability flags OpenRouter lacks
     (`tool_call`, `structured_output`, `temperature`, `reasoning_options`).
   - Not on OpenRouter: `.source == "models.dev"` with `.models_dev.model`
     carrying the full capability set and vendor pricing.
2. **Unknown id** — `mqx search <fragment>` searches both catalogs at once;
   the `SOURCE` column tells which catalog each row came from, and
   provider-qualified queries (`zhipuai/glm-5.3-flash`) resolve to the
   vendor's own entry instead of resellers.
3. **Price sanity check** — the search table lists per-1M input/output/cache
   prices per source, so you can compare vendor reference pricing against
   OpenRouter's actual prices.

## Output units and conventions

- Prices are USD per 1M tokens. `-` means unknown, `free` means zero.
- `--lang zh` (or `MQX_LANG=zh`) annotates every price with its CNY
  equivalent at a USD→CNY rate resolved from keyless public sources
  (ExchangeRate-API, Frankfurter/ECB, currency-api mirrors), cached on disk
  for 24h; a footnote under the output names the rate, source, and fetch
  time. English text output stays USD-only.
- `--json` output attaches the same rate as an `fx` object (`usd_cny`,
  `source`, `fetched_at`) next to the model fields — per element on
  `list`/`compare`, per hit on `show`/`search`; the field is omitted when
  the rate is unavailable.
- `status: deprecated` marks legacy entries; prefer a non-deprecated match.
- `reasoning_options` types: `toggle` (on/off), `effort` (allowed levels),
  `budget_tokens` (min-max range).
- All commands are read-only and pipe-safe; models.dev data and the USD→CNY
  rate are cached on disk for 24h, `--refresh` on `search`/`show` forces a
  re-download. When one source is unreachable, the other answers on its own;
  `MQX_FX_URL` overrides the rate endpoint.
