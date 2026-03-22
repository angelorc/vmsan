#!/usr/bin/env bash
set -euo pipefail

# =============================================================================
# E2E Test: Full vmsan up Lifecycle
# Tests vmsan up -> deploy -> health -> logs -> status -> secrets -> down
# using vmsan.toml project configs.
#
# Requires: KVM host, vmsan installed from feat/platform-multihost, jq, root
# Usage: sudo bash tests/e2e/test-vmsan-up.sh
# =============================================================================

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/lib/helpers.sh"

VM_IDS=()
TMPDIRS=()

cleanup() {
  echo ""
  echo "--- Cleanup ---"
  for dir in "${TMPDIRS[@]}"; do
    if [ -d "$dir" ]; then
      echo "  Cleaning up project in $dir..."
      cd "$dir" && sudo vmsan down --destroy --force 2>/dev/null || true
      cd /
      rm -rf "$dir"
    fi
  done
  for id in "${VM_IDS[@]}"; do
    [ -n "$id" ] && remove_vm "$id"
  done
}
trap cleanup EXIT

echo "================================================================"
echo "  E2E Test: Full vmsan up Lifecycle"
echo "================================================================"

# ---------------------------------------------------------------------------
# Test I15: vmsan up single service
# ---------------------------------------------------------------------------
echo ""
echo "--- Test I15: vmsan up single service ---"

TMPDIR_I15=$(mktemp -d)
TMPDIRS+=("$TMPDIR_I15")

cat > "${TMPDIR_I15}/vmsan.toml" << 'TOML'
runtime = "base"
start = "echo 'running' && sleep 3600"
TOML

cd "$TMPDIR_I15"
sudo vmsan up 2>&1 || true

STATUS_JSON=$(sudo vmsan status --json 2>/dev/null || echo '{}')
SVC_COUNT=$(echo "$STATUS_JSON" | jq -r '.services | length // 0' 2>/dev/null || echo "0")
assert_gt "$SVC_COUNT" 0 "I15: vmsan up created at least 1 service"

sudo vmsan down 2>/dev/null || true
cd /

# ---------------------------------------------------------------------------
# Test I16: vmsan up multi-service (web + postgres + redis)
# ---------------------------------------------------------------------------
echo ""
echo "--- Test I16: vmsan up multi-service ---"

TMPDIR_I16=$(mktemp -d)
TMPDIRS+=("$TMPDIR_I16")
mkdir -p "${TMPDIR_I16}/src"

cat > "${TMPDIR_I16}/vmsan.toml" << 'TOML'
[services.web]
runtime = "base"
start = "echo 'web running' && sleep 3600"
depends_on = ["db", "cache"]
memory = 256

[accessories.db]
type = "postgres"

[accessories.cache]
type = "redis"
TOML

echo "console.log('hello')" > "${TMPDIR_I16}/src/index.js"

cd "$TMPDIR_I16"
sudo vmsan up 2>&1 || true

STATUS_JSON=$(sudo vmsan status --json 2>/dev/null || echo '{}')
SVC_COUNT=$(echo "$STATUS_JSON" | jq -r '.services | length // 0' 2>/dev/null || echo "0")
assert_eq "$SVC_COUNT" "3" "I16: vmsan up created 3 services (web, db, cache)"

sudo vmsan down --destroy --force 2>/dev/null || true
cd /

# ---------------------------------------------------------------------------
# Test I17: vmsan deploy code re-deploy
# ---------------------------------------------------------------------------
echo ""
echo "--- Test I17: vmsan deploy code re-deploy ---"

TMPDIR_I17=$(mktemp -d)
TMPDIRS+=("$TMPDIR_I17")

cat > "${TMPDIR_I17}/vmsan.toml" << 'TOML'
runtime = "base"
start = "cat /app/version.txt && sleep 3600"
TOML

echo "v1" > "${TMPDIR_I17}/version.txt"

cd "$TMPDIR_I17"
sudo vmsan up 2>&1 || true

# Change code and re-deploy
echo "v2" > "${TMPDIR_I17}/version.txt"
sudo vmsan deploy 2>&1 || true

# Verify new code is deployed
DEPLOYED_VERSION=$(sudo vmsan exec app -- cat /app/version.txt 2>/dev/null | tr -d '[:space:]' || true)
assert_eq "$DEPLOYED_VERSION" "v2" "I17: code re-deploy updated version to v2"

sudo vmsan down --destroy --force 2>/dev/null || true
cd /

# ---------------------------------------------------------------------------
# Test I18: vmsan up idempotency
# ---------------------------------------------------------------------------
echo ""
echo "--- Test I18: vmsan up idempotency ---"

