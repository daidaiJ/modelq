# orx — OpenRouter model parameters and pricing from the terminal

`orx` answers "what can this model do and what does it cost" for any LLM you
can name — even if you only remember half the model id. It reads two public
catalogs, needs **no API key**, and stays read-only.

- **OpenRouter catalog** — context, max output, modalities, tokenizer,
  supported request parameters, and actual OpenRouter pricing.
- **[models.dev](https://models.dev)** — a community-maintained catalog
  across providers, used to resolve approximate model ids and to enrich
  results with vendor reference pricing and capability flags
  (`tool_call`, `structured_output`, `temperature`, reasoning effort
  levels) that OpenRouter does not carry.

## Install

```bash
go install github.com/daidaiJ/openrouter-cli@latest
```

or download a binary from [Releases](https://github.com/daidaiJ/openrouter-cli/releases)
(Windows / Linux / macOS, amd64 + arm64, each with a `.sha256` checksum).

## Usage

```bash
# One model in full detail (OpenRouter data + models.dev reference section)
orx show anthropic/claude-sonnet-4.5

# Only half the id? models.dev fuzzy lookup: parameters + vendor prices
orx dev zhipuai/glm-5.3-flash
orx dev kimi-k3            # several matches -> comparison table

# Side-by-side comparison
orx compare gpt-5-mini gpt-5-nano

# Browse and filter the 400+ model catalog
orx list --sort output --desc --limit 20
orx list --free --min-ctx 100000
orx search gemini flash
```

Every command takes `--json` for machine-readable output, and `show`/`dev`
accept ids from stdin (`echo <id> | orx show -`). agents can confirm a
model's capability parameters in one shot:

```bash
orx show anthropic/claude-sonnet-4.5 --json | jq '.models_dev.model |
  {tool_call, structured_output, temperature, reasoning_options, limit, cost}'
```

Pricing is USD per 1M tokens throughout; `-` means unknown, `free` means 0.

## Agent skills

Two ready-made skills ship in this repo:

- [`skills/orx`](skills/orx/SKILL.md) — teaches coding agents to use the
  `orx` CLI for model lookups, comparisons, and capability checks.
- [`skills/model-params`](skills/model-params/SKILL.md) — a standalone
  recipe (curl + jq only, nothing to install) for confirming model
  parameters and prices against the same two public catalogs.

## How matching works

`orx dev` ranks candidates by confidence: exact id in the openrouter
provider → exact id minus `:free`-style variants → exact id in any provider
→ `canonical_model_id` hit → `vendor/model` with dash-collapsed provider
aliases (`z-ai` ~ `zai`) → substring. Separator normalization folds
`claude-sonnet-4.5` and `claude-sonnet-4-5` together.

models.dev data is cached on disk for 24 hours and served stale when
models.dev is unreachable; `--refresh` on `show`/`dev` forces a
re-download and `MODELSDEV_API_URL` overrides the endpoint.

## Development

```bash
go test ./...
go build .
```

Releases are tag-driven: push an annotated `v*` tag and GitHub Actions
builds binaries for Windows/Linux/macOS (amd64/arm64) with the version
injected, attaches per-file SHA-256 checksums, and publishes the release
with the tag message as notes.

## License

[MIT](LICENSE)
