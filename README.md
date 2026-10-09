# mqx — model parameters and pricing from the terminal

English | [简体中文](README.zh-CN.md)

`mqx` answers "what can this model do and what does it cost" for any LLM you
can name — even if you only remember half the model id. It combines two
public catalogs, needs **no API key**, and stays read-only:

- **OpenRouter catalog** — context, max output, modalities, tokenizer,
  supported request parameters, and actual OpenRouter pricing.
- **[models.dev](https://models.dev)** — a community-maintained catalog
  across providers, used to resolve approximate model ids and to show vendor
  reference pricing and capability flags (`tool_call`, `structured_output`,
  `temperature`, reasoning effort levels) that OpenRouter does not carry.

Searches and lookups hit **both catalogs by default**; results carry a
`SOURCE` column, and `-s/--source` restricts a command to one catalog
(`openrouter` or `modelsdev`).

## Install

```bash
go install github.com/daidaiJ/modelq/cmd/mqx@latest
```

or download a binary from [Releases](https://github.com/daidaiJ/modelq/releases)
(Windows / Linux / macOS, amd64 + arm64, each with a `.sha256` checksum).

## Usage

```bash
# One model in full detail (OpenRouter data + models.dev reference section)
mqx show anthropic/claude-sonnet-4.5

# Not on OpenRouter? models.dev fallback with vendor parameters and prices
mqx show zhipuai/glm-5.3-flash

# Search both catalogs at once (SOURCE column tells them apart)
mqx search glm-5.3-flash
mqx search kimi-k3 -s modelsdev      # one catalog only

# Batch queries and pagination (truncated to 5 rows per page by default)
mqx search k3 5.3-flash              # two queries in one run
mqx search "5.3 flash" --page 2      # quoted space = AND terms; --page paginates

# Side-by-side comparison
mqx compare gpt-5-mini gpt-5-nano

# Browse and filter the OpenRouter catalog
mqx list --sort output --desc --limit 20
mqx list --free --min-ctx 100000
```

Every command takes `--json` for machine-readable output, `show` accepts ids
from stdin (`echo <id> | mqx show -`), and agents can confirm a model's
capability parameters in one shot:

```bash
mqx show anthropic/claude-sonnet-4.5 --json | jq '.models_dev.model |
  {tool_call, structured_output, temperature, reasoning_options, limit, cost}'
```

Output language follows `--lang` or the `MQX_LANG` environment variable
(`en`, `zh`). Pricing is USD per 1M tokens throughout; `-` means unknown,
`free` means 0. In Chinese output every price also carries its CNY
equivalent: the USD→CNY rate is resolved from keyless public sources —
ExchangeRate-API (open.er-api.com), Frankfurter (ECB reference rates), and
the fawazahmed0 currency-api via jsDelivr and Cloudflare Pages mirrors —
tried in order until one answers, then cached on disk for 24 hours. A stale
cache still answers when every source is unreachable (with a warning),
`--refresh` on `search`/`show` re-downloads it, and `MQX_FX_URL` swaps the
whole source list for one endpoint. `--json` output attaches the same rate
as an `fx` object (`usd_cny`, `source`, `fetched_at`) next to the model
fields, omitted when unavailable. English text output stays USD-only.

## Agent skills

Two ready-made skills ship in this repo:

- [`skills/mqx`](skills/mqx/SKILL.md) — teaches coding agents to use the
  `mqx` CLI for model lookups, comparisons, and capability checks.
- [`skills/model-params`](skills/model-params/SKILL.md) — a standalone
  recipe (curl + jq only, nothing to install) for confirming model
  parameters and prices against the same two public catalogs.

Both are available in Chinese (`SKILL.zh-CN.md` next to each `SKILL.md`).

## How matching works

Approximate ids are ranked by confidence: exact id in the openrouter
provider → exact id minus `:free`-style variants → exact id in any provider
→ `vendor/model` with dash-collapsed provider aliases (`z-ai` ~ `zai`) →
`canonical_model_id` hit → substring. A provider-qualified query prefers the
vendor's own catalog entry over the resellers declaring the same canonical
model. Separator normalization folds `claude-sonnet-4.5` and
`claude-sonnet-4-5` together.

models.dev data is cached on disk for 24 hours and served stale when
models.dev is unreachable; `--refresh` on `search`/`show` forces a
re-download and `MODELSDEV_API_URL` overrides the endpoint. When one source
is unreachable, the other one answers on its own.

## Development

```bash
go test ./...
go build ./cmd/mqx
```

Releases are tag-driven: push an annotated `v*` tag and GitHub Actions
builds binaries for Windows/Linux/macOS (amd64/arm64) with the version
injected, attaches per-file SHA-256 checksums, and publishes the release
with the tag message as notes. Release notes are bilingual (Chinese first,
then English) so the changelog is readable on both fronts.

## License

[MIT](LICENSE)
