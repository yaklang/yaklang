# Protocol sessions, round 5 — 2026-09-19

This delivery rebases the previous four commits onto main `ab520b4aac` and
implements the ten candidates from the September 18 list. While work was in
progress, #5137 added IEC104, DNP3, C37.118 and GOOSE. Its three other handlers,
fixtures and tests are retained. IEC104 combines its public event fields and
STARTDT association with the stricter directional control/ASDU implementation
below. ASDU confirmations use direction/type/common address/IOA; transport N(R) is not treated as an application transaction ID. No roadmap score or `done` row is raised by this session delivery.

## Delivered profiles and explicit limits

| Protocol | Implemented session content | Boundary |
|---|---|---|
| STUN | TCP framing and bounded UDP conversations; direction/endpoint/transaction association and expiry; IPv4/IPv6 XOR addresses, padded attributes, fingerprint validation | MESSAGE-INTEGRITY is explicitly unverified without credentials; classic pre-cookie STUN is separate |
| TURN | Allocate/Refresh observations, multi-peer CreatePermission, ChannelBind, TCP/UDP ChannelData and expiry; transaction/permission/channel/byte limits | Observed grants are not authenticated; relay payload stays opaque; ambiguous bidirectional channel IDs do not invent a peer |
| TFTP | RRQ/WRQ, server transfer-ID binding, OACK, blocksize/timeout/tsize, DATA/ACK/error, identical retransmission, 16-bit rollover and final empty block | No retained file body; windowsize above 1 is explicit unsupported; UDP only |
| RTSP | 1.0 headers/body, CSeq correlation, Session lifecycle, SDP lines and interleaved RTP/RTCP channel mapping | RTSP 2.0 and HTTP tunneling are unsupported; UDP media flows are not cross-linked; missing SETUP remains visible |
| VNC/RFB | 3.3/3.7/3.8 negotiation, None/VNC security phases, result, ClientInit/ServerInit, pixel format, input/clipboard/color map and rectangle framing | Authentication bytes are not verified; Raw/CopyRect/RRE/cursor/desktop-size metadata supported; ZRLE/zlib is bounded opaque compressed data; other encodings stop explicitly; proprietary 4.1 is unsupported |
| IPP | HTTP POST application/ipp; final-response association without consuming 100/103; request ID, operation/status, attribute groups, typed values and bounded collections | Document content stays opaque; transport is HTTP/1.x; no printer/job scheduler or IPPS decryption |
| Diameter | TCP message boundaries; known base/credit-control admission; direction/Hop-by-Hop/End-to-End/application matching; typed base and bounded grouped/vendor AVPs | Vendor payloads without known schema are opaque; SCTP and full application dictionaries are separate |
| S7comm | TPKT/COTP connection/setup and bounded segmentation; S7 PDU reference, ReadVar/WriteVar items and error items, negotiated PDU size | Userdata, block upload/download and unknown functions retain envelope/raw data with `unsupported-*` semantic status; S7commPlus is separate |
| OPC UA TCP | HEL/ACK/ERR/RHE, negotiated receive/message/chunk limits, channel/sequence/request IDs, bounded continuation/final/abort, known service type and response association under SecurityPolicy None | `service-envelope-only` does not decode complete service bodies; signed/encrypted or missing-security-context chunks report `security-context-required` and `Content Decoded=false`; no key recovery or cryptographic validation |
| IEC104 | I/S/U framing, directional U request/confirmation, STARTDT/STOPDT/TESTFR, sequence/gap/wrap observations, common ASDU objects/values/quality/time and commands | Midstream capture does not imply an observed STARTDT; unknown ASDU types remain explicitly opaque; no serial IEC101/102/103 or conformance certification |

The protocol engine's `decoded` status means the selected envelope and supported
fields were decoded. The semantic-status fields above distinguish that from
complete application support. OPC UA and RFB deliberately do not turn a valid
encrypted/compressed envelope into a claim of plaintext content.

## Real capture evidence

All rows below replay original upstream bytes through the public capture path,
with one and two workers and full/deferred parity. Tests assert exact framed
message counts and protocol content, then require zero retained bytes at close.
Capture record counts are not used as message counts.

