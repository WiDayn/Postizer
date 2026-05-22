#!/usr/bin/env bash
set -Eeuo pipefail

APP_NAME="postizer"
ORIGINAL_ARGS=("$@")
REPO_URL="${POSTIZER_REPO_URL:-https://github.com/WiDayn/Postizer.git}"
RELEASE_VERSION_REQUESTED="${POSTIZER_RELEASE_VERSION:-latest}"
SERVICE_NAME="${POSTIZER_SERVICE_NAME:-postizer}"
SERVICE_USER="${POSTIZER_SERVICE_USER:-postizer}"
SERVICE_GROUP="${POSTIZER_SERVICE_GROUP:-$SERVICE_USER}"
INSTALL_DIR="${POSTIZER_INSTALL_DIR:-/opt/postizer}"
DEFAULT_SOURCE_DIR="${POSTIZER_SOURCE_DIR:-/usr/local/src/postizer}"
UPDATE_SERVICE_NAME_ENV="${POSTIZER_UPDATE_SERVICE_NAME:-}"
UPDATE_TIMER_NAME_ENV="${POSTIZER_UPDATE_TIMER_NAME:-}"
UPDATE_SERVICE_NAME="${UPDATE_SERVICE_NAME_ENV:-$SERVICE_NAME-update}"
UPDATE_TIMER_NAME="${UPDATE_TIMER_NAME_ENV:-$UPDATE_SERVICE_NAME}"
UPDATE_TIMER_INTERVAL="${POSTIZER_UPDATE_INTERVAL:-15min}"
BIN_LINK="${POSTIZER_BIN_LINK:-/usr/local/bin/postizer}"
ENV_DIR="${POSTIZER_ENV_DIR:-/etc/postizer}"
ENV_FILE="${POSTIZER_ENV_FILE:-$ENV_DIR/postizer.env}"
LISTEN_ADDR="${POSTIZER_ADDR:-:8080}"
GO_CACHE_ROOT="${POSTIZER_GO_CACHE_ROOT:-}"
INSTALL_DIR_EXPLICIT=0
SOURCE_DIR_EXPLICIT=0
UPDATE_SERVICE_NAME_EXPLICIT=0
UPDATE_TIMER_NAME_EXPLICIT=0
BIN_LINK_EXPLICIT=0
ENV_DIR_EXPLICIT=0
ENV_FILE_EXPLICIT=0
GO_CACHE_ROOT_EXPLICIT=0
START_SERVICE=1
ENABLE_SERVICE=1
INSTALL_FROM_RELEASE=1
BUILD_BINARY=0
SKIP_DEPS=0
GIT_PULL=1
GIT_UPDATED=0
INSTALL_UPDATE_TIMER=1
AUTO_UPDATE_RUN=0
NO_BUILD_BINARY=0
SCRIPT_SELF="${BASH_SOURCE[0]:-$0}"
SCRIPT_DIR=""
if [[ -n "$SCRIPT_SELF" && -f "$SCRIPT_SELF" ]]; then
  SCRIPT_DIR="$(cd "$(dirname "$SCRIPT_SELF")" && pwd)"
fi
if [[ -n "${POSTIZER_SOURCE_DIR:-}" ]]; then
  SOURCE_DIR="$POSTIZER_SOURCE_DIR"
elif [[ -n "$SCRIPT_DIR" && -f "$SCRIPT_DIR/../go.mod" && -d "$SCRIPT_DIR/../cmd/postizer" ]]; then
  SOURCE_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
elif [[ -f "$PWD/go.mod" && -d "$PWD/cmd/postizer" ]]; then
  SOURCE_DIR="$PWD"
else
  SOURCE_DIR="$DEFAULT_SOURCE_DIR"
fi
BINARY_SOURCE="${POSTIZER_BINARY:-}"
UPDATE_TARGET_VERSION=""

json_escape() {
  local value="${1:-}"
  value="${value//\\/\\\\}"
  value="${value//\"/\\\"}"
  value="${value//$'\n'/ }"
  value="${value//$'\r'/ }"
  printf '%s' "$value"
}

record_update_event() {
  if [[ "${AUTO_UPDATE_RUN:-0}" -ne 1 ]]; then
    return 0
  fi
  local event="${1:-}" version="${2:-}" message="${3:-}" log_file timestamp
  if [[ -z "$event" ]]; then
    return 0
  fi
  if [[ "$event" == "update_detected" && -n "$version" ]]; then
    UPDATE_TARGET_VERSION="$version"
  fi
  log_file="$INSTALL_DIR/content/.update_log.jsonl"
  timestamp="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
  mkdir -p "$(dirname "$log_file")" 2>/dev/null || return 0
  printf '{"time":"%s","event":"%s","version":"%s","message":"%s"}\n' \
    "$timestamp" "$(json_escape "$event")" "$(json_escape "$version")" "$(json_escape "$message")" >> "$log_file" 2>/dev/null || true
}
GO_CMD="${POSTIZER_GO:-go}"
REQUIRED_GO_VERSION="${POSTIZER_GO_VERSION:-}"
RELEASE_EXTRACT_DIR=""
RESOLVED_RELEASE_VERSION=""

[[ -n "${POSTIZER_INSTALL_DIR:-}" ]] && INSTALL_DIR_EXPLICIT=1
[[ -n "${POSTIZER_SOURCE_DIR:-}" ]] && SOURCE_DIR_EXPLICIT=1
[[ -n "${POSTIZER_UPDATE_SERVICE_NAME:-}" ]] && UPDATE_SERVICE_NAME_EXPLICIT=1
[[ -n "${POSTIZER_UPDATE_TIMER_NAME:-}" ]] && UPDATE_TIMER_NAME_EXPLICIT=1
[[ -n "${POSTIZER_BIN_LINK:-}" ]] && BIN_LINK_EXPLICIT=1
[[ -n "${POSTIZER_ENV_DIR:-}" ]] && ENV_DIR_EXPLICIT=1
[[ -n "${POSTIZER_ENV_FILE:-}" ]] && ENV_FILE_EXPLICIT=1
[[ -n "${POSTIZER_GO_CACHE_ROOT:-}" ]] && GO_CACHE_ROOT_EXPLICIT=1

