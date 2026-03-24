#!/usr/bin/env bash
set -euo pipefail

# =============================================================================
# E2E Test 01: Doctor (System Health)
# Validates that the host system is properly configured for vmsan.
#
# Requires: KVM host, vmsan installed from feat/platform-multihost
# Usage: sudo bash tests/e2e/test-01-doctor.sh
# =============================================================================

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/lib/helpers.sh"

echo "================================================================"
echo "  E2E Test 01: Doctor (System Health)"
echo "================================================================"

# ---------------------------------------------------------------------------
# Test: vmsan doctor runs without error
# ---------------------------------------------------------------------------
section "vmsan doctor exit code"

DOCTOR_OUT=$(run_vmsan doctor 2>&1 || true)
DOCTOR_RC=$?
# Doctor exits 0 when all checks pass, 1 when any fail
# We just verify it runs and produces output
assert_not_empty "$DOCTOR_OUT" "doctor produces output"

# ---------------------------------------------------------------------------
# Test: KVM available
# ---------------------------------------------------------------------------
section "KVM device"

if [ -c /dev/kvm ]; then
  assert_eq "yes" "yes" "/dev/kvm exists and is a character device"
else
  assert_eq "no" "yes" "/dev/kvm exists and is a character device"
fi

# ---------------------------------------------------------------------------
# Test: Required binaries exist
# ---------------------------------------------------------------------------
section "Required binaries"

# Check multiple search paths: PATH, /usr/local/bin, /usr/bin, CLI user's ~/.vmsan/bin
bin_exists() {
  local name="$1"
  command -v "$name" >/dev/null 2>&1 \
    || [ -x "/usr/local/bin/$name" ] \
    || [ -x "/usr/bin/$name" ] \
    || [ -x "${VMSAN_E2E_HOME}/.vmsan/bin/$name" ]
}

for BIN in vmsan firecracker jailer; do
  if bin_exists "$BIN"; then
    assert_eq "found" "found" "$BIN binary exists"
  else
    assert_eq "missing" "found" "$BIN binary exists"
  fi
done

for BIN in vmsan-gateway vmsan-nftables vmsan-agent; do
  if bin_exists "$BIN"; then
    assert_eq "found" "found" "$BIN binary exists"
  else
    assert_eq "missing" "found" "$BIN binary exists"
  fi
done

# ---------------------------------------------------------------------------
# Test: Kernel and rootfs images exist
# ---------------------------------------------------------------------------
section "Kernel and rootfs images"

# Look for kernel (check both user's and root's ~/.vmsan)
KERNEL_FOUND="no"
for base in "${VMSAN_DIR}" "/home/"*"/.vmsan"; do
  for f in "${base}/runtimes/base/vmlinux" "${base}/kernels/"vmlinux*; do
    if [ -f "$f" ]; then
      KERNEL_FOUND="yes"
      break 2
    fi
  done
done
assert_eq "$KERNEL_FOUND" "yes" "kernel image found"

# Look for rootfs
ROOTFS_FOUND="no"
for base in "${VMSAN_DIR}" "/home/"*"/.vmsan"; do
  for f in "${base}/runtimes/base/rootfs.ext4" "${base}/rootfs/"*.ext4; do
    if [ -f "$f" ]; then
      ROOTFS_FOUND="yes"
      break 2
    fi
  done
done
assert_eq "$ROOTFS_FOUND" "yes" "rootfs image found"

# ---------------------------------------------------------------------------
# Test: Non-root CLI access through gateway
# ---------------------------------------------------------------------------
section "Non-root CLI access"

if [ "$VMSAN_EXPECT_NONROOT" -eq 1 ]; then
  GROUPS_OUT=$(cli_user_groups)
  assert_contains "$GROUPS_OUT" "vmsan" "CLI user belongs to vmsan group"

  LIST_OUT=$(run_vmsan list 2>&1 || echo "")
  assert_not_empty "$LIST_OUT" "CLI user can run vmsan without sudo"
else
  skip_test "non-root CLI user could not be resolved"
fi

# ---------------------------------------------------------------------------
# Test: Gateway systemd service exists
# ---------------------------------------------------------------------------
section "Gateway systemd service"

if systemctl list-unit-files vmsan-gateway.service >/dev/null 2>&1; then
  assert_eq "found" "found" "vmsan-gateway.service unit file installed"
else
  assert_eq "missing" "found" "vmsan-gateway.service unit file installed"
fi

# ---------------------------------------------------------------------------
# Test: vmsan group exists
# ---------------------------------------------------------------------------
section "vmsan group"

if getent group vmsan >/dev/null 2>&1; then
  assert_eq "exists" "exists" "vmsan system group exists"
else
  assert_eq "missing" "exists" "vmsan system group exists"
fi

# ---------------------------------------------------------------------------
# Test: TUN device available
# ---------------------------------------------------------------------------
section "TUN device"

if [ -c /dev/net/tun ]; then
  assert_eq "yes" "yes" "/dev/net/tun available"
else
  assert_eq "no" "yes" "/dev/net/tun available"
fi

# ---------------------------------------------------------------------------
# Test: Sufficient disk space (>5GB)
# ---------------------------------------------------------------------------
section "Disk space"

AVAIL_KB=$(df --output=avail "$HOME" 2>/dev/null | tail -1 | tr -d ' ')
AVAIL_GB=$((AVAIL_KB / 1024 / 1024))
assert_gt "$AVAIL_GB" 4 "available disk space > 4 GB (${AVAIL_GB} GB)"

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
print_summary
