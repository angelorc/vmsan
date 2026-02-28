#!/usr/bin/env bash
set -euo pipefail

# ── Config ──────────────────────────────────────────────────────────────────
TARGET=${1:-40}
MEMORY=128
VCPUS=1
BATCH_SIZE=5
RESULTS_DIR="/tmp/vmsan-stress-$(date +%s)"
mkdir -p "$RESULTS_DIR"

# ── Helpers ─────────────────────────────────────────────────────────────────
ts()  { date +%s%3N; }
now() { date '+%H:%M:%S'; }
sep() { printf '\n%s\n' "$(printf '=%.0s' {1..70})"; }

log() { echo "[$(now)] $*"; }

snapshot_resources() {
  local label="$1"
  local mem_avail mem_used cpu_line
  mem_avail=$(awk '/MemAvailable/{print $2}' /proc/meminfo)
  mem_used=$(awk '/MemTotal/{t=$2} /MemAvailable/{a=$2} END{print t-a}' /proc/meminfo)
  cpu_line=$(head -1 /proc/stat)
  local vm_count
  vm_count=$(vmsan list --json 2>/dev/null | python3 -c "import sys,json; print(json.load(sys.stdin).get('count',0))" 2>/dev/null || echo "?")
  echo "$label,$vm_count,$(( mem_used / 1024 )),$(( mem_avail / 1024 )),$cpu_line" >> "$RESULTS_DIR/resources.csv"
  log "  [$label] VMs=$vm_count  MemUsed=$(( mem_used / 1024 ))M  MemAvail=$(( mem_avail / 1024 ))M"
}

# ── Pre-flight ──────────────────────────────────────────────────────────────
sep
log "VMSAN STRESS TEST — Target: $TARGET VMs ($MEMORY MiB / $VCPUS vCPU each)"
log "Results dir: $RESULTS_DIR"
log "Batch size: $BATCH_SIZE concurrent creates"
sep

# Clean slate
existing=$(vmsan list --json 2>/dev/null | python3 -c "import sys,json; d=json.load(sys.stdin); print(' '.join(v['id'] for v in d.get('vms',[])))" 2>/dev/null || true)
if [[ -n "$existing" ]]; then
  log "Cleaning up ${existing// /, }..."
  vmsan remove -f $existing 2>&1 | tail -1
fi

echo "phase,vm_count,mem_used_mb,mem_avail_mb,cpu_stat" > "$RESULTS_DIR/resources.csv"
echo "vm_id,create_ms,agent_ready" > "$RESULTS_DIR/create_times.csv"
snapshot_resources "baseline"

# ── Phase 1: Batch Create ──────────────────────────────────────────────────
sep
log "PHASE 1: Creating $TARGET VMs in batches of $BATCH_SIZE"
sep

CREATED_VMS=()
TOTAL_CREATE_START=$(ts)
FAILED=0

