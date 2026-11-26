#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

out_dir="$root/build/web"
mkdir -p "$out_dir"

export GOTOOLCHAIN=auto
export GOOS=js
export GOARCH=wasm

echo "Running go vet (js/wasm)..."
go vet ./cmd/web ./internal/demo ./internal/rules ./internal/audio ./internal/codec ./internal/report

echo "Building build/web/main.wasm..."
go build -trimpath -ldflags "-s -w" -o "$out_dir/main.wasm" ./cmd/web

goroot="$(go env GOROOT)"
wasm_exec="$goroot/lib/wasm/wasm_exec.js"
if [[ ! -f "$wasm_exec" ]]; then
  echo "wasm_exec.js not found at $wasm_exec" >&2
  exit 1
fi
cp -f "$wasm_exec" "$out_dir/wasm_exec.js"

echo "Copying frontend assets..."
cp -rf "$root/frontend/." "$out_dir/"

echo "Generating licenses.txt..."
{
  add_file() {
    local rel="$1"
    printf '==================== %s ====================\n\n' "$rel"
    cat "$root/$rel"
    printf '\n\n'
  }
  add_file "LICENSE"
  add_file "THIRD_PARTY_NOTICES.md"
  while IFS= read -r lf; do
    add_file "licenses/$(basename "$lf")"
  done < <(LC_ALL=C find "$root/licenses" -maxdepth 1 -name '*.txt' -type f | sort)
} > "$out_dir/licenses.txt"

echo "Web build complete: build/web/"
