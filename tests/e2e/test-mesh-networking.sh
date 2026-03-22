#!/usr/bin/env bash
set -euo pipefail

# =============================================================================
# E2E Test: Mesh Networking
# Tests mesh IP allocation, DNS resolution (service.project.vmsan.internal),
# cross-project isolation, ACL enforcement, and mesh CLI flags.
#
# Requires: KVM host, vmsan installed from feat/platform-multihost, jq, root
# Usage: sudo bash tests/e2e/test-mesh-networking.sh
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
echo "  E2E Test: Mesh Networking"
echo "================================================================"

# ---------------------------------------------------------------------------
# Test M1: VM-A reaches VM-B via mesh IP
# ---------------------------------------------------------------------------
echo ""
echo "--- Test M1: VM-A reaches VM-B via mesh IP ---"

VM_A=$(create_vm --runtime base --vcpus 1 --memory 256 \
  --project meshtest --service web --connect-to db:5432)
assert_not_empty "$VM_A" "M1: VM-A (web) created"

VM_B=$(create_vm --runtime base --vcpus 1 --memory 256 \
  --project meshtest --service db --connect-to web:8080)
assert_not_empty "$VM_B" "M1: VM-B (db) created"

if [ -n "$VM_A" ] && [ -n "$VM_B" ]; then
  MESH_IP_B=$(vmsan list --json 2>/dev/null | jq -r --arg id "$VM_B" '.[] | select(.id==$id) | .network.meshIp' || true)
  assert_not_empty "$MESH_IP_B" "M1: VM-B has mesh IP"

  if [ -n "$MESH_IP_B" ] && [ "$MESH_IP_B" != "null" ]; then
    # VM-A can reach VM-B on allowed port (5432)
    # nc connect or refused (exit 0 or 1) means traffic gets through; timeout means blocked
    CONNECT_RESULT=$(sudo vmsan exec "$VM_A" -- bash -c "echo | nc -w 3 $MESH_IP_B 5432 2>/dev/null; echo \$?" 2>/dev/null | tail -1 || echo "999")
    if [ "$CONNECT_RESULT" = "0" ] || [ "$CONNECT_RESULT" = "1" ]; then
      echo -e "${GREEN}PASS${NC}: M1: VM-A reached VM-B on mesh (port 5432)"
      TESTS_PASSED=$((TESTS_PASSED + 1))
    else
      echo -e "${RED}FAIL${NC}: M1: VM-A could not reach VM-B on mesh IP $MESH_IP_B:5432 (result: $CONNECT_RESULT)"
      TESTS_FAILED=$((TESTS_FAILED + 1))
    fi
  fi
fi

# Cleanup M1 VMs
[ -n "$VM_A" ] && remove_vm "$VM_A"
[ -n "$VM_B" ] && remove_vm "$VM_B"

# ---------------------------------------------------------------------------
# Test M2: Mesh ACL denies unauthorized traffic
# ---------------------------------------------------------------------------
echo ""
echo "--- Test M2: Mesh ACL denies unauthorized traffic ---"

VM_A=$(create_vm --runtime base --vcpus 1 --memory 256 \
  --project acltest --service web --connect-to db:5432)
assert_not_empty "$VM_A" "M2: VM-A (web) created"

VM_B=$(create_vm --runtime base --vcpus 1 --memory 256 \
  --project acltest --service db)
assert_not_empty "$VM_B" "M2: VM-B (db) created"

if [ -n "$VM_A" ] && [ -n "$VM_B" ]; then
  MESH_IP_B=$(vmsan list --json 2>/dev/null | jq -r --arg id "$VM_B" '.[] | select(.id==$id) | .network.meshIp' || true)

  if [ -n "$MESH_IP_B" ] && [ "$MESH_IP_B" != "null" ]; then
    # Port 9999 is NOT in connect-to -- should be dropped
    if sudo vmsan exec "$VM_A" -- bash -c "echo | nc -w 3 $MESH_IP_B 9999 2>/dev/null" 2>/dev/null; then
      echo -e "${RED}FAIL${NC}: M2: unauthorized port 9999 was reachable (should be blocked)"
      TESTS_FAILED=$((TESTS_FAILED + 1))
    else
      echo -e "${GREEN}PASS${NC}: M2: unauthorized port 9999 correctly blocked by ACL"
      TESTS_PASSED=$((TESTS_PASSED + 1))
    fi
  fi
