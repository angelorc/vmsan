#!/usr/bin/env bash
set -euo pipefail

# =============================================================================
# E2E Test 07: Secrets Management
# Tests vmsan secrets set, list, and unset commands with project scoping.
#
# Requires: vmsan installed
# Usage: sudo bash tests/e2e/test-07-secrets.sh
# =============================================================================

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/lib/helpers.sh"

VM_IDS=()

cleanup() {
  echo ""
  echo "--- Cleanup ---"
  # Clean up test secrets
  run_vmsan secrets unset E2E_SECRET_A --project e2e-secrets-test 2>/dev/null || true
  run_vmsan secrets unset E2E_SECRET_B --project e2e-secrets-test 2>/dev/null || true
  run_vmsan secrets unset E2E_SECRET_C --project e2e-secrets-test 2>/dev/null || true
  for id in "${VM_IDS[@]}"; do
    [ -n "$id" ] && force_remove_vm "$id"
  done
}
trap cleanup EXIT

echo "================================================================"
echo "  E2E Test 07: Secrets Management"
echo "================================================================"

PROJECT="e2e-secrets-test"

# ===========================================================================
# SET SECRET
# ===========================================================================
section "Set secret"

SET_OUT=$(run_vmsan secrets set E2E_SECRET_A "test-value-alpha" --project "$PROJECT" 2>&1 || echo "FAIL")
if echo "$SET_OUT" | grep -qi "error\|fail"; then
  assert_eq "failed" "success" "set secret E2E_SECRET_A"
else
  assert_eq "success" "success" "set secret E2E_SECRET_A"
fi

# Set a second secret
SET_OUT2=$(run_vmsan secrets set E2E_SECRET_B "test-value-beta" --project "$PROJECT" 2>&1 || echo "FAIL")
if echo "$SET_OUT2" | grep -qi "error\|fail"; then
  assert_eq "failed" "success" "set secret E2E_SECRET_B"
else
  assert_eq "success" "success" "set secret E2E_SECRET_B"
fi

SECRETS_FILE="${VMSAN_DIR}/secrets/${PROJECT}.enc"
if [ -f "$SECRETS_FILE" ]; then
  assert_eq "yes" "yes" "encrypted project secrets file exists"
  if grep -q "test-value-alpha" "$SECRETS_FILE" 2>/dev/null; then
    assert_eq "plaintext" "encrypted" "secret values are not stored in plaintext"
  else
    assert_eq "encrypted" "encrypted" "secret values are not stored in plaintext"
  fi
else
  assert_eq "no" "yes" "encrypted project secrets file exists"
fi

# ===========================================================================
# LIST SECRETS
# ===========================================================================
section "List secrets"

LIST_OUT=$(run_vmsan secrets list --project "$PROJECT" 2>&1 || echo "")
assert_not_empty "$LIST_OUT" "secrets list produces output"
assert_contains "$LIST_OUT" "E2E_SECRET_A" "E2E_SECRET_A appears in list"
assert_contains "$LIST_OUT" "E2E_SECRET_B" "E2E_SECRET_B appears in list"

# Values should be masked or shown depending on implementation
# Just verify the keys are present

# ===========================================================================
# OVERWRITE SECRET
# ===========================================================================
section "Overwrite secret"

OVERWRITE_OUT=$(run_vmsan secrets set E2E_SECRET_A "updated-value" --project "$PROJECT" 2>&1 || echo "FAIL")
if echo "$OVERWRITE_OUT" | grep -qi "error\|fail"; then
  assert_eq "failed" "success" "overwrite secret E2E_SECRET_A"
else
  assert_eq "success" "success" "overwrite secret E2E_SECRET_A"
fi

# ===========================================================================
# UNSET SECRET
# ===========================================================================
section "Unset secret"

UNSET_OUT=$(run_vmsan secrets unset E2E_SECRET_B --project "$PROJECT" 2>&1 || echo "FAIL")
if echo "$UNSET_OUT" | grep -qi "error\|fail"; then
  assert_eq "failed" "success" "unset secret E2E_SECRET_B"
else
  assert_eq "success" "success" "unset secret E2E_SECRET_B"
fi

# Verify it's gone from the list
LIST_AFTER=$(run_vmsan secrets list --project "$PROJECT" 2>&1 || echo "")
assert_not_contains "$LIST_AFTER" "E2E_SECRET_B" "E2E_SECRET_B removed from list"
assert_contains "$LIST_AFTER" "E2E_SECRET_A" "E2E_SECRET_A still in list"

# ===========================================================================
# PROJECT ISOLATION
# ===========================================================================
section "Secret project isolation"

# Set a secret in a different project
run_vmsan secrets set E2E_SECRET_C "other-project-value" --project "other-project" 2>/dev/null || true

# List from original project should NOT show the other project's secret
LIST_ISOLATED=$(run_vmsan secrets list --project "$PROJECT" 2>&1 || echo "")
assert_not_contains "$LIST_ISOLATED" "E2E_SECRET_C" "secrets are project-scoped"

# Cleanup the other project's secret
run_vmsan secrets unset E2E_SECRET_C --project "other-project" 2>/dev/null || true

# ===========================================================================
# UNSET REMAINING SECRETS
# ===========================================================================
section "Final cleanup"

run_vmsan secrets unset E2E_SECRET_A --project "$PROJECT" 2>/dev/null || true
LIST_FINAL=$(run_vmsan secrets list --project "$PROJECT" 2>&1 || echo "")
assert_not_contains "$LIST_FINAL" "E2E_SECRET_A" "all secrets cleaned up"

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
print_summary
