# AgentOps Operator Guide

AgentOps is the internal Kafka-backed observer domain that powers the
Operations and Fusion surfaces. It tracks KafClaw-style group traffic on
KafScale, writes durable state to SQLite, and exposes that state through the
standalone kafSIEM analyst API.

The user-facing modes remain:

- `OSINT` for external intelligence workflows
- `Operations` for internal telemetry, agent traffic, plant, fleet, and system
  operations
- `Fusion` for workflows that join operations data with selected OSINT context

Runtime environment values remain `OSINT`, `AGENTOPS`, and `HYBRID` for
compatibility.

## Process Model

kafSIEM runs three cooperating services in Docker:

- `collector`: ingests OSINT sources and, when AgentOps is enabled, consumes
  Kafka traffic and writes `/data/agentops.db`
- `kafsiem-api`: serves typed `/api/v1/...` analyst resources from the SQLite
  store and proxies legacy `/api/*` routes to the collector where required
- `kafsiem`: serves the web UI through Caddy and reverse-proxies API traffic

The collector is the writer for observed AgentOps state. The analyst API serves
typed analyst resources from that database and may insert replay request rows,
but it must not mutate the live Kafka tracking group or rewrite observed
records. Analyst HTTP traffic should not be served by the collector process.

## Volume Contract

Use immutable images plus mounted config, data, and pack volumes:

- `/config`
  - `agentops_policy.yaml`
  - optional UI policy files
- `/data`
  - `agentops.db`
  - `agentops.db-wal`
  - `agentops.db-shm`
  - replay session metadata
  - generated OSINT runtime JSON
- `/packs`
  - active pack directories, mounted read-only

AgentOps treats a missing `/config/agentops_policy.yaml` as "use built-in
defaults".

## Backup And Restore

`agentops.db` runs with SQLite WAL sidecars. Do not copy only `agentops.db`
while the collector is running.

Preferred backup options:

- stop the Docker stack and copy `agentops.db`, `agentops.db-wal`, and
  `agentops.db-shm` together
- use a SQLite backup-aware snapshot tool against `/data/agentops.db`
- take a storage-level snapshot that includes the database and both sidecars
  at the same point in time

Restore the database by stopping the stack, replacing the database and sidecar
files as a set, then starting the stack again. If restoring from a backup file
created by SQLite's backup API, restore the single resulting database file and
let SQLite create new sidecars on startup.

## Runtime Configuration

Required environment for Operations mode:

- `AGENTOPS_ENABLED=true`
- `AGENTOPS_BROKERS=<broker list>`
- `AGENTOPS_GROUP_NAME=<kafclaw group name>`
- `AGENTOPS_GROUP_ID=<live tracking group>`
- `UI_MODE=AGENTOPS`
- `PROFILE=agentops-default`

Required environment for Fusion mode:

- all Operations settings above
- OSINT credentials needed by the selected sources
- `UI_MODE=HYBRID`
- `PROFILE=hybrid-ops`

Important optional environment:

- `AGENTOPS_POLICY_PATH=/config/agentops_policy.yaml`
- `AGENTOPS_REPLAY_ENABLED=true`
- `AGENTOPS_REPLAY_PREFIX=kafsiem-agentops-replay`
- `AGENTOPS_REJECT_TOPIC=group.<group>.agentops.rejects`
- `AGENTOPS_OUTPUT_PATH=/data/agentops.db`
- `KAFSIEM_PACKS_DIR=/packs`

The guided installer asks for the common site setting and only the runtime keys
needed by the selected operating profile. Advanced knobs such as replay
prefixes, policy paths, TLS overrides, and poll limits remain in `.env` or
mounted config files.

## Pack Layout

Packs are constrained domain interfaces. They declare ontology, detectors,
views, map layers, queries, and report templates as data. They do not execute
pack-local Go or JavaScript.

Active pack layout:

