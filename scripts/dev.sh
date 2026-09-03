#!/usr/bin/env bash
# Run Cronus in development: the Go API/server with a local SQLite DB, and the
# Vite dev server with hot reload proxying /api to the backend.
set -euo pipefail
cd "$(dirname "$0")/.."

DB="${CRONUS_DB_PATH:-./data/cronus.dev.db}"
mkdir -p "$(dirname "$DB")"

echo "starting backend on :8080 (db: $DB)"
CRONUS_LOG_LEVEL="${CRONUS_LOG_LEVEL:-debug}" go run ./cmd/cronus serve --db.path "$DB" &
BACKEND=$!
trap 'kill $BACKEND 2>/dev/null || true' EXIT

echo "starting frontend on :5173 (proxies /api -> :8080)"
cd web
[ -d node_modules ] || npm install
npm run dev
