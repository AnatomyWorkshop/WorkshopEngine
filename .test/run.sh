#!/usr/bin/env bash
# .test/run.sh — Run unit tests + integration tests against DeepSeek API
# Usage: bash .test/run.sh      (from backend-v2 root)
#        cd .test && bash run.sh

set -e
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Load .env
if [ -f "$SCRIPT_DIR/.env" ]; then
  set -a
  source "$SCRIPT_DIR/.env"
  set +a
fi

echo "=== Unit Tests (.test/) ==="
cd "$SCRIPT_DIR"
go test ./... -v -count=1

echo ""
echo "=== Integration Tests (DeepSeek API) ==="
cd "$ROOT"
go test -tags integration ./internal/integration/... -v -timeout 120s -count=1