usage() {
  cat <<EOF
Usage: $0 [options]

Install Postizer into a Linux systemd service.

Options:
  --install-dir PATH    Runtime directory (default: $INSTALL_DIR)
  --release-version V   GitHub release version to install (default: $RELEASE_VERSION_REQUESTED)
  --repo-url URL        GitHub repository for release assets (default: $REPO_URL)
  --source-dir PATH     Source checkout directory, only used with --build-from-source
  --update-interval N   Auto-update systemd timer interval (default: $UPDATE_TIMER_INTERVAL)
  --service-name NAME   systemd service name (default: $SERVICE_NAME)
  --update-service-name NAME
                        systemd oneshot update service name (default: <service>-update)
  --update-timer-name NAME, --timer-name NAME
                        systemd update timer name (default: <update-service>)
  --user NAME           Service user (default: $SERVICE_USER)
  --group NAME          Service group (default: same as user)
  --addr ADDR           POSTIZER_ADDR value (default: $LISTEN_ADDR)
  --port PORT           Listen on :PORT (shorthand for --addr :PORT)
  --bin-link PATH       Symlink path for the installed binary (default: /usr/local/bin/<service>)
  --env-dir PATH        Environment file directory (default: /etc/<service>)
  --env-file PATH       Environment file path (default: <env-dir>/<service>.env)
  --go-cache-dir PATH   Go build/module cache root (default: /var/cache/<service>/go)
  --binary PATH         Install an existing binary instead of building
  --no-build            Use ./postizer from the repository root
  --build-from-source   Build from the Git source checkout instead of using a release asset
  --no-git-pull         Do not update an existing Git checkout before building
  --skip-deps           Do not install missing OS packages or Go
  --no-update-timer     Do not register the auto-update systemd timer
  --no-enable           Do not enable the service at boot
  --no-start            Do not start/restart the service
  -h, --help            Show this help

Environment:
  POSTIZER_ADMIN_USER, POSTIZER_ADMIN_PASSWORD, and POSTIZER_SESSION_SECRET
  are written into $ENV_FILE on first install. If no admin password is set,
  this script generates one and prints it once.
EOF
}

require_arg() {
  local option="$1"
  local value="${2:-}"
  if [[ -z "$value" ]]; then
    echo "$option requires a value" >&2
    usage >&2
    exit 2
  fi
}

listen_addr_from_port() {
  local port="$1"
  if [[ ! "$port" =~ ^[0-9]+$ ]] || ((port < 1 || port > 65535)); then
    echo "invalid port: $port" >&2
    exit 2
  fi
  printf ':%s' "$port"
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --install-dir)
      require_arg "$1" "${2:-}"
      INSTALL_DIR="$2"
      INSTALL_DIR_EXPLICIT=1
      shift 2
      ;;
    --source-dir)
      require_arg "$1" "${2:-}"
      SOURCE_DIR="$2"
      SOURCE_DIR_EXPLICIT=1
      shift 2
      ;;
    --repo-url)
      require_arg "$1" "${2:-}"
      REPO_URL="$2"
      shift 2
      ;;
    --release-version)
      require_arg "$1" "${2:-}"
      RELEASE_VERSION_REQUESTED="$2"
      shift 2
      ;;
    --update-interval)
      require_arg "$1" "${2:-}"
      UPDATE_TIMER_INTERVAL="$2"
      shift 2
      ;;
    --service-name)
      require_arg "$1" "${2:-}"
      SERVICE_NAME="$2"
      shift 2
      ;;
    --update-service-name)
      require_arg "$1" "${2:-}"
      UPDATE_SERVICE_NAME="$2"
      UPDATE_SERVICE_NAME_EXPLICIT=1
      shift 2
      ;;
    --update-timer-name|--timer-name)
      require_arg "$1" "${2:-}"
      UPDATE_TIMER_NAME="$2"
      UPDATE_TIMER_NAME_EXPLICIT=1
      shift 2
      ;;
    --user)
      require_arg "$1" "${2:-}"
      SERVICE_USER="$2"
      shift 2
      ;;
    --group)
      require_arg "$1" "${2:-}"
      SERVICE_GROUP="$2"
      shift 2
      ;;
    --addr)
      require_arg "$1" "${2:-}"
      LISTEN_ADDR="$2"
      shift 2
      ;;
    --port)
      require_arg "$1" "${2:-}"
      LISTEN_ADDR="$(listen_addr_from_port "$2")"
      shift 2
      ;;
    --bin-link)
      require_arg "$1" "${2:-}"
      BIN_LINK="$2"
      BIN_LINK_EXPLICIT=1
      shift 2
      ;;
    --env-dir)
      require_arg "$1" "${2:-}"
      ENV_DIR="$2"
      ENV_DIR_EXPLICIT=1
      shift 2
      ;;
    --env-file)
      require_arg "$1" "${2:-}"
      ENV_FILE="$2"
      ENV_FILE_EXPLICIT=1
      shift 2
      ;;
    --go-cache-dir)
      require_arg "$1" "${2:-}"
      GO_CACHE_ROOT="$2"
      GO_CACHE_ROOT_EXPLICIT=1
      shift 2
      ;;
    --binary)
      require_arg "$1" "${2:-}"
      BINARY_SOURCE="$2"
      INSTALL_FROM_RELEASE=0
      BUILD_BINARY=0
      shift 2
      ;;
    --no-build)
      NO_BUILD_BINARY=1
      INSTALL_FROM_RELEASE=0
      BUILD_BINARY=0
      shift
      ;;
    --build-from-source)
      INSTALL_FROM_RELEASE=0
      BUILD_BINARY=1
      shift
      ;;
    --no-git-pull|--skip-git-pull)
      GIT_PULL=0
      shift
      ;;
    --skip-deps)
      SKIP_DEPS=1
      shift
      ;;
    --no-update-timer)
      INSTALL_UPDATE_TIMER=0
      shift
      ;;
    --auto-update-run)
      AUTO_UPDATE_RUN=1
      INSTALL_UPDATE_TIMER=0
      shift
      ;;
    --no-enable)
      ENABLE_SERVICE=0
      shift
      ;;
    --no-start)
      START_SERVICE=0
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

