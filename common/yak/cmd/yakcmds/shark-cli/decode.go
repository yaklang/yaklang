package sharkcli

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"net"
	"strings"
	"time"
	"unicode"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

type packetSummary struct {
	packetProtocol   string
	ProtocolScope    string    `json:"protocol_scope,omitempty"`
	Transport        string    `json:"transport,omitempty"`
	SourcePort       uint16    `json:"source_port,omitempty"`
	DestinationPort  uint16    `json:"destination_port,omitempty"`
	Layers           []string  `json:"layers,omitempty"`
	StreamID         uint64    `json:"stream_id,omitempty"`
	ProtocolEvidence string    `json:"protocol_evidence,omitempty"`
	Number           uint64    `json:"number"`
	Time             time.Time `json:"time"`
	Source           string    `json:"source"`
	Destination      string    `json:"destination"`
	Protocol         string    `json:"protocol"`
	Length           int       `json:"length"`
	CapturedLength   int       `json:"captured_length"`
	Info             string    `json:"info"`
}

// The hot path never invokes the Yak VM. Deep dissectors run only for selected packets.
func summarize(raw *capturedPacket) packetSummary {
	p := gopacket.NewPacket(raw.data, raw.link, gopacket.DecodeOptions{NoCopy: true})
	row := packetSummary{Number: raw.number, Time: raw.ci.Timestamp, Length: raw.ci.Length, CapturedLength: len(raw.data), Protocol: raw.link.String()}
	if l := p.LinkLayer(); l != nil {
		row.Source, row.Destination = l.LinkFlow().Src().String(), l.LinkFlow().Dst().String()
	}
	if l := p.NetworkLayer(); l != nil {
		row.Source, row.Destination = l.NetworkFlow().Src().String(), l.NetworkFlow().Dst().String()
	}
	for _, l := range p.Layers() {
		if l.LayerType() != gopacket.LayerTypePayload && l.LayerType() != gopacket.LayerTypeDecodeFailure {
			row.Protocol = l.LayerType().String()
			row.Layers = append(row.Layers, row.Protocol)
		}
	}
	var srcPort, dstPort uint16
	var payload []byte
	transport := ""
	if tcp, ok := p.Layer(layers.LayerTypeTCP).(*layers.TCP); ok {
		transport = "tcp"
		srcPort, dstPort = uint16(tcp.SrcPort), uint16(tcp.DstPort)
		payload = tcp.Payload
		var flags []string
		for _, f := range []struct {
			on   bool
			name string
		}{{tcp.SYN, "SYN"}, {tcp.ACK, "ACK"}, {tcp.FIN, "FIN"}, {tcp.RST, "RST"}, {tcp.PSH, "PSH"}, {tcp.URG, "URG"}} {
			if f.on {
				flags = append(flags, f.name)
			}
		}
		row.Info = fmt.Sprintf("%d → %d [%s] Seq=%d Ack=%d Payload=%d", srcPort, dstPort, strings.Join(flags, ","), tcp.Seq, tcp.Ack, len(payload))
	} else if udp, ok := p.Layer(layers.LayerTypeUDP).(*layers.UDP); ok {
		transport = "udp"
		srcPort, dstPort = uint16(udp.SrcPort), uint16(udp.DstPort)
		payload = udp.Payload
		row.Info = fmt.Sprintf("%d → %d Payload=%d", srcPort, dstPort, len(payload))
	}
	if dns, ok := p.Layer(layers.LayerTypeDNS).(*layers.DNS); ok {
		row.Protocol = "DNS"
		row.Info = fmt.Sprintf("ID=0x%04x Response=%t", dns.ID, dns.QR)
		for _, q := range dns.Questions {
			row.Info += " " + q.Type.String() + " " + safeText(string(q.Name))
		}
	} else if name := sniffPayload(payload); name != "" {
		row.Protocol = name
		if name == "HTTP" || name == "SSH" {
			row.Info += " " + safeText(strings.SplitN(string(payload), "\n", 2)[0])
		}
	} else if len(payload) > 0 {
		if name := portProtocol(transport, srcPort, dstPort); name != "" {
			row.Info += " (port hint: " + name + ")"
		}
	}
	if p.ErrorLayer() != nil {
		row.Info += " [incomplete/undecoded]"
	}
	if raw.ci.Length > len(raw.data) {
		row.Info += " [truncated capture]"
	}
	row.Info = safeText(row.Info)
	row.Transport, row.SourcePort, row.DestinationPort = transport, srcPort, dstPort
	row.packetProtocol = row.Protocol
	row.ProtocolScope = "packet"
	return applyApplication(row, raw.application, raw.evidence, raw.streamID)
}

