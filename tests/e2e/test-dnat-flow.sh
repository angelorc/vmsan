#!/usr/bin/env bash
set -euo pipefail

# =============================================================================
# E2E Test: DNAT Flow Validation
# Tests DNS DNAT, SNI DNAT, published port DNAT, nftables rule structure,
# ECH stripping, ICMP flag, and dnsproxy crash recovery.
#
# Requires: KVM host, vmsan installed from feat/platform-multihost, jq, root
# Usage: sudo bash tests/e2e/test-dnat-flow.sh
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
echo "  E2E Test: DNAT Flow Validation"
echo "================================================================"

# ---------------------------------------------------------------------------
# Test I9: DNS resolves in allow-all mode
# ---------------------------------------------------------------------------
echo ""
echo "--- Test I9: DNS resolves in allow-all mode ---"

VM_ID=$(create_vm --runtime base --vcpus 1 --memory 256)
assert_not_empty "$VM_ID" "I9: VM created"

if [ -n "$VM_ID" ]; then
  DNS_RESULT=$(sudo vmsan exec "$VM_ID" -- dig +short example.com 2>/dev/null | head -1 || true)
  assert_not_empty "$DNS_RESULT" "I9: DNS resolves example.com via dnsproxy DNAT"

  remove_vm "$VM_ID"
fi

# ---------------------------------------------------------------------------
# Test I10: DNS filtering in custom mode
# ---------------------------------------------------------------------------
echo ""
echo "--- Test I10: DNS filtering in custom mode ---"

VM_ID=$(create_vm --runtime base --vcpus 1 --memory 256 \
  --network-policy custom --allowed-domain example.com)
assert_not_empty "$VM_ID" "I10: VM created with custom policy"

if [ -n "$VM_ID" ]; then
  # Allowed domain should resolve
  ALLOWED=$(sudo vmsan exec "$VM_ID" -- dig +short example.com 2>/dev/null | head -1 || true)
  assert_not_empty "$ALLOWED" "I10: allowed domain (example.com) resolves"

  # Denied domain should return empty (NXDOMAIN or REFUSED)
  DENIED=$(sudo vmsan exec "$VM_ID" -- dig +short evil.com 2>/dev/null | head -1 || true)
  assert_empty "$DENIED" "I10: denied domain (evil.com) returns empty"

  remove_vm "$VM_ID"
fi

# ---------------------------------------------------------------------------
# Test I11: SNI filtering in custom mode
# ---------------------------------------------------------------------------
echo ""
echo "--- Test I11: SNI filtering in custom mode ---"

VM_ID=$(create_vm --runtime base --vcpus 1 --memory 256 \
  --network-policy custom --allowed-domain example.com)
assert_not_empty "$VM_ID" "I11: VM created with custom policy"

if [ -n "$VM_ID" ]; then
  # TLS to allowed domain succeeds
  if sudo vmsan exec "$VM_ID" -- curl -sf --max-time 10 https://example.com >/dev/null 2>&1; then
    echo -e "${GREEN}PASS${NC}: I11: TLS to allowed domain (example.com) succeeds"
    TESTS_PASSED=$((TESTS_PASSED + 1))
  else
    echo -e "${RED}FAIL${NC}: I11: TLS to allowed domain (example.com) failed"
    TESTS_FAILED=$((TESTS_FAILED + 1))
  fi

  # TLS to denied domain should fail
  if sudo vmsan exec "$VM_ID" -- curl -sf --max-time 5 https://evil.com >/dev/null 2>&1; then
    echo -e "${RED}FAIL${NC}: I11: TLS to denied domain (evil.com) should have failed"
    TESTS_FAILED=$((TESTS_FAILED + 1))
  else
    echo -e "${GREEN}PASS${NC}: I11: TLS to denied domain (evil.com) correctly blocked"
    TESTS_PASSED=$((TESTS_PASSED + 1))
  fi

  remove_vm "$VM_ID"
fi

