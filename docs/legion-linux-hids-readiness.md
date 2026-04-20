# Legion Linux HIDS Readiness

This document describes how to build, validate, and operate the current Legion HIDS capability in `yaklang-scannode-refactor`.

## Scope

- The current runnable Legion node entrypoint in this repository is `cmd/legion-smoke-node`.
- HIDS runtime logic is implemented behind the `hids` build tag.
- Runtime execution is only supported on Linux hosts.
- Desired spec validation is strict and should be checked before rollout.

## Capability Packaging Matrix

| Build mode | Host OS | Advertised capability keys | HIDS apply behavior |
| --- | --- | --- | --- |
| default build | any | `yak.execute`, `ssa.rule_sync.export` | `ErrHIDSCapabilityNotCompiled` |
| `-tags hids` | non-Linux | `yak.execute`, `hids`, `ssa.rule_sync.export` | `ErrHIDSCapabilityUnsupportedPlatform` |
| `-tags hids` | Linux | `yak.execute`, `hids`, `ssa.rule_sync.export` | HIDS runtime starts if at least one collector comes up |

Operational implication: a build that advertises `hids` must still be scheduled to Linux hosts only. Building with `-tags hids` on macOS or Windows advertises the capability key, but the runtime cannot start there.

## Build Commands

Use the root `Taskfile.yml` tasks added for Legion HIDS work:

```bash
task legion_smoke_node_build
task legion_smoke_node_build_hids
task legion_smoke_node_build_hids_linux_amd64
```

Equivalent raw commands:

```bash
go build -o ./legion-smoke-node ./cmd/legion-smoke-node
go build -tags hids -o ./legion-smoke-node-hids ./cmd/legion-smoke-node
GOOS=linux GOARCH=amd64 go build -tags hids -o ./legion-smoke-node-hids-linux-amd64 ./cmd/legion-smoke-node
```

Use `task legion_smoke_node_build_hids` for native debugging when your current host is already Linux. Use `task legion_smoke_node_build_hids_linux_amd64` when you need a deployable Linux artifact from any development host.

If a future production Legion node entrypoint is added, it must also be built with `-tags hids` before the binary is expected to advertise or run the HIDS capability.

## Desired Spec Contract

The current HIDS desired spec is phase-1 and intentionally narrow:

- `mode` must be `observe`
- at least one collector must be enabled
- `collectors.process.backend` must be `ebpf`
- `collectors.network.backend` must be `ebpf`
- `collectors.file.backend` must be `filewatch`
- `collectors.file.watch_paths` must contain one or more absolute paths
- `collectors.audit.backend` must be `auditd`

Temporary rules are compiled during apply-time by the HIDS rule engine backed by YakVM expression evaluation. Rollout should treat `temporary_rules[].condition` as an engine-validated expression, not as a free-form opaque blob or a classic YARA text payload.

## Minimal Example Desired Spec

```json
{
  "mode": "observe",
  "collectors": {
    "file": {
      "enabled": true,
      "backend": "filewatch",
      "watch_paths": [
        "/etc",
        "/usr/local/bin"
      ]
    }
  },
  "reporting": {
    "emit_capability_status": true,
    "emit_capability_alert": true
  }
}
```

Example with multiple collectors:

```json
{
  "mode": "observe",
  "collectors": {
    "process": {
      "enabled": true,
      "backend": "ebpf"
    },
    "network": {
      "enabled": true,
      "backend": "ebpf"
    },
    "file": {
      "enabled": true,
      "backend": "filewatch",
      "watch_paths": [
        "/etc",
        "/usr/bin"
      ]
    },
    "audit": {
      "enabled": true,
      "backend": "auditd"
    }
  }
}
```

## Preflight Validation

Validate a desired spec before sending it to a node:

```bash
task hids_spec_check -- ./specs/hids.json
```

Or without Task:

```bash
go run -tags hids ./common/hids/rule/cmd/hids-desired-spec-check --input ./specs/hids.json
```

The checker validates JSON structure, desired spec constraints, and HIDS rule-engine compilation.

## Targeted Validation Commands

Run the focused Legion HIDS test set:

```bash
task test_hids
```

This task intentionally runs the HIDS-focused `scannode` subset together with the HIDS runtime and rule-engine packages. Use a broader `go test ./scannode` sweep separately when you want general node regression coverage.

## Linux Host Readiness Checklist

Before assigning the `hids` capability to a node, confirm the host is ready:

- the node binary was built with `-tags hids`
- the host OS is Linux
- the file collector watch paths are absolute and exist on the host
- the process and network collectors have the privileges needed by the eBPF backend
- the audit collector has the privileges needed to read audit events
- the node has permission to emit and persist capability state under its local base directory

The current runtime is resilient to partial collector startup failure:

- if at least one collector starts, the capability is kept alive and reported as `degraded`
- if a collector fails with errors such as `operation not permitted`, that collector is surfaced in runtime detail as degraded
- if no collector starts successfully, the entire HIDS apply fails

Treat `degraded` as a real operational state that needs investigation, not as a silent success.

## Session Restore Behavior

The node persists the normalized desired spec locally after a successful apply. On restart:

- persisted HIDS desired spec is restored
- the runtime is restarted from the persisted state
- inventory observations are replayed when the session becomes ready

This means rollout verification should include both first apply and restart / reconnect behavior.

## Recommended Smoke Workflow

1. Build a Linux HIDS binary:

   ```bash
   task legion_smoke_node_build_hids_linux_amd64
   ```

   If you are already on the target Linux host and only need a local native build, `task legion_smoke_node_build_hids` is also valid.

2. Validate the desired spec:

   ```bash
   task hids_spec_check -- ./specs/hids.json
   ```

3. Run targeted HIDS tests:

   ```bash
   task test_hids
   ```

4. Start the smoke node on a Linux host:

   ```bash
   ./legion-smoke-node-hids \
     -api-url http://127.0.0.1:8080 \
     -enrollment-token <token> \
     -id smoke-node-hids
   ```

5. Apply the `hids` capability from the platform and verify:

- the node advertises `hids`
- capability status becomes `running` or `degraded`
- degraded collector details are visible when host permissions are incomplete
- status survives node restart and reconnect
