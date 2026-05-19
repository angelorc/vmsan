# vmsan Agent Instructions

Firecracker microVM sandbox toolkit with TypeScript CLI + Go in-VM agent.

## Build order (critical)

When setting up from source, **always build the agent first**:

```bash
cd agent && make install && cd ..
bun run build
```

The CLI expects `~/.vmsan/bin/vmsan-agent` to exist. Agent changes require `make install` to update the installed binary.

## Development commands

```bash
# Standard workflow
bun run lint      # oxlint + oxfmt check (not eslint/prettier)
bun run typecheck # tsc --noEmit
bun run build     # obuild bundle
bun run test      # vitest unit tests only

# Fix formatting/linting
bun run lint:fix  # or bun run fmt

# Dev mode (watch)
bun run dev       # obuild --stub

# Link local build for CLI testing
ln -sf "$(pwd)/dist/bin/cli.mjs" ~/.vmsan/bin/vmsan
```

**CI order**: `lint → typecheck → build → test` — match this locally to avoid CI failures.

## Go agent

The `agent/` directory is a standalone Go module (part of `go.work` workspace with `nftables/`).

```bash
cd agent
make build    # Static Linux binary
make install  # Copies to ~/.vmsan/bin/vmsan-agent
make clean    # Remove binary
```

Agent runs inside Firecracker VMs and exposes HTTP API for file ops, command execution, and shell access.

## Testing

- **Unit tests**: `bun run test` — runs `tests/unit/**/*.test.ts` (Vitest)
- **E2E tests**: Manual only — see `tests/e2e/MANUAL-TESTS.md` (requires KVM host + vmsan installed)
- Tests run with `singleFork: true` and 3-minute timeouts

## Changesets (PR requirement)

PRs **must include a changeset** unless changes are only in:
- `README.md`, `CHANGELOG.md`, `LICENSE`
- `docs/`, `.github/`, `.changeset/README.md`, `.changeset/config.json`, `.changeset/pre.json`

```bash
bunx changeset       # Create changeset
bunx changeset status # Check status
```

CI enforces this — PRs without changesets fail unless exempt.

## Architecture

```
bin/cli.ts          CLI entry (citty)
src/
  commands/         CLI subcommands (create, exec, connect, etc.)
  services/         Firecracker client, agent client, VM service
  lib/              Jailer, networking, shell, logging utilities
  errors/           Typed error system
  generated/        Firecracker API types
agent/              Go agent (runs in VMs)
  handler_*.go      HTTP API handlers
  shell/            PTY/shell implementation
```

**State directory**: `~/.vmsan/` (vms/, jailer/, bin/, kernels/, rootfs/, registry/, snapshots/)

## Tooling quirks

- **Formatter/linter**: `oxlint` + `oxfmt` (not eslint/prettier) — use `bun run lint:fix` or `bun run fmt`
- **Build tool**: `obuild` (not tsc/esbuild directly)
- **Package manager**: Bun >= 1.2 (enforced in `package.json`)
- **Module system**: ES modules only (`"type": "module"`)
- **TypeScript**: `isolatedDeclarations: true` — all exports need explicit types

## Common mistakes to avoid

1. **Forgetting to build agent** — CLI will fail at runtime if `~/.vmsan/bin/vmsan-agent` is missing
2. **Using eslint/prettier** — this repo uses oxlint/oxfmt
3. **Creating PR without changeset** — CI will fail unless files are in exempt list
4. **Running e2e tests** — they're manual only, require KVM + installed vmsan
5. **Skipping typecheck** — CI runs it after lint, before build

## Release workflow

```bash
bunx changeset version  # Bump version + update CHANGELOG
bun run build
bunx changeset publish  # Publish to npm
```

Automated via `.github/workflows/release.yml` on merge to main (when changesets exist).
