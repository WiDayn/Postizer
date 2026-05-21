#!/bin/sh
set -eu

runtime_root="${POSTIZER_RUNTIME_ROOT:-/app/runtime}"
seed_root="${POSTIZER_SEED_ROOT:-/usr/src/postizer}"
release_name="${POSTIZER_SEED_VERSION:-seed}"
release_dir="$runtime_root/releases/$release_name"
current_link="$runtime_root/current"

mkdir -p "$runtime_root/releases"

if [ ! -x "$current_link/postizer" ]; then
  tmp_dir="$runtime_root/.seed-$release_name-$$"
  rm -rf "$tmp_dir"
  mkdir -p "$tmp_dir"
  cp -R "$seed_root/." "$tmp_dir/"
  chmod +x "$tmp_dir/postizer"
  rm -rf "$release_dir"
  mv "$tmp_dir" "$release_dir"
  ln -sfn "$release_dir" "$current_link"
fi

export POSTIZER_APP_ROOT="${POSTIZER_APP_ROOT:-$current_link}"
export POSTIZER_CONTENT_ROOT="${POSTIZER_CONTENT_ROOT:-/app/content}"
export POSTIZER_MEDIA_ROOT="${POSTIZER_MEDIA_ROOT:-/app/media}"
export POSTIZER_STATIC_ROOT="${POSTIZER_STATIC_ROOT:-$current_link/web/static}"
export POSTIZER_TEMPLATES_ROOT="${POSTIZER_TEMPLATES_ROOT:-$current_link/web/templates}"
export POSTIZER_BUILTIN_BUNDLES_ROOT="${POSTIZER_BUILTIN_BUNDLES_ROOT:-$current_link/internal/bundles}"

cd "$POSTIZER_APP_ROOT"
exec "$POSTIZER_APP_ROOT/postizer"
