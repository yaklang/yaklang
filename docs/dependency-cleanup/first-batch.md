# First-batch dependency cleanup

This change removes seven modules from both `go.mod` and `go.sum`: `go.geojson`,
`go-opml`, `fatih/set.v0`, `tevino/abool`, libaudit v0, `govalidator`, and `RF.go`.
No unrelated dependency versions are upgraded. libaudit v2 remains in use.

## Retirement and compatibility

- `yaklib.GeoJsonExports` had no registration, generator, or caller reference in
  the repository; its commented registration and retired wrapper are removed.
- `common/feedly` had only its own test as a consumer. It and its two exclusive
  RSS OPML assets are removed. These exported Go APIs are intentionally retired;
  repository searches cannot establish whether external users imported them.
- Substring extraction still skips identical pairs, deduplicates each pair, and
  intersects all pair results. Fewer than two inputs/all-identical inputs return
  nil; a processed pair with no common substring returns a non-nil empty slice.
  Result order remains unspecified. A temporary differential test compared the
  replacement with the original implementation on 2,008 input groups, including
  Unicode, duplicates, empty inputs, and multiple intersections.
- `common/utils/atomicbool` copies abool's used implementation and tests from
  `9b9efcf221b5`, retaining its MIT license. The existing `utils.AtomicBool` is a
  type alias. Constructors, zero value, Set/UnSet/SetTo/SetToIf are unchanged.
  Node, MQ, task, and ticker fields remain pointers; no atomic state is copied.
  Consumers of the new package import only `sync/atomic`, not `common/utils`.
- The HIDS facade uses libaudit v2 clients, auparse, and reassembly, keeping its
  public login/command APIs. Its receive loop captures each run's stop channel
  and performs maintenance in the same goroutine so EOF and Stop cannot strand
  a maintenance goroutine or let an old run overwrite a restarted run's state.
  Replay tests cover complete/partial commands, login, lost records, EOF, and
  cancellation on an idle receiver. They do not require root or modify audit
  rules. Real privileged kernel audit delivery is not exercised by these tests.

## Lexical validation

`common/utils/textvalidate` copies only four predicates, their regexes, and their
upstream tests from govalidator `f21760c49a8d`; its MIT license is retained.
Before replacing imports, all four predicates passed differential comparison on
50,021 fixed-seed/edge-case inputs. The upstream dependency is not kept for tests.
The copied tests and additional frozen behavior table remain in the repository.

| Input / boundary | Preserved behavior |
| --- | --- |
| Empty string | Integer and printable ASCII: true; float and Base64: false |
| `01` | Integer: false; float: true |
| `+0`, `-0` | Integer and float: true |
| `.`, `e3`, `.e3` | Float: true (lexical legacy behavior) |
| `-.1`, `+.1`, `Inf`, `NaN`, surrounding whitespace | Float: false |
| `1e9999` | Float: true; no machine-range check |
| `Zg==`, `Zh==` | Base64: true; no canonical padding-bit check |
| `Zg`, URL alphabet, trailing newline | Base64: false |
| Unicode, control characters, DEL | Printable ASCII: false |

These are recognition functions, not replacements for successful numeric or
Base64 decoding. OpenAPI inference, nuclei DSL, pcapx, fuzzx, mutate, and codec
continue to use the same predicates.

## RPA random forest

`common/rpa/randomforest/internal/rf` is the classification core from RF.go
`46700521f302`. The upstream snapshot contains no license file. Source provenance
is retained in the copied files. Regression, examples, unused default training,
unused Gini calculation, and upstream filesystem wrappers are excluded.

The existing 80-tree gzip model and its JSON schema are unchanged. Training,
per-tree normalized votes, numeric `<=` and categorical equality decisions, and
unspecified tie order are preserved. A differential check compared every tree's
leaf counts over 1,007 inputs. `rf_golden_test.go` keeps a SHA-256 of those upstream
leaf results. Separate tests cover training, votes, boundaries, and model I/O.
Model I/O now returns filesystem/JSON errors through the facade's existing error
return and truncates overwritten model files instead of hiding failures.

## Configuration dependency boundary

`common/utils/appconfig` defines its own `FieldDescriptor` and imports only the
standard library. The utils forwarding API returns these descriptors and has no
protobuf import. gRPC conversion exists only in
`common/yakgrpc/grpc_third_party_config_template.go`.

Go callers that explicitly depended on `[]*ypb.ThirdPartyAppConfigItemTemplate`
from `utils.ParseAppTagToOptions` must adapt at their service boundary. Tag
ordering, defaults, overrides, errors, and supported field types are retained.
The small-package closure is guarded by `TestStandardLibraryDependencyBoundary`.
Other packages such as lowhttp and bizhelper still reference ypb; this does not
claim that every utils consumer has lost all gRPC dependencies.

## Reproduce focused validation

```sh
go test -race ./common/utils/atomicbool ./common/utils/appconfig ./common/utils/textvalidate ./common/rpa/randomforest/...
go test ./common/utils -run 'Test(GetSameSubStringsCompatibility|ImportAppConfig|ExportAppConfigToMap)'
go test ./common/yakgrpc -run '^TestAppConfigDescriptorAdapter$'
# Linux, no root required:
go test -race ./common/hids -run '^TestAuditV2'
go list -deps -f '{{if not .Standard}}{{.ImportPath}}{{end}}' ./common/utils/appconfig
go list -m all
go list -deps ./common/yak
GOOS=linux go list -deps ./common/hids
```

The new core packages and audit replay tests run in Essential Tests. Both the
module graph and the Yak/Linux HIDS build closures were checked for the seven
retired modules, separately from checking direct source imports.
