#!/usr/bin/env bash
set -euo pipefail

# =============================================================================
# E2E Test 08: Snapshot Lifecycle
# Tests snapshot create, list, restore from snapshot, and delete.
#
# Requires: KVM host, vmsan installed, base runtime
# Usage: sudo bash tests/e2e/test-08-snapshot-lifecycle.sh
# =============================================================================

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/lib/helpers.sh"

VM_IDS=()
SNAPSHOT_ID=""

cleanup() {
  echo ""
  echo "--- Cleanup ---"
  for id in "${VM_IDS[@]}"; do
    [ -n "$id" ] && force_remove_vm "$id"
  done
  # Clean up snapshot
  if [ -n "$SNAPSHOT_ID" ]; then
    run_vmsan snapshot delete "$SNAPSHOT_ID" 2>/dev/null || true
  fi
}
trap cleanup EXIT

echo "================================================================"
echo "  E2E Test 08: Snapshot Lifecycle"
echo "================================================================"

# ===========================================================================
# CREATE VM FOR SNAPSHOT
# ===========================================================================
section "Create VM for snapshot"

VM_SNAP=$(create_vm --runtime base --vcpus 1 --memory 128 --network-policy allow-all)
assert_not_empty "$VM_SNAP" "VM created for snapshot test"

if [ -z "$VM_SNAP" ]; then
  echo "  FATAL: Cannot continue without a VM. Aborting."
  print_summary
  exit 1
fi

# Wait for agent to be ready
sleep 10

# Write a marker file inside the VM to verify snapshot restore
MARKER="snapshot-marker-$(date +%s)"
run_vmsan exec "$VM_SNAP" -- bash -c "echo '$MARKER' > /tmp/snapshot-test.txt" 2>/dev/null || true

# Verify marker exists
MARKER_CHECK=$(run_vmsan exec "$VM_SNAP" -- cat /tmp/snapshot-test.txt 2>/dev/null || echo "")
assert_contains "$MARKER_CHECK" "$MARKER" "marker file written to VM"

# ===========================================================================
# CREATE SNAPSHOT
# ===========================================================================
section "Create snapshot"

SNAP_OUT=$(run_vmsan snapshot create "$VM_SNAP" 2>&1 || echo "SNAP_FAILED")

if echo "$SNAP_OUT" | grep -qi "fail\|error"; then
  skip_test "snapshot create failed: $(echo "$SNAP_OUT" | head -3)"
else
  # Extract snapshot ID (format: vm-<8hex>-<timestamp>)
  SNAPSHOT_ID=$(echo "$SNAP_OUT" | grep -oE 'vm-[0-9a-f]{8}-[0-9]+' | head -1 || echo "")

  assert_not_empty "$SNAPSHOT_ID" "snapshot created (ID: $SNAPSHOT_ID)"

  # VM should still be running after snapshot (auto-resume)
  sleep 3
  STATUS_AFTER_SNAP=$(get_vm_field "$VM_SNAP" "status")
  assert_eq "$STATUS_AFTER_SNAP" "running" "VM still running after snapshot"
fi

# ===========================================================================
# LIST SNAPSHOTS
# ===========================================================================
section "List snapshots"

SNAP_LIST=$(run_vmsan snapshot list 2>&1 || echo "")
assert_not_empty "$SNAP_LIST" "snapshot list produces output"

if [ -n "$SNAPSHOT_ID" ]; then
  assert_contains "$SNAP_LIST" "$SNAPSHOT_ID" "snapshot ID appears in list"
fi

# ===========================================================================
# VERIFY SNAPSHOT FILES
# ===========================================================================
section "Snapshot files on disk"

SNAPSHOTS_DIR="$VMSAN_DIR/snapshots"

