#!/usr/bin/env bash
# kafSIEM remote installer
# Usage:
#   wget -qO- https://raw.githubusercontent.com/scalytics/kafSIEM/main/deploy/install.sh | bash
#
# Optional environment overrides:
#   REPO_URL=https://github.com/scalytics/kafSIEM.git
#   REPO_REF=main
#   INSTALL_DIR=$HOME/kafsiem
#   IMAGE_TAG=latest

set -euo pipefail

REPO_URL="${REPO_URL:-https://github.com/scalytics/kafSIEM.git}"
REPO_REF="${REPO_REF:-main}"
INSTALL_DIR="${INSTALL_DIR:-$HOME/kafsiem}"
IMAGE_TAG="${IMAGE_TAG:-latest}"
INSTALL_MODE="${INSTALL_MODE:-update}"
TLS_MODE="false"
REPO_SLUG=""
RESET_ZONE_BRIEF_REQUESTED="0"
OPERATING_MODE="${OPERATING_MODE:-OSINT}"

info() { echo "[kafsiem-install] $*"; }
warn() { echo "[kafsiem-install][warn] $*" >&2; }
fatal() { echo "[kafsiem-install][error] $*" >&2; exit 1; }

read_prompt() {
  local prompt_text="$1"
  local out
  if [[ -t 0 ]]; then
    read -r -p "$prompt_text" out
  elif [[ -r /dev/tty ]]; then
    read -r -p "$prompt_text" out < /dev/tty
  else
    out=""
  fi
  echo "$out"
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || fatal "Missing required command: $1"
}

prompt_install_mode() {
  local value
  while true; do
    value="$(read_prompt "Install mode (update/install) [${INSTALL_MODE}]: ")"
    value="${value:-$INSTALL_MODE}"
    value="$(echo "$value" | tr '[:upper:]' '[:lower:]')"
    case "$value" in
      update|install) echo "$value"; return 0 ;;
      *) echo "Please answer 'update' or 'install'." ;;
    esac
  done
}

prompt_operating_mode() {
  local current="${1:-OSINT}"
  local value
  while true; do
    value="$(read_prompt "Operating mode (OSINT/AGENTOPS/HYBRID) [${current}]: ")"
    value="${value:-$current}"
    value="$(echo "$value" | tr '[:lower:]' '[:upper:]')"
    case "$value" in
      OSINT|AGENTOPS|HYBRID) echo "$value"; return 0 ;;
      *) echo "Please answer 'OSINT', 'AGENTOPS', or 'HYBRID'." ;;
    esac
  done
}

env_value() {
  local file="$1"
  local key="$2"
  local fallback="${3:-}"
  if grep -qE "^${key}=" "$file" 2>/dev/null; then
    grep -E "^${key}=" "$file" | head -1 | cut -d= -f2-
  else
    echo "$fallback"
  fi
}

bool_default() {
  local raw
  raw="$(echo "${1:-false}" | tr '[:upper:]' '[:lower:]')"
  case "$raw" in
    1|true|yes|y|on) echo "yes" ;;
    *) echo "no" ;;
  esac
}

prompt_yes_no() {
  local label="$1"
  local current="${2:-false}"
  local value
  local default_choice
  default_choice="$(bool_default "$current")"
  while true; do
    value="$(read_prompt "${label} [${default_choice}]: ")"
    value="${value:-$default_choice}"
    value="$(echo "$value" | tr '[:upper:]' '[:lower:]')"
    case "$value" in
      yes|y|true|1|on) echo "true"; return 0 ;;
      no|n|false|0|off) echo "false"; return 0 ;;
      *) echo "Please answer 'yes' or 'no'." ;;
    esac
  done
}

prompt_choice() {
  local label="$1"
  local current="$2"
  shift 2
  local choices=("$@")
  local value
  while true; do
    value="$(read_prompt "${label} [${current}]: ")"
    value="${value:-$current}"
    for choice in "${choices[@]}"; do
      if [[ "$value" == "$choice" ]]; then
        echo "$value"
        return 0
      fi
    done
    echo "Please answer one of: ${choices[*]}"
  done
}

