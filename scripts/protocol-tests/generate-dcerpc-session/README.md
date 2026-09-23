# DCE/RPC v5 complete Bind exchange fixture

This directory contains an independently constructed, deterministic packet
fixture for the connection-oriented DCE/RPC session path. Run:

```sh
python3 scripts/protocol-tests/generate-dcerpc-session/generate.py
```

The script uses only Python's standard library to build Ethernet, IPv4, TCP,
and DCE/RPC wire bytes. It does not replay, edit, or claim to be a capture from
a live endpoint. The exchange contains a TCP handshake followed by four
single-fragment CO v5 PDUs: Bind, a complete BindAck accepting EPM with NDR32
version 2, an opaque request on context 0, and its response. The request uses
operation number 99 so the fixture proves context negotiation and request /
response correlation without implying that an EPM `ept_map` stub was built.

The wire layout follows the public [MS-RPCE presentation-context negotiation
rules](https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-rpce/ca4d3552-4647-4f40-830b-fd2090adec8f)
and the DCE/RPC connection-oriented PDU definitions. The source interface UUID
is EPM (`e1af8308-5d1f-11c9-91a4-08002b14a0fa`); the accepted transfer syntax
is NDR (`8a885d04-1ceb-11c9-9fe8-08002b104860`, version 2). Addresses use
documentation-only IPv4 `192.0.2.0/24` space.

Generated capture:

- `common/pcapx/pcaputil/testdata/protocol-sessions/dcerpc-v5-bindack-request-response.pcap`
- 7 Ethernet packets: SYN, SYN/ACK, ACK, Bind, BindAck, Request, Response.
- SHA-256: `9da45e447781caf868f64892833f8dc9c3ccafe04b400e8dde4a2e4095c6821b`
- Classification: generated synthetic evidence, not live network evidence.

Independent dissector output:

- `tshark-4.4.8.tsv` was produced with TShark 4.4.8 using the field command
  below; the output identifies the four RPC PDU frames, accepted context use,
  and request/response operation correlation.
- SHA-256: `a31660fd2616db4a57b031ba1958c5d1281b816b1db93dd17b75c5f3be5f967c`
- `tshark -r common/pcapx/pcaputil/testdata/protocol-sessions/dcerpc-v5-bindack-request-response.pcap -T fields -E header=y -E separator=/t -E occurrence=f -e frame.number -e _ws.col.protocol -e dcerpc.ver -e dcerpc.pkt_type -e dcerpc.cn_call_id -e dcerpc.cn_ctx_id -e dcerpc.opnum -e dcerpc.cn_bind_to_uuid`

TShark's verbose decode of frame 5 reports one result: “Acceptance, 32bit
NDR”, syntax version 2. The existing `dcerpc-epm-srvsvc.pcap` remains a
separate real-capture-derived malformed negative (SHA-256
`c23f3bec06a192aafa7079ddfb38ba37510a14f33b562220a05100efdb3cdf75`): its
BindAck has only a 10-byte body and must not activate a presentation context.
