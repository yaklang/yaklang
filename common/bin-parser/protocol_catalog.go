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
	{Name: "ICMP", Layer: "L3", RuleFile: "internet_control_message_protocol.yaml", Status: statusPartial, Notes: "echo, dest-unreach, time-exceeded"},
	{Name: "ICMPv6", Layer: "L3", RuleFile: "internet_control_message_protocol_v6.yaml", Status: statusPartial},
	{Name: "GRE", Layer: "L3", RuleFile: "generic_routing_encapsulation.yaml", Status: statusStable},
	{Name: "TCP", Layer: "L4", RuleFile: "transmission_control_protocol.yaml", Status: statusStable},
	{Name: "UDP", Layer: "L4", RuleFile: "user_datagram_protocol.yaml", Status: statusStable},
	{Name: "DNS", Layer: "L7", RuleFile: "application-layer/dns.yaml", Status: statusStable, SampleFrom: "existing ethernet fixtures"},
	{Name: "HTTP", Layer: "L7", RuleFile: "application-layer/http.yaml", Status: statusPartial, SampleFrom: "existing ethernet fixtures"},
	{Name: "TLS", Layer: "L7", RuleFile: "application-layer/tls.yaml", Status: statusPartial, SampleFrom: "existing ethernet fixtures"},
	{Name: "DHCP", Layer: "L7", RuleFile: "application-layer/dhcp.yaml", Status: statusNew, SampleFrom: "gopacket DHCPv4 (RFC 2131)"},
	{Name: "NTP", Layer: "L7", RuleFile: "application-layer/ntp.yaml", Status: statusNew, SampleFrom: "Wireshark SampleCaptures NTP_sync.pcap (via gopacket testdata)"},
	{Name: "SNMP", Layer: "L7", RuleFile: "application-layer/snmp.yaml", Status: statusNew, Notes: "SNMPv1/v2c Get/Set/Response", SampleFrom: "RFC 1157 GetRequest"},
	{Name: "SSH", Layer: "L7", RuleFile: "application-layer/ssh.yaml", Status: statusNew, Notes: "identification string", SampleFrom: "RFC 4253"},
	{Name: "FTP", Layer: "L7", RuleFile: "application-layer/ftp.yaml", Status: statusNew, Notes: "reply line", SampleFrom: "RFC 959"},
	{Name: "SMTP", Layer: "L7", RuleFile: "application-layer/smtp.yaml", Status: statusNew, Notes: "reply line", SampleFrom: "RFC 5321"},
	{Name: "Redis", Layer: "L7", RuleFile: "application-layer/redis.yaml", Status: statusNew, Notes: "RESP arrays/bulk/simple", SampleFrom: "Redis RESP PING"},
	{Name: "MQTT", Layer: "L7", RuleFile: "application-layer/mqtt.yaml", Status: statusNew, Notes: "CONNECT/CONNACK + remaining-length check", SampleFrom: "MQTT 3.1.1 CONNECT"},
	{Name: "SOCKS5", Layer: "L7", RuleFile: "application-layer/socks5.yaml", Status: statusStable, SampleFrom: "existing socks5 fixtures"},
	{Name: "LDAP", Layer: "L7", RuleFile: "application-layer/ldap.yaml", Status: statusPartial},
	{Name: "NTLM", Layer: "L7", RuleFile: "application-layer/ntlm.yaml", Status: statusPartial},
	{Name: "MSRdp", Layer: "L7", RuleFile: "application-layer/msrdp.yaml", Status: statusPartial},
	{Name: "IIOP", Layer: "L7", RuleFile: "application-layer/iiop.yaml", Status: statusPartial},
	{Name: "T3", Layer: "L7", RuleFile: "application-layer/t3.yaml", Status: statusPartial},
	{Name: "PPTP", Layer: "L7", RuleFile: "application-layer/pptp.yaml", Status: statusPartial},
	{Name: "BER", Layer: "L7", RuleFile: "application-layer/ber.yaml", Status: statusPartial},
	{Name: "SMB2", Layer: "L7", RuleFile: "application-layer/smb2.yaml", Status: statusNew, Notes: "header + NEGOTIATE + SESSION_SETUP", SampleFrom: "[MS-SMB2] 2.2.1/2.2.3/2.2.4/2.2.5"},
	{Name: "SMB", Layer: "L7", RuleFile: "application-layer/smb.yaml", Status: statusNew, Notes: "header + NEGOTIATE dialects", SampleFrom: "[MS-CIFS] 2.2.3.1/2.2.4.52"},
	{Name: "NBT SS", Layer: "L4", RuleFile: "application-layer/nbss.yaml", Status: statusNew, Notes: "session message wrapping SMB/SMB2", SampleFrom: "RFC 1002"},
	{Name: "MySQL", Layer: "L7", RuleFile: "application-layer/mysql.yaml", Status: statusNew, Notes: "packet header + HandshakeV10/OK/ERR/COM", SampleFrom: "MySQL HandshakeV10 internals doc"},
}
