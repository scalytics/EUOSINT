# AgentOps Execution Checklist

Strict implementation order. No drift, no side quests.

## Global Rules

- [x] One concern per commit.
- [x] Every commit leaves the system buildable and testable.
- [x] Every behavior change ships with tests in the same commit.
- [ ] No UI work before backend contracts, transport semantics, and health models are stable.
- [ ] No replay UI before replay runtime and safety rules exist.
- [ ] No docs/examples before code and tests exist.
- [x] Use KafClaw and Kafscale semantics as the source of truth.
- [x] AgentOps decodes normal message content from Kafka.
- [x] LFS-backed records are pointer-only in AgentOps by default.

## Product Contract

- [x] `OSINT` mode stays as-is.
- [x] `AGENTOPS` mode tracks KafClaw group traffic over Kafka.
- [x] `HYBRID` mode adds selective OSINT context to a chosen agent flow or trace.
- [x] Tracking is consumer-group safe.
- [x] Replay never mutates the live tracking group.
- [x] Normal Kafka message bodies are viewable in AgentOps details.
- [x] LFS-backed records show pointer metadata only, not blob content.
- [x] AgentOps lives in its own bounded code domain, not a generic `plugins/` tree.

## Target Code Structure

Backend target:

- [x] `internal/agentops/config`
- [x] `internal/agentops/kafka`
- [x] `internal/agentops/contract`
- [x] `internal/agentops/flow`
- [x] `internal/agentops/trace`
- [x] `internal/agentops/replay`
- [x] `internal/agentops/health`
- [x] `internal/agentops/store`
- [x] `internal/agentops/api`

Frontend target:

- [ ] `src/agentops/pages`
- [ ] `src/agentops/components`
- [ ] `src/agentops/lib`
- [ ] `src/agentops/types`

Shared only where justified:

- [x] shared app bootstrap only
- [x] shared config wiring only
- [x] shared mode switch only
- [x] shared storage-path conventions only

## Commit 1: Config Contract

Deliver:

- [x] create `internal/agentops` domain root
- [x] `AGENTOPS_ENABLED`
- [x] `AGENTOPS_BROKERS`
- [x] `AGENTOPS_GROUP_NAME`
- [x] `AGENTOPS_GROUP_ID`
- [x] `AGENTOPS_CLIENT_ID`
- [x] `AGENTOPS_TOPIC_MODE`
- [x] `AGENTOPS_TOPICS`
- [x] `AGENTOPS_SECURITY_PROTOCOL`
- [x] `AGENTOPS_SASL_MECHANISM`
- [x] `AGENTOPS_USERNAME`
- [x] `AGENTOPS_PASSWORD`
- [x] `AGENTOPS_TLS_INSECURE_SKIP_VERIFY`
- [x] `AGENTOPS_POLICY_PATH`
- [x] `AGENTOPS_REPLAY_ENABLED`
- [x] `AGENTOPS_REPLAY_PREFIX`
- [x] `UI_MODE=OSINT|AGENTOPS|HYBRID`
- [x] `PROFILE=osint-default|agentops-default|hybrid-ops`

Tests:

- [x] env parsing
- [x] defaults
- [x] invalid enum handling
- [x] topic auto/manual resolution selection
- [x] config precedence

Gate:

- [x] no runtime behavior yet

## Commit 2: Policy Schema And Validation

Deliver:

- [x] `agentops_policy.yaml` schema
- [x] topic family enable/disable validation
- [x] required vs optional topic validation
- [x] grouping policy validation
- [x] replay limits validation
- [x] hybrid category visibility validation against registry taxonomy

Tests:

- [x] valid policy fixture
- [x] invalid policy fixtures
- [x] unknown hybrid category rejection
- [x] invalid replay limit rejection
- [x] invalid topic family rejection

Gate:

- [x] fail fast on invalid startup config

## Commit 3: Topic Resolution And KafClaw Contract

Deliver:

- [x] deterministic KafClaw topic derivation from group name
- [x] manual topic override path
- [x] topic-family catalog for `announce`
- [x] topic-family catalog for `control.roster`
- [x] topic-family catalog for `control.onboarding`
- [x] topic-family catalog for `requests`
- [x] topic-family catalog for `responses`
- [x] topic-family catalog for `tasks.status`
- [x] topic-family catalog for `traces`
- [x] topic-family catalog for `observe.audit`
- [x] topic-family catalog for `memory.shared`
- [x] topic-family catalog for `memory.context`
- [x] topic-family catalog for `orchestrator`
- [x] topic-family catalog for dynamic skill topics

Tests:

