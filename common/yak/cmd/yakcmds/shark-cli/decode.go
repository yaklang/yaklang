package sharkcli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

type packetSummary struct {
	TransportView        *packetTransportView    `json:"transport_view,omitempty"`
	PhysicalSource       string                  `json:"physical_source,omitempty"`
	PhysicalDestination  string                  `json:"physical_destination,omitempty"`
	TCPFlowID            uint64                  `json:"tcp_flow_id,omitempty"`
	TCPFlowDomain        *pcaputil.CaptureDomain `json:"tcp_flow_domain,omitempty"`
	TCPFlowState         string                  `json:"tcp_flow_state,omitempty"`
	MessageSource        string                  `json:"message_source,omitempty"`
	MessageDestination   string                  `json:"message_destination,omitempty"`
	MessageTransport     string                  `json:"message_transport,omitempty"`
	MessageDomain        *pcaputil.CaptureDomain `json:"message_domain,omitempty"`
	CaptureDomain        pcaputil.CaptureDomain  `json:"capture_domain"`
	PDUs                 []map[string]any        `json:"pdus,omitempty"`
	PublishedPDUs        []map[string]any        `json:"published_pdus,omitempty"`
	PublishedPDUScope    string                  `json:"published_pdu_scope,omitempty"`
	PDUObservationScope  string                  `json:"pdu_scope,omitempty"`
	ProtocolStack        []string                `json:"protocol_stack,omitempty"`
	ConnectionProtocol   string                  `json:"connection_protocol,omitempty"`
	ConnectionStack      []string                `json:"connection_stack,omitempty"`
	ConnectionEvidence   string                  `json:"connection_evidence,omitempty"`
	ConnectionDiagnostic string                  `json:"connection_diagnostic,omitempty"`
	Visibility           string                  `json:"visibility,omitempty"`
	ProtocolStatus       string                  `json:"protocol_status,omitempty"`
	packetProtocol       string
	packetInfo           string
	connectionAnnotated  bool
	payloadBytes         int
	opaqueTransport      bool
	closedTCPPayload     bool
	tcpUnconfirmed       bool
	tcpHeaderFailure     *tcpHeaderFailure
	ProtocolScope        string    `json:"protocol_scope,omitempty"`
	Transport            string    `json:"transport,omitempty"`
	SourcePort           uint16    `json:"source_port,omitempty"`
	DestinationPort      uint16    `json:"destination_port,omitempty"`
	Layers               []string  `json:"layers,omitempty"`
	StreamID             uint64    `json:"stream_id,omitempty"`
	ConversationID       uint64    `json:"conversation_id,omitempty"`
	ProtocolEvidence     string    `json:"protocol_evidence,omitempty"`
	Number               uint64    `json:"number"`
	Time                 time.Time `json:"time"`
	Source               string    `json:"source"`
	Destination          string    `json:"destination"`
	Protocol             string    `json:"protocol"`
	Length               int       `json:"length"`
	CapturedLength       int       `json:"captured_length"`
	Info                 string    `json:"info"`
}

