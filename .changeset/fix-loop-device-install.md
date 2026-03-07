---
"vmsan": patch
---

Fix install failure on systems without loop devices by replacing `mount -o loop` with `mkfs.ext4 -d` for rootfs creation, and auto-install Docker when not found instead of skipping runtime builds.
