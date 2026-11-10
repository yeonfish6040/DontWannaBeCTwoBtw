#!/bin/sh
set -eu
base=$(mktemp -d)
pid=
cleanup() { if [ -n "$pid" ]; then kill "$pid" 2>/dev/null || true; wait "$pid" || true; fi; }
trap cleanup EXIT INT TERM
mkdir "$base/mount"
mkdir -m 700 "$base/evidence"
printf '{"evidence_dir":"%s","max_evidence":16}\n' "$base/evidence" > "$base/config.json"
go build -o "$base/dwbc" ./cmd/dwbc
"$base/dwbc" -mountpoint "$base/mount" -log "$base/events.jsonl" -config "$base/config.json" &
pid=$!
i=0
until test -f "$base/mount/access.log"; do i=$((i+1)); test "$i" -lt 100; sleep .1; done
test "$(stat -c %s "$base/mount/access.log")" = 61337501696
head -c 32 "$base/mount/access.log" | grep -q '0.0.0.0'
if truncate -s 0 "$base/mount/access.log"; then exit 1; fi
if chmod 600 "$base/mount/access.log"; then exit 1; fi
if mv "$base/mount/access.log" "$base/mount/other"; then exit 1; fi
if touch "$base/mount/temp"; then exit 1; fi
if timeout 5 sed -i '1d' "$base/mount/access.log"; then exit 1; else test "$?" -ne 124; fi
if ln -s access.log "$base/mount/link"; then exit 1; fi
# Repeated short readers exercise concurrent callbacks before shutdown.
for n in 1 2 3 4; do head -c 1048576 "$base/mount/access.log" > /dev/null & done
sleep .2
kill "$pid"; wait "$pid"; pid=
test "$(find "$base/evidence" -name 'event-*.json' | wc -l)" -gt 0
for op in read setattr rename create; do grep -q "\"operation\":\"$op\"" "$base/events.jsonl"; done