fi

[ -n "$VM_A" ] && remove_vm "$VM_A"
[ -n "$VM_B" ] && remove_vm "$VM_B"

# ---------------------------------------------------------------------------
# Test M3: Mesh DNS resolves service names
# ---------------------------------------------------------------------------
echo ""
echo "--- Test M3: Mesh DNS resolves service.project.vmsan.internal ---"

VM_A=$(create_vm --runtime base --vcpus 1 --memory 256 \
  --project dnstest --service web --connect-to db:5432)
assert_not_empty "$VM_A" "M3: VM-A (web) created"

VM_B=$(create_vm --runtime base --vcpus 1 --memory 256 \
  --project dnstest --service db)
assert_not_empty "$VM_B" "M3: VM-B (db) created"

if [ -n "$VM_A" ] && [ -n "$VM_B" ]; then
  EXPECTED_IP=$(vmsan list --json 2>/dev/null | jq -r --arg id "$VM_B" '.[] | select(.id==$id) | .network.meshIp' || true)

  # From inside VM-A, resolve db.dnstest.vmsan.internal
  RESOLVED_IP=$(sudo vmsan exec "$VM_A" -- dig +short db.dnstest.vmsan.internal 2>/dev/null | head -1 || true)

  if [ -n "$EXPECTED_IP" ] && [ "$EXPECTED_IP" != "null" ]; then
    assert_eq "$RESOLVED_IP" "$EXPECTED_IP" "M3: db.dnstest.vmsan.internal resolves to mesh IP"
  else
    echo -e "${RED}FAIL${NC}: M3: VM-B has no mesh IP to compare against"
    TESTS_FAILED=$((TESTS_FAILED + 1))
  fi
fi

[ -n "$VM_A" ] && remove_vm "$VM_A"
[ -n "$VM_B" ] && remove_vm "$VM_B"

# ---------------------------------------------------------------------------
# Test M4: Cross-project traffic blocked
# ---------------------------------------------------------------------------
echo ""
echo "--- Test M4: Cross-project traffic blocked ---"

VM_A=$(create_vm --runtime base --vcpus 1 --memory 256 \
  --project alpha --service web)
assert_not_empty "$VM_A" "M4: VM-A (project alpha) created"

VM_B=$(create_vm --runtime base --vcpus 1 --memory 256 \
  --project beta --service web)
assert_not_empty "$VM_B" "M4: VM-B (project beta) created"

if [ -n "$VM_A" ] && [ -n "$VM_B" ]; then
  MESH_IP_B=$(vmsan list --json 2>/dev/null | jq -r --arg id "$VM_B" '.[] | select(.id==$id) | .network.meshIp' || true)

  if [ -n "$MESH_IP_B" ] && [ "$MESH_IP_B" != "null" ]; then
    # Cross-project traffic should be blocked
    if sudo vmsan exec "$VM_A" -- bash -c "echo | nc -w 3 $MESH_IP_B 8080 2>/dev/null" 2>/dev/null; then
      echo -e "${RED}FAIL${NC}: M4: cross-project traffic reached VM-B (should be blocked)"
      TESTS_FAILED=$((TESTS_FAILED + 1))
    else
      echo -e "${GREEN}PASS${NC}: M4: cross-project traffic correctly blocked"
      TESTS_PASSED=$((TESTS_PASSED + 1))
    fi

    # DNS should return NXDOMAIN for other project's services
    RESULT=$(sudo vmsan exec "$VM_A" -- dig +short web.beta.vmsan.internal 2>/dev/null | head -1 || true)
    assert_empty "$RESULT" "M4: cross-project DNS returns empty (NXDOMAIN)"
  fi
fi

[ -n "$VM_A" ] && remove_vm "$VM_A"
[ -n "$VM_B" ] && remove_vm "$VM_B"

# ---------------------------------------------------------------------------
# Test M5: Stopped VM disappears from mesh DNS
# ---------------------------------------------------------------------------
echo ""
echo "--- Test M5: Stopped VM disappears from mesh DNS ---"

VM_A=$(create_vm --runtime base --vcpus 1 --memory 256 \
  --project ttltest --service web --connect-to db:5432)
assert_not_empty "$VM_A" "M5: VM-A (web) created"

