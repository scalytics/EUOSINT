# kafSIEM

kafSIEM is an open-source operations and fusion analysis surface. It turns
Kafka-observed operational traffic and selected OSINT context into an
auditable entity graph for analyst workflows.

The product can run standalone for teams that need a local, Docker-first
analysis stack. It can also complement existing enterprise intelligence
platforms through typed APIs, pack-defined ontology, provenance-preserving
records, and exportable graph context.

It now ships with three operating modes:

- `OSINT`: the existing globe-first external intelligence workflow
- `Operations`: Kafka-backed flow tracking and internal operational workflows
- `Fusion`: operations plus selective external OSINT context

This repository has been prepared for public use by removing non-public, internal, and protected source integrations while keeping the operational pipeline structure intact.

## Open-Source Scope

- Public-ready OSINT pipeline architecture
- Operations flow tracking over Kafka for KafClaw-style agent traffic
- Entity, edge, provenance, map, and timeline APIs backed by SQLite
- Pack-defined ontology for unmanned systems and SCADA / critical infrastructure workflows
- Docker-first deployment for reproducible installs
- Web dashboard, Go collector runtime, and standalone analyst API service
- Configurable ingestion and refresh cadence

## Target Deployments

kafSIEM is designed for two initial operations profiles:

- unmanned systems teams that need readiness, sortie, EW, software, and
  signoff evidence connected across fleet activity
- SCADA and critical infrastructure teams that need plant, device, change,
  alarm, firmware, vulnerability, and session evidence connected across
  operational telemetry

