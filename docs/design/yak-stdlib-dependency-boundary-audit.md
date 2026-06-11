# Yak Standard Library Dependency Boundary Audit

Last updated: 2026-06-10T14:32:46+08:00

This file records dependency-boundary problems found while shrinking `ssa2llvm`
native artifacts. The target is not to maintain a separate shadow runtime, but
to make Yak standard-library Go packages independently importable so dynamic
stdlib generation only links the domains used by the current Yak script.

## Current Baseline

- SSA sample script: `common/coreplugin/base-yak-plugin/SSA 项目探测.yak`
- Fresh SSA pruned artifact: `build/coreplugin-ssa-project-detect-fresh`, 204,839,648 bytes, about 195.4 MiB
- Full stdlib SSA artifact from the earlier comparison: 248,225,152 bytes, about 236.7 MiB
- Reduction from current pruned build is only about 43.4 MB, which is too small for the amount of pcap/netstack/lowtun/grpc code that should disappear.
- Current generated runtime source: `/tmp/yakssa-compile-3265d30804e95dcdb9815606d29f4be3/ssa2llvm-stdlib-src`
- Current dependency count for `runtime_go` with `ssa2llvm_pruned_runtime`: 1022 non-standard packages
- Confirmed unrelated domains in the SSA sample: `google.golang.org/grpc`, `github.com/yaklang/pcap`, `common/cybertunnel`, `common/pcapx`, `common/netstackvm`, `common/lowtun/netstack`, `common/ai/*`, `common/syntaxflow/*`

Additional compile experiments on 2026-06-10:

| Script | Plugin type | Generated import file | Non-standard deps | Artifact size | Notes |
| --- | --- | --- | ---: | ---: | --- |
| `build/minimal-print-audit.yak` (`println("yak")`) | `yak` | empty `init()` | 1019 | 194.7 MiB | Dependency set is identical to POC and port-scan samples. |
| `common/yak/ssa2llvm/tests/script/poc_request.yak` | `yak` | empty `init()` | 1019 | 194.7 MiB | Link command still contains `-lpcap`. |
| `build/portscan-min-audit.yak` | `port-scan` | imports only `cli.CliExports` | 1019 | 194.7 MiB | Dependency set is identical to the minimal script. |

The minimal script proves the current lower bound is already polluted before any
Yak stdlib module is requested by the source program.

Local audit sample contents:

```yak
// build/minimal-print-audit.yak
println("yak")
```

```yak
// build/portscan-min-audit.yak
handle = func(result) {
    println(result.Target)
    println(result.Port)
    println(result.Fingerprint.ServiceName)
}
```

Representative shortest paths from `go list -deps`:

```text
runtime_go -> common/yakgrpc/ypb -> google.golang.org/grpc
runtime_go -> common/yak/yaklib -> common/cybertunnel -> github.com/yaklang/pcap
runtime_go -> common/yak/yaklib -> common/utils/pingutil -> common/netstackvm -> common/lowtun/netstack
runtime_go -> common/yak/yaklib -> common/utils/pingutil -> common/pcapx
runtime_go -> common/yak/ssaapi -> common/syntaxflow/sfvm
runtime_go -> common/yak/ssaapi -> common/yakgrpc/yakit -> common/ai/aispec
```

For the minimal, POC, and port-scan samples the dependency set is the same. The
shortest paths from `runtime_go` include:

```text
runtime_go -> common/yak/yaklib -> common/cybertunnel -> github.com/yaklang/pcap
runtime_go -> common/yakgrpc/ypb -> google.golang.org/grpc
runtime_go -> common/yak/yaklib -> common/utils/pingutil -> common/pcapx
runtime_go -> common/yak/yaklib -> common/utils/pingutil -> common/netstackvm -> common/lowtun/netstack
runtime_go -> common/yak/yaklib -> common/yak/ssaapi
runtime_go -> common/yak/yaklib -> common/syntaxflow/sfdb -> common/syntaxflow/sfvm
runtime_go -> common/yak/yaklib -> common/yakgrpc/yakit -> common/ai/aispec
runtime_go -> common/utils/lowhttp/poc
runtime_go -> common/utils/cli
```

`go mod why` in the generated port-scan stdlib shows:

```text
github.com/yaklang/pcap      <- common/cybertunnel
google.golang.org/grpc       <- common/cybertunnel
github.com/jinzhu/gorm       <- common/ai/aid/aicommon
github.com/mattn/go-sqlite3  <- common/ai/aid/aicommon/aiskillloader
```

Static reverse-import counts from non-test Go files:

```text
common/yakgrpc/ypb:       446 direct importers
common/yakgrpc/yakit:     261 direct importers
common/yak/yaklib:         54 direct importers
common/yak/ssaapi:         62 direct importers
common/syntaxflow:          2 direct importers
common/ai:                 25 direct importers
common/cybertunnel:         7 direct importers
github.com/yaklang/pcap:   18 direct importers
common/pcapx:              14 direct importers
common/netstackvm:          4 direct importers
common/lowtun:              8 direct importers
```

## SSA Generated Stdlib Root Analysis

The generated import file for `SSA 项目探测.yak` is structurally reasonable, but
several imported Go packages are themselves large aggregators. Comparing the SSA
sample against the minimal `println("yak")` sample shows only three additional
non-standard packages:

```text
common/yak/yaklang
common/yak/yaklang/lib/builtin
common/yak/yaklang/spec/types
```

That means almost all of the current binary size is already in the fixed
`runtime_go` baseline. The SSA generated imports add little size because the
baseline has already imported `yaklib`, `ypb`, `poc`, and `cli`.

Direct roots in the SSA generated/runtime package:

| Root | Non-standard deps | Heavy domains reached |
| --- | ---: | --- |
| `common/yak/yaklib` | 1012 | gRPC, pcap, cybertunnel, pcapx, netstack, lowtun, SSA, SyntaxFlow, AI, yakit, ypb, schema, consts, gorm, sqlite3, lowhttp/poc |
| `common/yak/yaklang/lib/builtin` | 1015 | same as `yaklib`, through `common/yak/yaklang -> yaklib` |
| `common/yak/ssaapi` | 703 | gRPC, pcap, cybertunnel, SyntaxFlow, AI, yakit, ypb, schema, consts, gorm, sqlite3, lowhttp/poc |
| `common/utils/lowhttp/poc` | 382 | gRPC, ypb, schema, consts, gorm, sqlite3 |
| `common/utils/lowhttp` | 344 | gRPC, ypb, schema, consts, gorm, sqlite3 |
| `common/utils/cli` | 306 | gRPC, ypb, schema, consts, gorm, sqlite3 |
| `common/yak/ssaproject` | 306 | gRPC, ypb, schema, consts, gorm, sqlite3 |
| `common/utils/filesys` | 292 | gRPC, ypb, gorm, sqlite3, via `common/utils` |
| `common/utils/orderedmap` | 289 | gRPC, ypb, gorm, sqlite3, via `common/utils`; imported directly by `runtime_go` |
| `common/yak/ssaapi/ssaconfig` | 289 | gRPC, ypb, gorm, sqlite3, via `ypb` and `common/utils` |
| `common/yakgrpc/ypb` | 104 | gRPC |
| `common/yak/yaklib/codec` | 59 | currently acceptable as a relatively small leaf |