// The hot path never invokes the Yak VM. Deep dissectors run only for selected packets.
func summarize(raw *capturedPacket) packetSummary {
	p := gopacket.NewPacket(raw.data, pcaputil.CaptureLinkDecoder(raw.link), gopacket.DecodeOptions{NoCopy: true})
	row := packetSummary{Number: raw.number, Time: raw.ci.Timestamp, Length: raw.ci.Length, CapturedLength: len(raw.data), Protocol: raw.link.String()}
	row.CaptureDomain = raw.reference.Domain
	row.TCPFlowID = raw.tcpFlowID
	row.TCPFlowState = raw.tcpFlowState
	row.tcpUnconfirmed = raw.tcpUnconfirmed
	if raw.tcpFlowID != 0 {
		d := raw.tcpFlowDomain
		row.TCPFlowDomain = &d
	}
	row.MessageSource, row.MessageDestination, row.MessageTransport = raw.messageSource, raw.messageDestination, raw.messageTransport
	if raw.messageTransport != "" {
		d := raw.messageDomain
		row.MessageDomain = &d
	}
	if boundary := inspectionVXLANBoundary(p); boundary != nil && needsVXLANInspectionFallback(raw, raw.events) && !raw.presentation.networkRefused {
		row = boundary.apply(row)
		if raw.ci.Length > len(raw.data) {
			row.Info += fmt.Sprintf(" [truncated capture: %d/%d bytes]", len(raw.data), raw.ci.Length)
			row.ProtocolStatus = "incomplete"
		}
		return row
	}
	if boundary := inspectTCPHeaderFailure(raw, p); boundary != nil {
		return boundary.apply(row)
	}
	network, selectedTransport := pcaputil.CaptureTransport(p)
	if l := p.LinkLayer(); l != nil {
		row.Source, row.Destination = l.LinkFlow().Src().String(), l.LinkFlow().Dst().String()
	}
	if l := network; l != nil {
		row.Source, row.Destination = l.NetworkFlow().Src().String(), l.NetworkFlow().Dst().String()
	}
	physicalSource, physicalDestination := row.Source, row.Destination
	transportBoundary := false
	for _, l := range p.Layers() {
		if transportBoundary {
			continue
		}
		if l.LayerType() != gopacket.LayerTypePayload && l.LayerType() != gopacket.LayerTypeDecodeFailure {
			row.Protocol = l.LayerType().String()
			row.Layers = append(row.Layers, row.Protocol)
		}
		transportBoundary = l == selectedTransport
	}
	var srcPort, dstPort uint16
	var payload []byte
	transport := ""
	if tcp, ok := selectedTransport.(*layers.TCP); ok {
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
	} else if udp, ok := selectedTransport.(*layers.UDP); ok {
		transport = "udp"
		srcPort, dstPort = uint16(udp.SrcPort), uint16(udp.DstPort)
		payload = udp.Payload
		row.Info = fmt.Sprintf("%d → %d Payload=%d", srcPort, dstPort, len(payload))
	}
	headerObservation := false
	if view := raw.transportView; view != nil && (transport != view.Protocol || row.Source != view.Source || row.Destination != view.Destination || srcPort != view.SourcePort || dstPort != view.DestinationPort || len(payload) != view.PayloadLength) {
		owned := *view
		owned.Layers = append([]string(nil), view.Layers...)
		owned.Header = bytes.Clone(view.Header)
		owned.HeaderSHA256 = fmt.Sprintf("%x", sha256.Sum256(owned.Header))
		owned.PacketRefs = append([]pcaputil.PacketReference(nil), view.PacketRefs...)
		row.TransportView = &owned
		row.Source, row.Destination = view.Source, view.Destination
		srcPort, dstPort, transport = view.SourcePort, view.DestinationPort, view.Protocol
		row.Protocol, row.Info = strings.ToUpper(view.Protocol), view.Info
		if !view.LayersComplete {
			row.Info += " · header layer projection incomplete"
		}
		row.ProtocolStack = append([]string(nil), view.Layers...)
		// The owned view can select an outer transport while the legacy
		// decoder speculates below it. That speculative inner address is not
		// a physical endpoint. Use only the complete matching captured prefix.
		if headers := capturedNetworkPrefix(p, raw.data, view.Layers); len(headers) > 0 {
			physicalSource, physicalDestination = headers[0].source, headers[0].destination
		}
		headerObservation = true
	}
	if raw.messageTransport != "" && transport == "" {
		// A validated reassembled message can reveal an inner TCP/UDP header
		// that is absent from this physical fragment. Preserve both endpoints
		// explicitly; never combine outer IPs with inner transport ports.
		src, sp, se := net.SplitHostPort(raw.messageSource)
		dst, dp, de := net.SplitHostPort(raw.messageDestination)
		sn, spe := strconv.ParseUint(sp, 10, 16)
		dn, dpe := strconv.ParseUint(dp, 10, 16)
		if se == nil && de == nil && spe == nil && dpe == nil && net.ParseIP(src) != nil && net.ParseIP(dst) != nil {
			row.Source, row.Destination = src, dst
			srcPort, dstPort, transport = uint16(sn), uint16(dn), raw.messageTransport
		}
	}
	dns, hasDNS := p.Layer(layers.LayerTypeDNS).(*layers.DNS)
	validDNS := false
	if hasDNS && raw.history == nil && p.ErrorLayer() == nil && int(dns.QDCount)+int(dns.ANCount)+int(dns.NSCount)+int(dns.ARCount) > 0 {
		_, err := pcaputil.DecodeDNSMessage(dns.LayerContents(), 4096)
		validDNS = err == nil
	}
	if validDNS {
		row.Protocol = "DNS"
		row.Info = fmt.Sprintf("ID=0x%04x Response=%t", dns.ID, dns.QR)
		for _, q := range dns.Questions {
			row.Info += " " + q.Type.String() + " " + safeText(string(q.Name))
		}
	} else if name := sniffTCPPayload(transport, payload); name != "" && raw.history == nil && len(raw.events) == 0 && len(raw.connectionStack) == 0 && raw.evidence != "validated protocol events" && (raw.application == "" || strings.EqualFold(raw.application, name)) {
		row.Protocol = name
		if name == "HTTP" || name == "SSH" {
			row.Info += " " + safeText(strings.SplitN(string(payload), "\n", 2)[0])
		}
	} else if len(payload) > 0 {
		if name := portProtocol(transport, srcPort, dstPort); name != "" {
			row.Info += " (port hint: " + name + ")"
		}
	}
	row.Info = safeText(row.Info)
	row.Transport, row.SourcePort, row.DestinationPort = transport, srcPort, dstPort
	row.payloadBytes = len(payload)
	if headerObservation {
		row.payloadBytes = raw.transportView.PayloadLength
		if row.payloadBytes > 0 {
			if hint := portProtocol(transport, srcPort, dstPort); hint != "" {
				row.Info += " (port hint: " + hint + ")"
			}
		}
	}
	row.opaqueTransport = raw.opaqueTransport
	row.closedTCPPayload = raw.tcpFlowState != "" && row.payloadBytes > 0
	row.packetProtocol = row.Protocol
	row.ProtocolScope = "packet"
	if headerObservation {
		row.ProtocolScope = "observed transport header"
	}
	if row.Source != physicalSource || row.Destination != physicalDestination {
		row.PhysicalSource, row.PhysicalDestination = physicalSource, physicalDestination
	}
	presentation := raw.presentation
	if presentation.protocol == "" && raw.history == nil {
		presentation = presentEvents(raw.events)
	}
	if presentation.networkRefused {
		row = applyCapturedNetworkEndpoints(row, &presentation, p, raw.data, presentationNetworkHeaders(presentation, raw.protocolEvents()))
	}
	row = presentation.apply(row)
	row.ConnectionStack = append([]string(nil), raw.connectionStack...)
	row.ConversationID = raw.conversationID
	if row.Transport == "udp" && len(row.ConnectionStack) > 0 && row.ConversationID != 0 {
		row.ConnectionProtocol, row.ConnectionEvidence = row.ConnectionStack[0], "protocol session association"
	}
	if p.ErrorLayer() != nil {
		covered := presentation.networkRefused
		actualIP115 := false
		switch ip := network.(type) {
		case *layers.IPv4:
			actualIP115 = ip.Protocol == 115
		case *layers.IPv6:
			actualIP115 = ip.NextHeader == 115
		}
		why := p.ErrorLayer().Error().Error()
		if espIncompleteDecoderCovered(p, presentation, why) {
			covered = true
		}
		if !covered && icmpv6PartialDecoderCovered(p, raw.protocolEvents(), raw.number, raw.reference.Domain) {
			covered = true
		}
		if presentation.nativeIP115Bytes > 0 && actualIP115 && (why == "Unable to decode IPProtocol 115" || why == "Layer type not currently supported") {
			wire := network.LayerPayload()
			covered = len(wire) == presentation.nativeIP115Bytes && sha256.Sum256(wire) == presentation.nativeIP115SHA
		}
		if !covered {
			row.Info += " [incomplete/undecoded]"
		}
	}
	if raw.ci.Length > len(raw.data) {
		row.Info += fmt.Sprintf(" [truncated capture: %d/%d bytes]", len(raw.data), raw.ci.Length)
		row.ProtocolStatus = "incomplete"
	}
	if raw.connectionDiagnostic != "" && len(payload) > 0 && presentation.protocol == "" {
		row.ConnectionDiagnostic = raw.connectionDiagnostic
		row.Info += " [" + raw.connectionDiagnostic + "]"
		if row.ProtocolStatus == "" {
			row.ProtocolStatus = "context-required"
		}
	}
	return applyApplication(row, raw.application, raw.evidence, raw.streamID)
}