func sniffPayload(p []byte) string {
	if inspectHTTP(p) != nil {
		return "HTTP"
	}
	if bytes.HasPrefix(p, []byte("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n")) {
		return "HTTP2"
	}
	if bytes.HasPrefix(p, []byte("SSH-")) {
		return "SSH"
	}
	if len(p) >= 5 && p[0] >= 20 && p[0] <= 24 && p[1] == 3 && p[2] <= 4 {
		return "TLS"
	}
	if len(p) >= 4 && bytes.Equal(p[:4], []byte{0xfe, 'S', 'M', 'B'}) {
		return "SMB2"
	}
	if len(p) >= 4 && bytes.Equal(p[:4], []byte{0xff, 'S', 'M', 'B'}) {
		return "SMB"
	}
	return ""
}

var tcpPortProtocols = map[uint16]string{20: "FTP-DATA", 21: "FTP", 22: "SSH", 23: "Telnet", 25: "SMTP", 80: "HTTP", 110: "POP3", 135: "DCERPC", 139: "NBSS", 143: "IMAP", 179: "BGP", 389: "LDAP", 443: "TLS", 445: "SMB", 465: "TLS", 554: "RTSP", 587: "SMTP", 636: "TLS", 993: "TLS", 995: "TLS", 1080: "SOCKS", 1433: "TDS", 1883: "MQTT", 1935: "RTMP", 3306: "MySQL", 3389: "RDP", 5432: "PostgreSQL", 5672: "AMQP", 5900: "VNC", 6379: "Redis", 8009: "AJP", 8080: "HTTP", 8443: "TLS", 8883: "TLS", 9092: "Kafka", 11211: "Memcached", 27017: "MongoDB"}
var udpPortProtocols = map[uint16]string{53: "DNS", 67: "DHCP", 68: "DHCP", 69: "TFTP", 88: "Kerberos", 111: "ONC-RPC", 123: "NTP", 137: "NBNS", 138: "NBT-DG", 161: "SNMP", 162: "SNMP", 443: "QUIC", 500: "IKE", 514: "Syslog", 520: "RIP", 546: "DHCPv6", 547: "DHCPv6", 623: "IPMI", 1194: "OpenVPN", 1701: "L2TP", 1812: "RADIUS", 1813: "RADIUS", 1900: "SSDP", 3478: "STUN", 4500: "IKE", 4789: "VXLAN", 5004: "RTP", 5005: "RTCP", 5060: "SIP", 5353: "MDNS", 5355: "LLMNR", 51820: "WireGuard"}

func portProtocol(transport string, src, dst uint16) string {
	ports := tcpPortProtocols
	if transport == "udp" {
		ports = udpPortProtocols
	}
	if name := ports[dst]; name != "" {
		return name
	}
	return ports[src]
}

func safeText(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
			return ' '
		}
		return r
	}, s)
}

type field struct {
	depth      int
	text       string
	branch     bool
	start, end int // Byte offsets in the captured packet, end exclusive.
	hasRange   bool
}
type packetDetail struct {
	protocol string
	number   uint64
	fields   []field
	hex      []string
}

