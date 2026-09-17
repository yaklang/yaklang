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
| `kafka-apiversions-metadata.pcap` | Kafka ApiVersions + Metadata v0 on non-9092 port 19092 |
| `tds-prelogin-login-batch.pcap` | MS-TDS 7.4 PRELOGIN, LOGIN7, SQLBatch and token-stream result on non-1433 port 11433 |
| `amqp-publish-deliver.pcap` | AMQP 0-9-1 protocol header, Connection.Start, channel open, Publish/Deliver/Ack on non-5672 port 15672 |
| `smb2-negotiate-create.pcap` | SMB 3.1.1 Negotiate, SessionSetup, TreeConnect and Create on non-445 port 1445 |
| `dcerpc-epm-srvsvc.pcap` | DCE/RPC v5 Bind/BindAck plus EPM ept_map and SRVSVC NetrShareEnum on non-135 port 13500 |
| `ssh-kex-newkeys.pcap` | RFC 4253 identification, KEXINIT, DH/ECDH exchange, NEWKEYS on non-22 port 10022 |
| `nfsv3-lookup-read.pcap` | NFSv3 ONC RPC LOOKUP/GETATTR/READ/WRITE with XID pairing on non-2049 port 12049 |
| `snmpv3-get-response.pcap` | SNMPv3 USM noAuthNoPriv Get/GetBulk/Set/Trap/Inform with request-id pairing on non-161 port 1161 |
| `rdp-tpkt-negotiate-mcs.pcap` | RDP TPKT/X.224 Cookie+NEG_REQ/RSP and plaintext MCS/GCC channels on non-3389 port 13389 |
| `dot-dns-tcp-length.pcap` | RFC 7858 DoT on TLS plaintext: 2-byte DNS length, Query/Response ID pairing on non-853 port 1853 |
| `doh-http-get-post.pcap` | RFC 8484 DoH HTTP/1.1 POST and GET application/dns-message with DNS ID pairing on non-443 port 18443 |
| `sip-invite-ack-bye.pcap` | RFC 3261 INVITE/100/200/ACK/BYE on non-5060 TCP 15060 with SDP audio |
| `rtp-seq-sr-rr.pcap` | RFC 3550 RTP PCMU sequence plus RTCP SR/RR on non-5004 TCP 15004 |
| `quic-v1-crypto-stream.pcap` | RFC 9000 QUIC v1 Initial CRYPTO, Handshake CRYPTO, 0-RTT STREAM and CONNECTION_CLOSE on non-443 TCP 14443 |
| `quic-v1-rfc9001-initial.pcap` | RFC 9001 A.2/A.3 Client/Server Initial with header protection removed and CRYPTO decrypted on non-443 TCP 14443 |
| `http3-settings-headers-data.pcap` | RFC 9114 HTTP/3 control SETTINGS, request HEADERS+DATA and response HEADERS on QUIC streams, non-443 TCP 14443 |
| `http3-qpack-encoder-headers.pcap` | RFC 9204 QPACK encoder inserts plus HEADERS decoded from the dynamic table on non-443 TCP 14443 |
| `doq-query-response.pcap` | RFC 9250 DoQ length-prefixed Query/Response on a QUIC stream, non-853 TCP 14853 |
| `smtp-ehlo-mail.pcap` | RFC 5321 EHLO multiline 250 and MAIL FROM on non-25 port 10025 |
| `imap-capability-login.pcap` | RFC 3501 greeting, tagged CAPABILITY and tagged OK on non-143 port 10143 |
| `pop3-capa-stat.pcap` | RFC 1939 +OK greeting and STAT on non-110 port 10110 |
| `ftp-user-pass-pasv.pcap` | RFC 959 220 FTP greeting, USER/PASS on non-21 port 10021 |

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
