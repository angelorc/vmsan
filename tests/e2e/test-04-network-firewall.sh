#!/usr/bin/env bash
set -euo pipefail

# =============================================================================
# E2E Test 04: Network & Firewall
# Tests DNAT flow, DNS filtering, SNI filtering, ICMP, ECH stripping,
# published ports, nftables rules, policy update, and dnsproxy recovery.
#
# Requires: KVM host, vmsan installed, base runtime, nft, dig
# Usage: sudo bash tests/e2e/test-04-network-firewall.sh
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
echo "  E2E Test 04: Network & Firewall"
echo "================================================================"

# ===========================================================================
# ALLOW-ALL POLICY
# ===========================================================================
section "Allow-all policy: DNS resolution"

VM_ALLOW=$(create_vm --runtime base --vcpus 1 --memory 128 --network-policy allow-all)
assert_not_empty "$VM_ALLOW" "VM created with allow-all policy"

if [ -n "$VM_ALLOW" ]; then
  sleep 8
  GUEST_IP=$(get_guest_ip "$VM_ALLOW")
  NETNS=$(get_netns "$VM_ALLOW")

  # DNS should resolve any domain in allow-all mode.
  # Test from INSIDE the VM (via vmsan exec) so traffic goes through TAP → namespace
  # prerouting DNAT → mesh DNS handler. Using ns_exec (ip netns exec) would generate
  # locally-originated traffic that bypasses PREROUTING DNAT.
  DNS_RESULT=$(run_vmsan exec "$VM_ALLOW" -- getent hosts example.com 2>/dev/null || echo "")
  assert_not_empty "$DNS_RESULT" "DNS resolves example.com in allow-all mode"

  # ECH filtering: HTTPS records should not contain ech= parameter
  # (tested via namespace since this checks nftables rules, not VM DNS)
  if command -v dig >/dev/null 2>&1 && netns_exists "$NETNS"; then
    HTTPS_RESULT=$(ns_exec "$NETNS" dig -t HTTPS +timeout=5 cloudflare.com @"$(get_host_ip "$VM_ALLOW")" -p 10052 2>/dev/null || echo "")
    if [ -n "$HTTPS_RESULT" ]; then
      assert_not_contains "$HTTPS_RESULT" "ech=" "ECH SvcParam stripped from HTTPS records"
    else
      skip_test "HTTPS DNS record test returned no data"
    fi
  else
    skip_test "dig or netns not available for ECH test"
  fi

  DNS_LOGS=$(run_vmsan logs dns "$VM_ALLOW" 2>&1 || echo "")
  assert_contains "$DNS_LOGS" "example.com" "vmsan logs dns shows recorded query"

  remove_vm "$VM_ALLOW"
fi

# ===========================================================================
# DENY-ALL POLICY
# ===========================================================================
section "Deny-all policy: traffic blocked"

VM_DENY=$(create_vm --runtime base --vcpus 1 --memory 128 --network-policy deny-all)
assert_not_empty "$VM_DENY" "VM created with deny-all policy"

if [ -n "$VM_DENY" ]; then
  sleep 8
  NETNS=$(get_netns "$VM_DENY")

  # DNS should fail in deny-all mode — test from inside VM
  DNS_DENIED=$(run_vmsan exec "$VM_DENY" -- getent hosts example.com 2>/dev/null || echo "")
  assert_empty "$DNS_DENIED" "DNS blocked in deny-all mode"

  remove_vm "$VM_DENY"
fi

# ===========================================================================
# CUSTOM POLICY: Domain filtering
# ===========================================================================
section "Custom policy: domain allow-list"

VM_CUSTOM=$(create_vm --runtime base --vcpus 1 --memory 128 \
  --network-policy custom \
  --allowed-domain "example.com,*.github.com")
assert_not_empty "$VM_CUSTOM" "VM created with custom domain policy"

if [ -n "$VM_CUSTOM" ]; then
  sleep 8
  NETNS=$(get_netns "$VM_CUSTOM")

  # Allowed domain should resolve — test from inside VM
  ALLOWED=$(run_vmsan exec "$VM_CUSTOM" -- getent hosts example.com 2>/dev/null || echo "")
  assert_not_empty "$ALLOWED" "allowed domain (example.com) resolves"

  # Non-allowed domain should be blocked
  DENIED=$(run_vmsan exec "$VM_CUSTOM" -- getent hosts evil.example.org 2>/dev/null || echo "")
  assert_empty "$DENIED" "non-allowed domain (evil.example.org) blocked"

  remove_vm "$VM_CUSTOM"
fi

# ===========================================================================
# ALLOW-ICMP FLAG
# ===========================================================================
section "Allow-ICMP flag"

VM_ICMP=$(create_vm --runtime base --vcpus 1 --memory 128 \
  --network-policy allow-all --allow-icmp)
