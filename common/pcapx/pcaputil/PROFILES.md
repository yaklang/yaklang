# Protocol session profiles

A decoded envelope is not complete application support. The tables below describe the implemented version, context and opaque-content boundaries. Capture-backed regression tests load the sealed ZIP batches; current execution evidence belongs in the PR description. Historical delivery iterations are preserved in the auxiliary ZIP, not used as current acceptance results.

Additional native profiles and their exact message/carrier boundaries are registered in `protocol_registry.go`; this table covers selected core profiles. Public session and sealed replay regressions validate the registered boundaries.

## Core profiles

| Protocol | Profile implemented | Must-have coverage on the session path | Explicitly not complete |
|---|---|---|---|
| OpenDroneID selected v2 | Caller-selected raw UDP test/tunnel carrier; Basic ID, Location and bounded packs containing at most two Basic IDs and one Location | Official pinned C encode/decode fields; exact 25-byte messages, pack counts/types and atomic refusal; workers/observer/full/deferred parity, no datagram joining or automatic admission, byte/collection budgets and owned output | BLE/Wi-Fi framing, other messages, authentication, aircraft identity or physical validity of measurements; reserved and sentinel values are preserved without those claims |
| DLMS Wrapper v1 | Unciphered LN Get-normal scalars/octets and selected bounded Data graphs, selected Get-with-list Data, and observed Get-normal/Get-with-list data-block chains over TCP/UDP; ordered descriptors and mixed per-item data/error results | Exact frame/field spans and owned output; reversed direction/application wPorts, full invoke flags, service choice and item count for observed pairing; reuse stays ambiguous across choices; reserve graph before expansion; local list64, 256 Data nodes, depth8, octet/text1024 and 64 blocks/1024 assembled bytes plus caller budgets; block1 and every consecutive Get-next are required; TCP chunks and UDP never joined | Normal selective access, other Data types, GBT/cipher services, HDLC list transport, ACSE/authentication, negotiated MULTIPLE_REFERENCES, object/selector meaning or meter operation success; empty lists and literal80/count widths above4 remain explicit unsupported selected boundaries pending normative clauses |
| DLMS HDLC tunnel | Standalone type3 frames over TCP/UDP, selected SNRM/UA/DISC/DM/supervisory controls and unciphered LN Get-normal scalars | HCS/FCS, address/length/LLC validation; exact scalar widths and lossless 64-bit decimal strings; observed endpoints/logical addresses, invoke and HDLC sequence progression; retry, ID reuse, disconnect/ambiguity, byte/collection limits, ownership and close | IEC IP wrapper, shared idle flags, ACSE/authentication/ciphering, segmentation/block/list transfer, other services or meter object interpretation; one pending exchange and at most 128 distinct invoke/send-sequence tokens (also limited by the configured collection budget), reuse requires close or 30-second UDP idle reset |
| SLMP | Manufacturer SH(NA)-080956ENG-M, TCP/UDP binary 3E/4E connected-station Self Test 0619/0000 | Complete timer/count/loopback and error fields; one observed request, exact echo/route/serial association, conservative overlap and serial reuse guard, 30-second idle reset, shared byte/collection budgets and owned data | ASCII, other commands, private MELSOFT UDP 5560, device authentication; 3E has no wire transaction ID, so a second exchange within the idle conversation is context-required |
| DroneCAN | Explicit `WithCANDecodeAs(interface, "dronecan")`, classical extended CAN, v0 standard ID 341 single-frame NodeStatus | Complete uptime/health/mode/sub-mode/vendor code and transport fields; strict tail/length, per-interface evidence, byte budgets and owned data | Multi-frame CRC/reassembly, anonymous messages, services, CAN FD application, Cyphal v1, node authentication, online/restart inference or request/response association |
| SocketCAN / selected J1939 | DLT227 classical controller records and canonical 72-byte CAN FD; J1939 requires `WithCANDecodeAs(interface, "j1939")` | Network-order IDs, strict flags/lengths, RTR, controller error details, FD flags, interface domains and owned data; selected classical Request PGN and Address Claim NAME | Bus CRC/authenticated identity, address-claim success, J1939 TP/FD application, automotive ISO-TP/UDS/OBD; other parameter-group payloads remain opaque |
| PostgreSQL | Protocol 3.0 | Startup, SSLRequest, auth, Query, Parse/Bind/Describe/Execute/Sync, RowDescription/DataRow/CommandComplete/Error/Ready; C/D/E not guessed from port 5432 | GSS/SCRAM crypto; encrypted TLS body |
| WebSocket | RFC 6455 v13 | Validated HTTP Upgrade, masking, fragmentation, UTF-8 and negotiated permessage-deflate | 15-bit window profile; no unsupported extension guessing |
| LDAP | LDAPv3 RFC 4511 | BER LDAPMessage, MessageID, Bind, Search entry/done/reference, Modify/Add/Delete/ModifyDN, Extended, StartTLS | LDAPS decrypt without keys |
| Redis | RESP2 + RESP3 | nested collections, attributes, pushes, pipeline, binary-safe bulk and bounded context | streamed strings and aggregates |
| gRPC | HTTP/2 mapping | Cross-DATA framing, unary/bidi, trailers, identity/gzip messages | No protobuf schema names; unsupported compression stays explicit |
| MQTT | 5.0 | CONNECT/CONNACK properties, reason codes, topic alias, QoS 0/1/2 | MQTT-SN |
| MongoDB | OP_MSG + OP_COMPRESSED | kind 0/1 sequence, requestID correlation, snappy/zlib decompress | other opcodes; unknown compressor |
| Kafka | Bounded ApiVersions / Metadata / Produce / Fetch profiles | Correlation IDs, legacy and selected flexible versions, none/gzip record batches | Version whitelist below; unsupported APIs and codecs remain explicit |
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