prompt_env_value() {
  local file="$1"
  local key="$2"
  local label="$3"
  local sensitive="${4:-false}"
  local current_val
  local display_val
  local new_val

  current_val="$(env_value "$file" "$key")"
  display_val="$current_val"
  if [[ "$sensitive" == "true" && -n "$current_val" ]]; then
    display_val="****${current_val: -4}"
  fi

  new_val="$(read_prompt "  ${label} [${display_val}]: ")"
  if [[ -n "$new_val" ]]; then
    upsert_env "$file" "$key" "$new_val"
  else
    upsert_env "$file" "$key" "$current_val"
  fi
}

prompt_env_bool() {
  local file="$1"
  local key="$2"
  local label="$3"
  local current_val
  local new_val

  current_val="$(env_value "$file" "$key" "false")"
  new_val="$(prompt_yes_no "  ${label}" "$current_val")"
  upsert_env "$file" "$key" "$new_val"
  echo "$new_val"
}

default_reject_topic() {
  local group_name="${1:-}"
  if [[ -z "$group_name" ]]; then
    echo ""
  else
    echo "group.${group_name}.agentops.rejects"
  fi
}

configure_operating_profile() {
  local env_file="$1"
  local mode="$2"

  case "$mode" in
    OSINT)
      upsert_env "$env_file" "UI_MODE" "OSINT"
      upsert_env "$env_file" "PROFILE" "osint-default"
      upsert_env "$env_file" "AGENTOPS_ENABLED" "false"
      ;;
    AGENTOPS)
      upsert_env "$env_file" "UI_MODE" "AGENTOPS"
      upsert_env "$env_file" "PROFILE" "agentops-default"
      upsert_env "$env_file" "AGENTOPS_ENABLED" "true"
      ;;
    HYBRID)
      upsert_env "$env_file" "UI_MODE" "HYBRID"
      upsert_env "$env_file" "PROFILE" "hybrid-ops"
      upsert_env "$env_file" "AGENTOPS_ENABLED" "true"
      ;;
  esac
}

configure_common_settings() {
  local env_file="$1"
  echo "# Common setup"
  prompt_env_value "$env_file" "KAFSIEM_SITE_ADDRESS" "Live URL"
}

configure_osint_settings() {
  local env_file="$1"
  local source_vetting_enabled
  local alert_llm_enabled

  echo ""
  echo "# OSINT setup"
  prompt_env_value "$env_file" "UCDP_ACCESS_TOKEN" "UCDP API Token"
  prompt_env_value "$env_file" "ACLED_USERNAME" "ACLED Username"
  prompt_env_value "$env_file" "ACLED_PASSWORD" "ACLED Password" "true"

  source_vetting_enabled="$(prompt_env_bool "$env_file" "SOURCE_VETTING_ENABLED" "Enable Source Vetting LLM")"
  if [[ "$source_vetting_enabled" == "true" ]]; then
    prompt_env_value "$env_file" "SOURCE_VETTING_API_KEY" "Source Vetting API Key" "true"
    prompt_env_value "$env_file" "SOURCE_VETTING_MODEL" "Source Vetting Model"
  fi

  alert_llm_enabled="$(prompt_env_bool "$env_file" "ALERT_LLM_ENABLED" "Enable Alert LLM (higher token usage)")"
  if [[ "$alert_llm_enabled" == "true" ]]; then
    prompt_env_value "$env_file" "ALERT_LLM_MODEL" "Alert LLM Model"
  fi
}

