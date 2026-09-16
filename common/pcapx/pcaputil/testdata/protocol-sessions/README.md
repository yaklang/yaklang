# Generated HTTP/2 and MySQL session captures

These four files are deterministic generated protocol examples, not captured
production traffic. Test addresses use RFC 5737 documentation ranges. TCP
headers/checksums/timestamps and seven-byte application segmentation come from
`sessionTestPCAP`; HTTP/2 header blocks use the independent `x/net/http2/hpack`
encoder. MySQL messages are assembled from the documented classic wire layouts.
Authentication bytes are dummy fixture data; no real credentials are present.

| File | Coverage |
|---|---|
| `http2-multiplex.pcap` | Initial preface and bilateral SETTINGS/ACK; streams 1/3, persistent dynamic references, HEADERS/CONTINUATION, reversed response order, DATA and GOAWAY |
| `mysql-classic.pcap` | Greeting/login, authentication switch/continuation/fast-auth OK, query with two EOF-terminated results, query ERR, PING and QUIT |
| `mysql-deprecated-eof.pcap` | Same exchange using negotiated DEPRECATE_EOF and OK result terminators |
| `mysql-tracked-eof.pcap` | DEPRECATE_EOF plus SESSION_TRACK negotiation and the corresponding explicit result profile |

Regenerate intentionally from the repository root:

```sh
YAK_UPDATE_SESSION_PCAP=1 go test ./common/pcapx/pcaputil -run '^TestLiveProtocolPCAP$' -count=1
```

The normal test reads the committed files and compares their bytes with the
deterministic generator. It also replays pcap/pcapng, IPv4/IPv6 and 1/2/4 workers.
The manifest records SHA-256, generator identity and independent tshark output.
The adjacent `.tshark.tsv` files are the actual decode summaries from the stated
Wireshark version, with trailing display whitespace removed. Zero malformed frames establishes dissector acceptance for
these examples, not general protocol conformance.

```sh
tshark -r http2-multiplex.pcap -d tcp.port==18080,http2 -Y http2
tshark -r mysql-classic.pcap -d tcp.port==13306,mysql -Y mysql
tshark -r mysql-deprecated-eof.pcap -d tcp.port==13306,mysql -Y mysql
# Replace the filename and port as appropriate; expected output is empty.
tshark -r http2-multiplex.pcap -d tcp.port==18080,http2 -Y _ws.malformed -T fields -e frame.number
```

Independent real captures remain in the existing bin-parser corpus, under
`captures/ndpi/ndpi-http2.pcapng` and `captures/ndpi/ndpi-mysql.pcapng`.
`TestLiveProtocolOriginalCaptures` pins their original SHA-256, exact decoded
message/byte counts, HTTP status, SQL query and result value. Their upstream
commit, URLs and LGPL license are retained in that corpus's `sources.json` and
`licenses/ndpi-COPYING.txt`; the real files have not been rewritten.

Generated fixtures and test code follow this repository's license.
