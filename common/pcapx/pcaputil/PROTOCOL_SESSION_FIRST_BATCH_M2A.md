# M2A — performance, Redis and Kafka (2026-09-22)

Baseline `0cdf63cc48763e32a0fb0337bfa386b4b64f7556`. This is the user-selected
first M2 increment. QUIC/H3/DoQ, MySQL prepared, PostgreSQL extended/COPY and
Kafka flexible versions remain outside this increment; the entire M2 is not
marked complete. No protocol inventory scores were inflated.

## Changes

- Internal TCP replay carries packet references by value and only materializes
  owned CaptureInfo ancillary data for observable packets/UDP/fallback. Public
  CaptureReader records and callback metadata remain owned. HTTP requests avoid
  unnecessary per-message response-method configuration maps.
- Redis has one bounded RESP2/3 typed value codec including attribute maps,
  push, null, verbatim, blob errors, maps/sets and signed scalars. Map items remain
  ordered key/value pairs so non-string/duplicate keys are not silently lost.
  Commands establish only observed command-shape role evidence, not identity.
  Pipeline replies correlate to PDU IDs; RESP3 pushes and observed RESP2 PubSub
  messages do not consume ordinary requests. Subscription acknowledgments,
  unsubscribe-all, HELLO protocol observation, MULTI/EXEC/DISCARD and AUTH/HELLO
  AUTH semantic redaction are covered. Raw PCAP/Raw bytes remain explicit evidence.
  Authentication display stays redacted after correlation expiry; a multi-channel
  subscription error consumes one complete request, preserving the following reply.
- Kafka pending context stores API, version, PDU and time. acks=0 creates no
  pending response. Duplicate outstanding IDs and timed-out ID reuse fail with
  context diagnostics. Responses without requests remain envelopes with explicit
  missing-request-version context; no version is inferred from response bytes.
  Every version-specific response consumes its complete body. Topic/partition
  arrays preserve all entries; legacy scalar aliases remain for compatibility.
- Kafka record sets validate every batch/message, CRC32C (magic 2) or IEEE CRC
  (magic 0/1), record lengths/counts/varints, nullable keys/values and headers.
  none/gzip codecs have aggregate inflated-byte, record and nesting budgets.
  Unsupported versions/codecs are explicit unsupported errors, not accepted
  trailing bytes. Nullable Produce record sets preserve null vs empty.

The old v1 ApiVersions audit now supplies version 1 and asserts throttle_time;
its former versionless/v0 entry point must reject the v1 tail. The old zero-CRC
batch bytes remain a negative case; the positive generator now computes CRC.
No original captured packet bytes were rewritten to pass a new validator.

## Profiles and material

| API | Non-flexible versions | Native positive evidence in this increment |
|---|---|---|
| ApiVersions | 0..2 | All three versions |
| Metadata | 0..8 | All nine versions |
| Produce | 0..7; legacy message formats retained | 3..7, none/gzip and acks=0 |
| Fetch | 0..11; legacy low-version paths retained | 4..11 |

Source schema: Apache Kafka tag 3.9.1, message JSON definitions. Modern flexible
ApiVersions 3 / Metadata 9 / Produce 9 / Fetch 12 were delivered in the subsequent
T15 flexible sub-milestone, separately from these legacy ranges. Its native Java
codec/broker capture and offline tag/null/error oracles are pinned by
`testdata/protocol-sessions/kafka-flex/manifest.json`; reproduction is in
`scripts/protocol-tests/generate-kafka-flex/`. Request header v2 keeps the classic
nullable client ID and adds header tags; response header v1 adds tags except
ApiVersions v3, which deliberately retains header v0. Compact lengths, nested
tag sections, known-tag windows and aggregate record budgets are validated.
Unknown tags retain their bytes; unsupported adjacent versions remain explicit. Admin/transaction APIs and snappy/lz4/zstd
are not added here. Redis streamed strings/aggregates remain unsupported.

`testdata/protocol-sessions/first-batch-m2a/manifest.json` pins a 235-packet native
loopback PCAP (tcpdump reported zero kernel drops), actual socket reply oracles,
and a 63-message Kafka TShark 4.4.8 field export. Redis 7.2.5 and Kafka 3.9.1
ran on isolated ports. Reproduction instructions and an independent Python
client are in `scripts/protocol-tests/generate-m2a/`.

Native replay passes full/deferred with workers 1/2/4. It checks all 31 Kafka
response associations, actual record values/offsets/CRC, Redis pushes, and
on-demand fields. Crafted tests cover attributes between pipeline replies,
all small-value split points, false PubSub detection, unsubscribe-all, AUTH
redaction, multi-topic/partition and multi-batch results, CRC/trailing errors,
expiry, pending budgets and response version reordering.

Sessions expire request context after 30 seconds of capture time. Redis loses
positional correlation after expiry until a new connection rather than attaching
late responses to new requests. Kafka retains bounded timed-out ID tombstones.
Subscription state and pending requests are bounded by ParserBudget; all value
and collection limits are checked before allocating their declared sizes.
Kafka envelopes/expanded record data are limited to 1 MiB, 4096 total collection
entries/records/headers per corresponding decode scope, and four legacy nested
compression levels. These are logical parser budgets, not exact Go heap limits.

## Performance evidence

Five alternating baseline/head runs, Go 1.22.12 darwin/arm64, Apple M1 Max,
GOMAXPROCS=2, worker=1, 500ms per benchmark; 3080 MQTT/HTTP/TLS PDUs per batch.
Raw output: `performance/2026-09-22-m2a/`. SHA256 of complete mixed-message raw
bytes, fields, session metadata and source references is unchanged:
`95cc27fd9e01eed45b16489264c69c23afe8312761086beb4a4d7825711cdd6f`.

| Mode | Baseline median ms | New median ms | Allocation reduction |
|---|---:|---:|---:|
| Full | 11.531 | 11.440 | 106358 -> 97093 (-8.7%) |
| Full + history | 13.441 | 13.289 | 117648 -> 108384 (-7.9%) |
| Deferred + history | 8.888 | 8.679 | 65133 -> 55869 (-14.2%) |

Allocated bytes decrease 2.5–3.8%. Wall-time changes of 0.8–2.4% are small and
within likely machine noise; this is primarily a repeatable allocation reduction.
It does not erase M1's added semantic/evidence cost or claim faster Internet-wide
capture throughput. Redis/Kafka now perform additional correctness work; this
mixed benchmark does not measure their new end-to-end throughput.

## Verification

Final integration: 48,858 test/subtest passes, zero failures and 21 existing
conditional skips. Vet passed; complete consumer/stream-parser race and final
Redis/Kafka state regression race passed. The 45-second fuzz run executed
207,719 cases without failure.

Commands: full `go test ./common/bin-parser/...`, full pcaputil/pcap-inspect/shark,
full race on those consumer packages plus stream_parser, vet, and a 45-second
`FuzzFirstBatchM2A` run. Final results and exact-head Essential Tests/Gate and
full Diff-Code-Check are recorded in the PR delivery note after actual completion.
The scannode-only alternative CI job may be scope-skipped; existing opt-in live
or large capture tests are not falsely counted as native-fixture acceptance.