configure_agentops_settings() {
  local env_file="$1"
  local security_protocol
  local sasl_mechanism
  local topic_mode
  local replay_enabled
  local mirror_rejects
  local group_name
  local reject_topic_current
  local reject_topic_default
  local reject_topic

  echo ""
  echo "# AgentOps setup"
  prompt_env_value "$env_file" "AGENTOPS_BROKERS" "Kafka brokers (comma-separated)"
  prompt_env_value "$env_file" "AGENTOPS_GROUP_NAME" "Agent group name"
  prompt_env_value "$env_file" "AGENTOPS_GROUP_ID" "Live tracking group id"
  prompt_env_value "$env_file" "AGENTOPS_CLIENT_ID" "Consumer client id"

  security_protocol="$(prompt_choice "  Kafka security protocol" "$(env_value "$env_file" "AGENTOPS_SECURITY_PROTOCOL" "PLAINTEXT")" "PLAINTEXT" "SSL" "SASL_PLAINTEXT" "SASL_SSL")"
  upsert_env "$env_file" "AGENTOPS_SECURITY_PROTOCOL" "$security_protocol"

  if [[ "$security_protocol" == SASL_* ]]; then
    sasl_mechanism="$(prompt_choice "  SASL mechanism" "$(env_value "$env_file" "AGENTOPS_SASL_MECHANISM" "PLAIN")" "PLAIN" "SCRAM-SHA-256" "SCRAM-SHA-512")"
    upsert_env "$env_file" "AGENTOPS_SASL_MECHANISM" "$sasl_mechanism"
    prompt_env_value "$env_file" "AGENTOPS_USERNAME" "Kafka username"
    prompt_env_value "$env_file" "AGENTOPS_PASSWORD" "Kafka password" "true"
  else
    upsert_env "$env_file" "AGENTOPS_USERNAME" ""
    upsert_env "$env_file" "AGENTOPS_PASSWORD" ""
  fi

  topic_mode="$(prompt_choice "  Topic mode" "$(env_value "$env_file" "AGENTOPS_TOPIC_MODE" "auto")" "auto" "manual")"
  upsert_env "$env_file" "AGENTOPS_TOPIC_MODE" "$topic_mode"
  if [[ "$topic_mode" == "manual" ]]; then
    prompt_env_value "$env_file" "AGENTOPS_TOPICS" "Topics (comma-separated)"
  else
    upsert_env "$env_file" "AGENTOPS_TOPICS" ""
  fi

  replay_enabled="$(prompt_env_bool "$env_file" "AGENTOPS_REPLAY_ENABLED" "Enable replay")"
  if [[ "$replay_enabled" == "true" ]]; then
    group_name="$(env_value "$env_file" "AGENTOPS_GROUP_NAME")"
    reject_topic_current="$(env_value "$env_file" "AGENTOPS_REJECT_TOPIC")"
    mirror_rejects="$(prompt_yes_no "  Mirror rejected records to Kafka" "$(if [[ -n "$reject_topic_current" ]]; then echo true; else echo false; fi)")"
    if [[ "$mirror_rejects" == "true" ]]; then
      reject_topic_default="$reject_topic_current"
      if [[ -z "$reject_topic_default" ]]; then
        reject_topic_default="$(default_reject_topic "$group_name")"
      fi
      reject_topic="$(read_prompt "  Reject topic [${reject_topic_default}]: ")"
      reject_topic="${reject_topic:-$reject_topic_default}"
      upsert_env "$env_file" "AGENTOPS_REJECT_TOPIC" "$reject_topic"
    else
      upsert_env "$env_file" "AGENTOPS_REJECT_TOPIC" ""
    fi
  else
    upsert_env "$env_file" "AGENTOPS_REJECT_TOPIC" ""
  fi
}

ensure_docker() {
  if ! command -v docker >/dev/null 2>&1; then
    fatal "Docker is not installed. Install Docker Engine + Compose plugin first, then re-run."
  fi
  if ! docker info >/dev/null 2>&1; then
    fatal "Docker daemon is not reachable. Start Docker and re-run."
  fi

  if docker compose version >/dev/null 2>&1; then
    COMPOSE_CMD="docker compose"
    return 0
  fi
  if command -v docker-compose >/dev/null 2>&1; then
    COMPOSE_CMD="docker-compose"
    return 0
  fi
  fatal "Docker Compose is not available (need 'docker compose' plugin or 'docker-compose')."
}

