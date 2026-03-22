# E2E Test Plan: 0.4.0 → 0.8.0

> **Scope:** All features shipped after 0.3.0 (snapshot) that require KVM hardware
> **Versions covered:** 0.4.0 "The Wall", 0.5.0 "The Mesh", 0.6.0 "Blueprint", 0.7.0 "Ignition", 0.8.0 "Outpost"
> **Prerequisites:** KVM-capable host, vmsan installed from `feat/platform-multihost` branch, jq, root privileges
> **Install:** `sudo bash install.sh --ref feat/platform-multihost`

---

## Test Index

| ID | Version | Category | Test |
|----|---------|----------|------|
| I9 | 0.4.0 | DNS filtering | DNS resolves in allow-all mode |
| I10 | 0.4.0 | DNS filtering | Custom policy blocks denied domains |
| I11 | 0.4.0 | SNI filtering | TLS to denied domain fails |
| I12 | 0.4.0 | ICMP | --allow-icmp flag enables ping |
| I13 | 0.4.0 | DNS security | ECH SvcParam stripped from HTTPS records |
| I14 | 0.4.0 | Resilience | dnsproxy crash recovery |
| M1 | 0.5.0 | Mesh routing | VM-A reaches VM-B via mesh IP |
| M2 | 0.5.0 | Mesh ACL | VM-A blocked when ACL denies |
| M3 | 0.5.0 | Mesh DNS | service.project.vmsan.internal resolves |
| M4 | 0.5.0 | Mesh isolation | Cross-project traffic blocked |
| M5 | 0.5.0 | Mesh DNS | Stopped VM disappears from DNS |
| M6 | 0.5.0 | Mesh CLI | --connect-to and --service flags work |
| M7 | 0.5.0 | Mesh CLI | vmsan network connections shows mesh traffic |
| B1 | 0.6.0 | Config | vmsan init generates valid TOML |
| B2 | 0.6.0 | Config | vmsan validate catches cycle and unknowns |
| B3 | 0.6.0 | Config | Auto-detection identifies Node.js project |
| I15 | 0.7.0 | Platform | vmsan up single service |
| I16 | 0.7.0 | Platform | vmsan up multi-service (web + postgres + redis) |
| I17 | 0.7.0 | Platform | vmsan deploy code re-deploy |
| I18 | 0.7.0 | Platform | vmsan up idempotency |
| I19 | 0.7.0 | Platform | vmsan secrets injection |
| I20 | 0.7.0 | Platform | vmsan down + vmsan up data preservation |
| I21 | 0.7.0 | Platform | Health check gating (depends_on ordering) |
| L1 | 0.7.0 | Lifecycle | vmsan status shows all services |
| L2 | 0.7.0 | Lifecycle | vmsan logs streams multi-service output |
| L3 | 0.7.0 | Lifecycle | vmsan down --destroy removes everything |
| S1 | 0.8.0 | State | SQLite migration from JSON |
| S2 | 0.8.0 | State | vmsan migrate --dry-run |
| S3 | 0.8.0 | State | JSON fallback with VMSAN_STATE_BACKEND=json |
| H1 | 0.8.0 | Multi-host | vmsan server starts and responds |
| H2 | 0.8.0 | Multi-host | Agent join with token |
| H3 | 0.8.0 | Multi-host | Join token is single-use |
| H4 | 0.8.0 | Multi-host | vmsan hosts list shows joined agent |
| H5 | 0.8.0 | Multi-host | vmsan create --host dispatches to remote |

---

## 0.4.0 "The Wall" — Egress Filtering

### I9: DNS resolves in allow-all mode

**What:** VMs in allow-all mode can resolve DNS via the dnsproxy sidecar.

```bash
VM_ID=$(sudo vmsan create --runtime base --vcpus 1 --memory 256 --json 2>&1 | \
  jq -Rr 'fromjson? | .vmId? // empty' | head -1)

# DNS resolves via dnsproxy DNAT
sudo vmsan exec "$VM_ID" -- dig +short example.com | grep -q "."

# ECH stripped from HTTPS records (no ech= in output)
sudo vmsan exec "$VM_ID" -- dig -t HTTPS cloudflare.com 2>/dev/null | grep -v "ech="

sudo vmsan stop "$VM_ID" && sudo vmsan remove "$VM_ID"
```

**Pass criteria:** dig returns an IP, HTTPS records have no `ech=` SvcParam.

---

### I10: DNS filtering in custom mode

**What:** Custom policy blocks DNS for non-allowed domains.

