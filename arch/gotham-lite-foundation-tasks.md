# Gotham-Lite Foundation — Task List

Local planning document. Do not commit.

Scope of this document: the foundation workstreams needed to move kafSIEM
from its current "OSINT + AgentOps JSON snapshot" posture toward a Gotham-lite
product targeting two named verticals — unmanned systems and SCADA / critical
infrastructure. The strategic rationale for those verticals lives in the
sibling artifacts:

- `arch/gotham-lite-drones-pack.md` — drones / unmanned systems pack
- `arch/gotham-lite-scada-pack.md` — SCADA / critical-infra pack

This document is the engineering counterpart: what we actually build, in what
order, and which red lines stop drift.

No database migrations are required. The stack was born as OSINT, AgentOps
landed on JSON snapshots, and the product has never shipped a relational
AgentOps schema to a customer. We are free to define the schema greenfield.

## Scope

Gotham-lite is kafSIEM. One product, this repo. The surrounding names are not
part of the product:

- `KafScale` is the external Kafka transport spine. kafSIEM reads from it.
- `kafclaw` agents are external. They emit messages onto KafScale. kafSIEM
  observes that traffic through the existing `internal/agentops/` package —
  no rename, no carve-out.
- `KafGraph` is an external adjacent system. It may exchange data with
  kafSIEM later, but it is not part of the product boundary and is not a
  dependency of gotham-lite.

The entity / edge / provenance graph that makes kafSIEM a link-analysis
surface lives inside this repo as a new `internal/graph/` package. Two packs
(drones, SCADA) live under `packs/` as constrained domain interfaces.
Geospatial support is core, not pack-local: every pack can project entities,
areas, tracks, and overlays onto the shared map surface.

The product has three user-facing operating modes:

- `OSINT` — external intelligence and source-driven research
- `Operations` — internal telemetry, agent traffic, plant / fleet / system
  operations, and workflow state
- `Fusion` — joined workflows where OSINT and operations data are used
  together

Mode names are product contract, not copy garnish. Avoid internal
implementation names (`AgentOps`, `kafSIEM`, `graph`) at the mode layer.

Package layout inside kafSIEM:

- `internal/agentops/` — existing observer for kafclaw-shaped traffic on
  KafScale; stays where it is
- `internal/graph/` — new in W2; entity / edge / provenance schema, writers,
  query primitives
- `internal/packs/` — new in W6; pack loader and ontology registration
- `cmd/kafsiem-collector/` — ingest binary (existing)
- `cmd/kafsiem-api/` — analyst-facing API binary (new in W4)

Binary split happens at W4 (`kafsiem-api`) and is the only multi-process
boundary in v1. There is no repo split planned.

## Evidence Base

Citations are to current `main` (commit `8bbc7da`):

- AgentOps state today is in-memory Go maps under a single `sync.Mutex`:
  `internal/agentops/kafka/runtime.go:66-72` declares `state{ flows, traces,
  tasks, msgs, topic }` as `map[string]*...`.
- Persistence is whole-document JSON re-serialization on every update:
  `internal/agentops/store/file.go:59-77` — every `Update(...)` writes the full
  `Document` via `json.MarshalIndent` and `os.WriteFile`. O(n) I/O per record.
- The logical graph exists in miniature: `internal/agentops/store/types.go:79-117`
  — `Flow` carries `trace_ids`, `task_ids`, `senders`, `topics` as string slices;
  `Task` carries `parent_task_id` for delegation chains.
- The frontend fetches the entire `AgentOpsState` and performs joins client-side
  via `messages.filter(m => m.correlation_id === flow.id)` at
  `src/agentops/pages/AgentOpsDesk.tsx:60, 81`. There is no query API beyond
  `agentOpsReplayURL()` POST (line 120).
- `go.mod:39` already declares `modernc.org/sqlite` (pure-Go, no cgo) as an
  indirect dependency through `internal/sourcedb`. No new driver needed; promote
  it to a direct require when the `internal/agentops` store and `internal/graph`
  start using it.
- `franz-go` is mis-listed as indirect at `go.mod:28` despite direct use from
  `internal/agentops/kafka/runtime.go:22`. `go mod tidy` is overdue.

## Non-Goals For This Workstream

- Replacing the OSINT collector's persistence (`internal/sourcedb/db.go` stays
  as-is for OSINT; the new graph tables coexist in the agentops DB).
- Introducing Postgres. Single-node SQLite is the target; multi-node posture is
  deferred to a later workstream once a real customer needs it.
- Redesigning the KafClaw envelope contract (`internal/agentops/contract`).
- Building entity-resolution scoring. That lives in the separate `internal/resolve`
  workstream. This document wires the graph so resolution has somewhere to land.
- A full graph query DSL. v1 ships k-hop traversal via recursive CTE only.
- Splitting kafSIEM into multiple repos. Package boundaries inside this repo
  are enough; the only process split is `kafsiem-collector` vs `kafsiem-api`.
- Building a third pack. Two packs (drones, SCADA) are the v1 contract; a
  third would mean we built a platform before proving the vertical product.

## Red Line Commitments

These are the inviolable decisions that define the workstream. Any PR that
crosses one of these lines should be bounced with a link to this section.
Orientation is explicit: we are modeling Palantir's ingest-vs-serve and
ontology-as-domain-interface patterns, scaled down to single-node SQLite,
two named verticals, and mid-market ops ergonomics.

1. **The collector process never serves analyst HTTP traffic.** Ingest and
   serve are separate binaries. A bad OSINT source or Kafka hiccup must
   never take down the analyst API.
2. **SQLite with WAL is the storage contract between ingest and serve.** The
   collector is the sole writer. The API tier and any future analytical job
   are readers. No in-memory state leaks across the process boundary.
3. **The API is entity-centric, versioned, and schema-contracted.** URLs are
   shaped around entity types, not storage tables. `/api/v1/` ships from the
   first merge. OpenAPI is generated from Go handlers and is the source of
   truth for frontend and external SDKs. `v2` is reserved for breaking
   changes; breaking changes never land in `v1`.
4. **Provenance is a first-class write, not an afterthought.** Every edge,
   every detector decision, and every alert writes a row to `provenance` in
   the same transaction that produced it. A UI affordance must be able to
   walk any visible artifact back to the source record offset.
5. **No plugin system.** Packs are constrained domain interfaces, not
   arbitrary code. They declare ontology, detectors, views, and queries as
   data; they do not ship Go or JavaScript that runs inside the core.
6. **Replay never mutates the live consumer group.** Preserves the red line
   in `arch/kafka-source-architecture.md:300-304`.
7. **The JSON snapshot at `internal/agentops/store/file.go` is deleted, not
   deprecated.** The schema is greenfield; carrying a compatibility shim is
   drift.
