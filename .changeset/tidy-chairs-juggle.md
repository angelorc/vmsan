---
"vmsan": patch
---

Improve runtime VM usability and the release lifecycle.

- fix PATH handling for agent exec and PTY shells so Node/npm and user-global installs work reliably inside runtime VMs
- improve source installs in `install.sh` with branch/commit bootstrap support and modern Go enforcement
- switch the project to a real Changesets workflow with authored changesets, release PRs, and npm/agent publishing from reviewed version commits