Important shortest paths:

```text
runtime_go -> common/yak/yaklang/lib/builtin -> common/yak/yaklang -> common/yak/yaklib -> common/cybertunnel -> github.com/yaklang/pcap
runtime_go -> common/yak/yaklang/lib/builtin -> common/utils -> common/yakgrpc/ypb -> google.golang.org/grpc
runtime_go -> common/yak/ssaapi -> common/syntaxflow/sfvm -> github.com/antlr/antlr4/runtime/Go/antlr/v4
runtime_go -> common/yak/ssaapi -> common/yak/java/java2ssa -> common/sca
runtime_go -> common/yak/ssaapi -> common/utils/yakgit -> github.com/go-git/go-git/v5
runtime_go -> common/yak/ssaapi -> common/utils/yakgit -> github.com/go-git/go-git/v5 -> github.com/ProtonMail/go-crypto/openpgp
runtime_go -> common/utils/lowhttp -> github.com/andybalholm/brotli
runtime_go -> common/utils/lowhttp -> github.com/refraction-networking/utls -> github.com/cloudflare/circl/pki
runtime_go -> common/yak/yaklib -> github.com/dop251/goja
runtime_go -> common/yak/yaklib -> common/fp
runtime_go -> common/yak/yaklib -> common/fuzzx
```

`go mod why -m` in the generated stdlib is useful, but it can be misleading
when used alone because it explains module-level reachability from the copied
module, not necessarily the exact target package path from `runtime_go`. Prefer
the `go list -deps` import graph and shortest-path check for runtime size
questions.

## Hard Boundaries

- SSA / SyntaxFlow / AI code should not enter network-only scripts such as port-scan, MITM, packet send, or POC request unless the script explicitly imports those Yak modules.
- `grpc` should not enter ordinary compiled Yak scripts through protobuf service stubs. Message DTOs and service clients/servers must be separated.
- `pcap`, netstack, lowtun, synscan, and tunnel packages should only enter scripts that explicitly use packet capture, raw packet, port-scan, MITM, or tunnel modules.
- `sqlite` / `gorm` are lower priority. They are currently accepted as temporary infrastructure dependencies, but should still be tracked when they are pulled by unrelated packages.

## Practical Size Strategy

The current dependency graph is deeply entangled. For example, SSA legitimately
needs code-source acquisition, which can require HTTP download and git/SVN
filesystem support. Today those capabilities are reached through `lowhttp`,
`poc`, `utils`, `consts`, `schema`, and `yakit`, which also bring unrelated
pcap, gRPC, DB, AI, and packet-network code. A full cleanup is therefore not a
small patch; it is a package-boundary project.

The original design goal was reasonable: let ssa2llvm reuse the existing
Yaklang implementation so behavior, bugs, fixes, and debugging stay aligned
with the interpreter. The problem is that the current reuse unit is too coarse.
ssa2llvm is reusing aggregate packages (`yaklib`, `ssaapi`, `utils`, `schema`,
`consts`, `yaklang/lib/builtin`) instead of small capability packages. That
turns semantic reuse into dependency reuse, so one required SSA capability can
pull the whole Yak runtime surface into the native binary.

The target should therefore be "shared implementation at leaf boundaries", not
"separate pruned runtime" and not "import the full interpreter packages". A
small shared downloader, git source adapter, SSA config package, SyntaxFlow VM
package, or yakit stdout adapter is still shared Yaklang code and remains easy
to debug. What must be avoided is using a package that also registers DB models,
gRPC service DTOs, packet capture, AI, and unrelated Yak modules.

Short-term size reduction should focus on the fixed baseline first:

- Stop compiling adapters that the script did not use, especially `runtime_poc.go`, `runtime_yaklib_yakit_pruned.go`, and generated `builtin.YaklangBaseLib`.
- Replace `runtime_go` imports of heavy utility packages with tiny local or leaf-package equivalents, especially `orderedmap -> common/utils`.
- Keep dynamic import generation, but make it select smaller export tables where they already exist or can be introduced cheaply.
- Use build tags or generated feature files only as a bridge toward shared small packages, not as a permanent shadow standard library.

Medium-term size reduction should split capability adapters, not remove needed
capabilities:

- SSA can keep HTTP/git acquisition, but it should depend on a light downloader/git adapter instead of `poc` and broad `utils/yakgit`.
- SyntaxFlow can keep DB-backed rule storage and AI completion, but VM/core execution should not import those adapters.
- `schema` can keep DB models and protobuf conversion, but model structs, gorm persistence, and `ypb` conversion should be separate packages.

This means a useful first target is not "make SSA have no HTTP/git". It is
"make SSA config/project scripts avoid packet capture, gRPC service stubs, AI,
SyntaxFlow VM, and full yaklib unless those features are explicitly used".

## Problems And Fix Direction

### P0: `runtime_go` pruned baseline compiles heavy stdlib adapters before script dependency selection

Evidence:

- `build/minimal-print-audit.yak` contains only `println("yak")`, and its generated `runtime_imports_generated.go` has an empty `init()`.
- The minimal script, `poc_request.yak`, and the minimal port-scan wrapper all produce the same 1019 non-standard package dependency set.
- All three artifacts are about 194.7 MiB and still link `github.com/yaklang/pcap` through `-lpcap`.
- In the generated port-scan stdlib, `go list -f '{{.GoFiles}}'` with `ssa2llvm_pruned_runtime` includes:
  - `runtime_cli.go`
  - `runtime_poc.go`
  - `runtime_yaklib_yakit_pruned.go`
  - `runtime_imports_generated.go`
- In the same package, `go list -f '{{.Imports}}'` includes `common/utils/cli`, `common/utils/lowhttp`, `common/utils/lowhttp/poc`, `common/yak/yaklib`, and `common/yakgrpc/ypb`.
- `common/yak/ssa2llvm/runtime/runtime_go/runtime_dispatch.go` always references `abi.IDPocTimeout`, `abi.IDPocGet`, and `abi.IDPocGetHTTPPacketBody`, so `runtime_poc.go` is compiled even when the script never uses `poc`.
- `common/yak/ssa2llvm/runtime/runtime_go/runtime_yaklib_yakit_pruned.go` always imports `yaklib` and `ypb` to build a virtual yakit client, even for scripts that do not use `yakit`.

Problem locations:

- `common/yak/ssa2llvm/runtime/runtime_go/runtime_dispatch.go`
- `common/yak/ssa2llvm/runtime/runtime_go/runtime_poc.go`
- `common/yak/ssa2llvm/runtime/runtime_go/runtime_cli.go`
- `common/yak/ssa2llvm/runtime/runtime_go/runtime_yaklib_yakit_pruned.go`
- `common/yak/ssa2llvm/runtime/embed/pruned_runtime.go`