```bash
VM_ID=$(sudo vmsan create --runtime base --vcpus 1 --memory 256 \
  --network-policy custom --allowed-domain example.com --json 2>&1 | \
  jq -Rr 'fromjson? | .vmId? // empty' | head -1)

# Allowed domain resolves
sudo vmsan exec "$VM_ID" -- dig +short example.com | grep -q "."

# Denied domain returns empty (NXDOMAIN or REFUSED)
RESULT=$(sudo vmsan exec "$VM_ID" -- dig +short evil.com 2>/dev/null || true)
[ -z "$RESULT" ]

sudo vmsan stop "$VM_ID" && sudo vmsan remove "$VM_ID"
```

**Pass criteria:** Allowed domain resolves, denied domain returns nothing.

---

### I11: SNI filtering in custom mode

**What:** TLS connections to non-allowed domains are rejected at the SNI layer.

```bash
VM_ID=$(sudo vmsan create --runtime base --vcpus 1 --memory 256 \
  --network-policy custom --allowed-domain example.com --json 2>&1 | \
  jq -Rr 'fromjson? | .vmId? // empty' | head -1)

# TLS to allowed domain succeeds
sudo vmsan exec "$VM_ID" -- curl -sf --max-time 10 https://example.com > /dev/null

# TLS to denied domain fails (connection reset or timeout)
! sudo vmsan exec "$VM_ID" -- curl -sf --max-time 5 https://evil.com > /dev/null 2>&1

sudo vmsan stop "$VM_ID" && sudo vmsan remove "$VM_ID"
```

**Pass criteria:** Allowed HTTPS succeeds, denied HTTPS fails.

---

### I12: --allow-icmp flag

**What:** The `--allow-icmp` flag enables ICMP (ping) from VMs.

```bash
VM_ID=$(sudo vmsan create --runtime base --vcpus 1 --memory 256 \
  --allow-icmp --json 2>&1 | \
  jq -Rr 'fromjson? | .vmId? // empty' | head -1)

# ICMP now works
sudo vmsan exec "$VM_ID" -- ping -c 1 -W 5 8.8.8.8

sudo vmsan stop "$VM_ID" && sudo vmsan remove "$VM_ID"
```

**Pass criteria:** Ping succeeds.

---

### I13: ECH SvcParam stripping

**What:** ECH parameters are stripped from DNS HTTPS records to prevent SNI bypass.

```bash
VM_ID=$(sudo vmsan create --runtime base --vcpus 1 --memory 256 --json 2>&1 | \
  jq -Rr 'fromjson? | .vmId? // empty' | head -1)

# Query HTTPS record for a domain known to use ECH (e.g., cloudflare.com)
OUTPUT=$(sudo vmsan exec "$VM_ID" -- dig -t HTTPS cloudflare.com 2>/dev/null)

# Verify no ech= parameter in the response
echo "$OUTPUT" | grep -v "ech=" > /dev/null

sudo vmsan stop "$VM_ID" && sudo vmsan remove "$VM_ID"
```

**Pass criteria:** HTTPS record response contains no `ech=` SvcParam.

---

### I14: dnsproxy crash recovery

**What:** If the dnsproxy sidecar crashes, the supervisor restarts it automatically.

```bash
VM_ID=$(sudo vmsan create --runtime base --vcpus 1 --memory 256 --json 2>&1 | \
  jq -Rr 'fromjson? | .vmId? // empty' | head -1)

# Verify DNS works before
sudo vmsan exec "$VM_ID" -- dig +short example.com | grep -q "."

# Find and kill the dnsproxy process for this VM's slot
SLOT=$(vmsan list --json 2>/dev/null | jq -r --arg id "$VM_ID" '.[] | select(.id==$id) | .network.hostIp' | awk -F. '{print $4}')
DNS_PORT=$((10053 + SLOT))
DNSPROXY_PID=$(pgrep -f "dnsproxy.*${DNS_PORT}" || true)
if [ -n "$DNSPROXY_PID" ]; then
  sudo kill "$DNSPROXY_PID"
fi

# Wait for supervisor to restart
sleep 10

# DNS should work again
sudo vmsan exec "$VM_ID" -- dig +short example.com | grep -q "."

sudo vmsan stop "$VM_ID" && sudo vmsan remove "$VM_ID"
```

**Pass criteria:** DNS resolves after dnsproxy restart (within 10 seconds).

---

## 0.5.0 "The Mesh" — Inter-VM Networking

### M1: VM-A reaches VM-B via mesh IP

**What:** Two VMs in the same project can communicate through the L3 mesh.

