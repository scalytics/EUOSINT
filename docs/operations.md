<!--
Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
SPDX-License-Identifier: Apache-2.0
-->

# Operations

## Runtime Model

The production stack has two containers:

- `collector`: the Go collector running in watch mode and writing refreshed JSON feeds into a shared Docker volume
- `euosint`: the React bundle served by Caddy, reading the shared JSON volume and serving the UI plus feed files

The web service no longer uses nginx. Caddy serves the SPA, exposes `/alerts.json`, `/alerts-filtered.json`, `/alerts-state.json`, and `/source-health.json`, and can manage TLS automatically when you give it a real domain.

## Local Compose

Copy the example environment file and start the stack:

```bash
cp .env.example .env
docker compose up --build -d
```

If your host only has the legacy plugin installed, use:

```bash
docker-compose up --build -d
```

Default local behavior:

- HTTP on `http://localhost:8080`
- HTTPS listener mapped to `https://localhost:8443` but not used unless `EUOSINT_SITE_ADDRESS` is changed to a hostname that enables TLS
- The collector initializes empty JSON outputs on a fresh shared feed volume, then replaces them with live collector output on the first successful run

## Domain Setup For A VM

1. Provision a VM with Docker Engine and Docker Compose available.
2. Point a DNS `A` record for your chosen hostname to the VM public IPv4 address.
3. Open inbound TCP `80` and `443` on the VM firewall and any cloud security group.
4. Copy the repository to the VM.
5. Create a `.env` file in the repo root:

```dotenv
EUOSINT_SITE_ADDRESS=alerts.example.com
EUOSINT_HTTP_PORT=80
EUOSINT_HTTPS_PORT=443
```

6. Start the stack:

```bash
docker compose up --build -d
```

With a real domain in `EUOSINT_SITE_ADDRESS`, Caddy will request and renew TLS certificates automatically and store them in the `caddy-data` volume.

## VM Service With systemd

Use the checked-in unit at [docs/euosint.service](/Users/alo/Development/scalytics/EUOSINT/docs/euosint.service) so the stack comes back after host reboots:

Install it on the VM:

```bash
sudo cp docs/euosint.service /etc/systemd/system/euosint.service
sudo systemctl daemon-reload
sudo systemctl enable --now euosint.service
```

If the VM only has `docker-compose`, adjust the unit commands accordingly.

## Operational Notes

- The collector writes feed output into the `feed-data` volume shared with the web container.
- The UI footer freshness line is derived from `source-health.json.generated_at` and shows the age of the current collector snapshot. It is normal below 20 minutes, warning from 20 to 60 minutes, and stale above 60 minutes.
- Discovery intake lives in [registry/source_candidates.json](/Users/alo/Development/scalytics/EUOSINT/registry/source_candidates.json).
- Dead sources are written to the terminal DLQ in `source_dead_letter.json` and are not crawled again.
- LLM-assisted source vetting is documented in [docs/source-vetting.md](/Users/alo/Development/scalytics/EUOSINT/docs/source-vetting.md).
- ACLED conflict data integration is documented in [docs/acled.md](/Users/alo/Development/scalytics/EUOSINT/docs/acled.md).
- TLS state and certificates persist in the `caddy-data` volume.
- Caddy runtime state persists in the `caddy-config` volume.
- Scheduled refreshes, Docker runtime, and local collection commands all run through the Go collector.

## Source Discovery

The collector runs a background discovery loop alongside feed collection. Discovery seeds candidate sources from:

- curated sovereign official-statement endpoints (`registry/sovereign_official_statements.seed.json`)
- FIRST.org and Wikidata (enrichment layers)
- gap analysis and replacement search

It then probes them for RSS/Atom feeds or HTML listing pages.

Discovery requires LLM source vetting to promote candidates into the live registry. Without vetting enabled, candidates are discovered and queued but never activated.

### How it works

1. **Seeding** — Curated sovereign official-statement seeds (head of state/government channels), FIRST.org CSIRT teams, Wikidata police/humanitarian/government orgs, and gap analysis (missing country+category combinations) generate candidate URLs.
2. **Probing** — Each candidate URL is checked for RSS/Atom feeds or stable HTML listing pages.
3. **Vetting** — If `SOURCE_VETTING_ENABLED=true`, discovered feeds are sampled and sent to the configured LLM for approval. The LLM scores source quality, operational relevance, and assigns mission tags.
4. **Promotion** — Approved sources are written into `sources.db` and picked up by the collector on its next sweep.

For sovereign official-statement seeds, stricter promotion thresholds are applied only when category is `legislative`.

| Variable | Default | Description |
|----------|---------|-------------|
| `SOVEREIGN_SEED_PATH` | `registry/sovereign_official_statements.seed.json` | Curated sovereign official-statement candidate seeds |
| `OFFICIAL_STATEMENTS_MIN_QUALITY` | `0.75` | Minimum source quality for legislative official-statement seed promotion |
| `OFFICIAL_STATEMENTS_MIN_OPERATIONAL_RELEVANCE` | `0.7` | Minimum operational relevance for legislative official-statement seed promotion |

