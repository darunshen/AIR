# Release And Distribution

[中文](release-distribution.md)

## 1. Current Artifacts

AIR currently produces release archives, Firecracker bundles, `.deb` packages, and apt-repo-style directory outputs.

## 2. Version Information

Release artifacts should carry version, commit, and build date metadata.

## 3. Local Builds

The repository includes scripts and workflows to build release assets locally before publishing.

## 4. GitHub Release

GitHub Actions can assemble and upload release artifacts so users can fetch official binaries directly.

## 5. `.deb` Installation

The Debian package installs the `air` binary and supports versioned upgrades on compatible systems.

## 6. apt Repository Layout

The project can also generate apt repository directory artifacts for later hosted distribution.

## 7. Recommended Next Steps

Continue hardening install UX, official image downloads, release notes, and packaging verification.