## Media, messaging and industrial extensions

| Protocol | Implemented session content | Boundary |
|---|---|---|
| LLDP | Ended Ethernet discovery with ordered Chassis/Port/TTL and optional TLVs, binary-safe text, management address, selected PNO Port Status/Chassis MAC and IEEE802.3 MAC/PHY fields; VLAN/domain evidence and bounded byte/TLV collections | Neighbor identity remains unverified; no learned topology, PROFINET DCP/RT, or End-less LLDP profile; unknown TLVs stay ordered and opaque |
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

## Native endpoint profiles

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
auto-discovered or exported by production readers. The existing `.keys` fixtures
contain generated loopback session secrets, sealed with the reproduction tools
in the supporting-materials ZIP. Tests materialize explicit private copies for
filename APIs. Record authentication does not prove certificate trust
or business authentication. Certificate metadata always says `not-evaluated`.

## Recognition and additional transport profiles

| API | Non-flexible versions | Native positive evidence in the sealed corpus |
|---|---|---|
| ApiVersions | 0..2 | All three versions |
| Metadata | 0..8 | All nine versions |
| Produce | 0..7; legacy message formats retained | 3..7, none/gzip and acks=0 |
| Fetch | 0..11; legacy low-version paths retained | 4..11 |

Source schema: Apache Kafka tag 3.9.1, message JSON definitions. Modern flexible
ApiVersions 3 / Metadata 9 / Produce 9 / Fetch 12 were also have dedicated profiles alongside these legacy ranges. Their native Java
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
client are sealed at `scripts/protocol-tests/generate-m2a/`.

Reproduction paths in this document are virtual ZIP aliases. Use
`go run ./internal/trafficfixture/cmd/corpus export /tmp/new-traffic-workspace`
to obtain configs, certificates, synthetic keylogs and the original build/capture
recipes, or `corpus exec -- ... @path` for temporary verified tool inputs.

Capture-backed tests run full/deferred with workers 1/2/4 and check all 31 Kafka
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

Wrapper GET-with-list additionally decodes selected nested A-XDR array/structure values and selection parameters. The local limits are 64 top-level items, 256 total Data nodes across the complete APDU, 8 Data levels and 1024 bytes per octet value, further constrained by caller collection/depth/shared-byte budgets. Get-with-list also supports bounded bit strings, big-endian IEEE binary32/binary64 (exact bits, JSON-safe non-finite representations), printable US-ASCII visible strings and strict RFC3629 UTF-8 without trimming or normalization. Get-normal retains existing scalar/octet fields and additionally decodes the same bounded bit strings, IEEE floats, printable ASCII and strict UTF-8. Get-normal also accepts bounded array/structure roots with the same shared256-node,8-level graph limits. Root legacy scalar/octet fields and caller budgets remain compatible; graph octet/text/bit values keep the local1024-byte bound. Normal selective access remains open. Observed Get-normal and Get-with-list data-block chains may assemble the selected Data graph or ordered list results after a complete consecutive request/response chain; TransactionID belongs to the initial request and ResponseTo to the immediately observed request. Missing, reused or conflicting context is ambiguous and releases retained transfer bytes. List block result counts must match the initial descriptors; result offsets are relative to the assembled list body. GBT, other Data types, HDLC list, ACSE, authentication and object/selector meaning remain open. UDP observed pairing after idle cannot prove the originating generation of byte-identical delayed replies.
