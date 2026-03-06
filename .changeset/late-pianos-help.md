---
"vmsan": patch
---

Harden installer and VM networking reliability across mixed Linux hosts.

- fix branch/commit installs and uninstalls in `install.sh`, including safer cleanup of per-VM iptables rules
- migrate the default VM subnet to `198.19.x.x` while preserving compatibility with legacy persisted `172.16.x.x` states
- keep stopped VM slots reserved, tighten persisted IP parsing, and restore agent connectivity on hosts with restrictive local firewalls
