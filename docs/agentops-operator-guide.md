# AgentOps Operator Guide

AgentOps is the Kafka-backed operator surface for KafClaw traffic over Kafscale.

## Deployment Contract

Use an immutable image plus mounted config and data volumes:

- `/config`
  - `agentops_policy.yaml`
  - optional UI policy files
- `/data`
  - `agentops-state.json`
  - replay session metadata
  - effective runtime snapshot artifacts

AgentOps treats a missing `/config/agentops_policy.yaml` as "use built-in defaults".

## Runtime Surface

Required environment:

- `AGENTOPS_ENABLED=true`
- `AGENTOPS_BROKERS=<broker list>`
- `AGENTOPS_GROUP_NAME=<kafclaw group name>`
- `AGENTOPS_GROUP_ID=<live tracking group>`
- `UI_MODE=AGENTOPS` or `UI_MODE=HYBRID`

Important optional environment:

- `AGENTOPS_POLICY_PATH=/config/agentops_policy.yaml`
- `AGENTOPS_REPLAY_ENABLED=true`
- `AGENTOPS_REPLAY_PREFIX=euosint-agentops-replay`
- `AGENTOPS_REJECT_TOPIC=group.<group>.agentops.rejects`
- `AGENTOPS_OUTPUT_PATH=/data/agentops-state.json`

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

Flow reconstruction is keyed by `correlation_id`. Trace reconstruction is keyed by `trace_id`. Task chains are keyed by `task_id` and `parent_task_id`.

## Replay Safety

Replay always uses a dedicated consumer group derived from `AGENTOPS_REPLAY_PREFIX`.

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

## Kafscale Operator Surface

AgentOps uses Kafka admin APIs already implemented by Kafscale:

- `ListGroups`
- `DescribeGroups`

The UI only exposes read-only group visibility backed by those capabilities.
Offset reset or destructive replay mutation is intentionally not exposed.

## Example: KafClaw Over Kafscale

1. Run Kafscale and expose Kafka brokers.
2. Configure AgentOps with the KafClaw group name.
3. Mount `/config/agentops_policy.yaml`.
4. Start EUOSINT with `UI_MODE=AGENTOPS`.
5. Open the Flow Desk and inspect live group plus replay groups.

## Example: Dedicated Replay Group

1. Open AgentOps.
2. Trigger replay.
3. Verify the replay group ID uses the configured replay prefix.
4. Confirm the live tracking group remains unchanged.

## Example: Hybrid OSINT Context

Hybrid mode is selective. It shows OSINT context only when an explicit match exists on:

- category
- geography
- sector
- vendor
- product
- CVE
- time-window proximity
