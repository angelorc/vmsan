#!/usr/bin/env bash
# =============================================================================
# Shared helpers for vmsan E2E tests
# Source this file at the top of every test script.
# =============================================================================

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
NC='\033[0m'

TESTS_PASSED=0
TESTS_FAILED=0
TESTS_SKIPPED=0

resolve_e2e_user() {
  if [ -n "${VMSAN_E2E_USER:-}" ]; then
    echo "$VMSAN_E2E_USER"
    return
  fi

  if [ "$(id -u)" -ne 0 ]; then
    id -un
    return
  fi

  if [ -n "${SUDO_USER:-}" ] && [ "${SUDO_USER}" != "root" ]; then
    echo "$SUDO_USER"
    return
  fi

  local owner
  owner=$(stat -c '%U' "$PWD" 2>/dev/null || echo "")
  if [ -n "$owner" ] && [ "$owner" != "root" ]; then
    echo "$owner"
    return
  fi

  echo "root"
}

resolve_user_home() {
  local user="$1"
  if [ "$user" = "$(id -un)" ]; then
    echo "$HOME"
    return
  fi

  getent passwd "$user" 2>/dev/null | cut -d: -f6
}

VMSAN_E2E_USER="$(resolve_e2e_user)"
VMSAN_E2E_HOME="${VMSAN_E2E_HOME:-$(resolve_user_home "$VMSAN_E2E_USER")}"
[ -n "$VMSAN_E2E_HOME" ] || VMSAN_E2E_HOME="$HOME"

VMSAN_DIR="${VMSAN_DIR:-${VMSAN_E2E_HOME}/.vmsan}"
VMSAN_STATE_DB="${VMSAN_DIR}/state.db"
VMSAN_EXPECT_NONROOT=0
if [ "$VMSAN_E2E_USER" != "root" ]; then
  VMSAN_EXPECT_NONROOT=1
fi

run_as_cli_user() {
  local -a env_args
  env_args=(
    "HOME=${VMSAN_E2E_HOME}"
    "USER=${VMSAN_E2E_USER}"
    "LOGNAME=${VMSAN_E2E_USER}"
    "VMSAN_DIR=${VMSAN_DIR}"
    "PATH=${PATH}"
  )

  if [ "$VMSAN_E2E_USER" = "$(id -un)" ]; then
    env "${env_args[@]}" "$@"
  else
    sudo -H -u "$VMSAN_E2E_USER" env "${env_args[@]}" "$@"
  fi
}

run_as_cli_user_env() {
  local -a custom_env
  custom_env=()

  while [ $# -gt 0 ] && [[ "$1" == *=* ]]; do
    custom_env+=("$1")
    shift
  done

  local -a env_args
  env_args=(
    "${custom_env[@]}"
    "HOME=${VMSAN_E2E_HOME}"
    "USER=${VMSAN_E2E_USER}"
    "LOGNAME=${VMSAN_E2E_USER}"
    "VMSAN_DIR=${VMSAN_DIR}"
    "PATH=${PATH}"
  )

  if [ "$VMSAN_E2E_USER" = "$(id -un)" ]; then
    env "${env_args[@]}" "$@"
  else
    sudo -H -u "$VMSAN_E2E_USER" env "${env_args[@]}" "$@"
  fi
}

run_as_cli_shell() {
  local cmd="$1"

  if [ "$VMSAN_E2E_USER" = "$(id -un)" ]; then
    env \
      HOME="${VMSAN_E2E_HOME}" \
      USER="${VMSAN_E2E_USER}" \
      LOGNAME="${VMSAN_E2E_USER}" \
      VMSAN_DIR="${VMSAN_DIR}" \
      PATH="${PATH}" \
      bash -lc "$cmd"
  else
    sudo -H -u "$VMSAN_E2E_USER" \
      env \
      HOME="${VMSAN_E2E_HOME}" \
      USER="${VMSAN_E2E_USER}" \
      LOGNAME="${VMSAN_E2E_USER}" \
      VMSAN_DIR="${VMSAN_DIR}" \
      PATH="${PATH}" \
      bash -lc "$cmd"
  fi
}

run_vmsan() {
  run_as_cli_user vmsan "$@"
}

run_vmsan_env() {
  local -a env_args
  env_args=()

  while [ $# -gt 0 ] && [[ "$1" == *=* ]]; do
    env_args+=("$1")
    shift
  done

  run_as_cli_user_env "${env_args[@]}" vmsan "$@"
}

cli_user_groups() {
  run_as_cli_user id -nG 2>/dev/null || true
}

gateway_rpc() {
  # Gateway uses gRPC (not JSON-RPC). Map old RPC method names to CLI commands.
  local method="$1"
  case "$method" in
    ping)   run_vmsan doctor >/dev/null 2>&1 && echo '{"ok":true}' ;;
    health) run_vmsan doctor >/dev/null 2>&1 && echo '{"ok":true}' ;;
    status) run_vmsan --json list 2>/dev/null ;;
    *)      echo '{"error":"unknown method"}' ;;
  esac
}