clone_or_update_repo() {
  if [[ -d "$INSTALL_DIR/.git" ]]; then
    info "Repository already exists in $INSTALL_DIR. Updating to $REPO_REF."
    git -C "$INSTALL_DIR" fetch --tags origin
    git -C "$INSTALL_DIR" checkout "$REPO_REF"
    git -C "$INSTALL_DIR" pull --ff-only origin "$REPO_REF" || true
  elif [[ -d "$INSTALL_DIR" && -n "$(ls -A "$INSTALL_DIR" 2>/dev/null || true)" ]]; then
    fatal "Install directory exists and is not empty: $INSTALL_DIR"
  else
    info "Cloning $REPO_URL into $INSTALL_DIR."
    git clone --depth 1 --branch "$REPO_REF" "$REPO_URL" "$INSTALL_DIR"
  fi
}

repo_slug() {
  local url="$REPO_URL"
  url="${url#https://github.com/}"
  url="${url#http://github.com/}"
  url="${url#git@github.com:}"
  url="${url%.git}"
  echo "$url"
}

fetch_repo_file() {
  local path="$1"
  local out="$2"
  local raw_url
  REPO_SLUG="${REPO_SLUG:-$(repo_slug)}"
  raw_url="https://raw.githubusercontent.com/${REPO_SLUG}/${REPO_REF}/${path}"

  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$raw_url" -o "$out"
    return 0
  fi
  if command -v wget >/dev/null 2>&1; then
    wget -qO "$out" "$raw_url"
    return 0
  fi
  fatal "Need curl or wget to fetch ${path} from ${raw_url}"
}

raw_repo_url() {
  local path="$1"
  REPO_SLUG="${REPO_SLUG:-$(repo_slug)}"
  echo "https://raw.githubusercontent.com/${REPO_SLUG}/${REPO_REF}/${path}"
}

refresh_compose_yaml_direct() {
  local compose_file="$INSTALL_DIR/docker-compose.yml"
  local tmp_file
  local backup_file

  [[ -d "$INSTALL_DIR" ]] || fatal "Install dir not found: $INSTALL_DIR"

  tmp_file="$(mktemp)"
  fetch_repo_file "docker-compose.yml" "$tmp_file"

  if [[ -f "$compose_file" ]]; then
    backup_file="$compose_file.preupdate.$(date +%Y%m%d%H%M%S).bak"
    cp "$compose_file" "$backup_file"
  else
    backup_file=""
  fi
  mv "$tmp_file" "$compose_file"
  if [[ -n "$backup_file" ]]; then
    info "Refreshed docker-compose.yml from repository ref '$REPO_REF' (backup: $backup_file)."
  else
    info "Fetched docker-compose.yml from repository ref '$REPO_REF'."
  fi
}

refresh_env_example_direct() {
  local env_example="$INSTALL_DIR/.env.example"
  local tmp_file
  local backup_file

  [[ -d "$INSTALL_DIR" ]] || fatal "Update mode requires existing install dir: $INSTALL_DIR"

  tmp_file="$(mktemp)"
  fetch_repo_file ".env.example" "$tmp_file"

  if [[ -f "$env_example" ]]; then
    backup_file="$env_example.preupdate.$(date +%Y%m%d%H%M%S).bak"
    cp "$env_example" "$backup_file"
    info "Backed up .env.example -> $backup_file"
  fi
  mv "$tmp_file" "$env_example"
  info "Refreshed .env.example from repository ref '$REPO_REF'."
}

