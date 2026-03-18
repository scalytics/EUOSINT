<!--
Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
SPDX-License-Identifier: Apache-2.0
-->

# Collector Migration

The collector runtime is now fully Go-based. The Node collector has been retired from operational paths, and scheduled feed generation, Docker runtime, and local commands all run through `cmd/euosint-collector`.

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

## Outcome

- Source registry remains external in `registry/source_registry.json`
- Scheduled feed generation runs through the Go collector
- Docker runtime runs the Go collector sidecar plus the Caddy-served UI
