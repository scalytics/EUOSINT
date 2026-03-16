#!/bin/sh
set -eu

seed_if_missing() {
  source_path="$1"
  target_path="$2"

  if [ ! -f "$target_path" ] && [ -f "$source_path" ]; then
    cp "$source_path" "$target_path"
  fi
}

mkdir -p /data

seed_if_missing /app/public-defaults/alerts.json /data/alerts.json
seed_if_missing /app/public-defaults/alerts-filtered.json /data/alerts-filtered.json
seed_if_missing /app/public-defaults/alerts-state.json /data/alerts-state.json
seed_if_missing /app/public-defaults/source-health.json /data/source-health.json

exec euosint-collector "$@"
