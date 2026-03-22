#!/usr/bin/env bash
set -euo pipefail

# =============================================================================
# E2E Test: Gateway Process Supervision
# Tests gateway systemd restart, VM survives gateway restart,
# health endpoint, and process supervision recovery.
#
# Requires: KVM host, vmsan installed from feat/platform-multihost, jq, root
# Usage: sudo bash tests/e2e/test-gateway-supervision.sh
# =============================================================================

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/lib/helpers.sh"

VM_IDS=()

cleanup() {
  echo ""
  echo "--- Cleanup ---"
  for id in "${VM_IDS[@]}"; do
    [ -n "$id" ] && remove_vm "$id"
  done
  # Ensure gateway is running after test
  sudo systemctl start vmsan-gateway 2>/dev/null || true
}
trap cleanup EXIT

echo "================================================================"
echo "  E2E Test: Gateway Process Supervision"
echo "================================================================"

# ---------------------------------------------------------------------------
# Test: Gateway systemd service is active
# ---------------------------------------------------------------------------
echo ""
echo "--- Test: Gateway systemd service is active ---"

GW_STATUS=$(systemctl is-active vmsan-gateway 2>/dev/null || echo "inactive")
assert_eq "$GW_STATUS" "active" "gateway systemd service is active"

# ---------------------------------------------------------------------------
# Test: Gateway health endpoint responds
# ---------------------------------------------------------------------------
echo ""
echo "--- Test: Gateway health endpoint ---"

# The gateway exposes a health check on the Unix socket
if wait_for_gateway 10; then
  echo -e "${GREEN}PASS${NC}: gateway health endpoint responds"
  TESTS_PASSED=$((TESTS_PASSED + 1))
else
  echo -e "${RED}FAIL${NC}: gateway health endpoint did not respond within 10s"
  TESTS_FAILED=$((TESTS_FAILED + 1))
fi

# ---------------------------------------------------------------------------
# Test: VM survives gateway restart
# ---------------------------------------------------------------------------
echo ""
echo "--- Test: VM survives gateway restart ---"

# Create a VM before restart
VM_ID=$(create_vm --runtime base --vcpus 1 --memory 256)
assert_not_empty "$VM_ID" "pre-restart: VM created"

if [ -n "$VM_ID" ]; then
  # Verify VM is running
  VM_STATE_BEFORE=$(vmsan list --json 2>/dev/null | jq -r --arg id "$VM_ID" '.[] | select(.id==$id) | .state // empty' || true)
  assert_eq "$VM_STATE_BEFORE" "running" "pre-restart: VM is running"

  # Restart gateway
  echo "  Restarting vmsan-gateway..."
  sudo systemctl restart vmsan-gateway 2>/dev/null

  # Wait for gateway to come back up
  sleep 3
  wait_for_gateway 15

  # Verify VM is still running after gateway restart
  VM_STATE_AFTER=$(vmsan list --json 2>/dev/null | jq -r --arg id "$VM_ID" '.[] | select(.id==$id) | .state // empty' || true)
  assert_eq "$VM_STATE_AFTER" "running" "post-restart: VM still running after gateway restart"

  # Verify exec still works (agent inside VM is still reachable)
  EXEC_RESULT=$(sudo vmsan exec "$VM_ID" -- echo "alive-after-restart" 2>/dev/null || true)
  assert_contains "$EXEC_RESULT" "alive-after-restart" "post-restart: exec works after gateway restart"

  remove_vm "$VM_ID"
fi

# ---------------------------------------------------------------------------
# Test: Gateway recovers from SIGKILL
# ---------------------------------------------------------------------------
echo ""
echo "--- Test: Gateway recovers from SIGKILL ---"

# Get current gateway PID
GW_PID=$(systemctl show vmsan-gateway --property=MainPID --value 2>/dev/null || echo "0")
assert_not_empty "$GW_PID" "gateway has a PID"

