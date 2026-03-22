#!/usr/bin/env bash
set -euo pipefail

# =============================================================================
# E2E Test: Server + Agent Join Flow
# Tests server start, token generation, agent join, heartbeat,
# and remote VM creation via sync.
#
# Requires: KVM host, Go binaries built (cd nftables && make server agent), jq
# Usage: sudo bash tests/e2e/test-server-agent.sh
# =============================================================================

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/lib/helpers.sh"

# Resolve project root (tests/e2e/../../ = project root)
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

VM_IDS=()
SERVER_PID=""
DB_FILES=()

cleanup() {
  echo ""
  echo "--- Cleanup ---"
  if [ -n "$SERVER_PID" ]; then
    echo "  Stopping server (PID $SERVER_PID)..."
    kill "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
  fi
  for id in "${VM_IDS[@]}"; do
    [ -n "$id" ] && remove_vm "$id"
  done
  for db in "${DB_FILES[@]}"; do
    rm -f "$db" 2>/dev/null || true
  done
  rm -f "$HOME/.vmsan/agent.json" 2>/dev/null || true
}
trap cleanup EXIT

echo "================================================================"
echo "  E2E Test: Server + Agent Join Flow"
echo "================================================================"

# Check for server and agent-host binaries
SERVER_BIN="${PROJECT_ROOT}/nftables/vmsan-server"
AGENT_BIN="${PROJECT_ROOT}/nftables/vmsan-agent-host"

if [ ! -x "$SERVER_BIN" ]; then
  # Try to build
  echo "Building server binary..."
  (cd "${PROJECT_ROOT}/nftables" && make server 2>/dev/null) || true
fi

if [ ! -x "$AGENT_BIN" ]; then
  echo "Building agent-host binary..."
  (cd "${PROJECT_ROOT}/nftables" && make agent 2>/dev/null) || true
fi

if [ ! -x "$SERVER_BIN" ]; then
  echo "SKIP: vmsan-server binary not found at $SERVER_BIN"
  echo "  Build with: cd nftables && make server"
  exit 0
fi

if [ ! -x "$AGENT_BIN" ]; then
  echo "SKIP: vmsan-agent-host binary not found at $AGENT_BIN"
  echo "  Build with: cd nftables && make agent"
  exit 0
fi

# ---------------------------------------------------------------------------
# Test H1: vmsan server starts and responds
# ---------------------------------------------------------------------------
echo ""
echo "--- Test H1: vmsan server starts and responds ---"

DB_H1="/tmp/vmsan-test-h1.db"
DB_FILES+=("$DB_H1")

sudo "$SERVER_BIN" --listen 127.0.0.1:16443 --db "$DB_H1" &
SERVER_PID=$!
sleep 2

