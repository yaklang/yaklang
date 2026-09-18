# Next 10 live protocol session candidates — 2026-09-18

SMTP, IMAP, POP3, FTP, TNS, RADIUS, DHCP, NTP, CoAP and Modbus already have
bounded session profiles. Their coverage and gaps are recorded in
[round 3](PROTOCOL_SESSION_ROUND3.md) and [round 4](PROTOCOL_SESSION_ROUND4.md).
They are no longer future candidates.

The following order is an engineering recommendation based on this branch's
existing decoders, captures, session gaps and useful overlap with SIP/RTP,
HTTP and Modbus. It is not a popularity ranking or a claim of full support.
None has a dedicated live session handler in `probeWire` / `consumeSession`.
Several already have bounded packet rules or `done` roadmap rows; those rows
measure a different contract and must not be presented as live session coverage.
TURN currently aliases the STUN scorecard, which does not establish allocation
or ChannelData lifecycle coverage.

## Recommended order

| Rank | Protocol / reason | Reuse and first deliverable | Required evidence / missing samples | Relative effort |
|---|---|---|---|---|
| 1 | **STUN**: fills the NAT/connectivity side of SIP/RTP traffic | Reuse `stun.yaml`; UDP datagram admission, transaction ID + endpoint correlation, attributes/padding, XOR addresses, bounded expiry | Existing `ndpi-stun`; add IPv6, retransmissions, wrong-endpoint replies, malformed padding and TCP framing. Integrity is unverified without keys | Small–medium |
| 2 | **TURN**: builds directly on STUN for relay traffic | Allocate/Refresh/CreatePermission/ChannelBind, channel-to-peer state, ChannelData; explicit allocation/permission/channel budgets | STUN samples do not prove complete TURN exchanges. Add real coturn allocation, refresh/expiry, errors, ChannelData and TCP padding captures | Medium |
| 3 | **TFTP**: compact transfer state machine and several existing corpora | Reuse `tftp.yaml`; RRQ/WRQ, server transfer-port binding, DATA/ACK block association, ERROR, bounded retransmission handling | Existing `ndpi-tftp`, `ws-tftp`, `tcpdump-tftp-packet-boundary`; add OACK/blocksize, duplicate/out-of-order blocks, rollover, final empty block, spoofed TID | Small–medium |
| 4 | **RTSP**: joins control sessions to the existing RTP/RTCP parsers | Reuse `rtsp.yaml`; RTSP/1.0 CSeq/Session, DESCRIBE/SETUP/PLAY/TEARDOWN, Content-Length and interleaved `$` frames | Existing `ndpi-rtsp-http` is **HTTP-tunneled** evidence, not direct RTSP coverage. Add direct TCP, pipelining, interleaving and UDP transport negotiation; declare RTSP/2.0 separately | Medium |
| 5 | **VNC / RFB**: adds remote desktop handshake visibility | Reuse `vnc.yaml`; 3.3/3.7/3.8 negotiation, security result, ClientInit/ServerInit, bounded framebuffer metadata | Existing `ndpi-vnc`; add version/security failures, variable pixel formats, rectangles and encoding changes. Unsupported compression must retain a clear boundary | Medium |
| 6 | **IPP**: extends HTTP inspection into printer/job semantics | Existing HTTP framing; `application/ipp` admission, request ID, operation/status, attribute groups and value bounds; leave document bytes opaque | Existing `ndpi-ipp`; add Get-Printer-Attributes/Print-Job errors, repeated values, collection limits and chunked HTTP with mixed ordinary traffic | Medium |
| 7 | **Diameter**: extends AAA coverage beyond RADIUS | Length framing, bounded/padded AVPs, Hop-by-Hop + direction matching, CER/CEA, DWR/DWA and DPR/DPA | Existing `ndpi-diameter`; add vendor/grouped AVPs, repeated requests, reconnect, error answers and oversized lengths. TCP first; SCTP needs separate transport work | Medium |
| 8 | **S7comm**: useful next industrial protocol after Modbus | TPKT/COTP framing, connection/setup communication, PDU-reference association, ReadVar/WriteVar item metadata | Existing `ndpi-s7comm`; add COTP segmentation, per-item errors, multiple jobs and vendor variants. `ndpi-s7comm-plus` is a separate protocol, not proof of S7comm support | Medium–large |
| 9 | **OPC UA TCP**: broadens industrial session/security visibility | HEL/ACK/ERR, negotiated limits, chunk sequencing, SecureChannel/RequestId metadata, bounded reassembly | Existing `ndpi-opcua` and `ws-opcua-signed`; add abort chunks, renewal, size limits and security-mode changes. Signed/encrypted service contents require keys/context | Large |
| 10 | **IEC 60870-5-104**: complements PLC protocols with telemetry/control | APDU length, I/S/U frame classification, send/receive sequence numbers, STARTDT/STOPDT/TESTFR, bounded ASDU envelope | Existing `ndpi-iec104`; add sequence wrap, reconnect, general interrogation, spontaneous events and malformed ASDU counts. Vendor ASDUs remain explicit extensions | Medium–large |

Sample IDs above refer to the existing
[corpus manifest](../../bin-parser/testdata/protocol-corpus/manifest.json), which
retains source commits, licenses and SHA-256 hashes. Presence is a starting point,
not evidence that every proposed session phase appears in a capture. No new
external captures were added in this maintenance round.

## Shared prerequisites and acceptance

STUN, TURN and TFTP need bounded UDP conversation handling before capture-level
association is claimed: direction-aware endpoint keys, idle expiry, maximum
sessions/transactions/bytes, close-time release, and isolated inspector snapshots.
The current stateless UDP path intentionally reports per-datagram observations.
TFTP additionally changes the server port; TURN carries peer/channel identities.
A plain five-tuple dictionary without those semantics is insufficient.

For each protocol, ship a small explicit profile first. Require public API and
actual capture-entrypoint tests, TCP split/coalesced frames or real UDP packets,
full/deferred parity, 1/2-worker replay, malformed/truncated/over-budget inputs,
wrong-direction association, close/reuse, race checks and original-capture byte
provenance. Standard ports cannot be the sole admission criterion. Encrypted or
unsupported phases must remain visible rather than being silently counted as
success. Do not increment roadmap completeness merely because a handler exists.

## Primary references

- STUN: [RFC 8489](https://www.rfc-editor.org/rfc/rfc8489.html).
- TURN: [RFC 8656](https://www.rfc-editor.org/rfc/rfc8656.html).
- TFTP base: [RFC 1350](https://www.rfc-editor.org/rfc/rfc1350.html).
- RTSP: [RFC 2326 (1.0)](https://www.rfc-editor.org/rfc/rfc2326.html), [RFC 7826 (2.0)](https://www.rfc-editor.org/rfc/rfc7826.html).
- RFB: [RFC 6143](https://www.rfc-editor.org/rfc/rfc6143.html).
- IPP encoding/transport: [RFC 8010](https://www.rfc-editor.org/rfc/rfc8010.html).
- Diameter base: [RFC 6733](https://www.rfc-editor.org/rfc/rfc6733.html).
- OPC UA: [OPC Foundation Part 6, connection protocol](https://reference.opcfoundation.org/specs/OPC-10000-6/7.1).
- S7comm: [Wireshark's implementation](https://github.com/wireshark/wireshark/blob/master/epan/dissectors/packet-s7comm.c), an implementation reference, not a normative protocol specification.
- IEC 104: [Wireshark's implementation](https://github.com/wireshark/wireshark/blob/master/epan/dissectors/packet-iec104.c), likewise an implementation reference; production conformance requires the applicable IEC specification.