Suggested fix:

- Do not treat the current pruned runtime as the long-term architecture. Keep one Yak stdlib implementation, but split the Go packages so `runtime_go` can import only the domains used by the compiled script.
- Generate the runtime dispatch table from actual ABI/runtime dependencies. If a script does not use `poc`, the compiled `runtime_go` package must not compile `runtime_poc.go`.
- Move `poc`, `cli`, `yakit`, and other adapters behind generated feature files or independent adapter packages that are only imported by `runtime_imports_generated.go`.
- Split the virtual yakit stdout bridge so basic `yakit.Info/Warn/Error` can stay message-free. Import `ypb` only for structured yakit APIs such as `Code`, risk/report objects, or gRPC-backed flows that explicitly require protobuf DTOs.

Acceptance checks:

- A `println("yak")` script should have an empty generated import file, no `common/yak/yaklib`, no `common/utils/lowhttp`, no `common/yakgrpc/ypb`, no `google.golang.org/grpc`, and no `github.com/yaklang/pcap`.
- A port-scan wrapper that only reads CLI fields and calls `handle(result)` should not include `poc`, `lowhttp`, `yaklib`, `ssaapi`, `syntaxflow`, `ai`, `pcap`, or `grpc`.

### P0: `common/yakgrpc/ypb` mixes message DTOs with gRPC service stubs

Evidence:

- `runtime_go` imports `common/yakgrpc/ypb` for yakit output handling.
- `common/yakgrpc/ypb/yakgrpc_grpc.pb.go` is in the same package and imports `google.golang.org/grpc`.
- Shortest path: `runtime_go -> common/yakgrpc/ypb -> google.golang.org/grpc`.
- Many non-network packages import `ypb` only for message structs, but still pay the gRPC dependency.

Problem locations:

- `common/yakgrpc/ypb/yakgrpc.pb.go`
- `common/yakgrpc/ypb/yakgrpc_grpc.pb.go`
- `common/yak/ssa2llvm/runtime/runtime_go/runtime_yaklib_lookup_full.go`
- `common/yak/ssa2llvm/runtime/runtime_go/runtime_yaklib_yakit_pruned.go`
- Broad importers: `common/schema`, `common/consts`, `common/yak/ssaapi/ssaconfig`, `common/yak/ssaproject`, `common/ai/aispec`, `common/yak/yaklib`

Suggested fix:

- Split generated protobuf output into message-only and service-stub packages, or generate service stubs under a different package path such as `common/yakgrpc/ypbgrpc`.
- Keep message-only `ypb` free of `google.golang.org/grpc`.
- Update service registration/client code to import the service package explicitly.
- Replace ssa2llvm runtime yakit output bridge with a local lightweight stdout bridge where possible, so compiled artifacts do not need `ypb` just to print `yakit.Code/Info/Error`.

Acceptance checks:

- `go list -deps ./common/yak/ssa2llvm/runtime/runtime_go` for a simple yakit-output script must not include `google.golang.org/grpc`.
- `go list -deps` for packages that only use protobuf messages must not include `google.golang.org/grpc`.

### P0: `common/yak/yaklib` is a monolithic Yak module aggregate

Evidence:

- Generated ssa2llvm import file imports `common/yak/yaklib` for `GlobalExport`, `JsonExports`, `FileExport`, `StringsExport`, `TimeExports`, `ZipExports`, and `YakitExports`.
- Shortest path to pcap: `runtime_go -> common/yak/yaklib -> common/cybertunnel -> github.com/yaklang/pcap`.
- Shortest path to netstack: `runtime_go -> common/yak/yaklib -> common/utils/pingutil -> common/netstackvm`.
- `common/yak/yaklib` contains unrelated Yak domains in one Go package: string/json/file/time/zip, yakit, POC, MITM, tunnel, synscan, brute, DB, risk, online, syntaxflow, AI-adjacent helpers.
- `go list -f '{{.Imports}}' ./common/yak/yaklib` includes `common/cybertunnel`, `common/utils/pingutil`, `common/utils/lowhttp/poc`, `common/yak/ssaapi`, `common/syntaxflow/sfdb`, `common/ai/ytoken`, `common/yakgrpc/yakit`, `common/yakgrpc/ypb`, `github.com/jinzhu/gorm`, and many network/HTTP/fuzz helpers.
- File-level import edges inside `yaklib`:
  - `yakit.go` imports `ssaapi`, `sfreport`, `yakit`, and `ypb`.
  - `dnslog.go` imports `cybertunnel`, `cybertunnel/tpb`, and `yakit`.
  - `syntaxflow_rule.go` imports `syntaxflow/sfdb`, `ssaconfig`, and `ypb`.
  - `str_aistream.go` imports `common/ai/ytoken`.
  - `traceroute.go`, `tools/ping.go`, `tools/synscan.go`, `tools/synscanx.go`, and `tools/fingerprint_scan.go` import `common/utils/pingutil`.
  - `tools/nuclei.go`, `tools/brute.go`, and `tools/pocinvoker.go` import `common/yakgrpc/yakit`.

Problem locations:

- `common/yak/yaklib/*.go`
- `common/yak/yaklib/tools/*.go`
- `common/yak/script_engine.go`
- `common/yak/ssa2llvm/runtime/embed/pruned_runtime.go`
- `common/yak/ssa2llvm/runtime/embed/script_engine_libs.go`

Suggested fix:

- Split Yak standard-library exports into domain packages, keeping the Yak module names stable:
  - lightweight core: `str`, `json`, `file`, `time`, `codec`, `cli`, basic globals
  - network HTTP/POC/fuzz: `poc`, `http`, `fuzz`, `csrf`
  - packet / scan: `synscan`, `finscan`, `servicescan`, `pcapx`, `netstack`, `mitm`, `tcpmitm`
  - SSA / SyntaxFlow: `ssa`, `syntaxflow`, `sfreport`
  - AI: `ai`, `aiagent`, `aim`, `liteforge`, `aireducer`
  - yakit / DB / reporting: `yakit`, `risk`, `report`, DB-backed helpers
- Make `script_engine.go` register these independent export packages directly, not through `yaklib` as a catch-all package.
- Keep compatibility aliases in `yaklib` temporarily, but do not use them from dynamic ssa2llvm import generation.
- Split `yaklib` by file domain first. Good first candidates are `yakit.go`, `dnslog.go`, `syntaxflow_rule.go`, `str_aistream.go`, and `tools/{ping,synscan,synscanx,fingerprint_scan,traceroute}.go` because they introduce the heaviest unrelated domains.

Acceptance checks:

- A script using only `str/json/file/time/zip/cli` should not import `common/yak/yaklib`, `common/cybertunnel`, `github.com/yaklang/pcap`, `common/yak/ssaapi`, or `common/ai`.
- A script using `poc.Get` may import `lowhttp/poc`, but should still not import `ssaapi`, `syntaxflow`, `ai`, `pcap`, or `grpc`.

