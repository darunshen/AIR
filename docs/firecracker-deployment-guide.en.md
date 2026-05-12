# Firecracker Deployment Guide

[中文](firecracker-deployment-guide.md)

This document covers only the host preparation, asset preparation, and baseline validation required to run AIR with the Firecracker provider on a real machine.

Scope boundary:

- command usage, session lifecycle, and log inspection belong in the [Operations Manual](operations-manual.en.md)
- this guide does not repeat AI agent workflow details

## 1. Scope

This guide is for:

- Linux hosts
- CPUs and kernels with KVM support
- local machines or self-hosted CI runners that need the Firecracker provider

This guide is not for:

- running Firecracker directly on macOS or Windows
- GitHub-hosted runners without KVM
- production-grade multi-tenant hardening details

GitHub Actions note:

- the `local` provider can run on GitHub-hosted runners
- the `firecracker` provider needs a self-hosted Linux runner
- that runner must expose readable and writable `/dev/kvm`

## 2. Host Prerequisites

At minimum you need:

- Linux
- `x86_64` or `aarch64`
- `/dev/kvm`
- a user with read/write access to `/dev/kvm`
- permission to create TAP devices and manage `iptables` rules (required only for `--network full`)
- a Firecracker binary
- `vmlinux`
- `rootfs.ext4`

AIR provides two unified entry points:

```bash
air init firecracker
air doctor --provider firecracker --human
```

Notes:

- `air init firecracker` interactively asks whether to download the official AIR image bundle or use self-managed assets
- when you choose the official AIR image path, it downloads the bundle that matches the current AIR version
- `air doctor` checks whether the environment is complete

## 3. Verify KVM

### 3.1 Check Modules

```bash
lsmod | grep kvm
```

### 3.2 Check The Device

```bash
ls -l /dev/kvm
```

### 3.3 Check Permissions

```bash
[ -r /dev/kvm ] && [ -w /dev/kvm ] && echo "OK" || echo "FAIL"
```

If permissions are missing, two common fixes are:

Option 1: add the user to the `kvm` group

```bash
sudo usermod -aG kvm "${USER}"
```

Option 2: apply a temporary ACL

```bash
sudo setfacl -m u:${USER}:rw /dev/kvm
```

## 4. Verify Full-Network Dependencies

For the full-network security level, AIR uses the standard Firecracker path:

- `virtio-net`
- host `tap`
- host `iptables MASQUERADE`

So the host also needs:

- `ip`
- `iptables`
- permission to create TAP devices
- permission to enable `net.ipv4.ip_forward`

Quick checks:

```bash
command -v ip
command -v iptables
cat /proc/sys/net/ipv4/ip_forward
ip route show default
```

In `full` mode, AIR will automatically:

- create a dedicated TAP device
- assign a host-side address to the TAP
- add `FORWARD` rules
- add `POSTROUTING MASQUERADE`
- configure the guest with a static IP, default route, and `resolv.conf`

When the session is deleted, AIR cleans up the rules and TAP device.

## 5. Firecracker Asset Strategy

The current recommended strategy is explicit:

1. use the official Firecracker release for the binary
2. use official demo or CI assets first for `vmlinux` and `rootfs.ext4`
3. then generate an AIR-ready rootfs with repository scripts

This maps to these scripts:

```bash
scripts/fetch-firecracker-ubuntu-assets.sh
scripts/prepare-firecracker-ubuntu-rootfs.sh
```

If you want an OpenClaude guest, also use:

```bash
scripts/prepare-openclaude-ubuntu-rootfs.sh
```

This OpenClaude guest build script also injects the minimum tool dependencies:

- `bash`
- `ripgrep`
- `curl`
- `git`
- `ca-certificates`

This is required so the OpenClaude Bash / Glob / Grep / basic network path can work directly inside the Firecracker guest.

## 6. Fetch Official Assets

### 5.1 Firecracker Binary

You can use the official release directly, or use the repository script:

```bash
scripts/fetch-firecracker-ubuntu-assets.sh
```

By default it places files under:

```text
assets/firecracker/
```

Including:

- `firecracker`
- `vmlinux.bin`
- `ubuntu-rootfs.ext4`