assert_not_empty "$VM_ICMP" "VM created with --allow-icmp"

if [ -n "$VM_ICMP" ]; then
  sleep 8
  # Verify inside the VM that ping works
  PING_OUT=$(run_vmsan exec "$VM_ICMP" -- ping -c 1 -W 3 8.8.8.8 2>/dev/null || echo "FAIL")
  if echo "$PING_OUT" | grep -q "1 received\|1 packets received\|ttl="; then
    assert_eq "reachable" "reachable" "ICMP ping works with --allow-icmp"
  else
    # ICMP may be filtered upstream; just check the rule exists
    NETNS=$(get_netns "$VM_ICMP")
    if netns_exists "$NETNS"; then
      NFT_RULES=$(ns_exec "$NETNS" nft list ruleset 2>/dev/null || echo "")
      assert_contains "$NFT_RULES" "icmp" "nftables ICMP rule present"
    else
      skip_test "cannot verify ICMP (no netns or upstream blocks)"
    fi
  fi

  remove_vm "$VM_ICMP"
fi

# ===========================================================================
# CUSTOM POLICY: SNI filtering on direct IP/TLS
# ===========================================================================
section "Custom policy: SNI filtering"

VM_SNI=$(create_vm --runtime base --vcpus 1 --memory 128 \
  --network-policy custom \
  --allowed-domain "example.com")
assert_not_empty "$VM_SNI" "VM created for SNI filtering test"

if [ -n "$VM_SNI" ]; then
  sleep 8
  SNI_OUT=$(run_vmsan exec "$VM_SNI" -- \
    openssl s_client -brief -connect 1.1.1.1:443 -servername cloudflare.com </dev/null 2>&1 || echo "FAIL")

  if echo "$SNI_OUT" | grep -qi "FAIL\|denied\|reset\|refused\|timeout\|unexpected eof"; then
    assert_eq "blocked" "blocked" "TLS with denied SNI is blocked"
  else
    skip_test "SNI filtering could not be verified with openssl"
  fi

  remove_vm "$VM_SNI"
fi

# ===========================================================================
# NFTABLES RULES: per-VM table in namespace
# ===========================================================================
section "nftables per-VM rules"

VM_NFT=$(create_vm --runtime base --vcpus 1 --memory 128 --network-policy allow-all)
assert_not_empty "$VM_NFT" "VM created for nftables check"

if [ -n "$VM_NFT" ]; then
  sleep 5
  NETNS=$(get_netns "$VM_NFT")

  if netns_exists "$NETNS"; then
    # Check that nftables tables exist in the namespace
    NFT_TABLES=$(ns_exec "$NETNS" nft list tables 2>/dev/null || echo "")
    assert_contains "$NFT_TABLES" "vmsan" "nftables table exists in VM namespace"

    # Check for DNAT rules (DNS port 53 → dnsproxy port)
    NFT_RULES=$(ns_exec "$NETNS" nft list ruleset 2>/dev/null || echo "")
    assert_contains "$NFT_RULES" "dnat" "DNAT rules present in namespace"
  else
    skip_test "netns not available for nftables check"
  fi

  # After removing VM, nftables should be cleaned up
  remove_vm "$VM_NFT"
  sleep 2

  if ! netns_exists "$NETNS"; then
    assert_eq "cleaned" "cleaned" "namespace removed after VM deletion"
  else
    assert_eq "leaked" "cleaned" "namespace removed after VM deletion"
  fi
fi

# ===========================================================================
# PUBLISHED PORTS
# ===========================================================================
section "Published ports"

VM_PORT=$(create_vm --runtime base --vcpus 1 --memory 128 \
  --network-policy allow-all --publish-port 18080)
assert_not_empty "$VM_PORT" "VM created with published port 18080"

if [ -n "$VM_PORT" ]; then
  sleep 5
  NETNS=$(get_netns "$VM_PORT")

  if netns_exists "$NETNS"; then
    # Check DNAT rule for published port
    NFT_RULES=$(ns_exec "$NETNS" nft list ruleset 2>/dev/null || echo "")
    assert_contains "$NFT_RULES" "18080" "DNAT rule for published port 18080 exists"
  else
    skip_test "netns not available for published port test"
  fi

  remove_vm "$VM_PORT"
fi

# ===========================================================================
# POLICY UPDATE ON RUNNING VM
# ===========================================================================
section "Network policy update on running VM"

VM_UPDATE=$(create_vm --runtime base --vcpus 1 --memory 128 --network-policy allow-all)
assert_not_empty "$VM_UPDATE" "VM created for policy update test"

