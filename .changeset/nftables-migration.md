---
"vmsan": minor
---

Migrate firewall backend from iptables to nftables with atomic rule application.

**Breaking changes:**

- ICMP blocked by default from VMs (prevents ICMP tunneling)
- UDP blocked by default except DNS (prevents UDP data exfiltration)
- nftables kernel support required on host (kernel ≥ 5.10)
- Reserved port ranges: 10053-10307, 10443-10697, 10080-10334 (for future DNS/SNI proxy)
- Host firewalls (ufw/firewalld) may need explicit allow rules for vmsan traffic

**New features:**

- Atomic nftables rule application via `google/nftables` netlink library
- Per-VM table isolation (`vmsan_<vmId>`) — one `DelTable()` for complete cleanup
- DoT (TCP 853) and DoH blocking for DNS bypass prevention
- Cross-VM isolation blocking internal subnets
- Deterministic port allocation for future DNS/SNI proxy
- Per-namespace `ip_forward` setting
- `vmsan doctor` checks for nftables kernel support and host firewall detection
- Backward compatibility: `VMSAN_LEGACY_IPTABLES=1` env var for iptables fallback
