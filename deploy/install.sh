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

prompt() {
  local label="$1"
  local default_value="${2:-}"
  local value
  if [[ -n "${default_value}" ]]; then
    value="$(read_prompt "$label [$default_value]: ")"
    echo "${value:-$default_value}"
  else
    value="$(read_prompt "$label: ")"
    echo "$value"
  fi
}

prompt_yes_no() {
  local label="$1"
  local default_value="$2"
  local value
  while true; do
    value="$(read_prompt "$label [$default_value]: ")"
    value="${value:-$default_value}"
    value="$(echo "$value" | tr '[:upper:]' '[:lower:]')"
    case "$value" in
      y|yes) echo "yes"; return 0 ;;
      n|no) echo "no"; return 0 ;;
      *) echo "Please answer yes or no." ;;
    esac
  done
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
    cp "$env_file" "$env_file.preinstall.$(date +%Y%m%d%H%M%S).bak"
    info "Existing .env backed up."
  else
    cp "$example_file" "$env_file"
    info "Created .env from .env.example."
  fi

  local image_tag
  image_tag="$(prompt "GHCR image tag to deploy" "$IMAGE_TAG")"
  upsert_env "$env_file" "EUOSINT_WEB_IMAGE" "ghcr.io/scalytics/euosint-web:${image_tag}"
  upsert_env "$env_file" "EUOSINT_COLLECTOR_IMAGE" "ghcr.io/scalytics/euosint-collector:${image_tag}"
  info "Configured GHCR images with tag '${image_tag}'."

  local domain
  domain="$(prompt "Domain for public access (blank for localhost dev mode)" "")"

  if [[ -n "$domain" ]]; then
    TLS_MODE="true"
    upsert_env "$env_file" "EUOSINT_SITE_ADDRESS" "$domain"
    upsert_env "$env_file" "EUOSINT_HTTP_PORT" "80"
    upsert_env "$env_file" "EUOSINT_HTTPS_PORT" "443"
    info "Configured domain '$domain' with ports 80/443."
    warn "Ensure DNS A/AAAA records point to this host and inbound 80/443 are open."
  else
    TLS_MODE="false"
    upsert_env "$env_file" "EUOSINT_SITE_ADDRESS" ":80"
    upsert_env "$env_file" "EUOSINT_HTTP_PORT" "8080"
    upsert_env "$env_file" "EUOSINT_HTTPS_PORT" "8443"
    info "Configured localhost mode on 8080/8443."
  fi

  local browser_choice
  browser_choice="$(prompt_yes_no "Enable browser-assisted fetches (higher accuracy, higher resource use)?" "yes")"
  if [[ "$browser_choice" == "yes" ]]; then
    upsert_env "$env_file" "BROWSER_ENABLED" "true"
  else
    upsert_env "$env_file" "BROWSER_ENABLED" "false"
  fi

  local vetting_choice
  vetting_choice="$(prompt_yes_no "Enable LLM source vetting?" "no")"
  if [[ "$vetting_choice" == "yes" ]]; then
    local provider base_url model api_key
    provider="$(prompt "Vetting provider label (openai/xai/mistral/...)" "xai")"
    base_url="$(prompt "Vetting base URL" "https://api.x.ai/v1")"
    model="$(prompt "Vetting model" "grok-4-1-fast")"
    api_key="$(prompt "Vetting API key (required)" "")"
    [[ -n "$api_key" ]] || fatal "Vetting enabled but API key is empty."

    upsert_env "$env_file" "SOURCE_VETTING_ENABLED" "true"
    upsert_env "$env_file" "SOURCE_VETTING_PROVIDER" "$provider"
    upsert_env "$env_file" "SOURCE_VETTING_BASE_URL" "$base_url"
    upsert_env "$env_file" "SOURCE_VETTING_MODEL" "$model"
    upsert_env "$env_file" "SOURCE_VETTING_API_KEY" "$api_key"
  else
    upsert_env "$env_file" "SOURCE_VETTING_ENABLED" "false"
    upsert_env "$env_file" "SOURCE_VETTING_API_KEY" ""
  fi

  local alert_llm_choice
  alert_llm_choice="$(prompt_yes_no "Enable LLM alert translation/classification?" "no")"
  if [[ "$alert_llm_choice" == "yes" ]]; then
    upsert_env "$env_file" "ALERT_LLM_ENABLED" "true"
  else
    upsert_env "$env_file" "ALERT_LLM_ENABLED" "false"
  fi

  configure_acled "$env_file"
}