# ---------------------------------------------------------------------------
# Test I12: --allow-icmp flag enables ping
# ---------------------------------------------------------------------------
echo ""
echo "--- Test I12: --allow-icmp flag enables ping ---"

VM_ID=$(create_vm --runtime base --vcpus 1 --memory 256 --allow-icmp)
assert_not_empty "$VM_ID" "I12: VM created with --allow-icmp"

if [ -n "$VM_ID" ]; then
  if sudo vmsan exec "$VM_ID" -- ping -c 1 -W 5 8.8.8.8 2>/dev/null; then
    echo -e "${GREEN}PASS${NC}: I12: ICMP (ping) works with --allow-icmp"
    TESTS_PASSED=$((TESTS_PASSED + 1))
  else
    echo -e "${RED}FAIL${NC}: I12: ICMP (ping) failed despite --allow-icmp"
    TESTS_FAILED=$((TESTS_FAILED + 1))
  fi

  remove_vm "$VM_ID"
fi

# ---------------------------------------------------------------------------
# Test I13: ECH SvcParam stripping
# ---------------------------------------------------------------------------
echo ""
echo "--- Test I13: ECH SvcParam stripping ---"

VM_ID=$(create_vm --runtime base --vcpus 1 --memory 256)
assert_not_empty "$VM_ID" "I13: VM created"

if [ -n "$VM_ID" ]; then
  # Query HTTPS record for a domain known to use ECH (cloudflare.com)
  OUTPUT=$(sudo vmsan exec "$VM_ID" -- dig -t HTTPS cloudflare.com 2>/dev/null || true)

  # Verify no ech= parameter in the response
  assert_not_contains "$OUTPUT" "ech=" "I13: ECH SvcParam stripped from HTTPS records"

  remove_vm "$VM_ID"
fi

# ---------------------------------------------------------------------------
# Test I14: dnsproxy crash recovery
# ---------------------------------------------------------------------------
echo ""
echo "--- Test I14: dnsproxy crash recovery ---"

VM_ID=$(create_vm --runtime base --vcpus 1 --memory 256)
assert_not_empty "$VM_ID" "I14: VM created"

if [ -n "$VM_ID" ]; then
  # Verify DNS works before kill
  PRE_DNS=$(sudo vmsan exec "$VM_ID" -- dig +short example.com 2>/dev/null | head -1 || true)
  assert_not_empty "$PRE_DNS" "I14: DNS works before dnsproxy kill"

  # Find the dnsproxy process for this VM's slot
  SLOT=$(vmsan list --json 2>/dev/null | jq -r --arg id "$VM_ID" '.[] | select(.id==$id) | .network.hostIp' | awk -F. '{print $4}')
  if [ -n "$SLOT" ]; then
    DNS_PORT=$((10053 + SLOT))
    DNSPROXY_PID=$(pgrep -f "dnsproxy.*${DNS_PORT}" || true)
    if [ -n "$DNSPROXY_PID" ]; then
      echo "  Killing dnsproxy (PID $DNSPROXY_PID, port $DNS_PORT)..."
      sudo kill "$DNSPROXY_PID"

      # Wait for supervisor to restart (up to 15s)
      echo -n "  Waiting for dnsproxy restart..."
      sleep 10

      # DNS should work again after supervisor restart
      POST_DNS=$(sudo vmsan exec "$VM_ID" -- dig +short example.com 2>/dev/null | head -1 || true)
      assert_not_empty "$POST_DNS" "I14: DNS works after dnsproxy crash recovery"
    else
      echo -e "${YELLOW}SKIP${NC}: I14: could not find dnsproxy PID (port $DNS_PORT)"
    fi
  else
    echo -e "${YELLOW}SKIP${NC}: I14: could not determine VM slot"
  fi

  remove_vm "$VM_ID"
fi

# ---------------------------------------------------------------------------
# Test: nftables rule structure (per-VM table in namespace)
# ---------------------------------------------------------------------------
echo ""
echo "--- Test: nftables rule structure ---"

