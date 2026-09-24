# PR #5013 — first-batch M1, 2026-09-22

Baseline: `5f4ea0ecaecec97a93e5dbd2ba8e46bf1a36e79b`, including latest main
`e16ff21238c60c02a1a363d05c9132d8f5830ee3`. This delivers the bounded M1
native-capture analysis loop from the attachment's DISPATCH. It does not mark
all T00–T21 tasks complete and does not promote the protocol roadmap to stable.
The historical `first-batch-capabilities.json` remains the M0 snapshot.

## Delivered M1 profiles

| Area | Behavior and verification |
|---|---|
| T01 reader | Replay, PacketAnalyzer and shark share bounded pcap/pcapng reading; owned public records, synchronous borrowed replay; section/interface identity and packet numbering survive. NRB names now have record, string and lifetime budgets. Embedded DSB secrets remain rejected. |
| T02 network | Capture-domain isolation includes interface/section, VLAN and GRE/VXLAN outer endpoints/identifier. IPv4/IPv6 fragments are bounded, out-of-order capable, and reject overlapping ranges (exact IPv4 retransmission is ignored). Missing fragments emit incomplete on timeout/close. |
| T03 admission | Native UDP DNS/mDNS/LLMNR/DHCP/DHCPv6 registry; explicit `WithProtocolDecodeAs` validates bytes rather than blessing them. TCP/stateful legacy bindings remain compatible. ARP/ICMP/ND enter through the shared native network path. |
| T04 evidence | PDU IDs, transaction/response references, domain, completeness, typed expert errors, captured/reassembled/decrypted byte sources, SHA256, contributing packets and TLS parent PDUs. Owned event/history snapshots; bounded history evicts explicitly. |
| T05 consumers | API, pcap-inspect and shark share the protocol engine. Shark advances protocol state before its lossy display queue, retains event IDs, and uses retained whole-session facts for details. Contributors can resolve PDUs completed by later packets. Typed `fields` filters do not change saved packet records. |
| T06 material | Three new native captures with hashes, source/generator, TShark 4.4.8 fields and immutable oracle TSV; existing upstream DNS/mDNS/DHCP/ICMP captures reused without rewrapping. |
| T07 DNS | Shared UDP/TCP/DoH DNS codec: bounded compression/RDATA cursors; common RRs, OPT and opaque SVCB/HTTPS parameter vectors. Correlation includes capture domain, endpoints, transport, ID and questions. mDNS exposes QU/cache-flush/TTL withdrawal as observations. Bare DNS/TCP is labeled DNS, not authenticated DoT. |
| T08 LAN | DHCPv4 option concatenation/overload, lease/address/router/DNS metadata and observed client/xid association; DHCPv6 DUID, IA/address/prefix/lifetime/status and bounded relay nesting. ARP and ND are unverified observations; ICMP quoted-packet/MTU metadata. |
| T09 TLS | Cross-record handshake metadata, negotiated version/cipher/SNI/ALPN, visible certificate summaries, immutable explicit NSS provider. Authenticated AES128 GCM TLS1.3 and TLS1.2 ECDHE RSA/ECDSA paths; independent direction/sequence/epoch, Finished transition and KeyUpdate. No plaintext on missing key, unsupported cipher or failed tag. |
| T10 web | HTTP/1 request/response IDs and latency, 1xx/HEAD/body framing, bounded explicit gzip/zlib body decoding; WS key/accept validation, masking/fragmentation and negotiated permessage-deflate (15-bit window profile, direction-specific dictionaries and takeover flags). |
| T11 H2/gRPC | Plain and authenticated TLS carriers share HPACK/HTTP2 state. Body byte count/Content-Length/HEAD/204/304 validation; gRPC cross-DATA messages, identity/gzip, index and trailers. Message errors stay stream-scoped. Unsupported server push consumes HPACK and preserves other streams. Protobuf reports field numbers/wire types only, never schema names. |

`ProtocolEvent.DisplayFields()` provides the registered filter aliases. Operators:
existence, equality/inequality, ordered numeric/string comparison, contains,
IP CIDR membership, and/or/not and parentheses. Missing fields do not match `!=`.
This is a documented subset, not full Wireshark filter compatibility.

Use `pcap-inspect -read capture.pcap -tls-keylog authorized.keys`, or
`yak shark --pcap-file capture.pcap --tls-keylog authorized.keys`. Keys are never
auto-discovered or exported. The committed `.keys` contains only generated
loopback session secrets. Record authentication does not prove certificate trust
or business authentication. Certificate metadata always says `not-evaluated`.

## Native evidence and executable acceptance

`testdata/protocol-sessions/first-batch-m1/manifest.json` is the material index.
Generator and exact oracle commands: `scripts/protocol-tests/generate-m1/README.md`.

* 92-record actual loopback TLS capture: TLS1.2 c02f + TLS1.3 1301, SNI
  `m1.example`, two HTTP2/gRPC bidi exchanges, 12 five-byte protobuf messages,
  final status 0. Test matrix: keys/no keys, full/deferred, workers 1/2/4;
  wrong secrets yield zero child plaintext. KeyUpdate/sequence/tag-negative
  checks are crafted cryptographic tests, not falsely labeled native captures.
