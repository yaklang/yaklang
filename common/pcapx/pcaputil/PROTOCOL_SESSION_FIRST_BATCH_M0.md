# PR #5013 first batch — M0 correction, 2026-09-21

This increment applies the attached first-batch audit to baseline
`9c50c0359754079108e81ccaeb348e57f98373e1`. It repairs the reproduced correctness
and evidence problems; it does **not** complete the 18 core tasks or M1/M2.
Attachment SHA256: `7426b479dc3684bfe3c5e8d852ab8704b8d8c241d8a457fa0233e9affc775727`.

## Delivered behavior

| Audit | Correction | Evidence |
|---|---|---|
| F01 | Wire QUIC requires successful packet authentication; missing keys and invalid tags are distinct. Failed authentication cannot expose frames or commit keys, PN, CIDs or application state. Initial/Handshake cannot carry application STREAM frames. PN and CRYPTO ranges are separated by direction. | `TestAudit5013QUICUntrustedWireMustNotAutoBecomePlaintext`, `TestFirstBatchT12`, `TestFirstBatchT12DirectionalState` |
| F02 | Check caplen/origlen/snaplen/block payload before pcapgo allocation, plus options, endian, trailer, interface and metadata budgets. Replay and shark use shared bounded pcap/pcapng constructors. | `TestFirstBatchT01`, `TestFirstBatchT01Shark`, attachment regression |
| F03 | Preserve old capture bytes; explicitly label synthetic TCP wrappers. Add native UDP QUIC v1 Initial ingress with coalesced packet splitting, bounded endpoint-pair state and owned events. | `TestFirstBatchT00`, `TestFirstBatchT03`, `TestFirstBatchT03BadTagAndBudget`, attachment UDP regression |
| F04 | Metadata v4/v8 flags and Fetch v5 int64 LogStartOffset; also v7 sessions/forgotten topics, v9 leader epoch, v11 rack. Nullable Metadata topic semantics are version-aware. Strict trailing-data validation remains. | Attachment Metadata v4 / Fetch v5 regressions; `TestFirstBatchT15` covers Metadata v0–8 and Fetch v0–11 requests |
| F05 | Partial RESP signed integers and big integers accept legal `+`/`-` prefixes without admitting signed bulk lengths or malformed CR. | Attachment Redis regression; every cut point in `TestFirstBatchT14` |
| F06 | DNS names use the 255-octet expanded-name bound, separate compression-pointer cycle/hop checks, and 63-octet label bounds. Valid 16/63/127-label names parse. | Attachment DNS regression; `TestFirstBatchT07` |

Continuous fuzz additionally found a truncated QUIC packet whose advertised
payload length exceeded available input. The consume boundary now rejects it
before slicing. The minimized 19-byte seed is committed under
`testdata/fuzz/FuzzFirstBatchM0/8abe1862026f5684`.

The original audit reproductions failed in seven top-level tests on the baseline;
two controls already passed. Original regression assertions have been preserved.
New first-batch test IDs identify this increment's sub-profile, not completion of
the full task bearing that ID.

## Evidence and API boundary

`testdata/protocol-sessions/first-batch-capabilities.json` records all 22 planned
tasks (18 core + 4 optional), the 39 M1 fixture rows, their actual transports,
hashes and evidence level, and the new UDP sample/oracle. The audit counted all
pcap files; the old M1 manifest specifically contained 39 rows, not 43. No catalog
or roadmap score was increased.

`NewProtocolSession` remains the wire API. Code which deliberately supplies
unprotected long-header QUIC frames must now use
`NewDecryptedQUICSession(budget, source)`. Source is required; emitted events say
`Input Representation=decrypted-stream`, include `Plaintext Source`, and set
`Authentication Verified=false`. This API is a trust declaration by its caller;
it does not verify that the caller actually decrypted anything. Ordinary replay
never selects it. Existing synthetic QUIC/HTTP3/QPACK/DoQ algorithm tests use this
API and additionally verify ordinary replay cannot silently accept their bytes
as authenticated application traffic.