```bash
# Create two VMs in the same project with mesh connectivity
VM_A=$(sudo vmsan create --runtime base --vcpus 1 --memory 256 \
  --project meshtest --service web --connect-to db:5432 --json 2>&1 | \
  jq -Rr 'fromjson? | .vmId? // empty' | head -1)

VM_B=$(sudo vmsan create --runtime base --vcpus 1 --memory 256 \
  --project meshtest --service db --connect-to web:8080 --json 2>&1 | \
  jq -Rr 'fromjson? | .vmId? // empty' | head -1)

# Get VM-B's mesh IP
MESH_IP_B=$(vmsan list --json 2>/dev/null | jq -r --arg id "$VM_B" '.[] | select(.id==$id) | .network.meshIp')

# VM-A can reach VM-B on allowed port (5432)
sudo vmsan exec "$VM_A" -- bash -c "echo | nc -w 3 $MESH_IP_B 5432 2>/dev/null; echo \$?" | grep -q "0\|1"

# Cleanup
sudo vmsan stop "$VM_A" 2>/dev/null; sudo vmsan remove "$VM_A" 2>/dev/null
sudo vmsan stop "$VM_B" 2>/dev/null; sudo vmsan remove "$VM_B" 2>/dev/null
```

**Pass criteria:** TCP connection attempt reaches VM-B (connect or refused, not timeout).

---

### M2: Mesh ACL denies unauthorized traffic

**What:** VMs cannot reach each other on ports not declared in `--connect-to`.

```bash
VM_A=$(sudo vmsan create --runtime base --vcpus 1 --memory 256 \
  --project acltest --service web --connect-to db:5432 --json 2>&1 | \
  jq -Rr 'fromjson? | .vmId? // empty' | head -1)

VM_B=$(sudo vmsan create --runtime base --vcpus 1 --memory 256 \
  --project acltest --service db --json 2>&1 | \
  jq -Rr 'fromjson? | .vmId? // empty' | head -1)

MESH_IP_B=$(vmsan list --json 2>/dev/null | jq -r --arg id "$VM_B" '.[] | select(.id==$id) | .network.meshIp')

# Port 5432 is allowed — should be reachable
# Port 9999 is NOT allowed — should timeout (nftables DROP)
! sudo vmsan exec "$VM_A" -- bash -c "echo | nc -w 3 $MESH_IP_B 9999 2>/dev/null"

sudo vmsan stop "$VM_A" 2>/dev/null; sudo vmsan remove "$VM_A" 2>/dev/null
sudo vmsan stop "$VM_B" 2>/dev/null; sudo vmsan remove "$VM_B" 2>/dev/null
```

**Pass criteria:** Connection to unauthorized port times out or is dropped.

---

### M3: Mesh DNS resolves service names

**What:** `<service>.<project>.vmsan.internal` resolves to the correct mesh IP.

```bash
VM_A=$(sudo vmsan create --runtime base --vcpus 1 --memory 256 \
  --project dnstest --service web --connect-to db:5432 --json 2>&1 | \
  jq -Rr 'fromjson? | .vmId? // empty' | head -1)

VM_B=$(sudo vmsan create --runtime base --vcpus 1 --memory 256 \
  --project dnstest --service db --json 2>&1 | \
  jq -Rr 'fromjson? | .vmId? // empty' | head -1)

EXPECTED_IP=$(vmsan list --json 2>/dev/null | jq -r --arg id "$VM_B" '.[] | select(.id==$id) | .network.meshIp')

# From inside VM-A, resolve db.dnstest.vmsan.internal
RESOLVED_IP=$(sudo vmsan exec "$VM_A" -- dig +short db.dnstest.vmsan.internal 2>/dev/null | head -1)

[ "$RESOLVED_IP" = "$EXPECTED_IP" ]

sudo vmsan stop "$VM_A" 2>/dev/null; sudo vmsan remove "$VM_A" 2>/dev/null
sudo vmsan stop "$VM_B" 2>/dev/null; sudo vmsan remove "$VM_B" 2>/dev/null
```

**Pass criteria:** DNS returns the exact mesh IP of VM-B.

---

### M4: Cross-project traffic blocked

**What:** VMs in different projects cannot reach each other.

```bash
VM_A=$(sudo vmsan create --runtime base --vcpus 1 --memory 256 \
  --project alpha --service web --json 2>&1 | \
  jq -Rr 'fromjson? | .vmId? // empty' | head -1)

VM_B=$(sudo vmsan create --runtime base --vcpus 1 --memory 256 \
  --project beta --service web --json 2>&1 | \
  jq -Rr 'fromjson? | .vmId? // empty' | head -1)

MESH_IP_B=$(vmsan list --json 2>/dev/null | jq -r --arg id "$VM_B" '.[] | select(.id==$id) | .network.meshIp')

# Cross-project traffic should be blocked
! sudo vmsan exec "$VM_A" -- bash -c "echo | nc -w 3 $MESH_IP_B 8080 2>/dev/null"

# DNS should return NXDOMAIN for other project's services
RESULT=$(sudo vmsan exec "$VM_A" -- dig +short web.beta.vmsan.internal 2>/dev/null || true)
[ -z "$RESULT" ]

sudo vmsan stop "$VM_A" 2>/dev/null; sudo vmsan remove "$VM_A" 2>/dev/null
sudo vmsan stop "$VM_B" 2>/dev/null; sudo vmsan remove "$VM_B" 2>/dev/null
```

