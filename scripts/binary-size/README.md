# Go executable composition and slim dependency gate

This tool inventories the **whole executable**, including stripped release
builds. It supports Go ELF (Linux) and Mach-O (macOS); it does not yet decode PE.
It uses only the Go standard library and introduces no engine runtime dependency.

## Build and measure

Prepare compressed resources using the `Generate tar.gz resources` step in
`.github/workflows/reuse-build.yml` before using `gzip_embed`. The workflow's
`build_type: yakslim` selects `gzip_embed,irify_exclude`; `yakslim` is not the
release build tag (the separate legacy tag also changes Oracle support).

Run from the repository root, using the same toolchain, platform, source and
resource inputs for every comparison:

```sh
mkdir -p /tmp/yak-size
# Full engine
go build -trimpath -tags gzip_embed -ldflags '-s -w' \
  -o /tmp/yak-size/yak-full common/yak/cmd/yak.go
go list -deps -json -tags gzip_embed ./common/yak/cmd/yak.go \
  > /tmp/yak-size/full-deps.json
go run ./scripts/binary-size -binary /tmp/yak-size/yak-full \
  -deps /tmp/yak-size/full-deps.json > /tmp/yak-size/full.json

# Slim engine
go build -trimpath -tags gzip_embed,irify_exclude -ldflags '-s -w' \
  -o /tmp/yak-size/yak-slim common/yak/cmd/yak.go
go list -deps -json -tags gzip_embed,irify_exclude ./common/yak/cmd/yak.go \
  > /tmp/yak-size/slim-deps.json
go run ./scripts/binary-size -binary /tmp/yak-size/yak-slim \
  -deps /tmp/yak-size/slim-deps.json -check-slim > /tmp/yak-size/slim.json
```

To check dependency boundaries without building the engine (also used in CI):

```sh
go list -deps -json -tags irify_exclude ./common/yak/cmd/yak.go > /tmp/yak-slim-deps.json
go run ./scripts/binary-size -deps /tmp/yak-slim-deps.json -check-slim
```

The check fails if the graph contains the excluded C, C#, Go, Java, PHP,
Python or TypeScript/JS SSA compiler frontends, or the full compiler/scan
orchestrators. Yak's VM parser, Yak SSA used for completion/static checking,
and the C preprocessor used by the formatting RPC remain allowed. This is not
an assertion that all SSA/SyntaxFlow runtime code has been removed.

## Reading the JSON

- `FileBytes` is the actual file size. `SectionBytes + OtherFileBytes` equals it.
  The latter includes file headers, alignment and format-specific link metadata.
  Zero-fill/BSS is excluded: it consumes virtual memory, not bytes in the file.
- `Packages`, `Subsystems` and `Modules` are **alternative views of the same Go
  code spans**, obtained from function entry/end PCs. They include instruction
  alignment and exclude shared data and metadata. They must not be added together
  or described as complete package size. `OtherCodeBytes` is the rest of the text
  section, including native C/cgo code and padding.
- `PCLNSubtables` partitions the runtime pclntab for known Go 1.18/1.20+ layouts.
  This is runtime information, not removable DWARF. It contains names, PC/line
  data and function records, including inlining/runtime bookkeeping. Unknown
  layouts retain their section-level size without a speculative breakdown.
- `EmbeddedResources` is a build-input inventory from `go list`, not a separate
  section of the executable. Linker elimination/deduplication can affect retained
  bytes. Go string literals, generated tables and protobuf descriptors are not
  `go:embed` inputs. Do not add this inventory to section sizes.
- `-check-slim` checks the supplied dependency graph. It cannot prove the graph
  was produced from the binary's exact source; keep their build inputs together.
  It complements startup/API tests rather than replacing them.

Avoid adding `go tool nm -size` aggregate aliases such as `runtime.pclntab`,
`runtime.funcnametab` and `runtime.functab`: their address ranges overlap. A
symbol-enabled diagnostic build can locate individual static tables, but its
symbol table also makes that build larger than the stripped release artifact.

The measured full/slim comparison and remaining reduction opportunities are in
[the 2026-09-20 report](../../docs/binary-size-slim-2026-09-20.md).
