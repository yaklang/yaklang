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
| All Linux CI artifact producers use osusergo, retaining their other tags | workflow/build audit and metadata | passed |
| Manifest build tags match producers | component contract tests | passed |
| Original official Node fails compat/no-HOME | pinned binary/container logs from issue investigation | reproduced |
| Fixed full Node reaches main in all three cases | artifact regression | passed on amd64 |
| Fixed full engine executes a script in all three cases | artifact regression | passed on amd64 |
| Local identities resolve; unknown identities preserve numeric IDs | focused HIDS tests with hids,osusergo | passed |
| Source/artifact provenance and container cleanup | source SHA, hashes, resource check | passed |

## Implementation decisions

Use osusergo for Linux distributed binaries and Linux CI helper engines.
Keep CGO enabled for native dependencies. Keep non-Linux behavior and existing
linkage unchanged. Do not infer that every dynamically linked artifact has the
same failure; consistent Linux user lookup is the selected artifact policy.

The amd64 glibc 2.28 regression checks real package initialization and engine
execution, not just --help. Node intentionally exits at missing Runtime Host
identity after its main-entry marker; this does not prove enrollment.

## Verified outcome

- Built source: `30ffecd345ac1c32b594774d86437c061e4f0b5e`, Go 1.22.12.
- Regression harness: `9fe51ad8d6b5048122dfbb1777989b08123bd0e1`.
  Subsequent changes only strengthen artifact metadata checks and record evidence;
  they do not change compiled product inputs.
- Product Node: official packaging script, static CGO, `hids,osusergo`; all three
  Buster cases reach main and exit at the expected missing release identity gate.
- Full engine: existing manylinux2014 script and CI compressed-resource stage,
  `gzip_embed,osusergo`; all three cases execute the script and exit 0. ELF symbol
  baseline check passes at GLIBC_2.14 (within the required GLIBC_2.17 ceiling).
- Node SHA256: `0b2c1bc72018251eaad618d7b856c4053dcadbab9407b25585ccccd11c5ee771`.
- Engine SHA256: `f6bd1cd961feb3b41d690f17f69059334dea57b3e74e1a1325ea4674ca0d2a8e`.
- Four component contract scripts and both new HIDS identity tests pass. Changed
  scripts pass ShellCheck and Bash syntax; changed YAML and 156 applicable
  workflow shell payloads parse successfully.
- Engine resource preparation regenerates tracked compressed archives and
  go.work.sum; their digests are retained with the runtime evidence. No extra
  business-source changes were applied remotely.
- Achieved: T1 and bounded amd64 startup/script-execution T2. No network scanning,
  collection, real enrollment, or customer deployment was executed. All test and
  build containers were removed. Source, build caches, logs and outputs are retained.
- Remaining: GitHub CI, ARM64 artifact runtime, other complete producer builds,
  target Kylin/enterprise-directory semantics, and full product acceptance. No
  branch push, PR, tag, release, or immutable asset replacement has occurred.