gateway_rpc_cli() {
  local method="$1"
  gateway_rpc "$method"
}

sqlite_query() {
  local sql="$1" db="${2:-$VMSAN_STATE_DB}"
  [ -f "$db" ] || return 1
  command -v python3 >/dev/null 2>&1 || return 1

  python3 - "$db" "$sql" <<'PY'
import sqlite3
import sys

db_path, sql = sys.argv[1], sys.argv[2]
conn = sqlite3.connect(db_path)
try:
    rows = conn.execute(sql).fetchall()
finally:
    conn.close()

for row in rows:
    if len(row) == 1:
        print("" if row[0] is None else row[0])
    else:
        print("\t".join("" if value is None else str(value) for value in row))
PY
}

get_vm_state_json() {
  local id="$1"
  local state_file="${VMSAN_DIR}/vms/${id}.json"

  if [ -f "$state_file" ]; then
    cat "$state_file"
    return 0
  fi

  if [ -f "$VMSAN_STATE_DB" ] && command -v python3 >/dev/null 2>&1; then
    python3 - "$VMSAN_STATE_DB" "$id" <<'PY'
import sqlite3
import sys

db_path, vm_id = sys.argv[1], sys.argv[2]
conn = sqlite3.connect(db_path)
try:
    row = conn.execute("SELECT state_json FROM vms WHERE id = ?", (vm_id,)).fetchone()
finally:
    conn.close()

print(row[0] if row else "")
PY
  fi
}

get_vm_state_field() {
  local id="$1" jq_expr="$2"
  local state_json
  state_json="$(get_vm_state_json "$id")"
  if [ -n "$state_json" ]; then
    echo "$state_json" | jq -r "${jq_expr} // empty"
  fi
}

# ---------------------------------------------------------------------------
# Assertions
# ---------------------------------------------------------------------------

assert_eq() {
  local actual="$1" expected="$2" msg="$3"
  if [ "$actual" = "$expected" ]; then
    echo -e "  ${GREEN}PASS${NC}: $msg"
    TESTS_PASSED=$((TESTS_PASSED + 1))
  else
    echo -e "  ${RED}FAIL${NC}: $msg (expected: '$expected', got: '$actual')"
    TESTS_FAILED=$((TESTS_FAILED + 1))
  fi
}

assert_not_empty() {
  local value="$1" msg="$2"
  if [ -n "$value" ] && [ "$value" != "null" ]; then
    echo -e "  ${GREEN}PASS${NC}: $msg ($value)"
    TESTS_PASSED=$((TESTS_PASSED + 1))
  else
    echo -e "  ${RED}FAIL${NC}: $msg (empty or null)"
    TESTS_FAILED=$((TESTS_FAILED + 1))
  fi
}

assert_empty() {
  local value="$1" msg="$2"
  if [ -z "$value" ] || [ "$value" = "null" ]; then
    echo -e "  ${GREEN}PASS${NC}: $msg"
    TESTS_PASSED=$((TESTS_PASSED + 1))
  else
    echo -e "  ${RED}FAIL${NC}: $msg (expected empty, got: '$value')"
    TESTS_FAILED=$((TESTS_FAILED + 1))
  fi
}

assert_contains() {
  local haystack="$1" needle="$2" msg="$3"
  if echo "$haystack" | grep -q "$needle"; then
    echo -e "  ${GREEN}PASS${NC}: $msg"
    TESTS_PASSED=$((TESTS_PASSED + 1))
  else
    echo -e "  ${RED}FAIL${NC}: $msg (does not contain: '$needle')"
    TESTS_FAILED=$((TESTS_FAILED + 1))
  fi
}

assert_not_contains() {
  local haystack="$1" needle="$2" msg="$3"
  if ! echo "$haystack" | grep -q "$needle"; then
    echo -e "  ${GREEN}PASS${NC}: $msg"
    TESTS_PASSED=$((TESTS_PASSED + 1))
  else
    echo -e "  ${RED}FAIL${NC}: $msg (unexpectedly contains: '$needle')"
    TESTS_FAILED=$((TESTS_FAILED + 1))
  fi
}