refresh_web_runtime_files_direct() {
  local caddyfile="$INSTALL_DIR/docker/Caddyfile"
  local tmp_file
  local backup_file

  tmp_file="$(mktemp)"
  fetch_repo_file "docker/Caddyfile" "$tmp_file"

  if [[ -f "$caddyfile" ]]; then
    backup_file="$caddyfile.preupdate.$(date +%Y%m%d%H%M%S).bak"
    cp "$caddyfile" "$backup_file"
    info "Backed up docker/Caddyfile -> $backup_file"
  fi
  mkdir -p "$(dirname "$caddyfile")"
  mv "$tmp_file" "$caddyfile"
  info "Refreshed docker/Caddyfile from repository ref '$REPO_REF'."
}

refresh_registry_files_direct() {
  local reg_dir="$INSTALL_DIR/registry"
  local geo_dir="$reg_dir/geo"
  local tmp_file

  mkdir -p "$reg_dir" "$geo_dir"

  local json_files=(
    "source_registry.json"
    "category_dictionary.json"
    "curated_agencies.seed.json"
    "incident_terms.json"
    "noise_policy.json"
    "noise_policy_b.json"
    "sovereign_official_statements.seed.json"
    "stop_words.json"
    "terror_actor_aliases.json"
  )

  for fname in "${json_files[@]}"; do
    tmp_file="$(mktemp)"
    fetch_repo_file "registry/${fname}" "$tmp_file"
    mv "$tmp_file" "$reg_dir/$fname"
  done

  local geo_files=(
    "countries-adm0.geojson"
    "terror-activity-seed.geojson"
  )

  for fname in "${geo_files[@]}"; do
    tmp_file="$(mktemp)"
    fetch_repo_file "registry/geo/${fname}" "$tmp_file"
    mv "$tmp_file" "$geo_dir/$fname"
  done

  info "Refreshed registry files from repository ref '$REPO_REF'."
}

refresh_watchdog_files_direct() {
  local watchdog_script="$INSTALL_DIR/scripts/browser_watchdog.sh"
  local watchdog_service="$INSTALL_DIR/docs/kafsiem-browser-watchdog.service"
  local watchdog_timer="$INSTALL_DIR/docs/kafsiem-browser-watchdog.timer"
  local tmp_file

  mkdir -p "$INSTALL_DIR/scripts" "$INSTALL_DIR/docs"

  tmp_file="$(mktemp)"
  fetch_repo_file "scripts/browser_watchdog.sh" "$tmp_file"
  mv "$tmp_file" "$watchdog_script"
  chmod +x "$watchdog_script"

  tmp_file="$(mktemp)"
  fetch_repo_file "docs/kafsiem-browser-watchdog.service" "$tmp_file"
  mv "$tmp_file" "$watchdog_service"

  tmp_file="$(mktemp)"
  fetch_repo_file "docs/kafsiem-browser-watchdog.timer" "$tmp_file"
  mv "$tmp_file" "$watchdog_timer"

  info "Refreshed browser watchdog files from repository ref '$REPO_REF'."
}

upsert_env() {
  local file="$1"
  local key="$2"
  local value="$3"
  if grep -qE "^${key}=" "$file"; then
    sed -i.bak -E "s|^${key}=.*$|${key}=${value}|" "$file"
  else
    echo "${key}=${value}" >> "$file"
  fi
}

