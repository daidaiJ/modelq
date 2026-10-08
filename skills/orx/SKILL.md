---
name: orx
description: >
  Look up LLM model capability parameters and reference pricing with the orx
  CLI (OpenRouter catalog + models.dev). Use when the agent must confirm what
  a model supports (context length, tool call, structured output, reasoning,
  temperature, modalities) or what it costs, resolve an approximate model id,
  or compare models. Read-only, keyless, non-interactive, --json available.
---

# orx — model parameters and pricing from the terminal

orx answers "what can this model do and what does it cost" from two public
sources: the OpenRouter model catalog and the models.dev reference catalog.
No API key and no network setup needed.

## Install

```bash
go install github.com/daidaiJ/openrouter-cli@latest
# or grab a binary from https://github.com/daidaiJ/openrouter-cli/releases
```

## Pick the command

| Task | Command |
|---|---|
| Full params + prices for one id | `orx show <model-id>` |
| Id is approximate or partial | `orx dev <fragment>` |
| Compare two or more models | `orx compare <id1> <id2>` |
| Browse / filter the catalog | `orx list [--free] [--min-ctx N] [--modality image]` |
| Find by keyword | `orx search <terms...>` |
| Machine-readable output | append `--json` to any command |

## Confirm a model's capability parameters (typical agent flow)

1. **Known id** — `orx show anthropic/claude-sonnet-4.5 --json`:
   - `.model.supported_parameters` lists request features the model accepts
     (tools, structured_outputs, reasoning, temperature, ...).
   - `.models_dev.model` adds the vendor-side capability flags OpenRouter
     does not carry: `tool_call`, `structured_output`, `temperature`,
     `reasoning` with `reasoning_options` (effort levels / budget range).
   - `models_dev` is absent when models.dev has no confident match for the id.
2. **Approximate id** — `orx dev <fragment>` matches fuzzily (strips `:free` /
   `:batch` variants, folds `.`/`_`/`-` separators, aliases `z-ai`~`zai`) and
   prints capability flags, token limits, and vendor reference pricing.
   One exact hit prints details directly; several hits print a comparison
   table.
3. **Price sanity check** — `orx dev <model>` across providers lists every
   provider carrying the model with per-1M input/output prices, so you can
   pick a cheap lane or compare vendor vs reseller pricing.

## Output units and conventions

- Prices are USD per 1M tokens. `-` means unknown, `free` means zero.
- `status: deprecated` marks legacy entries; prefer a non-deprecated match.
- `reasoning_options` types: `toggle` (on/off), `effort` (allowed levels),
  `budget_tokens` (min-max range).
- All commands are read-only and pipe-safe; models.dev data is cached on
  disk for 24h, `--refresh` on `show`/`dev` forces a re-download.
