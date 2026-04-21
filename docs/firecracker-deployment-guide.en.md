# Firecracker Deployment Guide

[中文](firecracker-deployment-guide.md)

## 1. Scope

This guide covers preparing a real machine to run AIR with the Firecracker provider.

## 2. Host Prerequisites

You need Linux with KVM support, access to `/dev/kvm`, and a user that can execute Firecracker.

## 3. Verify KVM

- confirm kernel modules are loaded
- confirm `/dev/kvm` exists
- confirm the current user has read/write access

## 4. Obtain The Firecracker Binary

Use the official Firecracker release first. Building from source is possible but not the recommended starting point.

## 5. Prepare Kernel And Rootfs

Current recommendation:

- use the official Firecracker release binary
- use official demo or CI assets first for `vmlinux` and `rootfs.ext4`
- move to custom-maintained images only after the flow is proven

## 6. Verify Assets

Check that the binary, kernel image, and rootfs file all exist and are readable.

## 7. Configure AIR

Export `AIR_FIRECRACKER_BIN`, `AIR_FIRECRACKER_KERNEL`, `AIR_FIRECRACKER_ROOTFS`, and related runtime variables.

## 8. Validate With AIR

Create a session, inspect the runtime directory, run commands, and delete the session.

## 9. Run Integration Tests

Use AIR commands or integration tests to validate lifecycle correctness before relying on the environment.

## 10. Logs And Artifacts

Key runtime artifacts include console logs, config snapshots, metrics, and per-session runtime directories.

## 11. Common Issues

Typical failures include missing Firecracker binary, missing kernel or rootfs, unavailable KVM, and guest-agent readiness problems.

## 12. What Not To Do Yet

Do not over-customize the guest image before the base deployment path is stable.

## 13. Recommended Next Step

Start with the official binary and demo assets, prove the lifecycle, then gradually harden the environment.
