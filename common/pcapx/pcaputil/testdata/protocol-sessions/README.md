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
| `postgres-extended-query.pcap` | Protocol 3.0 Startup, AuthenticationOk, ReadyForQuery, simple Query, CommandComplete (non-5432 port 15432) |
| `ldap-bind-search.pcap` | LDAPv3 anonymous Bind, BindResponse, SearchRequest, SearchResultEntry, SearchResultDone (port 14389) |
| `websocket-upgrade-text.pcap` | HTTP/1.1 Upgrade to RFC 6455 and an unmasked text frame (port 18090) |
| `redis-resp2-resp3.pcap` | RESP2 PING/PONG and a RESP3 Map (port 16379) |
| `grpc-unary-http2.pcap` | HTTP/2 preface/SETTINGS plus unary gRPC DATA prefix and trailers (port 18081) |
| `mqtt5-connect-qos.pcap` | MQTT 5.0 CONNECT with Topic Alias Maximum, CONNACK, QoS 1 PUBLISH/PUBACK (port 18830) |
| `mongodb-opmsg-compressed.pcap` | MongoDB OP_MSG ping/ok correlation on non-27017 port 27018 |

M1 samples are also listed in `m1-manifest.json` with SHA-256 and generator identity.

Regenerate intentionally from the repository root:

```sh
YAK_UPDATE_SESSION_PCAP=1 go test ./common/pcapx/pcaputil -run '^TestLiveProtocolPCAP$|^TestProtocolSessionM1PCAP$' -count=1
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