func sniffTCPPayload(transport string, payload []byte) string {
	if transport != "tcp" {
		return ""
	}
	return sniffPayload(payload)
}

func sniffPayload(p []byte) string {
	if inspectHTTP(p) != nil {
		return "HTTP"
	}
	// The capture engine owns binary admission and framing. A few magic bytes
	// are insufficient evidence for TLS, SMB, MQTT, or any other binary PDU.
	if end := bytes.Index(p, []byte("\r\n")); end > 8 && end+2 <= 255 {
		line := string(p[:end])
		if strings.HasPrefix(line, "SSH-2.0-") || strings.HasPrefix(line, "SSH-1.99-") {
			software := strings.SplitN(line, "-", 3)[2]
			if software != "" && software[0] != ' ' {
				valid := true
				for _, b := range []byte(line) {
					valid = valid && b >= 32 && b <= 126
				}
				if valid {
					return "SSH"
				}
			}
		}
	}
	return ""
}

var tcpPortProtocols = map[uint16]string{20: "FTP-DATA", 21: "FTP", 22: "SSH", 23: "Telnet", 25: "SMTP", 80: "HTTP", 110: "POP3", 135: "DCERPC", 139: "NBSS", 143: "IMAP", 179: "BGP", 389: "LDAP", 443: "TLS", 445: "SMB", 465: "TLS", 554: "RTSP", 587: "SMTP", 636: "TLS", 993: "TLS", 995: "TLS", 1080: "SOCKS", 1433: "TDS", 1883: "MQTT", 1935: "RTMP", 3306: "MySQL", 3389: "RDP", 5432: "PostgreSQL", 5672: "AMQP", 5900: "VNC", 6379: "Redis", 8009: "AJP", 8080: "HTTP", 8443: "TLS", 8883: "TLS", 9092: "Kafka", 11211: "Memcached", 27017: "MongoDB"}
var udpPortProtocols = map[uint16]string{53: "DNS", 67: "DHCP", 68: "DHCP", 69: "TFTP", 88: "Kerberos", 111: "ONC-RPC", 123: "NTP", 137: "NBNS", 138: "NBT-DG", 161: "SNMP", 162: "SNMP", 443: "QUIC", 500: "IKE", 514: "Syslog", 520: "RIP", 546: "DHCPv6", 547: "DHCPv6", 623: "IPMI", 1194: "OpenVPN", 1701: "L2TP", 1812: "RADIUS", 1813: "RADIUS", 1900: "SSDP", 1985: "HSRP", 2029: "HSRP", 3222: "GLBP", 3784: "BFD", 4784: "BFD", 3478: "STUN", 4500: "IKE", 4789: "VXLAN", 5004: "RTP", 5005: "RTCP", 5060: "SIP", 5353: "MDNS", 5355: "LLMNR", 51820: "WireGuard"}

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
	source     int // 0: captured frame; >0: packetDetail.sources index + 1.
}