8. **No new direct dependencies outside the allowlist.** Permitted additions
   for this workstream: `modernc.org/sqlite` (promote to direct),
   `go-chi/chi/v5` (router), `ogen-go/ogen` or `oapi-codegen` (OpenAPI),
   `goccy/go-yaml` or `sigs.k8s.io/yaml` (pack file parser), and on the
   frontend one of `react-flow` or `cytoscape` after a spike. Anything else
   needs an explicit note in this document.
9. **The default home stays the conversation-centric three-column view.**
   New analyst surfaces live alongside the existing Flow Desk; they do not
   replace it.
10. **Package boundaries inside kafSIEM are enforced.** `internal/agentops/`,
    `internal/graph/`, and `internal/packs/` do not reach across into each
    other except through explicit Go interfaces. The API tier depends on
    interfaces, not concrete stores.
11. **The graph schema is the entity / edge / provenance contract for every
    consumer of kafSIEM's API.** Pack-declared types extend the entity-type
    and edge-type whitelist at startup; the core schema does not churn when
    a pack is added.
12. **Pack registration is startup-time YAML under `packs/<name>/`, not
    runtime, not hot-reload, not a marketplace.** Two packs (drones, SCADA)
    are the v1 cargo. Loader behavior is deterministic and order-stable.
13. **Two packs ship in v1: drones and SCADA.** One pack would mean we
    built a vertical product disguised as a platform. Three packs would
    mean we built a platform before earning it. The v1 contract is exactly
    two, with the package boundary proving the core is pack-agnostic.
14. **The three mode names are fixed early: `OSINT`, `Operations`,
    `Fusion`.** Pack design, UI navigation, docs, and API wording use those
    names consistently. We do not rename modes late in the build.
15. **Geospatial is a first-class primitive across all modes and packs.**
    Core owns geometry storage, map query primitives, and GeoJSON wire
    format. Packs declare spatial types and views; they do not invent their
    own map stack.

---

## Workstream W0 — Package Skeleton

Rationale: W2 and W6 land new packages (`internal/graph/`, `internal/packs/`).
Stamping empty stubs first means later PRs are pure content additions, not
"create-file-then-fill" noise in the same diff. No renames, no behavior
change — `internal/agentops/` stays exactly where it is.

Tasks:

- [x] create `internal/graph/` with `doc.go` only (home for W2 schema, W2
      edge writers, W3 query primitives)
- [x] create `internal/packs/` with `doc.go` only (home for the pack loader
      that lands in W6)
- [x] codify the three product modes in docs and UI copy as `OSINT`,
      `Operations`, `Fusion`; remove older internal wording from user-facing
      surfaces before pack work starts
- [x] keep `internal/agentops/`, `internal/sourcedb/`, and the OSINT
      collector packages exactly as they are — no renames
- [x] one PR, no behavior change, green CI

Acceptance Criteria:

Validated in W12, not during the middle of the refactor.

- `go build ./...` and `go test ./...` pass
- the AgentOps demo (`npm run demo:agentops`) still renders
- the only diff is the two new `doc.go` files

---

## Workstream W1 — AgentOps SQLite Store

Rationale: the JSON snapshot caps observation at demo-scale traffic and makes
every downstream feature (provenance chain, graph queries, pagination)
harder. SQLite with WAL mode and prepared statements gets us to tens of
thousands of records per flow on a single operator box with no operational
burden. This workstream replaces the JSON document under
`internal/agentops/store/` with a SQLite implementation; the graph schema
for entities and edges lives in W2 and coexists in the same DB file.

### W1.1 Schema Definition

Ship a new subpackage `internal/agentops/schema` with embedded SQL.
Greenfield — no compat with the JSON document shape beyond matching the
semantics of the old `store/types.go`.

```sql
CREATE TABLE messages (
  record_id          TEXT PRIMARY KEY,       -- topic:partition:offset
  topic              TEXT NOT NULL,
  topic_family       TEXT NOT NULL,
  partition          INTEGER NOT NULL,
  offset             INTEGER NOT NULL,
  timestamp          TEXT NOT NULL,          -- RFC3339 UTC
  envelope_type      TEXT,
  sender_id          TEXT,
  correlation_id     TEXT,
  trace_id           TEXT,
  task_id            TEXT,
  parent_task_id     TEXT,
  status             TEXT,
  preview            TEXT,
  content            TEXT,
  lfs_bucket         TEXT,
  lfs_key            TEXT,
  lfs_size           INTEGER,
  lfs_sha256         TEXT,
  lfs_content_type   TEXT,
  lfs_created_at     TEXT,
  lfs_proxy_id       TEXT,
  outcome            TEXT NOT NULL,          -- accepted|rejected|mirrored
  reject_reason      TEXT
);
CREATE INDEX idx_messages_corr       ON messages(correlation_id, timestamp);
CREATE INDEX idx_messages_sender     ON messages(sender_id, timestamp);
CREATE INDEX idx_messages_trace      ON messages(trace_id);
CREATE INDEX idx_messages_task       ON messages(task_id);
CREATE INDEX idx_messages_family_ts  ON messages(topic_family, timestamp);

CREATE TABLE flows (
  id              TEXT PRIMARY KEY,           -- correlation_id
  first_seen      TEXT NOT NULL,
  last_seen       TEXT NOT NULL,
  message_count   INTEGER NOT NULL DEFAULT 0,
  latest_status   TEXT,
  latest_preview  TEXT
);
CREATE INDEX idx_flows_last_seen ON flows(last_seen DESC);

CREATE TABLE flow_participants (
  flow_id   TEXT NOT NULL REFERENCES flows(id) ON DELETE CASCADE,
  kind      TEXT NOT NULL,                 -- sender|topic|trace|task
  value     TEXT NOT NULL,
  PRIMARY KEY (flow_id, kind, value)
);

CREATE TABLE traces (
  id            TEXT PRIMARY KEY,
  span_count    INTEGER NOT NULL DEFAULT 0,
  latest_title  TEXT,
  started_at    TEXT,
  ended_at      TEXT,
  duration_ms   INTEGER
);
CREATE TABLE trace_agents (
  trace_id TEXT NOT NULL REFERENCES traces(id) ON DELETE CASCADE,
  agent_id TEXT NOT NULL,
  PRIMARY KEY (trace_id, agent_id)
);
CREATE TABLE trace_span_types (
  trace_id  TEXT NOT NULL REFERENCES traces(id) ON DELETE CASCADE,
  span_type TEXT NOT NULL,
  PRIMARY KEY (trace_id, span_type)
);

CREATE TABLE tasks (
  id                    TEXT PRIMARY KEY,
  parent_task_id        TEXT,
  delegation_depth      INTEGER NOT NULL DEFAULT 0,
  requester_id          TEXT,
  responder_id          TEXT,
  original_requester_id TEXT,
  status                TEXT,
  description           TEXT,
  last_summary          TEXT,
  first_seen            TEXT NOT NULL,
  last_seen             TEXT NOT NULL
);
CREATE INDEX idx_tasks_parent ON tasks(parent_task_id);

CREATE TABLE topic_stats (
  topic             TEXT PRIMARY KEY,
  message_count     INTEGER NOT NULL DEFAULT 0,
  active_agents     INTEGER NOT NULL DEFAULT 0,
  first_message_at  TEXT,
  last_message_at   TEXT
);
CREATE TABLE topic_agents (
  topic    TEXT NOT NULL REFERENCES topic_stats(topic) ON DELETE CASCADE,
  agent_id TEXT NOT NULL,
  PRIMARY KEY (topic, agent_id)
);

CREATE TABLE replay_sessions (
  id             TEXT PRIMARY KEY,
  group_id       TEXT NOT NULL,
  status         TEXT NOT NULL,
  started_at     TEXT NOT NULL,
  finished_at    TEXT,
  message_count  INTEGER NOT NULL DEFAULT 0,
  topics_json    TEXT,
  last_error     TEXT
);

CREATE TABLE health_snapshots (
  taken_at               TEXT PRIMARY KEY,
  connected              INTEGER NOT NULL,
  group_id               TEXT,
  accepted_count         INTEGER NOT NULL DEFAULT 0,
  rejected_count         INTEGER NOT NULL DEFAULT 0,
  mirrored_count         INTEGER NOT NULL DEFAULT 0,
  mirror_failed_count    INTEGER NOT NULL DEFAULT 0,
  last_reject            TEXT,
  last_mirror_error      TEXT,
  last_poll_at           TEXT,
  replay_status          TEXT,
  replay_active          INTEGER NOT NULL DEFAULT 0,
  replay_last_error      TEXT,
  replay_last_finished   TEXT,
  replay_last_count      INTEGER NOT NULL DEFAULT 0,
  rejected_by_reason_json TEXT
);
```