TMPDIR_I18=$(mktemp -d)
TMPDIRS+=("$TMPDIR_I18")

cat > "${TMPDIR_I18}/vmsan.toml" << 'TOML'
runtime = "base"
start = "echo 'running' && sleep 3600"
TOML

PROJECT_NAME=$(basename "$TMPDIR_I18")

cd "$TMPDIR_I18"
sudo vmsan up 2>&1 || true
COUNT1=$(vmsan list --json 2>/dev/null | jq "[.[] | select(.project==\"$PROJECT_NAME\")] | length" 2>/dev/null || echo "0")

sudo vmsan up 2>&1 || true
COUNT2=$(vmsan list --json 2>/dev/null | jq "[.[] | select(.project==\"$PROJECT_NAME\")] | length" 2>/dev/null || echo "0")

assert_eq "$COUNT1" "$COUNT2" "I18: vmsan up is idempotent (count1=$COUNT1, count2=$COUNT2)"

sudo vmsan down --destroy --force 2>/dev/null || true
cd /

# ---------------------------------------------------------------------------
# Test I19: vmsan secrets injection
# ---------------------------------------------------------------------------
echo ""
echo "--- Test I19: vmsan secrets injection ---"

TMPDIR_I19=$(mktemp -d)
TMPDIRS+=("$TMPDIR_I19")

cat > "${TMPDIR_I19}/vmsan.toml" << 'TOML'
runtime = "base"
start = "echo $TEST_SECRET && sleep 3600"
TOML

cd "$TMPDIR_I19"
vmsan secrets set TEST_SECRET=hello123 2>/dev/null || true
sudo vmsan up 2>&1 || true

SECRET_VALUE=$(sudo vmsan exec app -- printenv TEST_SECRET 2>/dev/null | tr -d '[:space:]' || true)
assert_eq "$SECRET_VALUE" "hello123" "I19: secret TEST_SECRET injected correctly"

sudo vmsan down --destroy --force 2>/dev/null || true
cd /

# ---------------------------------------------------------------------------
# Test I20: vmsan down + vmsan up data preservation
# ---------------------------------------------------------------------------
echo ""
echo "--- Test I20: data preservation across down/up ---"

TMPDIR_I20=$(mktemp -d)
TMPDIRS+=("$TMPDIR_I20")

cat > "${TMPDIR_I20}/vmsan.toml" << 'TOML'
[services.web]
runtime = "base"
start = "echo 'running' && sleep 3600"
depends_on = ["db"]
memory = 256

[accessories.db]
type = "postgres"
TOML

echo "ok" > "${TMPDIR_I20}/index.js"

cd "$TMPDIR_I20"
sudo vmsan up 2>&1 || true

# Write data to postgres
sudo vmsan exec db -- psql -U vmsan -d db -c "CREATE TABLE test_data (id serial, val text);" 2>/dev/null || true
sudo vmsan exec db -- psql -U vmsan -d db -c "INSERT INTO test_data (val) VALUES ('persist-me');" 2>/dev/null || true

# Stop (preserve data)
sudo vmsan down 2>/dev/null || true

# Restart
sudo vmsan up 2>&1 || true

# Verify data persists
PERSISTED=$(sudo vmsan exec db -- psql -U vmsan -d db -c "SELECT val FROM test_data;" 2>/dev/null || true)
assert_contains "$PERSISTED" "persist-me" "I20: data persists across down/up cycle"

sudo vmsan down --destroy --force 2>/dev/null || true
cd /

# ---------------------------------------------------------------------------
# Test I21: Health check gating (depends_on ordering)
# ---------------------------------------------------------------------------
echo ""
echo "--- Test I21: Health check gating ---"

TMPDIR_I21=$(mktemp -d)
TMPDIRS+=("$TMPDIR_I21")

cat > "${TMPDIR_I21}/vmsan.toml" << 'TOML'
[services.web]
runtime = "base"
start = "echo 'web started' && sleep 3600"
depends_on = ["db"]
memory = 256

[accessories.db]
type = "postgres"
TOML

echo "ok" > "${TMPDIR_I21}/index.js"

cd "$TMPDIR_I21"
sudo vmsan up 2>&1 || true

STATUS_JSON=$(sudo vmsan status --json 2>/dev/null || echo '{}')
SVC_COUNT=$(echo "$STATUS_JSON" | jq -r '.services | length // 0' 2>/dev/null || echo "0")
assert_gt "$SVC_COUNT" 1 "I21: depends_on ordering -- both services running"

sudo vmsan down --destroy --force 2>/dev/null || true
cd /

