# Collector Migration

The existing Node collector in `scripts/fetch-alerts.mjs` remains the reference implementation until the Go collector reaches output parity.

Operational rule: scheduled feed generation must stay on the legacy Node collector until parity is explicitly verified. The Go collector can ship in images and local tooling before it becomes the production feed generator.

## Goals

- Isolate the operational collector from the npm dependency tree.
- Keep the React dashboard unchanged while the ingestion engine is migrated.
- Port behavior in small slices with parity checks against the current JSON outputs.

## Initial Go Boundary

- CLI entrypoint: `cmd/euosint-collector`
- Config and runtime wiring: `internal/collector/app`, `internal/collector/config`
- Domain models: `internal/collector/model`
- Registry loading and validation: `internal/collector/registry`

## Migration Order

1. Registry loading and source validation
2. Source fetchers by transport type
3. Parser and normalization pipeline
4. Deduplication and scoring parity
5. Output writers for alerts, state, filtered alerts, and source health
6. Watch mode and retry orchestration

## Coexistence Rule

Until the Go collector can reproduce the Node collector outputs for a representative fixture set, production collection stays on Node.
