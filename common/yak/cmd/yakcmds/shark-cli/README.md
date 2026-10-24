# yak shark

`yak shark` opens a terminal packet analyzer and captures on the interface selected
by a physical default route with a router gateway. VPN/TUN, point-to-point,
loopback, and virtual bridge interfaces are excluded from automatic selection.
On macOS this reads the kernel routing table, so VPN routes to public IPs do not
override the physical default gateway. Use `--list-interfaces` and `--interface`
to choose another interface (including a loopback or VPN interface).
Live capture uses the existing libpcap/Npcap integration and requires permission
to open the selected capture device.

```sh
yak shark
yak shark --list-interfaces
yak shark -i en0 --protocol dns,tls
yak shark --protocol tcp --bpf 'host 192.0.2.10 and port 443'
yak shark --pcap-file traffic.pcapng
yak shark --pcap-file traffic.pcapng --tls-keylog test-session.keys
yak shark --output-file capture.pcapng
yak shark --plain --protocol dns --duration 30s -w dns.pcap
yak shark --pcap-file traffic.pcap --json --count 100
```

Without an interactive terminal, summaries are printed automatically. `--plain`
forces text output; `--json` emits one packet summary per line. `--count` and
`--duration` stop capture; the TUI stays open for inspection until `q` is pressed.
An offline capture also remains browsable after EOF.

## Filtering

`--bpf` (`-f`) accepts libpcap BPF syntax. `--protocol` accepts comma-separated
names or repeated flags. Protocol names are ORed, then ANDed with `--bpf`:

```sh
yak shark --protocol dns,ntp --bpf 'host 192.0.2.10'
yak shark --list-protocols
```

Application shortcuts select conventional ports. For example `tls` selects TCP
443/8443; it does not decrypt traffic or locate TLS on arbitrary ports. Use an
explicit BPF expression for nonstandard ports. The list uses packet-layer
identification and reassembled payload signatures where available, and labels
port-based application guesses with `port hint` in the Info column. Port hints
alone do not replace TCP/UDP labels. A verified connection prefix is shown as
`[connection #42: HTTP]` in Info; pure ACKs remain `TCP`. Connection attribution
does not claim that an ACK carries an application message. A packet containing valid HTTP
headers is labelled `HTTP`. HTTP recognition requires a complete start line and
valid headers, not just a method word. JSON summaries include `stream_id`,
`protocol_scope`, `protocol_evidence`, `protocol_stack`, `connection_protocol`,
`connection_stack`, `connection_evidence`, `conversation_id`, `visibility` and `protocol_status`; already printed
rows are not rewritten. Validated parser events select the most specific protocol:
for example, `TLSv1.3`, `HTTP/2`, or `gRPC`. TLS versions come from observed
negotiation, never the record's legacy version. Binary identity comes from the
protocol engine, not a second permissive signature scanner in the UI. A plausible
opaque TLS record without handshake/session evidence remains a candidate; it
cannot establish a sticky connection identity. ALPN and port numbers alone do
not assert that encrypted application content was decoded.

`--tls-keylog` accepts an NSS key log for authorized captured sessions. Supported
TLS records are authenticated before their plaintext enters application parsers.
The table shows handshake names, alerts, SNI/ALPN, HTTP/2 stream IDs, request paths
and gRPC message sizes when actually decoded. Missing keys, unavailable handshake
context and authentication failures stay visible; opaque records in established
TLS sessions retain their connection context.
Record AEAD verification and certificate trust are separate fields: this analyzer
does not evaluate certificate trust.
Live selection immediately shows packet headers and raw bytes. Pin a packet
with Enter, navigation keys, or a detail-pane key to run the branch's YAML
protocol dissectors in an isolated process (with a five-second limit). Unrecognized or truncated packets keep
their available lower-layer fields and raw bytes.

Within the TUI, `f` opens a BPF **display** filter and `p` opens a protocol
shortcut filter. These apply to the retained packets and incoming packets.
They do not change the capture filter or the packets written to the output file.
An invalid expression keeps the previous filter. Submit an empty filter to clear
it. BPF syntax is different from Wireshark display-filter syntax.

## Search and packet cache

Click the Search bar or press `/` (also Ctrl+F), type a query, and press Enter.
Search is combined with the display BPF using AND. Empty search or the clickable
Clear button clears only search. Escape cancels editing; invalid queries retain
the previous search and report the error. Search covers the current retained
packet list, or its frozen browsing snapshot, and never changes capture/output.