configure_acled() {
  local env_file="$1"
  local acled_choice
  acled_choice="$(prompt_yes_no "Enable ACLED conflict data (battles, military movements, protests)?" "no")"
  if [[ "$acled_choice" == "yes" ]]; then
    local username password
    info "Note: API access requires an institutional email. Gmail/public email = web export only."
    username="$(prompt "ACLED username (institutional email)" "")"
    password="$(prompt "ACLED password" "")"
    if [[ -z "$username" || -z "$password" ]]; then
      warn "ACLED credentials empty — skipping. Register at https://acleddata.com/register/"
      return 0
    fi
    upsert_env "$env_file" "ACLED_USERNAME" "$username"
    upsert_env "$env_file" "ACLED_PASSWORD" "$password"
    info "ACLED conflict data enabled."
  fi
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
  firewall_choice="$(prompt_yes_no "Run firewall checks for 80/443 (ufw/firewalld)?" "yes")"
  if [[ "$firewall_choice" == "yes" ]]; then
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
  if [[ "$INSTALL_MODE" == "update" ]]; then
    info "Update mode selected: leaving repository/.env/volumes untouched; pulling and restarting images only."
    (
      cd "$INSTALL_DIR"
      $COMPOSE_CMD pull
      $COMPOSE_CMD up -d --no-build
    )
    local http_port
    http_port="$(grep -E '^EUOSINT_HTTP_PORT=' "$INSTALL_DIR/.env" | cut -d= -f2 || echo "8080")"
    info "EUOSINT updated. HTTP endpoint: http://$(hostname -f 2>/dev/null || hostname):${http_port}"
    return 0
  fi

  local start_choice
  start_choice="$(prompt_yes_no "Start EUOSINT now with Docker Compose?" "yes")"
  if [[ "$start_choice" != "yes" ]]; then
    if [[ "$INSTALL_MODE" == "install" ]]; then
      info "Installation complete. Start later with: cd $INSTALL_DIR && $COMPOSE_CMD down --remove-orphans && $COMPOSE_CMD pull && $COMPOSE_CMD up -d --no-build"
    else
      info "Installation complete. Start later with: cd $INSTALL_DIR && $COMPOSE_CMD pull && $COMPOSE_CMD up -d --no-build"
    fi
    return 0
  fi

  preflight_tls_checks

  if [[ "$INSTALL_MODE" == "install" ]]; then
    info "Install mode selected: stopping stack (preserving feed-data volume)."
    (
      cd "$INSTALL_DIR"
      $COMPOSE_CMD down --remove-orphans || true
      # Only remove ephemeral Caddy caches, never feed-data which holds the DB.
      docker volume rm "${COMPOSE_PROJECT_NAME:-euosint}_caddy-data" 2>/dev/null || true
      docker volume rm "${COMPOSE_PROJECT_NAME:-euosint}_caddy-config" 2>/dev/null || true
    )
  else
    info "Update mode selected: keeping existing Docker volumes/data."
  fi

  info "Pulling latest GHCR images..."
  (
    cd "$INSTALL_DIR"
    $COMPOSE_CMD pull
  )

  info "Starting stack without local builds..."
  (
    cd "$INSTALL_DIR"
    $COMPOSE_CMD up -d --no-build
  )

  local http_port
  http_port="$(grep -E '^EUOSINT_HTTP_PORT=' "$INSTALL_DIR/.env" | cut -d= -f2 || echo "8080")"
  info "EUOSINT started. HTTP endpoint: http://$(hostname -f 2>/dev/null || hostname):${http_port}"
}

check_new_env_vars() {
  local env_file="$INSTALL_DIR/.env"
  local example_file="$INSTALL_DIR/.env.example"
  [[ -f "$example_file" && -f "$env_file" ]] || return 0

  # Collect keys present in .env.example but missing from .env.
  local new_keys=()
  while IFS= read -r line; do
    # Skip comments and empty lines.
    [[ "$line" =~ ^[[:space:]]*# ]] && continue
    [[ "$line" =~ ^[[:space:]]*$ ]] && continue
    local key="${line%%=*}"
    [[ -z "$key" ]] && continue
    if ! grep -qE "^${key}=" "$env_file" 2>/dev/null; then
      new_keys+=("$key")
    fi
  done < "$example_file"

  [[ ${#new_keys[@]} -eq 0 ]] && return 0

  info "New configuration options detected since last install:"
  for key in "${new_keys[@]}"; do
    echo "  - $key"
  done

  # Handle known feature blocks interactively.
  for key in "${new_keys[@]}"; do
    case "$key" in
      ACLED_USERNAME)
        configure_acled "$env_file"
        ;;
      ACLED_PASSWORD)
        # Handled together with ACLED_USERNAME above.
        ;;
      *)
        # For unknown new vars, copy the default from .env.example.
        local default_val
        default_val="$(grep -E "^${key}=" "$example_file" | head -1 | cut -d= -f2-)"
        upsert_env "$env_file" "$key" "$default_val"
        info "Added $key with default value."
        ;;
    esac
  done
}

main() {
  require_cmd git
  ensure_docker
  INSTALL_MODE="$(prompt_install_mode)"
  info "Selected install mode: ${INSTALL_MODE}"
  if [[ "$INSTALL_MODE" == "install" ]]; then
    clone_or_update_repo
    configure_env
  else
    if [[ ! -d "$INSTALL_DIR" || ! -f "$INSTALL_DIR/docker-compose.yml" ]]; then
      fatal "Update mode requires an existing install at $INSTALL_DIR (missing docker-compose.yml). Use install mode first."
    fi
    clone_or_update_repo
    check_new_env_vars
  fi
  start_stack
}

main "$@"
