# Versioned traffic corpus

Capture-backed tests read password ZIP batches through
[`internal/trafficfixture`](../../../../internal/trafficfixture). Password:
`bin-parser`. This is a packaging convention, not protection for credentials.
No additional ZIP library, download or extraction step is needed for Go tests.

`index.json` pins each ZIP SHA256, every member's SHA256 and expanded size,
and historical path aliases. Indexed inputs always come from the ZIP; a loose
copy cannot shadow them. All reads return owned bytes. Filename-only APIs use
verified copies in test-owned temporary directories, removed by `t.Cleanup`.

The baseline seals 694 captures (690 unique content hashes) and their original
answers from `2fb090d177c3bf52277f240b2dca05ea80216485`. The source ZIP SHA256 is
`2ac6c0e1861b3d9487230fd8d89106780c2f856e67c2873fe99c270d0b92b234`.
The repository batch preserves every capture byte and original path.
The auxiliary batch preserves remaining binary inputs, manifests and endpoint
evidence. Provenance, fixed upstream commits, licenses and evidence categories
remain inside these archives. Artificial and upstream fixtures do not become
claims of real capture merely because they are archived.

The supporting-materials batch seals 86 further files from `2598e4d6aafbd369a46be8c71aff3cbc799cebdb`:
generators and their tests, capture/build recipes, validator scripts, its original
107-test inventory, endpoint configs, six public test certificates, five
generated loopback session keylogs and the minimized Go fuzz regression seed.
The quality runner resolves the tested checkout through `YAK_TRAFFIC_REPO_ROOT`;
its companion validator and inventory still come from the verified workspace.
Original generator bytes, versions, configs and file paths remain available.
Independent Go oracle code and Go regression tests remain reviewable source.

The sealed answer inventory has 622 partially mapped and 72 unmapped cases.
Its executable assertions include 434 selected-input parses, 102 selected-input
rejections and 30 exact whole-capture protocol/status count maps. A selected
frame's assertions are not a golden for every message in the capture.
`TestFrozenCorpusSelectedInputs` consumes the sealed field/error assertions;
`TestFrozenCorpusReplay` executes every capture, checks full/deferred fields,
errors, associations, stream offsets and resource release, and compares sealed
event counts where available. Replay parity alone is not independent semantics.
The existing endpoint, field, negative and worker/callback/chunk regression
tests keep their stronger assertions while reading the same archived inputs.

Run from the repository root:

```sh
go run ./internal/trafficfixture/cmd/corpus test
go test -json -count=1 -timeout=5m ./internal/trafficfixture/... ./common/bin-parser/... ./common/pcapx/... ./common/yak/cmd/yakcmds/shark-cli > traffic.jsonl
go run ./internal/trafficfixture/cmd/corpus exec -- python3 @scripts/protocol-tests/check_go_test_json.py traffic.jsonl --inventory @scripts/protocol-tests/required-tests-v3.json --tier full
```

The single `corpus` entry point uses the same bounded Go ZIP loader as tests.
`test` validates Python/shell/Node source, executes the validator and builder
unit tests, and compiles archived Go recipes without running capture generators,
Docker or downloads. It uses existing `go`, `python3`, `bash` and `node` binaries;
it never installs dependencies. Child failure remains a command failure.

`exec -- COMMAND` resolves `@repository/path` arguments to verified private
copies and removes the workspace afterwards; other arguments and the caller's
working directory retain their usual meanings. `exec --workspace -- COMMAND`
runs from that private workspace. `export NEW_DIRECTORY` creates a persistent
private workspace for the archived reproduction instructions and Go builders;
existing output directories and symlinks are rejected. Exports include original
path aliases, module files, the small current Go source dependencies and an
`EXPORT-MANIFEST.json` with every file's hash. Expanded exports are bounded to
384 MiB. No tool automatically starts capture or regenerates accepted answers.

```sh
go run ./internal/trafficfixture/cmd/corpus verify
go run ./internal/trafficfixture/cmd/corpus export /tmp/new-traffic-workspace
go run ./internal/trafficfixture/cmd/corpus exec -- python3 @scripts/protocol-tests/run-quality-v2.py quick --output /tmp/new-quality-result
```

Add future captures and answers as a **new** batch. Do not rewrite old ZIPs,
replace old aliases, regenerate answers from the implementation under test,
or turn an unmapped oracle into a passing semantic result. Preserve fixed
source/version, content hash, format, packet counts, license, evidence category,
independent expected fields/errors and their validation scope. Deduplicate by
content hash and preserve distinct path aliases. Keep new batches below 25 MiB
compressed and each member below 25 MiB expanded; the loader also caps expanded
batch size and rejects unsupported encryption, traversal and duplicate members.
New answer-bearing batches use `validation/expected.json` schema 1 and enter the
frozen gates automatically. Add adapters for new assertion kinds before claiming
their semantic coverage. The existing synthetic session keys are test inputs;
production credentials, user private keys and material without redistribution
permission belong in an external fixed-source download manifest.

Current test requirements are sealed at `scripts/protocol-tests/required-tests-v3.json`;
the historical inventories and all four earlier batches remain unchanged.
The supplemental small-corpus batch contains 26 minimal controls, their explicit
expected facts, an offline generator and the compact pinned source manifest.
Two Modbus cases are sanitized derivatives of a licensed upstream resegmentation;
its attribution and changes are in `validation/sources.json`. The coalesced case
has a recorded tshark 4.2.5 field oracle; that version does not recognize the
seven-byte initial prefix. Full/deferred, workers 1/2/4, observer parity and split
session feeds are asserted by `TestSmallCorpusSealedControls`; LOGINACK also has
byte-boundary, malformed-length and ownership tests. These are scoped control
assertions, not a semantic golden for the entire upstream collection. Process
logs and the original large downloads remain outside the repository.
