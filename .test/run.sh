#!/usr/bin/env bash
# .test/run.sh — Run unit tests + integration tests against DeepSeek API
# Usage: bash .test/run.sh  (from backend-v2 root)
#        or: cd .test && bash run.sh

set -e
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Load .env
if [ -f "$SCRIPT_DIR/.env" ]; then
  set -a
  source "$SCRIPT_DIR/.env"
  set +a
fi

echo "=== Unit Tests ==="
cd "$ROOT"
go test \
  ./internal/core/tokenizer/... \
  ./internal/engine/parser/... \
  ./internal/engine/processor/... \
  ./internal/engine/scheduled/... \
  ./internal/engine/variable/... \
  ./internal/engine/pipeline/... \
  -v -count=1

echo ""
echo "=== Integration Tests (DeepSeek API) ==="
go test -tags integration ./internal/integration/... -v -timeout 120s -count=1
