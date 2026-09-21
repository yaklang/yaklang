# M1 native capture reproduction

Run from the repository root with Go 1.22.12. The generator binds only
127.0.0.1:19440–19442. It uses actual Go TLS/HTTP2 and gorilla WebSocket
connections, with generated traffic assertions independent of the parser.
TLS certificate verification is disabled **only in this loopback test client**;
the static httptest certificate is not a production identity.

```sh
go build -o /tmp/generate-m1 ./scripts/protocol-tests/generate-m1
# In another terminal, capture before starting the generator:
sudo tcpdump -i lo0 -U -s 0 -w /tmp/m1.pcap 'tcp port 19440 or tcp port 19441 or tcp port 19442'
/tmp/generate-m1 -keylog /tmp/m1.keys
# Stop tcpdump with SIGINT and retain its packet/drop counts.
```

For separate fixtures, capture ports 19440/19441 for TLS and 19442 for WebSocket.
Timing, TCP sequence numbers, certificates' session randomness, ephemeral ports,
and HTTP Date headers can differ; compare semantic oracles rather than hashes
of regenerated traffic. Committed fixtures are immutable and have manifest hashes.

Oracle commands (TShark 4.4.8; no production dependency):

```sh
tshark -r tls-h2-bidi.pcap -o tls.keylog_file:tls-h2-bidi.keys -d tcp.port==19440,tls -d tcp.port==19441,tls -T fields -e frame.number -e tls.handshake.extensions_server_name -e tls.handshake.ciphersuite -e http2.streamid -e grpc.message_length -e http2.header.name -e http2.header.value
tshark -r http-ws-native.pcap -d tcp.port==19442,http -T fields -e frame.number -e http.request.method -e http.response.code -e websocket.opcode -e websocket.rsv1
tshark -r dhcpv6-native.pcap -T fields -e frame.number -e dhcpv6.msgtype -e dhcpv6.xid -e dhcpv6.iaprefix.pref_addr
```

DHCPv6 is the original Wireshark SampleCaptures DHCPv6.pcap, downloaded without
rewrapping or changing its packets. Its source and digest are in
`common/pcapx/pcaputil/testdata/protocol-sessions/first-batch-m1/manifest.json`.
Existing DNS/mDNS/DHCP/ICMP fixtures retain their upstream manifest/license records.
These material distinctions prevent synthetic TCP wrappers from counting as native UDP evidence.
