---
"vmsan": minor
---

feat: defense-in-depth egress filtering with DNS and SNI layers

- DNS filtering via dnsproxy sidecar (per-VM, DNAT port 53)
- SNI filtering via in-process tcpproxy (per-VM, DNAT port 443)
- ECH SvcParam stripping prevents Encrypted ClientHello bypass
- DNS rebinding protection blocks private IPs in DNS responses
- vmsan-gateway daemon manages proxy lifecycle
- `--allow-icmp` flag for ICMP traffic
- `vmsan logs --dns` for DNS query visibility
- Legacy iptables codepath removed
- Port range overlap fixed (HTTP 10080→10698)
