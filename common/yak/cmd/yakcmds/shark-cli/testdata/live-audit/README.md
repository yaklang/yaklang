# Fresh Windows capture and differential audit

Run explicitly from the repository on Windows with Npcap installed. These tools
use real sockets and capture the resulting packets; they do not replay the sealed
test corpus. Normal `go test` does not launch services or access the Internet.
Use a repository path without `&` for Windows CGO builds.

```powershell
go build -o C:/tmp/shark-live/generate.exe ./common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/main.go
go build -o C:/tmp/shark-live/events.exe ./common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/events/main.go
go build -o build/shark-windows/yak.exe ./common/yak/cmd/yak.go
C:/tmp/shark-live/generate.exe C:/tmp/shark-live/capture-01
python ./common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/faults.py C:/tmp/shark-live C:/Tools/Wireshark/tshark.exe ./build/shark-windows/yak.exe C:/tmp/shark-live/events.exe
```

Create the parent output directory first. Each live run uses fresh ports, a fresh
certificate trusted by its own client, fresh secrets/CIDs, and a random nonce.
The manifest records application response/message hashes independently of either
decoder. TLS 1.2/1.3, session resumption, HTTP keepalive, bidirectional gRPC with
large messages, UDP/TCP DNS, three HTTP/3 connections sharing a UDP socket, and a
TCP tuple reused from TLS to Redis are exercised. A UDP proxy drops and delays
packets in the middle QUIC connection. BPF captures only these test ports, and
only the client leg of the QUIC proxy, avoiding duplicate proxy observations.

`faults.py` derives thirteen variants **from the fresh pcap**, preserving packet
bytes except each declared fault: TCP/QUIC reordering and loss, a duplicate TCP
segment, snaplen truncation, truncation repaired by retransmission, a TLS
ciphertext bit flip, missing ClientHello/Initial, an unknown short-header CID,
and tuple reuse with the second SYN/SYN-ACK omitted. Every variant has a source
SHA-256 and an original-frame map. Protocol parsers never see this sidecar.

Each result includes `tshark.tsv`, `yak.jsonl`, native `events.jsonl`, a
`frame-comparison.csv` covering every packet, and `summary.json`. The suite
checks exact gRPC payload hashes against both Wireshark and the live application,
no invented plaintext under faults, no CoAP admission of the protected QUIC
controls, and no TLS identity inherited across a reset with an unseen handshake.
Complete TLS records are also compared byte-for-byte by oracle TCP stream and
directional endpoints; all native contributor frames must bind to that stream.
The three typed record-header fields are checked against the same wire bytes.
`tls-wire-oracle.json` preserves incomplete views separately; these checks do
not treat equal record counts as proof of correct stream association or AEAD.

Packet-column names need not be identical: an ACK retains TCP as its packet
protocol, while its connection identity is separate. Reassembled message events
explicitly list their contributing frames. Wireshark's out-of-order TCP
reassembly is enabled. HTTP/3 retransmitted STREAM ranges remain in the packet
CSV but count once for logical DATA delivery. Wireshark 4.6.9 sometimes displays
duplicate request headers or omits DATA on reordered QUIC streams; those
differences remain in the summaries and are checked against the application's
independent response hashes instead of making the expected output match a bug.

The following optional harness adds public HTTPS/HTTP/3 requests using system
certificate roots. Capture filters contain only this process's reserved local
ports. A tunnel/firewall may prevent the QUIC handshake; the manifest reports
that as a failed network operation rather than successful decoder coverage.

```powershell
go build -o C:/tmp/shark-live/internet.exe ./common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/internet/main.go
C:/tmp/shark-live/internet.exe C:/tmp/shark-live/internet-01
```

Output directories contain session-owned NSS key logs and plaintext diagnostics;
keep them outside the repository. No browser/production key discovery is used.
The native event helper deliberately allows incomplete capture reassembly, as
Shark does; the default low-level replay API remains strict for existing callers.

## Remaining protocol peers and midstream TLS

`remaining/run.py` adds fresh SSH, compressed WebSocket, FTP/explicit FTPS,
SMTP/STARTTLS, SMB2/NTLM, MQTT 3.1.1 QoS 0/1/2, PostgreSQL, MySQL, LDAP,
POP3, IMAP, SNMP, DHCPv4 and NTP traffic. Bindings are loopback-only. Services
use independent libraries (pyftpdlib, aiosmtpd, Impacket, AMQTT/Paho, x/crypto
SSH and Gorilla WebSocket); database/directory/mail-store/UDP wire peers are
explicit limited scenarios, not production database or directory servers.
`manifest.json` records each operation's hash, library versions and peer scope.
Only test-owned credentials and NSS keys are exported.

