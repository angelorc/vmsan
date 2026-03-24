#!/usr/bin/env bash
set -euo pipefail

# =============================================================================
# vmsan E2E Test Runner
# Runs all E2E test scripts and reports aggregate results.
#
# Requires: KVM host, vmsan installed from feat/platform-multihost, jq, socat
# Usage: sudo bash tests/e2e/run-all.sh
# =============================================================================

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/lib/helpers.sh"
PASSED=0 FAILED=0 SKIPPED=0

echo "================================================================"
echo "  vmsan E2E Test Suite"
echo "  Branch: feat/platform-multihost"
echo "  Date: $(date -u '+%Y-%m-%d %H:%M:%S UTC')"
echo "  Host: $(hostname)"
echo "  Kernel: $(uname -r)"
echo "================================================================"

# ---------------------------------------------------------------------------
# Preflight checks
# ---------------------------------------------------------------------------
echo ""
echo "--- Preflight ---"

PREFLIGHT_OK=true

if ! command -v vmsan >/dev/null 2>&1; then
  echo "  ERROR: vmsan not found in PATH"
  echo "    Install with: sudo bash install.sh --ref feat/platform-multihost"
  PREFLIGHT_OK=false
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "  ERROR: jq not found in PATH"
  PREFLIGHT_OK=false
fi

if ! command -v socat >/dev/null 2>&1; then
  echo "  ERROR: socat not found in PATH (install: apt install socat)"
  PREFLIGHT_OK=false
fi

if ! command -v curl >/dev/null 2>&1; then
  echo "  ERROR: curl not found in PATH"
  PREFLIGHT_OK=false
fi

if ! command -v python3 >/dev/null 2>&1; then
  echo "  ERROR: python3 not found in PATH"
  PREFLIGHT_OK=false
fi

if [ "$(id -u)" -ne 0 ]; then
  echo "  WARNING: not running as root — some tests may fail"
fi

if [ "$PREFLIGHT_OK" = false ]; then
  echo ""
  echo "  Preflight failed. Install missing dependencies and retry."
  exit 1
fi

echo "  vmsan:  $(which vmsan)"
echo "  jq:     $(which jq)"
echo "  socat:  $(which socat)"
echo "  user:   $(whoami)"
echo "  cli:    ${VMSAN_E2E_USER} (${VMSAN_DIR})"

# Check KVM
if [ -c /dev/kvm ]; then
  echo "  KVM:    available"
else
  echo "  KVM:    NOT available — VM tests will fail"
fi

# Check gateway
GW_STATUS=$(systemctl is-active vmsan-gateway 2>/dev/null || echo "inactive")
echo "  Gateway: $GW_STATUS"
echo ""

# ---------------------------------------------------------------------------
# Run tests
# ---------------------------------------------------------------------------
run_test() {
  local name="$1" script="$2"
  echo ""
  echo "================================================================"
  echo "  Running: $name"
  echo "================================================================"
  if [ ! -f "$script" ]; then
    SKIPPED=$((SKIPPED + 1))
    echo "  SKIPPED: $name (file not found)"
    return
  fi
  if bash "$script"; then
    PASSED=$((PASSED + 1))
  else
    FAILED=$((FAILED + 1))
    echo "  FAILED: $name"
  fi
}

run_test "01 — Doctor (system health)"     "${SCRIPT_DIR}/test-01-doctor.sh"
run_test "02 — Gateway Lifecycle"          "${SCRIPT_DIR}/test-02-gateway-lifecycle.sh"
run_test "03 — VM Lifecycle"               "${SCRIPT_DIR}/test-03-vm-lifecycle.sh"
run_test "04 — Network & Firewall"         "${SCRIPT_DIR}/test-04-network-firewall.sh"
run_test "05 — Mesh Networking"            "${SCRIPT_DIR}/test-05-mesh-networking.sh"
run_test "06 — Project Orchestration"      "${SCRIPT_DIR}/test-06-project-orchestration.sh"
run_test "07 — Secrets Management"         "${SCRIPT_DIR}/test-07-secrets.sh"
run_test "08 — Snapshot Lifecycle"         "${SCRIPT_DIR}/test-08-snapshot-lifecycle.sh"
run_test "09 — State & Migration"          "${SCRIPT_DIR}/test-09-state-migration.sh"
run_test "10 — Control Plane"              "${SCRIPT_DIR}/test-10-control-plane.sh"

# ---------------------------------------------------------------------------
# Final report
# ---------------------------------------------------------------------------
echo ""
echo "================================================================"
echo "  Results: ${PASSED} passed, ${FAILED} failed, ${SKIPPED} skipped"
echo "================================================================"

[ "$FAILED" -eq 0 ] || exit 1