assert_gt() {
  local actual="$1" threshold="$2" msg="$3"
  if [ "$actual" -gt "$threshold" ] 2>/dev/null; then
    echo -e "  ${GREEN}PASS${NC}: $msg ($actual > $threshold)"
    TESTS_PASSED=$((TESTS_PASSED + 1))
  else
    echo -e "  ${RED}FAIL${NC}: $msg (expected > $threshold, got: $actual)"
    TESTS_FAILED=$((TESTS_FAILED + 1))
  fi
}

assert_exit_zero() {
  local msg="$1"
  shift
  if "$@" >/dev/null 2>&1; then
    echo -e "  ${GREEN}PASS${NC}: $msg"
    TESTS_PASSED=$((TESTS_PASSED + 1))
  else
    echo -e "  ${RED}FAIL${NC}: $msg (exit code: $?)"
    TESTS_FAILED=$((TESTS_FAILED + 1))
  fi
}

assert_exit_nonzero() {
  local msg="$1"
  shift
  if ! "$@" >/dev/null 2>&1; then
    echo -e "  ${GREEN}PASS${NC}: $msg"
    TESTS_PASSED=$((TESTS_PASSED + 1))
  else
    echo -e "  ${RED}FAIL${NC}: $msg (expected non-zero exit, got 0)"
    TESTS_FAILED=$((TESTS_FAILED + 1))
  fi
}

skip_test() {
  local msg="$1"
  echo -e "  ${YELLOW}SKIP${NC}: $msg"
  TESTS_SKIPPED=$((TESTS_SKIPPED + 1))
}

# ---------------------------------------------------------------------------
# VM operations
# ---------------------------------------------------------------------------

# Create a VM and return its ID. Tracks in VM_IDS for cleanup.
# Usage: VM_ID=$(create_vm --runtime base --vcpus 1 --memory 256)
create_vm() {
  local out
  out=$(run_vmsan create "$@" 2>&1)
  local rc=$?
  local id
  # Extract vm-XXXXXXXX pattern from output (works in all output modes)
  id=$(echo "$out" | grep -oE 'vm-[0-9a-f]{8}' | head -n1)
  if [ -z "$id" ] || [ $rc -ne 0 ]; then
    echo "CREATE FAILED (rc=$rc): $out" >&2
    echo ""
    return 1
  fi
  VM_IDS+=("$id")
  echo "$id"
}