```powershell
python -m pip install --target C:/tmp/shark-live/python-libs impacket==0.13.1 pyftpdlib==2.2.0 aiosmtpd==1.4.6 paho-mqtt==2.1.0
python -m pip install --target C:/tmp/shark-live/broker-libs amqtt==0.12.1 ntplib==0.4.0
go build -o C:/tmp/shark-live/remaining.exe ./common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/remaining/main.go
python ./common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/remaining/run.py C:/tmp/shark-live/remaining-01 C:/tmp/shark-live/remaining.exe C:/tmp/shark-live/python-libs C:/tmp/shark-live/broker-libs
python ./common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/remaining/compare.py C:/tmp/shark-live/remaining-01 C:/Tools/Wireshark/tshark.exe ./build/shark-windows/yak.exe C:/tmp/shark-live/events.exe
python ./common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/remaining/verify.py C:/tmp/shark-live/remaining-01
python ./common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/remaining/faults.py C:/tmp/shark-live/remaining-01 C:/tmp/shark-live/remaining-faults C:/Tools/Wireshark/tshark.exe ./build/shark-windows/yak.exe C:/tmp/shark-live/events.exe
```

`verify.py` compares file/message hashes with the independent application
manifest, then compares LDAP/SNMP/SMB IDs against Wireshark on source frames.
`faults.py` derives loss, snaplen, duplicate, repaired snaplen, midstream and
available consecutive-segment reorder cases. New decoded PDU bytes must occur
in the original capture; duplicates/repairs preserve the original message set.
Every variant retains its source hash and original-frame map.

`matrix.py VERIFIED_EXPORTED_CORPUS OUTPUT_DIR TSHARK YAK EVENTS` reads the
versioned 30-family test inventory and writes per-frame comparisons for every
case. Archived fixtures and its generated ARP control remain explicitly distinct
from fresh live traffic. Related profiles are also covered by the Go tests.

`remaining/midstream_tls.py REAL_TLS_PCAP OUTPUT_DIR TSHARK` removes all other
packets and keys from one real IPv4 TLS record, tries port 443 and a nonstandard
port, then removes the five-byte record header. It records Wireshark's actual
labels. The record legacy version is never a negotiated TLS version oracle.

`ldp_discovery_audit.py LDP_ORACLE_JSON FULL_NATIVE_JSONL ORIGINAL_CAPTURE SUMMARY
[FULL_CLI_JSONL]` compares the original UDP discovery bytes, ordered typed
header/message/TLV values, physical contributor interfaces and progressive paths.
Export the oracle from the whole original file with `tshark -2 -n -r FILE -Y ldp
-T json -x --no-duplicate-keys`, saving stdout and stderr separately. It records
malformed/incomplete discovery views and all non-UDP views as explicit gaps;
neither TCP label-distribution coverage nor successful adjacency is inferred.

`ldp_session_audit.py LDP_ORACLE_JSON FULL_NATIVE_JSONL SUMMARY` checks complete
TCP PDU occurrences, ordered registered message/TLV/FEC fields and bidirectional
oracle-stream/native-flow partitions within capture interfaces. It preserves
malformed or unselected views, requires positive matched coverage, and reports
extra native PDUs separately. When a whole-file oracle collapses interfaces,
export each observed interface into a separate external PCAPNG and map every
supplemental result back to original frame references; keep the original score.

`ldp_fault_corpus.py SINGLE_INTERFACE_PCAPNG EXTERNAL_FAULT_DIR` selects an actual
SYN-started IPv4 LDP connection and derives seven explicit offline controls:
baseline, split/reordered PDU, duplicated range, missing range, snaplen, repaired
snaplen, and same-tuple reuse with RST/new ISNs. Application bytes remain original;
changed headers/checksums and the later second-generation clock are declared.
`ldp_fault_compare.py EXTERNAL_FAULT_DIR TSHARK EVENTS` checks every complete PDU
against Wireshark, exact original payload multiplicity, intact reverse direction
under loss, and independent negotiation after reuse. These are supplemental
derived controls, not fresh router traffic or a universal LDP conformance suite.

Native Windows field-pane QA can set `SHARK_QA_FIELD_DOWN=65` (0..512) to navigate
the real pane before saving `01-deep-protocol-fields.json`. This tests nested
message/TLV fields where they render, without synthesizing a replacement page.

`discovery/main.go` captures physical SSDP traffic for 20 seconds, sends one
multicast M-SEARCH, and records independent loopback positive/negative controls.
It never retrieves a discovered LOCATION URL. The shared bounded SSDP rule and
native datagram decoder are compared on every real device frame, including
nonconforming vendor headers retained with warnings.

`discovery/tls_suites.py OUTPUT_DIR TSHARK OPENSSL PYTHON_LIBS` runs independent
OpenSSL clients against Python TLS servers, with certificate verification and
three HTTP/1.1 requests on each of six TLS 1.2/1.3 AES128/AES256/ChaCha profiles.
The application manifest records the negotiated cipher and received body hashes.
`discovery/compare.py PCAP OUTPUT_DIR TSHARK YAK EVENTS KEYLOG_OR_DASH ssdp_or_tls`
writes all-frame comparisons and verifies those independent values.