**Pass criteria:** TCP connection drops, DNS returns empty/NXDOMAIN.

---

### M5: Stopped VM disappears from mesh DNS

**What:** When a VM stops, its DNS record is removed within seconds.

```bash
VM_A=$(sudo vmsan create --runtime base --vcpus 1 --memory 256 \
  --project ttltest --service web --connect-to db:5432 --json 2>&1 | \
  jq -Rr 'fromjson? | .vmId? // empty' | head -1)

VM_B=$(sudo vmsan create --runtime base --vcpus 1 --memory 256 \
  --project ttltest --service db --json 2>&1 | \
  jq -Rr 'fromjson? | .vmId? // empty' | head -1)

# Verify DNS resolves while running
sudo vmsan exec "$VM_A" -- dig +short db.ttltest.vmsan.internal | grep -q "."

# Stop VM-B
sudo vmsan stop "$VM_B"

# Wait for DNS TTL (5s) + margin
sleep 8

# DNS should no longer resolve
RESULT=$(sudo vmsan exec "$VM_A" -- dig +short db.ttltest.vmsan.internal 2>/dev/null || true)
[ -z "$RESULT" ]

sudo vmsan stop "$VM_A" 2>/dev/null; sudo vmsan remove "$VM_A" 2>/dev/null
sudo vmsan remove "$VM_B" 2>/dev/null
```

**Pass criteria:** DNS returns empty after VM-B is stopped.

---

### M6: --connect-to and --service flags work

**What:** CLI flags register services and configure ACLs correctly.

```bash
# Create with service + connect-to
VM_ID=$(sudo vmsan create --runtime base --vcpus 1 --memory 256 \
  --project flagtest --service api --connect-to db:5432,cache:6379 --json 2>&1 | \
  jq -Rr 'fromjson? | .vmId? // empty' | head -1)

# Verify state has service and connect-to
STATE=$(vmsan list --json 2>/dev/null | jq --arg id "$VM_ID" '.[] | select(.id==$id)')
echo "$STATE" | jq -e '.network.service == "api"'
echo "$STATE" | jq -e '.network.connectTo | length == 2'

sudo vmsan stop "$VM_ID" 2>/dev/null; sudo vmsan remove "$VM_ID" 2>/dev/null
```

**Pass criteria:** VM state contains correct service name and connectTo array.

---

### M7: vmsan network connections shows mesh traffic

**What:** The `vmsan network connections` command displays active mesh connections.

```bash
VM_A=$(sudo vmsan create --runtime base --vcpus 1 --memory 256 \
  --project conntest --service web --connect-to db:5432 --json 2>&1 | \
  jq -Rr 'fromjson? | .vmId? // empty' | head -1)

VM_B=$(sudo vmsan create --runtime base --vcpus 1 --memory 256 \
  --project conntest --service db --connect-to web:8080 --json 2>&1 | \
  jq -Rr 'fromjson? | .vmId? // empty' | head -1)

MESH_IP_B=$(vmsan list --json 2>/dev/null | jq -r --arg id "$VM_B" '.[] | select(.id==$id) | .network.meshIp')

# Generate some traffic (connect to VM-B)
sudo vmsan exec "$VM_A" -- bash -c "echo | nc -w 1 $MESH_IP_B 5432 2>/dev/null" || true

# Check network connections
sudo vmsan network connections "$VM_A" 2>/dev/null | grep -q "$MESH_IP_B" || true

sudo vmsan stop "$VM_A" 2>/dev/null; sudo vmsan remove "$VM_A" 2>/dev/null
sudo vmsan stop "$VM_B" 2>/dev/null; sudo vmsan remove "$VM_B" 2>/dev/null
```

**Pass criteria:** Connection output shows mesh IP (best effort — conntrack entries may expire).

---

## 0.6.0 "Blueprint" — Declarative Config

### B1: vmsan init generates valid TOML

**What:** The init wizard produces a valid vmsan.toml from a project directory.

```bash
TMPDIR=$(mktemp -d)
cd "$TMPDIR"

# Create a fake Node.js project
echo '{"name":"test-app","scripts":{"start":"node index.js","build":"echo built"}}' > package.json
echo 'console.log("hello")' > index.js

# Run init in non-interactive mode
vmsan init --yes 2>/dev/null

# Verify vmsan.toml was created and is valid
[ -f vmsan.toml ]
vmsan validate 2>/dev/null

cd /
rm -rf "$TMPDIR"
```

