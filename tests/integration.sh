#!/bin/sh
set -eu
base=$(mktemp -d)
pid=
cleanup() { if [ -n "$pid" ]; then kill "$pid" 2>/dev/null || true; wait "$pid" || true; fi; }
trap cleanup EXIT INT TERM
mkdir "$base/mount"
go build -o "$base/dwbc" ./cmd/dwbc
"$base/dwbc" -mountpoint "$base/mount" -log "$base/events.jsonl" &
pid=$!
i=0
until test -f "$base/mount/access.log"; do i=$((i+1)); test "$i" -lt 100; sleep .1; done
test "$(stat -c %s "$base/mount/access.log")" = 61337501696
head -c 32 "$base/mount/access.log" | grep -q '0.0.0.0'
if truncate -s 0 "$base/mount/access.log"; then exit 1; fi
if chmod 600 "$base/mount/access.log"; then exit 1; fi
if mv "$base/mount/access.log" "$base/mount/other"; then exit 1; fi
if touch "$base/mount/temp"; then exit 1; fi
kill "$pid"; wait "$pid"; pid=
for op in read setattr rename create; do grep -q "\"operation\":\"$op\"" "$base/events.jsonl"; done
