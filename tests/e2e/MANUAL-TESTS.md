# Manual Test Matrix

These tests require a KVM-capable host with vmsan installed and cannot be fully automated.

| Scenario | Steps | Expected |
|----------|-------|----------|
| Docker image | `vmsan create --from-image ubuntu:latest` | VM boots, no agent (expected) |
| Custom policy | `vmsan create --network-policy custom --allowed-domain "github.com,*.github.com"` | Only GitHub domains reachable |
| Bandwidth limit | `vmsan create --bandwidth 50mbit` | Throttle applied |
| Timeout | `vmsan create --timeout 5m` | VM auto-stops after 5m |
| Multiple runtimes | Create with `--runtime node22`, `--runtime python3.13` | Each boots with correct runtime |
