#!/bin/bash
set -euo pipefail

# =============================================================================
# vmsan e2e smoke tests
# Requires: KVM-capable host, vmsan installed, jq, root privileges
# Usage: sudo bash tests/e2e/smoke.sh
# =============================================================================

PASSED=0
FAILED=0
TOTAL=7
VM_IDS=()

pass() { echo "  PASS: $1"; PASSED=$((PASSED + 1)); }
fail() { echo "  FAIL: $1 — $2"; FAILED=$((FAILED + 1)); }

cleanup() {
  echo ""
  echo "=== Cleanup ==="
  for id in "${VM_IDS[@]}"; do
    echo "  Removing $id ..."
    vmsan remove --force "$id" 2>/dev/null || true
  done
  rm -f /tmp/vmsan-test-upload.txt
}
trap cleanup EXIT

# Extract vmId from vmsan JSON output (may contain multiple JSON lines)
extract_vm_id() {
  local output="$1"
  printf '%s\n' "$output" \
    | jq -Rr 'fromjson? | .vmId? // empty' \
    | head -n1
}

echo "=== vmsan e2e smoke tests ==="
echo ""

# ---------------------------------------------------------------------------
# I1: Full lifecycle — create, exec, stop, remove
# ---------------------------------------------------------------------------
echo "[I1] Full lifecycle"
if out=$(vmsan create --runtime base --vcpus 1 --memory 256 --json 2>&1); then
  VM_ID=$(extract_vm_id "$out")
  if [ -n "$VM_ID" ]; then
    VM_IDS+=("$VM_ID")
    if vmsan exec "$VM_ID" echo "hello from VM" 2>/dev/null | grep -q "hello from VM"; then
      if vmsan stop "$VM_ID" 2>&1 && vmsan remove "$VM_ID" 2>&1; then
        VM_IDS=("${VM_IDS[@]/$VM_ID/}")
        pass "I1: full lifecycle (create, exec, stop, remove)"
      else
        fail "I1" "stop/remove failed"
      fi
    else
      fail "I1" "exec did not return expected output"
    fi
  else
    fail "I1" "could not extract vmId from create output"
  fi
else
  fail "I1" "create failed"
fi

# ---------------------------------------------------------------------------
# I2: File transfer — upload a file and verify contents inside VM
# ---------------------------------------------------------------------------
echo "[I2] File transfer"
echo "test-content" > /tmp/vmsan-test-upload.txt
if out=$(vmsan create --runtime base --vcpus 1 --memory 256 --json 2>&1); then
  VM_ID=$(extract_vm_id "$out")
  if [ -n "$VM_ID" ]; then
    VM_IDS+=("$VM_ID")
    if vmsan upload "$VM_ID" /tmp/vmsan-test-upload.txt 2>&1; then
      if vmsan exec "$VM_ID" --sudo cat /root/vmsan-test-upload.txt 2>/dev/null | grep -q "test-content"; then
        pass "I2: file transfer (upload + verify)"
      else
        fail "I2" "uploaded file content mismatch"
      fi
    else
      fail "I2" "upload failed"
    fi
    vmsan stop "$VM_ID" 2>/dev/null || true
    vmsan remove "$VM_ID" 2>/dev/null || true
    VM_IDS=("${VM_IDS[@]/$VM_ID/}")
  else
    fail "I2" "could not extract vmId"
  fi
else
  fail "I2" "create failed"
fi

