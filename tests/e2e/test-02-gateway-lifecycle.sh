#!/usr/bin/env bash
set -euo pipefail

# =============================================================================
# E2E Test 02: Gateway Lifecycle
# Tests gateway systemd service, Unix socket, health endpoint,
# restart recovery, and SIGKILL auto-restart.
#
# Requires: KVM host, vmsan installed, systemd, socat
# Usage: sudo bash tests/e2e/test-02-gateway-lifecycle.sh
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
  wait_for_gateway 10 || true
}
trap cleanup EXIT

echo "================================================================"
echo "  E2E Test 02: Gateway Lifecycle"
echo "================================================================"

# ---------------------------------------------------------------------------
# Test: Gateway systemd service is active
# ---------------------------------------------------------------------------
section "Gateway systemd service is active"

GW_STATUS=$(systemctl is-active vmsan-gateway 2>/dev/null || echo "inactive")
if [ "$GW_STATUS" != "active" ]; then
  echo "  Gateway not running, starting..."
  sudo systemctl start vmsan-gateway 2>/dev/null || true
  sleep 2
  GW_STATUS=$(systemctl is-active vmsan-gateway 2>/dev/null || echo "inactive")
fi
assert_eq "$GW_STATUS" "active" "gateway systemd service is active"

# ---------------------------------------------------------------------------
# Test: Gateway socket exists
# ---------------------------------------------------------------------------
section "Gateway Unix socket"

if [ -S /run/vmsan/gateway.sock ]; then
  assert_eq "yes" "yes" "socket exists at /run/vmsan/gateway.sock"
else
  assert_eq "no" "yes" "socket exists at /run/vmsan/gateway.sock"
fi

# ---------------------------------------------------------------------------
# Test: Gateway responds to ping
# ---------------------------------------------------------------------------
section "Gateway ping"

PING_RESULT=$(gateway_rpc_cli "ping" || gateway_rpc "ping" || echo "")
if echo "$PING_RESULT" | jq -e '.ok == true' >/dev/null 2>&1; then
  assert_eq "true" "true" "gateway ping returns ok=true"
else
  assert_eq "false" "true" "gateway ping returns ok=true"
fi

# Extract version from ping response
GW_VERSION=$(echo "$PING_RESULT" | jq -r '.version // empty' 2>/dev/null || echo "")
if [ -n "$GW_VERSION" ]; then
  assert_not_empty "$GW_VERSION" "gateway reports version"
fi

# ---------------------------------------------------------------------------
# Test: Gateway health endpoint
# ---------------------------------------------------------------------------
section "Gateway health endpoint"

HEALTH_RESULT=$(gateway_rpc_cli "health" || gateway_rpc "health" || echo "")
if echo "$HEALTH_RESULT" | jq -e '.ok == true' >/dev/null 2>&1; then
  assert_eq "true" "true" "gateway health returns ok=true"
else
  assert_eq "false" "true" "gateway health returns ok=true"
fi

# ---------------------------------------------------------------------------
# Test: Socket permissions allow CLI user access
# ---------------------------------------------------------------------------
section "Gateway socket permissions"

SOCKET_GROUP=$(stat -c '%G' /run/vmsan/gateway.sock 2>/dev/null || echo "")
assert_eq "$SOCKET_GROUP" "vmsan" "gateway socket group is vmsan"

SOCKET_MODE=$(stat -c '%a' /run/vmsan/gateway.sock 2>/dev/null || echo "")
assert_eq "$SOCKET_MODE" "660" "gateway socket mode is 660"

if [ "$VMSAN_EXPECT_NONROOT" -eq 1 ]; then
  CLI_PING=$(gateway_rpc_cli "ping" || echo "")
  if echo "$CLI_PING" | jq -e '.ok == true' >/dev/null 2>&1; then
    assert_eq "true" "true" "CLI user can connect to gateway socket"
  else
    assert_eq "false" "true" "CLI user can connect to gateway socket"
  fi
else
  skip_test "non-root CLI user could not be resolved"
fi

# ---------------------------------------------------------------------------
# Test: VM survives gateway restart
# ---------------------------------------------------------------------------
section "VM survives gateway restart"

VM_ID=$(create_vm --runtime base --vcpus 1 --memory 128)
assert_not_empty "$VM_ID" "VM created before restart"