### Enabling discovery with vetting

Set these in `.env` (the install/update script will prompt for them):

```dotenv
SOURCE_VETTING_ENABLED=true
SOURCE_VETTING_PROVIDER=xai
SOURCE_VETTING_BASE_URL=https://api.x.ai/v1
SOURCE_VETTING_API_KEY=your-key-here
SOURCE_VETTING_MODEL=grok-4-1-fast
```

### Token cost

Discovery vetting is lightweight — each candidate gets a single short prompt with up to 6 sample items. A full discovery cycle with 300+ candidates typically costs under 100k tokens. The cycle runs once per collection interval (default 15 minutes) but most candidates are already deduplicated or filtered by deterministic hygiene before the LLM is called.

To save tokens, disable vetting (`SOURCE_VETTING_ENABLED=false`). Discovery will still run and queue candidates, but they won't be promoted until vetting is re-enabled.

### Dead-letter queue

Sources that return 404, 410, 403, DNS errors, or TLS failures are moved to the dead-letter queue (`source_dead_letter.json`). Dead sources are skipped on subsequent sweeps and retried once every 7 days. Discovery also skips dead-lettered URLs when evaluating candidates.

## Noise Gate (Global Noise + Contextual Triage)

Noise control now has two layers:

1. `registry/stop_words.json` for hard-noise terms only (sports, giveaways, celebrity spam).
2. `registry/noise_policy.json` for contextual scoring (`block`, `downgrade`, `keep`) with actionable overrides and co-occurrence rules.

Optional A/B testing is supported with `registry/noise_policy_b.json`.

| Variable | Default | Description |
|----------|---------|-------------|
| `STOP_WORDS_PATH` | `registry/stop_words.json` | Path to the hard-noise stop-word file |
| `STOP_WORDS` | *(empty)* | Comma-separated extra hard-noise terms |
| `NOISE_POLICY_PATH` | `registry/noise_policy.json` | Primary scored noise policy |
| `NOISE_POLICY_B_PATH` | `registry/noise_policy_b.json` | Secondary policy for A/B evaluation |
| `NOISE_POLICY_B_PERCENT` | `0` | Percent of items routed to policy B (0–100) |
| `NOISE_METRICS_OUTPUT_PATH` | `public/noise-metrics.json` | Output JSON for noise quality/drift metrics |

Guidance:

- Keep ambiguous security/crime terms out of `stop_words.json`.
- Put contextual terms/rules in `noise_policy.json` so valid intelligence (for example sexual-assault police notices) is retained.
- Restart the collector after policy or stop-word edits.

### Analyst Feedback API

The collector API now supports analyst feedback used for precision tracking:

- `POST /api/noise-feedback`
  - body: `{"alert_id":"...","verdict":"false_positive|false_negative|promote_to_alarm|confirm","analyst":"...","notes":"..."}`
- `GET /api/noise-feedback/stats`
  - returns aggregate counts by verdict and source sample counts.

### Search API Default Behavior

`/api/search` defaults to active/current records unless explicitly overridden.

- default: `status=active` (if `status`, `include_filtered`, and `include_removed` are not provided)
- supported flags:
  - `include_filtered=true|false`
  - `include_removed=true|false`
  - `lane=alarm|intel|info|all`

### Noise Metrics Output

`NOISE_METRICS_OUTPUT_PATH` writes a periodic JSON snapshot including:

- lane distribution + lane drift vs previous snapshot
- average event geo confidence + drift
- source-level lane distribution
- source precision derived from analyst feedback

## Database Schema Updates

`sources.db` schema updates are applied at startup by the collector `Init()` path.

- New tables are created via `CREATE TABLE IF NOT EXISTS` in `internal/sourcedb/schema.sql`.
- New columns are migrated via `ALTER TABLE` in `internal/sourcedb/db.go`.

## Collector Lifecycle And Cadence Variables

- `RECENT_WINDOW_PER_SOURCE` (default `20`): cap per-run fetch window for non-HTML stream sources (Telegram/RSS/Atom/API).
- `HTML_SCRAPE_INTERVAL_HOURS` (default `24`): minimum interval between successful HTML full scrapes when probe status is `200`.
- `ALERT_COOLDOWN_HOURS` (default `24`): missing alerts move from `active` to `cooldown` after this horizon.
- `ALERT_STALE_DAYS` (default `14`): missing alerts move from `cooldown` to `stale` after this horizon.
- `ALERT_ARCHIVE_DAYS` (default `90`): missing alerts move from `stale` to `archived` after this horizon.

Alert lifecycle notes:

- Alerts are no longer dropped on first miss.
- `alerts.json` keeps currently visible lifecycle states from reconciliation.
- Long-tail history remains in `alerts-state.json`, including archived records until archive horizon expiry.
