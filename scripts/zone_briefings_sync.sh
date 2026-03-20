#!/usr/bin/env bash
# Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

COLLECTOR_CONTAINER="${COLLECTOR_CONTAINER:-euosint-collector-1}"

if ! command -v docker >/dev/null 2>&1; then
  echo "zone-briefings-sync: docker not found" >&2
  exit 1
fi

if ! docker ps --format '{{.Names}}' | grep -qx "$COLLECTOR_CONTAINER"; then
  echo "zone-briefings-sync: collector container not running (${COLLECTOR_CONTAINER})"
  exit 0
fi

echo "zone-briefings-sync: syncing cached UCDP zone briefings"
docker exec "$COLLECTOR_CONTAINER" \
  euosint-collector \
  --zone-briefings-sync-only \
  --registry /data/sources.db \
  --zone-briefings-output /data/zone-briefings.json
