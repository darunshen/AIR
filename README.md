# AIR

[中文](README.zh-CN.md)

AIR is a VM-backed runtime for coding agents. Its main entry point is:

```bash
air chat
```

`air chat` starts an interactive OpenClaude session and can prepare the needed
Firecracker assets, OpenClaude guest image, host runtime, and model config on
first use.

## Quick Start

Start a chat session:

```bash
air chat
```

Common Firecracker PTY mode:

```bash
sudo -E env PATH="$PATH" HOME="$HOME" air chat \
  --provider firecracker \
  --network full \
  --pty \
  --auto-approve \
  --workspace /path/to/repo
```

If the workspace is large and you restart often, reuse the workspace snapshot:

```bash
sudo -E env PATH="$PATH" HOME="$HOME" air chat \
  --provider firecracker \
  --network full \
  --pty \
  --auto-approve \
  --workspace /path/to/repo \
  --workspace-cache
```

## Common Commands

Run one command in an isolated runtime:

```bash
air run -- echo hello
air run --provider firecracker --workspace /path/to/repo -- make test
```

Create and use a stateful session:

```bash
air session create --provider firecracker --workspace /path/to/repo
air session exec <session-id> "go test ./..."
air session events <session-id> --tail=100
air session console <session-id> --tail=100
air session export-workspace <session-id> ./out --force
air session delete <session-id>
```

Check or prepare Firecracker:

```bash
air doctor --provider firecracker --human
air init firecracker
```

Clean stale sessions:

```bash
air gc --all
```

## Install

Use the latest GitHub release assets:

```text
https://github.com/darunshen/AIR/releases/latest
```

After installing a release build, `air chat` can download the matching official
Firecracker and OpenClaude bundles when they are missing.

## Build From Source

```bash
go build -o /tmp/air ./cmd/air
/tmp/air version
```

## Notes

- `air chat` is the recommended product entry point.
- Firecracker mode needs `/dev/kvm` access and usually requires `sudo` unless
  your user has permission to create TAP/network resources.
- `--workspace-cache` speeds up repeated starts by reusing a workspace snapshot.
  Refresh the cache when you need newer host workspace contents inside the VM.
- PTY mode forwards OpenClaude output directly, including terminal UI updates.

## Documentation

- [Documentation Index](docs/README.en.md) / [中文](docs/README.md)
- [OpenClaude Integration](docs/openclaude-integration.en.md) / [中文](docs/openclaude-integration.md)
- [Firecracker Deployment Guide](docs/firecracker-deployment-guide.en.md) / [中文](docs/firecracker-deployment-guide.md)
- [Release And Distribution](docs/release-distribution.en.md) / [中文](docs/release-distribution.md)