type detailSource struct {
	Name string
	Data []byte
}
type packetDetail struct {
	protocol string
	number   uint64
	fields   []field
	hex      []string
	sources  []detailSource
}

// The same admission barrier applies to both the summary and every inspector.
// A generic stateless dissector may expose speculative fragment/child layers
// after an invalid header. Those are not admitted protocol observations.
func protocolDetailEvents(events []*pcaputil.ProtocolEvent) ([]*pcaputil.ProtocolEvent, bool) {
	for _, event := range events {
		if presentEvents([]*pcaputil.ProtocolEvent{event}).networkRefused {
			return []*pcaputil.ProtocolEvent{event}, true
		}
	}
	return events, false
}

// A validated incomplete ESP observation already describes every available
// child byte. Keep its physical IP prefix, but do not retry a legacy grammar
// which requires bytes outside this captured carrier. A matching label alone
// cannot suppress an unrelated decoder failure or an ICMP quotation.
func appendESPIncompleteCapturedPrefix(d *packetDetail, raw *capturedPacket, events []*pcaputil.ProtocolEvent) bool {
	p := gopacket.NewPacket(raw.data, pcaputil.CaptureLinkDecoder(raw.link), gopacket.DecodeOptions{NoCopy: true})
	esp, ok := p.Layer(layers.LayerTypeIPSecESP).(*layers.IPSecESP)
	if !ok {
		return false
	}
	matched := false
	for _, event := range events {
		if validESPIncompleteObservation(event) && bytes.Equal(event.Raw, esp.LayerContents()) {
			matched = true // The strict contract also verifies the complete raw hash.
			break
		}
	}
	if !matched {
		return false
	}
	var names []string
	var parent gopacket.Layer
	offset, lastNetwork := 0, -1
	for _, layer := range p.Layers() {
		if layer == esp {
			break
		}
		if layer.LayerType() == layers.LayerTypeICMPv4 || layer.LayerType() == layers.LayerTypeICMPv6 || layer.LayerType() == layers.LayerTypeTCP {
			return false
		}
		if _, network := layer.(gopacket.NetworkLayer); network {
			lastNetwork = offset
		}
		names = append(names, layer.LayerType().String())
		offset += len(layer.LayerContents())
		parent = layer
	}
	var next layers.IPProtocol
	switch parent := parent.(type) {
	case *layers.IPv4:
		next = parent.Protocol
	case *layers.IPv6:
		next = parent.NextHeader
	case *layers.IPv6HopByHop:
		next = parent.NextHeader
	case *layers.IPv6Destination:
		next = parent.NextHeader
	case *layers.IPv6Routing:
		next = parent.NextHeader
	case *layers.IPSecAH:
		next = parent.NextHeader
	default:
		return false
	}
	if next != layers.IPProtocolESP || !bytes.Equal(parent.LayerPayload(), esp.LayerContents()) {
		return false
	}
	headers := capturedNetworkPrefix(p, raw.data, names)
	if len(headers) == 0 || headers[len(headers)-1].start != lastNetwork {
		return false // A failed/unselected outer carrier cannot confirm this child.
	}
	d.fields = append(d.fields, field{text: "Captured network prefix", branch: true})
	for _, header := range headers {
		d.fields = append(d.fields, field{text: header.label + " header", branch: true, start: header.start, end: header.end, hasRange: true}, field{depth: 1, text: "Source: " + header.source, start: header.sourceStart, end: header.sourceEnd, hasRange: true}, field{depth: 1, text: "Destination: " + header.destination, start: header.destStart, end: header.destEnd, hasRange: true})
	}
	return true
}