| Original capture | Framed profile messages | Evidence / original limitations |
|---|---:|---|
| `ndpi-stun` | STUN 56 + TURN 104; 1 malformed STUN | Allocate/Refresh/permission/channel exchanges. Original frame 195 has a bad FINGERPRINT, independently reported by tshark; retained as a negative case |
| `stun_signal_tcp.pcapng` | TURN 291 | TCP relay traffic |
| `stun_tcp_multiple_msgs_same_pkt.pcap` | TURN 6 | Multiple messages coalesced into TCP payload |
| `ndpi-tftp` | 107 | Request, data/ack and observed completion |
| `ndpi-rtsp-http` | 1 | Direct SETUP only, despite the filename |
| `rtsp.pcap` | 65 | DESCRIBE/SDP, SETUP and control responses; 568 capture records |
| `vnc-sample.pcap` | 70; 1 incomplete | RFB 3.8, VNC challenge/result, 1024×768 desktop named `test`, cursor/ZRLE. Original TCP sequence gap is asserted as a replay error |
| `ndpi-ipp` | 8 | IPP requests/responses and typed attributes |
| `ndpi-diameter` | 6 | Credit-Control command 272/application 4, including response association |
| `ndpi-s7comm` | 170 | COTP/setup, read/write items, five error items without value bytes; unsupported block/userdata semantics remain explicit |
| `ndpi-opcua` | 187 | HEL/ACK/channel and service envelopes |
| `ndpi-iec104` | 8 | Midstream S/I frames and type 36 telemetry |

`ndpi-vnc` independently proves rejection of proprietary RFB 004.001, and has
an original TCP gap. `ws-opcua-signed` proves the protected-content boundary.
These two files are not counted as positive standard/plaintext service fixtures.

The existing [corpus manifest](../../bin-parser/testdata/protocol-corpus/manifest.json)
contains pinned commits, licenses and hashes for ten reused captures.
The [new-capture manifest](testdata/protocol-sessions/upstream/manifest.json)
contains the four newly collected originals, URLs, SHA-256, sizes and record
counts. Three are pinned to nDPI commit
`4cae778e7e8f846b34f11d4f8392504cdebd3db8` (LGPL-3.0).
The Wireshark Wiki VNC attachment does not state a separate attachment license;
the manifest records that uncertainty instead of assigning the Wireshark code
license to it. No capture was rewritten to make a regression pass.

## Compatibility and validation contracts

- New STUN/TURN and RTSP session entries live in `stun_session.yaml` and
  `rtsp_session.yaml`. Existing `stun.yaml` / `rtsp.yaml` default roots and their
  original direct-rule/scorecard tests stay unchanged. Multi-entry session
  envelopes always use explicit entrypoints; the rule archive is regenerated.
- The shared HTTP pipeline carries IPP admission beside DoH, leaves ordinary
  deferred HTTP deferred, and does not consume request association on 100/103.
- Probe tests retain SIP OPTIONS and fragmented WebSocket behavior. S7 COTP
  admission requires S7/TSAP evidence and does not use port 102 alone.
- Synthetic cases are labeled by their test origin, not described as additional
  external captures: 0/1/7-byte splits, opposite directions, transaction limits,
  channel/permission expiry, transfer-ID mismatch, TFTP rollover/empty final,
  malformed AVPs/ASDUs/IPP values, UA chunk abort/limits and snapshot ownership.
- Session snapshots recursively clone and account for heterogeneous attribute
  lists and encoding lists. UDP conversations are capture-owned, bounded and
  expired; callbacks execute outside the state mutex.
- `FuzzRound5ProtocolBoundaries` exercised 167,311 inputs in 45 seconds without
  a failure before #5137 integration; the later ordinary suite reruns its seed
  corpus. This is not a claim of exhaustive protocol conformance.

Primary references: [STUN RFC8489](https://www.rfc-editor.org/rfc/rfc8489.html),
[TURN RFC8656](https://www.rfc-editor.org/rfc/rfc8656.html),
[TFTP RFC1350](https://www.rfc-editor.org/rfc/rfc1350.html),
[RTSP RFC2326](https://www.rfc-editor.org/rfc/rfc2326.html),
[RFB RFC6143](https://www.rfc-editor.org/rfc/rfc6143.html),
[IPP RFC8010](https://www.rfc-editor.org/rfc/rfc8010.html),
[Diameter RFC6733](https://www.rfc-editor.org/rfc/rfc6733.html),
[OPC Foundation Part 6](https://reference.opcfoundation.org/specs/OPC-10000-6/7.1).
Wireshark's S7comm/IEC104 dissectors provide implementation cross-checks, not
substitutes for normative IEC/Siemens conformance specifications.

Ordinary deferred HTTP request/response microbenchmark (same machine, three
300 ms runs) retains the baseline **45 allocations / 12,273 B per operation**.
Median time was 4,508 ns before and 4,172 ns after; these short runs are a
regression check, not a claim of capture throughput or controlled speedup.