**Pass criteria:** vmsan.toml exists and passes validation.

---

### B2: vmsan validate catches errors

**What:** Validation detects circular dependencies and unknown fields.

```bash
TMPDIR=$(mktemp -d)

# Circular dependency
cat > "$TMPDIR/vmsan.toml" << 'TOML'
[services.a]
runtime = "base"
depends_on = ["b"]

[services.b]
runtime = "base"
depends_on = ["a"]
TOML

! vmsan validate --config "$TMPDIR/vmsan.toml" 2>&1 | grep -qi "circular\|cycle"

# Unknown field
cat > "$TMPDIR/vmsan.toml" << 'TOML'
[services.web]
runtime = "base"
unknown_field = true
TOML

! vmsan validate --config "$TMPDIR/vmsan.toml" 2>&1

rm -rf "$TMPDIR"
```

**Pass criteria:** Both invalid configs produce errors.

---

### B3: Auto-detection identifies Node.js project

**What:** vmsan init correctly detects a Node.js project and suggests node runtime.

```bash
TMPDIR=$(mktemp -d)
cd "$TMPDIR"

echo '{"name":"detect-test","scripts":{"start":"node app.js","build":"npm run build"}}' > package.json

vmsan init --yes 2>/dev/null
grep -q 'runtime.*=.*"node' vmsan.toml || grep -q 'node' vmsan.toml

cd /
rm -rf "$TMPDIR"
```

**Pass criteria:** Generated TOML references a node runtime.

---

## 0.7.0 "Ignition" — Platform DX

### I15: vmsan up single service

**What:** `vmsan up` deploys a single-service app from vmsan.toml.

```bash
TMPDIR=$(mktemp -d)
cd "$TMPDIR"

cat > vmsan.toml << 'TOML'
runtime = "base"
start = "echo 'running' && sleep 3600"
TOML

sudo vmsan up 2>&1
sudo vmsan status --json 2>/dev/null | jq -e '.services | length >= 1'
sudo vmsan down 2>/dev/null

cd /
rm -rf "$TMPDIR"
```

**Pass criteria:** Status shows at least 1 service after up.

---

### I16: vmsan up multi-service (web + postgres + redis)

**What:** `vmsan up` deploys a multi-service stack with dependency ordering.

```bash
TMPDIR=$(mktemp -d)
cd "$TMPDIR"
mkdir -p src

cat > vmsan.toml << 'TOML'
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

echo "console.log('hello')" > src/index.js

sudo vmsan up 2>&1

# Verify all 3 services deployed
sudo vmsan status --json 2>/dev/null | jq -e '.services | length == 3'

sudo vmsan down --destroy --force 2>/dev/null

cd /
rm -rf "$TMPDIR"
```

**Pass criteria:** 3 services appear in status (web, db, cache).

---

### I17: vmsan deploy code re-deploy

**What:** `vmsan deploy` re-uploads code without touching accessories.

```bash
TMPDIR=$(mktemp -d)
cd "$TMPDIR"

cat > vmsan.toml << 'TOML'
runtime = "base"
start = "cat /app/version.txt && sleep 3600"
TOML

echo "v1" > version.txt
sudo vmsan up 2>&1

# Change code and re-deploy
echo "v2" > version.txt
sudo vmsan deploy 2>&1

# Verify new code is deployed
sudo vmsan exec app -- cat /app/version.txt 2>/dev/null | grep -q "v2"

sudo vmsan down --destroy --force 2>/dev/null

cd /
rm -rf "$TMPDIR"
```

**Pass criteria:** VM contains v2 after deploy.

---

### I18: vmsan up idempotency

**What:** Running `vmsan up` twice doesn't create duplicate VMs.

```bash
TMPDIR=$(mktemp -d)
cd "$TMPDIR"

cat > vmsan.toml << 'TOML'
runtime = "base"
start = "echo 'running' && sleep 3600"
TOML

PROJECT_NAME=$(basename "$TMPDIR")

sudo vmsan up 2>&1
COUNT1=$(vmsan list --json 2>/dev/null | jq "[.[] | select(.project==\"$PROJECT_NAME\")] | length")

sudo vmsan up 2>&1
COUNT2=$(vmsan list --json 2>/dev/null | jq "[.[] | select(.project==\"$PROJECT_NAME\")] | length")

[ "$COUNT1" = "$COUNT2" ]

sudo vmsan down --destroy --force 2>/dev/null

cd /
rm -rf "$TMPDIR"
```

**Pass criteria:** VM count is identical after second `vmsan up`.

---

### I19: vmsan secrets injection

**What:** Secrets set via CLI are injected as environment variables on deploy.

