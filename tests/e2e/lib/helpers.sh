#!/usr/bin/env bash
# Shared helpers for E2E tests

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m'

TESTS_PASSED=0
TESTS_FAILED=0

assert_exit_code() {
  local expected="$1" msg="$2"
  if [ "${PIPESTATUS[0]:-0}" -eq "$expected" ]; then
    echo -e "${GREEN}PASS${NC}: $msg"
    TESTS_PASSED=$((TESTS_PASSED + 1))
  else
    echo -e "${RED}FAIL${NC}: $msg (exit code: ${PIPESTATUS[0]:-?})"
    TESTS_FAILED=$((TESTS_FAILED + 1))
  fi
}

assert_eq() {
  local actual="$1" expected="$2" msg="$3"
  if [ "$actual" = "$expected" ]; then
    echo -e "${GREEN}PASS${NC}: $msg"
    TESTS_PASSED=$((TESTS_PASSED + 1))
  else
    echo -e "${RED}FAIL${NC}: $msg (expected: $expected, got: $actual)"
    TESTS_FAILED=$((TESTS_FAILED + 1))
  fi
}

assert_not_empty() {
  local value="$1" msg="$2"
  if [ -n "$value" ] && [ "$value" != "null" ]; then
    echo -e "${GREEN}PASS${NC}: $msg ($value)"
    TESTS_PASSED=$((TESTS_PASSED + 1))
  else
    echo -e "${RED}FAIL${NC}: $msg (empty or null)"
    TESTS_FAILED=$((TESTS_FAILED + 1))
  fi
}

assert_empty() {
  local value="$1" msg="$2"
  if [ -z "$value" ] || [ "$value" = "null" ]; then
    echo -e "${GREEN}PASS${NC}: $msg"
    TESTS_PASSED=$((TESTS_PASSED + 1))
  else
    echo -e "${RED}FAIL${NC}: $msg (expected empty, got: $value)"
    TESTS_FAILED=$((TESTS_FAILED + 1))
  fi
}

assert_contains() {
  local haystack="$1" needle="$2" msg="$3"
  if echo "$haystack" | grep -q "$needle"; then
    echo -e "${GREEN}PASS${NC}: $msg"
    TESTS_PASSED=$((TESTS_PASSED + 1))
  else
    echo -e "${RED}FAIL${NC}: $msg (does not contain: $needle)"
    TESTS_FAILED=$((TESTS_FAILED + 1))
  fi
}

assert_not_contains() {
  local haystack="$1" needle="$2" msg="$3"
  if ! echo "$haystack" | grep -q "$needle"; then
    echo -e "${GREEN}PASS${NC}: $msg"
    TESTS_PASSED=$((TESTS_PASSED + 1))
  else
    echo -e "${RED}FAIL${NC}: $msg (unexpectedly contains: $needle)"
    TESTS_FAILED=$((TESTS_FAILED + 1))
  fi
}

assert_gt() {
  local actual="$1" threshold="$2" msg="$3"
  if [ "$actual" -gt "$threshold" ] 2>/dev/null; then
    echo -e "${GREEN}PASS${NC}: $msg ($actual > $threshold)"
    TESTS_PASSED=$((TESTS_PASSED + 1))
  else
    echo -e "${RED}FAIL${NC}: $msg (expected > $threshold, got: $actual)"
    TESTS_FAILED=$((TESTS_FAILED + 1))
  fi
}

# Extract vmId from vmsan JSON output (may contain multiple JSON lines)
extract_vm_id() {
  local output="$1"
  printf '%s\n' "$output" \
    | jq -Rr 'fromjson? | .vmId? // empty' \
    | head -n1
}

# Create a VM and return its ID; adds to VM_IDS array for cleanup
create_vm() {
  local args="$*"
  local out
  out=$(sudo vmsan create $args --json 2>&1)
  local id
  id=$(extract_vm_id "$out")
  if [ -z "$id" ]; then
    echo ""
    return 1
  fi
  VM_IDS+=("$id")
  echo "$id"
}

# Stop and remove a VM, removing it from VM_IDS
remove_vm() {
  local id="$1"
  sudo vmsan stop "$id" 2>/dev/null || true
  sudo vmsan remove "$id" 2>/dev/null || true
  # Remove from tracking array
  local new_ids=()
  for existing in "${VM_IDS[@]}"; do
    [ "$existing" != "$id" ] && new_ids+=("$existing")
  done
  VM_IDS=("${new_ids[@]}")
}

wait_for_agent() {
  local ip="$1" timeout="${2:-30}"
  local elapsed=0
  echo -n "Waiting for agent at ${ip}:9119..."
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
  echo -n "Waiting for gateway..."
  while [ $elapsed -lt $timeout ]; do
    if echo '{"method":"ping"}' | socat - UNIX-CONNECT:/run/vmsan/gateway.sock >/dev/null 2>&1; then
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
  echo -n "Waiting for ${url}..."
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

print_summary() {
  echo ""
  echo "-------------------------------------"
  echo -e "  Passed: ${GREEN}${TESTS_PASSED}${NC}"
  echo -e "  Failed: ${RED}${TESTS_FAILED}${NC}"
  echo "-------------------------------------"
  [ "$TESTS_FAILED" -eq 0 ] || exit 1
}