if [ "$GW_PID" != "0" ] && [ -n "$GW_PID" ]; then
  echo "  Sending SIGKILL to gateway (PID $GW_PID)..."
  sudo kill -9 "$GW_PID" 2>/dev/null || true

  # systemd should restart it (Restart=always or on-failure)
  echo -n "  Waiting for systemd to restart gateway..."
  elapsed=0
  while [ $elapsed -lt 15 ]; do
    NEW_STATUS=$(systemctl is-active vmsan-gateway 2>/dev/null || echo "inactive")
    if [ "$NEW_STATUS" = "active" ]; then
      NEW_PID=$(systemctl show vmsan-gateway --property=MainPID --value 2>/dev/null || echo "0")
      if [ "$NEW_PID" != "0" ] && [ "$NEW_PID" != "$GW_PID" ]; then
        echo " restarted (new PID $NEW_PID, ${elapsed}s)"
        echo -e "${GREEN}PASS${NC}: gateway recovered from SIGKILL (new PID: $NEW_PID)"
        TESTS_PASSED=$((TESTS_PASSED + 1))
        break
      fi
    fi
    sleep 1
    elapsed=$((elapsed + 1))
  done

  if [ $elapsed -ge 15 ]; then
    echo " TIMEOUT"
    echo -e "${RED}FAIL${NC}: gateway did not restart after SIGKILL within 15s"
    TESTS_FAILED=$((TESTS_FAILED + 1))
    # Try to start it manually for subsequent tests
    sudo systemctl start vmsan-gateway 2>/dev/null || true
  fi
fi

# ---------------------------------------------------------------------------
# Test: Gateway socket recreated after restart
# ---------------------------------------------------------------------------
echo ""
echo "--- Test: Gateway socket recreated after restart ---"

# Verify the socket exists and is functional
if [ -S /run/vmsan/gateway.sock ]; then
  echo -e "${GREEN}PASS${NC}: gateway socket exists at /run/vmsan/gateway.sock"
  TESTS_PASSED=$((TESTS_PASSED + 1))
else
  echo -e "${RED}FAIL${NC}: gateway socket not found at /run/vmsan/gateway.sock"
  TESTS_FAILED=$((TESTS_FAILED + 1))
fi

if wait_for_gateway 10; then
  echo -e "${GREEN}PASS${NC}: gateway socket responds after restart"
  TESTS_PASSED=$((TESTS_PASSED + 1))
else
  echo -e "${RED}FAIL${NC}: gateway socket not responding after restart"
  TESTS_FAILED=$((TESTS_FAILED + 1))
fi

# ---------------------------------------------------------------------------
# Test: Multiple VMs survive gateway restart
# ---------------------------------------------------------------------------
echo ""
echo "--- Test: Multiple VMs survive gateway restart ---"

VM1=$(create_vm --runtime base --vcpus 1 --memory 128)
VM2=$(create_vm --runtime base --vcpus 1 --memory 128)

if [ -n "$VM1" ] && [ -n "$VM2" ]; then
  # Restart gateway
  sudo systemctl restart vmsan-gateway 2>/dev/null
  sleep 3
  wait_for_gateway 15

  # Both VMs should still be in the list
  VM1_EXISTS=$(vmsan list --json 2>/dev/null | jq --arg id "$VM1" '[.[] | select(.id==$id)] | length' || echo "0")
  VM2_EXISTS=$(vmsan list --json 2>/dev/null | jq --arg id "$VM2" '[.[] | select(.id==$id)] | length' || echo "0")

  assert_eq "$VM1_EXISTS" "1" "multi-VM: VM1 still listed after gateway restart"
  assert_eq "$VM2_EXISTS" "1" "multi-VM: VM2 still listed after gateway restart"
fi

[ -n "$VM1" ] && remove_vm "$VM1"
[ -n "$VM2" ] && remove_vm "$VM2"

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
print_summary
