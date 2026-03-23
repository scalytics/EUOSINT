#!/usr/bin/env bash
# EUOSINT remote installer
# Usage:
#   wget -qO- https://raw.githubusercontent.com/scalytics/EUOSINT/main/deploy/install.sh | bash
#
# Optional environment overrides:
#   REPO_URL=https://github.com/scalytics/EUOSINT.git
#   REPO_REF=main
#   INSTALL_DIR=$HOME/euosint
#   IMAGE_TAG=latest

set -euo pipefail

REPO_URL="${REPO_URL:-https://github.com/scalytics/EUOSINT.git}"
REPO_REF="${REPO_REF:-main}"
INSTALL_DIR="${INSTALL_DIR:-$HOME/euosint}"
IMAGE_TAG="${IMAGE_TAG:-latest}"
INSTALL_MODE="${INSTALL_MODE:-update}"
TLS_MODE="false"
REPO_SLUG=""

info() { echo "[euosint-install] $*"; }
warn() { echo "[euosint-install][warn] $*" >&2; }
fatal() { echo "[euosint-install][error] $*" >&2; exit 1; }

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
    "source_candidates.json"
    "source_dead_letter.json"
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
  local watchdog_service="$INSTALL_DIR/docs/euosint-browser-watchdog.service"
  local watchdog_timer="$INSTALL_DIR/docs/euosint-browser-watchdog.timer"
  local tmp_file

  mkdir -p "$INSTALL_DIR/scripts" "$INSTALL_DIR/docs"

  tmp_file="$(mktemp)"
  fetch_repo_file "scripts/browser_watchdog.sh" "$tmp_file"
  mv "$tmp_file" "$watchdog_script"
  chmod +x "$watchdog_script"

  tmp_file="$(mktemp)"
  fetch_repo_file "docs/euosint-browser-watchdog.service" "$tmp_file"
  mv "$tmp_file" "$watchdog_service"

  tmp_file="$(mktemp)"
  fetch_repo_file "docs/euosint-browser-watchdog.timer" "$tmp_file"
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

  info "Review essential configuration only — press Enter to keep current value, or type a new one."
  info "Advanced tuning stays on built-in defaults. Edit .env manually only if needed."
  echo ""

  local last_comment=""
  while IFS= read -r line; do
    # Print comment lines as section headers.
    if [[ "$line" =~ ^[[:space:]]*# ]]; then
      last_comment="$line"
      continue
    fi
    [[ "$line" =~ ^[[:space:]]*$ ]] && continue

    local key="${line%%=*}"
    [[ -z "$key" ]] && continue
    local example_val="${line#*=}"

    # Ports are auto-derived; bearer token is auto-rotated — skip.
    case "$key" in
      EUOSINT_HTTP_PORT|EUOSINT_HTTPS_PORT|API_BEARER_TOKEN) continue ;;
    esac

    # Current value from .env if present, otherwise .env.example default.
    local current_val
    if grep -qE "^${key}=" "$env_file" 2>/dev/null; then
      current_val="$(grep -E "^${key}=" "$env_file" | head -1 | cut -d= -f2-)"
    else
      current_val="$example_val"
    fi

    # Print section comment once before its first key.
    if [[ -n "$last_comment" ]]; then
      echo "$last_comment"
      last_comment=""
    fi

    # Mask secrets in the display but keep the real value as default.
    local display_val="$current_val"
    case "$key" in
      *API_KEY*|*PASSWORD*|*SECRET*)
        if [[ -n "$current_val" ]]; then
          display_val="****${current_val: -4}"
        fi
        ;;
    esac

    local prompt_label="$key"
    case "$key" in
      EUOSINT_SITE_ADDRESS) prompt_label="Live URL" ;;
      COLLECTOR_ROLE) prompt_label="Collector Role" ;;
      UCDP_ACCESS_TOKEN) prompt_label="UCDP API Token" ;;
      ACLED_USERNAME) prompt_label="ACLED Username" ;;
      ACLED_PASSWORD) prompt_label="ACLED Password" ;;
      SOURCE_VETTING_ENABLED) prompt_label="Enable Source Vetting LLM" ;;
      SOURCE_VETTING_API_KEY) prompt_label="Source Vetting API Key" ;;
      SOURCE_VETTING_MODEL) prompt_label="Source Vetting Model" ;;
      ALERT_LLM_ENABLED) prompt_label="Enable Alert LLM (higher token usage)" ;;
      ALERT_LLM_MODEL) prompt_label="Alert LLM Model" ;;
    esac

    local new_val
    new_val="$(read_prompt "  $prompt_label [$display_val]: ")"
    if [[ -n "$new_val" ]]; then
      upsert_env "$env_file" "$key" "$new_val"
    else
      upsert_env "$env_file" "$key" "$current_val"
    fi
  done < "$example_file"

  # Auto-configure ports based on site address.
  local site_addr
  site_addr="$(grep -E '^EUOSINT_SITE_ADDRESS=' "$env_file" | head -1 | cut -d= -f2-)"
  if [[ -n "$site_addr" && "$site_addr" != ":80" && "$site_addr" != ":8080" ]]; then
    TLS_MODE="true"
    upsert_env "$env_file" "EUOSINT_HTTP_PORT" "80"
    upsert_env "$env_file" "EUOSINT_HTTPS_PORT" "443"
    info "Domain '$site_addr' detected — ports set to 80/443 for automatic TLS."
    warn "Ensure DNS A/AAAA records point to this host and inbound 80/443 are open."
  else
    TLS_MODE="false"
    upsert_env "$env_file" "EUOSINT_HTTP_PORT" "8080"
    upsert_env "$env_file" "EUOSINT_HTTPS_PORT" "8443"
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

  site_addr="$(grep -E '^EUOSINT_SITE_ADDRESS=' "$env_file" | head -1 | cut -d= -f2- || true)"
  http_port="$(grep -E '^EUOSINT_HTTP_PORT=' "$env_file" | head -1 | cut -d= -f2- || echo "8080")"
  https_port="$(grep -E '^EUOSINT_HTTPS_PORT=' "$env_file" | head -1 | cut -d= -f2- || echo "8443")"
  host_name="$(hostname -f 2>/dev/null || hostname)"

  if [[ -n "$site_addr" && "$site_addr" != ":80" && "$site_addr" != ":8080" ]]; then
    live_url="https://${site_addr}"
  else
    live_url="http://${host_name}:${http_port}"
  fi

  compose_url="$(raw_repo_url "docker-compose.yml")"
  watchdog_url="$(raw_repo_url "scripts/browser_watchdog.sh")"

  echo ""
  echo "================ EUOSINT Setup Summary ================"
  echo "Install Mode : ${INSTALL_MODE}"
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

  if [[ "$INSTALL_MODE" == "update" ]] && container_running_for_service "euosint"; then
    info "Existing euosint service is running; skipping strict local port-collision pre-check."
  else
    info "Validating that ports 80 and 443 are free..."
    validate_tls_ports
  fi
}