Tasks:

- [x] write `internal/agentops/schema/schema.sql` with the block above
- [x] add `go:embed` loader and `Apply(db)` function for bootstrap
- [x] wire `PRAGMA journal_mode=WAL`, `PRAGMA synchronous=NORMAL`,
      `PRAGMA foreign_keys=ON`, `PRAGMA busy_timeout=5000` on connection open
- [x] table tests verifying schema applies cleanly on a fresh DB

### W1.2 Store Refactor

Replace `internal/agentops/store/file.go` with a SQLite implementation
behind the same package-public surface.

- [x] define `Store` interface in `internal/agentops/store/store.go`:
      `Apply(func(tx Tx) error) error`, `Snapshot(...) (Document, error)`,
      typed query methods (listed in W3)
- [x] implement `SqliteStore` (new file `store/sqlite_store.go`) with prepared
      statements and transactional writes
- [x] delete `store/file.go` and `store/types.go` JSON-only fields;
      rename `Document` → `Snapshot` to make intent clear
- [x] port existing sorting (`store/file.go:86-92`) into SQL `ORDER BY` clauses
- [x] unit tests: each write path (`handleRecord`, `mirrorReject`,
      `startReplay`, `finishReplay`) produces the expected rows
- [x] drop the in-memory maps from `runtime.go:66-72` and replace with typed
      repository calls; `stateMu` becomes the DB tx boundary

### W1.3 Acceptance Criteria

Validated in W12, not during the middle of the refactor.

- the AgentOps demo (`npm run demo:agentops`) still renders the Flow Desk
- a synthetic load test of 10k messages across 500 flows completes without
  the collector process exceeding 150 MB RSS or writing more than the
  message rate × ~4 KB to disk per record
- `handleRecord` latency at steady state stays under 5 ms p99 on the demo box
- the JSON snapshot file is no longer written; `store/file.go` is gone

---

## Workstream W2 — Entity Graph Schema and Writers

Rationale: the current model expresses relationships as string slices on
`Flow` and as `parent_task_id` on `Task`. That works for "show me this one
conversation" but cannot answer "show me every task agent X touched in the
last 24 hours and who else was involved." A typed graph with validity
intervals and explicit provenance is the single change that moves us from
SIEM UX to link-analysis UX. This is also the schema that packs declare
against — core ships agent/task/trace/topic/correlation entity types, packs
extend the whitelist.

### W2.1 Graph Schema

Extends the W1 database (same SQLite file). Added tables, no schema churn on
W1 tables. Lives in `internal/graph/schema/schema.sql`.

```sql
CREATE TABLE entities (
  id            TEXT PRIMARY KEY,        -- "<type>:<canonical_id>"
  type          TEXT NOT NULL,           -- whitelisted by core + active packs
  canonical_id  TEXT NOT NULL,
  display_name  TEXT,
  first_seen    TEXT NOT NULL,
  last_seen     TEXT NOT NULL,
  attrs_json    TEXT NOT NULL DEFAULT '{}'
);
CREATE UNIQUE INDEX idx_entities_type_canonical ON entities(type, canonical_id);
CREATE INDEX idx_entities_last_seen ON entities(last_seen DESC);

CREATE TABLE edges (
  id             INTEGER PRIMARY KEY AUTOINCREMENT,
  src_id         TEXT NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
  dst_id         TEXT NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
  type           TEXT NOT NULL,       -- whitelisted by core + active packs
  valid_from     TEXT NOT NULL,
  valid_to       TEXT,                -- NULL = still valid
  evidence_msg   TEXT REFERENCES messages(record_id) ON DELETE SET NULL,
  weight         REAL NOT NULL DEFAULT 1.0,
  attrs_json     TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX idx_edges_src_type ON edges(src_id, type, valid_from);
CREATE INDEX idx_edges_dst_type ON edges(dst_id, type, valid_from);
CREATE INDEX idx_edges_evidence ON edges(evidence_msg);

CREATE TABLE provenance (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  subject_kind  TEXT NOT NULL,   -- entity|edge|flow|alert
  subject_id    TEXT NOT NULL,
  stage         TEXT NOT NULL,   -- ingest|classify|resolve|graph|detect
  policy_ver    TEXT,
  inputs_json   TEXT,
  decision      TEXT,
  reasons_json  TEXT,
  produced_at   TEXT NOT NULL
);
CREATE INDEX idx_provenance_subject ON provenance(subject_kind, subject_id);

CREATE TABLE entity_geometry (
  entity_id      TEXT PRIMARY KEY REFERENCES entities(id) ON DELETE CASCADE,
  geometry_type  TEXT NOT NULL,        -- point|line|polygon|multipolygon|bbox
  geojson        TEXT NOT NULL,        -- RFC 7946 object
  srid           INTEGER NOT NULL DEFAULT 4326,
  min_lat        REAL,
  min_lon        REAL,
  max_lat        REAL,
  max_lon        REAL,
  z_min          REAL,
  z_max          REAL,
  observed_at    TEXT NOT NULL,
  valid_to       TEXT
);
CREATE INDEX idx_entity_geometry_bbox ON entity_geometry(min_lat, min_lon, max_lat, max_lon);
```

Tasks:

- [x] write `internal/graph/schema/schema.sql`
- [x] helper for opaque `entity_id` construction (`agent:alice`,
      `task:<uuid>`, `trace:<uuid>`, `topic:group.core.requests`,
      `correlation:<uuid>`); pack-declared types add to the prefix set
- [x] indexes audited with `EXPLAIN QUERY PLAN` on realistic loads
- [x] add shared geometry storage for points, tracks, polygons, and bboxes;
      RFC 7946 GeoJSON on the wire, WGS84 (`SRID 4326`) at rest
- [x] first-class core spatial types available to every pack:
      `location`, `area`; packs may add `site`, `zone`, `track`, `contact`
      etc. on top

### W2.2 Edge Writer Hooks (Core Types Only)

The logical relationships already exist in `internal/agentops/kafka/runtime.go`
handlers. We add edge writes, we do not rewrite the handlers. Pack-specific
edge writers come in W6 / W7.

| handler                                  | edges produced                                                                   |
| ---------------------------------------- | -------------------------------------------------------------------------------- |
| `handleRecord` requests branch (390-398) | `agent:<requester> -sent-> task:<task_id>`, `task:<parent> -delegated_to-> task:<child>` |
| `handleRecord` responses branch (399-406)| `agent:<responder> -responded-> task:<task_id>`                                  |
| `handleRecord` tasks.status (407-414)    | status transition stored on task entity attrs, edge weight update                |
| `handleRecord` traces branch (415-421)   | `agent:<sender> -spans-> trace:<trace_id>`                                       |
| every branch                             | `correlation:<id> -mentions-> topic:<t>`, `agent -member_of-> correlation`       |

Tasks:

- [x] `internal/graph/writer.go` with `UpsertEntity(tx, Entity)`,
      `AppendEdge(tx, Edge)`; both idempotent on unique `(type, canonical)`
      keys
- [x] extend each `handleRecord` branch in `internal/agentops/kafka` to
      emit edges after the existing message upsert; `evidence_msg` = the new
      `record_id`
- [x] `valid_to` is set when a contradicting observation arrives (e.g. task
      reassigned). First pass: leave `valid_to` NULL; flip only on explicit
      terminal states (`completed`, `failed`, `cancelled`)
