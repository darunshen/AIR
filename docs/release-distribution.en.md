# Release And Distribution

[中文](release-distribution.md)

## 1. Current Artifacts

AIR currently produces release archives, Firecracker bundles, `.deb` packages, and apt-repo-style directory outputs.

Release artifacts now also include Linux OpenClaude host runtime bundles:

- `air_openclaude_linux_amd64.tar.gz`

## 2. Version Information

Release artifacts should carry version, commit, and build date metadata.

## 3. Local Builds

The repository includes scripts and workflows to build release assets locally before publishing.

## 4. GitHub Release

GitHub Actions can assemble and upload release artifacts so users can fetch official binaries directly.

## 5. `.deb` Installation

The Debian package installs the `air` binary and supports versioned upgrades on compatible systems.

After installation, the recommended first command is:

```bash
air chat
```

`air chat` performs first-run setup interactively:

- choose `local` or `firecracker`
- offer to download the official AIR Firecracker bundle when assets are missing
- on `linux/amd64`, prefer downloading the official AIR OpenClaude bundle when the host runtime is missing; on other architectures, or when that download path fails, fall back to source download plus `bun install`
- prompt for model settings such as `OPENAI_BASE_URL`, `OPENAI_MODEL`, and `OPENAI_API_KEY`
- persist the first saved model settings to `~/.config/air/chat.json`
- enter AI chat directly once the runtime is ready

If you later need to switch models, rotate keys, or force fresh input, run:

```bash
air chat --reconfigure
```

## 6. apt Repository Layout

The project can also generate apt repository directory artifacts for later hosted distribution.

## 7. Recommended Next Steps

Continue hardening install UX, official image downloads, release notes, and packaging verification.