## 7. Prepare An AIR-Usable Rootfs

### 6.1 Baseline Guest

Inject `air-agent` into the official Ubuntu rootfs:

```bash
scripts/prepare-firecracker-ubuntu-rootfs.sh
```

The output typically includes:

- `assets/firecracker/ubuntu-rootfs-air.ext4`

### 6.2 OpenClaude Guest

If you want to run OpenClaude inside the guest:

```bash
scripts/prepare-openclaude-ubuntu-rootfs.sh
```

That script prepares a guest rootfs suitable for OpenClaude, Bun, and `air-agent`.

## 8. How AIR Finds Assets

AIR first honors explicit environment variables:

```bash
export AIR_VM_RUNTIME=firecracker
export AIR_VM_NETWORK=none
export AIR_FIRECRACKER_BIN=/absolute/path/to/firecracker
export AIR_FIRECRACKER_KERNEL=/absolute/path/to/vmlinux
export AIR_FIRECRACKER_ROOTFS=/absolute/path/to/rootfs.ext4
export AIR_KVM_DEVICE=/dev/kvm
```

If you do not set them, AIR also looks in these directories:

- `assets/firecracker/`
- `/usr/lib/air/firecracker/`
- `/usr/local/lib/air/firecracker/`
- `~/.local/share/air/firecracker/`

## 9. Baseline Validation

### 8.1 Preflight

```bash
air init firecracker
air doctor --provider firecracker --human
```

### 8.2 Lifecycle Validation

```bash
air session create --provider firecracker
air session inspect <session_id>
air session exec <session_id> "uname -a"
air session console <session_id> --follow
air session delete <session_id>
```

### 8.3 Workspace Validation

```bash
air session create --provider firecracker --workspace /absolute/path/to/repo
air session export-workspace <session_id> /tmp/air-export
```

This validates that:

- Firecracker boots
- guest `air-agent` becomes ready
- `vsock exec` works
- `/workspace` overlay mounts correctly
- the merged workspace can be exported

### 9.4 Full Network Validation

```bash
air session create --provider firecracker --network full
air session inspect <session_id>
air session exec <session_id> "ip addr show eth0"
air session exec <session_id> "ip route"
air session exec <session_id> "curl -I https://api.deepseek.com/anthropic"
```

This validates that:

- Firecracker `virtio-net` is attached
- the guest static IP is configured
- the default route points at the host TAP gateway
- host NAT egress works

## 9. Integration Test

The repository includes a real-machine lifecycle test:

```bash
AIR_FIRECRACKER_INTEGRATION=1 \
AIR_VM_RUNTIME=firecracker \
AIR_FIRECRACKER_BIN=/absolute/path/to/firecracker \
AIR_FIRECRACKER_KERNEL=/absolute/path/to/vmlinux \
AIR_FIRECRACKER_ROOTFS=/absolute/path/to/rootfs.ext4 \
AIR_KVM_DEVICE=/dev/kvm \
go test ./internal/vm -run TestFirecrackerIntegrationLifecycle -v
```

## 10. Common Issues

### 10.1 `firecracker binary not found`

Action:

- verify `PATH`
- or set `AIR_FIRECRACKER_BIN` explicitly

### 10.2 `AIR_FIRECRACKER_KERNEL is required`

Action:

- configure `AIR_FIRECRACKER_KERNEL`

### 10.3 `AIR_FIRECRACKER_ROOTFS is required`

Action:

- configure `AIR_FIRECRACKER_ROOTFS`

### 10.4 `kvm device is unavailable for firecracker runtime`

Action:

- go back to section 3 and verify `/dev/kvm`

### 10.5 `guest agent is not ready`

Action:

- inspect `console.log`
- inspect `events.jsonl`
- verify that the rootfs was prepared with repository scripts rather than using the raw demo rootfs directly

## 11. Current Recommendation

Move in this order:

1. official release `firecracker`
2. official demo `vmlinux` and `rootfs.ext4`
3. `prepare-firecracker-ubuntu-rootfs.sh`
4. `air doctor`
5. `air session create/exec/delete`
6. `--workspace` and `export-workspace`
7. OpenClaude guest and real agent workflows