- [x] topic derivation for group name
- [x] required topic family set
- [x] optional topic family set
- [x] manual override behavior
- [x] topic-family classification

Gate:

- [x] topic plan is deterministic and test-backed

## Commit 4: Kafka Runtime Foundation

Deliver:

- [x] franz-go client construction
- [x] long-lived tracking consumer loop
- [x] bounded polling
- [x] SASL/TLS/auth wiring
- [x] startup validation
- [x] shutdown behavior
- [x] source bootstrap health

Tests:

- [x] client option construction
- [x] startup validation
- [x] lifecycle with mocks/fakes
- [x] timeout and fetch error propagation
- [x] disabled-source behavior

Gate:

- [x] AgentOps transport degrades independently from OSINT

## Commit 5: Envelope Parsing And Outcome Model

Deliver:

- [x] `GroupEnvelope` parsing
- [x] topic-family aware payload parsing
- [x] `accepted` outcome
- [x] `rejected` outcome
- [ ] optional `mirrored` outcome
- [x] commit after outcome resolution
- [x] reject counters
- [x] last-reject sample
- [ ] optional reject-topic publish path

Tests:

- [x] valid envelope accepted
- [x] invalid JSON rejected and committed
- [x] invalid topic/payload rejected and committed
- [ ] mirrored reject path
- [ ] mirror failure behavior
- [x] poison-record non-replay

Gate:

- [x] no silent drops

## Commit 6: Flow Normalization

Deliver:

- [x] canonical flow model keyed by `correlation_id`
- [x] linked topic set
- [x] linked sender set
- [x] linked trace IDs
- [x] linked task IDs
- [x] first seen / last seen
- [x] latest status
- [x] envelope type counts

Tests:

- [x] multi-topic flow aggregation
- [x] multi-sender flow aggregation
- [x] missing correlation handling
- [x] stable flow identity
- [x] out-of-order record handling

Gate:

- [x] AgentOps can reconstruct a usable flow from real KafClaw traffic

## Commit 7: Trace And Task Chain Model

Deliver:

- [x] trace-span normalization keyed by `trace_id`
- [x] parent/child span linkage
- [x] task chain model keyed by `task_id`
- [x] delegation linkage via `parent_task_id`
- [x] status-chain modeling from `tasks.status`
- [x] audit event attachment

Tests:

- [x] trace graph assembly
- [x] task chain assembly
- [x] delegation depth handling
- [x] audit attachment
- [x] mixed request/response/status correlation

Gate:

- [x] graph and task-chain semantics use protocol facts, not heuristics

## Commit 8: Content Decode Policy

Deliver:

- [x] normal message decoding from Kafka record value
- [x] payload preview generation for queue/detail views
- [x] full decoded content loading for selected flow / trace / task
- [x] LFS envelope detection
- [x] pointer-only LFS rendering model
- [x] `s3://bucket/key` presentation model

Tests:

- [x] normal JSON message decoding
- [ ] non-JSON but valid raw-content handling
- [x] preview truncation behavior
- [x] LFS envelope detection
- [x] LFS pointer metadata rendering contract
- [x] no blob fetch in default AgentOps path

Gate:

- [x] normal messages are readable
- [x] LFS records stay metadata-only

## Commit 9: Topic Health And Flow Metrics

Deliver:

- [x] per-topic message counters
- [ ] message-density buckets
- [x] active-agent counts
- [x] stale-topic detection
- [x] flow counters
- [x] accepted/rejected/mirrored totals in source health

Tests:

- [x] topic health aggregation
- [ ] stale-topic behavior
- [ ] active-agent counting
- [ ] flow-count aggregation
- [x] health snapshot serialization

Gate:

- [x] operators can see whether AgentOps transport is healthy before opening UI

## Commit 10: Persistence Contract

Deliver:

- [x] mounted `/config` support
- [x] mounted `/data` support
- [x] policy loading from `/config`
- [x] derived AgentOps state persisted in `/data`
- [x] replay session metadata persisted in `/data`
- [x] effective-config snapshot persisted for diagnostics

Tests:

- [x] startup with mounted config
- [x] persisted state reload
- [ ] missing config file handling
- [x] read-only image assumptions

Gate:

- [x] Docker deployment model is stable and upgrade-safe

## Commit 11: Replay Runtime

Deliver:

- [x] dedicated replay session model
- [x] replay consumer-group naming from `AGENTOPS_REPLAY_PREFIX`
- [x] replay from earliest support
- [ ] scoped topic subscription
- [x] replay status lifecycle
- [ ] replay record counters
- [x] replay isolation from live tracking group

