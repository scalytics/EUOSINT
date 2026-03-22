#!/usr/bin/env bash
set -euo pipefail

env_file="${1:-.env}"

if [[ ! -f "$env_file" ]]; then
  touch "$env_file"
fi

current_token="$(grep -E '^API_BEARER_TOKEN=' "$env_file" | tail -n1 | cut -d'=' -f2- || true)"
if [[ -n "${current_token}" ]]; then
  echo "API_BEARER_TOKEN already set in ${env_file}"
  exit 0
fi

new_token="$(LC_ALL=C tr -dc 'A-Za-z0-9' </dev/urandom | head -c 40 || true)"
if [[ "${#new_token}" -lt 40 ]]; then
  new_token="$(openssl rand -hex 24 | cut -c1-40)"
fi
tmp_file="$(mktemp)"

awk -v token="$new_token" '
BEGIN { replaced = 0 }
/^API_BEARER_TOKEN=/ {
  print "API_BEARER_TOKEN=" token
  replaced = 1
  next
}
{ print }
END {
  if (!replaced) {
    print "API_BEARER_TOKEN=" token
  }
}
' "$env_file" >"$tmp_file"

mv "$tmp_file" "$env_file"
echo "API_BEARER_TOKEN generated in ${env_file}"
