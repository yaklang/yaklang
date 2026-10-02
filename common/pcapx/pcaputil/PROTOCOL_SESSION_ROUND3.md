# Protocol session review, round 3 — 2026-09-18

The last reviewed head was `6fd48ac0cd`. PRs #5125, #5126 and #5127 then
added ten protocol profiles and corrected high-bit Modbus transaction admission.
This review rebases the complete PR tree onto main `397fd3a9da` and squashes its
256 branch commits. Main's YakVM fixes remain intact, with no YakVM diff.

## Reproduced integration failure

At `35bf463b62`, Essential Tests run 35238906512 failed
`TestDoHHTTP1MixedPipeline`. Broadening session decoding to `hasSession()` made
ordinary HTTP after a DoH exchange re-enter DoH validation. Restoring the
per-message HTTP admission condition passes the original full/deferred tests
with 0/1/7-byte chunks. No assertion was removed or weakened.

## New protocol assessment

These are bounded profiles, not complete implementations or `done` catalog rows.

| Protocol | Verified profile and improvements | Remaining limits / missing evidence |
|---|---|---|
| SMTP | Command/reply, STARTTLS boundary, ordered PIPELINING queue, DATA plus following command, long greeting admission; 0/1/7-byte tests and original nDPI capture | BDAT and AUTH explicitly unsupported; TLS decrypt and broader production mail coverage absent |
| IMAP | Tagged/untagged replies, literal basics including closing trailer, STARTTLS and original nDPI capture | General literal continuations, IDLE, extensions and diverse mailbox captures remain incomplete |
| POP3 | CAPA/STAT, multiline replies, STLS | SASL, APOP verification, production captures and broader pipelining remain incomplete |
| FTP | Control commands including CLNT/MLST/MLSD; vsFTPd greeting, arbitrary intermediate multiline text; 150 and 226 retain RETR association; original nDPI control capture | Data-socket association, FTPS decrypt and server-diverse captures absent |
| TNS | Connect/Accept/Refuse/Data; invalid CONNECT data offset/length rejected | Native crypto negotiation and SQL payload semantics absent |
| RADIUS | Access messages and common attributes, exact TLV and collection-budget validation, UDP replay | Authenticator verification/hidden attributes and EAP absent; replay reports per-datagram observation, not conversation association |
| DHCP | DORA fields and UDP replay, fragmented option prefix cannot prematurely complete TCP test framing | Option overload, DHCPv6, lease semantics and UDP conversation association absent |
| NTP | v4 mode 3/4 fields and UDP replay | Extensions/authentication explicitly unsupported in datagram path; NTS and persistent UDP association absent |
| CoAP | CON/NON/ACK/RST, Token-based separate response correlation in explicit sessions; UDP replay; empty-message, option-nibble and payload-marker checks | Observe, OSCORE, blockwise aggregation, retransmission lifecycle and persistent UDP association absent |
| Modbus TCP | FC 1–6/15/16, direction-aware matching including missing-request responses, quantity/byte-count/exception checks, unit/function match, high-bit IDs retained; original nDPI capture | Other function codes, RTU/ASCII, response-only ambiguous starts and diverse vendor captures absent |

## Sample correction and scope

The four RADIUS/DHCP/NTP/CoAP PCAPs previously contained TCP packets. They now
contain deterministic Ethernet/IPv4/**UDP** packets, with regenerated SHA-256
and transport provenance in `m1-manifest.json`. They are still generated test
fixtures, not real captures. The capture UDP entrypoint now invokes the bounded
packet decoders. Existing DNS/Kerberos precedence is retained. Packet fields are
available under full and deferred decoding; no fabricated cross-packet request
association is returned by this stateless path.

`TestRound3UDPReplay` checks every event, transport, protocol and deferred decode
for 1/2 workers. The other `TestRound3*` regressions cover SMTP/FTP fragmentation,
CoAP separate response and token mismatch, malformed RADIUS TLVs, Modbus duplicate
request direction and malformed PDUs, DHCP partial options and TNS offsets.
CoAP automatic discovery uses assigned RFC 7252 codes so unsupported AMQP/SSH
banners cannot become false-positive responses; established CoAP sessions still
accept unrecognized response details. Both existing fail-closed tests pass.

## Existing upstream captures, newly locked to session replay

`TestRound3UpstreamCaptureReplay` retains original bytes and the shared corpus's
source URL, license and SHA-256 provenance. Full capture replay with 1/2 workers
now locks these protocol message/byte counts and rejects any malformed event:

| Capture | Protocol events | Message bytes |
|---|---:|---:|
| nDPI SMTP | 73 | 17,955 |
| nDPI IMAP | 27 | 1,580 |
| nDPI FTP control | 35 | 1,063 |
| nDPI Modbus | 102 | 1,173 |
| tcpdump RADIUS | 4 | 519 |
| nDPI NTPv4 | 1 | 48 |
| Wireshark DHCP | 4 | 1,144 |
| nDPI CoAP | 819 | 47,512 |

These captures exposed long SMTP greeting rejection, missed vsFTPd admission,
IMAP literal-trailer rejection and Modbus unmatched responses being treated as
requests; all four paths were repaired. FTP data connections remain unclassified.
Mail/FTP phases can expose session metadata when the generic rule cannot export
the complete field tree; the replay count is not a claim of complete field coverage.
The POP3 capture also contains SASL, which now reports unsupported explicitly
instead of calling a valid SASL exchange malformed. Its authentication and later
mail download are not claimed as fully supported. The CoAP capture also exposed
a false positive on unrelated TCP traffic. Captured TCP now excludes the four
UDP-only profiles (RADIUS, DHCP, NTP, RFC 7252 CoAP); their distinct TCP variants
are not implemented. The original CoAP UDP messages are locked separately from
the unrelated, unclassified TCP streams.
The capture also contains MQTT; those messages are excluded from this CoAP count.
Wireshark needs `-d udp.port==17500,coap` for its nonstandard-port messages and
then finds the same 819 datagrams / 47,512 bytes. It reports option-number/range
warnings on five original IPv6 packets; this profile validates structural option
bounds but does not claim complete RFC option-semantic validation.

## Verification

- `go test ./common/bin-parser/... -count=1 -timeout=5m`
- `go test ./common/pcapx/pcaputil ./common/pcapx/cmd/pcap-inspect -count=1 -timeout=5m`
- `go test -race ./common/pcapx/pcaputil -count=1 -timeout=5m`
- `YAK_UPDATE_SESSION_PCAP=1 go test ./common/pcapx/pcaputil -run '^TestProtocolSessionM1PCAP$' -count=1`

The package suites, complete pcaputil race suite and fixture regeneration passed
locally. CI acceptance is the final pushed head's complete Essential Tests Gate and
Diff-Code-Check, not an earlier head's green run. No new throughput claim is
made for this correctness round.

References: [SMTP PIPELINING RFC 2920](https://www.rfc-editor.org/rfc/rfc2920.html#section-3),
[FTP RFC 959](https://www.rfc-editor.org/rfc/rfc959.html#section-4.2),
[CoAP RFC 7252](https://www.rfc-editor.org/rfc/rfc7252.html#section-5.3),
[Modbus specifications](https://www.modbus.org/modbus-specifications).
