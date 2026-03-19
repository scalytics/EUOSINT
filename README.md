# EUOSINT

EUOSINT is the open-source edition of our OSINT pipeline, used across multiple installations and packaged for local and server deployment.

This repository has been prepared for public use by removing non-public, internal, and protected source integrations while keeping the operational pipeline structure intact.

## Open-Source Scope

- Public-ready OSINT pipeline architecture
- Docker-first deployment for reproducible installs
- Web dashboard + Go collector runtime
- Configurable ingestion and refresh cadence

## Run With Docker

```bash
if command -v docker-compose >/dev/null 2>&1; then
  docker-compose up --build
else
  docker compose up --build
fi
```

The application will be available at `http://localhost:8080`.

You can also use the Make targets for local HTTP development:

```bash
make dev-start
make dev-stop
make dev-restart
make dev-logs
```

## Remote Install (wget bootstrap)

```bash
wget -qO- https://raw.githubusercontent.com/scalytics/EUOSINT/main/deploy/install.sh | bash
```

The installer will:
- verify Docker + Compose availability
- clone or update the repo on the host
- set GHCR runtime images (`ghcr.io/scalytics/euosint-web` + `ghcr.io/scalytics/euosint-collector`)
- prompt for install mode (`preserve` or `fresh` volume reset)
- prompt for domain (`EUOSINT_SITE_ADDRESS`)
- when domain mode is enabled, optionally check `ufw`/`firewalld` and validate local 80/443 availability
- prompt for key runtime flags (browser + LLM vetting settings)
- optionally run `docker compose pull` and start with `--no-build`

- The release pipeline builds two images: a web image and a Go collector image.
- The scheduled feed refresh workflow runs the Go collector.
- The web image uses Caddy, with collector output mounted into the web container at runtime.
- In Docker dev mode, the collector initializes empty JSON outputs on a fresh volume and writes live output on the first successful run.

## Run Locally Without Docker

```bash
volta install node@25.8.1 npm@11.11.0
npm install
npm run fetch:alerts:watch
npm run dev
```

For resilient 24/7 collection with auto-restart on crashes:

```bash
npm run collector:run
```

Tuning examples:

```bash
INTERVAL_MS=120000 MAX_PER_SOURCE=80 npm run collector:run
INTERVAL_MS=120000 RECENT_WINDOW_PER_SOURCE=20 ALERT_STALE_DAYS=14 npm run collector:run
```

Key collector variables:

- `RECENT_WINDOW_PER_SOURCE`: rolling max items per non-HTML source per run (default `20`)
- `HTML_SCRAPE_INTERVAL_HOURS`: minimum hours between successful HTML full scrapes (default `24`)
- `ALERT_COOLDOWN_HOURS`: first lifecycle step for missing alerts before stale (default `24`)
- `ALERT_STALE_DAYS`: days before missing alerts become stale (default `14`)
- `ALERT_ARCHIVE_DAYS`: days before stale alerts move to archived history (default `90`)
- `NOISE_POLICY_PATH`: primary contextual noise-gate policy (default `registry/noise_policy.json`)
- `NOISE_POLICY_B_PATH`: optional A/B policy file (default `registry/noise_policy_b.json`)
- `NOISE_POLICY_B_PERCENT`: percentage routed to policy-B (default `0`)
- `NOISE_METRICS_OUTPUT_PATH`: noise quality/drift output JSON (default `public/noise-metrics.json`)
- `SOVEREIGN_SEED_PATH`: curated sovereign official-statements seed candidates (default `registry/sovereign_official_statements.seed.json`)
- `OFFICIAL_STATEMENTS_MIN_QUALITY`: stricter vetting minimum for legislative official-statement seeds (default `0.75`)
- `OFFICIAL_STATEMENTS_MIN_OPERATIONAL_RELEVANCE`: stricter operational relevance minimum for legislative official-statement seeds (default `0.7`)
- `UCDP_ACCESS_TOKEN`: optional UCDP API token (`x-ucdp-access-token`) for `ucdp-json` conflict ingestion
- `MILITARY_BASES_ENABLED`: enable periodic static military-bases GeoJSON refresh (default `true`)
- `MILITARY_BASES_URL`: source URL for military-bases GeoJSON refresh
- `MILITARY_BASES_OUTPUT_PATH`: output path for static military-bases layer (default `public/geo/military-bases.geojson`)
- `MILITARY_BASES_REFRESH_HOURS`: refresh cadence for military-bases layer (default `168`)

## Operations

```bash
make check
make ci
make docker-build
```

- `make release-patch`, `make release-minor`, and `make release-major` create and push semver tags that trigger the release workflow.
- `.github/workflows/branch-protection.yml` applies protection to `main` using the `ADMIN_GITHUB_TOKEN` repository secret.
- Docker validation runs through `buildx`, and release images publish to GHCR on semver tags.
- Release images are published as `ghcr.io/<owner>/<repo>-web` and `ghcr.io/<owner>/<repo>-collector`.
- `docker-compose up --build` or `docker compose up --build` runs the Go collector as a background refresh service and serves generated JSON through the Caddy web container.
- VM/domain deployment instructions live in [docs/operations.md](/Users/alo/Development/scalytics/EUOSINT/docs/operations.md).
- Noise gate, search defaults, analyst feedback endpoints, and metrics output are documented in [docs/operations.md](/Users/alo/Development/scalytics/EUOSINT/docs/operations.md#noise-gate-global-noise--contextual-triage).

## Notes

- Local toolchain is pinned to Node `25.8.1` and npm `11.11.0` via `package.json`, `.nvmrc`, and `.node-version`.
- The Go collector is the operational backend for scheduled feed refreshes, Docker runtime, and local commands.
- This repository intentionally excludes non-public/internal/protected integrations and is maintained as the open-source-ready distribution.
- The root `LICENSE` applies to repository-local materials and modifications.
