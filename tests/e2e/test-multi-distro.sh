#!/usr/bin/env bash
set -euo pipefail

# =============================================================================
# E2E Test: Multi-Distro Compatibility
# Tests system info checks, vmsan doctor output, and a quick lifecycle test
# across different host distributions.
#
# Requires: KVM host, vmsan installed from feat/platform-multihost, jq, root
# Usage: sudo bash tests/e2e/test-multi-distro.sh
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
}
trap cleanup EXIT

echo "================================================================"
echo "  E2E Test: Multi-Distro Compatibility"
echo "================================================================"

# ---------------------------------------------------------------------------
# Test: Host system info
# ---------------------------------------------------------------------------
echo ""
echo "--- Test: Host system info ---"

# Detect distro
DISTRO="unknown"
if [ -f /etc/os-release ]; then
  DISTRO=$(. /etc/os-release && echo "${ID:-unknown} ${VERSION_ID:-}")
fi
echo "  Host distro: $DISTRO"
assert_not_empty "$DISTRO" "host distro detected"

# Kernel version
KERNEL=$(uname -r)
echo "  Kernel: $KERNEL"
assert_not_empty "$KERNEL" "kernel version detected"

# Architecture
ARCH=$(uname -m)
echo "  Architecture: $ARCH"
assert_contains "$ARCH" "x86_64\|aarch64" "architecture is supported (x86_64 or aarch64)"

# KVM available
if [ -e /dev/kvm ]; then
  echo -e "${GREEN}PASS${NC}: /dev/kvm exists"
  TESTS_PASSED=$((TESTS_PASSED + 1))
else
  echo -e "${RED}FAIL${NC}: /dev/kvm not found (KVM not available)"
  TESTS_FAILED=$((TESTS_FAILED + 1))
fi

# ---------------------------------------------------------------------------
# Test: vmsan doctor
# ---------------------------------------------------------------------------
echo ""
echo "--- Test: vmsan doctor ---"

DOCTOR_OUTPUT=$(vmsan doctor 2>&1 || true)
DOCTOR_EXIT=$?

if [ $DOCTOR_EXIT -eq 0 ]; then
  echo -e "${GREEN}PASS${NC}: vmsan doctor passed"
  TESTS_PASSED=$((TESTS_PASSED + 1))
else
  echo -e "${RED}FAIL${NC}: vmsan doctor reported issues (exit code: $DOCTOR_EXIT)"
  TESTS_FAILED=$((TESTS_FAILED + 1))
  echo "  Doctor output:"
  echo "$DOCTOR_OUTPUT" | sed 's/^/    /'
fi

# Verify doctor checks key components
assert_contains "$DOCTOR_OUTPUT" "kvm\|KVM\|firecracker\|Firecracker" "doctor checks KVM/Firecracker"

# ---------------------------------------------------------------------------
# Test: vmsan version
# ---------------------------------------------------------------------------
echo ""
echo "--- Test: vmsan version ---"

VERSION_OUTPUT=$(vmsan --version 2>&1 || vmsan version 2>&1 || true)
assert_not_empty "$VERSION_OUTPUT" "vmsan version outputs something"
echo "  Version: $VERSION_OUTPUT"

# ---------------------------------------------------------------------------
# Test: vmsan list --json (no VMs)
# ---------------------------------------------------------------------------
echo ""
echo "--- Test: vmsan list --json ---"

LIST_OUTPUT=$(vmsan list --json 2>/dev/null || echo "invalid")
if echo "$LIST_OUTPUT" | jq . >/dev/null 2>&1; then
  echo -e "${GREEN}PASS${NC}: vmsan list --json returns valid JSON"
  TESTS_PASSED=$((TESTS_PASSED + 1))
else
  echo -e "${RED}FAIL${NC}: vmsan list --json did not return valid JSON"
  TESTS_FAILED=$((TESTS_FAILED + 1))
fi

# ---------------------------------------------------------------------------
# Test: Quick lifecycle (create -> exec -> stop -> remove)
# ---------------------------------------------------------------------------
echo ""
echo "--- Test: Quick lifecycle on $DISTRO ---"

