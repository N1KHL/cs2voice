#!/usr/bin/env bash
# Same checks as scripts/test.ps1, for Linux and macOS.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

echo "1/5 Checking gofmt..."
unformatted="$(gofmt -l .)"
if [[ -n "$unformatted" ]]; then
  printf 'gofmt reported unformatted files:\n%s\n' "$unformatted" >&2
  exit 1
fi

echo "2/5 Running go test ./..."
go test ./...

echo "3/5 Running js/wasm go vet..."
GOOS=js GOARCH=wasm go vet ./cmd/web ./internal/demo ./internal/rules ./internal/audio ./internal/codec ./internal/report

echo "4/5 Checking JS syntax (node --check)..."
for f in frontend/*.js; do
  node --check "$f"
done

echo "5/5 Running JS test suite (node --test)..."
shopt -s nullglob
tests=(tests/*.test.mjs)
if (( ${#tests[@]} == 0 )); then
  echo "No test files found in tests/*.test.mjs" >&2
  exit 1
fi
node --test "${tests[@]}"

echo "All checks passed."
