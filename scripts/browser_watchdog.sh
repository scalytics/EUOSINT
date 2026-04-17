#!/usr/bin/env bash
# Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

BROWSER_CONTAINER="${BROWSER_CONTAINER:-euosint-browser}"
COLLECTOR_CONTAINER="${COLLECTOR_CONTAINER:-kafsiem-collector-1}"
WINDOW_MINUTES="${WINDOW_MINUTES:-10}"
FAILURE_THRESHOLD="${FAILURE_THRESHOLD:-3}"
COOLDOWN_SECONDS="${COOLDOWN_SECONDS:-120}"
FAILURE_PATTERN="${FAILURE_PATTERN:-remote browser unreachable}"
STATE_FILE="${STATE_FILE:-/tmp/euosint-browser-watchdog.state}"

now_epoch="$(date +%s)"
last_restart_epoch="0"
if [[ -f "$STATE_FILE" ]]; then
  last_restart_epoch="$(cat "$STATE_FILE" 2>/dev/null || echo 0)"
fi

restart_browser() {
  local reason="$1"
  if (( now_epoch - last_restart_epoch < COOLDOWN_SECONDS )); then
    echo "watchdog: skip restart (cooldown active) reason=${reason}"
    return 0
  fi
  echo "watchdog: restarting ${BROWSER_CONTAINER} reason=${reason}"
  docker restart "$BROWSER_CONTAINER" >/dev/null
  date +%s >"$STATE_FILE"
}

if ! docker ps --format '{{.Names}}' | grep -qx "$BROWSER_CONTAINER"; then
  restart_browser "browser-not-running"
  exit 0
fi

health_status="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' "$BROWSER_CONTAINER" 2>/dev/null || echo unknown)"
if [[ "$health_status" == "unhealthy" || "$health_status" == "starting" || "$health_status" == "unknown" ]]; then
  restart_browser "browser-health-${health_status}"
  exit 0
fi

fallback_count="$(
  docker logs --since "${WINDOW_MINUTES}m" "$COLLECTOR_CONTAINER" 2>&1 \
    | grep -c "$FAILURE_PATTERN" || true
)"
if [[ "${fallback_count}" -ge "${FAILURE_THRESHOLD}" ]]; then
  restart_browser "collector-ws-failures-${fallback_count}"
  exit 0
fi

echo "watchdog: healthy (health=${health_status}, ws_fallbacks=${fallback_count})"
