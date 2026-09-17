# Protocol session §29 report

Stacked into `wip/optimize-protocol-parse` (PR #5013) on 2026-09-17.
This report records first-version ProtocolSession coverage for the task-book
matrix. Catalog rows stay `partial`/`new`. Nothing here promotes a protocol
to `done`.

Host: Windows amd64, Go 1.22.12 (`GOTOOLCHAIN=local`), CGO with llvm-mingw
gcc/clang 22.1.8. `tshark` is not installed on this host.

## 1. Submissions, branch, and PRs

| Item | Value |
|---|---|
| Integration branch | `wip/optimize-protocol-parse` |
| Integration PR to `main` | [#5013](https://github.com/yaklang/yaklang/pull/5013) (OPEN; not merged to `main`) |
| Head at report time | `8182960ef` Merge pull request #5121 (DoQ), plus this report PR |
| Public contract | `ProtocolSession` Probe / Feed / Close / Stats in `protocol_session.go` wrapping isolated `binFlow` |
| Probe ports | 40000 / 40001 (standard ports are not protocol truth) |

Stacked PRs merged into #5013:

| PR | Scope |
|---|---|
| [#5097](https://github.com/yaklang/yaklang/pull/5097) | LDAP / PostgreSQL / WebSocket YAML field depth |
| [#5099](https://github.com/yaklang/yaklang/pull/5099) | M0 session contract + PostgreSQL, WebSocket, LDAP, Redis, gRPC |
| [#5100](https://github.com/yaklang/yaklang/pull/5100) | MQTT 5.0 |
| [#5101](https://github.com/yaklang/yaklang/pull/5101) | MongoDB OP_MSG + OP_COMPRESSED |
| [#5102](https://github.com/yaklang/yaklang/pull/5102) | Kafka ApiVersions / Metadata / Produce / Fetch |
| [#5104](https://github.com/yaklang/yaklang/pull/5104) | TDS PRELOGIN / LOGIN7 / SQLBatch / RPC |
| [#5105](https://github.com/yaklang/yaklang/pull/5105) | AMQP 0-9-1 |
| [#5107](https://github.com/yaklang/yaklang/pull/5107) | SMB2/3 |
| [#5108](https://github.com/yaklang/yaklang/pull/5108) | DCE/RPC |
| [#5109](https://github.com/yaklang/yaklang/pull/5109) | SSH RFC 4253 first handshake |
| [#5110](https://github.com/yaklang/yaklang/pull/5110) | NFSv3 |
| [#5111](https://github.com/yaklang/yaklang/pull/5111) | SNMPv3 |
| [#5112](https://github.com/yaklang/yaklang/pull/5112) | RDP TPKT/X.224 |
| [#5113](https://github.com/yaklang/yaklang/pull/5113) | DoT RFC 7858 |
| [#5114](https://github.com/yaklang/yaklang/pull/5114) | DoH RFC 8484 |
| [#5115](https://github.com/yaklang/yaklang/pull/5115) | SIP RFC 3261 |
| [#5116](https://github.com/yaklang/yaklang/pull/5116) | RTP/RTCP RFC 3550 |
| [#5117](https://github.com/yaklang/yaklang/pull/5117) | QUIC v1 transport |
| [#5118](https://github.com/yaklang/yaklang/pull/5118) | QUIC Initial decrypt / keyed 1-RTT |
| [#5119](https://github.com/yaklang/yaklang/pull/5119) | HTTP/3 |
| [#5120](https://github.com/yaklang/yaklang/pull/5120) | QPACK |
| [#5121](https://github.com/yaklang/yaklang/pull/5121) | DoQ RFC 9250 |

Probe order (first non-Reject wins): HTTP/2, MySQL, PostgreSQL, LDAP, SNMP,
Redis, WebSocket, MQTT, Mongo, NFS, Kafka, TDS, AMQP, SMB2, DCE/RPC, SSH,
RDP, DoT, SIP, RTP, QUIC. HTTP→WebSocket / HTTP→DoH / HTTP/2→gRPC /
QUIC→HTTP/3 / QUIC→DoQ are transitions, not `probeWire` protocols.

## 2. Twenty-protocol completion status

Status key:

- **session**: first-version profile is admitted on `ProtocolSession` with
  named-field, fail-closed, fragmentation, and replay tests.
- **catalog**: bin-parser roadmap/catalog row is unchanged (`partial` or `new`).
- **not done**: catalog is not `done`; P0 scorecard gates are not claimed passed.

| Protocol | Session | Catalog | Claimed done? |
|---|---|---|---|
| PostgreSQL | yes | partial | no |
| WebSocket | yes | new | no |
| LDAP | yes | partial | no |
| Redis | yes | new | no |
| gRPC | yes (over HTTP/2) | n/a (uses HTTP/2) | no |
| MQTT 5.0 | yes | new / MQTT 3.x partial | no |
| MongoDB | yes | new | no |
| Kafka | yes | new | no |
| SMB2/3 | yes | new / SMB3 partial | no |
| DCE/RPC | yes | new | no |
| TDS | yes | new / fields partial | no |
| SSH | yes | new / plaintext partial | no |
| AMQP 0-9-1 | yes | new | no |
| DoT | yes | partial | no |
| DoH | yes | partial | no |
| SIP | yes | new | no |
| RTP/RTCP | yes | new | no |
| NFSv3 | yes | new | no |
| SNMPv3 | yes | partial | no |
| RDP | yes | new | no |
| QUIC / HTTP/3 / QPACK / DoQ | yes | new / partial | no |

## 3. Version / profile per protocol

| Protocol | Profile implemented | Must-have coverage on the session path | Explicitly not complete |
|---|---|---|---|
| PostgreSQL | Protocol 3.0 | Startup, SSLRequest, auth, Query, Parse/Bind/Describe/Execute/Sync, RowDescription/DataRow/CommandComplete/Error/Ready; C/D/E not guessed from port 5432 | GSS/SCRAM crypto; encrypted TLS body |
| WebSocket | RFC 6455 v13 | HTTP Upgrade, mask/unmask, text/binary/continuation, control, Close, UTF-8 | `permessage-deflate` |
| LDAP | LDAPv3 RFC 4511 | BER LDAPMessage, MessageID, Bind, Search entry/done/reference, Modify/Add/Delete/ModifyDN, Extended, StartTLS | LDAPS decrypt without keys |
| Redis | RESP2 + RESP3 | nested Array/Map/Set/Push, Pipeline, binary-safe bulk, bounded nesting | streamed aggregates; RESP3 attributes |
| gRPC | HTTP/2 mapping | 5-byte prefix, span/pack DATA, unary and bidi, trailers/`grpc-status` | protobuf schema; compressed flag decode |
| MQTT | 5.0 | CONNECT/CONNACK properties, reason codes, topic alias, QoS 0/1/2 | MQTT-SN |
| MongoDB | OP_MSG + OP_COMPRESSED | kind 0/1 sequence, requestID correlation, snappy/zlib decompress | other opcodes; unknown compressor |
| Kafka | ApiVersions / Metadata / Produce / Fetch v0 whitelist | Correlation ID, uncompressed RecordBatch magic 2 | flexible versions; compressed batches |
| SMB | SMB2 + SMB3 negotiate | Compound, MessageId pairing, Session/Tree/FileId, Create/Read/Write/Close, transform Encrypted boundary | SMB Direct; encrypted body without keys |
| DCE/RPC | CO v5 | Bind/BindAck/Request/Response/Fault, FIRST/LAST, Call ID, EPM/SRVSVC | full stub schema beyond first interfaces |
| TDS | 7.x subset | PRELOGIN, LOGIN7, SQLBatch, RPC, token stream, ENCRYPT→tls | MARS |
| SSH | RFC 4253 first handshake | Banner, KEXINIT, selected algorithms, host-key SHA256 fingerprint, NEWKEYS Encrypted boundary | ciphertext USERAUTH/channel |
| AMQP | 0-9-1 only | Connection/Channel, Method/Header/Body, Publish/Deliver/Ack/Nack, heartbeat; AMQP 1.0 rejected | AMQP 1.0 |
| DoT | RFC 7858 | 2-byte DNS length on TLS plaintext, ID/QNAME association | TLS decrypt |
| DoH | RFC 8484 | GET (base64url `dns=`) and POST `application/dns-message` on HTTP/1.1 and HTTP/2 | DoH over TLS without plaintext |
| SIP | RFC 3261 | REGISTER/INVITE/ACK/CANCEL/BYE, Call-ID/CSeq/Via-branch, compact headers, TCP framing, SDP | IMS profile |
| RTP/RTCP | RFC 3550 | SSRC, seq wrap/dup/gap/jitter, SR/RR; gap is capture-missing not network loss | RFC 4571 length prefix (collides with DoT) |
| NFS | NFSv3 | ONC RPC v2, TCP RM, XDR, XID, LOOKUP/GETATTR/READ/WRITE | NFSv4 COMPOUND |
| SNMPv3 | USM + ScopedPDU | HeaderData, Engine/Context, Get/GetBulk/Set/Trap/Inform, VarBind/OID; priv→Encrypted | decrypt without keys |
| RDP | TPKT/X.224 | Cookie, NEG_REQ/RSP/FAILURE, SSL/HYBRID→tls, plaintext MCS/GCC | graphics/audio/device redirection |
| QUIC | RFC 9000 v1 | long-header CID/version, PN spaces, CRYPTO/STREAM/ACK/RESET/CONNECTION_CLOSE | short-header 1-RTT without keys |
| QUIC TLS | RFC 9001 | Initial secrets from client DCID, header protection, AES-GCM/ChaCha20, `ApplyQUICKeys`; missing keys Encrypted | Handshake/1-RTT without caller keys |
| HTTP/3 | RFC 9114 | control SETTINGS, bidi HEADERS/DATA, req/resp association, reset | push; datagrams |
| QPACK | RFC 9204 | encoder/decoder streams, static+dynamic, insert/capacity, blocked RIC=ContextRequired | Huffman-only edge cases beyond tests |
| DoQ | RFC 9250 | 2-byte DNS length on client-initiated bidi STREAM; missing query ContextRequired; RESET | HTTP/3-claimed streams skipped |

## 4. Samples

Generator: `protocol_session_samples_test.go:TestProtocolSessionM1PCAP` and
`bin_parser_session_test.go:TestLiveProtocolPCAP`. Kind:
`deterministic-generated-not-real-capture`. Ethernet+IPv4+TCP, RFC 5737
addresses, non-standard ports. SHA-256 is of the committed pcap bytes.

Directory: `common/pcapx/pcaputil/testdata/protocol-sessions/`

| File | Protocol | SHA-256 | Bytes | Port |
|---|---|---|---:|---:|
| postgres-extended-query.pcap | postgresql | `939e39f10eb7667d77db6707d908e6140dc3be275bda368fb89ee5e09ec00ed0` | 982 | 15432 |
| ldap-bind-search.pcap | ldap | `0ccfdd3626b19c30960bbe1968c6699013cc18cc9d29d8b578f5c6dc51f9cfd8` | 917 | 14389 |
| websocket-upgrade-text.pcap | websocket | `ee383c291d18abcc85b5e0c3cf50c812dea1f1a4831dc86d1672d84df308943b` | 921 | 18090 |
| redis-resp2-resp3.pcap | redis | `97b6427e0c5f4e1488bfdaa8341fc034645c85c7251a1d82373c781c442ee0e8` | 727 | 16379 |
| grpc-unary-http2.pcap | http2/grpc | `8b05a8b1293dce8c0f0ba1e4084e2fbb9e6390bff13d4a89e82ae382b6ee09a1` | 1206 | 18081 |
| mqtt5-connect-qos.pcap | mqtt | `d8a7a21673a7572d3cafe289c0c64fcaa6acf625d3c7c8dec3363a2815b35215` | 806 | 18830 |
| mongodb-opmsg-compressed.pcap | mongodb | `b1182f341fa9655ea69e8c0dcc7a4f2f229bbb56f7cc62c4f56a505701113a9a` | 690 | 27018 |
| kafka-apiversions-metadata.pcap | kafka | `9d27f7a1890c85a9c705ac9de7dff77db0131605c38cb5d808613c7a32253287` | 745 | 19092 |
| tds-prelogin-login-batch.pcap | tds | `cea2e5ba621f42df0123e8d67e82bf8371532cb8a0e6f051de0910876cbbf383` | 1173 | 11433 |
| amqp-publish-deliver.pcap | amqp | `6e1a89c345f4d67f9d8dad40bfbc318decb4c8b265f63fec96b11b9784c9d45d` | 1580 | 15672 |
| smb2-negotiate-create.pcap | smb2 | `19f7638dcffee691c112273ae808fe6de1c8e1e23a615e1ab57ee7196eff164d` | 1926 | 1445 |
| dcerpc-epm-srvsvc.pcap | dcerpc | `c23f3bec06a192aafa7079ddfb38ba37510a14f33b562220a05100efdb3cdf75` | 1342 | 13500 |
| ssh-kex-newkeys.pcap | ssh | `1d0e4bd4628c9e8bf66e3fcb97035d77a79529497b46a1b2a8626836768f4f59` | 1571 | 10022 |
| nfsv3-lookup-read.pcap | nfs | `4776939711962023d859e1434eb33dfd39c743a43ee06bcec67d991facb024f0` | 1704 | 12049 |
| snmpv3-get-response.pcap | snmp | `618af80452a61bf2eda8996c6ced70e9fe5638cc9d6d695cb363ab91e34f42ee` | 1936 | 1161 |
| rdp-tpkt-negotiate-mcs.pcap | rdp | `5033da8288408b2bccc0443cc91661487c24eee99c64ed71fe5b4a5d4c64d6eb` | 1055 | 13389 |
| dot-dns-tcp-length.pcap | dot | `ca073f1abebf0c0ded92297722a867db895ff3fe3a28a04084d25c7fa1ae5c92` | 910 | 1853 |
| doh-http-get-post.pcap | doh | `6d4e1f15ec3f50fe9cc790d943272aeb56c1e0948804549516e52965e77702e9` | 1267 | 18443 |
| sip-invite-ack-bye.pcap | sip | `06e06284db82ac3b5c6f0d7ec8e5b5c3391a9774c90c241a824bd268194f496c` | 2795 | 15060 |
| rtp-seq-sr-rr.pcap | rtp | `a3199a0d2d5f585a52d12116065fcb7f03468c852ff0a4b5a3878972e390f893` | 926 | 15004 |
| quic-v1-crypto-stream.pcap | quic | `3612ee02ab8b832a189fceba1b63dda67512011c6da9234043a62e7a41059028` | 882 | 14443 |
| quic-v1-rfc9001-initial.pcap | quic | `a04715a7f0ff155411d6eefd3379563807d63c7b2154224a00f95f5ee287ab99` | 1955 | 14443 |
| http3-settings-headers-data.pcap | http3 | `cb4c758077bc1b1b6c555215b86f9a4f9a8f2765d336572c75b44141651b28c0` | 910 | 14443 |
| http3-qpack-encoder-headers.pcap | http3 | `ee33438be781182484b2e84bb6c906e91615e08faf7c8b1ce3308c6c80a8c3b8` | 919 | 14443 |
| doq-query-response.pcap | doq | `a63ebbff6555bdbdb38e6c72f0971cfe7e2b1b444fbcf83392a035c9898233f8` | 851 | 14853 |
| http2-multiplex.pcap | http2 | `863b52523daa19711ba54f3e3d23a49e06624d5f78c54a926cae830ebf8b99c1` | 3703 | 18080 |
| mysql-classic.pcap | mysql | `a2562f5cbc945cab45f6ae1aad4d47068dd9ccd0bfd82fed851ec0283544e6f7` | 5780 | 13306 |

Oracle: HTTP/2 and MySQL have committed `*.tshark.tsv` from Wireshark 4.4.8
(`tshark_malformed_frames: []`). Later protocol pcaps do not ship tshark
oracles. Independent nDPI captures remain under
`common/bin-parser/testdata/protocol-corpus/`.

## 5. Verification commands and results

Commands run 2026-09-17 on this host. Logs:
`{scratch}/generate-rules.log`, `generate-catalog.log`, `vet.log`,
`protocol-tests.log`, `fragmentation.log`, `replay.log`, `race.log`,
`fuzz-review.log`, `fuzz-live.log`, `benchmark.log`, `regression.log`,
`git-diff-check.log`, `s24-summary.log`.

| Command | Result |
|---|---|
| `go generate ./common/bin-parser/rules` | exit 0, 0.4s |
| `go generate ./common/bin-parser` | exit 0, 0.7s; only `PROTOCOL_TODO.md` CRLF dirty, restored (pre-existing Windows generate noise) |
| `git diff --check` | clean |
| `go vet ./common/pcapx/pcaputil ./common/bin-parser/... ./common/pcapx/cmd/...` | exit 1: session-test `append` with no values (fixed in this PR); pre-existing `protocol-impl/tpkt.go` `WriteTo` signature vs `io.WriterTo` |
| `go vet ./common/pcapx/pcaputil ./common/bin-parser/parser/stream_parser` after fix | exit 0 |
| `go test ./common/pcapx/pcaputil -count=1 -timeout=10m -run TestProtocolSession` | ok 1.583s |
| `go test ./common/pcapx/pcaputil -count=1 -timeout=5m -run TestProtocolSessionFragmentation` | ok 2.3s |
| `go test ./common/pcapx/pcaputil -count=1 -timeout=5m -run TestProtocolSessionM1PCAP` | ok 2.5s (replay + SHA pin) |
| `go test ./common/pcapx/pcaputil -race -count=1 -timeout=8m -run TestProtocolSession` | ok 20.957s |
| `go test ./common/pcapx/pcaputil -run ^$ -fuzz FuzzProtocolSessionReview -fuzztime 45s` | PASS, 443616 execs, 526 new interesting |
| `go test ./common/pcapx/pcaputil -run ^$ -fuzz FuzzLiveProtocolSessions -fuzztime 45s` | PASS, 176340 execs, 66 new interesting |
| `go test ./common/pcapx/pcaputil -run ^$ -bench 'BenchmarkLiveProtocolSessions\|BenchmarkPcapBinParserHandshakeMix\|BenchmarkProtocolReview' -benchmem` | PASS, see §8 |
| `go test ./common/bin-parser/... ./common/pcapx/pcaputil/... ./common/pcapx/cmd/... ./common/utils/embeddedfs/... -count=1 -timeout=15m` | pcaputil/parser/stream_parser/protocol-impl/cmd/embeddedfs ok; `TestProtocolCorpusIntegrity` FAIL on `licenses/ndpi-COPYING.txt` SHA (Windows CRLF vs LF; pre-existing, file not in this stack) |

CGO warning on every compile: `p2l_go_callout` dllexport redeclaration in
`go-pcre2-lite` — third-party, not this stack.

## 6. Differential testing

This host has no `tshark`. HTTP/2 and MySQL generated pcaps already pin
Wireshark 4.4.8 TSV oracles with zero malformed frames
(`testdata/protocol-sessions/manifest.json`). Later protocol samples are
cross-checked against RFC/MS layouts in Go tests, not live tshark.

Absence of tshark is evidence-only. It is not treated as protocol success.

## 7. Fuzz / race

Shared, not per-protocol dedicated:

- `FuzzProtocolSessionReview`: Redis/LDAP/WebSocket/PostgreSQL seeds, chunked
  Feed, Close residual buffer = 0, peak ≤ budget. 45s, 443616 execs, PASS.
- `FuzzLiveProtocolSessions`: HTTP/2 and MySQL live replay path plus mutated
  tail. 45s, 176340 execs, PASS.
- `-race TestProtocolSession`: PASS, no race reports.

There is no per-protocol state-sequence fuzzer beyond these shared entry
points and the protocol fail-closed tests.

## 8. Benchmark

Same host, `count=1`. HTTP/2 and MySQL remain the public regression oracles.
There is no stored before-stack baseline on this machine; numbers are the
current after-stack snapshot.

| Benchmark | ns/op | MB/s | B/op | allocs/op |
|---|---:|---:|---:|---:|
| BenchmarkLiveProtocolSessions/http2-16 | 467080 | 3.37 | 569491 | 3114 |
| BenchmarkLiveProtocolSessions/mysql-16 | 213105 | 8.59 | 413125 | 1434 |
| BenchmarkPcapBinParserHandshakeMix/workers=1-16 | 29030892 | 77.23 | 43315997 | 333541 |
| BenchmarkPcapBinParserHandshakeMix/workers=2-16 | 16313415 | 137.43 | 46576745 | 338725 |
| BenchmarkPcapBinParserHandshakeMix/workers=4-16 | 11170613 | 200.70 | 49090767 | 338805 |
| BenchmarkProtocolReview/http-fragmented/chunked=false-16 | 296765 | 54.14 | 68424 | 26 |
| BenchmarkProtocolReview/http-fragmented/chunked=true-16 | 247475 | 64.98 | 5889 | 25 |

No unexplained O(n²) scan was observed on these oracles. Handshake-mix
throughput scales with workers. Alloc counts are high on the multiplex
replay path; they are reported, not gated as a 5%/10% regression, because
this host has no pre-stack `benchstat` file.

## 9. Resource limits

`ParserBudget` defaults (`DefaultParserBudget`):

| Field | Default |
|---|---|
| MaxFrameBytes | 1<<20 |
| MaxMessageBytes | 1<<20 |
| MaxBufferedBytes | 32<<20 |
| MaxRecursionDepth | 64 |
| MaxCollectionElements | 4096 |
| ProbeBytes | 64 |

Zero fields fill defaults. Negative values reject session creation.
`MaxRecursionDepth` > 64 or `MaxCollectionElements` > 4096 reject.
Exhaustion publishes `ResourceExceeded` and does not return a success tree.

Task-book fields **not** on the struct (no silent defaults):
`MaxActiveRequests`, `MaxStreams`, `MaxChannels`, `MaxDictionaryBytes`,
`MaxDecompressedBytes`, `MaxCompressionRatio`, `MaxStoredEvents`.
QPACK capacity uses the existing structural budget. Adding unused fields
without enforcement was not done.

## 10. Capture / replay examples

Replay generated samples (no extra tools):

```sh
go test ./common/pcapx/pcaputil -run '^TestProtocolSessionM1PCAP$' -count=1
go test ./common/pcapx/pcaputil -run '^TestLiveProtocolPCAP$' -count=1
```

Regenerate (only when intentionally rewriting committed bytes):

```sh
YAK_UPDATE_SESSION_PCAP=1 go test ./common/pcapx/pcaputil -run '^TestLiveProtocolPCAP$|^TestProtocolSessionM1PCAP$' -count=1
```

Public session API (same path as live capture):

```go
s, err := NewProtocolSession(DefaultParserBudget())
p := s.Probe(firstBytes)          // Reject / NeedMore / Accept
r := s.Feed(0, ts, firstBytes)    // Consumed, Events, State, Err
_ = s.Close("FIN")
_ = s.Stats()
```

If tshark is available:

```sh
tshark -r testdata/protocol-sessions/http2-multiplex.pcap -d tcp.port==18080,http2 -Y http2
tshark -r testdata/protocol-sessions/mysql-classic.pcap -d tcp.port==13306,mysql -Y mysql
```

## 11. Unimplemented extensions

- WebSocket `permessage-deflate` (negotiation, takeover, bomb budget)
- gRPC protobuf schema inference and message compression
- Kafka flexible versions and compressed RecordBatch
- NFSv4 COMPOUND
- AMQP 1.0
- SMB Direct / encrypted SMB body without keys
- SSH post-NEWKEYS ciphertext
- RDP graphics / audio / device redirection
- QUIC Handshake/1-RTT without caller keys
- HTTP/3 push and datagrams
- MQTT-SN
- MARS (TDS)
- RFC 4571 RTP framing
- Dedicated per-protocol fuzz, race, and 1KB/16KB/1MB × 1/100/1000-conn benches
- `ProtocolRelation` / `ContextLevel` types (context lives on `ProtocolEvent.Session`)
- Scored multi-protocol arbitration (fixed probe order plus NeedMore/Reject rules)

## 12. Out-of-scope limitations (task book)

- TLS decrypt of LDAPS / `wss` / DoT / RDP CredSSP without caller keys
- Claiming encrypted bodies parsed
- Rubber-stamping catalog `done`
- Merging #5013 into `main`
- GUI / expert diagnostics
- Replacing existing HTTP/TLS/MQTT 3.1/DNS/Kerberos/Memcached/Cassandra auto-admission

Fail-closed Encrypted / ContextRequired / Malformed / ResourceExceeded is the
supported outcome for those cases. It is not success.

## 13. Blocked on this host

| Item | Evidence |
|---|---|
| Live tshark differential for M1–M5 samples | `Get-Command tshark` empty |
| `TestProtocolCorpusIntegrity` | `licenses/ndpi-COPYING.txt` SHA `03c570a0…` vs `9ccf26cf…` (CRLF vs LF); file not in this stack |
| `go vet` `protocol-impl/tpkt.go:25` `WriteTo(int, error)` vs `io.WriterTo` | present on `main`; not part of the session stack |
| Pre-stack benchstat comparison | no committed before numbers on this machine |

## This PR

Fixes the session-test `append` calls `go vet` reported (`appends` check) in
`protocol_session_kafka_test.go`, `protocol_session_samples_test.go`, and
`mqtt_fields_test.go`. Does not change parse behavior. Does not flip catalog
status.
