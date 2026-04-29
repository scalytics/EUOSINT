# Architecture

kafSIEM is an entity-centric operations and fusion analysis surface. It joins
Kafka-observed operational traffic with selected OSINT context, writes durable
evidence to SQLite, and serves analyst workflows through a typed API and web UI.

It is designed to complement existing enterprise intelligence platforms rather
than replace them. The integration contract is explicit: kafSIEM exposes
OpenAPI, ontology endpoints, pack-defined types, provenance, map features, and
graph neighborhoods that other systems can consume.

## Product Boundary

kafSIEM is the product in this repository.

- KafScale is the external Kafka transport spine. kafSIEM reads from it.
- KafClaw-style agents are external producers. They emit messages onto
  KafScale topics.
- AgentOps is the internal observer domain that tracks that traffic.
- Packs are constrained domain interfaces, not plugins.

The v1 product ships two operating packs:

- `drones`: unmanned systems readiness, sortie, EW, software, and signoff
  workflows
- `scada`: plant, device, change, alarm, firmware, vulnerability, and session
  workflows

## Operating Modes

- `OSINT`: external intelligence and source-driven research
- `Operations`: internal telemetry, agent traffic, plant, fleet, system
  operations, and workflow state
- `Fusion`: joined workflows where OSINT and operations data are used together

Runtime values remain `OSINT`, `AGENTOPS`, and `HYBRID` for compatibility.
User-facing copy should use `OSINT`, `Operations`, and `Fusion`.

## Package Layout

- `cmd/kafsiem-collector/`: ingest binary
- `cmd/kafsiem-api/`: standalone analyst API binary
- `cmd/pack-docs/`: generated pack reference docs
- `internal/agentops/`: Kafka observer, replay, policy, SQLite store, and
  KafClaw envelope handling
- `internal/graph/`: entity, edge, provenance, geometry, and traversal storage
- `internal/packs/`: pack loader, validation, and ontology registration
- `internal/kafsiemapi/`: HTTP API serving typed analyst resources
- `packs/`: bundled drones and SCADA pack declarations
- `src/agentops/`: Operations and Fusion UI surfaces
- `src/agentops/lib/api-client/`: generated TypeScript API client
- `api/`: generated OpenAPI contract and spec generator

## Runtime Topology

```text
KafClaw-style agents
        |
        v
KafScale / Kafka topics
        |
        v
cmd/kafsiem-collector  --->  /data/agentops.db (+ WAL/SHM)
        |                              |
        | legacy /api/*                v
        +---------------------> cmd/kafsiem-api
                                      |
                                      v
                              Caddy + web UI
```

The collector is responsible for ingest and writes. The analyst API is
responsible for reads, typed query surfaces, and legacy route compatibility.

## Storage Contract

AgentOps state is stored in SQLite at `/data/agentops.db` in Docker. WAL mode
means the sidecar files are part of the live storage contract:

- `/data/agentops.db`
- `/data/agentops.db-wal`
- `/data/agentops.db-shm`

The collector is the writer for observed AgentOps runtime state. The analyst
API serves typed resources from the same SQLite file and can enqueue replay
requests, but it must not rewrite observed records or mutate the live Kafka
tracking group. Backups must be SQLite-aware or include the DB and sidecars
together.

## API Contract

The analyst API is versioned under `/api/v1`. Important surfaces:

- entity profile, neighborhood, provenance, geometry, and timeline
- graph path lookup
- flow list/detail/messages/tasks/traces
- replay list/create
- map layers and GeoJSON features
- active ontology types and packs
- typed search

`api/openapi.yaml` is generated from `api/specgen/specgen.go`. The generated
TypeScript client lives under `src/agentops/lib/api-client/`.

## Pack Contract

Packs declare domain behavior as YAML and Markdown templates:

```text
packs/<name>/
  pack.yaml
  detectors/*.yaml
  maps/layers.yaml
  queries/*.yaml
  reports/*.md.tmpl
  views/*.yaml
```

Pack validation is startup-time. Operators restart services to change active
packs. There is no hot reload and no pack-local executable code.

Generated pack references:

- [Drones pack](packs/drones.md)
- [SCADA pack](packs/scada.md)

## Complementary Platform Integration

kafSIEM should be described as a complementary operations/fusion layer. Public
docs should avoid positioning the product as a clone or derivative of another
vendor product.

The practical integration posture is:

- expose typed entity and graph APIs
- expose active ontology through `/api/v1/ontology/types` and
  `/api/v1/ontology/packs`
- preserve provenance back to source records
- keep packs inspectable as YAML
- keep generated OpenAPI stable for external client generation
