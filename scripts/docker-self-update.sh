#!/bin/sh
set -eu

repo_url="${POSTIZER_REPO_URL:-https://github.com/WiDayn/Postizer.git}"
release_version="${POSTIZER_RELEASE_VERSION:-latest}"
runtime_root="${POSTIZER_RUNTIME_ROOT:-/app/runtime}"
current_link="${POSTIZER_CURRENT_LINK:-$runtime_root/current}"
lock_dir="$runtime_root/.self-update.lock"

log() {
  printf '%s\n' "$*" >&2
}

update_event() {
  printf 'POSTIZER_UPDATE_EVENT\t%s\t%s\t%s\t%s\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')" "$1" "$2" "$3" >&2
}

fail() {
  log "$*"
  exit 1
}

download_file() {
  url="$1"
  output="$2"
  if [ -n "${POSTIZER_GITHUB_TOKEN:-}" ]; then
    curl -fsSL -H "Authorization: Bearer $POSTIZER_GITHUB_TOKEN" "$url" -o "$output"
  else
    curl -fsSL "$url" -o "$output"
  fi
}

github_repo_slug() {
  value="$repo_url"
  value="${value#https://github.com/}"
  value="${value#http://github.com/}"
  value="${value#git@github.com:}"
  value="${value%.git}"
  case "$value" in
    */*) printf '%s' "$value" ;;
    *) fail "POSTIZER_REPO_URL must point to a GitHub repository: $repo_url" ;;
  esac
}

is_release_tag() {
  printf '%s' "$1" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$'
}

latest_release_tag() {
  slug="$1"
  tmp="$2"
  api_url="https://api.github.com/repos/$slug/releases/latest"
  download_file "$api_url" "$tmp"
  tag="$(sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$tmp" | head -n 1)"
  [ -n "$tag" ] || fail "Could not find tag_name in GitHub latest release response."
  is_release_tag "$tag" || fail "Latest GitHub release tag is not vX.X.X: $tag"
  printf '%s' "$tag"
}

platform_name() {
  os="linux"
  case "$(uname -m)" in
    x86_64|amd64) arch="amd64" ;;
    aarch64|arm64) arch="arm64" ;;
    *) fail "Unsupported Linux architecture for Postizer release updates: $(uname -m)" ;;
  esac
  printf '%s-%s' "$os" "$arch"
}

current_version() {
  if [ -f "$current_link/.postizer-version" ]; then
    tr -d '\r\n' < "$current_link/.postizer-version"
  fi
}

verify_checksum() {
  sums_file="$1"
  asset_name="$2"
  asset_path="$3"
  checksum="$(awk -v asset="$asset_name" '$2 == asset || $2 == "*" asset { print $1; exit }' "$sums_file")"
  if [ -z "$checksum" ]; then
    fail "SHA256SUMS does not contain $asset_name."
  fi
  actual="$(sha256sum "$asset_path" | awk '{print $1}')"
  [ "$checksum" = "$actual" ] || fail "Checksum mismatch for $asset_name."
}

cleanup() {
  rm -rf "${tmp_root:-}"
  rmdir "$lock_dir" 2>/dev/null || true
}

mkdir -p "$runtime_root/releases"
if ! mkdir "$lock_dir" 2>/dev/null; then
  log "Postizer self-update is already running."
  exit 0
fi
trap cleanup EXIT INT TERM

tmp_root="$(mktemp -d)"
slug="$(github_repo_slug)"
if [ "$release_version" = "latest" ] || [ -z "$release_version" ]; then
  release_version="$(latest_release_tag "$slug" "$tmp_root/latest.json")"
fi
is_release_tag "$release_version" || fail "Release version must match vX.X.X: $release_version"

if [ "$(current_version)" = "$release_version" ] && [ -x "$current_link/postizer" ]; then
  log "Already running Postizer $release_version."
  exit 0
fi

update_event "update_detected" "$release_version" "Detected a newer Postizer release."

platform="$(platform_name)"
asset_name="postizer-$release_version-$platform.tar.gz"
asset_url="https://github.com/$slug/releases/download/$release_version/$asset_name"
sums_url="https://github.com/$slug/releases/download/$release_version/SHA256SUMS"
archive="$tmp_root/$asset_name"
sums="$tmp_root/SHA256SUMS"
extract_dir="$tmp_root/extract"
release_dir="$runtime_root/releases/$release_version"

if [ -x "$release_dir/postizer" ] && [ -d "$release_dir/web" ] && [ -d "$release_dir/internal/bundles" ]; then
  log "Switching to already-downloaded Postizer $release_version."
  ln -sfn "$release_dir" "$current_link"
  update_event "update_completed" "$release_version" "Switched to an already-downloaded Postizer release."
  exit 42
fi

log "Downloading $asset_url"
download_file "$asset_url" "$archive"
log "Downloading $sums_url"
download_file "$sums_url" "$sums"
verify_checksum "$sums" "$asset_name" "$archive"

mkdir -p "$extract_dir"
tar -xzf "$archive" -C "$extract_dir"

if [ ! -x "$extract_dir/postizer" ] && [ "$(find "$extract_dir" -mindepth 1 -maxdepth 1 -type d | wc -l | tr -d ' ')" = "1" ]; then
  extract_dir="$(find "$extract_dir" -mindepth 1 -maxdepth 1 -type d | head -n 1)"
fi

[ -x "$extract_dir/postizer" ] || fail "Release asset does not contain an executable postizer binary."
[ -d "$extract_dir/web" ] || fail "Release asset does not contain web/."
[ -d "$extract_dir/internal/bundles" ] || fail "Release asset does not contain internal/bundles/."

printf '%s\n' "$release_version" > "$extract_dir/.postizer-version"
rm -rf "$release_dir"
mv "$extract_dir" "$release_dir"
ln -sfn "$release_dir" "$current_link"

log "Updated Postizer runtime to $release_version."
update_event "update_completed" "$release_version" "Updated the local Postizer runtime."
exit 42