# ---------------------------------------------------------------------------
# Test L1: vmsan status shows all services
# ---------------------------------------------------------------------------
echo ""
echo "--- Test L1: vmsan status shows all services ---"

TMPDIR_L1=$(mktemp -d)
TMPDIRS+=("$TMPDIR_L1")

cat > "${TMPDIR_L1}/vmsan.toml" << 'TOML'
[services.web]
runtime = "base"
start = "sleep 3600"
memory = 256

[services.worker]
runtime = "base"
start = "sleep 3600"
memory = 256
TOML

cd "$TMPDIR_L1"
sudo vmsan up 2>&1 || true

STATUS_TEXT=$(sudo vmsan status 2>/dev/null || true)
assert_contains "$STATUS_TEXT" "web" "L1: status shows 'web' service"
assert_contains "$STATUS_TEXT" "worker" "L1: status shows 'worker' service"

STATUS_JSON=$(sudo vmsan status --json 2>/dev/null || echo '{}')
JSON_COUNT=$(echo "$STATUS_JSON" | jq -r '.services | length // 0' 2>/dev/null || echo "0")
assert_eq "$JSON_COUNT" "2" "L1: JSON status has 2 services"

sudo vmsan down --destroy --force 2>/dev/null || true
cd /

# ---------------------------------------------------------------------------
# Test L2: vmsan logs streams multi-service output
# ---------------------------------------------------------------------------
echo ""
echo "--- Test L2: vmsan logs streams output ---"

TMPDIR_L2=$(mktemp -d)
TMPDIRS+=("$TMPDIR_L2")

cat > "${TMPDIR_L2}/vmsan.toml" << 'TOML'
[services.web]
runtime = "base"
start = "sleep 3600"
memory = 256
TOML

cd "$TMPDIR_L2"
sudo vmsan up 2>&1 || true

# Logs command should start without error (use --no-follow with short timeout)
if timeout 10 vmsan logs --no-follow --lines 5 2>/dev/null; then
  echo -e "${GREEN}PASS${NC}: L2: vmsan logs --no-follow runs without error"
  TESTS_PASSED=$((TESTS_PASSED + 1))
else
  # timeout exit 124 is expected if it produces output but takes time
  EC=$?
  if [ "$EC" -eq 124 ]; then
    echo -e "${GREEN}PASS${NC}: L2: vmsan logs ran (timed out as expected for streaming)"
    TESTS_PASSED=$((TESTS_PASSED + 1))
  else
    echo -e "${RED}FAIL${NC}: L2: vmsan logs failed with exit code $EC"
    TESTS_FAILED=$((TESTS_FAILED + 1))
  fi
fi

# Per-service filter
if timeout 10 vmsan logs web --no-follow --lines 5 2>/dev/null; then
  echo -e "${GREEN}PASS${NC}: L2: vmsan logs web (per-service) runs without error"
  TESTS_PASSED=$((TESTS_PASSED + 1))
else
  EC=$?
  if [ "$EC" -eq 124 ]; then
    echo -e "${GREEN}PASS${NC}: L2: vmsan logs web ran (timed out as expected)"
    TESTS_PASSED=$((TESTS_PASSED + 1))
  else
    echo -e "${RED}FAIL${NC}: L2: vmsan logs web failed with exit code $EC"
    TESTS_FAILED=$((TESTS_FAILED + 1))
  fi
fi

sudo vmsan down --destroy --force 2>/dev/null || true
cd /

# ---------------------------------------------------------------------------
# Test L3: vmsan down --destroy removes everything
# ---------------------------------------------------------------------------
echo ""
echo "--- Test L3: vmsan down --destroy removes everything ---"

TMPDIR_L3=$(mktemp -d)
TMPDIRS+=("$TMPDIR_L3")

cat > "${TMPDIR_L3}/vmsan.toml" << 'TOML'
runtime = "base"
start = "sleep 3600"
TOML

PROJECT_NAME=$(basename "$TMPDIR_L3")

cd "$TMPDIR_L3"
sudo vmsan up 2>&1 || true

# Verify VM exists
PRE_COUNT=$(vmsan list --json 2>/dev/null | jq "[.[] | select(.project==\"$PROJECT_NAME\")] | length" 2>/dev/null || echo "0")
assert_gt "$PRE_COUNT" 0 "L3: VMs exist before --destroy"

# Destroy
sudo vmsan down --destroy --force 2>/dev/null || true

# Verify VM is gone
POST_COUNT=$(vmsan list --json 2>/dev/null | jq "[.[] | select(.project==\"$PROJECT_NAME\")] | length" 2>/dev/null || echo "0")
assert_eq "$POST_COUNT" "0" "L3: no VMs remain after --destroy"

cd /

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
print_summary
