# vmsan

## 0.1.0

### Minor Changes

- [#56](https://github.com/angelorc/vmsan/pull/56) [`07ac73b`](https://github.com/angelorc/vmsan/commit/07ac73b578e3cb96bc1504390ab905ff00dc08c9) Thanks [@angelorc](https://github.com/angelorc)! - feat: add `vmsan doctor` diagnostic command and fix JSON output consistency

- [#47](https://github.com/angelorc/vmsan/pull/47) [`bfc12a7`](https://github.com/angelorc/vmsan/commit/bfc12a795ff2c4024b60a2df624c140e86c3806a) Thanks [@angelorc](https://github.com/angelorc)! - Add KVM pre-flight check to `vmsan create` and cleanup verification after `vmsan stop`/`vmsan remove`

- [#46](https://github.com/angelorc/vmsan/pull/46) [`913e721`](https://github.com/angelorc/vmsan/commit/913e72150acbb449ba8be345f580f78fd11c563f) Thanks [@angelorc](https://github.com/angelorc)! - Add state file versioning to VM state store for future migration support

### Patch Changes

- [#48](https://github.com/angelorc/vmsan/pull/48) [`c824d44`](https://github.com/angelorc/vmsan/commit/c824d44c7961808ec08c965cfa87f94a5fb164c6) Thanks [@angelorc](https://github.com/angelorc)! - Audit and fix CLI help text for all commands

- [#55](https://github.com/angelorc/vmsan/pull/55) [`0899f7f`](https://github.com/angelorc/vmsan/commit/0899f7fee2ee18ffa7346612bf41756479764ac7) Thanks [@angelorc](https://github.com/angelorc)! - docs: sync documentation with beta.1 CLI changes

- [#59](https://github.com/angelorc/vmsan/pull/59) [`14492aa`](https://github.com/angelorc/vmsan/commit/14492aa12970bfcfab6c1938cad6970a30eee7c3) Thanks [@angelorc](https://github.com/angelorc)! - docs: add known limitations and doctor command to README

- [#54](https://github.com/angelorc/vmsan/pull/54) [`d518f9a`](https://github.com/angelorc/vmsan/commit/d518f9a8a917285e373cf2b2adbbd56d8f6e5235) Thanks [@angelorc](https://github.com/angelorc)! - test: add e2e smoke test script and manual test matrix

- [#60](https://github.com/angelorc/vmsan/pull/60) [`d96e740`](https://github.com/angelorc/vmsan/commit/d96e74097500cbeba8ac5ba0f7c6c92a97a29d21) Thanks [@angelorc](https://github.com/angelorc)! - fix: use hoisted linker for docs to work around @nuxt/content context isolation bug

- [#63](https://github.com/angelorc/vmsan/pull/63) [`de8fb76`](https://github.com/angelorc/vmsan/commit/de8fb7683fd6aa59bcf68bbc1edb135e1892ff79) Thanks [@angelorc](https://github.com/angelorc)! - fix: include error code and fix/why fields at top level of JSON error output

- [#44](https://github.com/angelorc/vmsan/pull/44) [`725924c`](https://github.com/angelorc/vmsan/commit/725924c0eb05ac09649a1dfd593258c9ba0f719d) Thanks [@angelorc](https://github.com/angelorc)! - Fix install failure on systems without loop devices by replacing `mount -o loop` with `mkfs.ext4 -d` for rootfs creation, and auto-install Docker when not found instead of skipping runtime builds.

- [#40](https://github.com/angelorc/vmsan/pull/40) [`fe41441`](https://github.com/angelorc/vmsan/commit/fe4144124d9fc371f7cd3d449c03d9db744008e7) Thanks [@angelorc](https://github.com/angelorc)! - Harden installer and VM networking reliability across mixed Linux hosts.

  - fix branch/commit installs and uninstalls in `install.sh`, including safer cleanup of per-VM iptables rules
  - migrate the default VM subnet to `198.19.x.x` while preserving compatibility with legacy persisted `172.16.x.x` states
  - keep stopped VM slots reserved, tighten persisted IP parsing, and restore agent connectivity on hosts with restrictive local firewalls

- [#62](https://github.com/angelorc/vmsan/pull/62) [`5092c9d`](https://github.com/angelorc/vmsan/commit/5092c9d0c839ccb53f0b52301277c9edab9743be) Thanks [@angelorc](https://github.com/angelorc)! - Persist isolation flags (disableSeccomp, disablePidNs, disableCgroup) in VM state so they are honored on restart

- [#36](https://github.com/angelorc/vmsan/pull/36) [`b9a5d9c`](https://github.com/angelorc/vmsan/commit/b9a5d9c595301e07419cefb9af6a355fe4ca686b) Thanks [@angelorc](https://github.com/angelorc)! - Improve runtime VM usability and the release lifecycle.

  - fix PATH handling for agent exec and PTY shells so Node/npm and user-global installs work reliably inside runtime VMs
  - improve source installs in `install.sh` with branch/commit bootstrap support and modern Go enforcement
  - switch the project to a real Changesets workflow with authored changesets, release PRs, and npm/agent publishing from reviewed version commits

- [#49](https://github.com/angelorc/vmsan/pull/49) [`cab910a`](https://github.com/angelorc/vmsan/commit/cab910a2a07f374892362af4f227e1c8d9d2f245) Thanks [@angelorc](https://github.com/angelorc)! - Add comprehensive unit test suite and reduce stale lock timeout from 5m to 30s

## 0.1.0-beta.2

### Minor Changes

- [#56](https://github.com/angelorc/vmsan/pull/56) [`07ac73b`](https://github.com/angelorc/vmsan/commit/07ac73b578e3cb96bc1504390ab905ff00dc08c9) Thanks [@angelorc](https://github.com/angelorc)! - feat: add `vmsan doctor` diagnostic command and fix JSON output consistency

### Patch Changes

- [#55](https://github.com/angelorc/vmsan/pull/55) [`0899f7f`](https://github.com/angelorc/vmsan/commit/0899f7fee2ee18ffa7346612bf41756479764ac7) Thanks [@angelorc](https://github.com/angelorc)! - docs: sync documentation with beta.1 CLI changes

- [#54](https://github.com/angelorc/vmsan/pull/54) [`d518f9a`](https://github.com/angelorc/vmsan/commit/d518f9a8a917285e373cf2b2adbbd56d8f6e5235) Thanks [@angelorc](https://github.com/angelorc)! - test: add e2e smoke test script and manual test matrix

## 0.1.0-beta.1

### Minor Changes

- [#47](https://github.com/angelorc/vmsan/pull/47) [`bfc12a7`](https://github.com/angelorc/vmsan/commit/bfc12a795ff2c4024b60a2df624c140e86c3806a) Thanks [@angelorc](https://github.com/angelorc)! - Add KVM pre-flight check to `vmsan create` and cleanup verification after `vmsan stop`/`vmsan remove`

- [#46](https://github.com/angelorc/vmsan/pull/46) [`913e721`](https://github.com/angelorc/vmsan/commit/913e72150acbb449ba8be345f580f78fd11c563f) Thanks [@angelorc](https://github.com/angelorc)! - Add state file versioning to VM state store for future migration support

### Patch Changes

- [#48](https://github.com/angelorc/vmsan/pull/48) [`c824d44`](https://github.com/angelorc/vmsan/commit/c824d44c7961808ec08c965cfa87f94a5fb164c6) Thanks [@angelorc](https://github.com/angelorc)! - Audit and fix CLI help text for all commands

- [#49](https://github.com/angelorc/vmsan/pull/49) [`cab910a`](https://github.com/angelorc/vmsan/commit/cab910a2a07f374892362af4f227e1c8d9d2f245) Thanks [@angelorc](https://github.com/angelorc)! - Add comprehensive unit test suite and reduce stale lock timeout from 5m to 30s

## 0.1.0-alpha.27

### Patch Changes

- [#44](https://github.com/angelorc/vmsan/pull/44) [`725924c`](https://github.com/angelorc/vmsan/commit/725924c0eb05ac09649a1dfd593258c9ba0f719d) Thanks [@angelorc](https://github.com/angelorc)! - Fix install failure on systems without loop devices by replacing `mount -o loop` with `mkfs.ext4 -d` for rootfs creation, and auto-install Docker when not found instead of skipping runtime builds.

## 0.1.0-alpha.26

### Patch Changes

- [#40](https://github.com/angelorc/vmsan/pull/40) [`fe41441`](https://github.com/angelorc/vmsan/commit/fe4144124d9fc371f7cd3d449c03d9db744008e7) Thanks [@angelorc](https://github.com/angelorc)! - Harden installer and VM networking reliability across mixed Linux hosts.

  - fix branch/commit installs and uninstalls in `install.sh`, including safer cleanup of per-VM iptables rules
  - migrate the default VM subnet to `198.19.x.x` while preserving compatibility with legacy persisted `172.16.x.x` states
  - keep stopped VM slots reserved, tighten persisted IP parsing, and restore agent connectivity on hosts with restrictive local firewalls

## 0.1.0-alpha.25

### Patch Changes

- [#36](https://github.com/angelorc/vmsan/pull/36) [`b9a5d9c`](https://github.com/angelorc/vmsan/commit/b9a5d9c595301e07419cefb9af6a355fe4ca686b) Thanks [@angelorc](https://github.com/angelorc)! - Improve runtime VM usability and the release lifecycle.

  - fix PATH handling for agent exec and PTY shells so Node/npm and user-global installs work reliably inside runtime VMs
  - improve source installs in `install.sh` with branch/commit bootstrap support and modern Go enforcement
  - switch the project to a real Changesets workflow with authored changesets, release PRs, and npm/agent publishing from reviewed version commits

## 0.1.0

### Patch Changes

- Add Cloudflare Tunnel plugin and improvements (#34)

## 0.1.0

### Patch Changes

- Merge pull request #32 from angelorc/fix/connect-500-home-permissions

## 0.1.0

### Patch Changes

- docs: make demo gif full-width section

## 0.1.0

### Patch Changes

- docs: add demo gif to homepage

## 0.1.0

### Patch Changes

- Merge pull request #30 from angelorc/docs/readme-refresh

## 0.1.0

### Patch Changes

- Merge pull request #29 from angelorc/docs/add-exec-command

## 0.1.0

### Patch Changes

- Merge pull request #28 from angelorc/refactor/sysuser-credentials

## 0.1.0

### Patch Changes

- Merge pull request #27 from angelorc/fix/exec-command-ux

## 0.1.0

### Patch Changes

- Merge pull request #26 from angelorc/feat/exec-command

## 0.1.0

### Patch Changes

- Merge pull request #25 from angelorc/refactor/vm-validation-timeout-killer

## 0.1.0

### Patch Changes

- Merge pull request #23 from angelorc/feat/vmsan-context

## 0.1.0

### Patch Changes

- docs: use vmsan.dev/install URL and update landing page

## 0.1.0

### Patch Changes

- fix: pin unimport to 5.6.0 to work around mlly export misparse

## 0.1.0

### Patch Changes

- docs: update dependencies, config and assets

## 0.1.0

### Patch Changes

- docs: comment out hooks section in nuxt.config.ts to prevent runtime errors

## 0.1.0

### Patch Changes

- Merge pull request #22 from angelorc/fix/networking-cgroup-download

## 0.1.0

### Patch Changes

- docs: remove unnecessary installation command from index

## 0.1.0

### Patch Changes

- Merge pull request #21 from angelorc/docs/add-documentation-site

## 0.1.0

### Patch Changes

- Merge pull request #20 from angelorc/feat/auto-release-on-merge

## 0.1.0

### Patch Changes

- [#17](https://github.com/angelorc/vmsan/pull/17) [`9a60cd6`](https://github.com/angelorc/vmsan/commit/9a60cd68ba8a820664e82a59f5e41b68846ad44d) Thanks [@angelorc](https://github.com/angelorc)! - simplify release workflow: single PR to version and publish

## 0.1.0

### Patch Changes

- [#15](https://github.com/angelorc/vmsan/pull/15) [`c3df850`](https://github.com/angelorc/vmsan/commit/c3df8500d9108d04122bb1f5f931fcef28c5c9d7) Thanks [@angelorc](https://github.com/angelorc)! - fix install script to support pre-release tags

## 0.1.0

### Patch Changes

- [#13](https://github.com/angelorc/vmsan/pull/13) [`1056bd6`](https://github.com/angelorc/vmsan/commit/1056bd6da50f2534561af2030cc5848c0a535565) Thanks [@angelorc](https://github.com/angelorc)! - fix install script to use GitHub releases API for agent download URL

## 0.1.0

### Patch Changes

- [#8](https://github.com/angelorc/vmsan/pull/8) [`6df6885`](https://github.com/angelorc/vmsan/commit/6df6885783804b822e30a1e6a72270b446bd23f2) Thanks [@angelorc](https://github.com/angelorc)! - fix release pipeline: use NODE_AUTH_TOKEN for npm publish and idempotent tag step

## 0.1.0

### Patch Changes

- [`1fa2ed0`](https://github.com/angelorc/vmsan/commit/1fa2ed040803722afb00d4831c98a8f1a484cfa9) Thanks [@angelorc](https://github.com/angelorc)! - test: verify automated release pipeline