configure_env() {
  local env_file="$INSTALL_DIR/.env"
  local example_file="$INSTALL_DIR/.env.example"

  [[ -f "$example_file" ]] || fatal "Missing .env.example in repository."

  if [[ -f "$env_file" ]]; then
    cp "$env_file" "$env_file.backup.$(date +%Y%m%d%H%M%S).bak"
    info "Existing .env backed up."
  else
    cp "$example_file" "$env_file"
    info "Created .env from .env.example."
  fi

  info "Guided setup only asks for the profile-relevant settings."
  info "Advanced tuning stays on built-in defaults. Edit .env or mounted policy files only if needed."
  echo ""
  OPERATING_MODE="$(prompt_operating_mode "$(env_value "$env_file" "UI_MODE" "OSINT")")"
  configure_operating_profile "$env_file" "$OPERATING_MODE"
  configure_common_settings "$env_file"
  case "$OPERATING_MODE" in
    OSINT)
      configure_osint_settings "$env_file"
      ;;
    AGENTOPS)
      configure_agentops_settings "$env_file"
      ;;
    HYBRID)
      configure_osint_settings "$env_file"
      configure_agentops_settings "$env_file"
      ;;
  esac

  # Auto-configure ports based on site address.
  local site_addr
  site_addr="$(grep -E '^KAFSIEM_SITE_ADDRESS=' "$env_file" | head -1 | cut -d= -f2-)"
  if [[ -n "$site_addr" && "$site_addr" != ":80" && "$site_addr" != ":8080" ]]; then
    TLS_MODE="true"
    upsert_env "$env_file" "KAFSIEM_HTTP_PORT" "80"
    upsert_env "$env_file" "KAFSIEM_HTTPS_PORT" "443"
    info "Domain '$site_addr' detected — ports set to 80/443 for automatic TLS."
    warn "Ensure DNS A/AAAA records point to this host and inbound 80/443 are open."
  else
    TLS_MODE="false"
    upsert_env "$env_file" "KAFSIEM_HTTP_PORT" "8080"
    upsert_env "$env_file" "KAFSIEM_HTTPS_PORT" "8443"
    info "Localhost mode — ports set to 8080/8443."
  fi

  # Auto-generate or rotate API bearer token on every install/update.
  # Shared between Caddy (injects on proxy) and collector (validates).
  local new_token
  new_token="$(head -c 32 /dev/urandom | base64 | tr -d '/+=' | head -c 40)"
  upsert_env "$env_file" "API_BEARER_TOKEN" "$new_token"
  info "API bearer token rotated."

  echo ""
  info "Configuration saved to $env_file"
}

print_runtime_summary() {
  local env_file="$INSTALL_DIR/.env"
  local site_addr host_name http_port https_port live_url compose_url watchdog_url

  site_addr="$(grep -E '^KAFSIEM_SITE_ADDRESS=' "$env_file" | head -1 | cut -d= -f2- || true)"
  http_port="$(grep -E '^KAFSIEM_HTTP_PORT=' "$env_file" | head -1 | cut -d= -f2- || echo "8080")"
  https_port="$(grep -E '^KAFSIEM_HTTPS_PORT=' "$env_file" | head -1 | cut -d= -f2- || echo "8443")"
  host_name="$(hostname -f 2>/dev/null || hostname)"

  if [[ -n "$site_addr" && "$site_addr" != ":80" && "$site_addr" != ":8080" ]]; then
    live_url="https://${site_addr}"
  else
    live_url="http://${host_name}:${http_port}"
  fi

  compose_url="$(raw_repo_url "docker-compose.yml")"
  watchdog_url="$(raw_repo_url "scripts/browser_watchdog.sh")"

  echo ""
  echo "================ kafSIEM Setup Summary ================"
  echo "Install Mode : ${INSTALL_MODE}"
  echo "Profile      : ${OPERATING_MODE}"
  echo "Install Dir  : ${INSTALL_DIR}"
  echo "Live URL     : ${live_url}"
  if [[ "$TLS_MODE" == "true" ]]; then
    echo "TLS Ports    : 80 / 443"
  else
    echo "HTTP Ports   : ${http_port} / ${https_port}"
  fi
  echo "Compose Cmd  : ${COMPOSE_CMD}"
  echo "Compose YAML : ${compose_url}"
  echo "Watchdog Src : ${watchdog_url}"
  echo "======================================================="
  echo ""
}

container_running_for_service() {
  local service="$1"
  local output
  output="$(cd "$INSTALL_DIR" && $COMPOSE_CMD ps --status running --services 2>/dev/null || true)"
  echo "$output" | grep -qx "$service"
}