These profiles are shipped as data packs under `packs/`. The active pack
contract is documented in [docs/packs/drones.md](https://github.com/scalytics/kafSIEM/blob/main/docs/packs/drones.md)
and [docs/packs/scada.md](https://github.com/scalytics/kafSIEM/blob/main/docs/packs/scada.md).

## Operating Modes

The runtime mode is driven by environment and mounted policy files.

- `UI_MODE=OSINT` keeps the existing OSINT product behavior.
- `UI_MODE=AGENTOPS` switches the desktop UI to the Operations desk.
- `UI_MODE=HYBRID` switches the desktop UI to Fusion mode with selective external-intel context.

Current runtime values remain `OSINT`, `AGENTOPS`, and `HYBRID` for
compatibility. User-facing product naming is `OSINT`, `Operations`, and
`Fusion`.

AgentOps is a separate bounded domain in the codebase:

- backend: `internal/agentops/...`
- frontend: `src/agentops/...`

It is not implemented as a generic plugin tree.

Architecture details live in [docs/architecture.md](https://github.com/scalytics/kafSIEM/blob/main/docs/architecture.md).

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

The old JSON-backed AgentOps demo UI has been removed. Operations and Fusion
development now uses the live runtime desk against `kafsiem-api` and the
collector-written SQLite store.

## Remote Install (wget bootstrap)

```bash
wget -qO- https://raw.githubusercontent.com/scalytics/kafSIEM/main/deploy/install.sh | bash
```

The installer will:
- verify Docker + Compose availability
- clone or update the repo on the host
- ask for the operating profile (`OSINT`, `Operations`, or `Fusion`)
- set GHCR runtime images (`ghcr.io/scalytics/kafsiem-web` + `ghcr.io/scalytics/kafsiem-collector`)
- prompt for install mode (`preserve` or `fresh` volume reset)
- prompt for the common site setting (`KAFSIEM_SITE_ADDRESS`)
- when domain mode is enabled, optionally check `ufw`/`firewalld` and validate local 80/443 availability
- prompt only for the profile-relevant runtime keys
- optionally run `docker compose pull` and start with `--no-build`

- The release pipeline builds three images: a web image, a Go collector image, and a Go analyst API image.
- The scheduled feed refresh workflow runs the Go collector.
- The web image uses Caddy, with collector output mounted into the web container at runtime and `/api/*` reverse-proxied to the standalone analyst API service.
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

Minimal required runtime variables are in [.env.example](https://github.com/scalytics/kafSIEM/blob/main/.env.example).
Advanced tuning variables and defaults are documented in [docs/advanced-config.md](https://github.com/scalytics/kafSIEM/blob/main/docs/advanced-config.md).

## Installer Profiles

The installer is profile-driven and only asks for the settings that matter for the selected operating mode.

- `OSINT`
  - prompts for `KAFSIEM_SITE_ADDRESS`
  - prompts for OSINT credentials and optional LLM toggles
  - writes `UI_MODE=OSINT` and `PROFILE=osint-default`
- `Operations`
  - prompts for `KAFSIEM_SITE_ADDRESS`
  - prompts for AgentOps Kafka brokers, auth mode, group identifiers, topic mode, replay, and optional reject mirroring
  - writes `UI_MODE=AGENTOPS` and `PROFILE=agentops-default`
- `Fusion`
  - prompts for both the OSINT and AgentOps settings above
  - writes `UI_MODE=HYBRID` and `PROFILE=hybrid-ops`

Advanced settings such as replay prefixes, policy file paths, Kafka poll limits, and TLS overrides stay in `.env` or mounted config files and are not part of the guided install flow.

AgentOps-specific runtime knobs include:

- `AGENTOPS_ENABLED`
- `AGENTOPS_BROKERS`
- `AGENTOPS_GROUP_NAME`
- `AGENTOPS_GROUP_ID`
- `AGENTOPS_POLICY_PATH`
- `AGENTOPS_REPLAY_ENABLED`
- `AGENTOPS_REJECT_TOPIC`
- `AGENTOPS_OUTPUT_PATH`
- `KAFSIEM_PACKS_DIR`
- `UI_MODE`
- `PROFILE`
- `UI_POLICY_PATH`

When AgentOps is enabled, the collector writes `agentops.db` into the runtime data volume and the analyst API serves typed `/api/v1/...` resources from that SQLite store.

Mount contract:

- `/config`: policy and UI steering files
- `/data`: generated AgentOps SQLite state (`agentops.db` plus WAL/SHM sidecars), alerts JSON, and replay metadata
- `/packs`: active bundled or mounted pack directories

Content behavior is explicit:

- normal Kafka records are decoded from Kafka values and shown in AgentOps detail views
- LFS-backed records are shown as pointer metadata only (`s3://bucket/key`)
- the default product flow does not fetch blob content for LFS-backed records
- rejected records can be mirrored to `AGENTOPS_REJECT_TOPIC`
- replay always uses a dedicated consumer group and never mutates the live tracking group

Operator reference and examples live in [docs/agentops-operator-guide.md](https://github.com/scalytics/kafSIEM/blob/main/docs/agentops-operator-guide.md).
The analyst API contract lives in [api/openapi.yaml](https://github.com/scalytics/kafSIEM/blob/main/api/openapi.yaml);
client guidance is in [docs/api-clients.md](https://github.com/scalytics/kafSIEM/blob/main/docs/api-clients.md)
and problem details are registered in [docs/agentops-api-errors.md](https://github.com/scalytics/kafSIEM/blob/main/docs/agentops-api-errors.md).

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
- VM/domain deployment instructions live in [docs/operations.md](https://github.com/scalytics/kafSIEM/blob/main/docs/operations.md).
- Noise gate, search defaults, analyst feedback endpoints, and metrics output are documented in [docs/operations.md](https://github.com/scalytics/kafSIEM/blob/main/docs/operations.md#noise-gate-global-noise--contextual-triage).

## Notes

- Local toolchain is pinned to Node `25.8.1` and npm `11.11.0` via `package.json`, `.nvmrc`, and `.node-version`.
- The Go collector is the operational backend for scheduled feed refreshes, Docker runtime, and local commands.
- This repository intentionally excludes non-public/internal/protected integrations and is maintained as the open-source-ready distribution.
- The root `LICENSE` applies to repository-local materials and modifications.