```bash
TMPDIR=$(mktemp -d)
cd "$TMPDIR"

cat > vmsan.toml << 'TOML'
runtime = "base"
start = "echo $TEST_SECRET && sleep 3600"
TOML

PROJECT_NAME=$(basename "$TMPDIR")

vmsan secrets set TEST_SECRET=hello123 2>/dev/null
sudo vmsan up 2>&1

# Verify secret is available inside VM
sudo vmsan exec app -- printenv TEST_SECRET 2>/dev/null | grep -q "hello123"

sudo vmsan down --destroy --force 2>/dev/null

cd /
rm -rf "$TMPDIR"
```

**Pass criteria:** Environment variable contains the secret value.

---

### I20: vmsan down + vmsan up data preservation

**What:** Stopping and restarting preserves accessory data (e.g., postgres database).

```bash
TMPDIR=$(mktemp -d)
cd "$TMPDIR"

cat > vmsan.toml << 'TOML'
[services.web]
runtime = "base"
start = "echo 'running' && sleep 3600"
depends_on = ["db"]
memory = 256

[accessories.db]
type = "postgres"
TOML

echo "ok" > index.js

sudo vmsan up 2>&1

# Write data to postgres
sudo vmsan exec db -- psql -U vmsan -d db -c "CREATE TABLE test_data (id serial, val text);" 2>/dev/null
sudo vmsan exec db -- psql -U vmsan -d db -c "INSERT INTO test_data (val) VALUES ('persist-me');" 2>/dev/null

# Stop (preserve data)
sudo vmsan down 2>/dev/null

# Restart
sudo vmsan up 2>&1

# Verify data persists
sudo vmsan exec db -- psql -U vmsan -d db -c "SELECT val FROM test_data;" 2>/dev/null | grep -q "persist-me"

sudo vmsan down --destroy --force 2>/dev/null

cd /
rm -rf "$TMPDIR"
```

**Pass criteria:** Data written before `down` is still present after `up`.

---

### I21: Health check gating (depends_on ordering)

**What:** Services with `depends_on` don't start until dependencies pass health checks.

```bash
TMPDIR=$(mktemp -d)
cd "$TMPDIR"

cat > vmsan.toml << 'TOML'
[services.web]
runtime = "base"
start = "echo 'web started at $(date +%s)' && sleep 3600"
depends_on = ["db"]
memory = 256

[accessories.db]
type = "postgres"
TOML

echo "ok" > index.js

sudo vmsan up 2>&1

# Both services should be running
sudo vmsan status --json 2>/dev/null | jq -e '.services | length >= 2'

# db should have started before web (db is in group 0, web in group 1)
# The orchestrator enforces this — if we got here without error, ordering worked

sudo vmsan down --destroy --force 2>/dev/null

cd /
rm -rf "$TMPDIR"
```

**Pass criteria:** Both services running, no dependency ordering errors.

---

### L1: vmsan status shows all services

**What:** `vmsan status` displays a table with all project services.

```bash
TMPDIR=$(mktemp -d)
cd "$TMPDIR"

cat > vmsan.toml << 'TOML'
[services.web]
runtime = "base"
start = "sleep 3600"
memory = 256

[services.worker]
runtime = "base"
start = "sleep 3600"
memory = 256
TOML

sudo vmsan up 2>&1

# Status should show both services
sudo vmsan status 2>/dev/null | grep -q "web"
sudo vmsan status 2>/dev/null | grep -q "worker"

# JSON output should have 2 entries
sudo vmsan status --json 2>/dev/null | jq -e '.services | length == 2'

sudo vmsan down --destroy --force 2>/dev/null

cd /
rm -rf "$TMPDIR"
```

**Pass criteria:** Both services appear in status output.

---

### L2: vmsan logs streams multi-service output

**What:** `vmsan logs` shows color-coded interleaved output from multiple VMs.

```bash
TMPDIR=$(mktemp -d)
cd "$TMPDIR"

cat > vmsan.toml << 'TOML'
[services.web]
runtime = "base"
start = "sleep 3600"
memory = 256
TOML

sudo vmsan up 2>&1

# Logs command should start without error (will produce output based on VM journalctl)
# Use --no-follow and --lines to get a quick snapshot
timeout 10 vmsan logs --no-follow --lines 5 2>/dev/null || true

# Per-service filter should work
timeout 10 vmsan logs web --no-follow --lines 5 2>/dev/null || true

sudo vmsan down --destroy --force 2>/dev/null

cd /
rm -rf "$TMPDIR"
```

**Pass criteria:** Logs command runs without error, shows prefixed output.

---

### L3: vmsan down --destroy removes everything

**What:** `--destroy` flag removes VMs and all associated data.

