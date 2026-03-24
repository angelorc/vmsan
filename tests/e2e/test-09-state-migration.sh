#!/usr/bin/env bash
set -euo pipefail

# =============================================================================
# E2E Test 09: State & Migration
# Tests SQLite as the default state backend, JSON fallback, and migration.
#
# Requires: KVM host, vmsan installed, python3
# Usage: sudo bash tests/e2e/test-09-state-migration.sh
# =============================================================================

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/lib/helpers.sh"

VM_IDS=()

cleanup() {
  echo ""
  echo "--- Cleanup ---"
  for id in "${VM_IDS[@]}"; do
    [ -n "$id" ] || continue
    run_vmsan_env VMSAN_STATE_BACKEND=json remove --force "$id" 2>/dev/null || true
    force_remove_vm "$id"
  done
}
trap cleanup EXIT

echo "================================================================"
echo "  E2E Test 09: State & Migration"
echo "================================================================"

# ---------------------------------------------------------------------------
# Test: SQLite is the default backend
# ---------------------------------------------------------------------------
section "SQLite default backend"

VM_SQLITE=$(create_vm --runtime base --vcpus 1 --memory 128)
assert_not_empty "$VM_SQLITE" "VM created with default backend"

if [ -n "$VM_SQLITE" ]; then
  sleep 5
  if [ -f "$VMSAN_STATE_DB" ]; then
    assert_eq "yes" "yes" "state.db exists at $VMSAN_STATE_DB"
  else
    assert_eq "no" "yes" "state.db exists at $VMSAN_STATE_DB"
  fi

  SQLITE_IDS=$(sqlite_query "SELECT id FROM vms WHERE id = '${VM_SQLITE}'" 2>/dev/null || echo "")
  assert_contains "$SQLITE_IDS" "$VM_SQLITE" "default backend persists VM in SQLite"
fi

# ---------------------------------------------------------------------------
# Test: JSON fallback backend
# ---------------------------------------------------------------------------
section "JSON fallback backend"

VM_JSON=$(create_vm_env VMSAN_STATE_BACKEND=json --runtime base --vcpus 1 --memory 128)
assert_not_empty "$VM_JSON" "VM created with JSON backend fallback"

if [ -n "$VM_JSON" ]; then
  sleep 5
  JSON_STATE_FILE="${VMSAN_DIR}/vms/${VM_JSON}.json"
  if [ -f "$JSON_STATE_FILE" ]; then
    assert_eq "yes" "yes" "JSON backend writes ${JSON_STATE_FILE}"
  else
    assert_eq "no" "yes" "JSON backend writes ${JSON_STATE_FILE}"
  fi
fi

# ---------------------------------------------------------------------------
# Test: Migration dry-run
# ---------------------------------------------------------------------------
section "vmsan migrate --dry-run"

DRY_RUN_OUT=$(run_vmsan migrate --dry-run 2>&1 || echo "")
assert_not_empty "$DRY_RUN_OUT" "migrate --dry-run produces output"

if [ -n "${VM_JSON:-}" ]; then
  assert_contains "$DRY_RUN_OUT" "$VM_JSON" "dry-run mentions JSON-backed VM"
fi

# ---------------------------------------------------------------------------
# Test: JSON -> SQLite migration
# ---------------------------------------------------------------------------
section "JSON to SQLite migration"

if [ -n "${VM_JSON:-}" ]; then
  MIGRATE_OUT=$(run_as_cli_shell "printf 'y\n' | vmsan migrate" 2>&1 || echo "")
  assert_not_empty "$MIGRATE_OUT" "migrate command produces output"

  MIGRATED_IDS=$(sqlite_query "SELECT id FROM vms WHERE id = '${VM_JSON}'" 2>/dev/null || echo "")
  assert_contains "$MIGRATED_IDS" "$VM_JSON" "migration imports JSON VM into SQLite"
else
  skip_test "JSON backend VM was not created"
fi

# ---------------------------------------------------------------------------
# Test: Cleanup works across backends
# ---------------------------------------------------------------------------
section "Cross-backend cleanup"

if [ -n "${VM_JSON:-}" ]; then
  run_vmsan_env VMSAN_STATE_BACKEND=json remove --force "$VM_JSON" 2>/dev/null || true
  run_vmsan remove --force "$VM_JSON" 2>/dev/null || true
  new_ids=()
  for existing in "${VM_IDS[@]}"; do
    [ "$existing" != "$VM_JSON" ] && new_ids+=("$existing")
  done
  VM_IDS=("${new_ids[@]}")
  JSON_LEFT=$(get_vm_state_json "$VM_JSON")
  assert_empty "$JSON_LEFT" "JSON-backed VM removed from state stores"
fi

if [ -n "${VM_SQLITE:-}" ]; then
  remove_vm "$VM_SQLITE"
fi

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
print_summary
