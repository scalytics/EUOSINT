# EUOSINT

EUOSINT is the open-source edition of our OSINT pipeline, used across multiple installations and packaged for local and server deployment.

It now ships with two distinct operating surfaces:

- `OSINT`: the existing globe-first external intelligence workflow
- `AgentOps`: Kafka-backed flow tracking for KafClaw agent communication
- `HYBRID`: AgentOps plus selective external OSINT context

This repository has been prepared for public use by removing non-public, internal, and protected source integrations while keeping the operational pipeline structure intact.

## Open-Source Scope

- Public-ready OSINT pipeline architecture
- AgentOps flow tracking over Kafka for KafClaw-style agent traffic
- Docker-first deployment for reproducible installs
- Web dashboard + Go collector runtime
- Configurable ingestion and refresh cadence

## Operating Modes

The runtime mode is driven by environment and mounted policy files.

- `UI_MODE=OSINT` keeps the existing OSINT product behavior.
- `UI_MODE=AGENTOPS` switches the desktop UI to the AgentOps flow desk.
- `UI_MODE=HYBRID` keeps AgentOps primary and adds selective external-intel context.

AgentOps is a separate bounded domain in the codebase:

- backend: `internal/agentops/...`
- frontend: `src/agentops/...`

It is not implemented as a generic plugin tree.

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

For a local AgentOps demo with mocked Kafka-derived traffic and the real dashboard:

```bash
npm run demo:agentops
```

This opens the desktop UI directly in `AgentOps` mode via `/?demo=agentops`, serves demo state from `public/demo/*.json`, and mocks the replay endpoint locally.

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
- prompt for essential runtime keys only (URL, API credentials, optional LLM toggles)
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

Minimal required runtime variables are in [.env.example](https://github.com/scalytics/EUOSINT/blob/main/.env.example).
Advanced tuning variables and defaults are documented in [docs/advanced-config.md](https://github.com/scalytics/EUOSINT/blob/main/docs/advanced-config.md).

AgentOps-specific runtime knobs include:

- `AGENTOPS_ENABLED`
- `AGENTOPS_BROKERS`
- `AGENTOPS_GROUP_NAME`
- `AGENTOPS_GROUP_ID`
- `AGENTOPS_POLICY_PATH`
- `AGENTOPS_REPLAY_ENABLED`
- `AGENTOPS_REJECT_TOPIC`
- `AGENTOPS_OUTPUT_PATH`
- `UI_MODE`
- `PROFILE`
- `UI_POLICY_PATH`

When AgentOps is enabled, the collector writes `agentops-state.json` into the runtime data volume and the web UI reads that state directly.

Mount contract:

- `/config`: policy and UI steering files
- `/data`: generated AgentOps state and replay metadata

Content behavior is explicit:

- normal Kafka records are decoded from Kafka values and shown in AgentOps detail views
- LFS-backed records are shown as pointer metadata only (`s3://bucket/key`)
- the default product flow does not fetch blob content for LFS-backed records
- rejected records can be mirrored to `AGENTOPS_REJECT_TOPIC`
- replay always uses a dedicated consumer group and never mutates the live tracking group

Operator reference and examples live in [docs/agentops-operator-guide.md](https://github.com/scalytics/EUOSINT/blob/main/docs/agentops-operator-guide.md).

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
- VM/domain deployment instructions live in [docs/operations.md](https://github.com/scalytics/EUOSINT/blob/main/docs/operations.md).
- Noise gate, search defaults, analyst feedback endpoints, and metrics output are documented in [docs/operations.md](https://github.com/scalytics/EUOSINT/blob/main/docs/operations.md#noise-gate-global-noise--contextual-triage).

## Notes

- Local toolchain is pinned to Node `25.8.1` and npm `11.11.0` via `package.json`, `.nvmrc`, and `.node-version`.
- The Go collector is the operational backend for scheduled feed refreshes, Docker runtime, and local commands.
- This repository intentionally excludes non-public/internal/protected integrations and is maintained as the open-source-ready distribution.
- The root `LICENSE` applies to repository-local materials and modifications.