create_vm_env() {
  local -a env_args
  env_args=()

  while [ $# -gt 0 ] && [[ "$1" == *=* ]]; do
    env_args+=("$1")
    shift
  done

  local out
  out=$(run_vmsan_env "${env_args[@]}" create "$@" 2>&1)
  local rc=$?
  local id
  id=$(echo "$out" | grep -oE 'vm-[0-9a-f]{8}' | head -n1)
  if [ -z "$id" ] || [ $rc -ne 0 ]; then
    echo "CREATE FAILED (rc=$rc): $out" >&2
    echo ""
    return 1
  fi
  VM_IDS+=("$id")
  echo "$id"
}

# Stop and remove a VM, removing it from VM_IDS tracking array
remove_vm() {
  local id="$1"
  run_vmsan stop "$id" 2>/dev/null || true
  run_vmsan remove "$id" 2>/dev/null || true
  # Remove from tracking array
  local new_ids=()
  for existing in "${VM_IDS[@]}"; do
    [ "$existing" != "$id" ] && new_ids+=("$existing")
  done
  VM_IDS=("${new_ids[@]}")
}

# Force-remove a VM (stop + remove in one step)
force_remove_vm() {
  local id="$1"
  run_vmsan remove --force "$id" 2>/dev/null || true
  local new_ids=()
  for existing in "${VM_IDS[@]}"; do
    [ "$existing" != "$id" ] && new_ids+=("$existing")
  done
  VM_IDS=("${new_ids[@]}")
}

# ---------------------------------------------------------------------------
# Query VM state via vmsan list
#
# vmsan --json list outputs an evlog JSON event with shape:
#   {"event":"list", "count":N, "vms":[{"id":"vm-xxx","status":"running",...}]}
# ---------------------------------------------------------------------------

# Get the full JSON list output (last JSON line from stdout)
list_vms_json() {
  run_vmsan --json list 2>/dev/null | tail -1
}

# Get a field from a specific VM in the list
# Usage: get_vm_field <vmId> <field>
# Example: get_vm_field vm-abc123 status  →  "running"
get_vm_field() {
  local id="$1" field="$2"
  list_vms_json | jq -r --arg id "$id" --arg f "$field" \
    '.vms[]? | select(.id==$id) | .[$f] // empty'
}

# Count VMs with a given status
# Usage: count_vms_by_status running  →  2
count_vms_by_status() {
  local status="$1"
  list_vms_json | jq --arg s "$status" '[.vms[]? | select(.status==$s)] | length'
}

# Get total VM count
count_vms() {
  list_vms_json | jq '.count // 0'
}

# ---------------------------------------------------------------------------
# Wait helpers
# ---------------------------------------------------------------------------

wait_for_agent() {
  local ip="$1" timeout="${2:-30}"
  local elapsed=0
  echo -n "  Waiting for agent at ${ip}:9119..."
  while [ $elapsed -lt $timeout ]; do
    if curl -sf "http://${ip}:9119/health" >/dev/null 2>&1; then
      echo " ready (${elapsed}s)"
      return 0
    fi
    sleep 1
    elapsed=$((elapsed + 1))
  done
  echo " TIMEOUT after ${timeout}s"
  return 1
}

wait_for_gateway() {
  local timeout="${1:-10}"
  local elapsed=0
  echo -n "  Waiting for gateway..."
  while [ $elapsed -lt $timeout ]; do
    # Gateway uses gRPC — check via CLI or socket existence + systemd status
    if [ -S /run/vmsan/gateway.sock ] && run_vmsan --json list >/dev/null 2>&1; then
      echo " ready (${elapsed}s)"
      return 0
    fi
    sleep 1
    elapsed=$((elapsed + 1))
  done
  echo " TIMEOUT after ${timeout}s"
  return 1
}

wait_for_http() {
  local url="$1" timeout="${2:-30}"
  local elapsed=0
  echo -n "  Waiting for ${url}..."
  while [ $elapsed -lt $timeout ]; do
    if curl -sf "$url" >/dev/null 2>&1; then
      echo " ready (${elapsed}s)"
      return 0
    fi
    sleep 1
    elapsed=$((elapsed + 1))
  done
  echo " TIMEOUT after ${timeout}s"
  return 1
}

# Wait for a VM to appear in list with a specific status
wait_for_vm_status() {
  local id="$1" expected="$2" timeout="${3:-60}"
  local elapsed=0
  echo -n "  Waiting for $id to be $expected..."
  while [ $elapsed -lt $timeout ]; do
    local status
    status=$(get_vm_field "$id" "status")
    if [ "$status" = "$expected" ]; then
      echo " done (${elapsed}s)"
      return 0
    fi
    sleep 1
    elapsed=$((elapsed + 1))
  done
  echo " TIMEOUT (current: $(get_vm_field "$id" "status"))"
  return 1
}

# ---------------------------------------------------------------------------
# Network helpers
# ---------------------------------------------------------------------------

# Get guest IP for a VM from its state file
get_guest_ip() {
  local id="$1"
  get_vm_state_field "$id" '.network.guestIp'
}

get_host_ip() {
  local id="$1"
  get_vm_state_field "$id" '.network.hostIp'
}

get_mesh_ip() {
  local id="$1"
  get_vm_state_field "$id" '.network.meshIp'
}

# Get network namespace name for a VM
get_netns() {
  local id="$1"
  echo "vmsan-${id}"
}

# Check if a network namespace exists
netns_exists() {
  local ns="$1"
  ip netns list 2>/dev/null | grep -q "^${ns}" 2>/dev/null
}

# Run a command inside a network namespace
ns_exec() {
  local ns="$1"
  shift
  sudo ip netns exec "$ns" "$@"
}

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------

print_summary() {
  echo ""
  echo "-------------------------------------"
  echo -e "  Passed:  ${GREEN}${TESTS_PASSED}${NC}"
  [ "$TESTS_FAILED" -gt 0 ] && echo -e "  Failed:  ${RED}${TESTS_FAILED}${NC}" || echo -e "  Failed:  ${TESTS_FAILED}"
  [ "$TESTS_SKIPPED" -gt 0 ] && echo -e "  Skipped: ${YELLOW}${TESTS_SKIPPED}${NC}" || true
  echo "-------------------------------------"
  [ "$TESTS_FAILED" -eq 0 ] || exit 1
}

section() {
  echo ""
  echo -e "${CYAN}--- $1 ---${NC}"
}
