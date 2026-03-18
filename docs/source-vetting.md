<!--
Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
SPDX-License-Identifier: Apache-2.0
-->

# Source Vetting

## Runtime Model

The crawler and source vetter now have separate roles:

- `registry/source_candidates.json`: untrusted crawler intake
- `registry/sources.db`: vetted live sources only
- `registry/source_dead_letter.json`: terminal dead-letter queue, never crawled again

Discovery reads `source_candidates.json`, probes for RSS/Atom or durable HTML listing pages, samples content, and optionally calls an LLM source vetter. Only approved sources are promoted into `sources.db`. Promoted candidates are removed from the candidate queue.

If `SEARCH_DISCOVERY_ENABLED=true`, discovery also uses the configured OpenAI-compatible model as a narrow search accelerator. It asks for a small number of official candidate URLs for a capped set of agencies and feeds those URLs back into the same candidate queue and vetting pipeline.

## LLM Endpoint Contract

The source vetter uses an OpenAI-compatible `chat/completions` endpoint. It supports:

- OpenAI
- Mistral
- xAI
- Scalytics Copilot
- Claude-compatible gateways
- Gemini-compatible gateways
- vLLM
- Ollama

The implementation is endpoint-driven, not vendor-SDK driven. If a provider does not expose a native OpenAI-compatible endpoint, place a compatible gateway in front of it and point `SOURCE_VETTING_BASE_URL` at that gateway.

## Search-Capable Models

Search-capable models can be used as discovery accelerators instead of scraping public search engines directly.

Examples:

- xAI `grok-4-1-fast`
- Gemini fast variants
- Claude Haiku variants
- Scalytics Copilot models with search enabled

Recommended use:

- generate a small set of candidate URLs for a specific agency, country, or sector
- pass those URLs into the candidate queue
- let the crawler, deterministic hygiene, and source vetter decide whether they are usable

Do not use search-capable models as direct truth sources or direct promotion sources.

### Token-Safe Search

Use the configured model in a narrow, token-safe way:

- ask for a small number of candidate URLs, not a long report
- constrain by agency, country, and sector
- request only official or high-confidence source URLs
- avoid broad prompts like "find everything about police feeds worldwide"
- prefer short JSON output with URLs and a one-line reason

Good pattern:

- input: `Find up to 5 official feed/API/newsroom URLs for Bundeskriminalamt related to wanted suspects or public appeals. Return JSON only.`
- output: small candidate list

Bad pattern:

- input: `Research German law enforcement internet presence in detail and summarize everything.`

## Environment Variables

```dotenv
SEARCH_DISCOVERY_ENABLED=true
SEARCH_DISCOVERY_MAX_TARGETS=4
SEARCH_DISCOVERY_MAX_URLS_PER_TARGET=3
HTTP_TIMEOUT_MS=60000
SOURCE_VETTING_ENABLED=true
SOURCE_VETTING_PROVIDER=xai
SOURCE_VETTING_BASE_URL=https://api.x.ai/v1
SOURCE_VETTING_API_KEY=
SOURCE_VETTING_MODEL=grok-4-1-fast
SOURCE_VETTING_TEMPERATURE=0
SOURCE_VETTING_MAX_SAMPLE_ITEMS=6
ALERT_LLM_ENABLED=true
ALERT_LLM_MODEL=grok-4-1-fast
ALERT_LLM_MAX_ITEMS_PER_SOURCE=4
```

Put the real API key only in your local `.env`. Do not commit it.

## Example Endpoints

OpenAI:

```dotenv
SOURCE_VETTING_PROVIDER=openai
SOURCE_VETTING_BASE_URL=https://api.openai.com/v1
```

Mistral:

```dotenv
SOURCE_VETTING_PROVIDER=mistral
SOURCE_VETTING_BASE_URL=https://api.mistral.ai/v1
```

xAI:

```dotenv
SOURCE_VETTING_PROVIDER=xai
SOURCE_VETTING_BASE_URL=https://api.x.ai/v1
SOURCE_VETTING_MODEL=grok-4-1-fast
ALERT_LLM_MODEL=grok-4-1-fast
```

Scalytics Copilot:

```dotenv
SOURCE_VETTING_PROVIDER=scalytics-copilot
SOURCE_VETTING_BASE_URL=https://YOUR_SCALYTICS_COPILOT_URL/v1
SOURCE_VETTING_MODEL=your-copilot-model
ALERT_LLM_MODEL=your-copilot-model
```

