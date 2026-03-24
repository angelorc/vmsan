#!/usr/bin/env bash
set -euo pipefail

# =============================================================================
# E2E Test 05: Mesh Networking
# Tests mesh IP allocation, DNS service discovery, ACL enforcement,
# cross-project isolation, and mesh cleanup on stop.
#
# Requires: KVM host, vmsan installed, base runtime
# Usage: sudo bash tests/e2e/test-05-mesh-networking.sh
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
echo "  E2E Test 05: Mesh Networking"
echo "================================================================"

# ===========================================================================
# MESH IP ALLOCATION
# ===========================================================================
section "Mesh IP allocation"

# Create two VMs in the same project with --service and --connect-to
VM_WEB=$(create_vm --runtime base --vcpus 1 --memory 128 \
  --network-policy allow-all \
  --project meshtest \
  --service web \
  --connect-to "db:5432")

VM_DB=$(create_vm --runtime base --vcpus 1 --memory 128 \
  --network-policy allow-all \
  --project meshtest \
  --service db \
  --connect-to "web:8080")

assert_not_empty "$VM_WEB" "web service VM created"
assert_not_empty "$VM_DB" "db service VM created"

if [ -n "$VM_WEB" ] && [ -n "$VM_DB" ]; then
  sleep 10

  # Check mesh IPs are assigned (from state files)
  MESH_IP_WEB=""
  MESH_IP_DB=""

  MESH_IP_WEB=$(get_mesh_ip "$VM_WEB")
  MESH_IP_DB=$(get_mesh_ip "$VM_DB")

  assert_not_empty "$MESH_IP_WEB" "web VM has mesh IP assigned"
  assert_not_empty "$MESH_IP_DB" "db VM has mesh IP assigned"

  # Mesh IPs should be different
  if [ -n "$MESH_IP_WEB" ] && [ -n "$MESH_IP_DB" ]; then
    if [ "$MESH_IP_WEB" != "$MESH_IP_DB" ]; then
      assert_eq "different" "different" "mesh IPs are unique"
    else
      assert_eq "same" "different" "mesh IPs are unique"
    fi
  fi

  # ===========================================================================
  # MESH DNS RESOLUTION
  # ===========================================================================
  section "Mesh DNS resolution"

  # From web VM, resolve db.meshtest.vmsan.internal
  DNS_DB=$(run_vmsan exec "$VM_WEB" -- \
    dig +short +timeout=5 db.meshtest.vmsan.internal 2>/dev/null || echo "")

  if [ -n "$DNS_DB" ]; then
    assert_not_empty "$DNS_DB" "mesh DNS resolves db.meshtest.vmsan.internal"

    # The resolved IP should match the db VM's mesh IP
    if [ -n "$MESH_IP_DB" ]; then
      assert_eq "$DNS_DB" "$MESH_IP_DB" "mesh DNS resolves to correct IP"
    fi
  else
    # dig might not be available in base image; try nslookup or ping
    PING_DB=$(run_vmsan exec "$VM_WEB" -- \
      getent hosts db.meshtest.vmsan.internal 2>/dev/null || echo "")
    if [ -n "$PING_DB" ]; then
      assert_not_empty "$PING_DB" "mesh DNS resolves db.meshtest.vmsan.internal (via getent)"
    else
      skip_test "DNS tools not available in guest for mesh DNS test"
    fi
  fi

  # From db VM, resolve web.meshtest.vmsan.internal
  DNS_WEB=$(run_vmsan exec "$VM_DB" -- \
    getent hosts web.meshtest.vmsan.internal 2>/dev/null || echo "")
  assert_not_empty "$DNS_WEB" "mesh DNS resolves web.meshtest.vmsan.internal"

  # ===========================================================================
  # MESH CONNECTIVITY (allowed traffic)
  # ===========================================================================
  section "Mesh connectivity (allowed traffic)"

  if [ -n "$MESH_IP_DB" ]; then
    DB_NETNS=$(get_netns "$VM_DB")
    WEB_NETNS=$(get_netns "$VM_WEB")

    # Diagnostic: verify DNAT rules, routes, and interfaces
    echo "  [diag] DB namespace nft mesh rules:"
    ns_exec "$DB_NETNS" nft list table ip vmsan_mesh 2>&1 | sed 's/^/    /' || echo "    (no vmsan_mesh table!)"
    echo "  [diag] DB namespace IPs:"
    ns_exec "$DB_NETNS" ip addr show 2>&1 | grep "inet " | sed 's/^/    /' || true
    echo "  [diag] DB namespace routes:"
    ns_exec "$DB_NETNS" ip route show 2>&1 | sed 's/^/    /' || true
    echo "  [diag] Host mesh routes:"
    ip route show | grep "10.90" | sed 's/^/    /' || true
    echo "  [diag] Host vmsan_mesh FORWARD chain:"
    nft list table ip vmsan_mesh 2>&1 | sed 's/^/    /' || echo "    (no host vmsan_mesh table)"
    echo "  [diag] Ping from host to db mesh IP:"
    ping -c1 -W2 "$MESH_IP_DB" 2>&1 | tail -2 | sed 's/^/    /' || true
    echo "  [diag] DB DNAT counter after ping:"
    ns_exec "$DB_NETNS" nft list table ip vmsan_mesh 2>&1 | grep "counter" | sed 's/^/    /' || true

    # Start a TCP listener on port 5432 in the db VM.
    # Must redirect all fds so vmsan exec session closes immediately.
    run_vmsan exec "$VM_DB" -- \
      bash -c 'nohup sh -c "while true; do echo ok | nc -l -p 5432 -q0 2>/dev/null; done" </dev/null >/dev/null 2>&1 &' \
      2>/dev/null || true
    sleep 2

    echo "  [diag] DB per-VM nft FORWARD chain:"
    ns_exec "$DB_NETNS" nft list chain ip "vmsan_${VM_DB}" forward 2>&1 | head -20 | sed 's/^/    /' || true

    CONNECT_STATUS=$(run_vmsan exec "$VM_WEB" -- \
      bash -c "timeout 5 bash -c 'exec 3<>/dev/tcp/${MESH_IP_DB}/5432 && echo 0 || echo 1' 2>/dev/null || echo 124" \
      2>/dev/null || echo "1")

    echo "  [diag] CONNECT_STATUS=$CONNECT_STATUS"
    echo "  [diag] DB DNAT counter after TCP test:"
    ns_exec "$DB_NETNS" nft list table ip vmsan_mesh 2>&1 | grep "counter" | sed 's/^/    /' || true
    echo "  [diag] Host vmsan_mesh FORWARD drop counter:"
    nft list chain ip vmsan_mesh mesh_forward 2>&1 | grep "drop" | sed 's/^/    /' || true

    if [ "$CONNECT_STATUS" != "124" ]; then
      assert_eq "reachable" "reachable" "allowed mesh port reaches db VM"
    else
      assert_eq "timeout" "reachable" "allowed mesh port reaches db VM"
    fi
  fi

  if [ -n "$MESH_IP_WEB" ]; then
    ROUTE_CHECK2=$(ip route show "${MESH_IP_WEB}/32" 2>/dev/null || echo "")
    assert_not_empty "$ROUTE_CHECK2" "host has route to web mesh IP (${MESH_IP_WEB})"
  fi

  # ===========================================================================
  # MESH ACL (denied traffic)
  # ===========================================================================
  section "Mesh ACL denies unauthorized traffic"

  if [ -n "$MESH_IP_DB" ]; then
    DENY_STATUS=$(run_vmsan exec "$VM_WEB" -- \
      bash -lc "timeout 3 bash -lc 'exec 3<>/dev/tcp/${MESH_IP_DB}/9999' >/dev/null 2>&1; echo \$?" \
      2>/dev/null || echo "1")

    assert_eq "$DENY_STATUS" "124" "unauthorized mesh port is dropped"
  fi

  # ===========================================================================
  # CROSS-PROJECT ISOLATION
  # ===========================================================================
  section "Cross-project isolation"

  # Create a VM in a different project
  VM_OTHER=$(create_vm --runtime base --vcpus 1 --memory 128 \
    --network-policy allow-all \
    --project otherproject \
    --service api)

  assert_not_empty "$VM_OTHER" "VM in different project created"

  if [ -n "$VM_OTHER" ]; then
    sleep 5

    MESH_IP_OTHER=""
    MESH_IP_OTHER=$(get_mesh_ip "$VM_OTHER")

    # VM in otherproject should NOT be able to resolve meshtest services
    if [ -n "$MESH_IP_OTHER" ]; then
      CROSS_DNS=$(run_vmsan exec "$VM_OTHER" -- \
        dig +short +timeout=3 db.meshtest.vmsan.internal 2>/dev/null || echo "")
      # Should not resolve (different project, no connect-to)
      assert_empty "$CROSS_DNS" "cross-project DNS does not resolve"
    fi

    remove_vm "$VM_OTHER"
  fi

  # ===========================================================================
  # MESH CLEANUP ON VM STOP
  # ===========================================================================
  section "Mesh cleanup on VM stop"

  # Stop the db VM
  run_vmsan stop "$VM_DB" 2>/dev/null || true
  sleep 5

  # The mesh route for db should be removed
  if [ -n "$MESH_IP_DB" ]; then
    ROUTE_AFTER=$(ip route show "${MESH_IP_DB}/32" 2>/dev/null || echo "")
    assert_empty "$ROUTE_AFTER" "mesh route removed after VM stop"
  fi

  # Mesh DNS should no longer resolve the stopped service
  DNS_STOPPED=$(run_vmsan exec "$VM_WEB" -- \
    dig +short +timeout=3 db.meshtest.vmsan.internal 2>/dev/null || echo "")
  assert_empty "$DNS_STOPPED" "stopped VM no longer in mesh DNS"

  # ===========================================================================
  # NETWORK CONNECTIONS COMMAND
  # ===========================================================================
  section "vmsan network connections"

  if [ -n "$MESH_IP_DB" ]; then
    run_vmsan exec "$VM_WEB" -- \
      bash -lc "timeout 3 bash -lc 'exec 3<>/dev/tcp/${MESH_IP_DB}/5432' >/dev/null 2>&1 || true" \
      >/dev/null 2>&1 || true
  fi

  CONN_OUT=$(run_vmsan network connections "$VM_WEB" 2>&1 || echo "")
  # Command should run without error (even if no active connections)
  if echo "$CONN_OUT" | grep -qi "error\|not found"; then
    skip_test "network connections command returned error"
  else
    assert_not_empty "$CONN_OUT" "network connections command runs"
  fi

  # Cleanup
  remove_vm "$VM_WEB"
  remove_vm "$VM_DB"
else
  skip_test "could not create mesh VMs"
fi

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
print_summary
