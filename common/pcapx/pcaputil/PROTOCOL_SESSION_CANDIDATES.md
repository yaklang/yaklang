# Next 10 ProtocolSession candidates

These protocols are **not** on the existing 20-protocol session stack
(PostgreSQL, WebSocket, LDAP, Redis, gRPC, MQTT 5, MongoDB, Kafka, SMB2/3,
DCE/RPC, TDS, SSH, AMQP 0-9-1, DoT, DoH, SIP, RTP/RTCP, NFSv3, SNMPv3, RDP,
QUIC/HTTP3/QPACK/DoQ). Catalog / roadmap rows stay `partial` / `new` / `todo`.
This file does not mark anything `done`.

First-version work in this stack: SMTP, IMAP, POP3, and FTP on the shared
Probe / Feed / Close path. The remaining six stay sequential follow-ups.

| Protocol | First-version profile | Spec | Must-have | Not complete |
|---|---|---|---|---|
| SMTP | RFC 5321 command/reply | [RFC 5321](https://www.rfc-editor.org/rfc/rfc5321.html) | EHLO/HELO, MAIL/RCPT/DATA, multiline `xxx-`/`xxx ` replies, request/reply association, STARTTLS → Encrypted | SMTPS decrypt, CHUNKING/BDAT, DSN, PIPELINING pipeline reordering |
| IMAP | RFC 3501 IMAP4rev1 | [RFC 3501](https://www.rfc-editor.org/rfc/rfc3501.html) | tagged commands, untagged `*`, synchronizing `{n}` literals, tagged OK/NO/BAD correlation, STARTTLS → Encrypted | IDLE, COMPRESS, QRESYNC, IMAPS decrypt, BODYSTRUCTURE |
| POP3 | RFC 1939 + CAPA/STLS | [RFC 1939](https://www.rfc-editor.org/rfc/rfc1939.html), [RFC 2449](https://www.rfc-editor.org/rfc/rfc2449.html), [RFC 2595](https://www.rfc-editor.org/rfc/rfc2595.html) | USER/PASS or CAPA/STAT, `+OK`/`-ERR`, multiline `.\r\n` (CAPA/LIST/RETR), STLS → Encrypted | APOP crypto, POP3S decrypt, SASL AUTH |
| FTP | RFC 959 + AUTH TLS | [RFC 959](https://www.rfc-editor.org/rfc/rfc959.html), [RFC 4217](https://www.rfc-editor.org/rfc/rfc4217.html) | 220 greeting, USER/PASS, PASV/PORT, RETR/STOR, multiline replies, AUTH TLS → Encrypted | FTP data connection (port 20 / PASV socket), FTPS decrypt, SFTP/SSH |
| Oracle TNS | Connect/Accept/Refuse/Data | TNS connect packet + `tns.yaml` / `tns_fields.yaml` | Connect, Accept, Refuse, Data, version, fail-closed short header | Native crypto negotiation, SQL*Net payload schema |
| RADIUS | RFC 2865 Access | [RFC 2865](https://www.rfc-editor.org/rfc/rfc2865.html) | Access-Request/Accept/Reject, Identifier correlation, common attributes, UDP datagram association | EAP methods, RADIUS/TLS, decrypt of hidden attributes without keys |
| DHCP | RFC 2131/2132 | [RFC 2131](https://www.rfc-editor.org/rfc/rfc2131.html) | Discover/Offer/Request/Ack, xid, options cookie, chaddr | DHCPv6, failover, lease state machine as network truth |
| NTP | RFC 5905 | [RFC 5905](https://www.rfc-editor.org/rfc/rfc5905.html) | v4 mode 3/4 header, stratum, originate/receive/transmit timestamps | NTS, control/private modes, auth |
| CoAP | RFC 7252 | [RFC 7252](https://www.rfc-editor.org/rfc/rfc7252.html) | CON/NON/ACK/RST, token, Uri-Path, 2.05/4.xx codes | OSCORE, CoAP-over-TCP/WebSocket, observe |
| Modbus TCP | MBAP + PDU | [Modbus TCP](https://modbus.org/docs/Modbus_Messaging_Implementation_Guide_V1_0b.pdf) | MBAP length framing, Unit ID, FC 1–6/15/16, exception codes | Modbus RTU/ASCII, TLS Modbus, file record functions |

Standard ports are not protocol truth. STARTTLS/STLS/AUTH TLS without caller keys
is an Encrypted boundary, not a decrypted application tree.