### P0: `common/yak/script_engine.go` is both a registry and a heavy import aggregate

Evidence:

- `common/yak/script_engine.go` directly imports AI packages such as `common/aiengine`, `common/aiforge`, `common/ai/aid/aitool`, `common/ai/rag`, `common/aireducer`, `common/ai`, and `common/ai/aispec`.
- The same file imports `common/yakgrpc/ypb`, `common/yak/yaklib`, and `common/yakgrpc/yakit`.
- It registers many Yak modules in one file through `yaklang.Import(...)`.
- `common/yak/ssa2llvm/runtime/embed/script_engine_libs.go` can parse this file to derive Yak module to Go export mappings, but parsing the source does not solve the package-level import coupling in the normal Yak engine.
- The long-term target is one Yak stdlib implementation shared by the interpreter and ssa2llvm, so `script_engine.go` cannot remain the only place where all module export packages are tied together.

Suggested fix:

- Split standard-library registration into small module registration packages, for example `yakstdlib/str`, `yakstdlib/poc`, `yakstdlib/ssa`, `yakstdlib/ai`, `yakstdlib/packet`, and `yakstdlib/yakit`.
- Keep a full-engine aggregator for the ordinary interpreter, but let ssa2llvm import the same small module packages directly instead of importing a separate pruned runtime implementation.
- Generate or maintain a registry metadata table from these small packages so dynamic ssa2llvm import generation can map `yak module -> Go package -> export table -> method`.
- Avoid top-level imports of AI, gRPC, pcap/netstack, and SSA packages in any package that represents the light core interpreter runtime.

Acceptance checks:

- Importing a light standard-library registration package should not include AI, SSA/SyntaxFlow, gRPC, pcap, netstack, lowtun, or cybertunnel.
- The full Yak engine can still compose all registration packages explicitly, but ssa2llvm should compose only the ones selected from the script's externlib/use set.

### P0: `builtin.YaklangBaseLib` imports the full Yak engine

Evidence:

- The SSA generated import file registers globals through `runtimeRegisterYaklibGlobals(builtin.YaklangBaseLib)`.
- `common/yak/yaklang/lib/builtin/builtin.go` imports `common/yak/yaklang`.
- `common/yak/yaklang/engine.go` imports `common/yak/antlr4yak` and `common/yak/yaklib`.
- `common/yak/yaklang/lib/builtin` has 1015 non-standard transitive dependencies in the generated stdlib.
- Shortest heavy paths include:
  - `runtime_go -> common/yak/yaklang/lib/builtin -> common/yak/yaklang -> common/yak/yaklib -> common/cybertunnel -> github.com/yaklang/pcap`
  - `runtime_go -> common/yak/yaklang/lib/builtin -> common/utils -> common/yakgrpc/ypb -> google.golang.org/grpc`
- This means registering basic globals such as `println`, `error`, and `retry` currently imports the interpreter engine and the full `yaklib` aggregate.

Problem locations:

- `common/yak/yaklang/lib/builtin/builtin.go`
- `common/yak/yaklang/lib/builtin/exports.go`
- `common/yak/yaklang/engine.go`
- `common/yak/ssa2llvm/runtime/embed/pruned_runtime.go`

Suggested fix:

- Split pure builtin functions and operator helpers into a small package that does not import `common/yak/yaklang` or `common/yak/yaklib`.
- Move interpreter-engine integration and `yaklangspec` wiring to a separate adapter package used only by the interpreter.
- In ssa2llvm generated imports, register only the globals used by the script. Prefer runtime-local implementations for `print/printf/println/len/cap/error` when they are already handled by runtime ABI.
- Keep `YaklangBaseLib` for the full engine aggregator if needed, but do not use it as the source for native runtime globals.

Acceptance checks:

- A script using `println`, `len`, or `error` should not include `common/yak/yaklang`, `common/yak/antlr4yak`, `common/yak/yaklib`, `common/cybertunnel`, `github.com/yaklang/pcap`, or `google.golang.org/grpc`.

### P0: `common/utils` is a large infrastructure aggregate

Evidence:

- `common/utils` has 287 non-standard transitive dependencies.
- `common/utils/utils.go` imports `github.com/jinzhu/gorm`, blank-imports `github.com/mattn/go-sqlite3`, and imports `common/yak/yaklib/codec`.
- `common/utils/app_config.go` imports `common/yakgrpc/ypb`.
- `common/utils` direct imports include `common/utils/lowhttp/httpctx`, `common/cybertunnel/ctxio`, `common/yakgrpc/ypb`, `gorm`, and `sqlite3`.
- Light-looking packages inherit this weight:
  - `common/utils/orderedmap` has 289 non-standard deps because `orderedmap/map.go` imports `common/utils`. `runtime_go` imports `orderedmap` directly for runtime objects, so this is fixed baseline pollution independent of `yaklib`.
  - `common/utils/filesys` has 292 non-standard deps because files like `filesys/base.go` import `common/utils` for helpers such as `utils.Error`.
  - `common/yak/ssaapi/ssaconfig` reaches gorm/sqlite3 through `common/utils`.
  - `common/yak/yaklang/lib/builtin` reaches `ypb`/gRPC through `common/utils`.

Problem locations:

- `common/utils/utils.go`
- `common/utils/app_config.go`
- `common/utils/orderedmap/map.go`
- `common/utils/filesys/base.go`
- Broad importers of `common/utils` from `common/yak/ssaapi`, `common/yak/ssaapi/ssaconfig`, `common/yak/yaklang/lib/builtin`, `common/schema`, and `common/consts`

Suggested fix:

- Split `common/utils` into small leaf packages:
  - `utilserrors` / `errorsx`: `Error`, `Errorf`, `Wrap`
  - `stringsx`: pure string helpers
  - `hashx`: pure hashes that do not require yaklib codec
  - `cachex`: TTL/singleflight helpers
  - `dbutil`: gorm/sqlite helpers
  - `appconfigpb`: ypb-based app tag parsing
  - `httpctx` / lowhttp helpers outside generic utils
- Stop importing `common/utils` from leaf packages when only one small helper is needed.
- Make `orderedmap` independent of `common/utils`; inline or move `InterfaceToString`, `InterfaceToMapInterface`, and nil checks into a small leaf package with no DB/protobuf imports.
- Move sqlite driver registration out of `common/utils` so importing a string or filesystem helper does not link sqlite.

Acceptance checks:

- `go list -deps ./common/utils/filesys` should not include `gorm`, `sqlite3`, `ypb`, `grpc`, `lowhttp`, or `yaklib`.
- `go list -deps ./common/utils/orderedmap` should not include `gorm`, `sqlite3`, `ypb`, `grpc`, `lowhttp`, or `yaklib`.
- `go list -deps ./common/yak/ssaapi/ssaconfig` should not include `gorm/sqlite3` through `common/utils`.