prompt_reset_zone_briefs() {
  local env_file="$INSTALL_DIR/.env"
  local choice
  choice="$(read_prompt "Reset conflict history and analysis? [no]: ")"
  choice="${choice:-no}"
  choice="$(echo "$choice" | tr '[:upper:]' '[:lower:]')"
  if [[ "$choice" == "yes" || "$choice" == "y" ]]; then
    upsert_env "$env_file" "RESET_ZONE_BRIEF_LLM" "1"
    info "Zone brief LLM history will be regenerated on next collector startup."
  else
    upsert_env "$env_file" "RESET_ZONE_BRIEF_LLM" "0"
  fi
}

start_stack() {
  local start_choice
  start_choice="$(read_prompt "Start/restart EUOSINT now? [yes]: ")"
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
      docker volume rm "${COMPOSE_PROJECT_NAME:-euosint}_caddy-data" 2>/dev/null || true
      docker volume rm "${COMPOSE_PROJECT_NAME:-euosint}_caddy-config" 2>/dev/null || true
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

  local site_addr http_port host_name live_url
  site_addr="$(grep -E '^EUOSINT_SITE_ADDRESS=' "$INSTALL_DIR/.env" | head -1 | cut -d= -f2- || true)"
  http_port="$(grep -E '^EUOSINT_HTTP_PORT=' "$INSTALL_DIR/.env" | cut -d= -f2 || echo "8080")"
  host_name="$(hostname -f 2>/dev/null || hostname)"
  if [[ -n "$site_addr" && "$site_addr" != ":80" && "$site_addr" != ":8080" ]]; then
    live_url="https://${site_addr}"
  else
    live_url="http://${host_name}:${http_port}"
  fi
  info "EUOSINT started. Live URL: ${live_url}"
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
  svc_file="$user_unit_dir/euosint-browser-watchdog.service"
  timer_file="$user_unit_dir/euosint-browser-watchdog.timer"

  cp "$INSTALL_DIR/docs/euosint-browser-watchdog.service" "$svc_file"
  cp "$INSTALL_DIR/docs/euosint-browser-watchdog.timer" "$timer_file"
  sed -i.bak -E "s|^WorkingDirectory=.*$|WorkingDirectory=${INSTALL_DIR}|" "$svc_file"
  sed -i.bak -E "s|^ExecStart=.*$|ExecStart=/bin/bash ${INSTALL_DIR}/scripts/browser_watchdog.sh|" "$svc_file"

  systemctl --user daemon-reload
  systemctl --user enable --now euosint-browser-watchdog.timer
  info "Enabled user timer: euosint-browser-watchdog.timer"
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