func detail(raw *capturedPacket) (d packetDetail) {
	d.number = raw.number
	d.hex = strings.Split(strings.TrimSuffix(hex.Dump(raw.data), "\n"), "\n")
	defer func() {
		if r := recover(); r != nil {
			d.fields = append(d.fields, field{text: fmt.Sprintf("Dissector failed: %v", r)})
		}
	}()
	d.fields = append(d.fields, field{text: fmt.Sprintf("Frame %d: %d bytes on wire, %d captured", raw.number, raw.ci.Length, len(raw.data)), hasRange: len(raw.data) > 0, end: len(raw.data)})
	rule := ""
	data := raw.data
	baseOffset := 0
	switch raw.link {
	case layers.LinkTypeEthernet:
		rule = "ethernet"
	case layers.LinkTypeLinuxSLL:
		rule = "linux_sll"
	case layers.LinkTypeIEEE802_11:
		rule = "ieee_802_11"
	case layers.LinkTypeIPv4:
		rule = "internet_protocol"
	case layers.LinkTypeIPv6:
		rule = "internet_protocol_version_6"
	default:
		// Handles loopback, raw IP, radiotap, etc. using the decoded network layer.
		p := gopacket.NewPacket(data, raw.link, gopacket.DecodeOptions{NoCopy: true})
		if l := p.NetworkLayer(); l != nil {
			for _, preceding := range p.Layers() {
				if preceding == l {
					break
				}
				baseOffset += len(preceding.LayerContents())
			}
			// IPv6's decoder may consume extension headers into separate layers;
			// joining its Contents and Payload would silently omit those bytes.
			data = raw.data[baseOffset:]
			switch l.LayerType() {
			case layers.LayerTypeIPv4:
				rule = "internet_protocol"
			case layers.LayerTypeIPv6:
				rule = "internet_protocol_version_6"
			}
		}
	}
	if rule != "" {
		node, err := parser.ParseBinary(bytes.NewReader(data), rule)
		if err == nil {
			value, resultErr := node.Result()
			if resultErr == nil && value != nil {
				flattenFields(value, 0, &d.fields)
				for i := 1; i < len(d.fields); i++ {
					f := &d.fields[i]
					f.start += baseOffset
					f.end += baseOffset
					f.hasRange = f.hasRange && f.start >= 0 && f.end > f.start && f.end <= len(raw.data)
				}
				d.protocol = applicationProtocol(value)
				return d
			}
			err = resultErr
		}
		d.fields = append(d.fields, field{text: fmt.Sprintf("YAML dissector incomplete: %v; showing available packet layers", err)})
	}
	p := gopacket.NewPacket(raw.data, raw.link, gopacket.DecodeOptions{NoCopy: true})
	for _, layer := range p.Layers() {
		d.fields = append(d.fields, field{text: safeText(gopacket.LayerString(layer))})
	}
	return d
}

func flattenFields(n *base.NodeValue, depth int, out *[]field) {
	flatFields(n, "", "", out)
}

func applicationProtocol(n *base.NodeValue) string {
	if !n.IsValue() && applicationNodes[n.Name] != "" {
		return applicationNodes[n.Name]
	}
	for _, child := range n.Children() {
		if name := applicationProtocol(child); name != "" {
			return name
		}
	}
	return ""
}
func flatFields(n *base.NodeValue, section, path string, out *[]field) {
	if len(*out) >= 8192 || len(path) > 1024 {
		return
	}
	if n.IsValue() {
		value := formatFieldValue(n.Name, n.Value)
		if b, ok := n.Value.([]byte); ok {
			if (section == "IP" || section == "IPv4" || section == "IPv6") && (n.Name == "Source" || n.Name == "Destination") && (len(b) == 4 || len(b) == 16) {
				value = net.IP(b).String()
			} else if section == "Ethernet" && (n.Name == "Source" || n.Name == "Destination") && len(b) == 6 {
				value = net.HardwareAddr(b).String()
			}
		}
		*out = append(*out, nodeField(n, safeText(path+n.Name+": "+value), false))
		return
	}
	if n.Name == "root" || n.Name == "Package" || n.Name == "Payload" {
		// Structural containers do not become another level of indentation.
	} else if applicationNodes[n.Name] != "" || n.Name == "Ethernet" || n.Name == "IP" || n.Name == "IPv6" || n.Name == "TCP" || n.Name == "UDP" || n.Name == "ARP" || section == "" {
		section, path = n.Name, ""
		*out = append(*out, nodeField(n, safeText(n.Name), true))
	} else {
		path += n.Name + "."
	}
	for _, child := range n.Children() {
		flatFields(child, section, path, out)
	}
}

