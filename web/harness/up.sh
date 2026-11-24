#!/usr/bin/env bash
# harness/up.sh — brings the whole reversal surface up on real data, signed in.
#
#   bash web/harness/up.sh          # from the repo root
#
# Starts ledgerd on a scratch SQLite file, seeds it, starts the reverse-endpoint stub,
# then starts Vite pointed at the stub. Sign in as `alice` (the toy's token IS the
# principal). Ctrl-C stops all three.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RUN="${RUN_DIR:-$(mktemp -d)}"
cd "$ROOT"

echo "harness: run dir $RUN"
go build -o "$RUN/ledgerd" ./cmd/ledgerd

"$RUN/ledgerd" -db "$RUN/ledger.db" -addr :8080 -admins mint > "$RUN/ledgerd.log" 2>&1 &
LEDGERD_PID=$!
trap 'kill $LEDGERD_PID ${STUB_PID:-} ${VITE_PID:-} 2>/dev/null || true' EXIT

for _ in $(seq 1 50); do
  curl -sf http://localhost:8080/healthz > /dev/null && break
  sleep 0.1
done

# The stub starts BEFORE the seed and the seed runs THROUGH it: the stub learns each
# transfer's two legs from the POST /transfers responses it proxies, and a reversal needs
# both legs to build the compensating entry. Seeding straight at ledgerd would leave the
# stub unable to reverse anything it had not already watched.
bun web/harness/reverse-stub.mjs > "$RUN/stub.log" 2>&1 &
STUB_PID=$!
sleep 0.5

LEDGERD_URL=http://localhost:8081 bun web/harness/seed.mjs

echo "harness: sign in as 'alice' at http://localhost:5173"
cd web
VITE_API_TARGET=http://localhost:8081 bun run dev &
VITE_PID=$!
wait $VITE_PID
