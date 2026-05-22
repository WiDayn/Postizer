#!/usr/bin/env bash
set -Eeuo pipefail

VERSION="${POSTIZER_RELEASE_VERSION:-}"
OUT_DIR="${POSTIZER_DIST_DIR:-dist/release}"

if [[ -z "$VERSION" ]]; then
  if git describe --tags --exact-match --match 'v[0-9]*.[0-9]*.[0-9]*' HEAD >/dev/null 2>&1; then
    VERSION="$(git describe --tags --exact-match --match 'v[0-9]*.[0-9]*.[0-9]*' HEAD)"
  else
    VERSION="$(git describe --tags --match 'v[0-9]*.[0-9]*.[0-9]*' --always 2>/dev/null || true)"
  fi
fi

if [[ ! "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "POSTIZER_RELEASE_VERSION must match vX.X.X; got: ${VERSION:-empty}" >&2
  exit 1
fi

rm -rf "$OUT_DIR"
mkdir -p "$OUT_DIR"

build_asset() {
  local goos="$1"
  local goarch="$2"
  local platform="$goos-$goarch"
  local stage="$OUT_DIR/postizer-$VERSION-$platform"
  local binary="$stage/postizer"
  if [[ "$goos" == "windows" ]]; then
    binary="$stage/postizer.exe"
  fi

  mkdir -p "$stage/internal"
  echo "Building $platform..."
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build -trimpath -ldflags="-s -w -X postizer/internal/http.AppVersion=$VERSION" -o "$binary" ./cmd/postizer
  cp -a web "$stage/web"
  cp -a internal/bundles "$stage/internal/bundles"
  cp -a marketplace "$stage/marketplace"
  mkdir -p "$stage/scripts"
  cp scripts/install-linux-service.sh "$stage/scripts/install-linux-service.sh"
  printf '%s\n' "$VERSION" > "$stage/.postizer-version"

  if [[ "$goos" == "windows" ]]; then
    (cd "$stage" && zip -qr "../postizer-$VERSION-$platform.zip" .)
  else
    tar -C "$stage" -czf "$OUT_DIR/postizer-$VERSION-$platform.tar.gz" .
  fi
}

build_asset linux amd64
build_asset linux arm64

(
  cd "$OUT_DIR"
  : > SHA256SUMS
  for asset in postizer-"$VERSION"-*.tar.gz postizer-"$VERSION"-*.zip; do
    [[ -f "$asset" ]] || continue
    sha256sum "$asset" >> SHA256SUMS
  done
)

echo "Release assets written to $OUT_DIR"