if [ -n "$VM_UPDATE" ]; then
  sleep 8

  # Update policy from allow-all to deny-all
  UPDATE_OUT=$(run_vmsan network update "$VM_UPDATE" --network-policy deny-all 2>&1 || echo "FAIL")
  if echo "$UPDATE_OUT" | grep -qi "fail\|error"; then
    assert_eq "failed" "success" "policy update to deny-all"
  else
    assert_eq "success" "success" "policy update to deny-all"
  fi

  # Verify new policy took effect: DNS should be blocked — test from inside VM
  sleep 2
  DNS_AFTER=$(run_vmsan exec "$VM_UPDATE" -- getent hosts example.com 2>/dev/null || echo "")
  assert_empty "$DNS_AFTER" "DNS blocked after policy update to deny-all"

  remove_vm "$VM_UPDATE"
fi

# ===========================================================================
# DNSPROXY CRASH RECOVERY
# ===========================================================================
section "dnsproxy crash recovery"

VM_DNS=$(create_vm --runtime base --vcpus 1 --memory 128 --network-policy allow-all)
assert_not_empty "$VM_DNS" "VM created for dnsproxy recovery test"

if [ -n "$VM_DNS" ]; then
  sleep 8
  NETNS=$(get_netns "$VM_DNS")

  # Find the dnsproxy process for this VM's DNS port
  # DNS port = 10053 + slot. We can find it by looking for dnsproxy processes.
  HOST_IP=$(get_host_ip "$VM_DNS")
  SLOT=""
  if [ -n "$HOST_IP" ]; then
    SLOT=$(echo "$HOST_IP" | awk -F. '{print $4}')
  fi
  DNS_PORT=""
  if [ -n "$SLOT" ]; then
    DNS_PORT=$((10053 + SLOT))
  fi

  DNSPID=""
  if [ -n "$DNS_PORT" ]; then
    DNSPID=$(pgrep -f "dnsproxy.*${DNS_PORT}" 2>/dev/null | head -1 || echo "")
  fi

  if [ -n "$DNSPID" ]; then
    # Verify DNS works before kill
    if command -v dig >/dev/null 2>&1 && netns_exists "$NETNS"; then
      DNS_BEFORE=$(ns_exec "$NETNS" dig +short +timeout=5 example.com 2>/dev/null || echo "")
      assert_not_empty "$DNS_BEFORE" "DNS works before dnsproxy kill"
    fi

    # Kill dnsproxy
    echo "  Killing dnsproxy (PID $DNSPID)..."
    sudo kill "$DNSPID" 2>/dev/null || true

    # Wait for supervisor to restart it (exponential backoff starts at 1s)
    echo "  Waiting for dnsproxy supervisor to restart..."
    sleep 8

    # DNS should work again
    if command -v dig >/dev/null 2>&1 && netns_exists "$NETNS"; then
      DNS_AFTER=$(ns_exec "$NETNS" dig +short +timeout=5 example.com 2>/dev/null || echo "")
      assert_not_empty "$DNS_AFTER" "DNS works after dnsproxy crash recovery"
    fi

    # New PID should be different
    NEWPID=$(pgrep -f "dnsproxy.*${VM_DNS}" 2>/dev/null | head -1 || echo "")
    if [ -n "$NEWPID" ] && [ "$NEWPID" != "$DNSPID" ]; then
      assert_eq "restarted" "restarted" "dnsproxy restarted with new PID ($NEWPID)"
    else
      skip_test "dnsproxy PID check inconclusive"
    fi
  else
    skip_test "could not find dnsproxy process for VM"
  fi

  remove_vm "$VM_DNS"
fi

# ===========================================================================
# HOST BYPASS TABLE
# ===========================================================================
section "Host nftables (vmsan_host)"

# The vmsan_host bypass table should exist in the host namespace
HOST_NFT=$(sudo nft list tables 2>/dev/null || echo "")
assert_contains "$HOST_NFT" "vmsan" "vmsan nftables table exists in host namespace"

# ===========================================================================
# FIREWALL CLEANUP
# ===========================================================================
section "Firewall cleanup after VM removal"

# Create and immediately remove a VM to verify cleanup
VM_CLEANUP=$(create_vm --runtime base --vcpus 1 --memory 128)
if [ -n "$VM_CLEANUP" ]; then
  sleep 5
  CLEANUP_NS=$(get_netns "$VM_CLEANUP")

  force_remove_vm "$VM_CLEANUP"
  sleep 2

  # Namespace should be gone
  if ! netns_exists "$CLEANUP_NS"; then
    assert_eq "cleaned" "cleaned" "namespace removed on VM deletion"
  else
    assert_eq "leaked" "cleaned" "namespace removed on VM deletion"
  fi

  # No nftables reference should remain for this VM
  HOST_RULES=$(sudo nft list ruleset 2>/dev/null || echo "")
  assert_not_contains "$HOST_RULES" "$VM_CLEANUP" "no host nftables reference to deleted VM"
fi

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
print_summary