SERVICE_NAME="${SERVICE_NAME%.service}"
UPDATE_SERVICE_NAME="${UPDATE_SERVICE_NAME%.service}"
UPDATE_TIMER_NAME="${UPDATE_TIMER_NAME%.timer}"

if [[ "$INSTALL_DIR_EXPLICIT" -eq 0 ]]; then
  INSTALL_DIR="/opt/$SERVICE_NAME"
fi
if [[ "$SOURCE_DIR_EXPLICIT" -eq 0 && "$SOURCE_DIR" == "$DEFAULT_SOURCE_DIR" ]]; then
  SOURCE_DIR="/usr/local/src/$SERVICE_NAME"
fi
if [[ "$BIN_LINK_EXPLICIT" -eq 0 ]]; then
  BIN_LINK="/usr/local/bin/$SERVICE_NAME"
fi
if [[ "$ENV_DIR_EXPLICIT" -eq 0 ]]; then
  ENV_DIR="/etc/$SERVICE_NAME"
fi
if [[ "$ENV_FILE_EXPLICIT" -eq 0 ]]; then
  ENV_FILE="$ENV_DIR/$SERVICE_NAME.env"
fi
if [[ "$GO_CACHE_ROOT_EXPLICIT" -eq 0 ]]; then
  GO_CACHE_ROOT="/var/cache/$SERVICE_NAME/go"
fi
if [[ "$UPDATE_SERVICE_NAME_EXPLICIT" -eq 0 ]]; then
  UPDATE_SERVICE_NAME="$SERVICE_NAME-update"
fi
if [[ "$UPDATE_TIMER_NAME_EXPLICIT" -eq 0 ]]; then
  UPDATE_TIMER_NAME="$UPDATE_SERVICE_NAME"
fi

if [[ "$NO_BUILD_BINARY" -eq 1 && -z "$BINARY_SOURCE" ]]; then
  BINARY_SOURCE="$SOURCE_DIR/$APP_NAME"
fi

need_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Missing required command: $1" >&2
    exit 1
  fi
}

optional_command_exists() {
  command -v "$1" >/dev/null 2>&1
}

source_tree_available() {
  [[ -f "$SOURCE_DIR/go.mod" && -d "$SOURCE_DIR/cmd/postizer" && -d "$SOURCE_DIR/web" && -d "$SOURCE_DIR/internal/bundles" ]]
}