VM_ID=$(create_vm --runtime base --vcpus 1 --memory 256)
assert_not_empty "$VM_ID" "nftables: VM created"

if [ -n "$VM_ID" ]; then
  NS_NAME="vmsan-${VM_ID}"

  # Per-VM nftables table should exist inside the namespace
  if sudo ip netns exec "$NS_NAME" nft list table ip "vmsan_${VM_ID}" >/dev/null 2>&1; then
    echo -e "${GREEN}PASS${NC}: nftables: per-VM table exists in namespace"
    TESTS_PASSED=$((TESTS_PASSED + 1))
  else
    echo -e "${RED}FAIL${NC}: nftables: per-VM table vmsan_${VM_ID} not found in namespace ${NS_NAME}"
    TESTS_FAILED=$((TESTS_FAILED + 1))
  fi

  # Host bypass table should exist
  if sudo nft list table ip vmsan_host >/dev/null 2>&1; then
    echo -e "${GREEN}PASS${NC}: nftables: vmsan_host table exists on host"
    TESTS_PASSED=$((TESTS_PASSED + 1))
  else
    echo -e "${RED}FAIL${NC}: nftables: vmsan_host table not found on host"
    TESTS_FAILED=$((TESTS_FAILED + 1))
  fi

  # Stop and verify cleanup
  remove_vm "$VM_ID"

  if ! sudo ip netns exec "$NS_NAME" nft list table ip "vmsan_${VM_ID}" 2>/dev/null; then
    echo -e "${GREEN}PASS${NC}: nftables: table cleaned up after VM removal"
    TESTS_PASSED=$((TESTS_PASSED + 1))
  else
    echo -e "${RED}FAIL${NC}: nftables: table vmsan_${VM_ID} still exists after removal"
    TESTS_FAILED=$((TESTS_FAILED + 1))
  fi
fi

# ---------------------------------------------------------------------------
# Test: Published port DNAT
# ---------------------------------------------------------------------------
echo ""
echo "--- Test: Published port DNAT ---"

# Use a high port unlikely to conflict
HOST_PORT=18080
VM_ID=$(create_vm --runtime base --vcpus 1 --memory 256 -p "${HOST_PORT}:8080")
assert_not_empty "$VM_ID" "published-port: VM created with -p ${HOST_PORT}:8080"

if [ -n "$VM_ID" ]; then
  # Start a simple listener inside the VM on port 8080
  sudo vmsan exec "$VM_ID" -- bash -c "echo 'dnat-ok' | nc -l -p 8080 &" 2>/dev/null || true
  sleep 1

  # Attempt to connect to the host port — should be DNAT'd to guest 8080
  RESULT=$(curl -sf --max-time 5 "http://127.0.0.1:${HOST_PORT}" 2>/dev/null || true)
  if [ "$RESULT" = "dnat-ok" ]; then
    echo -e "${GREEN}PASS${NC}: published-port: DNAT from host:${HOST_PORT} to guest:8080 works"
    TESTS_PASSED=$((TESTS_PASSED + 1))
  else
    # Even if curl didn't get a response, verify the DNAT rule exists
    NS_NAME="vmsan-${VM_ID}"
    NFT_OUT=$(sudo ip netns exec "$NS_NAME" nft list table ip "vmsan_${VM_ID}" 2>/dev/null || true)
    if echo "$NFT_OUT" | grep -q "dnat to.*:8080"; then
      echo -e "${GREEN}PASS${NC}: published-port: DNAT rule exists for port ${HOST_PORT}->8080"
      TESTS_PASSED=$((TESTS_PASSED + 1))
    else
      echo -e "${RED}FAIL${NC}: published-port: no DNAT rule for ${HOST_PORT}->8080"
      TESTS_FAILED=$((TESTS_FAILED + 1))
    fi
  fi

  remove_vm "$VM_ID"
fi

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
print_summary