| Query | Meaning |
| --- | --- |
| `10.223` | Contains this string in addresses, protocol, info or raw packet text |
| `proto:http` / `proto=HTTP` | Protocol contains HTTP / exactly HTTP; uses identified packet or connection protocols, not port hints |
| `fields protocol == "grpc"` | Match actual retained gRPC message events; unlike connection-aware `proto:grpc`, this does not include ACKs merely belonging to the connection |
| `ip:10.223` / `ip.str:10.223` | Either address contains this partial string |
| `ip=10.223.1.2` / `ip!=10.223.1.2` | Exact address / exclude exact address |
| `src:10.223` / `dst:fe80::` | Source / destination address substring (IPv4 or IPv6) |
| `ip:10.0.0.0/8` | Either address is within a CIDR network |
| `port:80,443` / `dport:8000-8090` / `sport:53` | Either, destination or source port; lists and ranges supported |
| `content:"Hello World"` | Literal case-sensitive bytes within a captured packet |
| `text:hello` | Packet bytes with ASCII case-insensitive matching |
| `hex:"00 ff 41"` / `hex:00ff41` | Raw byte sequence, including binary values |
| `info:"GET /login"` | Packet Info column contains this text |
| `stream:"hello world"` | Case-insensitive text in the retained reassembled TCP connection, including text split across segments |

Space-separated terms are ANDed. Use `OR` or `|` for alternatives and `!` or `-`
to negate a term, e.g. `proto:http ip:10.223 !port:8080` or
`proto:dns OR proto:tls ip:10.223`. AND binds tighter than OR; grouping parentheses
are not supported. Quote values containing spaces or `|`; a backslash escapes the
next character. Protocol/address/info comparisons ignore case. `:` means contains
and `=` means exact for those fields. Packet content includes captured headers
and payload. Stream search includes the independently preserved prefixes and the
recent reassembled window, but never joins directions or crosses missing/omitted
bytes. Evicted history and encrypted plaintext cannot be searched. A stream match
keeps all retained packets belonging to that connection; use 4 to inspect it.

The packet cache defaults to **20,000**. Set `--max-packets 50000` at startup, or
click Cache / press `c` to change it while running (1–100,000). The in-app editor
accepts `20000`, `20k` and `2w`. Shrinking keeps the newest packets and trims the
browsing snapshot; a selected packet can remain pinned until another is selected.
Growing allows more future packets and cannot restore evicted history. Search
matches are cached per packet and rechecked when its connection gains data or a
verified protocol, so a frozen list can match newly reassembled content.

## Terminal controls

| Key | Action |
| --- | --- |
| Mouse wheel / vertical trackpad scroll | Scroll only the pane under the pointer; keep the selected packet/field |
| Left click | Select a packet, field, or tab; field selection highlights its bytes, byte selection locates its field |
| Drag right-side scrollbar | Scroll that pane, including when the pointer leaves its bounds |
| Up/Down, j/k | Select a packet/field or scroll bytes in the focused pane |
| Tab / Shift+Tab / 1, 2, 3, 4 | Switch panes; opening an inspector pins the packet list |
| Enter | Pin and deeply decode the selected packet; on a field, open its bytes |
| 4 / s | Open the selected TCP connection in Stream |
| Page Up/Down | Move one pane page |
| g / G, Home / End | First/last row; G or End resumes following in the packet list |
| Space | Freeze the current list / return to live packets (capture continues) |
| d | Open all decoded fields; in Stream, show application fields from contiguous prefixes |
| t / x (Stream) | Text with binary fallback / hexadecimal bytes |
| a (Stream) | Decoded application messages / original reconstructed wire bytes |
| b | Cycle captured frame / retained reassembled or decrypted message byte sources |
| h / l / e (Stream) | Preserved beginning / recent bytes / protocol recognition evidence |
| v (Stream) | Both directions / A→B / B→A |
| r (Stream) | Refresh the stream snapshot and resume automatic updates |
| / or Ctrl+F | Open Search (also clickable) |
| c | Change the packet cache capacity (also clickable) |
| f | Edit the display BPF filter |
| p | Build a display filter from protocol shortcuts |
| Ctrl+U / Escape | Clear filter input / cancel editing |
| q / Ctrl+C | Close and flush capture output |