vLLM:

```dotenv
SOURCE_VETTING_PROVIDER=vllm
SOURCE_VETTING_BASE_URL=http://vllm-host:8000/v1
SOURCE_VETTING_API_KEY=dummy
```

Ollama:

```dotenv
SOURCE_VETTING_PROVIDER=ollama
SOURCE_VETTING_BASE_URL=http://localhost:11434/v1
SOURCE_VETTING_API_KEY=dummy
```

Claude-compatible gateway:

```dotenv
SOURCE_VETTING_PROVIDER=claude
SOURCE_VETTING_BASE_URL=https://your-gateway.example/v1
```

Gemini-compatible gateway:

```dotenv
SOURCE_VETTING_PROVIDER=gemini
SOURCE_VETTING_BASE_URL=https://your-gateway.example/v1
```

## CLI Usage

Run the crawler and vetter once:

```bash
go run ./cmd/euosint-collector \
  --discover \
  --registry registry/sources.db \
  --candidate-queue registry/source_candidates.json \
  --replacement-queue registry/source_dead_letter.json \
  --search-discovery \
  --search-discovery-max-targets 4 \
  --search-discovery-max-urls 3 \
  --source-vetting \
  --source-vetting-provider xai \
  --source-vetting-base-url https://api.x.ai/v1 \
  --source-vetting-api-key "$SOURCE_VETTING_API_KEY" \
  --source-vetting-model grok-4-1-fast \
  --alert-llm \
  --alert-llm-model grok-4-1-fast
```

## Promotion Policy

The LLM does not bypass deterministic hygiene.

Sources are rejected before the LLM stage if they look like:

- local or municipal police
- generic institutional news
- low-signal public relations pages
- sources with no sample items to assess

Approved sources are promoted into `sources.db` with:

- `promotion_status`
- `source_quality`
- `operational_relevance`
- `level`
- `mission_tags`

The live watcher only loads `promotion_status = active`.

## Alert-Level LLM Gate

You can also enable an item-level LLM gate for ambiguous `html-list` sources.

When enabled, each candidate HTML item is sent to the same OpenAI-compatible endpoint with a short prompt that must return strict JSON:

- `yes`: whether the item is intelligence-relevant or just noise
- `translation`: a short English title
- `category_id`: the normalized category id if `yes = true`

If `yes = false`, the collector drops the item.
If `yes = true`, the collector uses the English title and category override during normalization.

RSS, Atom, and structured API sources stay on the deterministic collector path so the live watcher does not stall behind LLM latency.

Example:

```dotenv
ALERT_LLM_ENABLED=true
ALERT_LLM_MODEL=grok-4-1-fast
ALERT_LLM_MAX_ITEMS_PER_SOURCE=4
```

This uses the same provider/base URL/API key as source vetting.

For xAI and similar reasoning-heavy models, keep `ALERT_LLM_MAX_ITEMS_PER_SOURCE` low and raise `HTTP_TIMEOUT_MS` above the default collector timeout. A practical starting point is:

```dotenv
HTTP_TIMEOUT_MS=60000
ALERT_LLM_MAX_ITEMS_PER_SOURCE=4
```

The same principle applies here: if your configured model supports search, use it to return a strict, short yes/no decision, a short English title, and a category id. Keep prompts short and outputs structured to avoid wasting tokens.

Equivalent xAI request shape:

```bash
curl https://api.x.ai/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $SOURCE_VETTING_API_KEY" \
  -d '{
    "messages": [
      {"role": "system", "content": "You are a test assistant."},
      {"role": "user", "content": "Testing. Just say hi and hello world and nothing else."}
    ],
    "model": "grok-4-1-fast",
    "stream": false,
    "temperature": 0
  }'
```

Equivalent Scalytics Copilot request shape:

```bash
curl https://YOUR_SCALYTICS_COPILOT_URL/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $SOURCE_VETTING_API_KEY" \
  -d '{
    "messages": [
      {"role": "system", "content": "You are a test assistant."},
      {"role": "user", "content": "Testing. Just say hi and hello world and nothing else."}
    ],
    "model": "your-copilot-model",
    "stream": false,
    "temperature": 0
  }'
```
