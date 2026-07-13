#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SOURCE_DIR="$ROOT_DIR/examples/bundles/webdav-backup"
PLUGIN_DIR="$SOURCE_DIR/plugins/webdav-backup"
OUT_DIR="${POSTIZER_PLUGIN_DIST_DIR:-$ROOT_DIR/dist/plugins}"
VERSION="${WEBDAV_BACKUP_VERSION:-1.0.0}"
ASSET="webdav-backup-v$VERSION.zip"
STAGE="$(mktemp -d)"

mkdir -p "$OUT_DIR"
OUT_DIR="$(cd "$OUT_DIR" && pwd)"

cleanup() {
  rm -rf "$STAGE"
}
trap cleanup EXIT

mkdir -p "$STAGE/plugins/webdav-backup/bin"
cp "$SOURCE_DIR/manifest.json" "$STAGE/manifest.json"
cp "$PLUGIN_DIR/manifest.json" "$STAGE/plugins/webdav-backup/manifest.json"
cp -a "$PLUGIN_DIR/ui" "$STAGE/plugins/webdav-backup/ui"

build_plugin() {
  local goos="$1"
  local goarch="$2"
  local suffix=""
  if [[ "$goos" == "windows" ]]; then
    suffix=".exe"
  fi
  echo "Building webdav-backup for $goos/$goarch..."
  (
    cd "$PLUGIN_DIR"
    CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
      go build -trimpath -ldflags="-s -w" \
      -o "$STAGE/plugins/webdav-backup/bin/webdav-backup-$goos-$goarch$suffix" \
      ./cmd/webdav-backup
  )
}

build_plugin linux amd64
build_plugin linux arm64
build_plugin darwin amd64
build_plugin darwin arm64
build_plugin windows amd64

rm -f "$OUT_DIR/$ASSET"
(
  cd "$STAGE"
  zip -qr "$OUT_DIR/$ASSET" .
)

(
  cd "$OUT_DIR"
  sha256sum "$ASSET" > "$ASSET.sha256"
)

echo "Resource pack written to $OUT_DIR/$ASSET"