The dashboard shows a traffic-rate history, capture counters, a protocol mix
from the most recent 128 retained packets, the packet table, and the inspector.
The screen needs at least 70 columns and 24 rows and adapts when resized.
On Windows, the TUI enables virtual-terminal output and UTF-8 input/output for
the session, disables legacy console Quick Edit so mouse reports reach the TUI,
then restores the shell's modes and code pages on exit. Its grid
uses single-cell borders and other ambiguous symbols regardless of the initial
Chinese/Japanese/Korean code page. Colored redraws clear with the dashboard's
background, including in light-themed terminals. `NO_COLOR` keeps the shell's
colors. Compact packet tables retain an Info column.
Wide terminals show fields and bytes side by side; narrow terminals use panes
2 and 3 in the same area. Each pane keeps an independent viewport. Scrolling,
clicking or navigating freezes a bounded snapshot of the packet list, so new
arrivals and ring-buffer eviction cannot move rows under the pointer. Press
Space, or G in the packet list, to return to the latest packets. Capture counters
and output recording continue while browsing. A late deep decode does not replace
the flat fields being browsed; press d when the status says it is ready.
Fields are grouped by protocol, with no root tree or collapsible levels.
Packet details start with the frame, followed by the most specific decoded
application events, TLS metadata, and byte-backed wire protocol fields. Readable
HTTP/2 headers, gRPC messages, TLS handshake names and cipher names appear before
generic parser metadata.
Opaque data and binary string values show a byte count and HEX/ASCII preview.
Select a field to seek to and highlight its full raw range in both byte columns;
click either HEX or ASCII to locate the smallest containing field. Offsets come
from the parser, including bytes containing bit fields. Generated values without
raw offsets do not claim a byte range. Reassembled records and decrypted or
decompressed messages have separately named byte sources; selecting their fields
switches the byte pane to the corresponding bytes. These offsets never highlight
unrelated captured ciphertext. Press b to cycle sources. On narrow terminals, Enter opens the byte
pane for the selected field. Wheel scrolling still affects only the hovered pane.

## Stream reassembly

The Stream tab fills the lower inspector area with both directions of the
selected TCP connection or a recognized UDP protocol session. UDP/QUIC uses the
engine's domain-scoped flow/CID association; it does not concatenate datagrams or
guess a connection from UDP port 443. Its bounded message window includes the
selected retained PDU, and the wire view labels individual datagram boundaries.
Connection-only metadata survives message-history eviction within a separate
bounded cache. New TCP generations, capture domains and unrelated QUIC CIDs do
not inherit another connection's identity. The tab distinguishes current packet
layers from previously observed connection layers. A and B are the first observed endpoints, not an inferred
client/server role. Text is reconstructed across TCP segmentation; binary data
falls back to hex. When retained decoded events are available, the initial view
shows application messages and their protocol stack, direction, headers and
bounded payload previews. For gRPC, message bytes are displayed without inventing
protobuf field names when a schema is unavailable. Press `a` to switch messages
and wire bytes. The wire beginning shows each direction's independently preserved
first 16 KiB. Press `h` for that beginning, `l` for the recent window, or `e` for
recognition evidence (also clickable in the toolbar). Evidence includes the
observed HTTP start line, headers, raw prefix bytes, timestamp, and whether the SYN
was captured. Missing SYN/gaps mean these are the first captured bytes, not proof
of the actual connection beginning. HTTP headers remain readable ahead of binary
bodies, and CONNECT/101 transitions are explicitly marked; later binary content
is not identified as HTTP merely because its connection began with HTTP.
Stream data updates automatically once per second. Scrolling,
dragging, or choosing a view/direction pins that snapshot so its rows stay still;
press r to resume updates. A pinned packet list does not stop assembly.

TCP assembly runs before the lossy UI queue. It handles out-of-order segments,
retransmitted/overlapping bytes, sequence rollover and FIN/RST, and keeps the
directions separate. Missing data is queued for up to two seconds of capture time
(or until memory pressure/EOF), then emitted with an explicit gap. If no SYN was
captured, an unknown-start marker is shown. Prefixes separated by gaps are never
concatenated for identification. Kernel loss, capture filters that exclude parts
of a connection, snaplen truncation, and missing IP fragments can still leave gaps.
Inspection continues through these capture defects without joining bytes across
missing ranges. Truncated rows show the captured/wire byte counts and incomplete
status. A TLS authentication failure preserves the observed connection stack,
halts plaintext decoding, and explains that boundary on subsequent data rows.
After an observed TCP reset, packets without a new handshake cannot inherit the
closed connection's identity. An apparent HTTP/2 window violation caused by
missing peer TCP control bytes is marked unverifiable instead of malformed.