func detail(raw *capturedPacket) (d packetDetail) {
	d.number = raw.number
	row := summarize(raw)
	d.protocol = row.Protocol
	d.hex = strings.Split(strings.TrimSuffix(hex.Dump(raw.data), "\n"), "\n")
	defer func() {
		if r := recover(); r != nil {
			d.fields = append(d.fields, field{text: fmt.Sprintf("Dissector failed: %v", r)})
		}
		// Inspect the frame's real byte-backed fields first. Generated event
		// metadata has no packet offsets and must not displace the Frame entry
		// at index zero, which is excluded from network-header offset rebasing.
	}()
	d.fields = append(d.fields, field{text: fmt.Sprintf("Frame %d: %d bytes on wire, %d captured", raw.number, raw.ci.Length, len(raw.data)), hasRange: len(raw.data) > 0, end: len(raw.data)})
	if row.tcpHeaderFailure != nil {
		row.tcpHeaderFailure.appendFields(&d)
		return d // Invalid TCP cannot enter legacy YAML or a partial option formatter.
	}
	events, networkRefused := protocolDetailEvents(raw.protocolEvents())
	if networkRefused {
		appendCapturedNetworkPrefix(&d, raw, events)
	}
	espIncomplete := !networkRefused && appendESPIncompleteCapturedPrefix(&d, raw, events)
	mldObserved := !networkRefused && appendMLDCapturedPrefix(&d, raw, events)
	igmpObserved := !networkRefused && appendIGMPCapturedPrefix(&d, raw, events)
	pimObserved := !networkRefused && appendPIMCapturedPrefix(&d, raw, events)
	icmpv6Partial := !networkRefused && appendICMPv6PartialCapturedPrefix(&d, raw, events)
	icmpv4Partial := !networkRefused && appendICMPv4PartialCapturedPrefix(&d, raw, events)
	if !networkRefused && needsVXLANInspectionFallback(raw, events) {
		p := gopacket.NewPacket(raw.data, pcaputil.CaptureLinkDecoder(raw.link), gopacket.DecodeOptions{NoCopy: true})
		if boundary := inspectionVXLANBoundary(p); boundary != nil {
			boundary.appendFields(&d, raw)
			return d
		}
	}
	if !networkRefused {
		appendCapturedVXLANHeaders(&d, raw)
	}
	if len(events) > 0 {
		fields, sources := protocolEventDetail(events, !raw.workerFieldProjection)
		d.fields = append(d.fields, fields...)
		d.sources = sources
		if d.protocol == "" {
			d.protocol = presentEvents(events).protocol
		}
	}
	if networkRefused || espIncomplete || mldObserved || igmpObserved || pimObserved || icmpv6Partial || icmpv4Partial {
		return d
	}
	appendTransportObservation(&d, row)
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
		p := gopacket.NewPacket(data, pcaputil.CaptureLinkDecoder(raw.link), gopacket.DecodeOptions{NoCopy: true})
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
				wireStart := len(d.fields)
				flattenFields(value, 0, &d.fields)
				if summarize(raw).ProtocolScope == "connection-associated bytes" {
					for i := wireStart; i < len(d.fields); i++ {
						if d.fields[i].branch && applicationNodes[d.fields[i].text] != "" {
							// Bytes inside an established stream are not a fresh PDU.
							// A coincidental SSH/HTTP signature in a body or ciphertext
							// must not create a contradictory raw-packet subtree.
							d.fields = append(d.fields[:i], field{text: "Stream continuation: no complete decoded application message in this packet; inspect the reassembled PDU or conversation"})
							break
						}
					}
				}
				if d.protocol == "TLS?" || !hasObservedTLS(raw) {
					for i := range d.fields {
						if d.fields[i].branch && (d.fields[i].text == "TLS" || d.fields[i].text == "Transport Layer Security") {
							label := "TLS?"
							if d.protocol == "TLS" {
								label = "TLS"
							}
							d.fields[i].text = label + " · record envelope only; handshake not captured; negotiated version unknown"
						}
					}
				}
				for i := wireStart; i < len(d.fields); i++ {
					f := &d.fields[i]
					if f.source != 0 {
						continue
					}
					f.start += baseOffset
					f.end += baseOffset
					f.hasRange = f.hasRange && f.start >= 0 && f.end > f.start && f.end <= len(raw.data)
				}
				if d.protocol == "" {
					d.protocol = applicationProtocol(value)
				}
				return d
			}
			err = resultErr
		}
		d.fields = append(d.fields, field{text: fmt.Sprintf("YAML dissector incomplete: %v; showing available packet layers", err)})
	}
	p := gopacket.NewPacket(raw.data, pcaputil.CaptureLinkDecoder(raw.link), gopacket.DecodeOptions{NoCopy: true})
	for _, layer := range inspectionPacketLayers(p, row.TransportView) {
		d.fields = append(d.fields, field{text: safeText(gopacket.LayerString(layer))})
	}
	return d
}

