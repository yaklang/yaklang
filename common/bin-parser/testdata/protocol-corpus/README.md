# Protocol sample corpus

Passive, local parser fixtures for the 616-item `ProtocolRoadmap`. Collection,
frame traversal and application-field decoding are separate kinds of evidence.

## Inventory

The merged inventory contains **552 captures and 58,533 packet records**, with
material mapped to **341 roadmap names**. See `reports/REPORT.md` for the
generated source and family breakdown.

PR #5022 and PR #5023 are both included. The 168 generated captures in #5023
include 96 new names and 72 regenerated existing files. They are retained in
`captures/generated-pr5023/` with separate IDs; no earlier capture is replaced.
The merge also restores four upstream originals under neutral protocol labels.
`reports/PR_MERGE_AUDIT.json` records the exact PR revisions, retained digests
and the generator-fingerprint discrepancy found in #5023. The actual script
snapshot is pinned. Reproduction is audited separately: 167 of 168 generated
files are packet-identical, while the SNTP file differs only in generated
timestamp-dependent fields recorded in `reports/PR5023_REPRODUCTION.json`.

Every file is pinned by SHA-256, its source path and a repository commit or
local recipe digest. The manifest records its actual packet count, link type
and representative full-frame bytes. A non-empty capture must match its
recorded Wireshark display filter; there is no fallback to frame 1. Required
decode-as mappings are recorded explicitly.

Evidence classes are:

- `upstream-positive`: 215 upstream protocol fixtures.
- `upstream-negative`: 16 upstream boundary fixtures.
- `generated-positive`: 232 generated positive fixtures.
- `generated-negative`: 88 retained malformed, incomplete or unsupported local fixtures.
- `generated-identification`: 1 retained fixture whose bytes support protocol identification but not a field-level validity claim.

Original local boundary inputs are retained byte-for-byte. Eighty-one
corrected or more specific companions are separate files in
`captures/generated-validated/`, with a deterministic recipe and provenance. A
source label or a Wireshark filter hit does not establish validity or complete
parser support.

## Verification contract

The 2026-09-06 scope adjustment explicitly defers ten outer-format and four
identification names; their captures and existing contracts remain unchanged.
See [active scope and acceptance order](../../ACTIVE_SCOPE.md) for the exact list.
The sampled Cassandra and Memcached fields and protocol-specific checks are now
complete within their documented limits. The first performance, bandwidth and
aid/AI capability assessment is available in the [assessment report](../../PERFORMANCE_ASSESSMENT.md);
implementation has not expanded to other protocols. Historical full-suite results
below or in the main README do not validate the newly added code.

The integrity and artifact-inventory tests read **every capture and every
record**, verify digests, counts, link types and representative bytes, and
reject missing, unreferenced or mismatched artifacts.

The integrity and all-packet envelope tests were rerun on 2026-09-06 after the
Cassandra and Memcached additions. The envelope test passed all **58,533 records
in 552 captures**, using 58,533 bounded bin-parser entries, in **22.455 seconds**.
It checks complete byte coverage and distinguishes normal frames, expected
rejects, truncated records and explicit capture containers. This is **not**
evidence of complete application decoding or a full-library regression run.
Current results and reproduction commands are in the assessment report above.

The validation matrix assigns every stored capture exactly one executable
disposition and intentionally fails when a capture has none or more than one.
It must not be bypassed or replaced by the envelope result. The matrix currently
registers 434 direct contracts, 14 identification fixtures, 102 expected rejects and
two structural container cases. Every one of the 341 mapped roadmap names has a
field-level, outer-format or identification disposition.
Positive parses require meaningful processed fields, exact bounded input
consumption and selected independent field values. Negative fixtures require
a working positive companion and the actual innermost error, excluding the
operator source printed in a stack trace.

At name level, **327** have field-level contracts, **10** retain outer-format
contracts and **4** retain identification contracts. These counts classify
registered evidence, not completed protocol implementations or a percentage of
Wireshark functionality. A field-level contract can cover a limited message
family while other visible message types still need implementation.

The ten outer-format names are AnyDesk, DingTalk, DoH, DoQ, DoT, HTTP/3, IMAPS,
SMTPS, T.38 and WeChat/MicroMsg. DoQ and HTTP/3 additionally have explicit
derived-stream field entries; T.38 has SDP advertisement fields, not a proven
UDPTL media positive. The SMTPS capture also contains a separately parsed
plaintext SMTP reply. Visible TLS handshakes are separate implementation work;
they must not be grouped with inaccessible protected application data.

The four identification-only names are iQIYI P2P, Tencent Games, NetEase Games
and miHoYo/HoYoverse. This does not imply that all their bytes are encrypted:
reliable private field grammars are missing. Supplemental zlib JSON, standard
KCP and TLS entries preserve their own narrow evidence without upgrading the
application labels. In particular, the observed 28-byte KCP-like header in the
miHoYo capture is not the standard 24-byte KCP header and is not silently sliced
into one. The two structural cases are an empty pcapng and one unsupported
DLT-160 capture record, not two additional unimplemented application protocols.

Whole-capture tests additionally cover flow- or record-dependent cases.
EtherCAT validation traverses every frame and datagram, including physical
and logical addresses, lengths, continuation flags, working counters and
padding. DoIP checks the complete diagnostic acknowledgement. Other dedicated
tests now check every Sampled Values ASDU, GOOSE dataset value, Profinet DCP
block, captured TPKT/COTP message, corrected RARP/AoE message, and all captured
OSPFv3 Hello, J1939, LLMNR/mDNS and corrected CLDAP fields. Nonzero variants
and truncated inputs exercise offsets and bounds beyond the original data.
The original `gen-6to4` is retained as 6in4 evidence; a separate fixture checks
the historical 2002 prefix and embedded IPv4 addresses. Additional dedicated
tests cover the existing DHCP, HTTP, IPX/RIP, USB HID, RADIUS, LLDP, RadioTap,
TLS, MMS and application-format fixtures. Unimplemented names and unverified
message variants remain incomplete even when their representative frame is
recognized.

SV and GOOSE now dispatch through bounded Ethernet, 802.1Q, single 802.1ad
and double-tagged QinQ entries. Direct entries still reject malformed messages;
nonempty failed carrier candidates retain their complete original payload,
while empty carriers reject explicitly. Differential tests compare exact Go
value types, full trees, bit spans, source edits, result isolation and reader
transactions across native scalar, cached-expression and original-expression
paths. These are bounded decoding contracts, not claims of full structured
SV/GOOSE message generation or configured channel interpretation.

The separate AARP companion contains Request, Reply and Probe records encoded
from the packed 28-byte Ethernet/AppleTalk layout in Linux v6.12. All 13 fields
and spans are checked through direct, LLC/SNAP and Ethernet-II entries, including
nonuniform addresses, malformed fields, all shorter prefixes and link trailers.
The original 26-byte PR #5023 body remains unchanged as a negative fixture; none
of the previous 40 companions was replaced. Empty carriers reject; nonempty
failed candidates preserve all raw bytes and leave no reader transaction open.
DDP, other hardware layouts and live address ownership are not implemented by
this entry.

Separate DDP, CFM and NHRP companions add eleven independently encoded records:
three extended DDP datagrams, one CCM and all seven base NHRP message types.
Every record has direct and carrier field/bit-span checks. The three original
incomplete PR #5023 captures remain byte-identical negative inputs, with a
working positive control and a specific innermost diagnostic. The previous
forty-one companions are unchanged. DDP application payloads, non-CCM CFM
opcodes, NHRP error quotations and unknown extension bodies remain explicitly
uninterpreted. Receiver-ignored CFM fields are retained, not normalized or
rejected merely for differing from transmit defaults. NHRP validation includes
packet checksums, address lengths, per-request client-count constraints and
4096-client/1024-extension positive boundaries with their next-outside rejects;
CFM likewise bounds TLV traversal to 1024 entries, including End when present.
No continuity or routing-state claim is made.

Four further companions add ten independently encoded records: three IGRP,
four DVMRP v1, one XNS IDP and two TRILL. Every stored record is compared with
independent wire bytes and field/bit-span expectations through direct and
carrier entries. The four original PR #5023 inputs remain unchanged negative
fixtures; the preceding forty-four companions retain their exact digests.
IGRP covers all three route classes within an explicit 104-vector supported
limit, with interface-dependent Interior addresses left unexpanded. XNS IDP
checks both addresses and optional checksum, including the nonzero garbage
byte required on odd wire lengths, and decodes the Echo operation/data. Other
IDP applications remain opaque. TRILL decodes the six-byte base header,
C-tagged inner frame and extended flag groups. Inner imports cover only direct
ICMP/ICMPv6 and Ethernet/IPv4 ARP. Other IP protocols, options/fragments,
tunnels, unknown extensions and nested TRILL remain whole explicit bytes.
The inner import checks version, declared length and complete actual consumption
before saving, preventing recursive tunnel bypass or partial payloads being
mislabeled as link trailers. Carrier trials leave no
partial fields on failure; IGRP/DVMRP IPv4 options are consumed before dispatch
and fragments remain whole byte sequences for caller-owned reassembly.

DVMRP covers RFC 1075's four v1 subtypes and tagged commands within 512 bytes.
Wireshark's default permissive v3 heuristic misclassifies these v1 datagrams;
its independent field check must use `-o dvmrp.strict_v3:TRUE`. With that option,
all four records are v1 and the multi-address request is decoded without an
expert error. The default heuristic's apparent success on other records is
not v1 field evidence. De-facto v3 and routing-state behavior remain outside
the entry. The direct-capture material-name matrix is now 327 field-level, 10 outer-format
and 4 identification-only names, not a claim of complete protocol support.

DECnet and TIPC add thirteen independent companion records without replacing
either original PR #5023 input or changing the preceding forty-eight companion
digests. The original DECnet routing count is two bytes with invalid format
bits; bytes outside that count cannot supply a header. The original TIPC
version, header size and message size conflict with its captured body. Both
are retained negative cases, each with a separate positive control.

DECnet follows Digital AA-X435A-TK section 10 and RFC 1376 section 4: the count,
optional padding, short/long routing headers and all seven control formats have
explicit boundaries. The five companion records contain short/long data,
padded verification, a level-one route update and an endnode hello; NSP bytes
remain uninterpreted and no session/routing-state claim is made.
Long-header destination/source IDs must use the specified HIORD prefix.
Exactly header-sized short/long inputs are accepted only as routing structures,
with `NSP Present=false` and `NSP Decoded=false`, not as complete NSP messages.
TIPC follows Linux v6.12 `net/tipc/msg.h`, `msg.c`, `socket.c` and `group.c`:
payload users 0..3 and message types 0/1/2/3/5/6/7 have type-specific field and
bit-span checks, through 60-byte headers and 66,000-byte application data.
Internal users and locally constructed group-member events are unsupported;
application bytes and additional header words remain explicit. Eight companion
records cover every supported type and a named message with extension words.
The local Wireshark version recognizes the four conventional types, but its
group/extended-header display is not a field oracle: those fields are checked
against the pinned Linux accessors and independent bit extraction instead.

Steam local discovery now validates the unchanged 48-record nDPI capture:
six complete discovery/status messages have direct and Ethernet/UDP field and
bit-span checks; forty TCP-carried records and two other UDP records retain
their separate disposition. This is not a claim that arbitrary Steam traffic
or session state is decoded. The bounded native parser uses the SteamTracking
extracted client schema pinned at `fcbb9a107a0ff5293385f24d6c774eeb5e3d4c84`
and Wireshark's framing, not a Valve-published standard. Types 0..16, nested
users/gamepads, packed and unpacked scalar lists, ordered unknown fields and
matched unknown groups are supported within 65,535 bytes, 4,096 shared
fields/elements and 16 nesting levels. Required fields and wire/length bounds
are checked; unknown message types retain generic fields, strings require
UTF-8, uint32 overflow is rejected, and encrypted bytes remain explicit.
Structured generation is unsupported. Scalar output has an exact-expression
fast path checked against the normal expression evaluator without value caches.

SliMP3 adds five independent legacy discovery, infrared, display and MPEG
layouts. Its original five-byte PR #5023 input remains a truncated negative:
the original Slim Devices/SlimServer header is eighteen bytes. The entry
retains the receiver's 1,500-byte limit, explicit ambiguous-direction fields
and opaque media bytes. TDMoE adds three independent official DAHDI receiver
layouts, including five channels with two signaling words. The receiver's
formula is `8 + (signaling ? ceil(channels/4)*2 : 0) + channels*8` bytes,
with 1..255 channels; Wireshark's different non-multiple-of-four signaling
display is not used as the length oracle. Original zero-sample input remains
a negative. Both carriers preserve the whole original payload on a failed
trial. The preceding fifty companion captures remain byte-identical.

NAT-T, MACSec and WSMP add twenty-two independent field vectors in three
companions. The preceding fifty-two companion captures remain byte-identical.
All three original PR #5023 inputs remain unchanged negatives: an IKE length
of zero after the non-ESP marker, a zero MACSec short length without the required
48 data bytes, and an unsupported legacy WSMP element identifier.