find_port_listener() {
  local port="$1"

  if command -v lsof >/dev/null 2>&1; then
    lsof -nP -iTCP:"$port" -sTCP:LISTEN 2>/dev/null | awk 'NR==2 {print $1 " (pid " $2 ") " $9; exit}'
    return 0
  fi

  if command -v ss >/dev/null 2>&1; then
    ss -ltnp 2>/dev/null | awk -v p=":$port" '$4 ~ p {print $0; exit}'
    return 0
  fi

  return 1
}

validate_tls_ports() {
  local listener
  for port in 80 443; do
    listener="$(find_port_listener "$port" || true)"
    if [[ -n "$listener" ]]; then
      fatal "Port ${port} is already in use (${listener}). Free it before TLS mode startup."
    fi
  done
}

run_firewall_checks() {
  if command -v ufw >/dev/null 2>&1; then
    info "ufw detected. Current status:"
    ufw status || true
  elif command -v firewall-cmd >/dev/null 2>&1; then
    info "firewalld detected. Listing public zone rules:"
    firewall-cmd --zone=public --list-services || true
    firewall-cmd --zone=public --list-ports || true
  else
    warn "No ufw/firewalld command found; skipping firewall inspection."
  fi
}

preflight_tls_checks() {
  if [[ "$TLS_MODE" != "true" ]]; then
    return 0
  fi

  info "TLS mode detected (domain set)."
  local firewall_choice
  firewall_choice="$(read_prompt "Run firewall checks for 80/443? [yes]: ")"
  firewall_choice="${firewall_choice:-yes}"
  if [[ "$firewall_choice" == "yes" || "$firewall_choice" == "y" ]]; then
    run_firewall_checks
  fi

  if [[ "$INSTALL_MODE" == "update" ]] && container_running_for_service "kafsiem"; then
    info "Existing kafsiem service is running; skipping strict local port-collision pre-check."
  else
    info "Validating that ports 80 and 443 are free..."
    validate_tls_ports
  fi
}

prompt_reset_zone_briefs() {
  local env_file="$INSTALL_DIR/.env"
  local choice
  local default_choice="no"
  if [[ "$INSTALL_MODE" == "update" ]]; then
    default_choice="yes"
  fi
  choice="$(read_prompt "Reset conflict history and analysis? [${default_choice}]: ")"
  choice="${choice:-$default_choice}"
  choice="$(echo "$choice" | tr '[:upper:]' '[:lower:]')"
  if [[ "$choice" == "yes" || "$choice" == "y" ]]; then
    upsert_env "$env_file" "RESET_ZONE_BRIEF_LLM" "1"
    RESET_ZONE_BRIEF_REQUESTED="1"
    info "Zone brief LLM history will be regenerated on next collector startup."
  else
    upsert_env "$env_file" "RESET_ZONE_BRIEF_LLM" "0"
    RESET_ZONE_BRIEF_REQUESTED="0"
  fi
}

