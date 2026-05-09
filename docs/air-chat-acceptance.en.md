# `air chat` Real-Machine Acceptance Record

[中文](air-chat-acceptance.md)

This document records one real-machine acceptance run for the path:

`.deb -> air chat -> auto-download -> enter chat`

It is not a long-term SLA statement. It is an engineering acceptance snapshot that includes the issues found and the fixes applied during the run. Always interpret it together with:

- the current commit
- the current GitHub Release artifacts
- the current external network / GitHub download state

## 1. Acceptance Goal

The goal was not to validate one subcommand in isolation. The goal was to validate whether an end user could install the `.deb` and then reach a usable AI chat entrypoint with one command:

1. install the `air` `.deb`
2. run `air chat`
3. automatically prompt for and download missing Firecracker assets
4. automatically prompt for and download the OpenClaude guest rootfs
5. automatically prepare the OpenClaude host runtime
6. complete model configuration
7. enter the `AIR OpenClaude Chat` prompt

## 2. Acceptance Environment

- Date: 2026-05-08
- Host architecture: `x86_64`
- `/dev/kvm`: readable and writable
- Package: `air_0.1.1-dev3_amd64.deb`
- Provider: `firecracker`
- Simulated user environment: fresh temporary `HOME`
- Simulated official distribution source: local HTTP mirror

Notes:

- The first direct GitHub Release attempt hit one `TLS handshake timeout` while downloading the Firecracker bundle
- To continue validating the first-run state machine and auto-download logic, later attempts used a local HTTP mirror that served the same-version release artifacts
- This run validated the product workflow, not GitHub network stability

## 3. Final Result

The run successfully reached:

```text
AIR OpenClaude Chat
provider=firecracker session=sess_748ef6b0f627bd88 workdir=/workspace
Connected to 127.0.0.1:<ephemeral-port>. Type /exit to quit.
air:openclaude@sess_748ef6b0f627bd88>
```

It also produced a transcript:

```text
/tmp/air-chat-home5/workspace/runtime/sessions/firecracker/sess_748ef6b0f627bd88/openclaude-chat-transcript.jsonl
```

In the current implementation, `air chat` allocates an ephemeral host loopback port for the local forward path by default, and prints stage timing such as `firecracker session + openclaude ready in 42.86s` so cold-start latency is directly visible.

## 4. Issues Found And Fixed During This Run

### 4.1 The `.deb` artifact was stale and did not include `air chat`

Symptoms:

- the earlier `dist/air_0.1.1_amd64.deb` still contained the old CLI
- running the packaged binary only showed the old usage and did not expose `air chat`

Fix:

- rebuild release artifacts from the current source
- confirm that the packaged `air version` matched the active source commit

### 4.2 OpenClaude host bundle extraction failed

Symptoms:

- the official OpenClaude host bundle initially failed during extraction
- the observed tar entry failures surfaced first as type `49`, then as type `50`

Root cause:

- the bundle contains hard links and symlinks
- `extractTarGz` initially only supported directories and regular files

Fix:

- add `tar.TypeLink` support to `extractTarGz`
- add `tar.TypeSymlink` support to `extractTarGz`
- add focused test coverage for both cases

### 4.3 The source-install fallback path was incomplete after a bundle failure

Symptoms:

- after the host bundle failed, `bun install` ran inside a half-extracted directory
- that produced errors such as missing `package.json`

Fix:

- clean the managed install directory before falling back to `git clone + bun install`

### 4.4 `air chat` kept using stale Firecracker config after interactive downloads

Symptoms:

- `air chat` successfully downloaded the Firecracker bundle
- `air doctor` reported the environment as ready
- but the later session creation still failed with:
  - `firecracker binary not found: firecracker`

Root cause:

- `main()` created `session.Manager` early in process startup
- `air chat` later completed interactive downloads, but the actual session run still used the old `vm.Config`

Fix:

- recreate `session.Manager` after `air chat` completes interactive setup

### 4.5 The official OpenClaude host bundle could extract, but was still rejected as an invalid repo

Symptoms:

- the bundle downloaded and unpacked successfully
- but the next validation still failed with:
  - `stat .../scripts/start-grpc.ts: no such file or directory`

Root cause:

- the official host bundle layout is:
  - `<install_dir>/openclaude/...`
  - `<install_dir>/bin/bun`
- the initial validation incorrectly treated `<install_dir>` as the source repo root

Fix:

- after installing the official host bundle, always resolve it through `resolveOpenClaudeRepo()`
- do not validate the bundle root as if it were a raw source checkout

## 5. Commits Directly Related To This Run

- `ef1ac0c` `Add OpenClaude guest bundle setup to air chat`
- `fc293d3` `Verify OpenClaude guest release artifacts`
- `c4f1365` `Fix air chat first-run bundle setup`
- `c6fef95` `Isolate runtime session state`

## 6. Current Conclusion

Based on this 2026-05-08 real-machine acceptance run, the following can now be stated:

- the `.deb` package now exposes `air chat` as a first-run entrypoint
- in `firecracker` mode, it can guide users through the required runtime asset downloads in order
- the automatic OpenClaude guest rootfs preparation path is working
- the automatic OpenClaude host runtime preparation path is working
- once the session starts, users can reach the real `AIR OpenClaude Chat` prompt

One more release-like revalidation is still recommended:

1. use the real GitHub Release instead of a local HTTP mirror
2. use a real working model key
3. continue past the prompt and confirm one real chat request returns successfully