For reproducible fresh socket captures, per-packet Wireshark comparisons and
fault injection, see [the live audit harness](testdata/live-audit/README.md).
IP fragmentation itself is not defragmented.

Protocol sessions advance in the capture pipeline before the display queue.
Stream fields project their retained events, including authenticated inner
protocols; they do not reparse ciphertext as application data. The isolated
worker also supports the branch's YAML dissectors on each direction's contiguous
prefix (up to 16 KiB) when no event snapshot is supplied. Fields from an incomplete
message may be unavailable even though reconstructed text/hex is visible. A
protocol rule being installed does not mean arbitrary bytes can be identified as that protocol.
TLS plaintext is available only when the capture pipeline has authenticated it
using the supplied session keys. Wire view always preserves the reconstructed
wire bytes. SSH and unsupported or unavailable TLS plaintext remain encrypted.

The history retains up to `--max-streams` connections (256 by default), with
`--stream-bytes` recent bytes per connection (512 KiB by default) and a global
32 MiB byte-history limit. Classification prefixes and the bounded reorder queue
are additional memory. The UI marks omitted bytes and evicted streams. Idle
assembly closes after two minutes; a new SYN/ISN starts a separate stream. Reaching
the active-connection limit or seeing tuple reuse flushes the assembly window,
with `assembly window reset` recorded instead of silently merging generations.
Use `--output-file` to keep all captured packets for later inspection.

SMB2 Session Setup exposes validated SPNEGO and NTLMSSP envelopes as a progressive
stack. NTLM names are assertions; record structure, server result and guest
mapping never imply credential or MIC verification. NBNS uses a separate bounded
RFC 1002 parser: its NBSTAT records are not interpreted as DNS SRV records.

The Top 30 family regression matrix is in `top30_test.go`, with related profiles,
negative admission cases, session/generation isolation and TCP segmentation tests.
It covers the declared fixtures and boundaries, not every version or encryption
suite of each family. Unsupported carriers and profiles remain explicit.

Midstream captures can display `TLS` from complete protected record framing even
without a handshake. That label identifies the record envelope; the negotiated
version, session identity and plaintext remain unavailable. Weaker candidates
display `TLS?`. Repeated complete records can retain an explicitly inferred TLS
carrier, while captured negotiation or authenticated records provide stronger
connection evidence. Port 443 alone never confirms TLS. Data segments inside
an observed connection retain its protocol with `connection-associated bytes`
scope; ACK-only packets retain TCP and the separate connection stack.

Native STARTTLS capture continues through TLS into authenticated SMTP/FTP/IMAP/
POP3 plaintext when supported keys are supplied. Passive FTP data needs an
observed control negotiation, transfer command, matching peers and capture
domain; ambiguous mappings remain unassociated. Active-mode data association
is not implemented by this new native path. SSH after NEWKEYS stays encrypted.

Native ingress includes bounded IKE/IKE NAT-T, WireGuard, RIP, NetBIOS
Datagram, ONC RPC, L2TPv2, OpenVPN clear control, IPMI, Telnet and NBSS control
profiles. Their canonical entries are in `network_control_fields.yaml` and the
public catalog; `ProfileOf` links them to existing roadmap families without
promoting delivery scores. Legacy rule entries remain compatible. These are
partial profiles: encrypted VPN payloads, authenticated identities, complete
routing/VPN state and arbitrary RPC procedures are not inferred.

`--decode-as udp:PORT:PROFILE` selects one supported native UDP profile on a
nonstandard port, for example `--decode-as udp:55000:ike`. Repeat for different
ports. Explicit selection still validates lengths, structure and bounds; an
invalid message is a diagnostic, never a successful protocol decode. Automatic
UDP admission requires both service context and validated wire structure;
WireGuard transport also needs an observed receiver index. OpenVPN hard-reset
session changes discard prior server evidence on a reused tuple.

SSH initial packets validate the full KEXINIT tail and distinguish fixed DH,
DH group exchange, P-256 ECDH and Curve25519 layouts. Negotiated names, host key
fingerprint, encryption and verification scope are visible in the detail pane.
Unknown/hybrid method bodies remain explicitly unconfirmed. NEWKEYS changes the
connection to encrypted SSH; ciphertext is never displayed as login/channel data.
Telnet recognizes bounded IAC negotiation/subnegotiation and escaped IAC bytes;
NBSS control can precede SMB2 in the progressive stack. Common scalar fields
have exact PDU byte ranges. The UI does not display its internal range map.