```text
packs/<name>/
  pack.yaml
  detectors/*.yaml
  maps/layers.yaml
  queries/*.yaml
  reports/*.md.tmpl
  views/*.yaml
```

Use the validation and docs targets after editing packs:

```bash
make pack-lint
make pack-docs
```

Generated pack references:

- [docs/packs/drones.md](packs/drones.md)
- [docs/packs/scada.md](packs/scada.md)

Pack changes require a service restart. Hot reload is intentionally out of
scope for the v1 pack contract.

## Analyst API

The standalone API serves typed resources under `/api/v1`:

- `/api/v1/flows`
- `/api/v1/entities/{type}/{id}`
- `/api/v1/entities/{type}/{id}/neighborhood`
- `/api/v1/entities/{type}/{id}/provenance`
- `/api/v1/entities/{type}/{id}/timeline`
- `/api/v1/map/layers`
- `/api/v1/map/features`
- `/api/v1/ontology/types`
- `/api/v1/ontology/packs`
- `/api/v1/search`

The generated OpenAPI contract is [api/openapi.yaml](../api/openapi.yaml).
Client usage is documented in [docs/api-clients.md](api-clients.md), and API
problem types are registered in [docs/agentops-api-errors.md](agentops-api-errors.md).

## Local Demo And Dev

For a local AgentOps dashboard demo with mocked Kafka-derived traffic:

```bash
npm install
npm run demo:agentops
```

This starts Vite, opens `/?demo=agentops`, serves fixture data from
`public/demo/*.json`, and mocks the replay POST path locally. The demo is a
UI/dev workflow only; it does not require a live Kafka broker or SQLite
AgentOps database.

For live local development against collector and API output:

```bash
npm run fetch:alerts:watch
npm run dev
```

For the full Docker development stack:

```bash
make dev-start
```

The application will be available at `http://localhost:8080` unless
`KAFSIEM_HTTP_PORT` overrides the host port.

## KafClaw Topic Model

AgentOps derives or accepts KafClaw group topics for:

- `announce`
- `control.roster`
- `control.onboarding`
- `requests`
- `responses`
- `tasks.status`
- `traces`
- `observe.audit`
- `memory.shared`
- `memory.context`
- `orchestrator`
- dynamic skill topics

Flow reconstruction is keyed by `correlation_id`. Trace reconstruction is keyed
by `trace_id`. Task chains are keyed by `task_id` and `parent_task_id`.

## Replay Safety

Replay always uses a dedicated consumer group derived from
`AGENTOPS_REPLAY_PREFIX`.

- Replay starts at `earliest`
- Replay never mutates the live tracking group
- Replay can be scoped to a subset of topics
- Replay progress and terminal status are written into AgentOps state

## Reject Mirroring

Bad records do not poison-loop the live tracker.

- rejected records are committed after outcome resolution
- if `AGENTOPS_REJECT_TOPIC` is configured, rejected records are mirrored there
- mirror failures are counted and surfaced in health
- mirror failure does not block forward progress

## KafScale Operator Surface

AgentOps uses Kafka admin APIs already implemented by KafScale:

- `ListGroups`
- `DescribeGroups`

The UI only exposes read-only group visibility backed by those capabilities.
Offset reset or destructive replay mutation is intentionally not exposed.

## Example: KafClaw Over KafScale

1. Run KafScale and expose Kafka brokers.
2. Configure AgentOps with the KafClaw group name.
3. Mount `/config/agentops_policy.yaml`.
4. Start kafSIEM with `UI_MODE=AGENTOPS`.
5. Open the Operations desk and inspect live group plus replay groups.

## Example: Dedicated Replay Group

1. Open the Operations desk.
2. Trigger replay.
3. Verify the replay group ID uses the configured replay prefix.
4. Confirm the live tracking group remains unchanged.

## Example: Fusion OSINT Context

Fusion mode is selective. It shows OSINT context only when an explicit match
exists on:

- category
- geography
- sector
- vendor
- product
- CVE
- time-window proximity