`discovery/quic_epochs.py OUTPUT_DIR TSHARK AIOQUIC_LIBS CERT KEY` uses aioquic
1.7.0 over real Windows UDP sockets, with three AEAD profiles, two key updates
per connection, CID rotation, NAT rebinding, same-socket connection reuse,
actual delayed/dropped datagrams and one corrupt ciphertext per profile. Use a
fresh directory and a currently valid certificate from the TLS harness. Install
aioquic in a separate external target directory; it is not a Yak dependency.
`discovery/quic_compare.py CAPTURE_DIR OUTPUT_DIR TSHARK YAK EVENTS` verifies
all frame numbers, authenticated packet numbers, phase transitions, connection
boundaries, request paths, and all 36 request/response body hashes. It explicitly
records differences between Wireshark's HTTP/3 PDU output and an independent
reassembly of Wireshark's authenticated QUIC STREAM bytes.

`discovery/render_console.py NATIVE_BUFFER_JSON OUTPUT_PNG` renders the exact
Windows console cells and attributes captured by the native ConPTY QA helper.
These are actual terminal buffer snapshots, not proposed UI mockups. The QA
helper and its invocation are preserved in `discovery/console/main_windows.go`.


`discovery/ssh_live.py OUTPUT_DIR TSHARK PYTHON_LIBS OPENSSH` captures five
independent OpenSSH/Paramiko connections: Curve25519, P-256, fixed DH, DH group
exchange, and the client's actual default. It validates host keys with a fresh
known_hosts file, uses test-only public-key authentication, transfers 128 KiB per
connection and sets RekeyLimit=32K. Paramiko is pinned to 5.1.0. The modern default
may use ML-KEM; Yak reports its method body as unconfirmed rather than claiming
a complete hybrid implementation. `capture_ready.py` waits for an actually
captured UDP marker before application work; sleep alone did not guarantee Npcap
startup. Marker packets stay in the capture and are checked as non-SSH traffic.

`discovery/control_live.py OUTPUT_DIR TSHARK PYTHON_LIBS` uses Scapy 2.8.0 to
encode and send socket controls for IKE/NAT-T, RIP, NBT-DG, ONC RPC, L2TP, OpenVPN
and IPMI, plus incomplete/garbage negative cases. These are controlled wire
exchanges, not full production VPN/router/RPC servers. It also captures two real
telnetlib3 5.0.1 binary transfers (all 256 octets, IAC escaping) and an NBSS
session request/response/keepalive exchange. If Windows reserves UDP 500/4500,
random ports are recorded and selected explicitly in both analyzers; no OS
service, firewall or route is changed.

`discovery/wireguard` is a separate test-only Go module pinned to wireguard-go
v0.0.0-20231211153847-12269c276173. Its two real endpoints exchange encrypted UDP
and authenticate/decrypt 16 inner packets using memory-only TUN objects. It does
not create an OS network adapter or change routes. Its private keys are never
provided to Yak; native output remains an unverified encrypted envelope.

```powershell
python -m pip install --target C:/tmp/shark-live/ingress-libs paramiko==5.1.0 scapy==2.8.0 telnetlib3==5.0.1
python ./common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/discovery/ssh_live.py C:/tmp/shark-live/ssh-01 C:/Tools/Wireshark/tshark.exe C:/tmp/shark-live/ingress-libs C:/Tools/OpenSSH/ssh.exe
python ./common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/discovery/control_live.py C:/tmp/shark-live/control-01 C:/Tools/Wireshark/tshark.exe C:/tmp/shark-live/ingress-libs
```

`discovery/ingress_compare.py CAPTURE_DIR OUTPUT_DIR TSHARK YAK EVENTS
ssh|wireguard|control` writes every-frame CSV, TShark fields, CLI rows and native
events, then checks exact wire bytes and applicable message IDs, algorithms,
index/counter fields, scalar headers, host fingerprints and delivered Telnet
bytes against independent observations. Protocol label differences on malformed
packets and ACK-only packets remain in the CSV instead of being normalized away.

`discovery/ingress_faults.py ROOT TSHARK YAK EVENTS` uses verified `ssh-02` and
`wireguard-01` inputs under ROOT to derive 14 loss, reorder, duplicate, snaplen,
retransmission repair, missing identification/initial, receiver mutation and TCP
tuple-reuse cases. Source hashes and mutations are recorded. SSH cannot invent a
new decoded PDU; intact/repaired streams must retain the original message set.
WireGuard without observed initial receiver context must not gain confirmed
association. Both microsecond and nanosecond little-endian pcap are supported.

The WireGuard harness includes inner lengths at/below MTU 1420; valid transport
lengths can be 1452 because upstream clips padding to MTU. The fault matrix also
splits a real SSH KEXINIT into valid TCP/IP packets, recomputes both checksums
and delivers the latter sequence range first; all original PDUs must survive.


`discovery/field_contract_compare.py OUTPUT TSHARK YAK INGRESS_ROOT RECOVERY_ROOT`
replays retained, hashed real captures through the registered field schema and
CLI `--display-filter`. It verifies map/occurrence identity, integer widths and
exact decimal representations, protocol namespaces, logical ranges and selected
filter frame sets against TShark. TLS is replayed both without and with the
independent key log to distinguish opaque record types from authenticated inner
types. An independently known negative RIP prefix may expose a Wireshark field
under explicit selection; that difference is retained with its frame number.
Other query discrepancies fail. These are replays, not new live captures.
