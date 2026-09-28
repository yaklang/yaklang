# Linux user lookup portability

## Contract

- Objective: prevent the pre-main libc private-symbol abort in Linux Yaklang
  engine and Node artifacts when HOME is absent and NSS selects compat.
- Owner: Yaklang, branch `fix/yaklang/linux-nss-portability`, based on
  `0d3e4baac18deab03ae54c723edf2c56d69f0a17`.
- Scope: Linux CI build tags, matching manifests, artifact startup regression,
  HIDS identity fallback tests, and build documentation.
- Out of scope: DNS resolver policy, replacing glibc, customer deployment,
  installer diagnostics, formal tags, and release-lock updates.
- Environment: task-isolated remote WSL build; disposable no-network Buster
  containers. Existing deployments, services, and shared current links remain
  untouched. Exact committed source and artifact digests bind evidence.
- Publication: local verified commit; Yaklang push/PR requires user approval.
- Stop boundary: no release publication; do not claim target Kylin/HIDS kernel,
  registration, or full product acceptance from startup checks.

## Acceptance matrix

| Item | Evidence | Status |
|---|---|---|
| All Linux CI artifact producers use osusergo, retaining their other tags | workflow/build audit and metadata | pending |
| Manifest build tags match producers | component contract tests | pending |
| Original official Node fails compat/no-HOME | pinned binary/container logs from issue investigation | reproduced |
| Fixed full Node reaches main in all three cases | artifact regression | pending |
| Fixed full engine executes a script in all three cases | artifact regression | pending |
| Local identities resolve; unknown identities preserve numeric IDs | focused HIDS tests with hids,osusergo | pending |
| Source/artifact provenance and container cleanup | source SHA, hashes, resource check | pending |

## Implementation decisions

Use osusergo for Linux distributed binaries and Linux CI helper engines.
Keep CGO enabled for native dependencies. Keep non-Linux behavior and existing
linkage unchanged. Do not infer that every dynamically linked artifact has the
same failure; consistent Linux user lookup is the selected artifact policy.

The amd64 glibc 2.28 regression checks real package initialization and engine
execution, not just --help. Node intentionally exits at missing Runtime Host
identity after its main-entry marker; this does not prove enrollment.