directory_empty() {
  if [[ ! -d "$1" ]]; then
    return 0
  fi
  local entries
  shopt -s nullglob dotglob
  entries=("$1"/*)
  shopt -u nullglob dotglob
  ((${#entries[@]} == 0))
}

source_dir_owner() {
  if optional_command_exists stat; then
    stat -c '%U' "$SOURCE_DIR" 2>/dev/null || true
  fi
}

run_source_git() {
  local owner
  owner="$(source_dir_owner)"
  if [[ "${EUID}" -eq 0 && -n "$owner" && "$owner" != "root" && "$owner" != "UNKNOWN" ]]; then
    if optional_command_exists sudo; then
      sudo -H -u "$owner" git -C "$SOURCE_DIR" "$@"
      return
    fi
    if optional_command_exists runuser; then
      runuser -u "$owner" -- git -C "$SOURCE_DIR" "$@"
      return
    fi
  fi
  git -C "$SOURCE_DIR" "$@"
}

clone_source_tree() {
  if ! optional_command_exists git; then
    echo "Missing git for cloning $REPO_URL. Install git or rerun without --skip-deps." >&2
    exit 1
  fi
  if [[ -e "$SOURCE_DIR" ]] && ! directory_empty "$SOURCE_DIR"; then
    echo "Source directory exists but is not a usable Postizer checkout: $SOURCE_DIR" >&2
    echo "Set POSTIZER_SOURCE_DIR or --source-dir to another path, or clean that directory first." >&2
    exit 1
  fi

  mkdir -p "$(dirname "$SOURCE_DIR")"
  echo "Cloning Postizer from $REPO_URL into $SOURCE_DIR..."
  git clone "$REPO_URL" "$SOURCE_DIR"
  GIT_UPDATED=1
}

update_source_tree() {
  if [[ "$GIT_PULL" -ne 1 || "${POSTIZER_GIT_PULL_DONE:-0}" -eq 1 ]]; then
    if ! source_tree_available; then
      clone_source_tree
    fi
    return
  fi
  if [[ ! -e "$SOURCE_DIR/.git" ]]; then
    if source_tree_available; then
      echo "Using source tree without git pull: $SOURCE_DIR is not a Git checkout."
      export POSTIZER_GIT_PULL_DONE=1
      return
    fi
    clone_source_tree
    export POSTIZER_GIT_PULL_DONE=1
    return
  fi
  if ! optional_command_exists git; then
    if [[ "$SKIP_DEPS" -eq 1 ]]; then
      echo "Missing git for updating the source tree. Install git or rerun with --no-git-pull." >&2
      exit 1
    fi
    echo "Git is not installed yet; dependencies will be installed first."
    return
  fi
  if ! run_source_git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    echo "Skipping git pull: $SOURCE_DIR is not a Git work tree."
    export POSTIZER_GIT_PULL_DONE=1
    return
  fi
  if ! run_source_git remote get-url origin >/dev/null 2>&1; then
    echo "Skipping git pull: no origin remote is configured."
    export POSTIZER_GIT_PULL_DONE=1
    return
  fi

  local before after
  before="$(run_source_git rev-parse HEAD 2>/dev/null || true)"
  echo "Updating source tree with git pull --ff-only..."
  if ! run_source_git pull --ff-only; then
    echo "git pull failed. Resolve local changes or rerun with --no-git-pull." >&2
    exit 1
  fi
  after="$(run_source_git rev-parse HEAD 2>/dev/null || true)"
  if [[ -n "$before" && -n "$after" && "$before" != "$after" ]]; then
    GIT_UPDATED=1
  fi
  export POSTIZER_GIT_PULL_DONE=1
}

is_release_tag() {
  [[ "$1" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]
}

release_tag_version() {
  local tag="$1"
  printf '%s' "${tag#v}"
}

version_gt() {
  local have="$1"
  local want="$2"
  [[ "$have" != "$want" ]] && version_ge "$have" "$want"
}

latest_release_tag() {
  local latest="" tag
  while IFS= read -r tag; do
    if ! is_release_tag "$tag"; then
      continue
    fi
    if [[ -z "$latest" ]] || version_gt "$(release_tag_version "$tag")" "$(release_tag_version "$latest")"; then
      latest="$tag"
    fi
  done < <(run_source_git tag --list 'v[0-9]*.[0-9]*.[0-9]*')
  printf '%s' "$latest"
}

current_release_tag() {
  local current="" tag
  while IFS= read -r tag; do
    if ! is_release_tag "$tag"; then
      continue
    fi
    if [[ -z "$current" ]] || version_gt "$(release_tag_version "$tag")" "$(release_tag_version "$current")"; then
      current="$tag"
    fi
  done < <(run_source_git tag --points-at HEAD)
  printf '%s' "$current"
}

source_build_version() {
  local exact_tag short_commit dirty_suffix
  if [[ -e "$SOURCE_DIR/.git" ]] && optional_command_exists git && run_source_git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    exact_tag="$(run_source_git describe --tags --exact-match --match 'v[0-9]*.[0-9]*.[0-9]*' HEAD 2>/dev/null || true)"
    if [[ -n "$exact_tag" ]] && is_release_tag "$exact_tag"; then
      printf '%s' "$exact_tag"
      return
    fi
    short_commit="$(run_source_git rev-parse --short HEAD 2>/dev/null || true)"
    if [[ -n "$short_commit" ]]; then
      dirty_suffix=""
      if [[ -n "$(run_source_git status --porcelain 2>/dev/null || true)" ]]; then
        dirty_suffix="-dirty"
      fi
      printf 'dev-%s%s' "$short_commit" "$dirty_suffix"
      return
    fi
  fi
  printf 'dev'
}

update_source_tree_to_latest_release_tag() {
  if [[ "$GIT_PULL" -ne 1 || "${POSTIZER_GIT_PULL_DONE:-0}" -eq 1 ]]; then
    if ! source_tree_available; then
      clone_source_tree
    fi
    return
  fi
  if [[ ! -e "$SOURCE_DIR/.git" ]]; then
    if source_tree_available; then
      echo "Skipping release tag update: $SOURCE_DIR is not a Git checkout."
      export POSTIZER_GIT_PULL_DONE=1
      return
    fi
    clone_source_tree
  fi
  if ! optional_command_exists git; then
    echo "Missing git for checking release tags. Install git or rerun with --no-git-pull." >&2
    exit 1
  fi
  if ! run_source_git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    echo "Skipping release tag update: $SOURCE_DIR is not a Git work tree."
    export POSTIZER_GIT_PULL_DONE=1
    return
  fi
  if ! run_source_git remote get-url origin >/dev/null 2>&1; then
    echo "Skipping release tag update: no origin remote is configured."
    export POSTIZER_GIT_PULL_DONE=1
    return
  fi

  local before after current latest target_commit
  before="$(run_source_git rev-parse HEAD 2>/dev/null || true)"
  echo "Checking for release tags matching vX.X.X..."
  run_source_git fetch --tags origin

  latest="$(latest_release_tag)"
  if [[ -z "$latest" ]]; then
    echo "No release tags matching vX.X.X were found."
    GIT_UPDATED=0
    export POSTIZER_GIT_PULL_DONE=1
    return
  fi

  current="$(current_release_tag)"
  if [[ "$current" == "$latest" ]]; then
    echo "Already running release tag $latest."
    GIT_UPDATED=0
    export POSTIZER_GIT_PULL_DONE=1
    return
  fi
  if [[ -n "$current" ]] && ! version_gt "$(release_tag_version "$latest")" "$(release_tag_version "$current")"; then
    echo "No newer release tag found. Current: $current, latest: $latest."
    GIT_UPDATED=0
    export POSTIZER_GIT_PULL_DONE=1
    return
  fi

  target_commit="$(run_source_git rev-list -n 1 "$latest" 2>/dev/null || true)"
  if [[ -z "$target_commit" ]]; then
    echo "Could not resolve release tag $latest." >&2
    exit 1
  fi
  if [[ -z "$current" ]] && ! run_source_git merge-base --is-ancestor HEAD "$target_commit"; then
    echo "Latest release tag $latest is not ahead of current HEAD; skipping to avoid downgrading."
    GIT_UPDATED=0
    export POSTIZER_GIT_PULL_DONE=1
    return
  fi
  if [[ -n "$(run_source_git status --porcelain)" ]]; then
    echo "Source tree has local changes; refusing to checkout release tag $latest." >&2
    exit 1
  fi

  record_update_event "update_detected" "$latest" "Detected a newer release tag in the local source checkout."
  echo "Updating source tree to release tag $latest..."
  if ! run_source_git checkout --detach "$latest"; then
    echo "git checkout $latest failed. Resolve local changes or rerun with --no-git-pull." >&2
    exit 1
  fi
  after="$(run_source_git rev-parse HEAD 2>/dev/null || true)"
  if [[ -n "$before" && -n "$after" && "$before" != "$after" ]]; then
    GIT_UPDATED=1
  fi
  export POSTIZER_GIT_PULL_DONE=1
}

load_go_requirement() {
  if [[ -n "$REQUIRED_GO_VERSION" ]]; then
    return
  fi
  if [[ ! -f "$SOURCE_DIR/go.mod" ]]; then
    echo "Cannot find go.mod in source directory: $SOURCE_DIR" >&2
    exit 1
  fi
  REQUIRED_GO_VERSION="$(awk '/^go[[:space:]]+/ { print $2; exit }' "$SOURCE_DIR/go.mod")"
  if [[ -z "$REQUIRED_GO_VERSION" ]]; then
    echo "Cannot detect required Go version from $SOURCE_DIR/go.mod" >&2
    exit 1
  fi
}

auto_update_enabled() {
  local settings_file="$INSTALL_DIR/content/settings.json"
  if [[ ! -f "$settings_file" ]]; then
    return 1
  fi
  sed -n '/"auto_update"[[:space:]]*:/,/}/p' "$settings_file" | grep -Eq '"enabled"[[:space:]]*:[[:space:]]*true'
}

validate_paths() {
  validate_unit_name "service" "$SERVICE_NAME"
  validate_unit_name "update service" "$UPDATE_SERVICE_NAME"
  validate_unit_name "update timer" "$UPDATE_TIMER_NAME"
  if [[ -z "$INSTALL_DIR" || "$INSTALL_DIR" == "/" ]]; then
    echo "Refusing unsafe install directory: $INSTALL_DIR" >&2
    exit 1
  fi
  if [[ -z "$ENV_FILE" || "$ENV_FILE" == "/" ]]; then
    echo "Refusing unsafe environment file: $ENV_FILE" >&2
    exit 1
  fi
  if [[ -z "$BIN_LINK" || "$BIN_LINK" == "/" ]]; then
    echo "Refusing unsafe binary link: $BIN_LINK" >&2
    exit 1
  fi
  if [[ "$BUILD_BINARY" -eq 1 && ( -z "$GO_CACHE_ROOT" || "$GO_CACHE_ROOT" == "/" ) ]]; then
    echo "Refusing unsafe Go cache directory: $GO_CACHE_ROOT" >&2
    exit 1
  fi
}

validate_unit_name() {
  local label="$1"
  local value="$2"
  if [[ -z "$value" || "$value" == *"/"* ]]; then
    echo "Invalid $label name: $value" >&2
    exit 1
  fi
}

version_ge() {
  local have="$1"
  local want="$2"
  local have_major have_minor have_patch want_major want_minor want_patch
  IFS=. read -r have_major have_minor have_patch <<<"$have"
  IFS=. read -r want_major want_minor want_patch <<<"$want"
  have_major="${have_major:-0}"
  have_minor="${have_minor:-0}"
  have_patch="${have_patch:-0}"
  want_major="${want_major:-0}"
  want_minor="${want_minor:-0}"
  want_patch="${want_patch:-0}"

  if ((have_major != want_major)); then
    ((have_major > want_major))
    return
  fi
  if ((have_minor != want_minor)); then
    ((have_minor > want_minor))
    return
  fi
  ((have_patch >= want_patch))
}

go_version() {
  local output
  if ! output="$("$1" version 2>/dev/null)"; then
    return 1
  fi
  sed -nE 's/.* go([0-9]+(\.[0-9]+){1,2}).*/\1/p' <<<"$output"
}

go_satisfies_requirement() {
  local cmd="$1"
  if ! optional_command_exists "$cmd"; then
    return 1
  fi
  local have
  have="$(go_version "$cmd")"
  if [[ -z "$have" || -z "$REQUIRED_GO_VERSION" ]]; then
    return 1
  fi
  version_ge "$have" "$REQUIRED_GO_VERSION"
}

install_os_packages() {
  if [[ "$SKIP_DEPS" -eq 1 ]]; then
    return
  fi

  echo "Installing missing dependencies with the system package manager..."
  if optional_command_exists apt-get; then
    export DEBIAN_FRONTEND=noninteractive
    apt-get update
    apt-get install -y ca-certificates curl tar gzip coreutils passwd git
  elif optional_command_exists dnf; then
    dnf install -y ca-certificates curl tar gzip coreutils shadow-utils systemd git
  elif optional_command_exists yum; then
    yum install -y ca-certificates curl tar gzip coreutils shadow-utils systemd git
  elif optional_command_exists zypper; then
    zypper --non-interactive install ca-certificates curl tar gzip coreutils shadow systemd git
  elif optional_command_exists pacman; then
    pacman -Sy --noconfirm ca-certificates curl tar gzip coreutils shadow systemd git
  elif optional_command_exists apk; then
    apk add --no-cache ca-certificates curl tar gzip coreutils shadow bash git
  else
    echo "No supported package manager found. Install dependencies manually or rerun with --skip-deps." >&2
    exit 1
  fi
}

needs_dependency_install() {
  local cmd
  for cmd in systemctl cp getent install groupadd useradd; do
    if ! optional_command_exists "$cmd"; then
      return 0
    fi
  done
  if [[ "$INSTALL_FROM_RELEASE" -eq 1 ]]; then
    if ! optional_command_exists curl && ! optional_command_exists wget; then
      return 0
    fi
    for cmd in tar gzip sha256sum; do
      if ! optional_command_exists "$cmd"; then
        return 0
      fi
    done
    return 1
  fi
  if { [[ "$GIT_PULL" -eq 1 && -e "$SOURCE_DIR/.git" ]] || ! source_tree_available; } && ! optional_command_exists git; then
    return 0
  fi
  if [[ "$BUILD_BINARY" -eq 1 ]] && ! go_satisfies_requirement "$GO_CMD"; then
    if ! optional_command_exists curl && ! optional_command_exists wget; then
      return 0
    fi
    for cmd in tar gzip; do
      if ! optional_command_exists "$cmd"; then
        return 0
      fi
    done
  fi
  return 1
}

download_file() {
  local url="$1"
  local output="$2"
  if optional_command_exists curl; then
    if [[ -n "${POSTIZER_GITHUB_TOKEN:-}" ]]; then
      curl -fsSL -H "Authorization: Bearer $POSTIZER_GITHUB_TOKEN" "$url" -o "$output"
    else
      curl -fsSL "$url" -o "$output"
    fi
  elif optional_command_exists wget; then
    wget -qO "$output" "$url"
  else
    echo "Missing curl or wget for downloading files." >&2
    exit 1
  fi
}

github_repo_slug() {
  local value="$REPO_URL"
  value="${value#https://github.com/}"
  value="${value#http://github.com/}"
  value="${value#git@github.com:}"
  value="${value%.git}"
  if [[ "$value" != */* ]]; then
    echo "Repository URL must point to GitHub: $REPO_URL" >&2
    exit 1
  fi
  printf '%s' "$value"
}

release_platform() {
  local arch
  case "$(uname -m)" in
    x86_64|amd64) arch="amd64" ;;
    aarch64|arm64) arch="arm64" ;;
    *)
      echo "Unsupported Linux architecture for Postizer release assets: $(uname -m)" >&2
      exit 1
      ;;
  esac
  printf 'linux-%s' "$arch"
}

latest_github_release_tag() {
  local slug tmp tag
  slug="$(github_repo_slug)"
  tmp="$(mktemp)"
  download_file "https://api.github.com/repos/$slug/releases/latest" "$tmp"
  tag="$(sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$tmp" | head -n 1)"
  rm -f "$tmp"
  if [[ -z "$tag" ]]; then
    echo "Could not detect latest GitHub release tag for $slug." >&2
    exit 1
  fi
  if ! is_release_tag "$tag"; then
    echo "Latest GitHub release tag does not match vX.X.X: $tag" >&2
    exit 1
  fi
  printf '%s' "$tag"
}

resolve_release_version() {
  if [[ -n "$RESOLVED_RELEASE_VERSION" ]]; then
    return
  fi
  if [[ "$RELEASE_VERSION_REQUESTED" == "latest" || -z "$RELEASE_VERSION_REQUESTED" ]]; then
    RESOLVED_RELEASE_VERSION="$(latest_github_release_tag)"
  else
    RESOLVED_RELEASE_VERSION="$RELEASE_VERSION_REQUESTED"
  fi
  if ! is_release_tag "$RESOLVED_RELEASE_VERSION"; then
    echo "Release version must match vX.X.X: $RESOLVED_RELEASE_VERSION" >&2
    exit 1
  fi
}

installed_release_version() {
  if [[ -f "$INSTALL_DIR/.postizer-version" ]]; then
    tr -d '\r\n' < "$INSTALL_DIR/.postizer-version"
  fi
}

release_asset_name() {
  resolve_release_version
  printf 'postizer-%s-%s.tar.gz' "$RESOLVED_RELEASE_VERSION" "$(release_platform)"
}

verify_release_checksum() {
  local sums_file="$1"
  local asset_name="$2"
  local asset_path="$3"
  local checksum actual
  checksum="$(awk -v asset="$asset_name" '$2 == asset || $2 == "*" asset { print $1; exit }' "$sums_file")"
  if [[ -z "$checksum" ]]; then
    echo "SHA256SUMS does not contain $asset_name." >&2
    exit 1
  fi
  actual="$(sha256sum "$asset_path" | awk '{print $1}')"
  if [[ "$checksum" != "$actual" ]]; then
    echo "Checksum mismatch for $asset_name." >&2
    exit 1
  fi
}

download_release_asset() {
  resolve_release_version
  local current slug asset asset_url sums_url tmp archive sums extract_dir child_count child
  current="$(installed_release_version)"
  if [[ "$AUTO_UPDATE_RUN" -eq 1 && "$current" == "$RESOLVED_RELEASE_VERSION" && -x "$INSTALL_DIR/$APP_NAME" ]]; then
    echo "Already running release $RESOLVED_RELEASE_VERSION."
    exit 0
  fi

  record_update_event "update_detected" "$RESOLVED_RELEASE_VERSION" "Detected a newer Postizer release asset."
  slug="$(github_repo_slug)"
  asset="$(release_asset_name)"
  asset_url="https://github.com/$slug/releases/download/$RESOLVED_RELEASE_VERSION/$asset"
  sums_url="https://github.com/$slug/releases/download/$RESOLVED_RELEASE_VERSION/SHA256SUMS"
  tmp="$(mktemp -d)"
  archive="$tmp/$asset"
  sums="$tmp/SHA256SUMS"
  extract_dir="$tmp/extract"

  echo "Downloading Postizer $RESOLVED_RELEASE_VERSION from $asset_url"
  download_file "$asset_url" "$archive"
  echo "Downloading release checksums..."
  download_file "$sums_url" "$sums"
  verify_release_checksum "$sums" "$asset" "$archive"

  mkdir -p "$extract_dir"
  tar -xzf "$archive" -C "$extract_dir"
  if [[ ! -x "$extract_dir/$APP_NAME" ]]; then
    child_count="$(find "$extract_dir" -mindepth 1 -maxdepth 1 -type d | wc -l | tr -d ' ')"
    if [[ "$child_count" == "1" ]]; then
      child="$(find "$extract_dir" -mindepth 1 -maxdepth 1 -type d | head -n 1)"
      extract_dir="$child"
    fi
  fi

  if [[ ! -x "$extract_dir/$APP_NAME" || ! -d "$extract_dir/web" || ! -d "$extract_dir/internal/bundles" ]]; then
    echo "Release asset is missing postizer, web/, or internal/bundles/." >&2
    exit 1
  fi
  printf '%s\n' "$RESOLVED_RELEASE_VERSION" > "$extract_dir/.postizer-version"
  RELEASE_EXTRACT_DIR="$extract_dir"
}

go_download_arch() {
  case "$(uname -m)" in
    x86_64|amd64) echo "amd64" ;;
    aarch64|arm64) echo "arm64" ;;
    armv6l) echo "armv6l" ;;
    i386|i686) echo "386" ;;
    *)
      echo "Unsupported Linux architecture for automatic Go install: $(uname -m)" >&2
      exit 1
      ;;
  esac
}

install_go_toolchain() {
  if [[ "$BUILD_BINARY" -ne 1 ]]; then
    return
  fi
  if go_satisfies_requirement "$GO_CMD"; then
    return
  fi
  if [[ "$SKIP_DEPS" -eq 1 ]]; then
    need_command "$GO_CMD"
    local have
    have="$(go_version "$GO_CMD")"
    echo "Go $REQUIRED_GO_VERSION or newer is required, found ${have:-unknown}." >&2
    exit 1
  fi

  need_command tar
  need_command gzip
  local arch target tmp archive url
  arch="$(go_download_arch)"
  target="/usr/local/go-postizer-$REQUIRED_GO_VERSION"
  if [[ -x "$target/bin/go" ]] && go_satisfies_requirement "$target/bin/go"; then
    GO_CMD="$target/bin/go"
    return
  fi

  tmp="$(mktemp -d)"
  archive="$tmp/go.tar.gz"
  url="https://go.dev/dl/go${REQUIRED_GO_VERSION}.linux-${arch}.tar.gz"
  echo "Installing Go $REQUIRED_GO_VERSION from $url"
  download_file "$url" "$archive"
  tar -C "$tmp" -xzf "$archive"
  rm -rf "$target"
  mv "$tmp/go" "$target"
  rm -rf "$tmp"
  GO_CMD="$target/bin/go"

  if ! optional_command_exists go; then
    mkdir -p /usr/local/bin
    ln -sfn "$GO_CMD" /usr/local/bin/go
  fi
}

prepare_go_build_environment() {
  if [[ "$BUILD_BINARY" -ne 1 ]]; then
    return
  fi
  export GOPATH="${GOPATH:-$GO_CACHE_ROOT/gopath}"
  export GOMODCACHE="${GOMODCACHE:-$GO_CACHE_ROOT/pkg/mod}"
  export GOCACHE="${GOCACHE:-$GO_CACHE_ROOT/build}"
  export HOME="${HOME:-/root}"
  mkdir -p "$GOPATH" "$GOMODCACHE" "$GOCACHE"
}

random_secret() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex "${1:-32}" | tr -d '\n'
    return
  fi
  local value
  set +o pipefail
  value="$(tr -dc 'A-Za-z0-9_-' </dev/urandom | head -c "${1:-48}")"
  set -o pipefail
  printf '%s' "$value"
}

env_quote() {
  local value="$1"
  value="${value//\\/\\\\}"
  value="${value//\"/\\\"}"
  printf '"%s"' "$value"
}

install_runtime_dir() {
  local source_path="$1"
  local target_path="$2"
  rm -rf "$target_path"
  mkdir -p "$(dirname "$target_path")"
  cp -a "$source_path" "$target_path"
}

ensure_service_identity() {
  if ! getent group "$SERVICE_GROUP" >/dev/null 2>&1; then
    groupadd --system "$SERVICE_GROUP"
  fi
  if ! id -u "$SERVICE_USER" >/dev/null 2>&1; then
    useradd --system --gid "$SERVICE_GROUP" --home-dir "$INSTALL_DIR" --shell /usr/sbin/nologin "$SERVICE_USER"
  fi
}

build_or_select_binary() {
  if [[ "$INSTALL_FROM_RELEASE" -eq 1 ]]; then
    BINARY_SOURCE="$RELEASE_EXTRACT_DIR/$APP_NAME"
    if [[ ! -x "$BINARY_SOURCE" ]]; then
      echo "Release binary not found or not executable: $BINARY_SOURCE" >&2
      exit 1
    fi
    return
  fi

  if [[ "$BUILD_BINARY" -eq 1 ]]; then
    local build_dir build_version ldflags
    build_dir="$(mktemp -d)"
    build_version="$(source_build_version)"
    ldflags="-s -w -X postizer/internal/http.AppVersion=$build_version"
    trap "rm -rf '$build_dir'" EXIT
    (cd "$SOURCE_DIR" && "$GO_CMD" build -trimpath -ldflags="$ldflags" -o "$build_dir/$APP_NAME" ./cmd/postizer)
    BINARY_SOURCE="$build_dir/$APP_NAME"
  fi

  if [[ ! -x "$BINARY_SOURCE" ]]; then
    echo "Binary not found or not executable: $BINARY_SOURCE" >&2
    exit 1
  fi
}

write_env_file() {
  mkdir -p "$(dirname "$ENV_FILE")"
  if [[ -f "$ENV_FILE" ]]; then
    echo "Keeping existing environment file: $ENV_FILE"
    return
  fi

  local admin_user="${POSTIZER_ADMIN_USER:-admin}"
  local admin_password="${POSTIZER_ADMIN_PASSWORD:-$(random_secret 18)}"
  local session_secret="${POSTIZER_SESSION_SECRET:-$(random_secret 48)}"

  (
    umask 077
    cat >"$ENV_FILE" <<EOF
POSTIZER_ADDR=$(env_quote "$LISTEN_ADDR")
POSTIZER_ADMIN_USER=$(env_quote "$admin_user")
POSTIZER_ADMIN_PASSWORD=$(env_quote "$admin_password")
POSTIZER_SESSION_SECRET=$(env_quote "$session_secret")
EOF
  )
  chmod 600 "$ENV_FILE"

  echo
  echo "Created $ENV_FILE"
  echo "Initial admin username: $admin_user"
  echo "Initial admin password: $admin_password"
  echo "Save this password now. It will not be printed again."
  echo
}

install_files() {
  local runtime_source="$SOURCE_DIR"
  if [[ "$INSTALL_FROM_RELEASE" -eq 1 ]]; then
    runtime_source="$RELEASE_EXTRACT_DIR"
    BINARY_SOURCE="$runtime_source/$APP_NAME"
  fi

  install -d -m 0755 "$INSTALL_DIR"
  install -m 0755 "$BINARY_SOURCE" "$INSTALL_DIR/$APP_NAME"
  install -d -m 0755 "$(dirname "$BIN_LINK")"
  ln -sfn "$INSTALL_DIR/$APP_NAME" "$BIN_LINK"

  install_runtime_dir "$runtime_source/web" "$INSTALL_DIR/web"
  install -d -m 0755 "$INSTALL_DIR/internal"
  install_runtime_dir "$runtime_source/internal/bundles" "$INSTALL_DIR/internal/bundles"
  if [[ -d "$runtime_source/scripts" ]]; then
    install_runtime_dir "$runtime_source/scripts" "$INSTALL_DIR/scripts"
  fi
  if [[ -f "$runtime_source/.postizer-version" ]]; then
    install -m 0644 "$runtime_source/.postizer-version" "$INSTALL_DIR/.postizer-version"
  fi

  if [[ ! -d "$INSTALL_DIR/content" && -d "$runtime_source/content" ]]; then
    cp -a "$runtime_source/content" "$INSTALL_DIR/content"
  fi
  mkdir -p "$INSTALL_DIR/content/posts" "$INSTALL_DIR/content/pages" "$INSTALL_DIR/content/tags"
  mkdir -p "$INSTALL_DIR/media"

  chown root:"$SERVICE_GROUP" "$INSTALL_DIR"
  chmod 0750 "$INSTALL_DIR"
  chown -R root:root "$INSTALL_DIR/$APP_NAME" "$INSTALL_DIR/web" "$INSTALL_DIR/internal"
  if [[ -d "$INSTALL_DIR/scripts" ]]; then
    chown -R root:root "$INSTALL_DIR/scripts"
    chmod -R u=rwX,go=rX "$INSTALL_DIR/scripts"
  fi
  if [[ -f "$INSTALL_DIR/.postizer-version" ]]; then
    chown root:root "$INSTALL_DIR/.postizer-version"
  fi
  chmod 0755 "$INSTALL_DIR/$APP_NAME"
  chmod -R u=rwX,go=rX "$INSTALL_DIR/web" "$INSTALL_DIR/internal"
  chown -R "$SERVICE_USER:$SERVICE_GROUP" "$INSTALL_DIR/content" "$INSTALL_DIR/media"
}

write_service_file() {
  local service_path="/etc/systemd/system/$SERVICE_NAME.service"
  cat >"$service_path" <<EOF
[Unit]
Description=Postizer blog service
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=$SERVICE_USER
Group=$SERVICE_GROUP
WorkingDirectory=$INSTALL_DIR
EnvironmentFile=-$ENV_FILE
ExecStart=$INSTALL_DIR/$APP_NAME
Restart=on-failure
RestartSec=5s
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=full
ReadWritePaths=$INSTALL_DIR/content $INSTALL_DIR/media

[Install]
WantedBy=multi-user.target
EOF
}

write_update_timer_files() {
  if [[ "$INSTALL_UPDATE_TIMER" -ne 1 ]]; then
    return
  fi

  local updater_path="/usr/local/bin/$SERVICE_NAME-auto-update"
  local update_service_path="/etc/systemd/system/$UPDATE_SERVICE_NAME.service"
  local update_timer_path="/etc/systemd/system/$UPDATE_TIMER_NAME.timer"
  local script_path="$INSTALL_DIR/scripts/install-linux-service.sh"

  cat >"$updater_path" <<EOF
#!/usr/bin/env bash
set -Eeuo pipefail
exec bash $(printf '%q' "$script_path") \\
  --auto-update-run \\
  --repo-url $(printf '%q' "$REPO_URL") \\
  --release-version $(printf '%q' "$RELEASE_VERSION_REQUESTED") \\
  --install-dir $(printf '%q' "$INSTALL_DIR") \\
  --service-name $(printf '%q' "$SERVICE_NAME") \\
  --update-service-name $(printf '%q' "$UPDATE_SERVICE_NAME") \\
  --update-timer-name $(printf '%q' "$UPDATE_TIMER_NAME") \\
  --user $(printf '%q' "$SERVICE_USER") \\
  --group $(printf '%q' "$SERVICE_GROUP") \\
  --addr $(printf '%q' "$LISTEN_ADDR") \\
  --bin-link $(printf '%q' "$BIN_LINK") \\
  --env-file $(printf '%q' "$ENV_FILE") \\
  --update-interval $(printf '%q' "$UPDATE_TIMER_INTERVAL")
EOF
  chmod 0755 "$updater_path"

  cat >"$update_service_path" <<EOF
[Unit]
Description=Postizer automatic update
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
WorkingDirectory=$INSTALL_DIR
ExecStart=$updater_path
EOF

  cat >"$update_timer_path" <<EOF
[Unit]
Description=Check for Postizer updates

[Timer]
OnBootSec=5min
OnUnitActiveSec=$UPDATE_TIMER_INTERVAL
RandomizedDelaySec=2min
Persistent=true
Unit=$UPDATE_SERVICE_NAME.service

[Install]
WantedBy=timers.target
EOF
}

if [[ "${EUID}" -ne 0 ]]; then
  validate_paths
  if [[ "$INSTALL_FROM_RELEASE" -ne 1 && -w "$SOURCE_DIR" && ( -e "$SOURCE_DIR/.git" || -f "$SOURCE_DIR/go.mod" ) ]]; then
    if [[ "$AUTO_UPDATE_RUN" -eq 1 ]]; then
      update_source_tree_to_latest_release_tag
    else
      update_source_tree
    fi
  fi
  if [[ ! -f "$SCRIPT_SELF" ]]; then
    echo "Please save this installer to a file before running it without root privileges." >&2
    exit 1
  fi
  exec sudo -E bash "$SCRIPT_SELF" "${ORIGINAL_ARGS[@]}"
fi

validate_paths
if needs_dependency_install; then
  install_os_packages
fi
if [[ "$AUTO_UPDATE_RUN" -eq 1 ]] && ! auto_update_enabled; then
  echo "Automatic updates are disabled in $INSTALL_DIR/content/settings.json"
  exit 0
fi
if [[ "$INSTALL_FROM_RELEASE" -eq 1 ]]; then
  download_release_asset
else
  if [[ "$AUTO_UPDATE_RUN" -eq 1 ]]; then
    update_source_tree_to_latest_release_tag
  else
    update_source_tree
  fi
  if [[ "$AUTO_UPDATE_RUN" -eq 1 && "$GIT_UPDATED" -ne 1 ]]; then
    echo "No newer release tag found."
    exit 0
  fi
  load_go_requirement
  prepare_go_build_environment
  install_go_toolchain
fi
need_command systemctl
need_command cp
need_command getent
need_command install
need_command groupadd
need_command useradd

ensure_service_identity
build_or_select_binary
write_env_file
install_files
write_service_file
write_update_timer_files

systemctl daemon-reload
if [[ "$ENABLE_SERVICE" -eq 1 ]]; then
  systemctl enable "$SERVICE_NAME.service"
  if [[ "$INSTALL_UPDATE_TIMER" -eq 1 ]]; then
    systemctl enable "$UPDATE_TIMER_NAME.timer"
  fi
fi
if [[ "$START_SERVICE" -eq 1 ]]; then
  systemctl restart "$SERVICE_NAME.service"
  if [[ "$INSTALL_UPDATE_TIMER" -eq 1 ]]; then
    systemctl restart "$UPDATE_TIMER_NAME.timer"
  fi
fi

if [[ "$AUTO_UPDATE_RUN" -eq 1 ]]; then
  record_update_event "update_completed" "$UPDATE_TARGET_VERSION" "Completed the local Postizer update."
fi

echo "Installed Postizer to $INSTALL_DIR"
echo "Service: $SERVICE_NAME.service"
if [[ "$INSTALL_UPDATE_TIMER" -eq 1 ]]; then
  echo "Auto-update service: $UPDATE_SERVICE_NAME.service"
  echo "Auto-update timer: $UPDATE_TIMER_NAME.timer"
fi
echo "Status: systemctl status $SERVICE_NAME.service"
echo "Logs:   journalctl -u $SERVICE_NAME.service -f"