for (( batch_start=0; batch_start < TARGET; batch_start += BATCH_SIZE )); do
  batch_end=$(( batch_start + BATCH_SIZE ))
  [[ $batch_end -gt $TARGET ]] && batch_end=$TARGET
  batch_count=$(( batch_end - batch_start ))

  log "Batch $((batch_start / BATCH_SIZE + 1)): creating VMs $((batch_start+1))-$batch_end..."
  BATCH_START_T=$(ts)

  pids=()
  for (( i=batch_start; i < batch_end; i++ )); do
    (
      t0=$(ts)
      out=$(vmsan create --runtime base --vcpus "$VCPUS" --memory "$MEMORY" --json 2>&1) || true
      t1=$(ts)
      vm_id=$(echo "$out" | python3 -c "
import sys, json
for line in sys.stdin:
  line = line.strip()
  if not line: continue
  try:
    d = json.loads(line)
    if 'vmId' in d:
      print(d['vmId'])
      break
  except: pass
" 2>/dev/null || echo "FAIL")
      elapsed=$(( t1 - t0 ))
      echo "$vm_id,$elapsed" >> "$RESULTS_DIR/batch_${batch_start}.out"
    ) &
    pids+=($!)
  done

  # Wait for batch
  for pid in "${pids[@]}"; do
    wait "$pid" 2>/dev/null || true
  done

  BATCH_END_T=$(ts)
  log "  Batch done in $(( BATCH_END_T - BATCH_START_T )) ms"

  # Collect results
  if [[ -f "$RESULTS_DIR/batch_${batch_start}.out" ]]; then
    while IFS=, read -r vm_id elapsed; do
      if [[ "$vm_id" != "FAIL" && -n "$vm_id" ]]; then
        CREATED_VMS+=("$vm_id")
        echo "$vm_id,$elapsed,yes" >> "$RESULTS_DIR/create_times.csv"
      else
        FAILED=$((FAILED + 1))
        echo "FAIL,$elapsed,no" >> "$RESULTS_DIR/create_times.csv"
      fi
    done < "$RESULTS_DIR/batch_${batch_start}.out"
  fi

  snapshot_resources "after_batch_$((batch_start / BATCH_SIZE + 1))"

  # Check if we're running low on memory
  mem_avail=$(awk '/MemAvailable/{print $2}' /proc/meminfo)
  if (( mem_avail < 512000 )); then
    log "WARNING: Memory below 512 MB available — stopping creation"
    break
  fi
done

TOTAL_CREATE_END=$(ts)
TOTAL_CREATE_MS=$(( TOTAL_CREATE_END - TOTAL_CREATE_START ))
CREATED=${#CREATED_VMS[@]}

sep
log "PHASE 1 RESULTS: $CREATED created, $FAILED failed, total ${TOTAL_CREATE_MS}ms"
if (( CREATED > 0 )); then
  log "  Avg create time: $(( TOTAL_CREATE_MS / CREATED ))ms per VM"
fi
sep

# ── Phase 2: Health Check All VMs ──────────────────────────────────────────
sep
log "PHASE 2: Agent health check on all $CREATED VMs"
sep

HEALTH_OK=0
HEALTH_FAIL=0
HEALTH_START=$(ts)

echo "vm_id,health_ok,latency_ms" > "$RESULTS_DIR/health.csv"

for vm_id in "${CREATED_VMS[@]}"; do
  t0=$(ts)
  state=$(cat "/root/.vmsan/vms/${vm_id}.json" 2>/dev/null)
  guest_ip=$(echo "$state" | python3 -c "import sys,json; print(json.load(sys.stdin)['network']['guestIp'])" 2>/dev/null || echo "")
  token=$(echo "$state" | python3 -c "import sys,json; print(json.load(sys.stdin)['agentToken'])" 2>/dev/null || echo "")
  port=$(echo "$state" | python3 -c "import sys,json; print(json.load(sys.stdin).get('agentPort',9119))" 2>/dev/null || echo "9119")

  if [[ -n "$guest_ip" && -n "$token" ]]; then
    resp=$(curl -s --max-time 5 -H "Authorization: Bearer $token" "http://${guest_ip}:${port}/health" 2>/dev/null || echo "")
    t1=$(ts)
    if echo "$resp" | grep -q '"ok"'; then
      HEALTH_OK=$((HEALTH_OK + 1))
      echo "$vm_id,yes,$((t1 - t0))" >> "$RESULTS_DIR/health.csv"
    else
      HEALTH_FAIL=$((HEALTH_FAIL + 1))
      echo "$vm_id,no,$((t1 - t0))" >> "$RESULTS_DIR/health.csv"
    fi
  else
    HEALTH_FAIL=$((HEALTH_FAIL + 1))
    echo "$vm_id,no,0" >> "$RESULTS_DIR/health.csv"
  fi
done

HEALTH_END=$(ts)
log "Health: $HEALTH_OK OK, $HEALTH_FAIL FAIL ($(( HEALTH_END - HEALTH_START ))ms total)"

# ── Phase 3: TCP Outbound on sample ────────────────────────────────────────
sep
log "PHASE 3: TCP outbound test (sample of 5 VMs)"
sep

SAMPLE_SIZE=5
(( SAMPLE_SIZE > CREATED )) && SAMPLE_SIZE=$CREATED
TCP_OK=0
TCP_FAIL=0

echo "vm_id,tcp_ok,http_code,latency_ms" > "$RESULTS_DIR/tcp.csv"

for (( i=0; i < SAMPLE_SIZE; i++ )); do
  vm_id="${CREATED_VMS[$i]}"
  state=$(cat "/root/.vmsan/vms/${vm_id}.json" 2>/dev/null)
  guest_ip=$(echo "$state" | python3 -c "import sys,json; print(json.load(sys.stdin)['network']['guestIp'])" 2>/dev/null || echo "")
  token=$(echo "$state" | python3 -c "import sys,json; print(json.load(sys.stdin)['agentToken'])" 2>/dev/null || echo "")
  port=$(echo "$state" | python3 -c "import sys,json; print(json.load(sys.stdin).get('agentPort',9119))" 2>/dev/null || echo "9119")

  t0=$(ts)
  resp=$(curl -s --max-time 15 -X POST \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $token" \
    "http://${guest_ip}:${port}/exec" \
    -d '{"cmd":"curl","args":["-s","-o","/dev/null","-w","%{http_code}","--max-time","8","https://www.google.com"]}' 2>/dev/null || echo "")
  t1=$(ts)

  http_code=$(echo "$resp" | grep '"stdout"' | grep -oP '"data":"(\d+)"' | grep -oP '\d+' | tail -1 || echo "0")
  if [[ "$http_code" == "200" || "$http_code" == "301" ]]; then
    TCP_OK=$((TCP_OK + 1))
    echo "$vm_id,yes,$http_code,$((t1 - t0))" >> "$RESULTS_DIR/tcp.csv"
    log "  $vm_id: HTTP $http_code ($((t1 - t0))ms)"
  else
    TCP_FAIL=$((TCP_FAIL + 1))
    echo "$vm_id,no,$http_code,$((t1 - t0))" >> "$RESULTS_DIR/tcp.csv"
    log "  $vm_id: FAIL ($((t1 - t0))ms)"
  fi
done

log "TCP outbound: $TCP_OK/$SAMPLE_SIZE OK"

# ── Phase 4: Resource snapshot at peak ─────────────────────────────────────
sep
log "PHASE 4: Peak resource snapshot ($CREATED VMs running)"
sep

snapshot_resources "peak"

log "Firecracker processes: $(pgrep -c firecracker 2>/dev/null || echo 0)"
log "Total Firecracker RSS: $(ps aux | grep '[f]irecracker' | awk '{sum+=$6} END{printf "%.0f MiB\n", sum/1024}')"
log "iptables rules: $(iptables -L -n 2>/dev/null | wc -l) lines"
log "Network namespaces: $(ip netns list 2>/dev/null | wc -l)"
log "TAP devices: $(ls /sys/class/net/ | grep -c fhvm 2>/dev/null || echo 0)"

# Per-VM RSS breakdown
echo "vm_id,pid,rss_kb" > "$RESULTS_DIR/rss.csv"
for vm_id in "${CREATED_VMS[@]}"; do
  state=$(cat "/root/.vmsan/vms/${vm_id}.json" 2>/dev/null)
  pid=$(echo "$state" | python3 -c "import sys,json; print(json.load(sys.stdin)['pid'])" 2>/dev/null || echo "")
  if [[ -n "$pid" ]] && [[ -d "/proc/$pid" ]]; then
    rss=$(awk '/VmRSS/{print $2}' "/proc/$pid/status" 2>/dev/null || echo "0")
    echo "$vm_id,$pid,$rss" >> "$RESULTS_DIR/rss.csv"
  fi
done

# ── Phase 5: Cleanup ───────────────────────────────────────────────────────
sep
log "PHASE 5: Removing all $CREATED VMs"
sep

CLEANUP_START=$(ts)

# Remove in batches to avoid overwhelming
for (( i=0; i < CREATED; i += 10 )); do
  batch=("${CREATED_VMS[@]:$i:10}")
  vmsan remove -f "${batch[@]}" 2>&1 | grep -c "Removed" || true
done

CLEANUP_END=$(ts)
CLEANUP_MS=$(( CLEANUP_END - CLEANUP_START ))

snapshot_resources "after_cleanup"

log "Cleanup: $CREATED VMs removed in ${CLEANUP_MS}ms"
if (( CREATED > 0 )); then
  log "  Avg removal time: $(( CLEANUP_MS / CREATED ))ms per VM"
fi

# Verify clean
remaining=$(vmsan list --json 2>/dev/null | python3 -c "import sys,json; print(json.load(sys.stdin).get('count',0))" 2>/dev/null || echo "?")
remaining_netns=$(ip netns list 2>/dev/null | wc -l)
remaining_taps=$(ls /sys/class/net/ 2>/dev/null | grep -c fhvm || echo 0)
log "Remaining: VMs=$remaining netns=$remaining_netns TAPs=$remaining_taps"

# ── Final Report ────────────────────────────────────────────────────────────
sep
log "STRESS TEST COMPLETE — FINAL REPORT"
sep

# Parse CSV stats
create_times=$(tail -n +2 "$RESULTS_DIR/create_times.csv" | grep -v FAIL | cut -d, -f2)
if [[ -n "$create_times" ]]; then
  min_create=$(echo "$create_times" | sort -n | head -1)
  max_create=$(echo "$create_times" | sort -n | tail -1)
  avg_create=$(echo "$create_times" | awk '{s+=$1} END{printf "%.0f", s/NR}')
  p50_create=$(echo "$create_times" | sort -n | awk "NR==int($(echo "$create_times" | wc -l)*0.5){print}")
  p95_create=$(echo "$create_times" | sort -n | awk "NR==int($(echo "$create_times" | wc -l)*0.95){print}")
fi

health_latencies=$(tail -n +2 "$RESULTS_DIR/health.csv" | grep ",yes," | cut -d, -f3)
if [[ -n "$health_latencies" ]]; then
  avg_health=$(echo "$health_latencies" | awk '{s+=$1} END{printf "%.0f", s/NR}')
fi

cat <<REPORT

╭────────────────────────────────────────────────────────────╮
│                   STRESS TEST RESULTS                      │
├────────────────────────────────────────────────────────────┤
│                                                            │
│  Target VMs:        $TARGET
│  Created:           $CREATED
│  Failed:            $FAILED
│                                                            │
│  ── Creation ──────────────────────────────────            │
│  Total time:        ${TOTAL_CREATE_MS}ms
│  Avg per VM:        ${avg_create:-n/a}ms
│  Min:               ${min_create:-n/a}ms
│  Max:               ${max_create:-n/a}ms
│  p50:               ${p50_create:-n/a}ms
│  p95:               ${p95_create:-n/a}ms
│                                                            │
│  ── Health ────────────────────────────────────            │
│  Healthy:           $HEALTH_OK / $CREATED
│  Avg latency:       ${avg_health:-n/a}ms
│                                                            │
│  ── TCP Outbound ──────────────────────────────            │
│  OK:                $TCP_OK / $SAMPLE_SIZE
│                                                            │
│  ── Cleanup ───────────────────────────────────            │
│  Total time:        ${CLEANUP_MS}ms
│  Avg per VM:        $(( CREATED > 0 ? CLEANUP_MS / CREATED : 0 ))ms
│                                                            │
╰────────────────────────────────────────────────────────────╯

REPORT

log "Detailed CSVs in: $RESULTS_DIR/"
ls -la "$RESULTS_DIR/"