### P0: `common/consts` and `common/schema` mix config, DB models, protobuf DTO conversion, and SSA types

Evidence:

- `common/consts` has 305 non-standard transitive dependencies.
- `common/schema` has 292 non-standard transitive dependencies.
- `common/consts/database.go` imports `gorm`, `sqlite3`, MySQL driver, `common/utils`, and `yaklib/codec`.
- `common/consts/utils.go` imports `common/yakgrpc/ypb`.
- `common/consts/ssa.go` imports `common/schema`, `common/utils`, `gorm`, and MySQL/Postgres dialect blank imports.
- `common/schema/ssa_project.go` imports `common/yak/ssaapi/ssaconfig` and `common/yakgrpc/ypb`.
- `common/schema/syntaxflow_rule.go` imports `common/yak/ssaapi/ssaconfig`, `common/yakgrpc/ypb`, `common/yak/yaklib/codec`, `common/utils`, and `gorm`.
- Many schema model files import both `gorm` and `ypb`, so model-only imports also become gRPC/protobuf adapter imports.
- Short paths:
  - `common/utils/cli -> common/consts -> common/yakgrpc/ypb -> google.golang.org/grpc`
  - `common/yak/ssaproject -> common/schema -> common/yak/ssaapi/ssaconfig -> common/yakgrpc/ypb`

Problem locations:

- `common/consts/database.go`
- `common/consts/utils.go`
- `common/consts/ssa.go`
- `common/schema/ssa_project.go`
- `common/schema/syntaxflow_rule.go`
- `common/schema/*` files that combine `gorm.Model` and `ToGRPCModel`/`ypb` helpers

Suggested fix:

- Split database handles and driver registration out of `common/consts` into DB-specific packages.
- Split protobuf conversion methods out of `common/schema` into adapter packages, e.g. `schema/ypbadapter` or domain-specific `yakgrpc/model`.
- Keep schema model structs independent from `ssaconfig` where possible. Store raw values or small internal DTOs instead of importing SSA config packages from schema.
- Keep `consts` as constants/env/path-only code. It should not own gorm database initialization or protobuf DTO helpers.

Acceptance checks:

- Importing `common/schema` model structs should not include `google.golang.org/grpc`.
- Importing `common/consts` for path/env constants should not include `gorm`, SQL drivers, `schema`, `ypb`, or `yaklib`.

### P0: `ssa` Yak module generation imports too much

Evidence:

- `common/yak/irify_libs.go` registers `ssa` as `lo.Assign(ssaapi.Exports, ssaproject.Exports, ssaconfig.Exports)`.
- `SSA 项目探测.yak` uses `ssa.NewConfig`, `ssa.withCodeSource*`, `ssa.GetSSAProjectByNameAndURL`, which mainly live in `ssaconfig.Exports` and `ssaproject.Exports`.
- Generated runtime import currently includes `ssaapi.Exports`, which pulls SyntaxFlow, yakit, and AI-adjacent packages.
- `common/yak/ssaapi` has 703 non-standard transitive dependencies.
- `common/yak/ssaapi/ssa_exports.go` exports compile APIs, project DB APIs, static-analyze placeholders, language constants, and DB-backed `GetLatestProgramNameByProjectName` in one map.
- `ssa_exports.go` imports `common/consts`, `common/yakgrpc/yakit`, `common/utils`, `common/utils/filesys`, `ssaconfig`, and `ssaproject`.
- `common/yak/ssaapi/ssa_compile_info.go` imports `common/utils/lowhttp/poc` to download code-source archives. This makes core SSA project parsing depend on the POC HTTP stack.
- `common/yak/ssaapi` imports all language builders in one package: `c2ssa`, `go2ssa`, `java2ssa`, `php2ssa`, `python2ssa`, `typescript/ts2ssa`, and `yak2ssa`.
- Shortest paths to non-SSA domains:
  - `runtime_go -> common/yak/ssaapi -> common/yak/java/java2ssa -> common/sca`
  - `runtime_go -> common/yak/ssaapi -> common/utils/yakgit -> github.com/go-git/go-git/v5`
  - `runtime_go -> common/yak/ssaapi -> common/yakgrpc/yakit -> common/cybertunnel -> github.com/yaklang/pcap`
  - `runtime_go -> common/yak/ssaapi -> common/syntaxflow/sfvm -> github.com/antlr/antlr4/runtime/Go/antlr/v4`
- Shortest paths:
  - `runtime_go -> common/yak/ssaapi -> common/syntaxflow/sfvm`
  - `runtime_go -> common/yak/ssaapi -> common/yakgrpc/yakit -> common/ai/aispec`

Problem locations:

- `common/yak/irify_libs.go`
- `common/yak/ssaapi/ssa_exports.go`
- `common/yak/ssaapi/ssa_compile_info.go`
- `common/yak/ssaapi/sf_*.go`
- `common/yak/ssaapi/program.go`
- `common/yak/ssaapi/values_db.go`
- `common/yak/ssaapi/ssaconfig/export.go`
- `common/yak/ssaproject/export.go`
- `common/yak/ssa2llvm/runtime/embed/script_engine_libs.go`
- `common/yak/ssa2llvm/runtime/embed/pruned_runtime.go`

Suggested fix:

- Enhance dynamic import generation to select the exact export table(s) containing used methods, not the whole `lo.Assign(...)` expression.
- Split `ssaapi.Exports` into at least:
  - compile/config/project APIs
  - SyntaxFlow query/result APIs
  - DB/report/yakit APIs
  - language frontend builders or language-specific registration packages
  - remote code-source acquisition adapters such as HTTP/POC and git/SVN
- Move `GetLatestProgramNameByProjectName` out of the light `ssa` export table because it imports `yakit` and DB-specific behavior.
- Keep `ssaconfig` core separate from SyntaxFlow gRPC request adapters.
- Avoid importing every language frontend when a script only needs config/project metadata. Language builders should be selected by requested language or compile mode.
- Move archive download and git/SVN filesystem construction behind explicit code-source adapter packages.

Acceptance checks:

- `SSA 项目探测.yak` may import SSA compile/project config packages, but should not import `common/syntaxflow/*`, `common/ai/*`, `common/cybertunnel`, `github.com/yaklang/pcap`, or `google.golang.org/grpc`.
- A script that only creates an SSA config JSON should not import Java/Python/PHP/Go/TypeScript frontends, `go-git`, `common/sca`, or `lowhttp/poc`.

### P0: `ssaconfig` mixes core compile config with SyntaxFlow gRPC request adapters

Evidence:

- `common/yak/ssaapi/ssaconfig/rule.go` imports `common/yakgrpc/ypb`.
- `common/yak/ssaapi/ssaconfig/syntaxflow.go` imports `common/yakgrpc/ypb`.
- `common/yak/ssaapi/ssaconfig/scan_policy.go` imports `common/yakgrpc/ypb`.
- `ssa.NewConfig` and `ssa.withCodeSource*` do not inherently need gRPC service stubs, but importing the package brings `ypb`, which currently brings `grpc`.