Tests:

- [x] replay group naming
- [x] replay-from-earliest behavior
- [ ] replay session lifecycle
- [x] replay does not mutate live group state
- [ ] replay cancellation
- [ ] replay health reporting

Gate:

- [x] replay is safe and explicit

## Commit 12: UI Mode And Profile Framework

Deliver:

- [x] `OSINT`
- [x] `AGENTOPS`
- [x] `HYBRID`
- [x] `PROFILE=osint-default|agentops-default|hybrid-ops`
- [x] capability gating per mode
- [x] effective policy/status exposure
- [x] empty-state behavior

Tests:

- [x] mode resolution
- [ ] profile defaults
- [x] route gating
- [x] unsupported panel suppression
- [ ] mode persistence

Gate:

- [x] one obvious default experience per mode

## Commit 13: AgentOps Information Architecture

Deliver:

- [x] `Flow Desk` shell
- [x] `Flow Queue`
- [x] `Trace Graph`
- [x] `Agent Context`
- [x] `Topic Health`
- [x] `Replay Panel`
- [x] message detail drawer or panel

Required queue semantics:

- [x] group by `correlation_id`
- [x] show topic count
- [x] show sender count
- [x] show latest event time
- [x] show latest known status
- [x] show payload preview when available

Required detail semantics:

- [x] decoded normal-message content view
- [x] LFS pointer metadata view
- [x] trace/task/topic linkage
- [x] offset / partition / timestamp visibility

Tests:

- [x] AgentOps route loads correct layout
- [x] queue grouping render
- [x] trace graph selection flow
- [x] topic health panel render
- [x] replay panel render
- [x] normal message detail render
- [x] LFS pointer-only detail render

Gate:

- [x] AgentOps home screen is flow-first, not map-first

## Commit 14: Hybrid Information Architecture

Deliver:

- [x] `Fusion Desk` shell
- [x] `Agent Flow`
- [x] `External Intel Context`
- [x] `Fusion Summary`

Required fusion semantics:

- [x] no fusion unless explicit rule matches
- [ ] supported match on registry category
- [ ] supported match on geography
- [ ] supported match on sector
- [ ] supported match on vendor / product
- [ ] supported match on CVE
- [ ] supported match on time-window proximity

Tests:

- [x] hybrid route loads correct layout
- [x] no unlabeled mixed stream
- [ ] matched fusion render
- [ ] no-match fallback render

Gate:

- [x] hybrid stays selective and low-noise

## Commit 15: Kafscale Operator Integration Surface

Deliver:

- [ ] consumer-group visibility integration
- [ ] group list support for operator UI
- [ ] group describe support for operator UI
- [ ] live tracking group visibility
- [ ] replay group visibility
- [ ] clear unsupported-action handling where Kafscale does not expose reset semantics

Tests:

- [ ] group-list response handling
- [ ] group-describe response handling
- [ ] missing or unsupported admin API behavior
- [ ] operator error rendering

Gate:

- [ ] UI only exposes transport actions backed by real Kafscale capabilities

## Commit 16: Documentation And Examples

Deliver:

- [x] tighten root README after implementation is complete
- [x] add AgentOps product positioning to README
- [x] add mode explanation to README
- [x] add Kafka-content vs LFS-pointer behavior to README
- [ ] operator deployment guide
- [ ] mounted `/config` and `/data` contract
- [ ] `agentops_policy.yaml` example
- [ ] KafClaw topic model reference
- [ ] Kafscale deployment example
- [ ] replay safety explanation
- [ ] hybrid example with OSINT context
- [x] explicit note that normal messages are decoded from Kafka
- [x] explicit note that LFS records are shown as pointers only

Required examples:

- [ ] KafClaw agents over Kafscale with AgentOps tracking
- [ ] replaying an agent flow from earliest through a dedicated replay group
- [ ] hybrid OSINT context on a selected agent trace

Tests:

- [ ] validate example config files
- [ ] smoke-test docs commands where practical

Gate:

- [ ] docs reflect shipped behavior, not aspirational behavior
- [ ] README reflects actual final product structure and operator workflow

## Release Gates

- [x] `go test ./...` passes
- [x] frontend tests for mode routing and AgentOps panels pass
- [ ] config fixtures cover valid and invalid AgentOps policies
- [ ] replay flow is tested end-to-end with a non-live consumer group
- [ ] health output exposes accepted/rejected/mirrored counts and effective topic set
- [ ] no remaining generic “SIEM alert queue” language in user-facing docs or UI
- [x] no default-path blob fetching for LFS-backed records