if [ -n "$VM_ID" ]; then
  # Verify VM is running
  sleep 5
  STATUS_BEFORE=$(get_vm_field "$VM_ID" "status")
  assert_eq "$STATUS_BEFORE" "running" "VM is running before restart"

  # Restart gateway
  echo "  Restarting vmsan-gateway..."
  sudo systemctl restart vmsan-gateway 2>/dev/null
  sleep 3
  wait_for_gateway 15

  # VM should still be running (Firecracker process is independent)
  STATUS_AFTER=$(get_vm_field "$VM_ID" "status")
  assert_eq "$STATUS_AFTER" "running" "VM still running after gateway restart"

  # Exec should work after gateway restart (agent is in the VM, not gateway)
  EXEC_OUT=$(run_vmsan exec "$VM_ID" -- echo "alive-after-restart" 2>/dev/null || echo "")
  assert_contains "$EXEC_OUT" "alive-after-restart" "exec works after gateway restart"

  remove_vm "$VM_ID"
fi

# ---------------------------------------------------------------------------
# Test: Gateway recovers from SIGKILL
# ---------------------------------------------------------------------------
section "Gateway recovers from SIGKILL"

GW_PID=$(systemctl show vmsan-gateway --property=MainPID --value 2>/dev/null || echo "0")
assert_not_empty "$GW_PID" "gateway has a PID"

if [ "$GW_PID" != "0" ] && [ -n "$GW_PID" ]; then
  echo "  Sending SIGKILL to gateway (PID $GW_PID)..."
  sudo kill -9 "$GW_PID" 2>/dev/null || true

  # systemd should restart it (Restart=on-failure in service file)
  echo -n "  Waiting for systemd to restart gateway..."
  elapsed=0
  restarted=false
  while [ $elapsed -lt 20 ]; do
    NEW_STATUS=$(systemctl is-active vmsan-gateway 2>/dev/null || echo "inactive")
    if [ "$NEW_STATUS" = "active" ]; then
      NEW_PID=$(systemctl show vmsan-gateway --property=MainPID --value 2>/dev/null || echo "0")
      if [ "$NEW_PID" != "0" ] && [ "$NEW_PID" != "$GW_PID" ]; then
        echo " restarted (new PID $NEW_PID, ${elapsed}s)"
        assert_eq "true" "true" "gateway recovered from SIGKILL (new PID: $NEW_PID)"
        restarted=true
        break
      fi
    fi
    sleep 1
    elapsed=$((elapsed + 1))
  done

  if [ "$restarted" = false ]; then
    echo " TIMEOUT"
    assert_eq "false" "true" "gateway recovered from SIGKILL within 20s"
    # Try to start manually for subsequent tests
    sudo systemctl start vmsan-gateway 2>/dev/null || true
    sleep 2
  fi
fi

# ---------------------------------------------------------------------------
# Test: Gateway socket recreated after restart
# ---------------------------------------------------------------------------
section "Socket recreated after restart"

wait_for_gateway 10
if [ -S /run/vmsan/gateway.sock ]; then
  assert_eq "yes" "yes" "socket recreated at /run/vmsan/gateway.sock"
else
  assert_eq "no" "yes" "socket recreated at /run/vmsan/gateway.sock"
fi

PING_OK=$(gateway_rpc_cli "ping" | jq -r '.ok // empty' 2>/dev/null || echo "")
[ -n "$PING_OK" ] || PING_OK=$(gateway_rpc "ping" | jq -r '.ok // empty' 2>/dev/null || echo "")
assert_eq "$PING_OK" "true" "gateway socket responds after restart"

# ---------------------------------------------------------------------------
# Test: Multiple VMs survive gateway restart
# ---------------------------------------------------------------------------
section "Multiple VMs survive gateway restart"

VM1=$(create_vm --runtime base --vcpus 1 --memory 128)
VM2=$(create_vm --runtime base --vcpus 1 --memory 128)

if [ -n "$VM1" ] && [ -n "$VM2" ]; then
  sleep 5

  # Restart gateway
  sudo systemctl restart vmsan-gateway 2>/dev/null
  sleep 3
  wait_for_gateway 15

  # Both VMs should still appear in list
  VM1_STATUS=$(get_vm_field "$VM1" "status")
  VM2_STATUS=$(get_vm_field "$VM2" "status")

  assert_eq "$VM1_STATUS" "running" "VM1 still running after gateway restart"
  assert_eq "$VM2_STATUS" "running" "VM2 still running after gateway restart"

  remove_vm "$VM1"
  remove_vm "$VM2"
else
  skip_test "could not create 2 VMs for multi-VM restart test"
fi

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
print_summary
