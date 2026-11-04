#!/usr/bin/env bash
# Runs vet + tests. Exit 0 on success, 1 on failure. Last line is machine-readable.
set -u
cd "$(dirname "$0")/.."

GO="${GO:-go}"
if ! command -v "$GO" >/dev/null 2>&1; then
  for p in "$HOME/sdk-go/go/bin/go" "$HOME/sdk-go/go/bin/go.exe" /usr/local/go/bin/go; do
    [ -x "$p" ] && GO="$p" && break
  done
fi

if ! "$GO" vet ./...; then
  echo "VERIFY wake-proxy FAIL stage=vet"
  exit 1
fi

out="$("$GO" test -v ./... 2>&1)"
status=$?
printf '%s\n' "$out" | grep -v '^=== ' | grep -E '^(--- FAIL|FAIL|ok|panic)'
passed="$(printf '%s\n' "$out" | grep -c '^--- PASS')"

if [ "$status" -ne 0 ]; then
  echo "VERIFY wake-proxy FAIL stage=test tests=$passed"
  exit 1
fi
echo "VERIFY wake-proxy PASS tests=$passed"
