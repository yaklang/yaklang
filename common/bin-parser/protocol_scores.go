package bin_parser

// ProtocolScorecard is one delivery-standard score for a roadmap protocol.
// Dimension points must be one of the rubric buckets in PROTOCOL_DELIVERY.md.
type ProtocolScorecard struct {
	Name        string
	Rule        string
	AliasOf     string
	G1          bool
	G2          bool
	G3          bool
	G4          bool
	G5          bool
	G6          bool
	G7          bool
	G8          bool
	Schema      int
	Traffic     int
	Tests       int
	Branches    int
	Stack       int
	SampleClass string
	Evidence    string
	OpaqueRaw   string
}

func (s ProtocolScorecard) GatesOK() bool {
	return s.G1 && s.G2 && s.G3 && s.G4 && s.G5 && s.G6 && s.G7 && s.G8
}

func (s ProtocolScorecard) Total() int {
	if !s.GatesOK() {
		return 0
	}
	return s.Schema + s.Traffic + s.Tests + s.Branches + s.Stack
}

func (s ProtocolScorecard) Grade() string {
	t := s.Total()
	switch {
	case !s.GatesOK():
		return "F"
	case t >= 90:
		return "A"
	case t >= 75:
		return "B"
	case t >= 60:
		return "C"
	default:
		return "D"
	}
}

func card(name, rule string, schema, traffic, tests, branches, stack int, sample, evidence, opaque string) ProtocolScorecard {
	// G5 requires L1/L2/L3; L4-only handmade PDUs fail the gate (Total 0).
	// P0 cards keep G1–G4/G6–G8 true at construction (P0 tests do not re-derive).
	g5 := sample == "L1" || sample == "L2" || sample == "L3"
	return ProtocolScorecard{
		Name: name, Rule: rule,
		G1: true, G2: true, G3: true, G4: true, G5: g5, G6: true, G7: true, G8: true,
		Schema: schema, Traffic: traffic, Tests: tests, Branches: branches, Stack: stack,
		SampleClass: sample, Evidence: evidence, OpaqueRaw: opaque,
	}
}

// p1card records dimensions only. G1–G4/G6–G8 stay false until TestP1ScorecardsCovered
// fills them from rule file / fail paths / ethernet / catalog evidence.
func p1card(name, rule string, schema, traffic, tests, branches, stack int, sample, evidence, opaque string) ProtocolScorecard {
	s := card(name, rule, schema, traffic, tests, branches, stack, sample, evidence, opaque)
	s.G1, s.G2, s.G3, s.G4, s.G6, s.G7, s.G8 = false, false, false, false, false, false, false
	return s
}

func alias(name, of string) ProtocolScorecard {
	return ProtocolScorecard{Name: name, AliasOf: of}
}