Suggested fix:

- Keep `ssaconfig` core types/options free of `ypb`.
- Move SyntaxFlow gRPC request conversion helpers to a sibling adapter package, e.g. `ssaconfiggrpc` or `ssaconfig/adaptergrpc`.
- Replace core config fields that store `*ypb.SyntaxFlowRuleInput` / `*ypb.SyntaxFlowRuleFilter` with internal plain DTOs or interfaces if the core package needs to represent them.

Acceptance checks:

- `go list -deps ./common/yak/ssaapi/ssaconfig` must not include `google.golang.org/grpc`.
- A pure SSA compile-config script must not include `common/yakgrpc/ypb`.

### P1: `common/yak/ssaproject` mixes project lookup, DB models, schema, and protobuf DTOs

Evidence:

- `common/yak/ssaproject` has 306 non-standard transitive dependencies.
- `common/yak/ssaproject/ssaproject.go` imports `gorm`, `common/consts`, `common/schema`, `common/yak/ssaapi/ssaconfig`, and `common/yakgrpc/ypb`.
- The generated SSA runtime imports `ssaproject.Exports` because `SSA 项目探测.yak` uses `ssa.GetSSAProjectByNameAndURL`.
- Short paths:
  - `runtime_go -> common/yak/ssaproject -> common/yakgrpc/ypb -> google.golang.org/grpc`
  - `runtime_go -> common/yak/ssaproject -> common/schema -> common/yak/ssaapi/ssaconfig`
  - `runtime_go -> common/yak/ssaproject -> common/consts -> github.com/mattn/go-sqlite3`

Problem locations:

- `common/yak/ssaproject/ssaproject.go`
- `common/yak/ssaproject/export.go`
- `common/schema/ssa_project.go`
- `common/consts/ssa.go`

Suggested fix:

- Split project metadata DTOs from DB lookup functions.
- Keep Yak export methods that only need project identity/config independent from protobuf DTOs.
- Move `ypb` conversion for SSA project objects to a gRPC adapter package.
- Make DB-backed project lookup explicit, e.g. separate `ssaprojectdb.Exports` from light `ssaproject.Exports`.

Acceptance checks:

- A script that only serializes an SSA config should not import `ssaproject`.
- A script that only loads project metadata should not import `google.golang.org/grpc`; DB imports may remain only if the method explicitly queries the profile DB.

### P0: SyntaxFlow packages mix core VM, DB/rule storage, SSA API, and AI completion

Evidence:

- `common/syntaxflow` has 1033 non-standard transitive dependencies.
- `common/syntaxflow/sfvm` has 338 non-standard transitive dependencies.
- `common/syntaxflow/sfdb` has 343 non-standard transitive dependencies.
- `common/syntaxflow/sfanalysis` has 705 non-standard transitive dependencies.
- `common/syntaxflow/sfcompletion` has 1202 non-standard transitive dependencies.
- `common/syntaxflow/export.go` imports `gorm`, `consts`, `schema`, `sfdb`, `bizhelper`, and `ssaapi`, then exports both `ExecRule` and DB-backed `QuerySyntaxFlowRules`.
- `common/syntaxflow/merge_beautification.go` imports `common/ai/aid/aitool`, `sfvm`, and `utils`. This makes the top-level SyntaxFlow package depend on AI tool parameter types.
- `common/syntaxflow/sfcompletion/desc_completion.go` imports `aicommon`, `aitool`, `aispec`, `aiforge`, `common/syntaxflow`, blank-imports `common/yak`, and imports `ypb`.
- `common/yak/ssaapi` imports many `common/syntaxflow/sfvm` and `common/syntaxflow/sfdb` files, while `common/syntaxflow/export.go` imports `ssaapi`. This is a domain-level cycle even if Go package cycles are avoided.
- `common/syntaxflow/sfvm/result.go` imports `schema`, `ypb`, and `yaklib/codec`, so core VM result values already know about DB models, protobuf messages, and Yak codec exports.

Problem locations:

- `common/syntaxflow/export.go`
- `common/syntaxflow/merge_beautification.go`
- `common/syntaxflow/sfcompletion/desc_completion.go`
- `common/syntaxflow/sfcompletion/test_cases_completion.go`
- `common/syntaxflow/sfvm/result.go`
- `common/syntaxflow/sfdb/*.go`
- `common/yak/ssaapi/sf_*.go`

Suggested fix:

- Split SyntaxFlow into layers:
  - `sfcore` / `sfvm`: parser, VM, in-memory values, no DB/ypb/AI/yak imports
  - `sfstore` / `sfdb`: rule/result persistence, gorm/schema only
  - `sfssa`: adapters between SSA programs and SyntaxFlow execution
  - `sfai`: AI completion/beautification helpers
  - `sfyak`: Yak export table for `syntaxflow` module
  - `sfgrpc`: protobuf/gRPC DTO conversion
- Move `MergeBeautificationResults` and completion APIs out of the top-level SyntaxFlow export package if they need AI types.
- Keep `sfvm` result data independent from `ypb`; conversion to protobuf should live in an adapter.
- Avoid blank-importing `common/yak` from SyntaxFlow completion packages; use an explicit registration function or dependency injection for AI forge callbacks.

Acceptance checks:

- Importing `common/syntaxflow/sfvm` should not include `common/schema`, `common/yakgrpc/ypb`, `common/yak/yaklib`, `common/ai`, `gorm`, or `google.golang.org/grpc`.
- A script that does not use SyntaxFlow should not include `common/syntaxflow/*` through `ssaapi`.
- A script that only runs SyntaxFlow VM logic should not include AI completion or Yak interpreter packages.

### P0: `common/yakgrpc/yakit` crosses DB, AI, tunnel, HTTP, SSA, and UI concerns

Evidence:

- `ssaapi.Exports` imports `common/yakgrpc/yakit`.
- `yaklib.YakitExports` and multiple yaklib helpers import `common/yakgrpc/yakit`.
- `common/yakgrpc/yakit` imports schema/consts, and through other files participates in AI/HTTP/SSA/reporting paths.
- Shortest path to AI in SSA sample: `runtime_go -> common/yak/ssaapi -> common/yakgrpc/yakit -> common/ai/aispec`.

Suggested fix:

- Split `yakit` into smaller packages:
  - lightweight output/log API for script runtime
  - DB query/mutation helpers
  - SSA project/risk helpers
  - AI persistence/helpers
  - HTTPFlow/report helpers
- Make Yak `yakit` module registration compose only the exports actually requested by the script.
- Do not import DB-heavy or AI-heavy yakit helpers for `yakit.Code`, `Info`, `Warn`, `Error`, `StatusCard`.

Acceptance checks:

- A script using only `yakit.Code` should not import `common/ai`, `common/cybertunnel`, `github.com/yaklang/pcap`, `common/yak/ssaapi`, or `google.golang.org/grpc`.

### P1: `str` module mixes pure strings with HTTP/network parsing

Evidence:

- `common/yak/yaklib/strings.go` imports `common/utils/lowhttp`, `common/utils/network`, `domainextractor`, `filter`, `suspect`, and many broad `common/utils` helpers.
- `SSA 项目探测.yak` uses light functions such as `str.TrimSpace`, `str.Split`, `str.ReplaceAll`, `str.Index`, `str.PathJoin`, and `str.MatchAllOfSubString`.
- Importing the whole `yaklib.StringsExport` brings HTTP parsing and network helpers into scripts that only need basic strings.

Suggested fix:

- Split string exports into `strcore` and heavier extension tables:
  - core: wrappers around `strings`, `strconv`, `path/filepath`, lightweight match helpers
  - network/http: URL, HTTP packet parsing, host/port parsing
  - extraction/security: JSON/domain/title/suspect/filter helpers
- Register Yak module `str` by composing these tables in the ordinary engine.
- Dynamic import generation should pick only tables containing the used keys.

Acceptance checks:

- A script using only core string functions should not import `common/utils/lowhttp`, `common/netx`, `common/pcapx`, `common/yak/yaklib`, or `common/ai`.

### P1: `cli` module imports DB-backed plugin env by default

Evidence:

- `common/utils/cli/cli.go` imports `common/consts` and `common/schema` for `SetPluginEnv`.
- Basic CLI functions such as `String`, `Bool`, `Int`, `StringSlice`, `setDefault`, `setHelp`, `check` do not require DB access.
- `common/consts` and `common/schema` currently import `ypb`, which can pull gRPC until P0 is fixed.

Suggested fix:

- Split CLI exports into core parser exports and DB/plugin-env exports.
- Keep `cli.setPluginEnv` in a separate export table or package that can import DB/schema.
- Dynamic import generation should only include plugin-env code when the script uses `cli.setPluginEnv`.

Acceptance checks:

- A script using basic `cli.String/Bool/Int/check` should not import `common/consts`, `common/schema`, `common/yakgrpc/ypb`, or `google.golang.org/grpc`.

### P1: `poc` / `lowhttp` imports `consts` and `schema`

Evidence:

- `common/utils/lowhttp/poc/poc.go` imports `common/consts`, `common/schema`, `common/utils/cli`, `common/utils/lowhttp`, and `common/yak/yaklib/codec`.
- For network scripts, `lowhttp/poc` is expected; `ssa/syntaxflow/ai/grpc/pcap` are not.
- Because `consts/schema/cli` currently drag `ypb` and broad infra, `poc` can inherit unrelated domains.
- `common/utils/lowhttp/httpctx/base.go` imports `common/yakgrpc/ypb`.
- `common/utils/lowhttp/http2_serve.go` imports `common/yakgrpc/ypb`.
- After `runtime_go` stops importing `ypb` directly, `poc.Get` can still inherit `ypb`/gRPC through the `lowhttp` package unless these adapter files are split out.

Suggested fix:

- Keep HTTP execution and packet manipulation in `lowhttp/poc`.
- Move DB/profile/config integration out of the hot path used by `PoCExports`.
- Ensure `poc` option helpers and request execution do not import `ssaapi`, `syntaxflow`, `ai`, `cybertunnel`, `pcap`, or gRPC.
- Split `lowhttp` core packet/request utilities from HTTP context/protobuf/gRPC-facing adapters. `httpctx` should use a local DTO or an adapter package for `ypb` conversion.

Acceptance checks:

- A simple `poc.Get` script may include `lowhttp` and HTTP stack, but not `common/yak/ssaapi`, `common/syntaxflow`, `common/ai`, `common/cybertunnel`, `github.com/yaklang/pcap`, or `google.golang.org/grpc`.

### P1: `pingutil` combines ordinary reachability helpers with pcap/netstack backends

Evidence:

- Shortest paths from minimal/POC/port-scan samples include:
  - `runtime_go -> common/yak/yaklib -> common/utils/pingutil -> common/pcapx`
  - `runtime_go -> common/yak/yaklib -> common/utils/pingutil -> common/netstackvm -> common/lowtun/netstack`
- `common/yak/yaklib/traceroute.go` imports `common/utils/pingutil`.
- `common/yak/yaklib/tools/ping.go`, `tools/synscan.go`, `tools/synscanx.go`, and `tools/fingerprint_scan.go` import `common/utils/pingutil`.
- `common/utils/pingutil/ping.go` imports `common/netstackvm`.
- `common/utils/pingutil/pcapx_ping.go` imports `common/pcapx`.

Suggested fix:

- Split `pingutil` into a light package for basic TCP/ICMP/DNS reachability helpers and separate raw-packet packages for pcap/netstack-backed behavior.
- Move Yak exports for `ping`, `traceroute`, `synscan`, `synscanx`, and fingerprint scan into separate Go packages so importing a light stdlib module does not compile packet backends.
- Make pcap/netstack-backed helpers opt in through explicit Yak modules or explicit methods.

Acceptance checks:

- A script that does not call `ping`, `traceroute`, `synscan`, `synscanx`, packet send, MITM, or lowtun functions should not include `common/pcapx`, `common/netstackvm`, `common/lowtun`, or `github.com/yaklang/pcap`.
- A script that only calls a TCP-level reachability helper should not include pcap/netstack unless that helper explicitly chooses a raw backend.

### P1: Network scan / packet modules should not import SSA, SyntaxFlow, or AI

Evidence:

- `common/yak/ssa2llvm/tests/script/poc_request.yak` compiles to about 194.7 MiB and has 1019 non-standard dependencies.
- `build/portscan-min-audit.yak` compiles to about 194.7 MiB and has 1019 non-standard dependencies.
- The POC and port-scan dependency sets are identical to the minimal `println("yak")` script, so current evidence points to shared runtime/yaklib pollution before network-specific code is isolated.
- Both samples include `common/yak/ssaapi`, `common/syntaxflow/*`, `common/ai/*`, `common/cybertunnel`, `common/pcapx`, `common/netstackvm`, `common/lowtun/*`, `github.com/yaklang/pcap`, and `google.golang.org/grpc`.

Suggested fix:

- After P0/P1 splits, compile representative network scripts and assert absence of `common/yak/ssaapi`, `common/syntaxflow`, and `common/ai`.
- If still present, trace shortest paths and add new audit entries before implementation.
- Treat port-scan/MITM/packet-send as packet/network domains. They can import pcap/netstack only when the plugin type or script method actually requires raw packet behavior, but they should not import SSA/SyntaxFlow/AI by default.

Acceptance checks:

- Port-scan / MITM / packet-send compiled artifacts should not include SSA/SyntaxFlow/AI dependencies unless script uses those modules explicitly.

## Verification Commands

Use a worktree-local DB:

```sh
mkdir -p .db build
YAKIT_HOME="$PWD/.db" ./build/ssa2llvm compile "common/coreplugin/base-yak-plugin/SSA 项目探测.yak" -l yak -f main --plugin-type yak -o build/coreplugin-ssa-project-detect-fresh -a -x > build/coreplugin-ssa-project-detect-fresh.compile.log 2>&1
```

Compile the current baseline samples:

```sh
YAKIT_HOME="$PWD/.db" ./build/ssa2llvm compile build/minimal-print-audit.yak -l yak -f main --plugin-type yak -o build/minimal-print-audit -a -x > build/minimal-print-audit.compile.log 2>&1
YAKIT_HOME="$PWD/.db" ./build/ssa2llvm compile common/yak/ssa2llvm/tests/script/poc_request.yak -l yak -f main --plugin-type yak -o build/poc-request-audit -a -x > build/poc-request-audit.compile.log 2>&1
YAKIT_HOME="$PWD/.db" ./build/ssa2llvm compile build/portscan-min-audit.yak -l yak -f main --plugin-type port-scan -o build/portscan-min-audit -a -x > build/portscan-min-audit.compile.log 2>&1
```

Find generated stdlib:

```sh
work=$(rg -o "WORK=.*" build/coreplugin-ssa-project-detect-fresh.compile.log | tail -n1 | sed "s/WORK=//")
cd "$work/ssa2llvm-stdlib-src"
GOFLAGS="-tags=ssa2llvm_pruned_runtime" go list -deps -f '{{if not .Standard}}{{.ImportPath}}{{end}}' ./common/yak/ssa2llvm/runtime/runtime_go | sed '/^$/d' | sort
```

Hard forbidden checks for the SSA sample:

```sh
GOFLAGS="-tags=ssa2llvm_pruned_runtime" go list -deps -f '{{if not .Standard}}{{.ImportPath}}{{end}}' ./common/yak/ssa2llvm/runtime/runtime_go |
  rg 'google.golang.org/grpc|github.com/yaklang/pcap|common/(cybertunnel|pcapx|netstack|lowtun|syntaxflow|ai/)'
```

Shortest-path helper:

```sh
GOFLAGS="-tags=ssa2llvm_pruned_runtime" go list -deps -f '{{.ImportPath}}{{printf "\t"}}{{join .Imports " "}}' ./common/yak/ssa2llvm/runtime/runtime_go > /tmp/yak-runtime-imports.tsv
```

Then run a small BFS from `github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/runtime_go` to target packages.

Compare dependency sets between baseline samples:

```sh
comm -3 /tmp/yak-minimal-print-deps.txt /tmp/yak-poc-request-deps.txt | wc -l
comm -3 /tmp/yak-minimal-print-deps.txt /tmp/yak-portscan-min-deps.txt | wc -l
```

## Next Audit Work

- Record a first implementation order that removes the fixed baseline pollution:
  - split `ypb` message DTOs from gRPC stubs
  - remove unconditional `yaklib`/`ypb` imports from `runtime_go`
  - replace `builtin.YaklangBaseLib` usage with small runtime-local/global builtin registration
  - split `common/utils` into small leaf helper packages and move DB/ypb helpers out
  - split `common/consts` and `common/schema` into constants, DB models, protobuf adapters, and SSA-specific model packages
  - split `yaklib` domain files into independent stdlib packages
  - split `ssaconfig`/`ssaapi` core from SyntaxFlow and yakit/DB adapters
  - split `ssaproject` DB/protobuf adapters from light project metadata APIs
  - split SyntaxFlow core VM, DB store, SSA adapter, AI completion, Yak exports, and gRPC adapters
  - split `lowhttp` core from `ypb` adapters
  - split `pingutil` light reachability from pcap/netstack backends
- Add reverse-import detail for remaining direct importers after the first split, especially `schema`, `consts`, `common/utils`, `sfweb`, `mcp`, and `yakgrpc` handlers.
- Build package-level acceptance tests for small leaves: `utils/filesys`, `ssaconfig`, `sfvm`, `schema` model-only, and `builtin` globals.
- Compile representative MITM and packet-send scripts after the baseline fixed cost is reduced.
- For each new finding, add shortest path, problem location, suggested fix, and acceptance check before changing implementation.

## Implemented ssa2llvm Runtime Gating

Updated: 2026-06-11T16:34:22+08:00

This pass only changes `common/yak/ssa2llvm/**`; it does not split shared Yak packages yet.

Changes:

- `runtime_poc.go` and the POC dispatch registration are compiled only for full runtime or pruned runtime builds with `ssa2llvm_runtime_poc`.
- `runtime_yaklib_yakit_pruned.go` is compiled only with `ssa2llvm_runtime_yakit`.
- `runtime_cli.go` is compiled only for full runtime or pruned runtime builds with `ssa2llvm_runtime_cli`.
- `yak_lib.go` no longer imports `common/utils/orderedmap`; object literals use a small runtime-local ordered map.
- pruned runtime build tags are derived from actual compiler dependencies:
  - `cli` yaklib module -> `ssa2llvm_runtime_cli`
  - POC direct dispatch IDs -> `ssa2llvm_runtime_poc`
  - `yakit` yaklib module -> `ssa2llvm_runtime_yakit`
- final clang linking now passes `-s`, and the ssa2llvm cache version was bumped so stale unstripped/cache-heavy binaries are not reused.

Measured final artifacts:

- Minimal `println("yak")`: `build/minimal-print-audit-final`, 2.7 MiB, pruned deps 7, `libyak.a` 3.2 MiB.
- POC request: `build/poc-request-final`, 34.2 MiB, pruned deps 389, `libyak.a` 53 MiB.
- `SSA 项目探测.yak`: `build/coreplugin-ssa-project-detect-final`, 160.5 MiB, pruned deps 1022, `libyak.a` 218 MiB.

Runtime smoke checks:

- `build/minimal-print-audit-final` prints `yak`.
- `build/poc-request-final` successfully fetches a local HTTP file through `poc.Get`.
- `build/coreplugin-ssa-project-detect-final --target ~/Target/DVWA` prints the expected config JSON and `[yakit][info] 项目探测完成。`.

Interpretation:

- The fixed baseline problem in `runtime_go` is mostly fixed for scripts that do not use Yak stdlib modules: minimal native output dropped from about 194.7 MiB to 2.7 MiB.
- POC output is now isolated from yaklib/yakit/cli baseline, but it still pulls `common/utils`, `common/utils/cli`, `common/utils/orderedmap`, `common/yakgrpc/ypb`, and `google.golang.org/grpc` through the real `common/utils/lowhttp/poc` dependency graph.
- SSA detect remains large because its generated import file still includes `cli`, `poc`, `ssa`, `yakit`, `yaklib`, `builtin`, `filesys`, `file`, `json`, `str`, `time`, and `zip`. This is now a real package-boundary problem in those Yak stdlib packages rather than an unconditional `runtime_go` baseline problem.
