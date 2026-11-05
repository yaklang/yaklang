# Traffic test fixtures

Tests load captures, expected fields and rejection cases through
`internal/trafficfixture`. `index.json` records logical paths, aliases, byte
lengths and SHA256 hashes. Physical archives are stored in
`internal/trafficfixture/testdata/captures.zip`; the adjacent manifest records
their original repository paths and hashes. The enterprise fixture ZIP remains
beside this file.

The fixture archives retain upstream source manifests, licenses, attribution,
independent expected answers and reproduction inputs. Synthetic captures and
keys are labelled as test inputs. Private captures, production credentials,
external large PCAPs and complete investigation exports belong outside Git.
The archive password `bin-parser` is a packaging convention, not encryption for
credentials.

Use `trafficfixture.ReadFile` for owned byte buffers or `Materialize` for APIs
that require a filename. Materialized files belong in a test temporary directory.
Indexed fixtures cannot be shadowed by expanded files in the source tree.

From the repository root:

```sh
go run ./internal/trafficfixture/cmd/corpus verify
go run ./internal/trafficfixture/cmd/corpus test
go test -json -count=1 -timeout=15m ./internal/trafficfixture/... ./common/bin-parser/... ./common/pcapx/... ./common/yak/cmd/yakcmds/shark-cli ./scripts/ci > traffic.jsonl
go run ./internal/trafficfixture/cmd/corpus exec -- python3 @scripts/protocol-tests/check_go_test_json_v2.py traffic.jsonl --inventory @scripts/protocol-tests/required-tests-traffic-unified-v191.json --tier full
```

The CI workflow selects the active required-test inventory. Historical archived
inputs remain available for their regression tests. Add independently sourced
captures and expectations without regenerating accepted answers from the parser
being tested. Preserve their source, license, scope and hash records.