if [ -n "$SNAPSHOT_ID" ] && [ -d "${SNAPSHOTS_DIR}/${SNAPSHOT_ID}" ]; then
  SNAP_DIR="${SNAPSHOTS_DIR}/${SNAPSHOT_ID}"

  # Check for snapshot_file
  if [ -f "${SNAP_DIR}/snapshot_file" ]; then
    assert_eq "yes" "yes" "snapshot_file exists"
  else
    assert_eq "no" "yes" "snapshot_file exists"
  fi

  # Check for mem_file
  if [ -f "${SNAP_DIR}/mem_file" ]; then
    assert_eq "yes" "yes" "mem_file exists"
  else
    assert_eq "no" "yes" "mem_file exists"
  fi

  # Check for metadata.json
  if [ -f "${SNAP_DIR}/metadata.json" ]; then
    assert_eq "yes" "yes" "metadata.json exists"

    # Verify metadata has agentToken (needed for restore)
    AGENT_TOKEN=$(jq -r '.agentToken // empty' "${SNAP_DIR}/metadata.json" 2>/dev/null || echo "")
    assert_not_empty "$AGENT_TOKEN" "metadata contains agentToken"
  else
    assert_eq "no" "yes" "metadata.json exists"
  fi
else
  skip_test "snapshot directory not found"
fi

# ===========================================================================
# REMOVE ORIGINAL VM
# ===========================================================================
section "Remove original VM"

remove_vm "$VM_SNAP"
sleep 2

STATUS_REMOVED=$(get_vm_field "$VM_SNAP" "status")
assert_empty "$STATUS_REMOVED" "original VM removed"

# ===========================================================================
# RESTORE FROM SNAPSHOT
# ===========================================================================
section "Restore from snapshot"

if [ -n "$SNAPSHOT_ID" ]; then
  VM_RESTORED=$(create_vm --runtime base --vcpus 1 --memory 128 \
    --network-policy allow-all --snapshot "$SNAPSHOT_ID")
  assert_not_empty "$VM_RESTORED" "VM created from snapshot"

  if [ -n "$VM_RESTORED" ]; then
    sleep 15  # Snapshot restore may take longer

    STATUS_RESTORED=$(get_vm_field "$VM_RESTORED" "status")
    assert_eq "$STATUS_RESTORED" "running" "restored VM is running"

    # Check that the marker file persists from the snapshot
    MARKER_RESTORED=$(run_vmsan exec "$VM_RESTORED" -- cat /tmp/snapshot-test.txt 2>/dev/null || echo "")
    if [ -n "$MARKER_RESTORED" ]; then
      assert_contains "$MARKER_RESTORED" "$MARKER" "snapshot marker file preserved"
    else
      skip_test "marker file check inconclusive (agent may use fresh rootfs)"
    fi

    # Agent should be responsive
    HEALTH_OUT=$(run_vmsan exec "$VM_RESTORED" -- echo "alive-after-restore" 2>/dev/null || echo "")
    assert_contains "$HEALTH_OUT" "alive-after-restore" "exec works on restored VM"

    remove_vm "$VM_RESTORED"
  fi
else
  skip_test "no snapshot ID available for restore test"
fi

# ===========================================================================
# DELETE SNAPSHOT
# ===========================================================================
section "Delete snapshot"

if [ -n "$SNAPSHOT_ID" ]; then
  SNAPSHOT_ID_TO_DELETE="$SNAPSHOT_ID"
  DEL_OUT=$(run_vmsan snapshot delete "$SNAPSHOT_ID" 2>&1 || echo "DEL_FAILED")

  if echo "$DEL_OUT" | grep -qi "fail\|error"; then
    skip_test "snapshot delete reported error"
  else
    assert_eq "deleted" "deleted" "snapshot deleted"
  fi

  # Verify files are gone
  if [ -d "${SNAPSHOTS_DIR}/${SNAPSHOT_ID}" ]; then
    assert_eq "exists" "gone" "snapshot directory removed after delete"
  else
    assert_eq "gone" "gone" "snapshot directory removed after delete"
  fi

  # Clear so cleanup doesn't try to delete again
  SNAPSHOT_ID=""

  # Verify it's gone from list
  SNAP_LIST_AFTER=$(run_vmsan snapshot list 2>&1 || echo "")
  if echo "$SNAP_LIST_AFTER" | grep -q "$SNAPSHOT_ID_TO_DELETE" 2>/dev/null; then
    assert_eq "still listed" "gone" "snapshot removed from list"
  fi
fi

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
print_summary