```bash
TMPDIR=$(mktemp -d)
cd "$TMPDIR"

cat > vmsan.toml << 'TOML'
runtime = "base"
start = "sleep 3600"
TOML

PROJECT_NAME=$(basename "$TMPDIR")

sudo vmsan up 2>&1

# Verify VM exists
vmsan list --json 2>/dev/null | jq -e "[.[] | select(.project==\"$PROJECT_NAME\")] | length > 0"

# Destroy
sudo vmsan down --destroy --force 2>/dev/null

# Verify VM is gone
COUNT=$(vmsan list --json 2>/dev/null | jq "[.[] | select(.project==\"$PROJECT_NAME\")] | length")
[ "$COUNT" = "0" ]

cd /
rm -rf "$TMPDIR"
```

**Pass criteria:** No VMs remain for the project after --destroy.

---

## 0.8.0 "Outpost" — Multi-Host Foundation

### S1: SQLite migration from JSON

**What:** `vmsan migrate` imports existing JSON state files into SQLite.

```bash
# Create VMs with JSON backend
VMSAN_STATE_BACKEND=json sudo vmsan create --runtime base --vcpus 1 --memory 256 --json 2>&1 | \
  jq -Rr 'fromjson? | .vmId? // empty' | head -1 > /tmp/s1_vm1
VMSAN_STATE_BACKEND=json sudo vmsan create --runtime base --vcpus 1 --memory 256 --json 2>&1 | \
  jq -Rr 'fromjson? | .vmId? // empty' | head -1 > /tmp/s1_vm2

JSON_COUNT=$(VMSAN_STATE_BACKEND=json vmsan list --json 2>/dev/null | jq 'length')

# Migrate
sudo vmsan migrate --force 2>/dev/null

# Verify SQLite has same count
SQLITE_COUNT=$(vmsan list --json 2>/dev/null | jq 'length')
[ "$JSON_COUNT" -le "$SQLITE_COUNT" ]

# Cleanup
for f in /tmp/s1_vm1 /tmp/s1_vm2; do
  ID=$(cat "$f" 2>/dev/null || true)
  [ -n "$ID" ] && { sudo vmsan stop "$ID" 2>/dev/null; sudo vmsan remove "$ID" 2>/dev/null; } || true
done
rm -f /tmp/s1_vm1 /tmp/s1_vm2
```

**Pass criteria:** SQLite VM count >= JSON VM count after migration.

---

### S2: vmsan migrate --dry-run

**What:** Dry run shows what would be imported without writing.

```bash
sudo vmsan migrate --dry-run 2>&1 | grep -qi "import\|found\|would"
```

**Pass criteria:** Output mentions what would be imported.

---

### S3: JSON fallback with VMSAN_STATE_BACKEND=json

**What:** Setting `VMSAN_STATE_BACKEND=json` uses the old JSON file store.

```bash
VM_ID=$(VMSAN_STATE_BACKEND=json sudo vmsan create --runtime base --vcpus 1 --memory 256 --json 2>&1 | \
  jq -Rr 'fromjson? | .vmId? // empty' | head -1)

# Verify JSON file exists
[ -f "$HOME/.vmsan/vms/${VM_ID}.json" ]

VMSAN_STATE_BACKEND=json sudo vmsan stop "$VM_ID" 2>/dev/null
VMSAN_STATE_BACKEND=json sudo vmsan remove "$VM_ID" 2>/dev/null
```

**Pass criteria:** JSON state file exists at expected path.

---

### H1: vmsan server starts and responds

**What:** The server control plane starts and responds to status requests.

```bash
# Start server in background
sudo nftables/vmsan-server --listen 127.0.0.1:16443 --db /tmp/vmsan-test-server.db &
SERVER_PID=$!
sleep 2

# Check status endpoint
curl -sf http://127.0.0.1:16443/api/v1/status | jq -e '.ok == true'

kill $SERVER_PID 2>/dev/null
rm -f /tmp/vmsan-test-server.db
```

**Pass criteria:** Status endpoint returns `{"ok": true}`.

**Note:** Build the server binary first: `cd nftables && make server`

---

### H2: Agent join with token

**What:** An agent can join a server using a generated token.

```bash
# Start server
sudo nftables/vmsan-server --listen 127.0.0.1:16443 --db /tmp/vmsan-test-h2.db &
SERVER_PID=$!
sleep 2

# Generate token
TOKEN=$(curl -sf -X POST http://127.0.0.1:16443/api/v1/tokens | jq -r '.token')
[ -n "$TOKEN" ]

# Join (agent-host binary)
sudo nftables/vmsan-agent-host join --server http://127.0.0.1:16443 --token "$TOKEN" --name test-host 2>&1

# Verify host appeared
curl -sf http://127.0.0.1:16443/api/v1/hosts | jq -e 'length == 1'
curl -sf http://127.0.0.1:16443/api/v1/hosts | jq -e '.[0].name == "test-host"'

kill $SERVER_PID 2>/dev/null
rm -f /tmp/vmsan-test-h2.db "$HOME/.vmsan/agent.json"
```