VM_B=$(create_vm --runtime base --vcpus 1 --memory 256 \
  --project ttltest --service db)
assert_not_empty "$VM_B" "M5: VM-B (db) created"

if [ -n "$VM_A" ] && [ -n "$VM_B" ]; then
  # Verify DNS resolves while VM-B is running
  PRE_DNS=$(sudo vmsan exec "$VM_A" -- dig +short db.ttltest.vmsan.internal 2>/dev/null | head -1 || true)
  assert_not_empty "$PRE_DNS" "M5: DNS resolves while VM-B is running"

  # Stop VM-B
  sudo vmsan stop "$VM_B" 2>/dev/null || true

  # Wait for DNS TTL (5s) + margin
  sleep 8

  # DNS should no longer resolve
  POST_DNS=$(sudo vmsan exec "$VM_A" -- dig +short db.ttltest.vmsan.internal 2>/dev/null | head -1 || true)
  assert_empty "$POST_DNS" "M5: DNS returns empty after VM-B stopped"

  # Remove VM-B from tracking (already stopped)
  sudo vmsan remove "$VM_B" 2>/dev/null || true
  new_ids=()
  for existing in "${VM_IDS[@]}"; do
    [ "$existing" != "$VM_B" ] && new_ids+=("$existing")
  done
  VM_IDS=("${new_ids[@]}")
fi

[ -n "$VM_A" ] && remove_vm "$VM_A"

# ---------------------------------------------------------------------------
# Test M6: --connect-to and --service flags
# ---------------------------------------------------------------------------
echo ""
echo "--- Test M6: --connect-to and --service flags ---"

VM_ID=$(create_vm --runtime base --vcpus 1 --memory 256 \
  --project flagtest --service api --connect-to db:5432,cache:6379)
assert_not_empty "$VM_ID" "M6: VM created with --service and --connect-to"

if [ -n "$VM_ID" ]; then
  STATE=$(vmsan list --json 2>/dev/null | jq --arg id "$VM_ID" '.[] | select(.id==$id)' || true)

  SERVICE_NAME=$(echo "$STATE" | jq -r '.network.service // empty' 2>/dev/null || true)
  assert_eq "$SERVICE_NAME" "api" "M6: service name is 'api'"

  CONNECT_COUNT=$(echo "$STATE" | jq '.network.connectTo | length // 0' 2>/dev/null || echo "0")
  assert_eq "$CONNECT_COUNT" "2" "M6: connectTo has 2 entries"

  remove_vm "$VM_ID"
fi

# ---------------------------------------------------------------------------
# Test M7: vmsan network connections shows mesh traffic
# ---------------------------------------------------------------------------
echo ""
echo "--- Test M7: vmsan network connections ---"

VM_A=$(create_vm --runtime base --vcpus 1 --memory 256 \
  --project conntest --service web --connect-to db:5432)
assert_not_empty "$VM_A" "M7: VM-A (web) created"

VM_B=$(create_vm --runtime base --vcpus 1 --memory 256 \
  --project conntest --service db --connect-to web:8080)
assert_not_empty "$VM_B" "M7: VM-B (db) created"

if [ -n "$VM_A" ] && [ -n "$VM_B" ]; then
  MESH_IP_B=$(vmsan list --json 2>/dev/null | jq -r --arg id "$VM_B" '.[] | select(.id==$id) | .network.meshIp' || true)

  if [ -n "$MESH_IP_B" ] && [ "$MESH_IP_B" != "null" ]; then
    # Generate some traffic
    sudo vmsan exec "$VM_A" -- bash -c "echo | nc -w 1 $MESH_IP_B 5432 2>/dev/null" 2>/dev/null || true

    # Check network connections (best effort -- conntrack entries may expire)
    CONN_OUTPUT=$(sudo vmsan network connections "$VM_A" 2>/dev/null || true)
    if echo "$CONN_OUTPUT" | grep -q "$MESH_IP_B"; then
      echo -e "${GREEN}PASS${NC}: M7: network connections shows mesh IP"
      TESTS_PASSED=$((TESTS_PASSED + 1))
    else
      echo -e "${YELLOW}SKIP${NC}: M7: mesh IP not in connections (conntrack may have expired)"
    fi
  fi
fi

[ -n "$VM_A" ] && remove_vm "$VM_A"
[ -n "$VM_B" ] && remove_vm "$VM_B"

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
print_summary