`quic-rfc9001-native-udp.pcap` contains the unchanged protected bytes from
[RFC 9001 A.2/A.3](https://www.rfc-editor.org/rfc/rfc9001.html#appendix-A), placed in
a deterministic synthetic UDP carrier. Two records contain three packets (client
PN 2 twice in one datagram, server PN 1). Test asserts direction, length/offset,
frames, authentication, duplicate handling, full/deferred semantic equivalence,
workers 1/2/4 and zero retained bytes after close. The pinned TShark 4.4.8 oracle
confirms packet numbers, CRYPTO/ACK and `example.com`. Raw/oracle hashes and the
exact command are in the capability JSON. This is independent standard-vector
verification, **not a live-server capture or native HTTP3/DoQ coverage**. Existing
upstream real captures and provenance remain intact.

QUIC UDP scope is v1 long headers and endpoint pairs, beginning with the client
Initial. Missing Handshake/Application keys remain opaque/context-required;
Initial authentication does not establish peer identity. CID migration,
server-first recovery, short-header UDP, multi-connection keylog ClientRandom
selection, Retry validation and complete stream reassembly remain outside this
increment. Authentication success is not a statement of full QUIC validity.

pcapng limits: block/packet/snaplen <=16 MiB, <=1024 sections/interfaces,
<=1 MiB retained section metadata, <=64 KiB options per block and <=4096 options.
NRB and DSB are explicitly rejected pending independently bounded retained
metadata support; this can reject captures that previously silently skipped those
blocks. EPB/PB/SPB, little/big endian, zero snaplen, options and trailers remain
within the documented bounded profile. Reader limits are separate from the
smaller configurable protocol/session message budget.

## Validation

Commands run from repository root, Go 1.22.12, macOS 14.1.2 arm64, Apple M1 Max:

```sh
go test -json ./common/pcapx/pcaputil ./common/pcapx/cmd/pcap-inspect \
  ./common/yak/cmd/yakcmds/shark-cli ./common/bin-parser/... -count=1 > acceptance.jsonl
go test -json ./common/pcapx/pcaputil -count=1 > pcaputil-final.jsonl
go test -race ./common/pcapx/pcaputil ./common/yak/cmd/yakcmds/shark-cli -count=1
go test ./common/pcapx/pcaputil -run '^$' -fuzz '^FuzzFirstBatchM0$' -fuzztime=45s -parallel=4
```

The combined package regression passed; subsequent QUIC/DNS boundary and test
refinements are covered by the final pcaputil run. Required tests are checked with
`scripts/protocol-tests/check_go_test_json.py` (`--require` repeated for
TestFirstBatchT00/T01/T03/T07/T12/T14/T15 and TestFirstBatchT01Shark). It rejects
missing/unpassed/required-skipped tests and any failure or malformed JSON. The
broad suite may contain pre-existing optional skips; these are not new evidence.
The sample regeneration test is deterministic and preserves all existing pcap
bytes. Continuous fuzz is separate from the seed-only ordinary test run. Final race passed for both packages; final 45-second fuzz completed 192,561 executions without failure. The required-test gate observed 48,782 tests in the broad run and 1,213 in the final pcaputil run, with no required skips.

### Local performance / resource cost

Same-machine five alternating A/B rounds against the baseline, GOMAXPROCS=4,
`-test.cpu=4 -test.benchtime=300ms -test.benchmem`; medians below. The identical
`first_batch_bench_test.go` was copied to the baseline worktree. These small
benchmarks measure names, capture replay and one request/response, not full
protocol throughput or live capture rate.

| Benchmark | Baseline ns/op | M0 ns/op | Baseline → M0 B/op | Allocs/op |
|---|---:|---:|---:|---:|
| DNS three-label name | 164.8 | 172.6 (+4.7%) | 144 → 144 | 7 → 7 |
| pcapng small replay | 8191 | 8571 (+4.6%) | 17067 → 17612 | 82 → 92 |
| HTTP ordinary deferred pair | 3867 | 3884 (+0.4%) | 12272 → 12272 | 45 → 45 |

The pcapng allocation increase buys validation before unsafe lengths reach the
underlying allocator. Invalid 32 MiB caplen inside an empty block is rejected
while the validation buffer capacity remains <=64 bytes; the test does not
attempt an OOM. A 2048-byte shared QUIC budget rejects retained state and closes
with zero ledger balance. No throughput improvement, RSS recovery or percentile
latency claim is made from these microbenchmarks. Full CPU/heap/RSS/export-layer
profiling and live-producer interoperability remain T06 follow-up work.

## Remaining work

M1: full transport registry/capture-domain foundation, shared event DTO and shark
session integration (only its capture reader is unified here). M2: real TLS and
QUIC keylog carrier, HTTP3/DoQ closure, DNS-family semantics, Redis push/transaction,
Kafka response/batch/version matrix, MySQL and PostgreSQL advanced state machines.
Metadata retention, L2/RTP native ingress and independent live-server corpora
remain explicitly incomplete in the capability matrix. Future work must preserve
these evidence boundaries instead of converting synthetic smoke tests into
“complete” protocol support.

Reference schemas: [Kafka MetadataRequest 3.9.0](https://github.com/apache/kafka/blob/3.9.0/clients/src/main/resources/common/message/MetadataRequest.json),
[FetchRequest 3.9.0](https://github.com/apache/kafka/blob/3.9.0/clients/src/main/resources/common/message/FetchRequest.json),
[QUIC permitted encryption levels](https://www.rfc-editor.org/rfc/rfc9000.html#section-12.4),
[DNS size limits](https://www.rfc-editor.org/rfc/rfc1035.html#section-2.3.4),
[RESP specification](https://redis.io/docs/latest/develop/reference/protocol-spec/).
