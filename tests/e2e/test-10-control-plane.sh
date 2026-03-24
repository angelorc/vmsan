#!/usr/bin/env bash
set -euo pipefail

# =============================================================================
# E2E Test 10: Control Plane
# Tests server status, token generation, agent join, hosts CLI, and
# remote VM dispatch through create --host.
#
# Requires: KVM host, vmsan installed, curl, jq
# Usage: sudo bash tests/e2e/test-10-control-plane.sh
# =============================================================================

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/lib/helpers.sh"

REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
HOSTD_BIN_DIR="/tmp/vmsan-e2e-hostd"
SERVER_BIN=""
AGENT_BIN=""
SERVER_PID=""
AGENT_PID=""
SERVER_DB="/tmp/vmsan-e2e-control-plane.db"
SERVER_URL="http://127.0.0.1:16443"
AGENT_HOME="/tmp/vmsan-e2e-agent-home"
REMOTE_VM_IDS=()

cleanup() {
  echo ""
  echo "--- Cleanup ---"
  if [ "${#REMOTE_VM_IDS[@]}" -gt 0 ]; then
    for id in "${REMOTE_VM_IDS[@]}"; do
      gateway_rpc "vm.delete" "{\"vmId\":\"${id}\",\"force\":true,\"jailerBaseDir\":\"${VMSAN_DIR}/jailer\"}" >/dev/null 2>&1 || true
    done
  fi
  [ -n "$AGENT_PID" ] && kill "$AGENT_PID" 2>/dev/null || true
  [ -n "$SERVER_PID" ] && kill "$SERVER_PID" 2>/dev/null || true
  rm -f "$SERVER_DB"
  rm -rf "$AGENT_HOME" "$HOSTD_BIN_DIR"
}
trap cleanup EXIT

find_or_build_hostd_bin() {
  local name="$1"

  if command -v "$name" >/dev/null 2>&1; then
    command -v "$name"
    return 0
  fi

  if [ -x "/usr/local/bin/$name" ]; then
    echo "/usr/local/bin/$name"
    return 0
  fi

  if [ -x "${VMSAN_DIR}/bin/$name" ]; then
    echo "${VMSAN_DIR}/bin/$name"
    return 0
  fi

  if ! command -v go >/dev/null 2>&1; then
    return 1
  fi

  mkdir -p "$HOSTD_BIN_DIR"
  (
    cd "${REPO_ROOT}/hostd"
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o "${HOSTD_BIN_DIR}/${name}" "./cmd/${name}"
  ) >/dev/null 2>&1

  if [ -x "${HOSTD_BIN_DIR}/${name}" ]; then
    echo "${HOSTD_BIN_DIR}/${name}"
    return 0
  fi

  return 1
}

wait_for_host_count() {
  local expected="$1" timeout="${2:-20}"
  local elapsed=0
  while [ "$elapsed" -lt "$timeout" ]; do
    local count
    count=$(curl -sf "${SERVER_URL}/api/v1/hosts" 2>/dev/null | jq 'length' 2>/dev/null || echo "0")
    if [ "$count" = "$expected" ]; then
      return 0
    fi
    sleep 1
    elapsed=$((elapsed + 1))
  done
  return 1
}

wait_for_gateway_vm_count() {
  local minimum="$1" timeout="${2:-30}"
  local elapsed=0
  while [ "$elapsed" -lt "$timeout" ]; do
    local count
    count=$(gateway_rpc "status" | jq -r '.vms // 0' 2>/dev/null || echo "0")
    if [ "$count" -ge "$minimum" ] 2>/dev/null; then
      return 0
    fi
    sleep 1
    elapsed=$((elapsed + 1))
  done
  return 1
}

echo "================================================================"
echo "  E2E Test 10: Control Plane"
echo "================================================================"

SERVER_BIN=$(find_or_build_hostd_bin "vmsan-server" || echo "")
AGENT_BIN=$(find_or_build_hostd_bin "vmsan-agent-host" || echo "")

if [ -z "$SERVER_BIN" ] || [ -z "$AGENT_BIN" ]; then
  skip_test "vmsan-server or vmsan-agent-host binary not available"
  print_summary
  exit 0
fi

# ---------------------------------------------------------------------------
# Test: Server starts and responds
# ---------------------------------------------------------------------------
section "Server status"

"$SERVER_BIN" --listen 127.0.0.1:16443 --db "$SERVER_DB" >/tmp/vmsan-e2e-server.log 2>&1 &
SERVER_PID=$!

if wait_for_http "${SERVER_URL}/api/v1/status" 15; then
  STATUS_JSON=$(curl -sf "${SERVER_URL}/api/v1/status" 2>/dev/null || echo "")
  STATUS_OK=$(echo "$STATUS_JSON" | jq -r '.ok // false' 2>/dev/null || echo "false")
  assert_eq "$STATUS_OK" "true" "server status endpoint returns ok=true"
