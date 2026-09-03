package bin_parser

// ProtocolInfo describes one dissector in the YAML rule set.
type ProtocolInfo struct {
	Name       string
	Layer      string
	RuleFile   string
	Status     string
	Notes      string
	SampleFrom string
}

const (
	statusStable  = "stable"
	statusPartial = "partial"
	statusNew     = "new"
)

// ProtocolCatalog is the protocol coverage list for the Wireshark-like
// binary parser. Status is "stable" when existing tests already lock
// behavior, "new" for dissectors added in this work, and "partial" when
// only a subset of PDUs/fields is decoded.
var ProtocolCatalog = []ProtocolInfo{
	{Name: "Ethernet", Layer: "L2", RuleFile: "ethernet.yaml", Status: statusStable, SampleFrom: "existing ethernet fixtures"},
	{Name: "IEEE 802.1Q", Layer: "L2", RuleFile: "ieee_802_1q.yaml", Status: statusNew, SampleFrom: "gopacket Dot1Q (Wireshark-compatible)"},
	{Name: "ARP", Layer: "L2", RuleFile: "address_resolution_protocol.yaml", Status: statusStable, SampleFrom: "existing ethernet fixtures"},
	{Name: "PPP", Layer: "L2", RuleFile: "ppp.yaml", Status: statusStable, SampleFrom: "existing PPP/GRE fixtures"},
	{Name: "LCP", Layer: "L2", RuleFile: "link_control_protocol.yaml", Status: statusStable},
	{Name: "PAP", Layer: "L2", RuleFile: "password_authentication_protocol.yaml", Status: statusStable},
	{Name: "CHAP", Layer: "L2", RuleFile: "challenge_handshake_authentication_protocol.yaml", Status: statusStable},
	{Name: "EAPOL", Layer: "L2", RuleFile: "eapol.yaml", Status: statusPartial},
	{Name: "IPv4", Layer: "L3", RuleFile: "internet_protocol.yaml", Status: statusStable, SampleFrom: "existing ethernet fixtures"},
	{Name: "IPv6", Layer: "L3", RuleFile: "internet_protocol_version_6.yaml", Status: statusStable, SampleFrom: "existing ethernet fixtures"},
	{Name: "ICMP", Layer: "L3", RuleFile: "internet_control_message_protocol.yaml", Status: statusNew, Notes: "echo, dest-unreach, time-exceeded, redirect, timestamp"},
	{Name: "ICMPv6", Layer: "L3", RuleFile: "internet_control_message_protocol_v6.yaml", Status: statusNew, Notes: "echo/NS/NA/RS/RA"},
	{Name: "GRE", Layer: "L3", RuleFile: "generic_routing_encapsulation.yaml", Status: statusStable},
	{Name: "TCP", Layer: "L4", RuleFile: "transmission_control_protocol.yaml", Status: statusStable},
	{Name: "UDP", Layer: "L4", RuleFile: "user_datagram_protocol.yaml", Status: statusStable},
	{Name: "DNS", Layer: "L7", RuleFile: "application-layer/dns.yaml", Status: statusStable, SampleFrom: "existing ethernet fixtures"},
	{Name: "HTTP", Layer: "L7", RuleFile: "application-layer/http.yaml", Status: statusNew, Notes: "request/response + CONNECT + WPAD", SampleFrom: "existing ethernet fixtures"},
	{Name: "TLS", Layer: "L7", RuleFile: "application-layer/tls.yaml", Status: statusPartial, SampleFrom: "existing ethernet fixtures"},
	{Name: "DHCP", Layer: "L7", RuleFile: "application-layer/dhcp.yaml", Status: statusNew, SampleFrom: "gopacket DHCPv4 (RFC 2131)"},
	{Name: "NTP", Layer: "L7", RuleFile: "application-layer/ntp.yaml", Status: statusNew, SampleFrom: "Wireshark SampleCaptures NTP_sync.pcap (via gopacket testdata)"},
	{Name: "SNMP", Layer: "L7", RuleFile: "application-layer/snmp.yaml", Status: statusNew, Notes: "SNMPv1/v2c Get/Set + varbind SEQUENCE", SampleFrom: "RFC 1157 GetRequest"},
	{Name: "SSH", Layer: "L7", RuleFile: "application-layer/ssh.yaml", Status: statusNew, Notes: "identification + binary packet", SampleFrom: "RFC 4253"},
	{Name: "FTP", Layer: "L7", RuleFile: "application-layer/ftp.yaml", Status: statusNew, Notes: "reply + commands", SampleFrom: "RFC 959"},
	{Name: "SMTP", Layer: "L7", RuleFile: "application-layer/smtp.yaml", Status: statusNew, Notes: "reply + commands", SampleFrom: "RFC 5321"},
	{Name: "Redis", Layer: "L7", RuleFile: "application-layer/redis.yaml", Status: statusNew, Notes: "RESP arrays/bulk/simple", SampleFrom: "Redis RESP PING"},
	{Name: "MQTT", Layer: "L7", RuleFile: "application-layer/mqtt.yaml", Status: statusNew, Notes: "CONNECT/CONNACK + remaining-length check", SampleFrom: "MQTT 3.1.1 CONNECT"},
	{Name: "SOCKS5", Layer: "L7", RuleFile: "application-layer/socks5.yaml", Status: statusStable, SampleFrom: "existing socks5 fixtures"},
	{Name: "LDAP", Layer: "L7", RuleFile: "application-layer/ldap.yaml", Status: statusNew, Notes: "SEQUENCE + bind/unbind/search"},
	{Name: "NTLM", Layer: "L7", RuleFile: "application-layer/ntlm.yaml", Status: statusPartial},
	{Name: "MSRdp", Layer: "L7", RuleFile: "application-layer/msrdp.yaml", Status: statusPartial},
	{Name: "IIOP", Layer: "L7", RuleFile: "application-layer/iiop.yaml", Status: statusPartial},
	{Name: "T3", Layer: "L7", RuleFile: "application-layer/t3.yaml", Status: statusPartial},
	{Name: "PPTP", Layer: "L7", RuleFile: "application-layer/pptp.yaml", Status: statusPartial},
	{Name: "BER", Layer: "L7", RuleFile: "application-layer/ber.yaml", Status: statusPartial},
	{Name: "SMB2", Layer: "L7", RuleFile: "application-layer/smb2.yaml", Status: statusNew, Notes: "header + NEGOTIATE + SESSION_SETUP", SampleFrom: "[MS-SMB2] 2.2.1/2.2.3/2.2.4/2.2.5"},
	{Name: "SMB", Layer: "L7", RuleFile: "application-layer/smb.yaml", Status: statusNew, Notes: "NEGOTIATE/AndX/CLOSE/TRANSACTION", SampleFrom: "[MS-CIFS] 2.2.3.1/2.2.4.52"},
	{Name: "NBT SS", Layer: "L4", RuleFile: "application-layer/nbss.yaml", Status: statusNew, Notes: "session message wrapping SMB/SMB2", SampleFrom: "RFC 1002"},
	{Name: "MySQL", Layer: "L7", RuleFile: "application-layer/mysql.yaml", Status: statusNew, Notes: "packet header + HandshakeV10/OK/ERR/COM", SampleFrom: "MySQL HandshakeV10 internals doc"},
	{Name: "NTLMSSP", Layer: "L7", RuleFile: "application-layer/ntlm.yaml", Status: statusNew, Notes: "signature + type 1/2/3 dispatcher", SampleFrom: "[MS-NLMP] 2.2.1"},
	{Name: "Kerberos", Layer: "L7", RuleFile: "application-layer/kerberos.yaml", Status: statusNew, Notes: "APPLICATION tags AS/TGS/AP/ERROR, TCP record", SampleFrom: "RFC 4120"},
	{Name: "PostgreSQL", Layer: "L7", RuleFile: "application-layer/postgresql.yaml", Status: statusNew, Notes: "startup/SSL/cancel + typed messages", SampleFrom: "PostgreSQL protocol 3.0"},
	{Name: "MSSQL TDS", Layer: "L7", RuleFile: "application-layer/tds.yaml", Status: statusNew, Notes: "TDS header PRELOGIN/LOGIN7/BATCH/RESPONSE", SampleFrom: "[MS-TDS] 2.2.1"},
	{Name: "AJP", Layer: "L7", RuleFile: "application-layer/ajp.yaml", Status: statusNew, Notes: "AJP13 magic 0x1234/AB, CPING/FORWARD", SampleFrom: "Apache AJP13"},
	{Name: "SPNEGO", Layer: "L7", RuleFile: "application-layer/spnego.yaml", Status: statusNew, Notes: "GSS-API 0x60 + OID", SampleFrom: "RFC 4178"},
	{Name: "NetNTLMv2", Layer: "L7", RuleFile: "application-layer/ntlm.yaml", Status: statusNew, Notes: "NTProofStr + client challenge blob", SampleFrom: "[MS-NLMP] 3.3.2"},
	{Name: "Kerberos PAC", Layer: "L7", RuleFile: "application-layer/kerberos.yaml", Status: statusNew, Notes: "PAC_INFO_BUFFER list", SampleFrom: "[MS-PAC] 2.3"},
	{Name: "HTTP/2", Layer: "L7", RuleFile: "application-layer/http2.yaml", Status: statusNew, Notes: "preface + frame header", SampleFrom: "RFC 9113"},
	{Name: "WebSocket", Layer: "L7", RuleFile: "application-layer/websocket.yaml", Status: statusNew, Notes: "RFC 6455 frame", SampleFrom: "RFC 6455"},
	{Name: "QUIC", Layer: "L4", RuleFile: "application-layer/quic.yaml", Status: statusNew, Notes: "long/short header + CID", SampleFrom: "RFC 9000"},
	{Name: "RADIUS", Layer: "L7", RuleFile: "application-layer/radius.yaml", Status: statusNew, Notes: "code/id/length/authenticator + attrs", SampleFrom: "RFC 2865"},
	{Name: "Oracle TNS", Layer: "L7", RuleFile: "application-layer/tns.yaml", Status: statusNew, Notes: "TNS packet header types 1-15", SampleFrom: "Oracle TNS"},
	{Name: "TNS", Layer: "L7", RuleFile: "application-layer/tns.yaml", Status: statusNew, Notes: "alias of Oracle TNS", SampleFrom: "Oracle TNS"},
	{Name: "DCE/RPC", Layer: "L7", RuleFile: "application-layer/dcerpc.yaml", Status: statusNew, Notes: "v5 header + bind/request", SampleFrom: "[MS-RPCE] 2.2.2.6"},
	{Name: "MSRPC", Layer: "L7", RuleFile: "application-layer/dcerpc.yaml", Status: statusNew, Notes: "same as DCE/RPC", SampleFrom: "[MS-RPCE]"},
	{Name: "MSRPC EPM", Layer: "L7", RuleFile: "application-layer/dcerpc.yaml", Status: statusNew, Notes: "bind UUID + request opnum", SampleFrom: "[MS-RPCE] epmapper"},
	{Name: "SAMR", Layer: "L7", RuleFile: "application-layer/dcerpc.yaml", Status: statusNew, Notes: "DCE/RPC bind UUID", SampleFrom: "[MS-SAMR]"},
	{Name: "LSARPC", Layer: "L7", RuleFile: "application-layer/dcerpc.yaml", Status: statusNew, Notes: "DCE/RPC bind UUID", SampleFrom: "[MS-LSAT]"},
	{Name: "NETLOGON", Layer: "L7", RuleFile: "application-layer/dcerpc.yaml", Status: statusNew, Notes: "DCE/RPC bind UUID", SampleFrom: "[MS-NRPC]"},
	{Name: "DRSUAPI", Layer: "L7", RuleFile: "application-layer/dcerpc.yaml", Status: statusNew, Notes: "DCE/RPC bind UUID", SampleFrom: "[MS-DRSR]"},
	{Name: "SRVSVC", Layer: "L7", RuleFile: "application-layer/dcerpc.yaml", Status: statusNew, Notes: "DCE/RPC bind UUID", SampleFrom: "[MS-SRVS]"},
	{Name: "SVCCTL", Layer: "L7", RuleFile: "application-layer/dcerpc.yaml", Status: statusNew, Notes: "DCE/RPC bind UUID", SampleFrom: "[MS-SCMR]"},
	{Name: "WINREG", Layer: "L7", RuleFile: "application-layer/dcerpc.yaml", Status: statusNew, Notes: "DCE/RPC bind UUID", SampleFrom: "[MS-RRP]"},
	{Name: "WMI", Layer: "L7", RuleFile: "application-layer/dcerpc.yaml", Status: statusNew, Notes: "DCOM/WMI via DCE/RPC header", SampleFrom: "[MS-WMI]"},
	{Name: "DCOM", Layer: "L7", RuleFile: "application-layer/dcerpc.yaml", Status: statusNew, Notes: "DCOM via DCE/RPC header", SampleFrom: "[MS-DCOM]"},
	{Name: "PsExec/SMB-svcctl", Layer: "L7", RuleFile: "application-layer/dcerpc.yaml", Status: statusNew, Notes: "SVCCTL OpenSCManagerW opnum 15", SampleFrom: "[MS-SCMR] 3.1.4"},
	{Name: "Java serialization", Layer: "L7", RuleFile: "application-layer/java_ser.yaml", Status: statusNew, Notes: "STREAM_MAGIC ACED v5 + TC type", SampleFrom: "Java Object Serialization Spec"},
	{Name: "SMB3", Layer: "L7", RuleFile: "application-layer/smb3.yaml", Status: statusNew, Notes: "transform \\xfdSMB", SampleFrom: "[MS-SMB2] 2.2.41"},
	{Name: "NBNS", Layer: "L7", RuleFile: "application-layer/nbns.yaml", Status: statusNew, Notes: "NetBIOS name service header", SampleFrom: "RFC 1002"},
	{Name: "NBT NS", Layer: "L4", RuleFile: "application-layer/nbns.yaml", Status: statusNew, Notes: "same as NBNS", SampleFrom: "RFC 1002"},
	{Name: "NetBIOS", Layer: "L4", RuleFile: "application-layer/nbss.yaml", Status: statusNew, Notes: "session + name service", SampleFrom: "RFC 1001/1002"},
	{Name: "LLMNR poison", Layer: "L7", RuleFile: "application-layer/nbns.yaml", Status: statusNew, Notes: "LLMNR is DNS-shaped on UDP 5355", SampleFrom: "RFC 4795"},
	{Name: "NBT-NS poison", Layer: "L7", RuleFile: "application-layer/nbns.yaml", Status: statusNew, Notes: "NBNS query/response", SampleFrom: "RFC 1002"},
	{Name: "JA3/JA4", Layer: "L7", RuleFile: "application-layer/tls_hello.yaml", Status: statusNew, Notes: "ClientHello cipher suites for fingerprint", SampleFrom: "TLS ClientHello"},
	{Name: "WPAD proxy", Layer: "L7", RuleFile: "application-layer/http.yaml", Status: statusNew, Notes: "HTTP GET /wpad.dat", SampleFrom: "WPAD"},
	{Name: "Yakit MITM magic", Layer: "L7", RuleFile: "application-layer/http.yaml", Status: statusNew, Notes: "HTTP CONNECT intercept", SampleFrom: "yak-mitm CONNECT"},
}