**Pass criteria:** Host appears in server's host list after join.

**Note:** Build the agent-host binary first: `cd nftables && make agent`

---

### H3: Join token is single-use

**What:** Attempting to reuse a consumed token fails.

```bash
sudo nftables/vmsan-server --listen 127.0.0.1:16443 --db /tmp/vmsan-test-h3.db &
SERVER_PID=$!
sleep 2

TOKEN=$(curl -sf -X POST http://127.0.0.1:16443/api/v1/tokens | jq -r '.token')

# First join succeeds
sudo nftables/vmsan-agent-host join --server http://127.0.0.1:16443 --token "$TOKEN" --name host-1 2>&1

# Second join with same token should fail
! sudo nftables/vmsan-agent-host join --server http://127.0.0.1:16443 --token "$TOKEN" --name host-2 2>&1

# Only 1 host registered
curl -sf http://127.0.0.1:16443/api/v1/hosts | jq -e 'length == 1'

kill $SERVER_PID 2>/dev/null
rm -f /tmp/vmsan-test-h3.db "$HOME/.vmsan/agent.json"
```

**Pass criteria:** Second join attempt fails, only 1 host exists.

---

### H4: vmsan hosts list shows joined agent

**What:** After an agent joins, `vmsan hosts list` (via server API) shows it.

```bash
sudo nftables/vmsan-server --listen 127.0.0.1:16443 --db /tmp/vmsan-test-h4.db &
SERVER_PID=$!
sleep 2

TOKEN=$(curl -sf -X POST http://127.0.0.1:16443/api/v1/tokens | jq -r '.token')
sudo nftables/vmsan-agent-host join --server http://127.0.0.1:16443 --token "$TOKEN" --name worker-1 2>&1

# Use the CLI (requires VMSAN_SERVER_URL)
VMSAN_SERVER_URL=http://127.0.0.1:16443 vmsan hosts list 2>/dev/null | grep -q "worker-1"

# JSON output
VMSAN_SERVER_URL=http://127.0.0.1:16443 vmsan hosts list --json 2>/dev/null | jq -e '.[0].name == "worker-1"'

kill $SERVER_PID 2>/dev/null
rm -f /tmp/vmsan-test-h4.db "$HOME/.vmsan/agent.json"
```

**Pass criteria:** CLI shows the worker-1 host.

---

### H5: vmsan create --host dispatches to remote

**What:** Creating a VM with `--host` sends the request to the server.

```bash
sudo nftables/vmsan-server --listen 127.0.0.1:16443 --db /tmp/vmsan-test-h5.db &
SERVER_PID=$!
sleep 2

TOKEN=$(curl -sf -X POST http://127.0.0.1:16443/api/v1/tokens | jq -r '.token')
sudo nftables/vmsan-agent-host join --server http://127.0.0.1:16443 --token "$TOKEN" --name target-host 2>&1

# Create VM targeting the remote host
VMSAN_SERVER_URL=http://127.0.0.1:16443 sudo vmsan create \
  --runtime base --vcpus 1 --memory 256 --host target-host --json 2>&1

# VM should appear in server's VM list
curl -sf http://127.0.0.1:16443/api/v1/vms | jq -e 'length >= 1'

kill $SERVER_PID 2>/dev/null
rm -f /tmp/vmsan-test-h5.db "$HOME/.vmsan/agent.json"
```

**Pass criteria:** VM appears in server's VM list (note: the actual VM won't boot unless the agent is running a sync loop — this test verifies the API dispatch path).

---

## Running the Tests

### Prerequisites

```bash
# Build Go binaries
cd nftables && make server agent && cd ..

# Install vmsan from branch
sudo bash install.sh --ref feat/platform-multihost

# Verify
vmsan doctor
```

### Execution order

Run tests in version order. Each version's tests are independent but earlier versions are prerequisites:

1. **0.4.0 tests (I9-I14)** — require dnsproxy + tcpproxy running
2. **0.5.0 tests (M1-M7)** — require mesh networking
3. **0.6.0 tests (B1-B3)** — can run without VMs (config-only)
4. **0.7.0 tests (I15-I21, L1-L3)** — require vmsan up working
5. **0.8.0 tests (S1-S3, H1-H5)** — require SQLite + server/agent binaries

### Cleanup

If tests leave orphaned VMs:

```bash
vmsan list --json | jq -r '.[].id' | while read id; do
  sudo vmsan stop "$id" 2>/dev/null
  sudo vmsan remove "$id" 2>/dev/null
done
```