# Check status endpoint
STATUS_RESPONSE=$(curl -sf http://127.0.0.1:16443/api/v1/status 2>/dev/null || echo '{}')
STATUS_OK=$(echo "$STATUS_RESPONSE" | jq -r '.ok // false' 2>/dev/null || echo "false")
assert_eq "$STATUS_OK" "true" "H1: server status endpoint returns ok=true"

# Stop server for next test
kill "$SERVER_PID" 2>/dev/null || true
wait "$SERVER_PID" 2>/dev/null || true
SERVER_PID=""

# ---------------------------------------------------------------------------
# Test H2: Agent join with token
# ---------------------------------------------------------------------------
echo ""
echo "--- Test H2: Agent join with token ---"

DB_H2="/tmp/vmsan-test-h2.db"
DB_FILES+=("$DB_H2")

sudo "$SERVER_BIN" --listen 127.0.0.1:16443 --db "$DB_H2" &
SERVER_PID=$!
sleep 2

# Generate token
TOKEN=$(curl -sf -X POST http://127.0.0.1:16443/api/v1/tokens 2>/dev/null | jq -r '.token // empty' || true)
assert_not_empty "$TOKEN" "H2: token generated"

if [ -n "$TOKEN" ]; then
  # Join
  sudo "$AGENT_BIN" join --server http://127.0.0.1:16443 --token "$TOKEN" --name test-host 2>&1 || true

  # Verify host appeared
  HOSTS=$(curl -sf http://127.0.0.1:16443/api/v1/hosts 2>/dev/null || echo '[]')
  HOST_COUNT=$(echo "$HOSTS" | jq 'length' 2>/dev/null || echo "0")
  assert_eq "$HOST_COUNT" "1" "H2: server has 1 host after join"

  HOST_NAME=$(echo "$HOSTS" | jq -r '.[0].name // empty' 2>/dev/null || true)
  assert_eq "$HOST_NAME" "test-host" "H2: host name is 'test-host'"
fi

kill "$SERVER_PID" 2>/dev/null || true
wait "$SERVER_PID" 2>/dev/null || true
SERVER_PID=""
rm -f "$HOME/.vmsan/agent.json" 2>/dev/null || true

# ---------------------------------------------------------------------------
# Test H3: Join token is single-use
# ---------------------------------------------------------------------------
echo ""
echo "--- Test H3: Join token is single-use ---"

DB_H3="/tmp/vmsan-test-h3.db"
DB_FILES+=("$DB_H3")

sudo "$SERVER_BIN" --listen 127.0.0.1:16443 --db "$DB_H3" &
SERVER_PID=$!
sleep 2

TOKEN=$(curl -sf -X POST http://127.0.0.1:16443/api/v1/tokens 2>/dev/null | jq -r '.token // empty' || true)

if [ -n "$TOKEN" ]; then
  # First join succeeds
  sudo "$AGENT_BIN" join --server http://127.0.0.1:16443 --token "$TOKEN" --name host-1 2>&1 || true
  rm -f "$HOME/.vmsan/agent.json" 2>/dev/null || true

  # Second join with same token should fail
  if sudo "$AGENT_BIN" join --server http://127.0.0.1:16443 --token "$TOKEN" --name host-2 2>&1; then
    echo -e "${RED}FAIL${NC}: H3: second join with same token should have failed"
    TESTS_FAILED=$((TESTS_FAILED + 1))
  else
    echo -e "${GREEN}PASS${NC}: H3: second join with consumed token correctly rejected"
    TESTS_PASSED=$((TESTS_PASSED + 1))
  fi

  # Only 1 host registered
  HOST_COUNT=$(curl -sf http://127.0.0.1:16443/api/v1/hosts 2>/dev/null | jq 'length' || echo "0")
  assert_eq "$HOST_COUNT" "1" "H3: only 1 host registered (token is single-use)"
else
  echo -e "${RED}FAIL${NC}: H3: could not generate token"
  TESTS_FAILED=$((TESTS_FAILED + 1))
fi

kill "$SERVER_PID" 2>/dev/null || true
wait "$SERVER_PID" 2>/dev/null || true
SERVER_PID=""
rm -f "$HOME/.vmsan/agent.json" 2>/dev/null || true

# ---------------------------------------------------------------------------
# Test H4: vmsan hosts list shows joined agent
# ---------------------------------------------------------------------------
echo ""
echo "--- Test H4: vmsan hosts list shows joined agent ---"

DB_H4="/tmp/vmsan-test-h4.db"
DB_FILES+=("$DB_H4")

sudo "$SERVER_BIN" --listen 127.0.0.1:16443 --db "$DB_H4" &
SERVER_PID=$!
sleep 2

TOKEN=$(curl -sf -X POST http://127.0.0.1:16443/api/v1/tokens 2>/dev/null | jq -r '.token // empty' || true)

if [ -n "$TOKEN" ]; then
  sudo "$AGENT_BIN" join --server http://127.0.0.1:16443 --token "$TOKEN" --name worker-1 2>&1 || true

  # Use the CLI (requires VMSAN_SERVER_URL)
  CLI_OUTPUT=$(VMSAN_SERVER_URL=http://127.0.0.1:16443 vmsan hosts list 2>/dev/null || true)
  assert_contains "$CLI_OUTPUT" "worker-1" "H4: vmsan hosts list shows worker-1"

  # JSON output
  JSON_OUTPUT=$(VMSAN_SERVER_URL=http://127.0.0.1:16443 vmsan hosts list --json 2>/dev/null || echo '[]')
  JSON_NAME=$(echo "$JSON_OUTPUT" | jq -r '.[0].name // empty' 2>/dev/null || true)
  assert_eq "$JSON_NAME" "worker-1" "H4: JSON output has worker-1"
else
  echo -e "${RED}FAIL${NC}: H4: could not generate token"
  TESTS_FAILED=$((TESTS_FAILED + 1))
fi

kill "$SERVER_PID" 2>/dev/null || true
wait "$SERVER_PID" 2>/dev/null || true
SERVER_PID=""
rm -f "$HOME/.vmsan/agent.json" 2>/dev/null || true

# ---------------------------------------------------------------------------
# Test H5: vmsan create --host dispatches to remote
# ---------------------------------------------------------------------------
echo ""
echo "--- Test H5: vmsan create --host dispatches to remote ---"

DB_H5="/tmp/vmsan-test-h5.db"
DB_FILES+=("$DB_H5")

sudo "$SERVER_BIN" --listen 127.0.0.1:16443 --db "$DB_H5" &
SERVER_PID=$!
sleep 2

TOKEN=$(curl -sf -X POST http://127.0.0.1:16443/api/v1/tokens 2>/dev/null | jq -r '.token // empty' || true)

if [ -n "$TOKEN" ]; then
  sudo "$AGENT_BIN" join --server http://127.0.0.1:16443 --token "$TOKEN" --name target-host 2>&1 || true

  # Create VM targeting the remote host
  VMSAN_SERVER_URL=http://127.0.0.1:16443 sudo vmsan create \
    --runtime base --vcpus 1 --memory 256 --host target-host --json 2>&1 || true

  # VM should appear in server's VM list
  VM_COUNT=$(curl -sf http://127.0.0.1:16443/api/v1/vms 2>/dev/null | jq 'length' || echo "0")
  assert_gt "$VM_COUNT" 0 "H5: VM dispatched to remote host appears in server VM list"
else
  echo -e "${RED}FAIL${NC}: H5: could not generate token"
  TESTS_FAILED=$((TESTS_FAILED + 1))
fi

kill "$SERVER_PID" 2>/dev/null || true
wait "$SERVER_PID" 2>/dev/null || true
SERVER_PID=""
rm -f "$HOME/.vmsan/agent.json" 2>/dev/null || true

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
print_summary
