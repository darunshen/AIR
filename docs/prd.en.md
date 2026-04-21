# AIR Product Requirements

[中文](prd.md)

This document is the English companion of the Chinese PRD.

## 1. Product Summary

AIR, short for Agent Isolation Runtime, provides a safer execution environment for AI agents by running untrusted code inside lightweight VMs instead of shared-kernel containers.

## 2. Product Vision

Make safe-by-default code execution standard infrastructure for AI agents.

## 3. Target Users

- AI coding agent platforms
- Sandbox execution providers
- Security-sensitive automation systems
- Developer tools that need long-lived isolated sessions

## 4. Core Problem

AI-generated code is often executed in environments designed for trusted workloads. AIR changes the default by assuming generated code is untrusted and by strengthening the isolation boundary.

## 5. Primary Scenarios

### 5.1 One-shot Execution

Run a task once and destroy the environment immediately after completion.

### 5.2 Stateful Session Execution

Create a session, execute multiple commands in the same VM, preserve filesystem state, and delete the session when work is done.

## 6. Functional Requirements

- `run`: create a disposable VM, execute a task, and return `stdout`, `stderr`, and `exit_code`
- `session`: create, execute inside, and delete a persistent session
- isolation control: default no-network mode with CPU, memory, and timeout limits
- environment management: read-only base image plus per-session writable state
- lifecycle management: clear states for create, run, idle, stop, and delete

## 7. Non-functional Requirements

- security
- reproducibility
- stability under transport or guest failures
- observability of commands, outputs, state, and duration

## 8. MVP Boundary

The MVP stays local-first: single node, CLI, local state, file or simple guest communication, and no multi-tenant platform features.

## 9. Success Metrics

- task success rate
- session creation latency
- average exec latency
- cleanup success rate
- number of isolation defects