// quickDetail is safe to call at the display refresh rate. Even during a live
// flood, the inspector always has concrete fields and bytes to show.
func quickDetail(raw *capturedPacket) packetDetail {
	d := packetDetail{number: raw.number, hex: strings.Split(strings.TrimSuffix(hex.Dump(raw.data), "\n"), "\n")}
	d.fields = append(d.fields, field{text: fmt.Sprintf("Frame #%d · %d bytes · %d captured", raw.number, raw.ci.Length, len(raw.data)), branch: true, hasRange: len(raw.data) > 0, end: len(raw.data)})
	p := gopacket.NewPacket(raw.data, raw.link, gopacket.DecodeOptions{NoCopy: true})
	add := func(name string, value any) {
		d.fields = append(d.fields, field{depth: 1, text: name + ": " + formatFieldValue(name, value)})
	}
	offset := 0
	for _, l := range p.Layers() {
		start := offset
		offset += len(l.LayerContents())
		if l.LayerType() == gopacket.LayerTypePayload {
			d.fields = append(d.fields, field{text: "Payload: " + formatFieldValue("Payload", l.LayerContents()), start: start, end: offset, hasRange: offset > start && offset <= len(raw.data)})
			continue
		}
		d.fields = append(d.fields, field{text: l.LayerType().String(), branch: true, start: start, end: offset, hasRange: offset > start && offset <= len(raw.data)})
		switch v := l.(type) {
		case *layers.Ethernet:
			add("Source", v.SrcMAC)
			add("Destination", v.DstMAC)
			add("EtherType", v.EthernetType)
		case *layers.IPv4:
			add("Source", v.SrcIP)
			add("Destination", v.DstIP)
			add("Protocol", v.Protocol)
			add("TTL", v.TTL)
			add("Length", v.Length)
		case *layers.IPv6:
			add("Source", v.SrcIP)
			add("Destination", v.DstIP)
			add("Next header", v.NextHeader)
			add("Hop limit", v.HopLimit)
		case *layers.TCP:
			add("Source port", uint16(v.SrcPort))
			add("Destination port", uint16(v.DstPort))
			add("Sequence", v.Seq)
			add("Acknowledgment", v.Ack)
			add("Window", v.Window)
			add("Flags", fmt.Sprintf("SYN=%t ACK=%t FIN=%t RST=%t PSH=%t", v.SYN, v.ACK, v.FIN, v.RST, v.PSH))
		case *layers.UDP:
			add("Source port", uint16(v.SrcPort))
			add("Destination port", uint16(v.DstPort))
			add("Length", v.Length)
			add("Checksum", fmt.Sprintf("0x%04x", v.Checksum))
		case *layers.DNS:
			add("Transaction ID", fmt.Sprintf("0x%04x", v.ID))
			add("Response", v.QR)
			for _, q := range v.Questions {
				add("Question", string(q.Name)+" "+q.Type.String())
			}
		default:
			add("Decoded", gopacket.LayerString(l))
		}
	}
	if p.ApplicationLayer() != nil && len(p.ApplicationLayer().Payload()) > 0 {
		add("Payload bytes", len(p.ApplicationLayer().Payload()))
	}
	return d
}
