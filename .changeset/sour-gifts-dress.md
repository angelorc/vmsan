---
"vmsan": patch
---

Move built-in runtime distribution to Cloudflare R2.

- switch release installs to manifest-driven runtime downloads
- keep source installs on local runtime builds
- reduce default install requirements by removing Docker from the normal release path
