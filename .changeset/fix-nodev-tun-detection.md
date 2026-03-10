---
"vmsan": patch
---

Detect `nodev` filesystem and `/dev/net/tun` issues that prevent Firecracker from opening TAP devices inside the jailer chroot. Adds doctor checks and actionable error messages with fix instructions.
