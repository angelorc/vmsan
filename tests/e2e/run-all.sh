#!/usr/bin/env bash
set -euo pipefail

# =============================================================================
# E2E Test Runner
# Runs all E2E test scripts and reports aggregate results.
#
# Requires: KVM host, vmsan installed from feat/platform-multihost, jq, root
# Usage: sudo bash tests/e2e/run-all.sh
# =============================================================================

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PASSED=0 FAILED=0 SKIPPED=0

echo "================================================================"
echo "  vmsan E2E Test Suite"
echo "  Branch: feat/platform-multihost"
echo "  Date: $(date -u '+%Y-%m-%d %H:%M:%S UTC')"
echo "================================================================"

# Preflight checks
echo ""
echo "--- Preflight ---"

if ! command -v vmsan >/dev/null 2>&1; then
  echo "ERROR: vmsan not found in PATH"
  echo "  Install with: sudo bash install.sh --ref feat/platform-multihost"
  exit 1
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "ERROR: jq not found in PATH"
  exit 1
fi

if [ "$(id -u)" -ne 0 ]; then
  echo "WARNING: not running as root -- some tests may fail"
fi

echo "  vmsan: $(which vmsan)"
echo "  jq: $(which jq)"
echo "  user: $(whoami)"
echo ""

run_test() {
  local name="$1" script="$2"
  echo ""
  echo "================================================================"
  echo "  Running: $name"
  echo "================================================================"
  if [ -x "$script" ]; then
    if bash "$script"; then
      PASSED=$((PASSED + 1))
    else
      FAILED=$((FAILED + 1))
      echo "FAILED: $name"
    fi
  else
    SKIPPED=$((SKIPPED + 1))
    echo "SKIPPED: $name (not executable)"
  fi
}

run_test "Multi-Distro Compat"  "${SCRIPT_DIR}/test-multi-distro.sh"
run_test "DNAT Flow"            "${SCRIPT_DIR}/test-dnat-flow.sh"
run_test "vmsan up Lifecycle"   "${SCRIPT_DIR}/test-vmsan-up.sh"
run_test "Mesh Networking"      "${SCRIPT_DIR}/test-mesh-networking.sh"
run_test "Server+Agent Join"    "${SCRIPT_DIR}/test-server-agent.sh"
run_test "Gateway Supervision"  "${SCRIPT_DIR}/test-gateway-supervision.sh"

echo ""
echo "================================================================"
echo "  Results: ${PASSED} passed, ${FAILED} failed, ${SKIPPED} skipped"
echo "================================================================"

[ "$FAILED" -eq 0 ] || exit 1