NAT-T follows [RFC 3948](https://www.rfc-editor.org/rfc/rfc3948.html): UDP 4500
dispatch now distinguishes a one-byte keepalive, nonzero ESP SPI and four-zero-byte
IKE marker. The marked IKEv1/v2 clear header and complete ordered payload chain
are checked separately from raw algorithm-dependent data. Notifications,
key-exchange group fields and [RFC 7383](https://www.rfc-editor.org/rfc/rfc7383.html)
fragment counters are exposed, while unknown payload layouts, encrypted contents,
negotiated algorithms, message exchanges and fragment reassembly remain explicit
unimplemented semantics. All ten records traverse direct, carrier and Ethernet
paths. Message lengths exclude the marker; v2 nonce lengths, fragment ordering
and v1 end-of-message alignment are checked without silently discarding bytes.

MACSec follows the clear tag layout in pinned Linux v6.12 and Wireshark code.
TCI/AN/SL/PN bits, optional SCI and data/ICV/trailer intervals are checked for all
four records. The default 16-byte ICV is profile context, not an on-wire field;
supported alternate ICV lengths and trailing bytes require explicit caller
configuration. The companion ICV bytes are arbitrary and unverified. No implicit
SCI, upper XPN bits, decryption or authentication outcome is inferred; only
unchanged clear text exposes an inner EtherType.
Records 1 and 3 deliberately retain arbitrary clear bytes after IPv4/IPv6
EtherTypes, so Wireshark reports an invalid inner IP version. They are MACSec
layout vectors, not valid inner IP packets or verified whole received frames;
the inner decoder errors are not counted as MACSec field validation.

WSMP covers v2 legacy WAVE elements and v3 subtype zero, variable PSIDs,
network/transport extensions and TPID 0..3. The extra TPID layouts are checked
against ISO16460:2021 sections 5.3--5.5, whose TPID mapping is wire-equivalent to
IEEE1609.3, rather than inferred from the pinned Wireshark TPID-zero implementation.
All eight records retain their exact application bytes. In particular, PSID
0x20 with `abc` is a WSMP layout vector, not a valid message for that PSID's
assigned application; an inner Wireshark decode error is not hidden by choosing
an unassigned PSID. Unsupported subtypes/extensions and application semantics
are not counted as implemented merely because the transport fields are parsed.

Two further companions contain ten independent FCoE/FC and AllJoyn records;
the preceding fifty-five companion digests are unchanged. The two original
FCoE and Fibre Channel captures keep their separate provenance even though
their 54-byte frames are identical. Each contains only eighteen inner FC bytes,
not a complete 24-byte header; CRC/EOF bytes cannot supply the missing fields.
Both remain explicit negatives. The first FCoE companion record supplies the
separate, bounded Fibre Channel field contract without duplicating its capture.

Modern FCoE follows the pinned [Linux FC-BB-5 header and trailer layout](https://github.com/torvalds/linux/blob/adc218676eef25575469234709c2d87185ca223a/include/scsi/fc/fc_fcoe.h):
fourteen header bytes, a word-aligned FC region and eight trailer bytes, with
IEEE CRC32 verified over the entire FC region using the existing Go standard
library. Its delimiter profile follows FC-BB-5 Rev 2.00 section 7.7, tables
22 and 23 ([same-revision full-text mirror](https://www.scribd.com/document/68578775/Fibre-Channel-Backbone)):
five SOF codes and four EOF codes, not the wider generic FC enumerations or
the different early Rev 1.01 draft. An external four-byte Ethernet FCS requires explicit caller context
and is retained but not verified. The FC rule exposes the fixed header, one
supported VFT and Network Header; unknown optional and application content
is not silently decoded. Unsupported optional-header layouts reject at the
direct entries, with complete raw carrier fallback; application bodies stay
bytes. Fill is separated only with a known header layout,
END_SEQ and sufficient delimiter context: EOFt, or EOFn with a known
acknowledgement-requiring SOF as in the pinned Linux sender. Without that
context its bytes are retained as data. Known
invalid/abort EOF states are distinguished from unknown delimiter codes.
The supported 2112-byte Data Field profile is not a claim of every FC variant,
physical-link framing, FCIP/iFCP, FC-4 application or exchange-state support.
All seven companion CRCs are independently reported Good by Wireshark.
This is a mixed capture, not seven valid inner messages: the first six records
have supported complete fields; record seven declares legacy Network,
Association and 16-byte Device headers with DF_CTL 0x31 but has only 32 Data
Field bytes. It remains byte-for-byte unchanged as an unsupported-layout
negative, verified through direct rejection and complete carrier fallback.
A good CRC or the absence of a Wireshark expert warning does not validate it.
These bounded parsing entries do not support structured `GenerateBinary`;
all four direct/carrier generation calls fail explicitly. `NodeToBytes` on
the parsed tree retains the complete original bytes and is checked separately.

AllJoyn here means the legacy AJNS IP name service, not the whole message bus.
The pinned [official codec](https://github.com/alljoyn/core-alljoyn/blob/103b0833801f8e36e648a6d66313518356b0218d/alljoyn_core/router/ns/IpNsProtocol.cc)
defines v0/v1 WHO-HAS/IS-AT records, counted byte strings, GUID bytes, address/port
combinations and transport masks. The original twelve-byte input declares no
records but has eight trailing bytes; the official UDP receiver rejects that
length mismatch. It is retained unchanged as a negative. The three companions
cover v0 question, v0 dual-address answer and v1 question with separate TCP/UDP
answers. The v0 official codec puts IPv4 before IPv6; the pinned Wireshark
implementation reverses their display and is not used as that field oracle.
The 1454-byte cap is an implementation profile, not an on-wire length field.
Sender-version metadata, the v1 WHO-HAS U compatibility bit, duplicate/empty
strings and unknown transport masks are retained; daemon policy and name/GUID
syntax are not silently imposed on wire-layout decoding. Other v1 WHO-HAS bits
are named as reserved fields, not active v0 T/S/F flags. Structured
`GenerateBinary` is explicitly unsupported at both public entries; parsed
`NodeToBytes` is verified separately. Discovery state,
service availability, message-bus/ARDP and v2 mDNS semantics remain unimplemented.

IPX now has a standalone `ipx.yaml` rule for LLC and Linux SLL2 imports;
the former `application-layer/extended_protocols.yaml` entry retains its
original definition, including support for third-party imports. Definition
equivalence is checked to prevent drift. All 16,827 stored IPX records still
exercise the legacy direct entry and Ethernet carrier, with an additional standalone
field check for every record. The smaller imported rule avoids constructing
unrelated protocol nodes; it does not reduce input or field validation.

The eMule/ED2K entry now checks all 17 records of the original nDPI capture:
two Hello and six HelloAnswer payload records, including repeated messages,
plus nine transport-only records. Both distinct complete messages have exact
fixed-field, typed-tag and advertised-capability checks through the bounded
application entry and Ethernet/TCP dispatch. Variants exercise supported tag
encodings, nonuniform bit fields, coalesced messages, truncated inputs and shared
resource limits. String encodings and floating-point bit patterns remain raw;
trailing extensions are explicitly opaque. Compressed, UDP, server and transfer
messages, unimplemented tag types and connection-state validation remain outside
this partial entry. Known family markers on the two added TCP ports preserve the
entire unsupported or partial payload rather than accepting an unrelated format.

XMPP validation accounts for all 376 records of the unchanged nDPI capture:
18 ARP records, 182 transport-only records, 137 XML-bearing payload records and
39 unknown binary payload records. A separate on-wire ledger checks all 143
framing items: nine declarations, eleven stream openings, four closes and 119
complete elements. Of those elements, 81 have captured directional namespace
context; the remaining 38 and one orphan close are explicitly context-required.
Four complete IQ elements each span three TCP records. Sequence gaps are never
joined. Every event span and each complete element's namespace, ordered
attributes and mixed content are checked against the original wire input. The
representative remains frame 6; its direct-parse contract includes frame 8 to obtain the actual 138-byte
declaration and opening rather than treating a declaration alone as a message.

The bounded entry follows [RFC 6120](https://www.rfc-editor.org/rfc/rfc6120.html)
and [RFC 6121](https://www.rfc-editor.org/rfc/rfc6121.html) for the implemented
stream and core stanza fields. Callers own TCP reassembly and directional
stream-header context. The explicit `XMPPFragment` entry retains unresolved XML
fields without inventing a stanza namespace; it is never an automatic TCP
candidate. Extensions remain namespace-aware XML, mechanism-specific tokens
remain opaque after base64 decoding, and connection-state validation is not
claimed. All 39 unknown port-5223 records retain their full original payload and
spans; the port is not evidence of TLS. A narrow record-header check preserves
valid TLS candidates there without changing the shared TLS rule. Tests retain
the complete 1 MiB, 4096-event, 8192-element, depth/attribute/namespace-64 and
64 KiB stream-header positive boundaries and their next-outside rejections.

The link/network additions cover every retained LACP-marker, Slow Protocols,
HomePlug AV, VNTag, PBB and Ethernet pseudowire record, plus complete IPComp,
NVGRE, E-LMI, EOAM, MMRP, MVRP, MSRP and Mobile IP companions. The MIPv6 companion
contains two complete messages, both checked. Original invalid or mismatched
labels remain separate negative inputs: plain GRE is not NVGRE, CPI-2 compressed
data must be a complete DEFLATE stream, and registration/vector messages need
their actual required fields. Tests check decoded event values, TLV lengths,
checksums, every bounded record and nonzero/truncated variants. Application-
specific bodies, live association state and unobserved operations remain
explicitly incomplete in the partial catalog entries.

IS-IS tests decode the original no-TLV Hello as a fixed-header boundary case,
not an operational positive. A separate 53-byte Hello validates Area Addresses,
supported NLPIDs, IPv4 interface addresses and an explicitly opaque extension.
Nine core PDU header forms and bounded common TLVs have nonzero and truncation
tests; database/adjacency state and LSP checksum calculation remain out of scope.
Powerlink retains its original invalid zero-node SoC as a negative and adds
three complete SoC/PReq/PRes records with nonzero time/PDO fields. ASnd service
bodies and object mappings remain explicit bytes, not inferred fields.

DCCP preserves the original incomplete PR #5023 record as rejection material:
its Request uses the prohibited short sequence format, and its declared header
exceeds the captured datagram. A separate deterministic 13-record companion
covers all ten base packet types and the three permitted short-sequence forms.
Every record is checked through direct and Ethernet/IP entry points, including
fields, header checksums and all captured-message short prefixes. Additional
cases cover bounded options, Data Dropped bit fields, full/partial checksum
coverage, routing-header final destinations and present CRC32c options. All 40
companions reproduce byte-for-byte from the pinned recipe; the original DCCP
capture and representative hex remain unchanged. Connection/CCID negotiation,
later protocol extensions, IPv4 fragment reassembly and IPv6 jumbograms remain
outside this partial entry. Missing IP or routing context is explicit, not a
successful header-checksum verdict; an absent CRC option is not marked verified.

DICOM validation uses all six TCP records to reconstruct four association PDUs:
two 683-byte messages and two 16,509-byte messages with 123 presentation contexts
each. Public TCP dispatch parses complete messages and retains fragments as
fragments; callers supply reassembly. Seven Upper Layer PDU types, nested item
lengths, field values and every captured-message short prefix are checked;
negotiated image datasets are not decoded. WS-Discovery covers all 14 original
IPv4/IPv6 records and the six April-2005 message body forms, with independent
header/endpoint values, namespace and XML resource bounds. The 2009 version,
SOAP faults and live discovery behavior are not inferred.

An earlier implementation batch checked all three captured IPFIX messages (including
template/session state), all three NVMe/TCP messages, all five MongoDB messages
from 27 captured frames, the LPD queue command, and all ten HTTP messages from
37 frames across nine generated application captures. HTTP bodies are not
automatically evidence of the inner application's semantics. Those historical
envelope checks are not evidence for later application-specific decoders.

The current request-codec batch adds eight separate companion captures with
40 records: WinRM SOAP Identify, GSS-API/SPNEGO token structures, Docker
ContainerList, Kubernetes ListOptions, ACME flattened JWS, etcd version JSON,
S3 Signature V4 request fields and Redfish service-root options. Their explicit
entries validate their documented bounded profiles and preserve original wire
fields; decoded XML/JSON/base64 values have separate provenance, not invented
packet coordinates. No operation is executed, and no valid signature,
authorization, working endpoint or successful session is inferred.

The abbreviated ACME/GSS-API/S3 inputs, wrong etcd `/v3/version` path and two
incomplete WinRM inputs remain unchanged negative fixtures with independent
positive controls. The new application-specific tests account for every record
in their original and companion captures. This batch does not constitute a
whole-library regression result. WPAD likewise has its own bounded request
profile; it does not retrieve or execute a PAC file.

HTTP/3 and historical DoQ also have separate decrypted-stream fixtures exported
from their public captures' embedded test keys. See `derived/README.md` for
source hashes, stream/frame coordinates and the read-only reproduction check.
Original encrypted-packet contracts remain outer-format checks; derived bytes
are not counted as new captures or bin-parser decryption support.

DoQ exposes separate complete-stream query and response entries for RFC 9250
and the captured `doq-i00` draft mapping. The latter has no modern two-octet
length prefix. Its two derived streams retain all eight fragment occurrences,
including response retransmissions; the original twenty-record test also
accounts for six ICMP quotations without counting them as new stream data.
DNS headers, compressed names, questions, A records and EDNS(0) are field-level;
other RData remains opaque. Mapping version, stream identity, offsets and FIN
must be established by the caller before invoking the explicit entry.

HTTP/3 uses `HTTP3RequestStream` and `HTTP3UnidirectionalStream`. Dynamic QPACK
requires `http3QPACKEncoderStream` containing the type-2 stream prefix and
`http3QPACKMaxTableCapacity` from the observed peer SETTINGS. The default maximum
is zero, not an invented peer setting. The captured request has 25 header fields
and two DATA bytes; its 1505-byte encoder prefix spans two source packets and
contains 21 insertions. The public test uses the captured capacity of 65536.
Missing encoder state is blocked rather than a successful raw-header decode.
Compressed representation spans and encoder-source provenance remain separate
from decoded header strings. Connection-level eviction references, feedback,
transport reassembly and session state are not maintained by this entry.

T.38 signaling now has separate complete-SDP and H.248 property-group entries.
The representative contract consumes the entire 161-byte Megaco SDP Text node
at frame offset 210, including both audio and image alternatives. It no longer
uses a magic substring and treats the rest of the packet as an SDP fragment.
Media/connection/format tuples, bounded attributes and RFC 3407 capability
advertisements are visible fields, not proof of media decoding or a complete
T.38 parameter/capability set. The apparent T.38 media in this capture retains
continuous RTP headers; it is not relabeled as successful UDPTL decoding.
The focused original test classifies all 1552 records and checks 49 SDP
property-group/body fragments across 14 Megaco and 28 SIP carrier records:
27 contain T.38 advertisements, 21 are other SDP, and one is an empty descriptor.
All 27 advertisements are also imported at their original full-frame bit
positions. Six separate inline controls exercise the supported field variants
without introducing another capture or replacing original traffic.

The unchanged `ndpi-mqtt` capture is audited across all **nine records**
(SHA-256 `7d6552eb815eead6eabe0adbaa13366b4863b4c1c54dbb6fa65dec68a9738465`).
Its eight message-bearing records contain two CONNECT, one CONNACK, one
SUBSCRIBE, one SUBACK, two PUBLISH and one DISCONNECT; the other record is a
TCP SYN/ACK. Every byte of the **875 TCP payload bytes** is accounted for.
The first seven messages use level 3, while the last CONNECT on a separate
connection uses level 4. Per-direction sequence checks find no payload gaps
or overlaps among these records, but the capture is not a complete TCP
lifecycle. The two original application bodies retain **369/60 bytes**, with
independent JSON and pinned raw SHA-256 checks; their contents are never
executed and their addresses are never contacted.

`application-layer/mqtt_fields.yaml` adds `MQTT31PacketFields` and
`MQTT311PacketFields`, each with an explicit version context and a transactional
raw carrier. These are based on the
[IBM/Eurotech MQTT 3.1 specification](https://public.dhe.ibm.com/software/dw/webservices/ws-mqtt/mqtt-v3r1.html)
and [OASIS MQTT 3.1.1](https://docs.oasis-open.org/mqtt/mqtt/v3.1.1/os/mqtt-v3.1.1-os.html).
The original subscription now exposes its complete topic/QoS pair, and SUBACK
exposes its return value instead of stopping after the packet identifier.
All 14 packet-type layouts also have inline controls. Version-specific DUP,
CONNACK and SUBACK fields are distinct; legacy missing trailing User Name /
Password fields are explicitly recorded, not synthesized. Connection flags
have decoded metadata with source-byte ranges, while original User Name /
Password and application bodies remain raw wire fields.

The standard UTF-8 profile preserves BOM and optional control characters;
required topic/flag/identifier bounds and wildcard placement are checked.
Four-byte remaining-length decoding is bounded by a local 1 MiB total input
cap; accepted nonminimal encodings are explicitly marked noncanonical.
Lists are capped at 1024 items. Original fields, all proper prefixes, complete
packet tails, 32 imported bit-offset/fallback cases, empty binary/string
values, caller isolation, unread sentinels and 64 native commit/rollback
outcomes have focused tests, including the exact inclusive byte bound.
These entries do not infer a version for non-CONNECT packets, reconstruct
TCP/WebSocket, support MQTT 5, decode application bodies or establish delivery
and session outcomes. The old `mqtt.yaml` API is unchanged. No sample or
dependency was added, and no unit-test time is treated as a bandwidth result.

Both original Kerberos captures are now audited across all **41 records**:
`ndpi-kerberos-error` (two records, SHA-256
`a7cf677e50ade6ec40a9fb71fdaef9af259e5dc1ce6ecbac4e981bc106e4c4d7`)
and `ndpi-kerberos-login` (39 records, SHA-256
`bccc9bf683c262c7dacc455c73133980db6233066f8f24305213da85103496c2`).
There are **30 message-bearing records**, including two byte-identical TCP
retransmissions, and 11 control-only records. The 28 unique messages comprise
one AS-REQ, 13 TGS-REQ, 13 TGS-REP and one KRB-ERROR. Physical message counts
are 1/14/14/1 respectively; duplicates remain individually verified.

`application-layer/kerberos_fields.yaml` adds `KerberosMessageFields` and
`KerberosTCPFields`, each with a transactional raw carrier. These follow
[RFC 4120](https://www.rfc-editor.org/rfc/rfc4120),
[RFC 6113](https://www.rfc-editor.org/rfc/rfc6113) and
[RFC 6806](https://www.rfc-editor.org/rfc/rfc6806): exact DER message boundaries,
version/message tags, names, realms, signed Int32 and UInt32 fields, UTC times,
flags, request preferences, Ticket and EncryptedData envelopes, and the
observed PA-DATA layouts. All **20 PA-DATA occurrences** (14 type 1, one type 2,
four type 136 and one empty type 149), **28 Ticket envelopes** and **61
EncryptedData envelopes** are checked; deduplicated counts are 26 tickets and
55 encrypted parts. Type 1 exposes its AP-REQ; FAST request/reply expose their
outer checksum and encoded-part fields. Nonempty type-149 request values are
accepted but ignored and retained raw, not claimed as decoded data.

An independent standard-library ASN.1 oracle checks every original integer,
string, time and flag primitive, including negative algorithm identifiers and
the FAST checksum type -138. DER integer leaves preserve their exact raw
encoding; signed numeric metadata carries relative source-byte ranges instead
of padding a short negative encoding into an unsigned-looking native value.
Every message is checked at origin and full physical capture offsets, with
all proper prefixes rejected. Focused controls cover 32 imported bit-offset /
fallback cases, unread sentinels, caller backups, isolated per-call metadata,
64 native commit/rollback outcomes, malformed DER, list/depth/node limits and
the exact inclusive byte bound. AS-REP and standalone AP messages are covered
by inline layout controls, not miscounted as additional original captures.

Cipher bytes, checksum bytes, unknown PA mechanisms and unrecognized FAST
sequence extensions remain opaque. These entries do not reconstruct TCP,
decrypt, verify checksums or establish peer identity or exchange outcomes.
Local bounds are 1 MiB per message/record, 1024 list items, 65536 bytes per
string, depth 32 and 16384 DER nodes, including embedded PA bodies. Historical
`kerberos.yaml` APIs remain unchanged. No capture/dependency was added, and
unit-test durations are not throughput measurements.

The unchanged `ndpi-http2` capture is audited across all **10 Linux SLL records**,
not just its initial SETTINGS. Its SHA-256 remains
`8ca9722db9527618db1f1a35841d01e46d0d82b2e3bde022e5d24e28cff0e598`.
The 591 TCP payload bytes contain the 24-byte client preface and **12 frames**:
four SETTINGS, three WINDOW_UPDATE, two HEADERS, two DATA and one GOAWAY.
Independent sequence checks account for every payload byte in the 319-byte
client and 272-byte server directions. There are no SYN/FIN records here:
the observed application preface and initial SETTINGS are explicit fixture
anchors, not evidence of a complete TCP lifecycle.

`application-layer/http2_fields.yaml` adds `HTTP2FrameFields`,
`HTTP2InitialClientStream` and `HTTP2InitialServerStream`, each with a bounded
transactional raw carrier. The strict single-frame entry follows
[RFC 9113](https://www.rfc-editor.org/rfc/rfc9113): 24-bit length, reserved/31-bit
stream fields, DATA, HEADERS with optional priority, PRIORITY, RST_STREAM,
SETTINGS pairs and ACK, PUSH_PROMISE, PING, GOAWAY, WINDOW_UPDATE,
CONTINUATION and padding. Unknown frame bodies are explicitly opaque.
Exact-frame parsing leaves compressed fragments raw; caller-selected initial
ordered directions also decode [HPACK](https://www.rfc-editor.org/rfc/rfc7541)
using the existing `golang.org/x/net` dependency and a local empty/default-4096
dictionary. Same-stream CONTINUATION adjacency and header-block completion are
checked. Table updates are checked at representation boundaries independently
of dictionary occupancy, including two leading updates with retained entries.

All **nine request and five response headers**, including Huffman strings and
dynamic references, match pinned original dissector values. The two DATA
fields retain their exact **135/86 bytes** of JSON; their URLs are never
contacted. Decompressed names/values are metadata with compressed-block byte
ranges and prior-block counts, not fabricated wire-span leaves. Original
frame fields are checked against an independent Go HTTP/2 framer, at both
message origin and full physical capture positions. RFC HPACK vectors also
check cross-block dictionary references and every split of a compressed
literal across HEADERS/CONTINUATION. All original frame prefixes, malformed
lengths/settings/padding, table reset/eviction, Huffman/integer boundaries,
48 imported bit-offset/fallback cases, caller isolation, unread sentinels and
96 native commit/rollback outcomes have focused regression coverage.

These initial-direction entries do not accept an arbitrary midstream table
snapshot, apply reverse-direction settings, reconstruct TCP/TLS, validate HTTP
semantics/flow control, or establish a connection outcome. Local bounds are
1 MiB of wire and expanded header accounting, 1024 frames/settings, 4096
headers and 65536 bytes per expanded name/value; these are not protocol
maxima or measured throughput. Historical `http2.yaml` APIs are unchanged.
The three supplemental profile records register bounded individual inputs;
their initial SETTINGS snapshots alone do not demonstrate header decoding.
The dedicated all-record test verifies the two complete captured directions.

The unchanged `ndpi-ssh` capture is now audited across all **322** records,
not only its representative identification line. Independent TCP sequence
traversal starts at each observed SYN, checks overlap/gaps and finds six
identification lines plus **20 initial plaintext binary packets**. Client-first
intersection of the actual KEXINIT lists selects one DH group exchange session
(eight packets), one fixed group14 DH session (six), and one nistp256 ECDH
session on port 8000 (six). The delayed client identification in frame 265 and
two coalesced packets in each of frames 16 and 270 are retained. Dissector
display labels are not substituted for the observed stream state.

`application-layer/ssh_plaintext.yaml` adds `SSHPlaintextPacket`,
`SSHPlaintextDH`, `SSHPlaintextDHGEX` and `SSHPlaintextECDHP256`, each with a
transactional raw carrier. These follow [RFC 4253](https://www.rfc-editor.org/rfc/rfc4253),
[RFC 4419](https://datatracker.ietf.org/doc/html/rfc4419),
[RFC 5656](https://www.rfc-editor.org/rfc/rfc5656) and
[RFC 8332](https://www.rfc-editor.org/rfc/rfc8332): exact initial no-MAC packet
length, minimum padding/alignment, cookie, ten name-lists, NEWKEYS, request/group
parameters, canonical nonnegative mpints, encoded P-256 coordinates, nested RSA
exponent/modulus and signature fields. Empty language lists remain real
zero-width results. Unknown key/signature formats and method-specific messages
without a selected exchange profile remain explicitly opaque. Historical
`ssh.yaml` APIs are unchanged. The 35000-byte total limit is a local resource
bound, not an asserted protocol maximum.

All 28,527 physical TCP payload bytes are accounted for: **171** identification,
**8,760** initial packet, and **19,596** protected-suffix bytes; 135 records have
no payload. Traversal stops at NEWKEYS separately in each direction. The public
parser itself performs no TCP reassembly or negotiation and never establishes
keys, validates signatures/points/identities or infers handshake completion.
Each original clear packet is checked at packet origin and physical frame bit
positions, with raw hashes, numeric/string values, contiguous field coverage
and RSA-format checks using the existing Go SSH package. All original packet
prefixes, malformed lengths, name-list boundaries, eight imported bit offsets,
raw fallback, caller isolation and 128 native transaction outcomes are covered
by focused tests. No capture or dependency was added, and no throughput claim
is derived from unit-test timings.

The mixed `ndpi-wechat` capture supplies additional protocols independently of
its representative TLS SNI. `GQUIC35ClientPacket` and `GQUIC35ServerPacket`
decode all 36 Q035 public headers with explicit caller-supplied direction and
version. `GQUIC35ClientHello` checks four complete stream-one CHLO messages,
including the legacy low-96-bit FNV checksum, ordered tag table and bounded
field layouts. Packet numbers are truncated wire values, not reconstructed
sequence numbers. The other 32 bodies remain opaque. These entries follow the
fixed [Chromium 57 wire implementation](https://raw.githubusercontent.com/chromium/chromium/57.0.2987.133/net/quic/core/quic_framer.cc),
not IETF QUIC or a private WeChat message format; they neither decrypt protected
payloads nor authenticate peers or infer successful handshakes.

The same four original CHLOs now expand eight additional observed tag layouts
(32 original occurrences): SMHL uint32, CTIM/XLCT uint64, CCS/CCRT uint64 hash
vectors, NONC fields, the fixed-width NONP bytes, and the empty CSCT request
marker. Integer/hash values are little-endian except NONC's four-byte timestamp,
which is big-endian. Its following eight orbit bytes and twenty random bytes
follow the pinned client's [FillClientHello](https://raw.githubusercontent.com/chromium/chromium/57.0.2987.133/net/quic/core/crypto/quic_crypto_client_config.cc)
and [GenerateNonce](https://raw.githubusercontent.com/chromium/chromium/57.0.2987.133/net/quic/core/crypto/crypto_utils.cc)
layout. This does not validate clocks, orbit correspondence, randomness, hash
matches, cached certificate availability or negotiated capabilities. NONP is a
different 32-byte value with no inferred timestamp. CTIM remains unsigned seconds,
not an unchecked calendar-time conversion. The common/cached hash vectors retain
order, duplicates and zero-length results; hash presence does not supply the
missing certificate bytes.

An empty CSCT in a client CHLO requests timestamp information; it is not a
serialized SCT list. Nonempty unknown CSCT values stay raw. PUBS in a CHLO is a
single public value, not the different SCFG u24-length-prefixed list. Tests
explicitly prevent fabricated PUBS subfields. Tokens, SCID, CETV, unknown tags
and all 32 other packet bodies remain opaque; these fields do not provide keys
or permission to infer the protected payload's format.

All 1672 capture records remain accounted for, including all 36 Q035 packets.
Independent byte-order/vector checks cover every added field in all four CHLOs
both at packet origin and original physical frame offsets. Inline controls cover
eight bit offsets, mixed byte orders, large unsigned values, every prefix of a
typed CHLO, malformed widths, empty hash vectors, opaque nonempty CSCT, unchanged
packet-only parsing and exact raw recovery. Native checks cover all fixed-field
lengths, vector lengths through the 1452-byte packet ceiling and 32 typed-message
transaction outcomes. The protocol/result/registration-focused run passed
(public parser 0.998 s, stream parser 1.143 s). These durations are not throughput
claims. No new captures/dependencies or whole-library/race/benchmark runs were
introduced for this batch.

`BrowserMailslotDatagram` decodes the same capture's three unfragmented NetBIOS
direct-group datagrams, SMB mailslot envelopes and domain/local-master Browser
announcements. All three retain their original `DataOffset=86`, which is not
four-byte aligned. The receiver-layout entry marks this explicitly; the separate
`BrowserMailslotStrictDatagram` rejects that alignment and accepts independent
aligned inline controls. Strict here means the additional mailslot alignment
check, not complete sender conformance. Reserved and receiver-ignored fields
are preserved, and advertised names do not prove endpoint identity or browser
state. Neither protocol adds a default port dispatcher or another capture.

Supplemental per-frame contracts keep these mixed-capture entries discoverable
without relabeling the entire source, modifying the fixed roadmap, or upgrading
the representative TLS contract. Their focused whole-capture tests additionally
check every relevant original record and exact imported field positions.

The four-record `ndpi-smtps` capture also has a plaintext counterpart: frame 3
is a TLS ClientHello, but frame 4 contains a complete 179-byte, three-line SMTP
reply. `SMTPReply` checks all its original lines and code/continuation/terminal
boundaries, following [RFC 5321 section 4.2](https://www.rfc-editor.org/rfc/rfc5321.html#section-4.2)
with receiver-compatible empty final text. The old single-line SMTP API remains
unchanged. This reply is not relabeled as encrypted data or successful SMTPS;
reply-line syntax alone does not validate greeting-specific grammar, command
semantics, a connection or mail delivery. The original representative TLS
contract and capture digest remain unchanged.

The separate `ndpi-smtp.pcap` now has an all-record audit, not just its initial
reply: **95 original records** contain **35 commands, 37 replies, 11 DATA
segments and 12 transport-only records**. The command set is EHLO, HELO, MAIL,
30 RCPTs, DATA and QUIT. All 17,955 application TCP bytes are accounted for in
both directions with contiguous sequence checks. Frame 2's nonzero link tail
and the other original padding are retained outside TCP payload boundaries.

`application-layer/smtp_fields.yaml` adds exact `SMTPCommandFields` and
`SMTPDataFields` entries, each with a transactional raw carrier, following
[RFC 5321 command and DATA layout](https://www.rfc-editor.org/rfc/rfc5321.html)
and [RFC 5322 header layout and unfolding](https://www.rfc-editor.org/rfc/rfc5322.html).
Commands expose mailbox local/domain fields, source routes, IPv4/IPv6 literals,
parameters and original separators; command keywords are case-insensitive and
trailing whitespace is receiver-compatible. Other literal tags and extension
commands retain raw fallback; extension parameters are not negotiated semantics.
Base command/path/local-part/domain limits are explicit and checked before
reading where the caller boundary suffices.

The original DATA spans frames 76--81, 83--86 and 88: **15,267 wire bytes**,
including the final dot line, with SHA-256
`e7b1814c258e5281fbbb862998081600ac2487c875c5f44d0c0fb23fd3880006`.
Its five header fields (Received, Date, To, Subject and Message-Id) occupy
40 physical lines with 35 folds. The body has **87 lines / 5,345 bytes**;
all field fragments map back to original frame offsets. Unfolded values have
per-fragment provenance instead of invented contiguous decoded-string spans.
Original headers/body are cross-checked with independent Go mail/textproto
readers; every proper prefix of every original command/reply and the complete
DATA is rejected by the corresponding exact decoder.

DATA keeps original transparency dots, applies receiver dot removal and marks
noncanonical sender stuffing. It has explicit local limits of 1 MiB, 8,192
content lines and 1,024 header fields, with 1,000 decoded octets per text line.
Tests exercise inclusive/exceeded limits, empty/folded headers, controls,
dot transparency, cache isolation, 64 native bridge transactions and 32 public
carrier outcomes across all eight bit offsets. This is a seven-bit layout
profile, not negotiated 8BITMIME/BINARYMIME/SMTPUTF8, MIME decoding, structured
header grammar or full message conformance. The original lacks a From header;
none is fabricated. Test-only contiguous segment reconstruction does not add
production TCP reassembly or prove session/delivery outcome. No new captures,
dependencies, whole-library tests or performance claims are added by this batch.

`ndpi-pop3.pcap` has a separate audit of **all 144 original records / six TCP
conversations**. Its 82 payload records contain 27 commands, three encoded client
continuations, 19 generic statuses, three server challenges, five CAPA lists,
two LISTs, two UIDLs, one STAT and 20 physical segments comprising four complete
message responses. The other 62 records are transport-only. This accounts for
**22,700 TCP application bytes / 66 complete parser units**. Twelve original
link tails totaling 176 bytes are excluded using IPv4/TCP lengths, not guessed
from the capture record size; frame 107's five nonzero trailing bytes are retained.

`application-layer/pop3_fields.yaml` supplies eleven exact entries plus raw
carriers: command, generic status, STAT, multiline LIST/UIDL/CAPA, message,
server challenge, client continuation and one-message LIST/UIDL. Response context
is selected explicitly, following [RFC 1939](https://www.rfc-editor.org/rfc/rfc1939.html),
[RFC 2449](https://www.rfc-editor.org/rfc/rfc2449.html) and
[RFC 5034](https://www.rfc-editor.org/rfc/rfc5034.html); numeric response text
never silently changes that context. The three original mechanism-less AUTH
probes are preserved as legacy layouts, not declared modern valid commands.
Encoded mechanism values remain raw; only syntax and decoded byte count are
checked, without publishing their decoded contents or inferring identity/outcome.

The four message responses begin at frames 20, 113, 121 and 132. Their dot blocks
are **1,479 / 5,569 / 8,416 / 5,217 bytes**, excluding each five-byte status prefix.
Both 7-bit text and quoted-printable multipart/alternative messages are decoded
with [MIME transfer rules](https://www.rfc-editor.org/rfc/rfc2045.html) and
[multipart boundary rules](https://www.rfc-editor.org/rfc/rfc2046.html).
They contain **8 MIME entities, 108 header fields and 6 decoded bodies totaling
10,426 bytes**. Each decoded body has its own pinned SHA-256; independent standard
mail/multipart readers select the same content. Every physical field and each
encoded MIME range maps back to the unchanged original frame bytes, including
header folds, removed transparency dots and cross-segment fragments. This is
test-only contiguous reconstruction, not production TCP reassembly.

MIME bodies are derived metadata, not fabricated wire leaves. The physical tree
retains original headers, body lines and delimiters. MIME processing supports
identity, quoted-printable and base64 transfer values, nested multipart and
identity-encoded message/rfc822; it does not validate sender conformance.
Unknown transfer encodings keep explicit opaque regions. Malformed MIME yields
an explicit diagnostic without losing the valid POP3 transport tree or publishing
a partial MIME result. Only valid US-ASCII/UTF-8 text gets a decoded text value;
other charsets retain bytes. HTML is never rendered, filenames are never used to
write files, and external content is never retrieved.

Command/status bounds are 255/512 bytes; continuation values have a local 64 KiB
bound. Multiline messages/lists have a local 1 MiB bound, lists 4,096 rows, and
MIME 64 parts / depth 8 / 1 MiB total decoded / 1,024 headers per entity. The
seven-bit message layout has the same line/header limits as the SMTP dot parser.
Tests reject every proper prefix of all 66 original units, check inclusive and
exceeded limits, 352 native bridge transactions and 176 public carrier outcomes
across eight bit offsets, zero-width lists, original-byte recovery, sentinels,
cache isolation and caller-selected response context. Old POP3 behavior is
unchanged; these profiles do not prove mailbox state, negotiated features or
successful sessions, and add no dependencies or throughput claims.

`ndpi-imap.pcap` now has a matching **33-record audit**: six commands, twelve
server payload records and fifteen transport-only records, accounting for
**1,580 TCP application bytes** with contiguous sequence checks. The server
records contain **19 complete responses**, including coalesced status/list rows
and a FETCH split across frames 30 and 32. The historical `imap.yaml` remains
unchanged; `application-layer/imap_fields.yaml` adds exact `IMAPCommandFields`,
`IMAPResponseFields` and bounded `IMAPResponseBlockFields` plus raw carriers.
A block is a sequence of complete responses, not a completed transaction.

The profiles follow [IMAP4rev1 RFC 3501](https://www.rfc-editor.org/rfc/rfc3501.html)
for tags, command/direction context, quoted escapes, counted literals, sequence
sets, flags, capability lists, status codes and FETCH selectors/values. All six
observed commands (CAPABILITY, LOGIN, LIST, LSUB, SELECT and UID FETCH) and all
nineteen original responses have byte-exact tests. Observed UID, INTERNALDATE,
RFC822.SIZE, FLAGS and BODY[HEADER] attributes expose separate fields/metadata.
Encoded user/password values remain original wire fields, not decoded summary
metadata. Mailbox bytes are retained without modified UTF-7 conversion.

The complete **781-byte FETCH** has SHA-256
`ac45e7bb7562ea403edfff9478ea9508a75db3c0770d352bac916f190e289fe9`.
Its **666-byte header literal** has SHA-256
`5ed4c540f8af52db4475ef6514737e442bdc64f410b9123d354631543e581c09`,
with **18 headers / two folds**, cross-checked with the standard mail reader.
Every leaf and unfolded fragment maps back to original frame offsets. The next
27-byte tagged response is parsed separately; literal content is never split at
an embedded CRLF. HEADER-only or explicitly partial sections are not relabeled
as complete messages or MIME-decoded bodies. Invalid header-only content keeps
the literal with an explicit diagnostic. The original PERMANENTFLAGS status
omits response text; this receiver-readable layout is retained but explicitly
marked as not satisfying the response-text grammar.

Local limits are 1 MiB, 4,096 collection items/responses and 32,768 wire leaves.
The field cap also bounds escape-heavy quoted strings. Tests cover every proper
prefix of all 25 individual original messages, quoted escapes, reversed/wildcard
sequence ranges, empty lists, 32-bit numeric bounds, literal truncation/NULs,
96 native bridge transactions and 48 public carrier outcomes across eight bit
offsets, preread resource rejection, reader recovery and cache isolation.
Unsupported commands, ENVELOPE/BODYSTRUCTURE response grammar, IMAP4rev2 and
non-synchronizing literals keep raw fallback; those are not implied by this
sample-backed IMAP4rev1 profile. There is no production TCP reconstruction,
tag-to-command matching, mailbox-state inference or negotiated feature claim.

`KCPDatagram` and `KCPSegment` expose the standard 24-byte little-endian segment
layout, following the [original author's fixed implementation](https://github.com/skywind3000/kcp/blob/b1a7a2101dcbb96017681a500d6b82bbe5a88766/ikcp.c).
The existing NetEase capture's frames 19 and 20 contain a PUSH and ACK after an
uninterpreted 16-byte UDP-payload prefix. Header fields and the declared 57/0-byte
data boundaries are decoded separately; the prefix and application data remain
original bytes. Aggregates require a common observed conversation ID and exact
consumption. This explicit profile does not emulate the reference receiver's
ignored final short tail, nor implement connection state, acknowledgements,
message reassembly or vendor-specific extended headers. The capture remains
identification evidence for the private application, not a complete application
protocol implementation.

The Tencent capture uses raw-IP link type. Its frames 20 and 22 contain
338/433-byte records at full-frame offset 40: a four-byte big-endian length and
one 334/429-byte zlib member. `ZlibJSONRecord` checks the member header, complete
consumption and Adler32 before exposing the 509/736-byte JSON documents.
The profile supports a 32-KiB window and no preset dictionary, with local limits
of 64 KiB compressed, 1 MiB expanded, 128 nested containers and 65536 values plus
member names. Smaller advertised windows are explicitly unsupported because
the reused standard-library inflater does not enforce their distance limits.
Ordered members retain duplicate JSON names and numbers retain decimal text;
decoded bytes and values carry provenance separately from compressed wire spans.
Private field meanings, identity and outcomes are not inferred, and original
credential-like contents are not reproduced here. The old raw compressed-record
entry and the application classifier disposition remain unchanged.

`TLSServerHelloRecord` and `TLSHandshakeServerHello` decode complete plaintext
ordinary ServerHello messages for TLS 1.2 and TLS 1.3, following
[RFC 5246 section 7.4.1.3](https://www.rfc-editor.org/rfc/rfc5246.html#section-7.4.1.3)
and [RFC 8446 section 4.1.3](https://www.rfc-editor.org/rfc/rfc8446.html#section-4.1.3).
Sequence-aware traversal of eight unchanged captures finds 38 original records:
four AnyDesk, one DingTalk, one DoH, one DoT, two IMAPS, 28 WeChat and one NetEase;
the SMTPS-labeled capture contributes no ServerHello. This includes AnyDesk
frame 14 on TCP 80 and NetEase frame 10, independently of default protocol labels.
The focused test accounts for all source records, follows contiguous TCP bytes
from SYN sequence boundaries, and stops plaintext traversal at ChangeCipherSpec
or protected application data. It does not search protected bytes for handshake
magic. Each original hello is also checked at its exact full-frame bit position.
Known extension layouts are named fields; unknown extension values, key bytes
and signatures remain unverified. Record input parses only the first complete
handshake and preserves any remaining handshake bytes explicitly as raw data.
HelloRetryRequest, live reassembly, client-offer correlation, peer identity,
certificate trust and a successful handshake remain outside this entry.
Existing TLS APIs, port dispatch, capture counts and representative application
dispositions are unchanged.

The explicit TLS 1.2 handshake entries now also cover Certificate,
CertificateRequest, CertificateVerify, ServerHelloDone and NewSessionTicket.
The same eight original captures (2080 physical records) contain 38 complete
Certificate messages with 73 opaque entries, four Requests, four Verifies,
34 Done messages and five Tickets. Thirty Certificate messages span TCP packets;
their logical input spans and per-fragment physical provenance are kept distinct.
The second IMAPS Certificate is incomplete: 2673 of 3250 handshake bytes are
available, with 577 absent. It remains a strict negative and complete raw fallback,
not a repaired positive. Empty certificate/name vectors retain an explicit
zero-width list result without a fabricated element.

Certificate layouts follow [RFC 5246 sections 7.4.2-7.4.8](https://www.rfc-editor.org/rfc/rfc5246.html#section-7.4.2);
the Request signature-pair vector is nonempty in this selected profile, with
the appendix omission corrected by [verified erratum 1585](https://www.rfc-editor.org/errata/eid1585).
Unknown numeric algorithm/type identifiers are preserved, not declared usable.
Names, certificates and signatures remain opaque: parsing does not validate DER,
trust, signatures, a transcript, request correlation, identity or sender conformance.
The [RFC 5077 section 3.3](https://www.rfc-editor.org/rfc/rfc5077.html#section-3.3)
ticket layout preserves the lifetime hint in seconds relative to receipt;
zero means unspecified, not a calculated expiry or evidence of session resumption.
All entries require caller-selected TLS 1.2 plaintext and an exact handshake
boundary; no automatic version/direction inference, live reassembly or generation
is introduced. Per-frame supplemental contracts explicitly account for trailing
records rather than silently treating a message as the rest of its capture frame.

The same traversal accounts for all 66 visible TLS 1.2 KeyExchange messages:
31 ECDHE and one DHE server message, and 31 ECDHE, one DHE and two RSA client
messages. Sixteen span TCP packets. Separate `TLS12ECDHEServerKeyExchange`,
`TLS12DHEServerKeyExchange`, `TLS12ECDHEClientKeyExchange`,
`TLS12DHEClientKeyExchange` and `TLS12RSAClientKeyExchange` entries implement
the selected vector layouts from
[RFC 5246 sections 4.7/7.4.3/7.4.7/A.7](https://www.rfc-editor.org/rfc/rfc5246.html#section-7.4.3)
and [RFC 8422 sections 5.4/5.7](https://www.rfc-editor.org/rfc/rfc8422.html#section-5.4).
Tests select the profile from each independently observed paired ServerHello
cipher, not a guess over the message bytes. Production callers must supply that
context. Only named-curve signed ECDHE and signed DHE server parameters, explicit
client public values and the length-prefixed RSA vector are included; anonymous,
implicit, explicit-curve, PSK and other unsampled layouts remain unsupported.
Unknown group/signature identifiers, point/DH bytes and ciphertext are preserved
without algorithm usability, point validation, signature verification, decryption
or successful handshake claims. The finite 262154-byte ceiling covers all four
DHE uint16 vectors at their wire maxima, including the handshake/signature headers;
the same ceiling applies before carrier fallback. Existing dispatch is unchanged.

This TLS extension batch uses only protocol-focused tests and small registration
checks. It adds no captures or dependencies and does not rerun the full library,
race suite, benchmarks or profiling. The 327/10/4 application-name matrix remains
unchanged; parsed TLS fields do not establish private application grammar.

Focused verification for these seven message types passed in both Go
packages (public parser 1.141 s, stream parser 1.237 s), including the zero-width
list result regression. Supplemental profile,
roadmap registration, contract-reference and name-disposition checks passed
(0.573 s). These durations are test-run records, not controlled performance
comparisons. Metadata regeneration verified the unchanged total of 552 captures
and 58533 records; all 533 pre-extension capture digests also match their saved
baseline. No original input was replaced by an inline boundary control.
`Result` and `NodeToMap` retain an observed empty vector as an empty list;
dormant schemas remain absent, raw terminals stay scalar, and `NodeToMap`
continues to bypass custom `out` expressions.
`TLSChangeCipherSpecRecord` additionally exposes the content type, legacy record
version, record length and value for all 72 original plaintext CCS records in the
same eight captures (2080 capture records). Counts are AnyDesk 8, DingTalk 2,
DoH 2, DoT 2, IMAPS 2 and WeChat 56; SMTPS and NetEase contribute none.
[RFC 5246 section 7.1](https://www.rfc-editor.org/rfc/rfc5246.html#section-7.1)
requires the CCS message to use the current record state, so the caller must
establish plaintext context. The four DingTalk/DoH records belong to TLS 1.3
compatibility flows, not evidence of a cipher-state transition
([RFC 8446 section 5](https://www.rfc-editor.org/rfc/rfc8446.html#section-5)).
The strict entry requires exactly six bytes before reading; the carrier requires
an explicit 1..16389-byte boundary and preserves rejected input as raw bytes.
Record versions 0x0301..0x0303 are exposed without negotiated-version inference.
Tests check every CCS field and physical span in the unchanged full frame, all
prefixes, trailing bytes, malformed fields, resource bounds, non-byte-aligned
imports, transaction rollback and per-parse isolation. A test-only sequence
oracle checks split/reordered/retransmitted records and refuses missing starts or
gaps; production still performs no TCP reassembly. The two WeChat Alert records
at frames 374 and 1114 retain all 26 opaque body bytes and cannot become CCS
fields. Frame 374 has no captured SYN/handshake and is tested by its independently
verified frame boundary, not counted as a reconstructed connection.
The final CCS/control/registration-focused run passed (public parser 0.734 s,
stream parser 0.986 s), including 32 native transaction cases. Metadata-only
regeneration retained 552 captures, 58533 records and 616 roadmap entries. No
whole-library, race, benchmark or profiling run was performed for this batch.

`X509CertificateDER` now separately expands the original certificate bytes into
the Certificate/TBSCertificate sequence, explicit or omitted version, signed
serial integer, algorithm identifiers and parameters, issuer/subject RDN
attributes, validity times, public-key info, optional unique IDs and extension
envelopes from [RFC 5280 section 4.1](https://www.rfc-editor.org/rfc/rfc5280.html#section-4.1).
All original tag/length/value bytes retain their own exact spans; decoded OIDs,
decimal serial number, version and UTC times are separately marked metadata.
Default v1 is reported without inventing a wire version field. The compact
AnyDesk frame-85 fixture is v1 with no extensions, not a v3 example.

The original-data test accounts for all 2080 records in the same eight captures:
73 complete certificates from the 38 complete TLS messages, plus one complete
certificate inside the truncated IMAPS message, giving **74 complete DER values**.
The inventory contains eight v1 and 66 v3 certificates, 71 RSA and three EC
public-key algorithm identifiers, and 518 extension envelopes across 12 OIDs.
These exact counts are asserted. The base entry does not decode extension bodies;
the separate opt-in entry below adds that syntax. Neither entry decodes
algorithm-specific public-key encodings.
The following incomplete IMAPS certificate is a strict reject with exact raw
fallback. The containing handshake is still incomplete. Standard-library
`crypto/x509` and separate ASN.1 decoding independently check every complete
certificate's serial, version, dates, TBS/name/SPKI/signature bytes, algorithm
OIDs and extension identifiers/critical flags/value bytes. Original fragments
are checked before extraction; imported offsets within reconstructed handshake
buffers are explicitly logical offsets, not fabricated physical frame offsets.

The explicit entry and carrier require a 1..1048576-byte boundary before reading.
Local limits are 32 nested levels, 16384 DER elements, 1024 extensions and 1024
serial-number bytes. Tests include DER length/tag canonicality, integer/boolean/
bit-string controls, SET ordering, optional-field/version rules, invalid dates,
all prefixes of an original certificate and inclusive resource boundaries.
This is selected DER/certificate syntax, **not** an exhaustive DER conformance,
signature, key, current-validity, trust, identity or handshake validator. Extension
OCTET STRING contents in the base entry and public-key/signature bits remain opaque; the
older TLS Certificate vector entry remains compatible with opaque certificate
representations and continues to report `DER Parsed: false`.

Non-byte-aligned imports exposed an existing composite-result defect: leaf spans
were correct, but reading a composite used integer-byte slicing. Composite raw
reads now use the same checked bit extraction as leaves, including pending
writer bits, while retaining zero-copy aligned reads and structured `Result`
shape. Eight dedicated bit-offset regressions and 32 native commit/rollback/
short-read/callback cases cover this path. The final X.509, related TLS, result
and registration-focused run passed (public parser 3.119 s, stream parser
0.919 s). These are test durations, not throughput claims; this batch adds no
captures/dependencies and runs no whole-library tests, race suite or benchmarks.

`X509CertificateDERWithExtensions` is an explicit opt-in companion to the unchanged
base entry. It expands all **518 extension envelopes across 12 OIDs** observed in
the same **74 complete original certificates**, including **10 SCT entries**:
subject/authority key identifiers, key usage, private-key usage period, subject
alternative names, basic constraints, CRL distribution points, certificate
policies and their qualifiers, extended key usage, authority information access,
Entrust version info, and signed certificate timestamp lists. Extension syntax
follows [RFC 5280](https://www.rfc-editor.org/rfc/rfc5280.html); the Entrust optional
flag field follows the pinned [Wireshark ASN.1 definition](https://github.com/wireshark/wireshark/blob/v4.4.0/epan/dissectors/asn1/x509ce/CertificateExtensions.asn).
SCTs use [RFC 6962 sections 3.2/3.3](https://www.rfc-editor.org/rfc/rfc6962.html#section-3.2):
an inner DER OCTET STRING containing TLS-length-prefixed entries, not another
ASN.1 sequence. Log IDs, unsigned millisecond timestamps, extension bytes,
algorithm identifiers and signature bytes retain their exact field spans.

The extension oracle uses `crypto/x509`, independent ASN.1 decoding and explicit
binary vector reads to check metadata and original bytes. Every processed leaf
is checked for exact coverage and original-byte correspondence. The supplemental
physical fixture is DoT frame 6, offset 149, length 1567, with 1419 following bytes
outside the certificate boundary; it has 10 extensions. The capture name is not
treated as proof of its encrypted application's contents.

Unknown extension OIDs stay raw; unknown SCT versions retain bounded opaque entry
data. OtherName values and X.400 fields have no invented type-specific schema;
unknown policy qualifiers retain generic DER. Display-text byte encodings are
preserved rather than claiming full character-set conversion. Parsed extension
syntax does not establish signature validity, log inclusion, chain validity,
identity, trust, permission semantics or a handshake outcome. URIs are fields
only: parsing does not visit them. The older TLS vector profile is unchanged.

Extension tests cover all original capture records, all prefixes of 12 inline
grammar controls, optional/unknown fields, malformed inner values, eight public
bit offsets, 32 native transaction cases, pre-read boundary checks, and shared
depth/node limits. SCT entry limits accept 1024 and reject 1025; payload-field
budgets do not inflate reported DER element counts. The extended carrier rolls
back to exact raw bytes when a known extension is malformed, while the base
profile still accepts the opaque extension envelope. This batch adds no captures
or dependencies and does not run whole-library, race, benchmark or profiling tests.

`X509CertificateDERWithPublicKey` adds a further explicit profile: the same
extension fields plus algorithm-specific SubjectPublicKeyInfo contents. For
`rsaEncryption` with NULL parameters, it decodes the inner DER sequence into
positive modulus/exponent INTEGERs, preserving their tag/length/value spans and
any required leading sign octet. The modulus bit length and decimal exponent are
metadata, not invented wire fields. This follows
[RFC 3279 section 2.3.1](https://www.rfc-editor.org/rfc/rfc3279.html#section-2.3.1).
Each encoded integer is limited to 8193 bytes before numeric conversion; the
existing DER depth/node and whole-certificate limits are shared, not reset.

For `id-ecPublicKey` with supported named-curve parameters, it expands the point
format and fixed-width coordinate bytes according to
[RFC 5480 section 2.2](https://www.rfc-editor.org/rfc/rfc5480.html#section-2.2).
P-256/P-384/P-521 widths are 32/48/66 bytes. Uncompressed points contain X and Y;
compressed points contain only the encoded X coordinate and format byte. No Y
coordinate is fabricated and no decompression or curve arithmetic is performed.
Unknown algorithms, unsupported parameters and other curve OIDs retain opaque
public-key bytes with an explicit reason and `Public Key Fields Decoded: false`.
Malformed bodies for a supported profile fail strictly and the carrier restores
the entire exact raw certificate. Older certificate/extension entries are unchanged.

The original-data oracle asserts **71 RSA and three P-256 public keys** in all
74 complete certificates, with all 2080 source records accounted for and the one
partial certificate rejected. Independent `crypto/x509` decoding checks modulus,
exponent, curve and coordinates; generic field tests check complete original-byte
coverage, while the extension oracle still checks all 518 extensions and 10 SCTs.
Native controls additionally cover compressed/uncompressed forms at all three
coordinate widths, negative/zero/nonminimal/trailing RSA values, both integer
resource boundaries, malformed point lengths/formats, unknown profiles, all
fixture prefixes, and 32 transaction cases. Public import controls cover eight
bit offsets, rollback, neighboring sentinel bytes and pre-read boundary checks.
The X.509/TLS Certificate/result/registration-focused regression passed (public
parser 13.132 s, stream parser 1.381 s); these are test durations, not a performance
comparison. No new captures, dependencies, whole-library tests, race tests or
benchmarks were added or run for this batch. Public-key mathematics, signatures,
certificate identity and trust remain explicitly unvalidated.
The same oracle pins the signature algorithm inventory: 69 SHA-256-with-RSA
and five SHA-1-with-RSA certificates. These signature octets are not an embedded
ECDSA/DSA INTEGER-pair encoding; no such fields are fabricated from them.

The final Certificate/CertificateRequest/CertificateVerify public regression,
including the `NodeToMap` custom-expression boundary, passed in 1.020 s;
the focused formatter/result/transaction regressions passed in 0.602 s.

HiSLIP validation covers all 184 records, 85 complete PDUs and 15 one-byte TCP
keepalive probes across eight directions. Complete frames distinguish transport
payload from bounded Ethernet trailer bytes without guessing FCS versus padding.
The EGD capture contributes five data messages. All 14 TRDP frames include six
PD/MD messages; independent CRC32 and byte-order checks cover each header field,
URI/session field, dataset boundary and alignment padding. Dataset-specific
schemas, instrument commands and unobserved message types remain out of scope
for these partial entries.

All 47 HL7 records are checked, including nine complete on-wire message records
and a message split across two TCP segments. Frames 10-12 reuse the same sequence
range for three different requests. They are not treated as identical retries:
the conflicting direction must fail unique reassembly, and every alternative is
parsed independently. Together with consistent directions this yields eight
distinct messages and 46 segments, with every delimited field compared. Component
and escape text is retained without interpreting application schemas.

The Checkmk capture's 98 frames contain 46 data segments. SYN and FIN prove the
boundary of the 13,758-byte report: all 338 lines, 19 ordered sections and their
columns are checked, including duplicate and empty sections. A first-frame
format marker alone is not a complete report. Individual TCP fragments remain
bytes; callers supply the complete reassembled input. Plain-text section syntax
follows the [Checkmk agent documentation](https://docs.checkmk.com/latest/en/devel_check_plugins.html).
Cached, piggyback and encrypted forms are not claimed by this entry point.

Both JSON-RPC captures are checked in full: 20 records, four messages, one
cross-packet HTTP response and three HTTP bodies. The bounded JSON-RPC 2.0 entry
validates request/notification/response/batch structure and retains all nested
values, member order and exact numeric text in typed metadata. Duplicate members,
invalid UTF-8 and unpaired surrogate escapes are rejected. Batch and nonempty-array variants follow the
[JSON-RPC 2.0 specification](https://www.jsonrpc.org/specification).
Error codes currently require an integer spelling. The legacy flat-message
field layout remains available separately; HTTP extraction and stream reassembly
must not be inferred from a single capture fragment.

The Prometheus and XML-RPC captures are checked across all nine records and
three HTTP messages. Prometheus's original representative remains frame 4's
`GET /metrics`; the validation contract selects the actual frame-5 response,
checks its Content-Length and compares both exposition lines and every field.
The XML-RPC body contains `sys.methodHelp` with no parameters; it is checked
as a request, not just XML-looking text. Re-segmentation at every byte boundary,
matching retransmissions and all shorter body prefixes are tested. A shorter
line-aligned exposition can be valid syntax, so its original HTTP length—not
an invented terminator—is used to reject incomplete capture bodies.

The bounded [Prometheus text-format](https://prometheus.io/docs/instrumenting/exposition_formats/)
entry retains HELP/TYPE metadata, label order, quoted UTF-8 names, exact numeric
text, optional millisecond timestamps, comments and blank lines. It rejects
duplicate series and labels; histogram/summary aggregate arithmetic and family
grouping are not yet validated. OpenMetrics, protobuf and compressed bodies
need separate decoding. The [XML-RPC](https://xmlrpc.com/spec.md) entry supports
requests, successful responses, faults, all original scalar types, nested
arrays and ordered struct members. It retains the basic date spelling without
assuming a timezone. Other date spellings, DTDs, namespaces, attributes and type
extensions are not claimed. Neither parser invokes methods or performs I/O;
HTTP extraction, decompression and stream reassembly remain caller-supplied.

The original NVMe/TCP data PDU has a zero data offset despite carrying data.
Strict parsing rejects it. The explicit `nvmeAllowZeroPDO` compatibility option
preserves and marks that anomaly; the source bytes are unchanged. Queue context
is required to interpret the discovery log, and fragmented transfers and
digest verification remain unsupported. IPFIX templates are scoped by exporter
session and observation domain; missing templates cannot be guessed.

The original TRDP PD messages have communication ID zero. Strict parsing rejects
them; `trdpAllowZeroCommunicationID` preserves and marks the anomaly explicitly.
EGD is accurately named GE Ethernet Global Data, not GE SRTP. These new observed
formats retain their real names without inflating or mislabeling the fixed
historical roadmap.

Continuous-message and byte-by-byte truncation tests cover delimiter wire
lengths, overlapping delimiter prefixes, required final markers, nested BSON
ends and HTTP chunk trailers. Readers must not privately prefetch the next
message. A delimiter-looking byte in the following field does not count as
consumed framing for an optional delimiter.

Explicit caller configuration also crosses imported rule boundaries: message
limits and state-table identity must survive, while rule-local root maps and
temporary runtime context must not leak into other messages. Independent TCP
extraction rejects gaps and conflicting overlap bytes even for the older
representative-frame contracts.

IAX2 validates all 50 records in its original nDPI capture: ten full frames,
40 mini frames and all eight ordered information elements, including an empty
caller-name value. The rule follows [RFC 5456](https://www.rfc-editor.org/rfc/rfc5456.html)
and the [Wireshark 4.4.8 dissector](https://gitlab.com/wireshark/wireshark/-/blob/wireshark-4.4.8/epan/dissectors/packet-iax2.c).
Additional tests cover full-video and mini-video flags, both trunk layouts,
scalar widths, packed calendar dates, repeated/empty/unknown elements and
truncated datagrams. Source captures are not edited. Media and compound IE
bytes remain encoded; the parser does not infer codecs or complete timestamps
for mini frames without peer-scoped call history. Encrypted frames require
prior decoding and are rejected when the caller marks them as encrypted.

Ether-S-Bus checks all 20 original records and ten exchanges: version/status,
memory-word and byte read/write requests, nine corresponding responses and
one ACK/NAK. Every field, all 24 returned memory words and each CRC are checked,
along with every shorter prefix and single-byte corruption. Response command,
count and address come from the original request, not the response port. The
optional `etherSBusRequest` input must come from the same endpoints, sequence
and exchange lifetime; its own length and CRC are revalidated. No global or
sequence-only request cache is used. Without a request, response data remains
explicitly opaque. Unknown command bodies are also retained as opaque bytes.
The supported layout follows the
[Wireshark 4.4.8 Ether-S-Bus dissector](https://gitlab.com/wireshark/wireshark/-/blob/wireshark-4.4.8/epan/dissectors/packet-sbus.c).

Ether-S-I/O checks all 36 original records: 34 transfer telegrams and two RIO
status telegrams. Every transfer header, destination, data byte and status
field is compared. Additional fixtures check zero through 255 transfers,
empty bodies and unequal transfer lengths, so an inner data loop cannot
silently stop the outer transfer loop. Datagram, record and status lengths
are independently enforced. Version 0 and RIO-status type 1 follow the
[Wireshark 4.4.8 Ether-S-I/O dissector](https://gitlab.com/wireshark/wireshark/-/blob/wireshark-4.4.8/epan/dissectors/packet-esio.c).
Station/application-specific transfer data remains bytes; its layout is not
guessed. Neither of these parsers transmits packets or performs device actions.

GIOP now decodes version 1.0 through 1.3 message headers and CDR envelope fields
according to [CORBA 3.2 Part 2, sections 9.3 and 9.4](https://www.omg.org/spec/CORBA/3.2/Interoperability/PDF).
The original 32 records are all traversed, including the 28-record nDPI CORBA
capture. Tests independently reconstruct the captured TCP byte streams, check
every complete Request/Reply header and its exact field spans, and retain
individual segments as bytes when they do not contain a complete PDU. Ten UDP
records contain MIOP 1.0 packets: packet number/count, endianness, unique ID,
eight-byte alignment and the complete inner GIOP ProfileAddr request are
checked. Independent MIOP vectors also cover unknown packet counts and
multi-packet fragments. The parser does not reassemble TCP or GIOP/MIOP
fragments, and it does not interpret IDL-dependent stubs or encapsulated
Profile/Context data. MIOP is an explicit entry, not an automatic UDP dispatch.

The retained PR #5023 GIOP input declares a zero-sized Request and carries four
extra bytes. It is an expected rejection, not a valid LocateRequest. A separate
24-record companion supplies six complete GIOP messages and their TCP control
records, covering all four supported minor versions and both byte orders.
The original nDPI capture also contains a nonstandard legacy ZIOP stream: its
declared original length and compressed-data layout do not follow the formal
ZIOP sequence layout. A bounded test-only inflater documents the recovered
GIOP bytes; production keeps those ZIOP segments opaque and does not claim ZIOP
support. No original capture bytes have been repaired in place.

WPAD checks all 16 records in its four original captures, including all four
HTTP requests. The explicit entry validates only default-path `/wpad.dat`
retrieval, following the [original WPAD draft](https://datatracker.ietf.org/doc/html/draft-ietf-wrec-wpad-01)
and [HTTP request-target syntax](https://www.rfc-editor.org/rfc/rfc9112.html#section-3.2).
Absolute-form requests do not prove proxy use. Missing draft-required Accept
headers are recorded as metadata, not treated as malformed HTTP. DHCP/DNS
discovery and PAC interpretation or execution are not implemented.

The original Redfish-SSDP sample uses a different service target from the
Redfish discovery profile and remains a negative fixture. A separate six-record
companion covers multicast/unicast search, response, alive and byebye messages.
The explicit entry follows [DMTF DSP0266 v1.23.1, section 12.4](https://www.dmtf.org/sites/default/files/standards/documents/DSP0266_1.23.1.html)
and validates the bodyless discovery profile and ordered header
fields; generic SSDP port dispatch remains unchanged. URL fields use bounded,
pure parsing with URI escaping, authority and port checks, without DNS or
network I/O. REST resource contents, remote UUID ownership and discovery state
are not inferred from these packets. All 57 earlier companions retain their
original SHA-256 digests.

New data follows the same requirements as existing data: immutable source,
complete inventory, explicit protocol and extraction contract, positive or
negative field-level checks, and whole-capture validation where relevant.
Classifier evidence and encrypted or opaque bodies must stay explicitly
labelled; neither is reported as decoded application content.

NCP, LAT and SEBEK add three separate companion files containing seventeen
records; the preceding fifty-nine companions and all original captures remain
unchanged. Each original and companion record is checked through its explicit
entry, including field values, result types, spans and complete bytes.

NCP supports bounded datagram connection controls, common request/reply fields
and request functions 20, 23/26 and 33. The original ten-byte connection request
is negative: the [Novell layout](https://www.novell.com/documentation/developer/ncp/ncp__enu/data/sdk333.html)
requires seven bytes and connection number FF. The independent acknowledgement
retains nonzero N/A fields as specified by the SDK, even though Wireshark labels
them as completion/status errors. Unknown request bodies and unassociated
reply bodies stay opaque. Only UDP port 524 selects the production carrier;
other ports do not acquire an NCP heuristic. TCP wrappers, burst, packet
signatures and connection state are outside this entry.

LAT supports exact bounded Run/Start/Stop virtual-circuit messages, six common
slot headers and class-one Start parameter envelopes. Its original zero-ID Run
is retained as a negative. The five new captured containers omit Ethernet
padding/FCS to supply exact LAT boundaries; this does not change minimum
on-wire Ethernet frame size. Start-message unpredictable tails and odd slot
padding are preserved. Other slot data/status remains opaque without service
state; Wireshark's class-one interpretation reports short Data_b data in record
5. Its separate nonzero-MBZ report reads the entire Attention header B0 instead
of the required low nibble zero; this parser checks the actual nibble. Neither
that record nor a successful header parse proves valid class-one data.
EtherType 6004 selects the bounded carrier;
failed messages retain their complete bytes, without guessing padding removal.
Discovery, negotiated limits and session state are unsupported.

SEBEK parses the original version-three record, a new version-two raw-data
record and a version-three type-two packed IPv4 endpoint record. Its
[fixed packed layout](https://raw.githubusercontent.com/honeynet/sebek/11bb9954c589f1a3e33c27c1d541f9d34d5d4bbc/linux-2.6/src/net.h)
supplies the field oracle; configurable Magic and ports are not fixed
identifiers. The explicit parser neither adds a UDP classifier nor infers
transport provenance, cross-record consistency, normalized timestamps or
application semantics. All twelve command-label bytes are retained.

### MySQL / MariaDB complete original-record audit

`mysql_fields.yaml` adds twelve explicit classic-protocol profiles without
changing the historical `mysql.yaml` entry. The selected profile supplies
phase, direction and capability context. First payload bytes alone cannot
distinguish commands, counters, column definitions and text rows.

The tests retain and check all three original captures independently:

- `ndpi-mysql.pcapng`: 41 records, 4271 TCP application octets, two flows.
- `gen-mariadb.pcap` and `pr5023-gen-mariadb.pcap`: four records and 102
  application octets each. Their capture hashes differ; their greeting payload
  hashes match. Neither generation substitutes for the other.
- Total: 49 physical records, 27 payload-free controls and 22 payload records.
  Ten complete clear messages contain fourteen classic packets. The TLS phase
  contains twenty records: ClientHello, ServerHello, two ChangeCipherSpec and
  sixteen protected records. Protected octets remain raw, not database rows.

Capture, application-payload and byte-span checks are pinned in
`protocol_corpus_mysql_fields_test.go`. TCP sequence continuity is checked
per direction and every physical record is accounted for. The original five
coalesced MariaDB resultset packets have sequences 1..5: column count with
`Metadata Follows`, an extended-metadata column definition, EOF, a text row,
and final EOF. The row value occupies `[64,76)` in that 85-byte message;
the column alias is `@@version_comment` and the observed value is
`Ubuntu 22.04`. These are passive observations, not executed queries.

New fields cover little-endian greeting capabilities, protocol41 client and
SSLRequest layouts, bounded plugin octets, length-encoded connection
attributes, classic commands, OK/ERR/EOF, and fresh-column text resultsets.
NULL and zero-length values remain distinct. Numeric lengths support all
length-encoded integer widths, including exact 24-bit spans. Existing
big-endian field bridges keep their default; new MySQL fields retain little
endian even under a big-endian enclosing rule.

The two resultset entries explicitly require legacy EOF terminators;
the MariaDB entry additionally requires `CACHE_METADATA` and
`EXTENDED_METADATA` context. Cached column definitions, compressed framing,
binary/prepared rows, query attributes, deprecated-EOF resultsets,
0xffffff-byte continuation packets and multi-result transactions are not
implied. Unsupported layouts have bounded whole-message raw fallback.
Limits are 1 MiB per message, 4096 columns/attributes/metadata items, and
4096 rows or total row values. No plugin response is interpreted as an
identity or successful session; row character encoding is not guessed.

The native tests check every proper prefix of every observed clear message,
nested length violations, sequence rollover/discontinuity, NULL/empty rows,
4096/4097 item limits, and 384 transactional bridge outcomes. Public tests
exercise twelve entries under both parent byte orders at every bit offset,
384 valid/fallback outcomes, pre-read limits, nested rollback, complete
byte reconstruction, parent/context relationships and cache isolation.

Sources: [MySQL classic protocol packet](https://dev.mysql.com/doc/dev/mysql-server/latest/page_protocol_basic_packets.html),
[HandshakeResponse41](https://dev.mysql.com/doc/dev/mysql-server/latest/page_protocol_connection_phase_packets_protocol_handshake_response.html),
[length-encoded integers](https://dev.mysql.com/doc/dev/mysql-server/latest/page_protocol_basic_dt_integers.html),
[MariaDB connection layout](https://mariadb.com/docs/server/reference/clientserver-protocol/1-connecting/connection),
[MariaDB resultset layout](https://mariadb.com/docs/server/reference/clientserver-protocol/4-server-response-packets/result-set-packets).

Targeted checks only (these are correctness checks, not throughput measurements):

```sh
go test ./common/bin-parser ./common/bin-parser/parser/stream_parser \
  -run '^(TestMySQLFields.*|TestProtocolCorpusMySQLFields.*|TestProtocolCorpusSupplementalProfiles)$' -count=1
```

### PostgreSQL complete original-record audit

`postgresql_fields.yaml` adds sixteen explicit, bounded protocol-3.0 profiles
while retaining the historical `postgresql.yaml` entry. Direction and phase
select the grammar: the same type byte can name different frontend/backend
messages, and `p` needs a password, SASL-initial or SASL-response profile.

All 88 physical records of `ndpi-postgresql.pcap` are checked against the
unchanged capture hash. Six TCP flows contain 35 payload records, 53
payload-free controls and 2993 application octets. Every directional sequence
and every payload hash is checked. These contain 117 complete messages:
105 typed messages and twelve startup/transport-negotiation messages.
Coalesced messages are split at their declared boundaries; no application
message in this particular capture needs cross-packet reassembly.

The typed audit includes Parse/Bind/Describe/Execute/Sync, phase-selected `p`,
method and SASL messages, ParameterStatus, BackendKeyData, ReadyForQuery,
completion indicators, RowDescription, DataRow and NoData. Five descriptions
contain 29 column definitions. Frame 25's single binary int4 value is `3`:
the row octets occupy `[76,80)` and its local column description occupies
`[31,65)` within that 98-byte payload. The parser retains both physical
evidence ranges, OID 23 and binary format 1. It only annotates rows using a
preceding description in the same bounded block, clearing context at command,
ready, error, method and NoData boundaries. A standalone row stays raw.
No cross-call portal, statement, OID registry or text encoding is assumed.

Four sampled SCRAM messages have ordered attribute fields, GS2 framing,
canonical base64 validation and bounded integer syntax. PostgreSQL's observed
ignored empty SCRAM username is retained explicitly. Iteration counts are
parsed, never executed as work factors; proof, verifier, nonce relationships
and channel binding are not verified. The two `G` responses are only observed
positive GSSENC responses, not established contexts. The two `N` SSL responses
are retained as negative indicators; no decryption or query execution occurs.

Independent controls additionally cover cancel, simple queries, other typed
completion/error/notice/notification and Copy envelopes, NULL versus empty
values, signed widths and unsupported/malformed layouts. Opaque Copy and
unselected SASL payloads remain raw. Protocol versions other than 3.0,
arbitrary binary column types, cross-block state, fragmented application
messages and complete database-session semantics are not implied. Limits
are 1 MiB per input, 4096 items per collection or messages per block, and
65536 wire leaves. Unsupported candidates have whole-boundary raw fallback.

Native tests check every proper prefix of all 117 observed messages, byte
mutations, local row-context boundaries, resource limits and 512 transactional
bridge outcomes. Public tests check sixteen entries at all eight bit offsets
under a little-endian parent (256 valid/fallback outcomes), big-endian numeric
results including signed negatives, pre-read limits, nested rollback,
byte reconstruction, parent/context/list relationships and cache isolation.

Sources: [PostgreSQL message formats](https://www.postgresql.org/docs/16/protocol-message-formats.html),
[SASL implementation notes](https://www.postgresql.org/docs/16/sasl-authentication.html),
[protocol flow](https://www.postgresql.org/docs/16/protocol-flow.html),
[SCRAM grammar](https://datatracker.ietf.org/doc/html/rfc5802), and
[PostgreSQL int4 receiver](https://raw.githubusercontent.com/postgres/postgres/REL_16_STABLE/src/backend/utils/adt/int.c).

Targeted correctness checks, not throughput measurements:

```sh
go test ./common/bin-parser ./common/bin-parser/parser/stream_parser \
  -run '^(TestPostgreSQL.*|TestProtocolCorpusPostgreSQLFields.*|TestProtocolCorpusSupplementalProfiles)$' -count=1
```

### LDAP original-generation and CLDAP boundary audit

`ldap_fields.yaml` supplies one exact LDAPv3 BindRequest profile without
changing the historical entry. Both `gen-ldap.pcap` and
`pr5023-gen-ldap.pcap` are retained with distinct capture hashes. Each contains
three TCP setup/control records and the same 14-byte BindRequest: message ID
1, version 3, empty directory name and empty simple-choice octets. All eight
physical records, TCP counters, field values, zero-width fields and byte
ranges are checked independently. A request is not a successful session.

The two separately hashed `gen-cldap` generations also contain those fourteen
bytes, but over UDP. Both remain negative under the CLDAP entry; recognizing
BindRequest syntax does not make it a connectionless search operation. The
independent `gen-cldap-valid` capture retains its 77-byte rootDSE search and
existing field audit. All eleven records across these five captures are
accounted for; originals are never replaced by the corrected companion.

Independent controls cover definite short/long BER lengths (including
nonminimal long lengths, which are not DER), numeric message IDs, UTF-8 names,
simple/SASL octets, absent versus empty optional values, and ordered control
OID/criticality/value fields. Control values are raw, not interpreted by OID.
Unknown operations, choices and sequence extensions have whole-input fallback.
DN syntax, mechanism state, control combinations, result messages and network
transport semantics are not inferred. Limits are 1 MiB, four BER length
octets and 4096 controls; exact and over-limit cases are tested.

Native checks include proper prefixes, byte mutations, BER/INTEGER/BOOLEAN
constraints and 32 transactional outcomes. Public checks add 48 valid/nested
fallback outcomes at every bit offset under a little-endian parent, complete
reconstruction, pre-read bounds, numeric widths and cache isolation.

Sources: [LDAP message and BindRequest formats](https://www.rfc-editor.org/rfc/rfc4511.html),
[LDAP numeric OID grammar](https://www.rfc-editor.org/rfc/rfc4512.html).

```sh
go test ./common/bin-parser ./common/bin-parser/parser/stream_parser \
  -run '^(TestLDAP.*|TestProtocolCorpusLDAPFields.*|TestProtocolCorpusCLDAPSearchEveryField|TestProtocolCorpusSupplementalProfiles)$' -count=1
```

### TDS complete original-message audit

`tds_fields.yaml` adds six explicit complete-message entries: SQLBatch, RPC and
response, each with a caller-selected 7.1 or 7.2 layout. These are separate from
the historical shallow `tds.yaml` entry and do not change its framing contract
or transfer its roadmap score into a claim of complete TDS support. No version
is guessed from a port, a token or a plausible first length. The capture lacks
the original connection negotiation, so selected versions describe layouts,
not independently established server versions.

Every record of the unchanged `ndpi-mssql.pcap` is accounted for: 38 records,
35 TCP payload records, three control records with separately retained Ethernet
padding, 14,142 TCP payload bytes, 30 TDS packets and 29 complete messages.
There are three SQLBatch messages, sixteen RPC messages (seventeen calls,
including a batched pair), and ten response messages. Tests check 78 parameters,
five returned values and 22 cells across seven rows, for 105 values in total.
The response column names/types, nullable lengths, signed integers, GUIDs,
bit values, date components, raw character/binary values and Unicode code units
are checked against original byte ranges and independent expected values.

Frames 26–32 supply a single 8,339-byte RPC message: the first 8,000-byte TDS
packet spans six TCP segments; a second 339-byte TDS packet ends the message.
Its `nvarchar(max)` parameter uses one PLP chunk containing 8,196 data bytes.
The value's message-relative physical spans are `[102,8000)` and `[8008,8306)`;
the intervening TDS header is not part of the value. A split leaf appears as
raw fragments in the physical tree, with its complete typed annotation and
all spans in metadata. Input order is checked in the corpus test; the parser
itself receives already ordered TCP bytes, not an implicit stream reassembler.

Frame 6 has a complete nine-byte DONE body under the 7.1 layout (four-byte row
count). Treating it as 7.2's eight-byte count would incorrectly make it appear
truncated. Conversely, 7.2 RPC/SQLBatch messages carry ALL_HEADERS; this profile
supports the sampled transaction-descriptor header, not arbitrary optional
headers. Response ROW values require fresh COLMETADATA in the same bounded
message. DONE clears that local context, and metadata never leaks to another
parse. RETURNVALUE, RETURNSTATUS and all three observed DONE token types are
decoded, including the version-dependent UserType and row-count widths.

The implementation retains exact bytes and does not execute SQL, invoke a
procedure, establish a session, convert an unknown non-Unicode code page or
invent a date's time zone. Unpaired Unicode surrogates retain code units and
`Unicode Text Valid: false`, without a synthesized replacement-character text.
Unsupported types, tokens, optional headers and later-version features have
whole-input raw fallback. There is no general TDS 7.x/8.0, MARS or complete
database-session claim. Original captures, hashes and provenance are unchanged.

Bounds are 1 MiB including all TDS headers; 4096 packets, calls, tokens,
columns per metadata token, total values and chunks per PLP value; and 65536
logical/projected leaves. Native tests cover every proper prefix of all 29
original messages, independent type/layout mutations, exact/over-limit cases,
and 192 transactional outcomes. Another 32 bridge outcomes check a rejected
individual byte-order override. Public tests cover six entries at eight bit
offsets (96 valid/fallback outcomes), mixed big-endian headers/little-endian
bodies under a little-endian parent, signed results, pre-read limits,
nested rollback, full byte reconstruction and independent cached parses.

Sources: [MS-TDS RPC grammar](https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-tds/619c43b6-9495-4a58-9e49-a4950db245b3),
[ALL_HEADERS](https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-tds/e17e54ae-0fac-48b7-b8a8-c267be297923),
[TYPE_INFO](https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-tds/cbe9c510-eae6-4b1f-9893-a098944d430a),
[COLMETADATA](https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-tds/58880b9f-381c-43b2-bf8b-0727a98c4f4c),
[RETURNVALUE](https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-tds/7091f6f6-b83d-4ed2-afeb-ba5013dfb18f),
[date/time components](https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-tds/786f5b8a-f87d-4980-9070-b9b7274c681d),
and [Microsoft PLP constants](https://github.com/microsoft/referencesource/blob/main/System.Data/System/Data/SqlClient/TdsEnums.cs).

Targeted correctness checks, not a bandwidth or throughput measurement:

```sh
go test ./common/bin-parser ./common/bin-parser/parser/stream_parser \
  -run '^(TestTDS.*|TestProtocolCorpusTDSFields.*|TestProtocolCorpusSupplementalProfiles|TestProtocolCatalogRuleFilesExist)$' -count=1
```

The final targeted run also included legacy TDS, catalog/roadmap integrity,
supplemental profiles and the MySQL/PostgreSQL/LDAP/certificate bridge
regressions: exit 0 (main package 1.435 s, native parser package 0.920 s).
These are correctness-test durations, not packet throughput. A separately
included historical `TestP0RoadmapCovered` check still rejects the existing
honest `partial` labels for LDAP, MySQL, PostgreSQL and SMB3. That broader
scope/status inconsistency remains for the coverage audit; this batch does
not change those labels or weaken the check. No full-library test, benchmark,
profile, dependency update, capture rewrite, commit or push was performed in
this batch.

### Oracle TNS complete original-record audit

Nine explicit `application-layer/tns_fields.yaml` entries cover the sampled
v315 CONNECT/ACCEPT, RESEND, 32-bit DATA service exchange, TTC protocol
request/response, native type exchange request/response and native LE64
function-118 short-CLR session parameters. The historical 16-bit `tns.yaml`
entry remains unchanged. These supplemental entries remain `partial`: they
are bounded message-layout contracts, not a complete Oracle conversation,
universal TTC implementation or automatic native-ABI detector.

The unchanged `ndpi-oracle.pcapng` has SHA-256
`842255a04c47c1dabe18d96bc5296ff705dc167ddcc6785651a14d0e27ffa79b`.
The public test checks all **20 original records**: one TCP flow, nine
transport-only records and eleven application records containing **1,382 TCP
payload bytes**. Every sequence/acknowledgement, payload boundary and control
flag is checked. Frames 4 and 8 contain identical 212-byte CONNECT messages;
frame 6 is RESEND; frame 10 is ACCEPT; frames 11/13 exchange service
subpackets; 14/16 exchange TTC versions, platform and capabilities; 17/19
exchange native type context; and frame 20 carries five session parameters.
All eleven pass the public bounded parser and whole-frame import path.

The capture switches from a 16-bit packet-length/checksum header to a 32-bit
length after ACCEPT. This is explicit caller context, never guessed from a
zero high word or the port. Both CONNECT and ACCEPT report a 32-bit SDU of
8,192 and TDU of 2,097,152; the latter is not another 8,192-byte field.
The 142-byte unquoted connect descriptor is parsed as an ordered nested
name/value tree, retaining repeated HOST keys and every delimiter/value byte.
Quoted/escaped descriptor dialects require separate profiles.

Service headers, counts, error values, typed subpackets and supervisor arrays
are decoded. The request's supervisor array is exactly `[4,1,1,2]`, including
its duplicate, while the response is `[4,1]`; no normalization is performed.
TTC protocol response fields include ten five-byte character-conversion
entries, character set 873, a 100-byte representation descriptor, national
character set 2000, forty compile-capability octets and seven runtime-capability
octets. Capability vectors retain their exact bytes; they are not presented
as a complete interpretation of every vendor capability bit. Only the
national-character-set slot is interpreted in the representation descriptor;
`Representation Descriptor Fully Decoded` remains false. Its SHA-256 is
`99e8bbd31af34133b6a780205681c1c8da28542df265d3dca42b49b616feca41`.

The native type exchange has no conversion table. Its tail contains an
eleven-byte timezone representation, a four-byte timezone-data version (18)
and, in the request, a little-endian national-character-set value (2000).
Both sampled offsets are zero. These entries explicitly require the
timezone/version layout; another capability context cannot silently reuse
them. The LE64 function-118 entry uses eight-byte pointer fields, four-byte
little-endian counts/mode/flags, and four native-alignment bytes. This narrow
ABI layout is inferred from the retained native-client bytes and the field
order in Oracle's thin driver; it is not asserted for arbitrary OCI versions
or platforms. Conversion capacities differ from CLR byte lengths: user
capacity 9 carries three bytes (`sys`), and the five parameter keys/values
likewise have threefold capacities. The parser does not require equality,
but rejects an actual value exceeding its capacity. Values, duplicates and
wire-level parameter names are preserved; nothing is applied or executed.

Oracle's primary implementation supplies the
[CONNECT/ACCEPT layout and length transition](https://github.com/oracle/python-oracledb/blob/eee31e33f8324a0ee851f3f8a90871f7c3d66b64/src/oracledb/impl/thin/messages/connect.pyx),
[service TLVs](https://github.com/oracle/python-oracledb/blob/eee31e33f8324a0ee851f3f8a90871f7c3d66b64/src/oracledb/impl/thin/messages/network_services.pyx),
[protocol and descriptor-slot reader](https://github.com/oracle/python-oracledb/blob/eee31e33f8324a0ee851f3f8a90871f7c3d66b64/src/oracledb/impl/thin/messages/protocol.pyx)
and [session-parameter field order](https://github.com/oracle/python-oracledb/blob/eee31e33f8324a0ee851f3f8a90871f7c3d66b64/src/oracledb/impl/thin/messages/auth.pyx).
The timezone layout and capability conditions are independently present in
the driver's author-maintained
[go-ora type exchange](https://github.com/sijms/go-ora/blob/b6e6835f760b3931ca80bff8559ccfe83b8eca72/v2/data_type_nego.go).
These are grammar references, not added runtime dependencies.

The resource profile bounds a whole packet to 1 MiB, 4,096 nested/list items,
65,536 leaves and 33 descriptor nesting levels. All proper prefixes of all
eleven original messages fail without publishing fields. Independent
fixtures vary service arrays, character-conversion counts, descriptor keys,
platform strings, pointer values, duplicate parameters, capacities, empty
values and nonzero timezones. Byte mutations, exact/over-limit cases, all
72 wrong-profile combinations, 288 native transaction outcomes and 144
public valid/fallback bit-offset outcomes check field coverage and rollback.
Public tests also check complete reconstruction, mixed endian numeric values,
metadata byte ranges, no-boundary/oversize rejection before reader access,
parser-only behavior and isolated cached parses. Unsupported variants retain
the entire supplied packet in the carrier, not a partially interpreted tree.
Checksums, conversation state and native representation negotiation are not
validated; no connection, replay or query execution is performed.

The targeted run, including TNS, adjacent TDS, supplemental-profile/catalog/
roadmap integrity and MySQL/PostgreSQL/LDAP/certificate bridge regressions,
passed (main package 1.461 s; native parser package 1.727 s). These are test
durations, **not throughput measurements**. No full-library run, benchmark,
profile, dependency update, capture rewrite, commit or push was performed.

### Memcached complete original-record audit (2026-09-06)

The new `application-layer/memcached_fields.yaml` entries decode the sampled
text `stats` request, the complete `STAT` response ending in `END`, and binary
GET requests. The historical single-line / binary envelope rule is unchanged.
Three public entries and their transactional carriers retain partial status:
other commands, responses, UDP, automatic TCP reassembly and session inference
are not implied. Nothing connects to a server or executes a command.

The grammar follows the [Memcached 1.4.15 text specification](https://github.com/memcached/memcached/blob/1.4.15/doc/protocol.txt)
and [official binary protocol](https://docs.memcached.org/protocols/binary/).
Statistics remain an ordered list with original name/value text and byte ranges;
duplicate names survive. Optional unsigned decimal annotations are exact lexical
values, not inferred metric units. Versions, decimal CPU-time strings and unknown
values retain their spelling. Binary GET checks the request magic/opcode,
datatype, absent extras/value, 1..250-byte key and exact body length; vbucket,
opaque and CAS are separate network-order fields, not response status. Keys
remain raw bytes. Message size is capped at 1 MiB and statistics at 4,096 items.

All three captures remain unchanged and were independently verified:

| Capture | SHA-256 | Records | Application messages |
|---|---|---:|---|
| `ndpi/ndpi-memcached.cap` | `3a74dd7c9f97e5a7ff75d201014accb76b673d5e13579ddf5faa8bf3844d17bd` | 10 | frame 4: 7-byte request; frame 6: 1,028-byte response with all 48 statistics |
| `generated-local/gen-memcache-bin.pcap` | `8ef5179f84123ec6a6fe435fd11a5b3291b5c6f60c8c441edf7980bead85e514` | 4 | frame 4: 27-byte GET, key `foo`, opaque 1 |
| `generated-pr5023/pr5023-gen-memcache-bin.pcap` | `a08ee0943de03aad2624f017cf876a2091ea07e776c67f90dba70184042479e2` | 4 | frame 4: same GET payload, independently retained capture |

The audit checks all 18 TCP records (14 controls and four messages, 1,089
application bytes), sequence/acknowledgement/flags, IP-derived boundaries,
every original statistic value, public field trees, metadata ranges and
whole-frame embedding. Tests include all 1,062 proper prefixes of the three
distinct messages, wrong profiles, trailing messages, nonzero binary fields,
raw keys, duplicate/empty lists, integer overflow preservation, 1 MiB and
4,096-item exact/over limits, 96 native transaction outcomes, 48 public
bit-offset carrier cases, reader prechecks, unsupported generation and parallel
cache isolation. Rejected carriers preserve the complete raw message without
partially publishing fields.

The final joint targeted command passed (main package 2.042 s, stream parser
0.946 s):

```sh
go test ./common/bin-parser ./common/bin-parser/parser/stream_parser -run '^(TestMemcached.*|TestProtocolCorpusMemcachedFields.*|TestCassandra.*|TestProtocolCorpusCassandraFields.*|TestProtocolCorpusSupplementalProfiles|TestProtocolCatalogRuleFilesExist|TestProtocolRoadmapIntegrity|TestProtocolCorpusContractReferencesExist|TestTNS.*|TestProtocolCorpusTNSFields.*|TestTDSFieldsBridgeTransactions|TestTLSCertificate.*)$' -count=1 -timeout=3m
```

No capture or dependency was added or changed. The implementation cutoff in
[active scope](../../ACTIVE_SCOPE.md) is now reached; performance, bandwidth
and aid/AI capability evaluation follows, with the 14 named protocols deferred.

### Cassandra CQL and internode complete original-record audit

Seven explicit `application-layer/cassandra_fields.yaml` entries decode
OPTIONS, STARTUP and SUPPORTED for CQL v4 and the initial unframed v5 exchange,
plus a separate modern Cassandra Internode Initiate. The historical
`CassandraCQL` envelope entry is unchanged; the supplemental field entries
remain `partial` and do not promote the historical roadmap or scores.

The retained `ndpi-cassandra.pcap` has SHA-256
`5992c6bbbd1f84fafb84052520e710adec4144fbf5f71b75a8a6d74bad57d155`.
All **20 records** are checked: three TCP flows, thirteen transport-only
records, six CQL messages and one internode message, totaling **332 TCP payload
bytes**. Tests assert all TCP sequence/acknowledgement numbers and control
flags, full IP/TCP payload boundaries, exact parser consumption and original
whole-frame reconstruction. Frames 4/6/8 carry v4 OPTIONS/SUPPORTED/STARTUP;
12/14/16 carry the corresponding v5 exchange; and 20 carries Initiate.
None of the flows contains a completed application session in this capture.

All nine string-map/multimap entries and fourteen option values in the four
nonempty CQL bodies are decoded, including the compression choices, CQL
versions, advertised native-protocol versions and driver name/version.
Counts and byte lengths constrain every nested value. UTF-8, ordering,
duplicate keys and empty strings/lists are preserved, with raw-byte provenance
alongside text. Strings are not NUL-terminated and are never interpreted as
commands. The presence of the mandatory CQL_VERSION startup key is exposed
separately; a structurally decoded map without that key is not claimed to be
a valid negotiation (`Negotiation Validated: false`).

The v5 initial messages really use the nine-byte envelope: its additional
CRC24/CRC32 framing begins after READY or AUTHENTICATE, neither of which is in
this capture. That phase is explicit caller context, not inferred from a port
or a first byte. Flagged/compressed envelopes, later v5 framing, queries,
results and session state require separate implementations. The primary
[CQL v4 specification](https://github.com/apache/cassandra/blob/45fa31beb44c12b01d1e6a8cca1565650f5b1c98/doc/native_protocol_v4.spec)
defines the map/list/header fields, and the
[v5 specification, section 2.3.1](https://github.com/apache/cassandra/blob/45fa31beb44c12b01d1e6a8cca1565650f5b1c98/doc/native_protocol_v5.spec)
defines this initial-exchange exception. On this workstation, TShark 4.4.8
displays only frames 4/6/8 as CQL, even with explicit `tcp.port==9042,cql`
decode-as. The additional layouts were verified against Apache's grammar and
actual bytes, not inferred from absence of a Wireshark diagnostic. This is a
specific local-version observation, not a general Wireshark capability claim.

The 19-byte internode message contains magic `ca552dfa`, flags `0c0a0c11`,
requested/minimum/maximum messaging versions 12/10/12, urgent-message category,
CRC framing, and explicit endpoint `198.18.0.2:7000`. The low bits for category,
streaming, framing and reserved values are distinguished; an advertised
minimum version of 10 does not make this a legacy eight-byte handshake.
The final CRC is `f3cb19b3`. Its input is the initial sequence `fa2d55ca`
followed by wire bytes `[0,15)`; the initial sequence is not another physical
field. The parser verifies it using standard-library IEEE CRC32, while tests
use an independent bit-at-a-time CRC and the unchanged original value.
Layout, endpoint serialization and CRC initialization come from Apache's
[HandshakeProtocol](https://github.com/apache/cassandra/blob/45fa31beb44c12b01d1e6a8cca1565650f5b1c98/src/java/org/apache/cassandra/net/HandshakeProtocol.java),
[InetAddressAndPort](https://github.com/apache/cassandra/blob/45fa31beb44c12b01d1e6a8cca1565650f5b1c98/src/java/org/apache/cassandra/locator/InetAddressAndPort.java)
and [Crc](https://github.com/apache/cassandra/blob/45fa31beb44c12b01d1e6a8cca1565650f5b1c98/src/java/org/apache/cassandra/net/Crc.java)
at the same pinned 4.0.19 revision. IPv4/IPv6 endpoints require their explicit
port; legacy address-only encodings do not acquire a guessed default port.
Later internode messages and successful peer negotiation are not claimed.

Bounds are 1 MiB per input, 4,096 combined option/list items and 65,536 leaves.
Native and public tests reject every proper prefix of every original message;
independent fixtures cover empty maps, duplicate keys, multibyte and invalid
UTF-8, embedded NUL, 65,535-byte string values, endpoint/framing variants,
maximum stream IDs and item/leaf limits. All 42 wrong-profile combinations
reject; 224 native transaction outcomes and 112 public valid/fallback
bit-offset outcomes check exact bytes, field types, metadata spans, parent
byte-order isolation, sentinel consumption and nested rollback. Invalid
boundaries fail before reading, structured generation rejects, cached parses
remain independent, and late failures keep the complete original input in
`Unparsed Cassandra Wire` without publishing a partial typed tree.

The targeted run included Cassandra, TNS, supplemental profiles, catalog/
roadmap/reference integrity and adjacent TDS/certificate bridge regressions:
exit 0 (main package 1.893 s; native parser package 0.933 s). These are
correctness-test durations, not packet throughput. No full-library test,
benchmark, profiling, dependency change, capture rewrite, commit or push was
performed in this batch.

## Rebuild

From existing local inputs:

```bash
go run ./common/bin-parser/testdata/protocol-corpus/tools/generate
```

Add `-fetch` to download the allow-listed immutable upstream files. The
generator requires Go and `tshark`. It does not contact endpoints recorded
inside captures. Decode-as settings and display filters come from
`sources.json`, described by `source-spec.schema.json`; generated output is
described by `manifest.schema.json`.

The generator validates every capture before replacing representative hex
files. A bad digest, unavailable dissector or nonmatching filter must leave
the previous hex inventory intact.

Local fixture recipes live under `tools/generate-local/` and use Scapy.
Recipe SHA-1 identifies the generation source; fixture SHA-256 independently
pins the resulting bytes. Original and corrected recipes are kept separate.
The local parser tests do not require Wireshark or network access.

## Provenance

License texts, or the upstream README where no license is provided, are
retained under `licenses/`. A README is provenance, not a blanket
redistribution grant. `reports/UPSTREAM_INDEX.md` distinguishes committed
sources from collections awaiting attachment-level review. Formal protocol
names and source URLs are retained accurately.

The CSV tables and SVG chart describe material availability, not completed
implementation. No capture is executed or replayed onto a network.
