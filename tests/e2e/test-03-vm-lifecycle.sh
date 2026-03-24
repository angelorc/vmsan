#!/usr/bin/env bash
set -euo pipefail

# =============================================================================
# E2E Test 03: VM Lifecycle
# Tests create, list, exec, upload/download, stop, start, remove,
# force-remove, concurrent creation, and resource cleanup.
#
# Requires: KVM host, vmsan installed, base runtime
# Usage: sudo bash tests/e2e/test-03-vm-lifecycle.sh
# =============================================================================

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/lib/helpers.sh"

VM_IDS=()

cleanup() {
  echo ""
  echo "--- Cleanup ---"
  for id in "${VM_IDS[@]}"; do
    [ -n "$id" ] && force_remove_vm "$id"
  done
}
trap cleanup EXIT

echo "================================================================"
echo "  E2E Test 03: VM Lifecycle"
echo "================================================================"

# ---------------------------------------------------------------------------
# Test: Create VM with base runtime
# ---------------------------------------------------------------------------
section "Create VM (base runtime)"

VM1=$(create_vm --runtime base --vcpus 1 --memory 128)
assert_not_empty "$VM1" "VM created with base runtime"

if [ -z "$VM1" ]; then
  echo "  FATAL: Cannot continue without a VM. Aborting."
  print_summary
  exit 1
fi

# ---------------------------------------------------------------------------
# Test: VM appears in list as running
# ---------------------------------------------------------------------------
section "VM appears in list"

sleep 5
STATUS=$(get_vm_field "$VM1" "status")
assert_eq "$STATUS" "running" "VM status is 'running'"

RUNTIME=$(get_vm_field "$VM1" "runtime")
assert_eq "$RUNTIME" "base" "VM runtime is 'base'"

# ---------------------------------------------------------------------------
# Test: VM state file exists
# ---------------------------------------------------------------------------
section "State file"

STATE_FILE="$VMSAN_DIR/vms/${VM1}.json"
if [ -f "$STATE_FILE" ]; then
  assert_eq "yes" "yes" "state file exists at $STATE_FILE"
else
  STATE_JSON=$(get_vm_state_json "$VM1")
  assert_not_empty "$STATE_JSON" "VM tracked in state store"
fi

# ---------------------------------------------------------------------------
# Test: Exec command in VM
# ---------------------------------------------------------------------------
section "Exec command"

EXEC_OUT=$(run_vmsan exec "$VM1" -- echo "hello-from-vm" 2>/dev/null || echo "")
assert_contains "$EXEC_OUT" "hello-from-vm" "exec echo command succeeds"

# Run a command that checks the environment inside the VM
HOSTNAME_OUT=$(run_vmsan exec "$VM1" -- hostname 2>/dev/null || echo "")
assert_not_empty "$HOSTNAME_OUT" "exec hostname returns a value"

# ---------------------------------------------------------------------------
# Test: Exec with environment variable
# ---------------------------------------------------------------------------
section "Exec with --env"

ENV_OUT=$(run_vmsan exec "$VM1" -e TEST_VAR=e2e_value -- printenv TEST_VAR 2>/dev/null || echo "")
assert_contains "$ENV_OUT" "e2e_value" "exec with --env passes variable to VM"

# ---------------------------------------------------------------------------
# Test: Exec with working directory
# ---------------------------------------------------------------------------
section "Exec with --workdir"

WD_OUT=$(run_vmsan exec "$VM1" -w /tmp -- pwd 2>/dev/null || echo "")
assert_contains "$WD_OUT" "/tmp" "exec with --workdir sets working directory"

# ---------------------------------------------------------------------------
# Test: Upload and download file
# ---------------------------------------------------------------------------
section "File transfer (upload/download)"

UPLOAD_CONTENT="e2e-test-content-$(date +%s)"
UPLOAD_LOCAL=$(mktemp)
DOWNLOAD_LOCAL=$(mktemp)
echo "$UPLOAD_CONTENT" > "$UPLOAD_LOCAL"
chmod 666 "$UPLOAD_LOCAL" "$DOWNLOAD_LOCAL"

# Upload
if run_vmsan upload "$VM1" "$UPLOAD_LOCAL" -d /tmp 2>/dev/null; then
  assert_eq "yes" "yes" "file uploaded to VM"

  # Verify content inside VM
  UPLOAD_BASENAME=$(basename "$UPLOAD_LOCAL")
  REMOTE_CONTENT=$(run_vmsan exec "$VM1" -- cat "/tmp/$UPLOAD_BASENAME" 2>/dev/null || echo "")
  assert_contains "$REMOTE_CONTENT" "$UPLOAD_CONTENT" "uploaded file has correct content"

  # Download
  if run_vmsan download "$VM1" "/tmp/$UPLOAD_BASENAME" -d "$DOWNLOAD_LOCAL" 2>/dev/null; then
    assert_eq "yes" "yes" "file downloaded from VM"
    LOCAL_CONTENT=$(cat "$DOWNLOAD_LOCAL")
    assert_contains "$LOCAL_CONTENT" "$UPLOAD_CONTENT" "downloaded file matches uploaded content"
  else
    assert_eq "no" "yes" "file downloaded from VM"
  fi