// An ingress-owned transport view selects the header prefix for this packet.
// Fast/fallback details must not expose a speculative decoder child beyond it.
// The original decoder inventory in packetSummary.Layers remains unchanged.
func inspectionPacketLayers(p gopacket.Packet, view *packetTransportView) []gopacket.Layer {
	layers := p.Layers()
	if view == nil {
		return layers
	}
	matched := 0
	for i, name := range view.Layers {
		if i >= len(layers) || layers[i].LayerType().String() != name {
			break
		}
		matched++
	}
	return layers[:matched]
}

func hasObservedTLS(raw *capturedPacket) bool {
	for _, name := range raw.connectionStack {
		if name == "TLS" || strings.HasPrefix(name, "TLSv") {
			return true
		}
	}
	for _, e := range raw.protocolEvents() {
		if e.Protocol == "tls" && credibleTLSIdentity(e) {
			return true
		}
	}
	return false
}

func flattenFields(n *base.NodeValue, depth int, out *[]field) {
	flatFields(n, "", "", out, depth)
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
func flatFields(n *base.NodeValue, section, path string, out *[]field, initialDepth ...int) {
	limited := false
	var walk func(*base.NodeValue, string, string, string, int, int)
	walk = func(n *base.NodeValue, section, path, label string, depth, traversed int) {
		if n == nil {
			return
		}
		if len(*out) >= 8191 || len(path) > 1024 || traversed > 32 {
			limited = true
			return
		}
		add := func(text string, branch bool) {
			f := nodeField(n, safeText(text), branch)
			f.depth = max(0, depth)
			*out = append(*out, f)
		}
		if n.IsValue() {
			value := formatFieldValue(n.Name, n.Value)
			if address, ok := legacyAddressFieldValue(section, n.Name, n.Value); ok {
				value = address
			}
			// Keep the existing full field text for filtering and byte selection.
			add(path+n.Name+": "+value, false)
			return
		}
		if n.Name == "root" || n.Name == "Package" || n.Name == "Payload" {
			// Parser-only wrappers are transparent. Real protocol layers remain
			// sibling headings; their own records/options retain nesting below.
		} else if applicationNodes[n.Name] != "" || n.Name == "Ethernet" || legacyIPWordLength(n.Name) > 0 || n.Name == "TCP" || n.Name == "UDP" || n.Name == "ARP" || section == "" {
			section, path, depth = n.Name, "", 0
			add(n.Name, true)
			depth++
		} else {
			add(path+label, true)
			path += n.Name + "."
			depth++
		}
		for i, child := range n.Children() {
			label := child.Name
			if n.IsList() {
				label = fmt.Sprintf("%s[%d]", label, i)
			}
			walk(child, section, path, label, depth, traversed+1)
		}
	}
	depth := 0
	if len(initialDepth) > 0 {
		depth = initialDepth[0]
	}
	if n != nil {
		walk(n, section, path, n.Name, depth, 0)
	}
	if limited && len(*out) < 8192 {
		*out = append(*out, field{text: "YAML field view limited: additional nodes or nesting omitted"})
	}
}

func legacyIPWordLength(section string) int {
	switch section {
	case "IP", "IPv4", "Internet Protocol":
		return 4
	case "IPv6", "Internet Protocol Version 6":
		return 16
	}
	return 0
}

func legacyAddressFieldValue(section, name string, value any) (string, bool) {
	if name != "Source" && name != "Destination" {
		return "", false
	}
	length := legacyIPWordLength(section)
	if section == "Ethernet" {
		length = 6
	}
	if length == 0 {
		return "", false
	}
	var wire []byte
	switch v := value.(type) {
	case []byte:
		wire = v
	case string:
		if len(v) != length {
			return "", false
		}
		wire = []byte(v)
	default:
		return "", false
	}
	if len(wire) != length {
		return "", false
	}
	if section == "Ethernet" {
		return net.HardwareAddr(wire).String(), true
	}
	// Addr retains the IPv6 identity of mapped addresses; net.IP.String would
	// render the same sixteen captured bytes as a bare IPv4 address.
	address, ok := netip.AddrFromSlice(wire)
	return address.String(), ok
}

// quickDetail is safe to call at the display refresh rate. Even during a live
// flood, the inspector always has concrete fields and bytes to show.
func quickDetail(raw *capturedPacket) packetDetail {
	d := packetDetail{number: raw.number, hex: strings.Split(strings.TrimSuffix(hex.Dump(raw.data), "\n"), "\n")}
	d.fields = append(d.fields, field{text: fmt.Sprintf("Frame #%d · %d bytes · %d captured", raw.number, raw.ci.Length, len(raw.data)), branch: true, hasRange: len(raw.data) > 0, end: len(raw.data)})
	row := summarize(raw)
	d.protocol = row.Protocol
	if row.tcpHeaderFailure != nil {
		row.tcpHeaderFailure.appendFields(&d)
		return d
	}
	events, networkRefused := protocolDetailEvents(raw.protocolEvents())
	if networkRefused {
		appendCapturedNetworkPrefix(&d, raw, events)
	}
	espIncomplete := !networkRefused && appendESPIncompleteCapturedPrefix(&d, raw, events)
	mldObserved := !networkRefused && appendMLDCapturedPrefix(&d, raw, events)
	igmpObserved := !networkRefused && appendIGMPCapturedPrefix(&d, raw, events)
	pimObserved := !networkRefused && appendPIMCapturedPrefix(&d, raw, events)
	if !networkRefused && needsVXLANInspectionFallback(raw, events) {
		p := gopacket.NewPacket(raw.data, pcaputil.CaptureLinkDecoder(raw.link), gopacket.DecodeOptions{NoCopy: true})
		if boundary := inspectionVXLANBoundary(p); boundary != nil {
			boundary.appendFields(&d, raw)
			return d
		}
	}
	if !networkRefused {
		appendCapturedVXLANHeaders(&d, raw)
	}
	if len(events) > 0 {
		fields, sources := protocolEventDetail(events, !raw.workerFieldProjection)
		d.fields = append(d.fields, fields...)
		d.sources = sources
	}
	if networkRefused || espIncomplete || mldObserved || igmpObserved || pimObserved {
		return d
	}
	appendTransportObservation(&d, row)
	p := gopacket.NewPacket(raw.data, pcaputil.CaptureLinkDecoder(raw.link), gopacket.DecodeOptions{NoCopy: true})
	add := func(name string, value any) {
		d.fields = append(d.fields, field{depth: 1, text: name + ": " + formatFieldValue(name, value)})
	}
	offset := 0
	for _, l := range inspectionPacketLayers(p, row.TransportView) {
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