else
  assert_eq "false" "true" "server status endpoint becomes ready"
fi

# ---------------------------------------------------------------------------
# Test: hosts add generates a join command
# ---------------------------------------------------------------------------
section "hosts add"

HOSTS_ADD_OUT=$(run_vmsan_env VMSAN_SERVER_URL="$SERVER_URL" hosts add worker-1 2>&1 || echo "")
assert_contains "$HOSTS_ADD_OUT" "vmsan agent join" "hosts add prints a join command"

TOKEN=$(echo "$HOSTS_ADD_OUT" | grep -oE -- '--token [^ ]+' | awk '{print $2}' | head -n1 || true)
assert_not_empty "$TOKEN" "hosts add output includes a join token"

# ---------------------------------------------------------------------------
# Test: Agent join and single-use token
# ---------------------------------------------------------------------------
section "Agent join"

mkdir -p "$AGENT_HOME"
JOIN_OUT=$(HOME="$AGENT_HOME" "$AGENT_BIN" join --server "$SERVER_URL" --token "$TOKEN" --name worker-1 2>&1 || echo "")
assert_contains "$JOIN_OUT" "joined successfully" "agent join succeeds with fresh token"

if wait_for_host_count 1 10; then
  HOST_NAME=$(curl -sf "${SERVER_URL}/api/v1/hosts" 2>/dev/null | jq -r '.[0].name // empty' || echo "")
  assert_eq "$HOST_NAME" "worker-1" "joined host appears in server host list"
else
  assert_eq "0" "1" "server registers the joined host"
fi

REUSE_OUT=$(HOME="$AGENT_HOME" "$AGENT_BIN" join --server "$SERVER_URL" --token "$TOKEN" --name worker-2 2>&1 || true)
if echo "$REUSE_OUT" | grep -qi "invalid\|expired\|401"; then
  assert_eq "rejected" "rejected" "join token is single-use"
else
  assert_eq "accepted" "rejected" "join token is single-use"
fi

# ---------------------------------------------------------------------------
# Test: hosts list/check CLI
# ---------------------------------------------------------------------------
section "hosts list/check"

HOSTS_LIST_OUT=$(run_vmsan_env VMSAN_SERVER_URL="$SERVER_URL" hosts list 2>&1 || echo "")
assert_contains "$HOSTS_LIST_OUT" "worker-1" "hosts list shows the joined host"

HOSTS_CHECK_OUT=$(run_vmsan_env VMSAN_SERVER_URL="$SERVER_URL" hosts check worker-1 2>&1 || echo "")
assert_contains "$HOSTS_CHECK_OUT" "worker-1" "hosts check returns joined host details"

# ---------------------------------------------------------------------------
# Test: Remote create dispatches through server + agent
# ---------------------------------------------------------------------------
section "create --host"

HOME="$AGENT_HOME" VMSAN_DIR="$VMSAN_DIR" "$AGENT_BIN" >/tmp/vmsan-e2e-agent.log 2>&1 &
AGENT_PID=$!
sleep 3

CREATE_REMOTE_OUT=$(run_vmsan_env VMSAN_SERVER_URL="$SERVER_URL" create --host worker-1 --runtime base --vcpus 1 --memory 128 2>&1 || echo "")
assert_not_empty "$CREATE_REMOTE_OUT" "create --host produces output"

SERVER_VM_COUNT=$(curl -sf "${SERVER_URL}/api/v1/vms" 2>/dev/null | jq 'length' 2>/dev/null || echo "0")
if [ "$SERVER_VM_COUNT" -ge 1 ] 2>/dev/null; then
  mapfile -t REMOTE_VM_IDS < <(curl -sf "${SERVER_URL}/api/v1/vms" 2>/dev/null | jq -r '.[].id')
  assert_eq "queued" "queued" "server records remote VM creation"
else
  assert_eq "missing" "queued" "server records remote VM creation"
fi

if wait_for_gateway_vm_count 1 20; then
  assert_eq "created" "created" "agent sync creates a VM via the local gateway"
else
  assert_eq "missing" "created" "agent sync creates a VM via the local gateway"
fi

# ---------------------------------------------------------------------------
# Test: hosts remove deletes the registration
# ---------------------------------------------------------------------------
section "hosts remove"

REMOVE_OUT=$(run_vmsan_env VMSAN_SERVER_URL="$SERVER_URL" hosts remove worker-1 2>&1 || echo "")
assert_contains "$REMOVE_OUT" "removed" "hosts remove deletes the host"

if wait_for_host_count 0 10; then
  assert_eq "gone" "gone" "host registration removed from server"
else
  assert_eq "present" "gone" "host registration removed from server"
fi

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
print_summary
