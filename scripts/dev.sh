#!/usr/bin/env bash
# Run the Go API and the Vite dev server together.
#
# Vite proxies /api and /healthz to the Go process (see web/vite.config.ts), so
# the SPA runs against real data with hot reload. Open http://localhost:5173.
set -euo pipefail

cd "$(dirname "$0")/.."

if [ ! -d web/node_modules ]; then
  echo "installing frontend dependencies..."
  (cd web && npm ci)
fi

cleanup() {
  # Kill the whole process group so neither half is left running.
  trap - EXIT INT TERM
  kill 0 2>/dev/null || true
}
trap cleanup EXIT INT TERM

CGO_ENABLED=0 go run ./cmd/blessedbot --dev --port 8080 &
(cd web && npm run dev) &

wait