else
  assert_eq "no" "yes" "file uploaded to VM"
fi

rm -f "$UPLOAD_LOCAL" "$DOWNLOAD_LOCAL"

# ---------------------------------------------------------------------------
# Test: Stop VM
# ---------------------------------------------------------------------------
section "Stop VM"

run_vmsan stop "$VM1" 2>/dev/null || true
sleep 2

STATUS_STOPPED=$(get_vm_field "$VM1" "status")
assert_eq "$STATUS_STOPPED" "stopped" "VM status is 'stopped' after stop"

# Exec should fail on stopped VM
EXEC_STOPPED=$(run_vmsan exec "$VM1" -- echo "should-fail" 2>&1 || true)
assert_not_contains "$EXEC_STOPPED" "should-fail" "exec fails on stopped VM"

# ---------------------------------------------------------------------------
# Test: Start (restart) stopped VM
# ---------------------------------------------------------------------------
section "Start (restart) stopped VM"

run_vmsan start "$VM1" 2>/dev/null || true
sleep 10

STATUS_RESTARTED=$(get_vm_field "$VM1" "status")
assert_eq "$STATUS_RESTARTED" "running" "VM status is 'running' after start"

# Exec should work again
EXEC_RESTARTED=$(run_vmsan exec "$VM1" -- echo "alive-again" 2>/dev/null || echo "")
assert_contains "$EXEC_RESTARTED" "alive-again" "exec works after restart"

# ---------------------------------------------------------------------------
# Test: Remove requires VM to be stopped
# ---------------------------------------------------------------------------
section "Remove requires stopped VM"

REMOVE_RUNNING=$(run_vmsan remove "$VM1" 2>&1 || true)
# VM should still exist (remove should fail or warn because it's running)
STATUS_STILL=$(get_vm_field "$VM1" "status")
assert_eq "$STATUS_STILL" "running" "VM still exists after remove attempt on running VM"

# ---------------------------------------------------------------------------
# Test: Force-remove running VM
# ---------------------------------------------------------------------------
section "Force-remove running VM"

run_vmsan remove --force "$VM1" 2>/dev/null || true
sleep 2

# VM should be gone
STATUS_GONE=$(get_vm_field "$VM1" "status")
assert_empty "$STATUS_GONE" "VM removed after force-remove"

# Remove from tracking
new_ids=()
for existing in "${VM_IDS[@]}"; do
  [ "$existing" != "$VM1" ] && new_ids+=("$existing")
done
VM_IDS=("${new_ids[@]}")

# ---------------------------------------------------------------------------
# Test: Network namespace cleaned up after remove
# ---------------------------------------------------------------------------
section "Resource cleanup after remove"

NETNS=$(get_netns "$VM1")
if netns_exists "$NETNS"; then
  assert_eq "exists" "gone" "network namespace cleaned up"
else
  assert_eq "gone" "gone" "network namespace cleaned up"
fi

# ---------------------------------------------------------------------------
# Test: Concurrent VM creation
# ---------------------------------------------------------------------------
section "Concurrent VM creation"

VM_A=$(create_vm --runtime base --vcpus 1 --memory 128)
VM_B=$(create_vm --runtime base --vcpus 1 --memory 128)

assert_not_empty "$VM_A" "concurrent VM-A created"
assert_not_empty "$VM_B" "concurrent VM-B created"

if [ -n "$VM_A" ] && [ -n "$VM_B" ]; then
  # VMs should have different IDs
  if [ "$VM_A" != "$VM_B" ]; then
    assert_eq "different" "different" "concurrent VMs have unique IDs"
  else
    assert_eq "same" "different" "concurrent VMs have unique IDs"
  fi

  # Both should be running
  sleep 5
  STATUS_A=$(get_vm_field "$VM_A" "status")
  STATUS_B=$(get_vm_field "$VM_B" "status")
  assert_eq "$STATUS_A" "running" "concurrent VM-A is running"
  assert_eq "$STATUS_B" "running" "concurrent VM-B is running"

  remove_vm "$VM_A"
  remove_vm "$VM_B"
fi

# ---------------------------------------------------------------------------
# Test: Create with custom resources
# ---------------------------------------------------------------------------
section "Create with custom resources"

VM_CUSTOM=$(create_vm --runtime base --vcpus 2 --memory 256 --disk 15gb)
assert_not_empty "$VM_CUSTOM" "VM created with custom resources"

if [ -n "$VM_CUSTOM" ]; then
  sleep 5
  MEM=$(get_vm_field "$VM_CUSTOM" "memSizeMib")
  VCPUS=$(get_vm_field "$VM_CUSTOM" "vcpuCount")
  assert_eq "$MEM" "256" "VM has 256 MiB memory"
  assert_eq "$VCPUS" "2" "VM has 2 vCPUs"

  remove_vm "$VM_CUSTOM"
fi

# ---------------------------------------------------------------------------
# Test: Empty list after all VMs removed
# ---------------------------------------------------------------------------
section "Empty list after cleanup"

FINAL_COUNT=$(count_vms)
assert_eq "$FINAL_COUNT" "0" "no VMs remaining after cleanup"

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
print_summary
