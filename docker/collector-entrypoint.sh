#!/bin/sh
# Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
# SPDX-License-Identifier: Apache-2.0

set -eu

seed_if_missing() {
  source_path="$1"
  target_path="$2"

  if [ ! -f "$target_path" ] && [ -f "$source_path" ]; then
    cp "$source_path" "$target_path"
  fi
}

init_json_if_missing() {
  target_path="$1"
  payload="$2"
  if [ ! -f "$target_path" ]; then
    printf '%s\n' "$payload" > "$target_path"
  fi
}

mkdir -p /data

# Start fresh volumes with empty JSON documents to avoid serving stale
# baked snapshots from previous registry revisions.
init_json_if_missing /data/alerts.json '[]'
init_json_if_missing /data/alerts-filtered.json '[]'
init_json_if_missing /data/alerts-state.json '[]'
init_json_if_missing /data/source-health.json '{"generated_at":"","critical_source_prefixes":[],"fail_on_critical_source_gap":false,"total_sources":0,"sources_ok":0,"sources_error":0,"duplicate_audit":{"suppressed_variant_duplicates":0,"repeated_title_groups_in_active":0,"repeated_title_samples":[]},"sources":[]}'
seed_if_missing /app/registry/source_candidates.json /data/source_candidates.json

if [ ! -f /data/sources.db ]; then
  if [ -f /app/registry/sources.seed.db ]; then
    cp /app/registry/sources.seed.db /data/sources.db
    echo "Seeded sources.db from pre-built snapshot"
  else
    euosint-collector --source-db /data/sources.db --source-db-init
    euosint-collector --source-db /data/sources.db --registry /app/registry/source_registry.json --source-db-import-registry
    if [ -f /app/registry/curated_agencies.seed.json ]; then
      euosint-collector --source-db /data/sources.db --curated-seed /app/registry/curated_agencies.seed.json --source-db-merge-registry
    fi
  fi
fi

# Always merge the baked-in JSON registry into the DB on startup.
# MergeRegistry upserts only — it adds new sources and updates existing
# ones but never deletes discovered or runtime-added sources.
# This ensures new feeds (FBI API, travel warnings, etc.) from image
# updates are picked up without manual intervention.
euosint-collector --source-db /data/sources.db --curated-seed /app/registry/source_registry.json --source-db-merge-registry

exec euosint-collector "$@"