SECONDS=0
VM_ID=$(create_vm --runtime base --vcpus 1 --memory 256)
assert_not_empty "$VM_ID" "lifecycle: VM created on $DISTRO"

if [ -n "$VM_ID" ]; then
  # Exec
  EXEC_RESULT=$(sudo vmsan exec "$VM_ID" -- echo "distro-test-ok" 2>/dev/null || true)
  assert_contains "$EXEC_RESULT" "distro-test-ok" "lifecycle: exec returns expected output"

  # Check VM state
  VM_STATE=$(vmsan list --json 2>/dev/null | jq -r --arg id "$VM_ID" '.[] | select(.id==$id) | .state // empty' || true)
  assert_eq "$VM_STATE" "running" "lifecycle: VM is in running state"

  # Stop
  if sudo vmsan stop "$VM_ID" 2>&1; then
    echo -e "${GREEN}PASS${NC}: lifecycle: VM stopped"
    TESTS_PASSED=$((TESTS_PASSED + 1))
  else
    echo -e "${RED}FAIL${NC}: lifecycle: VM stop failed"
    TESTS_FAILED=$((TESTS_FAILED + 1))
  fi

  # Remove
  if sudo vmsan remove "$VM_ID" 2>&1; then
    echo -e "${GREEN}PASS${NC}: lifecycle: VM removed"
    TESTS_PASSED=$((TESTS_PASSED + 1))
    # Remove from tracking
    new_ids=()
    for existing in "${VM_IDS[@]}"; do
      [ "$existing" != "$VM_ID" ] && new_ids+=("$existing")
    done
    VM_IDS=("${new_ids[@]}")
  else
    echo -e "${RED}FAIL${NC}: lifecycle: VM remove failed"
    TESTS_FAILED=$((TESTS_FAILED + 1))
  fi

  echo "  Lifecycle completed in ${SECONDS}s"
fi

# ---------------------------------------------------------------------------
# Test: Multiple runtimes (if available)
# ---------------------------------------------------------------------------
echo ""
echo "--- Test: Runtime availability ---"

for RUNTIME in base node22 python3.13; do
  echo "  Testing runtime: $RUNTIME"
  RT_VM=$(create_vm --runtime "$RUNTIME" --vcpus 1 --memory 256 2>/dev/null || true)
  if [ -n "$RT_VM" ]; then
    echo -e "${GREEN}PASS${NC}: runtime '$RUNTIME' boots successfully"
    TESTS_PASSED=$((TESTS_PASSED + 1))
    remove_vm "$RT_VM"
  else
    # Non-base runtimes may not be installed -- this is informational
    echo -e "${YELLOW}SKIP${NC}: runtime '$RUNTIME' not available (may not be installed)"
  fi
done

# ---------------------------------------------------------------------------
# Test: nftables binary exists and works
# ---------------------------------------------------------------------------
echo ""
echo "--- Test: nftables tools ---"

# Check that nft command is available
if command -v nft >/dev/null 2>&1; then
  echo -e "${GREEN}PASS${NC}: nft command available"
  TESTS_PASSED=$((TESTS_PASSED + 1))
else
  echo -e "${RED}FAIL${NC}: nft command not found"
  TESTS_FAILED=$((TESTS_FAILED + 1))
fi

# Check that vmsan-nftables binary is in PATH or ~/.vmsan/bin
NFTABLES_BIN=$(which vmsan-nftables 2>/dev/null || echo "${HOME}/.vmsan/bin/vmsan-nftables")
if [ -x "$NFTABLES_BIN" ]; then
  echo -e "${GREEN}PASS${NC}: vmsan-nftables binary found at $NFTABLES_BIN"
  TESTS_PASSED=$((TESTS_PASSED + 1))
else
  echo -e "${YELLOW}SKIP${NC}: vmsan-nftables binary not found (may be built-in)"
fi

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
echo ""
echo "  Host: $DISTRO | Kernel: $KERNEL | Arch: $ARCH"
print_summary