start_stack() {
  local start_choice
  start_choice="$(read_prompt "Start/restart kafSIEM now? [yes]: ")"
  start_choice="${start_choice:-yes}"
  start_choice="$(echo "$start_choice" | tr '[:upper:]' '[:lower:]')"
  if [[ "$start_choice" != "yes" && "$start_choice" != "y" ]]; then
    info "Skipped. Start later with: cd $INSTALL_DIR && $COMPOSE_CMD pull && $COMPOSE_CMD up -d --no-build"
    return 0
  fi

  preflight_tls_checks

  if [[ "$INSTALL_MODE" == "install" ]]; then
    info "Install mode: stopping stack (preserving feed-data volume)."
    (
      cd "$INSTALL_DIR"
      $COMPOSE_CMD down --remove-orphans || true
      docker volume rm "${COMPOSE_PROJECT_NAME:-kafsiem}_caddy-data" 2>/dev/null || true
      docker volume rm "${COMPOSE_PROJECT_NAME:-kafsiem}_caddy-config" 2>/dev/null || true
    )
  fi

  info "Pulling latest GHCR images..."
  (
    cd "$INSTALL_DIR"
    $COMPOSE_CMD pull
  )

  info "Starting stack..."
  (
    cd "$INSTALL_DIR"
    $COMPOSE_CMD up -d --no-build
  )

  # One-shot reset flag: keep enabled for this startup, then turn it off
  # to avoid wiping narratives on every future restart.
  if [[ "$RESET_ZONE_BRIEF_REQUESTED" == "1" ]]; then
    upsert_env "$INSTALL_DIR/.env" "RESET_ZONE_BRIEF_LLM" "0"
    info "RESET_ZONE_BRIEF_LLM auto-cleared to 0 after startup (one-shot reset applied)."
  fi

  local site_addr http_port host_name live_url
  site_addr="$(grep -E '^KAFSIEM_SITE_ADDRESS=' "$INSTALL_DIR/.env" | head -1 | cut -d= -f2- || true)"
  http_port="$(grep -E '^KAFSIEM_HTTP_PORT=' "$INSTALL_DIR/.env" | cut -d= -f2 || echo "8080")"
  host_name="$(hostname -f 2>/dev/null || hostname)"
  if [[ -n "$site_addr" && "$site_addr" != ":80" && "$site_addr" != ":8080" ]]; then
    live_url="https://${site_addr}"
  else
    live_url="http://${host_name}:${http_port}"
  fi
  info "kafSIEM started. Live URL: ${live_url}"
}

install_user_watchdog_timer() {
  local choice user_unit_dir svc_file timer_file
  choice="$(read_prompt "Install browser watchdog as user systemd timer? [yes]: ")"
  choice="${choice:-yes}"
  choice="$(echo "$choice" | tr '[:upper:]' '[:lower:]')"
  if [[ "$choice" != "yes" && "$choice" != "y" ]]; then
    info "Skipped watchdog timer setup."
    return 0
  fi

  command -v systemctl >/dev/null 2>&1 || {
    warn "systemctl not found; skipping user watchdog timer setup."
    return 0
  }

  user_unit_dir="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"
  mkdir -p "$user_unit_dir"
  svc_file="$user_unit_dir/kafsiem-browser-watchdog.service"
  timer_file="$user_unit_dir/kafsiem-browser-watchdog.timer"

  cp "$INSTALL_DIR/docs/kafsiem-browser-watchdog.service" "$svc_file"
  cp "$INSTALL_DIR/docs/kafsiem-browser-watchdog.timer" "$timer_file"
  sed -i.bak -E "s|^WorkingDirectory=.*$|WorkingDirectory=${INSTALL_DIR}|" "$svc_file"
  sed -i.bak -E "s|^ExecStart=.*$|ExecStart=/bin/bash ${INSTALL_DIR}/scripts/browser_watchdog.sh|" "$svc_file"

  systemctl --user daemon-reload
  systemctl --user enable --now kafsiem-browser-watchdog.timer
  info "Enabled user timer: kafsiem-browser-watchdog.timer"
  info "Tip: for boot-time execution without login, run once as root:"
  info "  sudo loginctl enable-linger $USER"
}

main() {
  ensure_docker
  INSTALL_MODE="$(prompt_install_mode)"
  info "Selected install mode: ${INSTALL_MODE}"
  REPO_SLUG="$(repo_slug)"

  if [[ "$INSTALL_MODE" == "update" ]]; then
    if [[ ! -d "$INSTALL_DIR" || ! -f "$INSTALL_DIR/docker-compose.yml" ]]; then
      fatal "Update mode requires an existing install at $INSTALL_DIR (missing docker-compose.yml). Use install mode first."
    fi
  else
    mkdir -p "$INSTALL_DIR"
  fi
  refresh_compose_yaml_direct
  refresh_env_example_direct
  refresh_web_runtime_files_direct
  refresh_registry_files_direct
  refresh_watchdog_files_direct

  configure_env
  prompt_reset_zone_briefs
  print_runtime_summary
  start_stack
  install_user_watchdog_timer
}

main "$@"
