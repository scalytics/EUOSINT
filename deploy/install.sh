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
  local slug
  local raw_url
  slug="$(repo_slug)"
  raw_url="https://raw.githubusercontent.com/${slug}/${REPO_REF}/${path}"

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

refresh_compose_yaml_direct() {
  local compose_file="$INSTALL_DIR/docker-compose.yml"
  local tmp_file
  local backup_file

  [[ "$INSTALL_MODE" == "update" ]] || return 0
  [[ -d "$INSTALL_DIR" ]] || fatal "Update mode requires existing install dir: $INSTALL_DIR"
  [[ -f "$compose_file" ]] || fatal "Update mode requires existing docker-compose.yml at $compose_file"

  tmp_file="$(mktemp)"
  fetch_repo_file "docker-compose.yml" "$tmp_file"

  backup_file="$compose_file.preupdate.$(date +%Y%m%d%H%M%S).bak"
  cp "$compose_file" "$backup_file"
  mv "$tmp_file" "$compose_file"
  info "Refreshed docker-compose.yml from repository ref '$REPO_REF' (backup: $backup_file)."
}

refresh_env_example_direct() {
  local env_example="$INSTALL_DIR/.env.example"
  local tmp_file
  local backup_file

  [[ "$INSTALL_MODE" == "update" ]] || return 0
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

  [[ "$INSTALL_MODE" == "update" ]] || return 0

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

  info "Review configuration — press Enter to keep current value, or type a new one."
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

    # Ports are auto-derived from EUOSINT_SITE_ADDRESS — skip.
    case "$key" in
      EUOSINT_HTTP_PORT|EUOSINT_HTTPS_PORT) continue ;;
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

    local new_val
    new_val="$(read_prompt "  $key [$display_val]: ")"
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

  echo ""
  info "Configuration saved to $env_file"
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

  local http_port
  http_port="$(grep -E '^EUOSINT_HTTP_PORT=' "$INSTALL_DIR/.env" | cut -d= -f2 || echo "8080")"
  info "EUOSINT started. HTTP endpoint: http://$(hostname -f 2>/dev/null || hostname):${http_port}"
}

main() {
  ensure_docker
  INSTALL_MODE="$(prompt_install_mode)"
  info "Selected install mode: ${INSTALL_MODE}"

  if [[ "$INSTALL_MODE" == "install" ]]; then
    require_cmd git
    clone_or_update_repo
  else
    if [[ ! -d "$INSTALL_DIR" || ! -f "$INSTALL_DIR/docker-compose.yml" ]]; then
      fatal "Update mode requires an existing install at $INSTALL_DIR (missing docker-compose.yml). Use install mode first."
    fi
    refresh_compose_yaml_direct
    refresh_env_example_direct
    refresh_web_runtime_files_direct
  fi

  configure_env
  start_stack
}

main "$@"