- [x] provenance row per edge insert with `stage='graph'`, `reasons` = the
      handler branch name (Red Line #4)

### W2.3 Acceptance Criteria

Validated in W12, not during the middle of the refactor.

- consuming any KafClaw flow produces a connected component in `entities` +
  `edges` reachable from the correlation entity
- every edge has a matching `provenance` row
- the existing `AgentOpsDesk` timeline continues to render without change
  (graph writes are additive)

---

## Workstream W3 — Graph Query Primitives + Store Query Methods

Rationale: the React app currently grabs the entire document. Once state
lives in SQLite, that pattern dies. The store and the graph each expose
typed query methods; the API tier (W4) is what wires those methods to HTTP.
No HTTP is added in W3.

Tasks on `internal/agentops/store`:

- [x] `ListFlows(ctx, filter FlowFilter, page Pagination) ([]Flow, Cursor)`,
      `GetFlow(ctx, id) (Flow, error)`, `ListMessagesForFlow(ctx, id, page)`,
      `ListTracesForFlow(ctx, id)`, `ListTasksForFlow(ctx, id)`,
      `TopicHealth(ctx) ([]TopicHealth)`, `RecentReplays(ctx, limit)`,
      `LatestHealth(ctx) (Health, error)`
- [x] cursor-based pagination, never offset; opaque cursor is base64-encoded
      `{last_seen, id}`
- [x] every method takes `context.Context`; cancellation propagates to
      `database/sql`
- [x] `Store` (read) and `WriteStore` (collector-only) are separate Go
      interfaces; the API binary compiles against `Store` alone

Tasks on `internal/graph/query.go`:

- [x] `Neighborhood(ctx, entityID, depth int, typeFilter []string,
      window TimeRange) ([]Entity, []Edge)` via recursive CTE; depth
      capped at 3
- [x] `Path(ctx, src, dst, maxDepth int, window TimeRange) ([]Edge, bool)`
- [x] `EntityProfile(ctx, entityID) (Profile)` returning counts by edge
      type, first/last seen, top neighbors by edge weight
- [x] `Geometry(ctx, entityID) (Geometry, error)`, `WithinBBox(ctx, bbox,
      typeFilter, window) ([]Entity, []Geometry)`, `Intersects(ctx, entityID,
      areaID, window) (bool, error)`, `Nearby(ctx, point, radiusMeters,
      typeFilter, window) ([]Entity, []Geometry)`
- [x] bbox-prefiltered spatial search in SQLite using stored min/max bounds;
      exact intersects / contains checks happen in Go on GeoJSON, not via
      SpatiaLite
- [x] benchmarks for each on a 100k-edge synthetic DB; p99 < 100 ms for
      `depth=2`
- [x] table tests per query method against a fixture DB

---

## Workstream W4 — kafSIEM API Tier (`cmd/kafsiem-api`)

Red Lines #1, #3, #11 apply. This workstream commits the binary split and the
entity-centric URL shape. After it merges, the collector no longer serves
analyst traffic and breaking-change protection on `/api/v1/` starts counting.

### W4.1 Binary and Runtime

- [ ] new binary `cmd/kafsiem-api/main.go`; one `func main` that builds the
      router, opens the DB read-only, loads packs, calls `http.ListenAndServe`
- [ ] DB opened with
      `file:/data/agentops.db?mode=ro&_busy_timeout=5000&_journal_mode=WAL`
      (the file is the same SQLite DB that the collector writes; agentops
      tables from W1 and graph tables from W2 coexist in it)
- [ ] `go-chi/chi/v5` router; middleware: `RequestID`, `RealIP`,
      `Recoverer`, structured access log
- [ ] bind `:8081` by default; `--listen` flag and `KAFSIEM_API_LISTEN` env
- [ ] `/healthz` returns 200 if the DB opens; `/readyz` returns 200 only if
      the most recent `health_snapshots` row is under 60s old
- [ ] graceful shutdown on SIGTERM

### W4.2 URL Scheme (Entity-Centric, Pack-Aware)

- [ ] `GET /api/v1/entities/{type}/{id}` — entity profile (W3
      `EntityProfile`)
- [ ] `GET /api/v1/entities/{type}/{id}/neighborhood?depth=2&types=...&window=24h`
      — k-hop graph
- [ ] `GET /api/v1/entities/{type}/{id}/provenance` — provenance chain for
      the subject
- [ ] `GET /api/v1/entities/{type}/{id}/geometry` — GeoJSON geometry +
      bbox metadata for the entity, if present
- [ ] `GET /api/v1/entities/{type}/{id}/timeline?after=&limit=` — paginated
      events in which this entity participates
- [ ] `GET /api/v1/graph/path?src=<entity_id>&dst=<entity_id>&max=3`
- [ ] `GET /api/v1/map/features?bbox=<minLon,minLat,maxLon,maxLat>&types=...&window=...`
      — pack-aware feature query for the shared map surface
- [ ] `GET /api/v1/map/layers` — active basemap / overlay definitions from
      core + packs
- [ ] `GET /api/v1/flows`, `/api/v1/flows/{id}`,
      `/api/v1/flows/{id}/messages`, `/api/v1/flows/{id}/timeline`
- [ ] `GET /api/v1/topic-health`, `/api/v1/health`, `/api/v1/replays`
- [ ] `POST /api/v1/replays` — writes a `replay_request` row; the collector
      picks it up on its next poll (collector remains sole writer, Red Line
      #2)
- [ ] `GET /api/v1/search?q=...` (frontend consumes in W10 Command Bar)
- [ ] `GET /api/v1/ontology/types` — returns the active entity-type and
      edge-type whitelist, with provenance per type (`source: core` or
      `source: pack/<name>`); this is the discovery endpoint that lets a
      beside-Palantir adapter or an auditor enumerate the deployment
- [ ] `GET /api/v1/ontology/packs` — returns the loaded packs, their
      versions, and their declared content (entity types, edge types,
      detector ids, view ids, query template ids)
- [ ] `{type}` whitelist enforced at the router: core types
      (`agent|task|trace|topic|correlation`) plus all pack-declared types
      from active packs; unknown types return 404 not 500

### W4.3 Contract and Codegen

- [ ] OpenAPI 3.1 is the source of truth for schemas. Tool choice between
      `oapi-codegen` and `ogen-go/ogen` happens in the W4 kickoff PR; once
      picked, the other is off the table
- [ ] spec lives at `api/openapi.yaml`, generated from Go via `go generate
      ./api/...`; CI fails if the regenerated spec differs from the
      checked-in copy
- [ ] same spec drives a generated TypeScript client at
      `src/agentops/lib/api-client/`; frontend imports typed
      request/response shapes; no hand-rolled DTOs remain in
      `src/agentops/types/index.ts`
- [ ] versioning rule: `/api/v1` never ships a breaking change. Renames,
      removals, or changed semantics bump to `/api/v2`; `v1` is deprecated
      for one release, then removed
- [ ] errors follow RFC 9457 Problem Details; every 4xx/5xx carries a
      `type` URI pointing at `docs/agentops-api-errors.md`
- [ ] pack-declared response shapes (entity attrs, view templates) are
      documented in OpenAPI as `oneOf` discriminated by entity `type`

### W4.4 Request / Response Discipline

- [ ] all list endpoints return `{items: [...], next: <cursor>|null}`
- [ ] `application/json` only
- [ ] timestamps are RFC3339 UTC
- [ ] entity IDs are opaque strings on the wire (`agent:alice`,
      `platform:auv-07`, `device:plc-12.bravo`); clients echo, never compose
- [ ] spatial wire format is GeoJSON only; clients may render with
      OpenStreetMap, OpenFreeMap, or self-hosted vector/raster tiles, but
      the API does not emit provider-specific map objects

---

## Workstream W5 — Frontend Adaptation

The current `AgentOpsDesk` receives the whole `AgentOpsState` as a prop
(`src/agentops/pages/AgentOpsDesk.tsx:16`). Move it to per-panel queries
that hit the W4 endpoints through the generated typed client.

- [ ] replace the single `useAlerts`-style hook with: `useFlows(filter)`,
      `useFlow(id)`, `useFlowMessages(id, page)`, `useTopicHealth()`,
      `useHealth()`, `useReplaySessions()`
- [ ] each hook uses the generated `openapi-fetch` client +
      `AbortController`; SWR-style revalidate on focus, 5s polling
- [ ] delete the client-side `filter(m => m.correlation_id === flow.id)`
      joins (`pages/AgentOpsDesk.tsx:60, 81`)
- [ ] delete the legacy whole-document fetch in `src/agentops/lib/api.ts`
      (`agentOpsStateURL()`); single biggest scale cliff, must leave the
      tree in the same PR as the hook rewrite
- [ ] `src/agentops/types/index.ts` is regenerated from OpenAPI; any
      hand-maintained interface there is deleted, not re-homed
- [ ] preserve `localStorage` prefs (`src/agentops/lib/preferences.ts`)
- [ ] entity-type renderers come from active packs (W6 / W7); a generic
      fallback renderer ships in core for entity types the active packs do
      not provide a view for
- [ ] core map surface ships beside timeline / graph / table views using the
      existing Leaflet path first; packs configure map layers, styling, and
      entity popovers rather than owning bespoke map code
- [ ] base-map defaults use open/free sources first (`OpenStreetMap`,
      `OpenFreeMap`, self-hosted tiles later if needed); overlays are served
      as GeoJSON from the API

---

## Workstream W6 — Pack System

Rationale: Red Line #5 says no plugins. Red Line #11 says packs declare
ontology and detectors as data. This workstream builds the loader and the
file format.

### W6.1 On-Disk Layout

```
packs/
  drones/
    pack.yaml              metadata: name, version, description, owner
    ontology.yaml          declared entity types and edge types
    detectors/
      <id>.yaml            subgraph patterns, severity, suggested actions
    views/
      <entity_type>.yaml   field order, hide/show, format hints
    maps/
      layers.yaml          basemap defaults, overlay defs, popup templates
    queries/
      <id>.yaml            saved query templates with parameters
    reports/
      <id>.md.tmpl         Go template for report generation
  scada/
    ... (same shape)
```

### W6.2 Loader

- [ ] `internal/packs/loader.go` reads `packs/*/pack.yaml` at startup,
      validates schema, registers entity types and edge types into the
      graph type whitelist, registers detectors, registers view templates,
      registers map layer definitions, registers query templates
- [ ] loader rejects on collision (two packs declaring the same entity
      type); collisions are a build-time error, never runtime
- [ ] loader is order-stable: pack file paths are sorted lexicographically
      so two operators with the same `packs/` directory get the same
      registration result
- [ ] no hot reload; restart to change packs (Red Line #12)
- [ ] `go test` covers: empty pack dir, valid pack, malformed YAML, type
      collision, missing required field

### W6.3 Pack Schema (Top-Level)

```yaml
name: drones
version: 0.1.0
description: Unmanned systems pack — readiness, FMEA, EW correlation
owner: scalytics
requires:
  core_min_version: "0.5.0"
entity_types:
  - id: platform
    display: Platform
    canonical_id_format: "<serial>"
  - id: subsystem
    ...
edge_types:
  - id: installed_in
    src_types: [component]
    dst_types: [subsystem]
    temporal: true
  - ...
map_layers:
  - id: live-platforms
    geometry_source: entity_geometry
    entity_types: [platform]
    render: point
```

- [ ] `pack.yaml` schema versioned; version field determines parser path
- [ ] entity_types and edge_types declarations validated against the graph
      capability matrix at load time
- [ ] map layer declarations validated against the core geometry capability
      matrix at load time; packs may style / label / filter only, not change
      the spatial storage contract

### W6.4 Detector File Schema

```yaml
id: cohort-failure-early-warning
severity: high
window: 72h
match:
  pattern: |
    SELECT fault_mode, COUNT(DISTINCT platform_id) AS n
      FROM edges_view_fault_per_platform_variant
     WHERE valid_from > NOW() - INTERVAL '72 hours'
     GROUP BY fault_mode, variant
    HAVING COUNT(DISTINCT platform_id) >= 3
explanation_template: >
  {{n}} platforms of variant {{variant}} produced fault {{fault_mode}}
  within 72h; possible software regression.
suggested_actions:
  - inspect autonomy software lineage on affected platforms
  - flag autonomy version for rollback review
```

- [ ] detector queries are SQL against named views (no arbitrary code
      execution)
- [ ] each detector run writes a `provenance` row with the detector id,
      pack name, and the matched subject ids
- [ ] CI: every shipped pack's detectors execute against a fixture DB
      without error

---

## Workstream W7 — Drones Pack Implementation

Implements the pack file set under `packs/drones/`, per
`arch/gotham-lite-drones-pack.md`.

### W7.1 Ontology

Declared entity types: `platform`, `variant`, `subsystem`, `component`,
`software`, `mission`, `sortie`, `contact`, `area`, `ew_event`, `fault`,
`fault_mode`, `operator`, `team`, `trial`, `signoff`.

Declared edge types (selection):

- `subsystem -part_of-> platform`
- `component -installed_in-> subsystem [valid_from, valid_to]`
- `mission -assigned_to-> platform`, `sortie -part_of-> mission`
- `sortie -experienced-> fault -instance_of-> fault_mode`
- `fault -suspected_cause-> component`
- `sortie -encountered-> ew_event -affected-> subsystem`
- `platform -runs-> software [valid_from, valid_to]`
- `signoff -references-> trial`, `signoff -approves-> platform`

### W7.2 Detectors (v1)

- `cohort-failure-early-warning` — same `fault_mode` on ≥ 3 platforms of
  the same `variant` in 72h
- `roe-drift` — sortie track crosses `area.kind=no-engage` polygon
- `silent-subsystem` — heartbeat gap > expected × 3 during active sortie
- `autonomy-rollback-candidate` — cross-variant fault rate spike aligned
  with a `software` version deployment
- `lot-anomaly` — `fault_mode:X` rate on `component.lot=Y` > 3σ above
  fleet baseline
- `pre-mission-readiness-gap` — open faults or open `signoff` items on
  selected `platform` for an upcoming `mission`

### W7.3 Views, Queries, Reports

- views: `platform`, `subsystem`, `component`, `mission`, `sortie`,
  `ew_event`, `fault`, `signoff`
- query templates: "what changed since last signoff", "same failure mode
  across fleet", "platforms running software X", "sorties with degraded
  comms in area Z"
- report templates: validation report, no-go decision memo, post-incident
  engineering review

### W7.4 Acceptance Criteria

Validated in W12, not during the middle of the refactor.

- `packs/drones/pack.yaml` validates and loads under W6
- all v1 detectors execute against a fixture DB in CI
- a synthetic dataset (10 platforms, 200 sorties, 50 faults) renders the
  drones-specific entity views in the analyst UI
- `/api/v1/ontology/types` lists all declared entity types under
  `source: pack/drones`

---

## Workstream W8 — SCADA Pack Implementation

Implements the pack file set under `packs/scada/`, per
`arch/gotham-lite-scada-pack.md`. Parallel structure to W7.

### W8.1 Ontology

Declared entity types: `plant`, `zone`, `device`, `firmware`, `process`,
`tag`, `alarm`, `alarm_event`, `change`, `engineer`, `operator`,
`vendor_tech`, `session`, `work_order`, `vulnerability`, `tradecraft`.

Declared edge types (selection):

- `device -in-> zone -in-> plant`
- `device -runs-> firmware [valid_from, valid_to] -vulnerable_to-> vulnerability`
- `engineer -authenticated_on-> device [valid_from, valid_to]`
- `engineer -applied-> change -modified-> device`
- `change -justified_by-> work_order`
- `change -affected-> tag`
- `alarm_event -fired_on-> alarm -defined_on-> tag`
- `device -controls-> process -reports-> tag`

### W8.2 Detectors (v1)

- `purdue-violation` — L1 device speaking directly to L4 without conduit
  via L3 / L3.5
- `change-without-work-order` — `change` without inbound `-justified_by->
  work_order` within 15-minute window
- `firmware-drift` — asset-registry `expected_firmware` ≠ observed
  `firmware`, ranked by criticality
- `tradecraft-match` — subgraph isomorphism against MITRE ATT&CK for ICS
  patterns (ship Triton, CRASHOVERRIDE, INDUSTROYER, FrostyGoop, Volt
  Typhoon as fixture patterns)
- `stale-session` — `engineer -authenticated_on-> device` open > 24h
  without activity
- `alarm-flood-after-change` — alarm rate spike within N hours of
  `change -affected-> tag` chain

### W8.3 Views, Queries, Reports

- views: `plant`, `zone`, `device`, `process`, `tag`, `change`,
  `alarm_event`, `vulnerability`, `tradecraft`
- query templates: "every device vulnerable to CVE X by criticality",
  "every change to tags A, B, C in last 72h", "every change without WO in
  30d", "every session > 24h on safety PLCs", "every Triton-pattern write
  sequence"
- report templates: CVE exposure memo, change-audit pack
  (NERC CIP / NIS2 / 21 CFR Part 11 variants), post-incident engineering
  review, compliance evidence bundle

### W8.4 Acceptance Criteria

Validated in W12, not during the middle of the refactor.

- `packs/scada/pack.yaml` validates and loads under W6
- all v1 detectors execute against a fixture DB in CI
- a synthetic dataset (1 plant, 4 zones, 30 devices, 120 tags, 200
  changes, 500 alarm events) renders the SCADA-specific entity views
- `/api/v1/ontology/types` lists all declared entity types under
  `source: pack/scada`
- with both packs loaded, `/api/v1/ontology/packs` returns both with
  no type collisions (proves the core is pack-agnostic — Red Line #13)

---

## Workstream W9 — Operational

- [ ] collector container keeps its existing Dockerfile; runs
      `cmd/kafsiem-collector`
- [ ] new Compose service `kafsiem-api` runs `cmd/kafsiem-api`; mounts the
      same `/data` volume read-only (`ro` mount flag)
- [ ] both services mount `/packs` (read-only) which contains the active
      pack directories; image build copies `packs/drones/` and
      `packs/scada/` into `/packs/`
- [ ] Caddy (`docker/Caddyfile`) reverse-proxies `/api/*` to
      `kafsiem-api:8081`; `/api/*` never resolves to the collector
- [ ] `docker/Dockerfile` grows a second build-stage target for the API
      binary; same base image
- [ ] `/data/agentops.db` (plus `*-wal`, `*-shm` sidecars) added to the
      docker volume contract
- [ ] `dev-start` brings up both services; `dev-start-collector` and
      `dev-start-api` for one-at-a-time debugging
- [ ] `--reset-agentops` flag on the collector deletes the DB file
- [ ] `--packs <path>` flag on both binaries; default `/packs/`
- [ ] `go mod tidy`; promote to direct deps: `modernc.org/sqlite`,
      `franz-go`, `go-chi/chi/v5`, the chosen OpenAPI toolchain, the
      chosen YAML parser
- [ ] `make ci` must pass; `make api-lint` runs spectral against
      `api/openapi.yaml`; `make pack-lint` runs the pack validator against
      every pack under `packs/`

---

## Workstream W10 — UI Improvements

Rationale: W1–W9 unlock a better UI but do not require it. This workstream
is the minimum set of UI changes that let an analyst actually use the new
graph and packs. Existing layout
(`src/agentops/pages/AgentOpsDesk.tsx`) stays the default home; new
surfaces live alongside (Red Line #9).

### W10.1 What Is Already Good (Preserve)

- conversation-centric Run Queue → Timeline → Context three-column default
- anomaly-forward filters at `pages/AgentOpsDesk.tsx:204-226`
- `Panel` / `MetricCard` / `StatusRow` / `Tag` / `EmptyState` vocabulary
  from `src/agentops/components/Chrome.tsx`
- replay-as-investigation model with optimistic local state
- per-panel `persistQueueFilter` / `persistSelectedRunId` prefs

### W10.2 What Is Missing For Link-Analysis UX

- no entity-centric view (can't pivot from "agent X" to "all runs")
- no graph visualization at all
- anomaly derivation is four hardcoded rules
  (`lib/investigation.ts:204-247`); not pack-driven
- full-text / structured search is absent
- all joins client-side with `slice(0, 16)` caps, so large flows truncate
- raw messages are a tab, so provenance walk-back takes multiple clicks

### W10.3 Tasks

Entity view (new top-level surface, pack-aware):

- [ ] route `?view=entity&type=<t>&id=<id>` renders `EntityProfilePage`
- [ ] panels: Identity, Activity Timeline (server-paginated),
      Neighborhood Graph, Related Flows, Provenance
- [ ] field order and labels read from active pack's `views/<type>.yaml`;
      core fallback view for unmapped types
- [ ] `useEntityNeighborhood(type, id, depth)` hook backed by W4 API
- [ ] cross-links from Run Queue / Run Context: any rendered sender_id /
      trace / task / pack-declared field becomes a clickable entity chip

Graph pane:

- [ ] `react-flow` or `cytoscape` after spike (Red Line #8 allowlist)
- [ ] `src/agentops/components/GraphCanvas.tsx`; edge-type colors come
      from active pack
- [ ] new tab `Topology` in Investigation Workspace tabs
      (`pages/AgentOpsDesk.tsx:487`): `replay | failures | raw |
      operator | topology`
- [ ] canvas consumes `Neighborhood` API for the selected flow's
      correlation entity at depth 2

Command bar / search:

- [ ] `CommandBar` at the top of `AgentOpsDesk`
- [ ] grammar v1: `agent:<id>`, `topic:<family>`, `status:<value>`,
      `window:<duration>`, plus pack-declared shortcuts
      (`platform:<serial>`, `device:<id>`)
- [ ] backend `/api/v1/search` (committed in W4) returns ranked entities,
      flows, and detector hits
- [ ] results render in Run Queue panel; clear button restores filter
      chip state

Provenance walk:

- [ ] every anomaly / event card gets a "why" affordance that pops a
      drawer showing the provenance chain for that subject
- [ ] data source: `/api/v1/entities/{type}/{id}/provenance` (committed in
      W4, handler in W2 / W4)

Server-paginated lists:

- [ ] replace all `.slice(0, N)` sites in `pages/AgentOpsDesk.tsx` with
      W5 paginated hooks

Saved investigations (minimal):

- [ ] `localStorage` key `kafsiem.investigation.<id>` stores
      `{pinnedEntities, notes, openedAt}`; no backend persistence yet
- [ ] new panel: `notes`
- [ ] pin affordance on entity chips and flow rows

### W10.4 Out Of Scope For W10

- replacing the three-column default home (Red Line #9)
- a new design system (stay inside `Chrome.tsx`)
- a Palantir-Workshop clone

### W10.5 Acceptance Criteria

Validated in W12, not during the middle of the refactor.

- from a selected run, two clicks reach the 2-hop neighborhood of its
  correlation entity rendered in the graph canvas
- the Command Bar can answer `platform:auv-07 window:1h` (drones pack
  loaded) and `device:plc-12 window:1h` (SCADA pack loaded)
- every alert or anomaly renders a provenance drawer that walks back to
  the source record offset
- no panel renders more than 50 rows without server-side pagination

---

## Workstream W11 — Documentation

Docs are not optional for either ICP — both rely on auditable evidence
production and customer or auditor self-service.

- [ ] `docs/architecture.md` — kafSIEM's place alongside KafScale and
      kafclaw-shaped agents, the in-repo package layout, the
      collector/api binary boundary
- [ ] `docs/agentops-operator-guide.md` rewrite — two-process model, DB
      file and WAL sidecars, back-up posture (WAL-aware snapshot, not raw
      file copy), pack file layout, link to `api/openapi.yaml`
- [ ] `docs/packs/` — one page per pack: drones, scada. Each page lists
      declared entity types, edge types, detectors, views, query
      templates, and report templates. Generated from the pack YAML by
      `make pack-docs` so docs cannot drift from the pack itself
- [ ] `docs/agentops-api-errors.md` — RFC 9457 problem-type registry
- [ ] `docs/api-clients.md` — how to consume the generated TypeScript
      client; how a third-party Go client would consume the generated
      Go server stubs
- [ ] `docs/upgrades/v1-to-v2.md` — placeholder; populated only when
      v2 is actually planned (Red Line #3)
- [ ] `README.md` top-level — refreshed with the Gotham-lite framing, the
      two ICPs, and a link to the foundation tasks

Acceptance Criteria:

Validated in W12, not during the middle of the refactor.

- a new operator can stand up the dev environment from `README.md` alone
- a new contributor can write a new detector for an existing pack from
  `docs/packs/<name>.md` alone
- a customer auditor can answer "what entity types and edges does this
  deployment recognize" from `/api/v1/ontology/types` plus
  `docs/packs/`

---

## Workstream W12 — Consolidated Acceptance, Coverage, And Performance

This is the final gate after the refactor work is done. During implementation
we keep targeted correctness checks green. We do not treat every sub-step as a
full acceptance pass. The expensive demo, UI, load, coverage, and performance
verification is centralized here.

### W12.1 Full Test Sweep

- [ ] `go test ./...`
- [ ] frontend test sweep for the active analyst surfaces
- [ ] production build passes (`npm run build`)
- [ ] generated artifacts are clean (`git diff --exit-code` after codegen)

### W12.2 Coverage Gate

- [ ] consolidated Go coverage report produced for the touched workstreams:
      `internal/agentops/`, `internal/graph/`, `internal/packs/`
- [ ] remaining low-coverage hotspots are called out explicitly before merge
- [ ] coverage delta is recorded in the task log before merge

### W12.3 Demo And UI Smoke

- [ ] AgentOps demo (`npm run demo:agentops`) renders the Flow Desk
- [ ] pack-aware UI smoke: flow view, graph view, map view, provenance walk,
      replay controls
- [ ] GIS smoke: shared map surface renders GeoJSON features, basemap loads,
      at least one drones overlay and one SCADA overlay render correctly

### W12.4 Operational And Performance Checks

- [ ] synthetic load test of 10k messages across 500 flows completes without
      the collector process exceeding 150 MB RSS or writing more than the
      message rate × ~4 KB to disk per record
- [ ] `handleRecord` latency at steady state stays under 5 ms p99 on the
      demo box
- [ ] graph neighborhood / path queries meet the W3 latency targets on the
      synthetic DB
- [ ] API readiness and health endpoints behave correctly against the
      collector-written SQLite DB

### W12.5 Final Acceptance Gate

- [ ] the JSON snapshot file is no longer written; `store/file.go` is gone
- [ ] consuming any KafClaw flow produces a connected component in
      `entities` + `edges` reachable from the correlation entity
- [ ] every edge has a matching `provenance` row
- [ ] exactly two packs ship and load cleanly: drones and SCADA
- [ ] operator and contributor docs are sufficient for a fresh setup and a
      new detector / view addition
- [ ] final merge only happens after W12 is green

---

## Sequencing

Each PR maps to a workstream and at most one red-line commitment; if a PR
would cross more, it is split before it lands.

1. **W0 first.** Module reorg, no behavior change. Single PR. Locks the
   package layout (Red Line #10).
2. **W1.** SQLite store replaces JSON snapshot. Red Lines #2, #7.
3. **W2.** Entity graph schema + edge writers + provenance. Red Lines #4, #11.
4. **W3.** Query primitives on store and graph. No HTTP yet.
5. **W4.** Binary split: `cmd/kafsiem-api`, chi, OpenAPI, generated TS
   client, entity-centric URLs, ontology discovery endpoints. Red Lines
   #1, #3.
6. **W5.** Frontend cutover. Same PR as W4 or immediately following.
7. **W6.** Pack loader and file format. Red Lines #5, #11, #12.
8. **W7 + W8 in parallel.** Drones and SCADA packs ship together; one
   pack alone would not prove the core is pack-agnostic (Red Line #13).
9. **W9.** Operational close-out: Compose, Caddy, dep promotion, volume
   contract on `/data/agentops.db`.
10. **W10.** Pack-aware UI: entity view, graph pane, command bar,
    provenance walk.
11. **W11.** Documentation pass.
12. **W12.** Consolidated acceptance, coverage, demo, and performance
    gate before any customer demo or merge-to-main candidate.

Rough effort estimate (single engineer, dense weeks): W0 = 0.5w, W1 =
1w, W2 = 1w, W3 = 0.5w, W4 = 2w (OpenAPI bootstrap), W5 = 0.5w, W6 =
1w, W7 = 1w, W8 = 1w, W9 = 0.5w, W10 = 2w, W11 = 0.5w. Total ≈ 11.5
engineer-weeks. Two-person split: one on W0–W4 + W6 + W9, one on W5 +
W7 + W8 + W10 + W11. Both can start once W0 lands.

## Exit Conditions

At the end of this workstream, kafSIEM:

- writes kafclaw-observed traffic and a typed entity / edge / provenance
  graph to a single SQLite file (`/data/agentops.db`) with WAL mode and
  indexed queries
- exposes the graph through a stateless `cmd/kafsiem-api` binary with an
  entity-centric, OpenAPI-contracted, versioned API
- ships exactly two packs (drones, SCADA) that declare ontology,
  detectors, views, query templates, and report templates as data
- presents a pack-aware analyst UI alongside the existing three-column
  home, with graph pivoting, server-paginated lists, and provenance
  walk-back
- documents kafSIEM's place alongside KafScale and kafclaw-shaped agents,
  the API contract, and each pack so a new contributor or auditor can
  self-serve

That is the credible foundation for the Gotham-lite mid-market narrative
across the unmanned-systems and SCADA / critical-infra ICPs. Everything
else — entity-resolution scoring, additional packs, multi-tenant, RBAC,
SSO — sits on top of this and should not be designed until this
foundation is in place.

## Optional Adjacent Integration — KafGraph As Enrichment Plane

`KafGraph` is not part of the kafSIEM product boundary and is not a dependency
of gotham-lite. It is, however, a plausible adjacent system that can enrich
kafSIEM if both are deployed in the same estate.

The architectural split is:

- `kafSIEM` owns the analyst-facing operational graph:
  - current entities / edges / provenance
  - pack-declared ontology
  - deterministic detectors
  - analyst UI and API
- `KafGraph` owns agent memory and learning:
  - long-horizon shared memory
  - semantic recall
  - reflection cycles
  - clustering and recurring-pattern discovery
  - agent-facing Brain Tool APIs

The boundary rule is strict:

- kafSIEM must remain fully usable without `KafGraph`
- `KafGraph` must never become the source of truth for kafSIEM entity state,
  edge state, pack registration, or analyst provenance
- any integration is over event exchange or API calls, never shared internal
  storage

Recommended integration model:

1. `kafclaw` agents communicate over KafScale as normal
2. `KafGraph` consumes that traffic into its own memory system
3. kafSIEM consumes the same traffic into its own SQLite-backed operational
   graph
4. `KafGraph` may emit selected enrichment artifacts back onto KafScale or
   expose them via API for kafSIEM to ingest as optional context
5. analyst decisions from kafSIEM may be published back out as feedback for
   `KafGraph` learning loops

The only acceptable enrichment artifacts are bounded and explicit, for example:

- prior-similar-incident suggestion
- recurring failure cluster suggestion
- candidate software-regression pattern
- related historical artifact bundle
- agent-generated hypothesis with confidence and evidence references
- reflection summary for a repeated operational pattern

These must enter kafSIEM as attributed external context, not as silent writes
to the core graph. If ingested, they are represented as normal rows with
provenance that records `source = kafgraph`.

What stays in kafSIEM:

- pack-defined entities and edges
- operational readiness and incident state
- detector execution that drives analyst-visible alerts
- provenance chain used for analyst citations and reports

What stays in `KafGraph`:

- shared agent memory
- semantic and vector search over prior conversations and artifacts
- reflection outputs
- cross-agent learning state
- exploratory clustering that is useful but not authoritative

This gives us a clean posture:

- gotham-lite does not depend on `KafGraph`
- `KafGraph` can make the system smarter when present
- the product boundary remains clear
- future integration does not force a rewrite of the core SQLite graph model