# ---------------------------------------------------------------------------
# I3: Network deny-all — outbound traffic should be blocked
# ---------------------------------------------------------------------------
echo "[I3] Network deny-all"
if out=$(vmsan create --runtime base --vcpus 1 --memory 256 --network-policy deny-all --json 2>&1); then
  VM_ID=$(extract_vm_id "$out")
  if [ -n "$VM_ID" ]; then
    VM_IDS+=("$VM_ID")
    # First verify exec works, then check that ping fails due to network blocking
    if ! vmsan exec "$VM_ID" true 2>/dev/null; then
      fail "I3" "exec failed before the network assertion ran"
    elif out=$(vmsan exec "$VM_ID" sh -c 'ping -c 1 -W 3 8.8.8.8' 2>&1); then
      fail "I3" "ping succeeded but should have been blocked"
    elif printf '%s\n' "$out" | grep -qiE 'unreachable|timed out|100% packet loss|Operation not permitted'; then
      pass "I3: network deny-all (outbound blocked)"
    else
      fail "I3" "ping failed for an unexpected reason: $out"
    fi
    vmsan stop "$VM_ID" 2>/dev/null || true
    vmsan remove "$VM_ID" 2>/dev/null || true
    VM_IDS=("${VM_IDS[@]/$VM_ID/}")
  else
    fail "I3" "could not extract vmId"
  fi
else
  fail "I3" "create failed"
fi

# ---------------------------------------------------------------------------
# I4: Concurrent creates — two VMs created in parallel get different IDs
# ---------------------------------------------------------------------------
echo "[I4] Concurrent creates"
VM1=""
VM2=""
OUT1=$(mktemp)
OUT2=$(mktemp)

vmsan create --runtime base --vcpus 1 --memory 128 --json > "$OUT1" 2>&1 &
PID1=$!
vmsan create --runtime base --vcpus 1 --memory 128 --json > "$OUT2" 2>&1 &
PID2=$!

wait "$PID1" || true
wait "$PID2" || true

VM1=$(extract_vm_id "$(cat "$OUT1")")
VM2=$(extract_vm_id "$(cat "$OUT2")")
rm -f "$OUT1" "$OUT2"

if [ -n "$VM1" ]; then VM_IDS+=("$VM1"); fi
if [ -n "$VM2" ]; then VM_IDS+=("$VM2"); fi

if [ -n "$VM1" ] && [ -n "$VM2" ] && [ "$VM1" != "$VM2" ]; then
  pass "I4: concurrent creates (different IDs: ${VM1:0:8}, ${VM2:0:8})"
else
  fail "I4" "VMs not created or IDs are identical (VM1=$VM1 VM2=$VM2)"
fi

# Cleanup I4 VMs
for cid in "$VM1" "$VM2"; do
  if [ -n "$cid" ]; then
    vmsan stop "$cid" 2>/dev/null || true
    vmsan remove "$cid" 2>/dev/null || true
    VM_IDS=("${VM_IDS[@]/$cid/}")
  fi
done

# ---------------------------------------------------------------------------
# I5: Force remove — remove a running VM without stopping first
# ---------------------------------------------------------------------------
echo "[I5] Force remove"
if out=$(vmsan create --runtime base --vcpus 1 --memory 256 --json 2>&1); then
  VM_ID=$(extract_vm_id "$out")
  if [ -n "$VM_ID" ]; then
    VM_IDS+=("$VM_ID")
    if vmsan remove --force "$VM_ID" 2>&1; then
      VM_IDS=("${VM_IDS[@]/$VM_ID/}")
      pass "I5: force remove (running VM removed)"
    else
      fail "I5" "force remove failed"
    fi
  else
    fail "I5" "could not extract vmId"
  fi
else
  fail "I5" "create failed"
fi

# ---------------------------------------------------------------------------
# I6: Doctor — vmsan doctor checks system prerequisites
# ---------------------------------------------------------------------------
echo "[I6] Doctor"
if vmsan doctor 2>&1; then
  pass "I6: vmsan doctor"
else
  fail "I6" "vmsan doctor reported failures"
fi

# ---------------------------------------------------------------------------
# I7: JSON output — vmsan list --json produces valid JSON
# ---------------------------------------------------------------------------
echo "[I7] JSON output"
if vmsan list --json 2>/dev/null | jq . > /dev/null 2>&1; then
  pass "I7: vmsan list --json (valid JSON)"
else
  fail "I7" "vmsan list --json did not produce valid JSON"
fi

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
echo ""
echo "=== $PASSED/$TOTAL SMOKE TESTS PASSED ==="

if [ "$FAILED" -gt 0 ]; then
  exit 1
fi
exit 0
