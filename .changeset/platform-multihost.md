---
"vmsan": minor
---

## 0.7.0 "Ignition" — Platform DX

- `vmsan up` — deploy services from vmsan.toml (single and multi-service)
- `vmsan deploy` — re-deploy code without recreating VMs
- `vmsan status` — project overview table
- `vmsan down` — stop all services (`--destroy` to remove)
- `vmsan secrets` — encrypted secrets management
- Built-in accessories: postgres and redis auto-provisioning
- Health checks (HTTP, TCP, exec) with dependency ordering
- Release commands for post-build migrations

## 0.8.0 "Outpost" — Multi-Host Foundation

- SQLite state store (replaces JSON files)
- `vmsan migrate` — JSON to SQLite migration
- `vmsan server` — control plane with HTTP API
- `vmsan agent join` — worker registration with token auth
- Pull-based sync engine (10s polling with backoff)
- `vmsan hosts` — manage remote hosts
- `--host` flag on `vmsan create` for remote deployment
