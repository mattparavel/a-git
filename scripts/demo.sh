#!/usr/bin/env bash
# End-to-end demo / smoke test: starts a server, a regional cache and a
# per-node daemon, then drives the agit CLI through the main workflows.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/agit-demo.XXXXXX")"
BIN="$WORK/bin"
SERVER_PORT="${SERVER_PORT:-17070}"
CACHE_PORT="${CACHE_PORT:-17071}"
export AGIT_DAEMON_SOCKET="$WORK/agitd.sock"
export AGIT_AUTHOR="Demo User <demo@example.com>" AGIT_USER=demo
export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1
unset AGIT_SERVER AGIT_REPO AGIT_TOKEN

pids=()
cleanup() {
  for p in "${pids[@]:-}"; do kill "$p" 2>/dev/null || true; done
  wait 2>/dev/null || true
  if [[ -z "${KEEP:-}" ]]; then rm -rf "$WORK"; else echo "kept $WORK"; fi
}
trap cleanup EXIT

step() { printf '\n\033[1m==> %s\033[0m\n' "$*"; }
wait_for() { for _ in $(seq 100); do if eval "$1" >/dev/null 2>&1; then return 0; fi; sleep 0.1; done; echo "timed out: $1"; exit 1; }

step "build"
(cd "$ROOT" && go build -o "$BIN/" ./cmd/...)
agit() { "$BIN/agit" "$@"; }

step "start agit-server <- agit-cache <- agitd"
"$BIN/agit-server" -listen "127.0.0.1:$SERVER_PORT" -data "$WORK/server-data" -create demo 2>"$WORK/server.log" & pids+=($!)
wait_for "curl -sf http://127.0.0.1:$SERVER_PORT/v1/info"
"$BIN/agit-cache" -listen "127.0.0.1:$CACHE_PORT" -upstream "http://127.0.0.1:$SERVER_PORT" 2>"$WORK/cache.log" & pids+=($!)
wait_for "curl -sf http://127.0.0.1:$CACHE_PORT/v1/info"
"$BIN/agitd" -socket "$AGIT_DAEMON_SOCKET" -upstream "http://127.0.0.1:$CACHE_PORT" -cache-dir "$WORK/daemon-objects" 2>"$WORK/daemon.log" & pids+=($!)
wait_for "test -S $AGIT_DAEMON_SOCKET"
agit info --repo demo

step "import an existing git repository"
G="$WORK/gitsrc"
git init -q -b main "$G"
mkdir -p "$G/src/app" "$G/src/lib" "$G/docs"
printf 'package app\n\nfunc Run() int {\n\treturn 1\n}\n' > "$G/src/app/app.go"
printf 'package lib\n\n// Add adds.\nfunc Add(a, b int) int { return a + b }\n' > "$G/src/lib/lib.go"
printf '# Demo\n' > "$G/docs/README.md"
git -C "$G" add -A && git -C "$G" -c user.name=Ada -c user.email=ada@example.com commit -qm "initial import"
printf 'package lib\n\n// Add adds two numbers.\nfunc Add(a, b int) int { return a + b }\n' > "$G/src/lib/lib.go"
git -C "$G" -c user.name=Ada -c user.email=ada@example.com commit -qam "lib: better comment"
agit import-git "$G" --repo demo --state "$WORK/import-state"
agit log --repo demo --oneline

step "sparse clone through the daemon (only src/app is fetched)"
cd "$WORK"
agit clone demo alice --sparse src/app
ls -R alice | sed -n '/./p'
cd alice
agit sparse add src/lib
agit status

step "edit, commit on the server, inspect"
printf 'package app\n\nfunc Run() int {\n\treturn 2\n}\n' > src/app/app.go
printf 'package app\n\nfunc Helper() {}\n' > src/app/helper.go
agit status
agit diff
agit commit -m "app: return 2 and add a helper"
agit stack
agit show --stat

step "a second developer lands a change concurrently"
cd "$WORK"
agit clone demo bob --sparse src/lib
cd bob
printf 'package lib\n\n// Add adds two numbers.\nfunc Add(a, b int) int { return a + b }\n\nfunc Sub(a, b int) int { return a - b }\n' > src/lib/lib.go
agit commit -m "lib: add Sub"
agit land

step "alice lands: the server rebases her stack onto bob's change without a global lock"
cd "$WORK/alice"
agit land
agit log --oneline -n 5
agit blame src/lib/lib.go
agit log --path src/app --oneline

step "read without any checkout"
agit cat --repo demo main:src/app/app.go
agit ls --repo demo main src
agit search --repo demo '**/*.go'

step "review with semantic approvals"
printf 'package app\n\nfunc Run() int {\n\treturn 3\n}\n' > src/app/app.go
agit commit -m "app: return 3"
agit review create -m "Return three"
printf 'package app\n\n// Run returns three.\nfunc Run() int {\n\treturn 3\n}\n' > src/app/app.go
agit commit -m "app: document Run"
AGIT_USER=reviewer agit review approve 1 | tail -3
agit review update 1
agit review land 1
agit review find main

step "revert, bisect and events"
agit revert main~1 --land
agit bisect --good main~4 --bad main -- sh -c '! grep -q "return 3" src/app/app.go'
agit events | sed -n '/^ref/p'

step "export to a git mirror and verify"
agit export-git "$WORK/mirror" --repo demo
git -C "$WORK/mirror" log --oneline -5
git -C "$WORK/mirror" fsck --no-progress && echo "git fsck: ok"

step "tier statistics"
agit info --metrics | grep -E '"(hit_rate|hits|misses|objects)"' | sed -n '1,8p'

echo
echo "demo completed successfully"