// P0Scorecards records delivery scores for every P0 roadmap name.
// Aliases share the target card via AliasOf; the test expands them.
var P0Scorecards = []ProtocolScorecard{
	card("Ethernet II", "ethernet.yaml", 20, 25, 20, 20, 10, "L1",
		"ethernet/next-type IEEE 802 0x88B5 Next Protocol Data; ethernet/ptp IEEE 1588-2008 Sync Sequence ID 1 EtherType 0x88F7; TestEthernetIPARPTruncated", ""),
	card("IEEE 802.1Q", "ieee_802_1q.yaml", 20, 8, 20, 20, 10, "L3",
		"TestVLANIPv4ARPInner ARP; TestVLANPayloadTypeArms IP/IPv6/EAPOL/default; TestVLANFailPaths", ""),
	card("ARP", "address_resolution_protocol.yaml", 20, 25, 16, 14, 10, "L1",
		"TestBaseProtocol arp; TestVLANIPv4ARPInner", ""),
	card("IPv4", "internet_protocol.yaml", 20, 25, 16, 14, 10, "L1",
		"TestBaseProtocol; TestEthernetIPARPTruncated", ""),
	card("IPv6", "internet_protocol_version_6.yaml", 20, 25, 16, 14, 10, "L1",
		"TestBaseProtocol icmp v6", ""),
	card("ICMP", "internet_control_message_protocol.yaml", 20, 25, 20, 20, 10, "L1",
		"TestBaseProtocol icmp; TestICMPDestinationUnreachable; TestICMPTimestampRedirectAndNBNSAnswer", "inner original datagram"),
	card("ICMPv6", "internet_control_message_protocol_v6.yaml", 20, 25, 16, 14, 10, "L1",
		"TestBaseProtocol icmp v6", ""),
	card("TCP", "transmission_control_protocol.yaml", 25, 25, 20, 20, 10, "L1",
		"tcp/mss gopacket SYN MSS 8192; tcp/timestamp RFC 7323 TS Val/Echo EtherType 0x0800; TestP1FailPaths TCP", ""),
	card("UDP", "user_datagram_protocol.yaml", 20, 25, 16, 14, 10, "L1",
		"TestBaseProtocol dns; TestNTPWiresharkSample", "application payload"),
	card("QUIC", "application-layer/quic.yaml", 20, 15, 16, 14, 10, "L2",
		"TestQUICHeadersAndEdges; RFC 9000 long/short header + UDP/443", "protected payload"),
	card("NetBIOS", "application-layer/nbss.yaml", 20, 15, 16, 14, 10, "L2",
		"TestNBSSWrapsSMB2; TestNBNSLLMNRAndEdges; TestNBSSFailPaths", ""),
	card("NBT NS", "application-layer/nbns.yaml", 20, 15, 16, 20, 10, "L2",
		"TestNBNSLLMNRAndEdges; TestICMPTimestampRedirectAndNBNSAnswer", "RDATA opaque"),
	card("NBT SS", "application-layer/nbss.yaml", 20, 15, 16, 14, 10, "L2",
		"TestNBSSWrapsSMB2 RFC 1002 session header; TestNBSSFailPaths", ""),
	card("DNS", "application-layer/dns.yaml", 20, 25, 20, 20, 10, "L1",
		"TestBaseProtocol dns request/response; TestDNSQueryFromExistingEthernetFixture; TestDNSFailPaths", "RDATA"),
	card("NBNS", "application-layer/nbns.yaml", 20, 15, 16, 20, 10, "L2",
		"TestNBNSLLMNRAndEdges RFC 1002", "RDATA"),
	card("DHCP", "application-layer/dhcp.yaml", 20, 8, 20, 20, 10, "L3",
		"TestDHCPGopacketDiscover; TestDHCPOfferYourIP; TestDHCPFailPaths (bad op/cookie/trunc)", ""),
	card("HTTP", "application-layer/http.yaml", 20, 25, 20, 20, 10, "L1",
		"TestBaseProtocol http request; http/post Content-Length foo=bar; http/chunked RFC 9112 hello; TestHTTPFailPaths", ""),
	card("HTTP/2", "application-layer/http2.yaml", 20, 20, 20, 20, 10, "L2",
		"TestHTTP2RFC9113SettingsTwoParams; http2/data hello; http2/rst PROTOCOL_ERROR; http2/goaway debug bye", ""),
	card("WebSocket", "application-layer/websocket.yaml", 25, 20, 20, 20, 10, "L2",
		"TestWebSocketRFC6455UnmaskedHello RFC 6455 §5.7 Hello Text; TestP1WiresharkAndRFCSamples close 1000 Ethernet+TCP/8080", ""),
	card("TLS", "application-layer/tls.yaml", 20, 25, 20, 20, 10, "L1",
		"TestTLSClientHelloFromCapture scapy-ssl_tls; TestTLSRecordGatesAndClientHello; TestBaseProtocol tls", "app-data Payload"),
	card("JA3/JA4", "application-layer/tls_hello.yaml", 20, 25, 16, 14, 10, "L1",
		"tls/suites 0x002f 0x0035; tls/sni RFC 6066 example.com; TestTLSClientHelloFromCapture", "JA3 hash not computed"),
	card("SMTP", "application-layer/smtp.yaml", 20, 15, 16, 14, 10, "L2",
		"TestSMTPReply RFC 5321; TestSSHPacketAndFTPSMTPCommands EHLO", ""),
	card("FTP", "application-layer/ftp.yaml", 20, 15, 16, 14, 10, "L2",
		"TestFTPReply RFC 959; TestSSHPacketAndFTPSMTPCommands USER", ""),
	card("SMB", "application-layer/smb.yaml", 20, 15, 20, 20, 10, "L2",
		"smb/tree-connect [MS-CIFS] 2.2.4.55 Path \\\\srv\\share Service A:; TestSMB1NegotiateRequest", ""),
	card("SMB2", "application-layer/smb2.yaml", 20, 15, 20, 20, 10, "L2",
		"smb2/tree-connect Path; smb2/create file.txt; smb2/read Octets; TestSMB2NegotiateRequest", ""),
	card("SMB3", "application-layer/smb3.yaml", 20, 15, 16, 14, 10, "L2",
		"TestJavaSerAndSMB3AndEdges [MS-SMB2] 2.2.41 \\xfdSMB transform", "encrypted payload"),
	alias("CIFS", "SMB"),
	card("LDAP", "application-layer/ldap.yaml", 20, 20, 16, 14, 10, "L2",
		"TestLDAPAnonymousBindSample RFC 4511; TestSMB1CloseTransactionLDAPJavaQUIC", "long-form BER not in this rule"),
	card("Kerberos", "application-layer/kerberos.yaml", 20, 15, 16, 20, 10, "L2",
		"TestKerberosTagsAndEdges RFC 4120 APPLICATION tags; TestKerberosEmptyBodyAndTCPCap", "SEQUENCE body Content"),
	card("NTLM", "application-layer/ntlm.yaml", 20, 25, 20, 20, 10, "L2",
		"ntlm/challenge [MS-NLMP] 2.2.1.2 Target Name DOMAIN; ntlm/authenticate User Name Admin TCP/445; TestNTLMSSPNegotiateAndEdges", "Value"),
	card("NTLMSSP", "application-layer/ntlm.yaml", 20, 25, 20, 20, 10, "L2",
		"ntlm/challenge Target Name DOMAIN; ntlm/authenticate User Name Admin SMB2 Session Setup; TestNTLMSSPInsideSMB2SessionSetup", "Value"),
	card("SPNEGO", "application-layer/spnego.yaml", 20, 15, 16, 14, 10, "L2",
		"TestSPNEGOAndEdges RFC 4178 GSS-API 0x60 + SPNEGO OID", "NegToken blob"),
	card("RADIUS", "application-layer/radius.yaml", 25, 25, 20, 20, 10, "L1",
		"radius/user-name Wireshark radtest.pcap User-Name Admin; radius/nas-ip 127.0.0.1 UDP/1812; TestRADIUSAndEdges", ""),
	card("SOCKS5", "application-layer/socks5.yaml", 20, 20, 16, 20, 10, "L2",
		"TestSOCKS5ConnectGoogleFromFixture; TestSocks5; TestSOCKS5FailPaths", ""),
	card("SSH", "application-layer/ssh.yaml", 20, 15, 16, 20, 10, "L2",
		"TestSSHIdentification RFC 4253; TestSSHPacketAndFTPSMTPCommands", "KEXINIT Data"),
	card("RDP", "application-layer/msrdp.yaml", 20, 20, 16, 20, 10, "L1",
		"TestRDPTPKTFromProtocolImplCapture; TestRDPX224CookieNegotiationAndTPKTABI", "TPDU raw ABI"),
	card("PsExec/SMB-svcctl", "application-layer/dcerpc.yaml", 20, 15, 16, 20, 10, "L2",
		"TestMSRPCInterfaceUUIDsAndPsExecOpnums SVCCTL OpenSCManagerW opnum 15", "NDR stub"),
	card("MySQL", "application-layer/mysql.yaml", 20, 15, 16, 14, 10, "L2",
		"TestMySQLHandshakeV10; TestMySQLQueryCommand; TestMySQLERRPacket; TestMySQLFailPaths", ""),
	card("PostgreSQL", "application-layer/postgresql.yaml", 20, 15, 16, 20, 10, "L2",
		"TestPostgreSQLStartupSSLQueryAndEdges Query SELECT 1 + Error SQLSTATE 42601; SSLRequest 80877103", ""),
	card("MSSQL TDS", "application-layer/tds.yaml", 20, 15, 16, 20, 10, "L2",
		"TestTDSPreloginLoginBatchAndEdges [MS-TDS] 2.2.1", "BATCH payload"),
	card("Oracle TNS", "application-layer/tns.yaml", 20, 15, 16, 20, 10, "L2",
		"tns/connect SERVICE_NAME=ORCL; tns/data Data Flag+Octets; TestTNSConnectAndEdges", ""),
	card("Redis", "application-layer/redis.yaml", 20, 15, 16, 20, 10, "L2",
		"TestRedisPING RESP *1 $4 PING; TestRedisFailPaths", ""),
	card("MQTT", "application-layer/mqtt.yaml", 20, 20, 16, 20, 10, "L2",
		"TestMQTTConnectAndConnack; TestMQTTPublishSubscribePing; mqtt/publish Topic+Message; mqtt/qos1 Packet ID", ""),
	card("DCE/RPC", "application-layer/dcerpc.yaml", 20, 15, 16, 20, 10, "L2",
		"TestDCERPCBindRequestAndEdges [MS-RPCE] 2.2.2.6; TestTNSConnectSNMPVarbindX224AndDCERPCStub", "NDR stub"),
	alias("MSRPC", "DCE/RPC"),
	card("SNMP", "application-layer/snmp.yaml", 20, 20, 16, 14, 10, "L2",
		"TestSNMPRFC1157GetRequestOID; TestSNMPGetRequest; TestSNMPFailPaths", ""),
	card("MSRPC EPM", "application-layer/dcerpc.yaml", 20, 15, 16, 20, 10, "L2",
		"TestMSRPCInterfaceUUIDsAndPsExecOpnums EPM UUID + opnum 3", "NDR stub"),
	card("SAMR", "application-layer/dcerpc.yaml", 20, 15, 16, 20, 10, "L2",
		"TestMSRPCInterfaceUUIDsAndPsExecOpnums SAMR UUID", "NDR stub"),
	card("LSARPC", "application-layer/dcerpc.yaml", 20, 15, 16, 20, 10, "L2",
		"TestMSRPCInterfaceUUIDsAndPsExecOpnums LSARPC UUID + opnum 6", "NDR stub"),
	card("NETLOGON", "application-layer/dcerpc.yaml", 20, 15, 16, 20, 10, "L2",
		"TestMSRPCInterfaceUUIDsAndPsExecOpnums NETLOGON UUID + opnum 26", "NDR stub"),
	card("DRSUAPI", "application-layer/dcerpc.yaml", 20, 15, 16, 20, 10, "L2",
		"TestMSRPCInterfaceUUIDsAndPsExecOpnums DRSUAPI UUID + opnum 0", "NDR stub"),
	card("SRVSVC", "application-layer/dcerpc.yaml", 20, 15, 16, 20, 10, "L2",
		"TestMSRPCInterfaceUUIDsAndPsExecOpnums SRVSVC UUID + opnum 15", "NDR stub"),
	card("SVCCTL", "application-layer/dcerpc.yaml", 20, 15, 16, 20, 10, "L2",
		"TestMSRPCInterfaceUUIDsAndPsExecOpnums SVCCTL UUID + opnum 15", "NDR stub"),
	card("WINREG", "application-layer/dcerpc.yaml", 20, 15, 16, 20, 10, "L2",
		"TestMSRPCInterfaceUUIDsAndPsExecOpnums WINREG UUID + opnum 2", "NDR stub"),
	card("WMI", "application-layer/dcerpc.yaml", 20, 15, 16, 20, 10, "L2",
		"TestMSRPCInterfaceUUIDsAndPsExecOpnums WMI UUID + opnum 6", "NDR stub"),
	card("DCOM", "application-layer/dcerpc.yaml", 20, 15, 16, 20, 10, "L2",
		"TestMSRPCInterfaceUUIDsAndPsExecOpnums DCOM UUID + opnum 5", "NDR stub"),
	card("Yakit proxy framing", "application-layer/http.yaml", 20, 25, 20, 20, 10, "L1",
		"TestTLSClientHelloJA3AndHTTPWPAD CONNECT", ""),
	card("AJP", "application-layer/ajp.yaml", 20, 15, 16, 14, 10, "L2",
		"TestAJPPingPongForwardAndEdges Apache AJP13 CPING 0x1234", ""),
	card("Java serialization", "application-layer/java_ser.yaml", 20, 20, 16, 14, 10, "L2",
		"TestSMB1CloseTransactionLDAPJavaQUIC STREAM_MAGIC ACED v5 TC_NULL", "object graph"),
	card("Kerberos PAC", "application-layer/kerberos.yaml", 20, 15, 16, 20, 10, "L2",
		"TestNetNTLMv2AndPAC [MS-PAC] 2.3 PAC_INFO_BUFFER list", "buffer payload"),
	alias("AS-REP / TGS", "Kerberos"),
	alias("NTLM v1/v2", "NTLMSSP"),
	card("NetNTLMv2", "application-layer/ntlm.yaml", 20, 15, 16, 14, 10, "L2",
		"TestNetNTLMv2AndPAC [MS-NLMP] 3.3.2", ""),
	card("LLMNR response", "application-layer/nbns.yaml", 20, 15, 16, 14, 10, "L2",
		"TestNBNSLLMNRAndEdges UDP/5355", "RDATA"),
	card("NBT-NS response", "application-layer/nbns.yaml", 20, 15, 16, 20, 10, "L2",
		"TestNBNSLLMNRAndEdges UDP/137", "RDATA"),
	card("WPAD proxy", "application-layer/http.yaml", 20, 25, 20, 20, 10, "L1",
		"TestTLSClientHelloJA3AndHTTPWPAD GET /wpad.dat", ""),
	alias("TNS", "Oracle TNS"),
}

func ResolveP0Scorecard(name string) (ProtocolScorecard, bool) {
	byName := make(map[string]ProtocolScorecard, len(P0Scorecards))
	for _, s := range P0Scorecards {
		byName[s.Name] = s
	}
	s, ok := byName[name]
	if !ok {
		return ProtocolScorecard{}, false
	}
	if s.AliasOf == "" {
		return s, true
	}
	base, ok := byName[s.AliasOf]
	if !ok {
		return ProtocolScorecard{}, false
	}
	base.Name = name
	base.AliasOf = s.AliasOf
	return base, true
}
