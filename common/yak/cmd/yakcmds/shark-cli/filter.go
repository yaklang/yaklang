package sharkcli

import (
	"fmt"
	"sort"
	"strings"
)

// Application shortcuts are capture filters for conventional ports, not a
// promise that every packet on those ports contains that application protocol.
var protocolFilters = map[string]string{
	"ip": "ip", "ipv4": "ip", "ipv6": "ip6", "arp": "arp",
	"tcp": "tcp", "udp": "udp", "icmp": "icmp", "icmpv6": "icmp6", "sctp": "sctp",
	"vlan": "vlan", "gre": "ip proto 47 or ip6 proto 47", "esp": "ip proto 50 or ip6 proto 50",
	"igmp": "igmp", "ospf": "ip proto 89 or ip6 proto 89", "eapol": "ether proto 0x888e",
	"lldp": "ether proto 0x88cc", "ptp": "ether proto 0x88f7 or udp port 319 or udp port 320",
	"dns": "port 53", "mdns": "udp port 5353", "llmnr": "udp port 5355",
	"http": "tcp and (port 80 or port 8080 or port 8000)", "http2": "tcp and (port 80 or port 443 or port 8080)",
	"https": "tcp port 443", "tls": "tcp and (port 443 or port 8443)", "quic": "udp port 443",
	"dtls": "udp and (port 443 or port 4433)", "ssh": "tcp port 22", "ftp": "tcp port 21", "ftp-data": "tcp port 20",
	"smtp": "tcp and (port 25 or port 465 or port 587)", "pop3": "tcp and (port 110 or port 995)",
	"imap": "tcp and (port 143 or port 993)", "telnet": "tcp port 23", "dhcp": "udp and (port 67 or port 68)",
	"dhcpv6": "udp and (port 546 or port 547)", "ntp": "udp port 123", "snmp": "udp and (port 161 or port 162)",
	"mysql": "tcp port 3306", "postgresql": "tcp port 5432", "redis": "tcp port 6379", "mongodb": "tcp port 27017",
	"mqtt": "tcp and (port 1883 or port 8883)", "amqp": "tcp and (port 5672 or port 5671)", "kafka": "tcp port 9092",
	"memcached": "port 11211", "smb": "tcp and (port 139 or port 445)", "smb2": "tcp port 445", "smb3": "tcp port 445",
	"rdp": "port 3389", "ldap": "port 389", "ldaps": "tcp port 636", "kerberos": "port 88",
	"nbns": "udp port 137", "nbss": "tcp port 139", "nbt-dg": "udp port 138",
	"radius": "udp and (port 1812 or port 1813 or port 1645 or port 1646)", "tftp": "udp port 69",
	"syslog": "port 514", "sip": "port 5060", "rtp": "udp port 5004", "rtcp": "udp port 5005",
	"stun": "port 3478", "ssdp": "udp port 1900", "vxlan": "udp port 4789", "l2tp": "udp port 1701",
	"ike": "udp and (port 500 or port 4500)", "wireguard": "udp port 51820", "openvpn": "port 1194",
	"bgp": "tcp port 179", "rip": "udp port 520", "vnc": "tcp port 5900", "socks": "tcp port 1080",
	"rtsp": "port 554", "rtmp": "tcp port 1935", "ipmi": "udp port 623", "tacacs": "tcp port 49",
	"dcerpc": "tcp port 135", "onc-rpc": "port 111", "ajp": "tcp port 8009", "tds": "tcp port 1433",
}

func protocolNames() []string {
	names := make([]string, 0, len(protocolFilters))
	for name := range protocolFilters {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func buildFilter(protocols []string, bpf string) (string, error) {
	var shortcuts []string
	seen := map[string]bool{}
	for _, group := range protocols {
		for _, name := range strings.Split(group, ",") {
			name = strings.ToLower(strings.TrimSpace(name))
			if name == "" {
				continue
			}
			expr, ok := protocolFilters[name]
			if !ok {
				return "", fmt.Errorf("unknown protocol %q; use --list-protocols or an explicit --bpf expression", name)
			}
			if !seen[name] {
				shortcuts = append(shortcuts, "("+expr+")")
				seen[name] = true
			}
		}
	}
	bpf = strings.TrimSpace(bpf)
	if len(shortcuts) == 0 {
		return bpf, nil
	}
	expr := "(" + strings.Join(shortcuts, " or ") + ")"
	if bpf != "" {
		expr += " and (" + bpf + ")"
	}
	return expr, nil
}