* 42-record actual loopback HTTP/WS capture: HEAD and GET with 103/200, chunked
  body/trailer, valid Upgrade, four compressed 1500-byte messages. API and shark
  compare field values and response references, then compare saved raw records.
* Original 12-record Wireshark DHCPv6 capture: six DHCPv6 messages, three
  xid-correlated responses, IA prefix `2001:0:0:fe00::`, four ND messages.
  Existing upstream mDNS capture also supplies real Ethernet ARP evidence;
  the similarly named Frame Relay `wireshark-arp.pcap` is not counted as ARP support.

Top-level test prefixes `TestFirstBatchT01`–`T11` identify profiles/cases, not
completion of each task's entire future list. Ownership, malformed length,
compression budget, fragment domain/overlap, HTTP message semantics, stream
isolation and DecodeAs negative cases accompany the native positive captures.
The old synthetic WS sample lacking a valid 101 accept is retained unchanged
as a negative fixture; the bare DNS/TCP fixture is relabeled truthfully.
Old gRPC tests now assert error isolation plus zero bytes at connection close,
rather than requiring an invalid message to destroy the whole HPACK connection.

Local verification commands (Go 1.22.12, macOS arm64):

```sh
go test -json ./common/pcapx/pcaputil ./common/pcapx/cmd/pcap-inspect ./common/yak/cmd/yakcmds/shark-cli -count=1
go test ./common/bin-parser/...
go test -race ./common/pcapx/pcaputil ./common/pcapx/cmd/pcap-inspect ./common/yak/cmd/yakcmds/shark-cli
GOMAXPROCS=2 go test ./common/pcapx/pcaputil -run '^$' -fuzz '^FuzzFirstBatchM1$' -fuzztime=45s -parallel=2
```

The sustained M1 fuzz run completed 134,325 executions without failure.
Full bin-parser passed. The complete API/inspect/shark JSON run passed 1,456 test/subtest entries; nine existing opt-in live/large/ paced tests were skipped. These skips are not native-fixture acceptance results. Final
integration/race results and exact-head CI are recorded in the delivery response;
CI success is not inferred from local tests or an earlier PR head.

## Performance and resource boundary

Raw A/B output and medians: `performance/2026-09-22-first-batch-m1/`.
GOMAXPROCS=2, one worker, three 300ms runs; each batch contains 3,080 synthetic
MQTT/HTTP/opaque TLS PDUs. Message counts are checked. This is neither Internet
throughput nor TLS decryption throughput. The M1 path does additional semantic
and provenance work absent from M0.

| Mode | M0 median ms/batch | M1 median ms/batch | Time change |
|---|---:|---:|---:|
| Full | 9.144 | 10.993 | +20.2% |
| Full + history | 10.142 | 12.932 | +27.5% |
| Deferred + history | 5.072 | 8.466 | +66.9% |

Compared with the first M1 implementation, eliminating replay's redundant packet
copy and duplicate retained TLS snapshots reduced deferred time by about 12%
and allocations from 75,406 to 65,133 per batch. **This milestone does not claim
an end-to-end speedup over M0.** Native semantic/state facts are still computed
in deferred mode; only the remaining field-tree work is deferred.

Packet blocks <=16 MiB; NRB metadata <=1 MiB/section and <=4 MiB lifetime;
fragments <=65,535 bytes, <=128 ranges/session and 30s capture-time expiry;
DNS/LAN associations expire at 30s; parser collections and aggregate retained
bytes use the existing ParserBudget. Keylogs <=1 MiB/4096 entries. HTTP body
output <=16 MiB and caller-selected lower bound. Shark history is 4096 PDUs /
32 MiB; each detail snapshot is <=256 KiB logical budget and <=128 PDUs.
Budgets are documented logical ownership estimates, not exact Go heap or RSS.

## Explicit remaining boundaries

M2 stays uncompleted: native QUIC/H3/QPACK/DoQ application closure, advanced
Redis/Kafka/MySQL/Postgres profiles and shared QUIC key selection. The full
T00–T21 plan remains open where it exceeds this M1 slice.

TLS AES256/ChaCha/CBC, DTLS, ECH, renegotiation/resumption recovery, late key
injection, missing-handshake recovery and certificate trust are not supported.
TLS1.2 ECDSA shares the cipher path but lacks a separately captured positive
ECDSA fixture. HTTP2 server-push application semantics, extended CONNECT/h2c
and schema-based Protobuf names remain outside the profile. Unsupported or
missing-context traffic must not be reported as a successful application message.

DHCP association is an observed client/xid exchange, not authoritative lease or
server identity validation. ARP/ND/mDNS are observations, not a trusted neighbor
cache or attack detector. DNSSEC validation, service-binding policy and automatic
TCP fallback orchestration are not claimed. IP overlap policies are conservative;
IPv6 overlap is rejected, including duplicate fragments. Arbitrary tunneling and
unknown DLTs are limited; shark explicitly refuses mixed-DLT export. Evicted
history and lost capture packets cannot be reconstructed by a viewer.