Only changed terminal rows are redrawn. Traffic updates run at 5 FPS; input
updates are coalesced at up to 40 FPS. Mouse reporting uses SGR coordinates for
wide terminals, with legacy report support. Horizontal wheel reports are consumed
without changing the vertical viewport. Terminal mouse, focus and paste modes are
restored on exit. Bracketed paste is treated as filter text, never as shortcuts.
`NO_COLOR` disables styling. Capture-controlled terminal escape sequences are
sanitized before display.

## Capture output and resource limits

`--output-file` (`-w`) creates a new capture file. A `.pcapng` suffix selects
pcapng; otherwise it writes nanosecond pcap. Existing files are never overwritten,
including when input and output name the same file. Output contains every packet
accepted by the capture filter, even if older packets have left the UI cache.

The UI retains the latest 20,000 packets by default (`--max-packets`, editable with c), each up to
`--snaplen` bytes for live capture (default 65,535). Under load, display updates
may be skipped after packets are written. The display queue keeps the newest
packets rather than accumulating an older backlog. The header reports retained/evicted
packets, skipped UI packets, and kernel-reported drops separately. While browsing,
one additional snapshot of at most `--max-packets` packets remains inspectable
even after those packets leave the rolling cache. Returning to live releases it.
Deep parsing runs in a child process, so VM diagnostics cannot write into the
terminal and malformed packets cannot stall capture or screen updates.

The display filter is a typed subset, not the full Wireshark expression language.
`--list-fields` lists the registered field projection; `--list-fields --json`
exports its versioned names, types, widths, enumeration labels and origin. The
same registrations drive SDK `DisplayFields()` / `DisplayFieldValues()`, the
filter compiler, CLI JSON and TUI `fields ...` search. The original parser trees
remain available; not every parser field has a registered display alias yet.

```text
yak shark --list-fields
yak shark --list-fields --json
yak shark -r traffic.pcap --json --display-filter "wg.type == 4 and wg.counter >= 2"
```

`--display-filter` (`-Y`) changes displayed/exported rows; `-w` still saves all
packets accepted by capture BPF and capture limits. It is validated before
opening a device or creating an output file. In the TUI the initial filter is
shown as editable `fields ...` search. Boolean combinations operate across the
PDUs of one packet; adjacent packets and other connections do not contribute.
SDK `Match(event)` remains PDU-scoped, while `MatchEvents(events)` and
`EvaluateEvents(events)` use the explicitly supplied packet event set.

Supported operators are `and`, `or`, `not`, parentheses, existence, `==`, `!=`,
`<`, `<=`, `>`, `>=`, string `contains`, and IP-address `in CIDR`. Strings are
case-sensitive; address spelling is normalized. Unsigned comparisons accept
bounded decimal or hexadecimal integers. Numeric `contains`, non-address CIDR,
fractional/negative/overflowing unsigned literals and ordered booleans are
rejected. `!=` requires a present field with all occurrences unequal; missing
is not zero, false, or a successful inequality. `not field` explicitly tests
absence. This language has bounded expression, traversal and occurrence budgets.

`field_values` in each JSON PDU contains the typed occurrence projection. The
registry distinguishes `event`, logical `pdu`, observed `context`, and `derived`
facts. `pdu_range` is relative to the logical PDU's bytes and source, never an
invented offset into an encrypted or compressed captured frame. Context and
verification facts have no fabricated wire range. Unsigned fields wider than
53 bits include `exact_value` as decimal text for JSON clients using floating
point; the SDK and decoder IPC also preserve exact 64-bit integers. A limited or
unrepresentable projection is explicit. The CLI reports an indeterminate filter
as an error; the TUI reports it as unavailable, and the SDK exposes completeness.

TLS fields distinguish `tls.record.wire_type` (outer record header),
`tls.record.opaque_type` (observed TLS 1.3 opaque envelope), and
`tls.record.content_type` (TLS 1.3 inner type only after authenticated decryption,
or a clear/known legacy type). Midstream encrypted framing without negotiated
version exposes only the wire type. `tls.version` is observed negotiation, never
a legacy-record-version guess. Verification booleans mean verified/not verified;
false does not itself prove authentication failure. Protocol namespace guards
prevent WireGuard/OpenVPN/SSH facts from becoming TLS fields.

Offline pcapng files can contain
multiple interfaces with the same link type; mixed link types fail explicitly
instead of silently dropping a subset of packets. Files exported from pcapng
preserve packets and timestamps, but not original interface names, comments,
or other pcapng metadata.
