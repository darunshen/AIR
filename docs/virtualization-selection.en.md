# AIR Virtualization Selection

[中文](virtualization-selection.md)

## 1. Goal

Select the primary virtualization stack for AIR based on isolation strength, operational simplicity, and fit for AI agent execution.

## 2. Candidates

- Firecracker
- Cloud Hypervisor
- QEMU/KVM

## 3. Comparison Summary

### Firecracker

Best fit for lightweight, sandbox-style workloads with a clear minimal device model and strong VM isolation.

### Cloud Hypervisor

Technically solid, but less aligned with the current product focus and ecosystem path.

### QEMU/KVM

Flexible and mature, but heavier than needed for AIR's default path.

## 4. Decision

Firecracker is the primary choice.

## 5. Recommended Stack

- virtualization: Firecracker
- guest transport: `vsock`
- disk/image model: base image plus writable overlay

## 6. Staged Rollout

Adopt Firecracker first, then improve guest communication, sessionization, and performance.

## 7. Why Others Are Not Primary

QEMU is too broad and heavy for the default product story. Cloud Hypervisor remains a possible alternative, not the first path.

## 8. Final Summary

AIR should optimize for a focused, safer-by-default VM runtime instead of a generic hypervisor abstraction too early.
