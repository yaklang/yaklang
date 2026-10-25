package sharkcli

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

// Presentation is a bounded projection of parser events, never another parser.
// It is built before publishing a packet, so UI rows need not decode messages
// or copy history on every redraw. Connection attribution remains separate.
type packetPresentation struct {
	evidence                              string
	protocol, summary, visibility, status string
	diagnostic                            string
	stack                                 []string
	priority                              int
	count                                 int
	rejectedTLS                           bool
}

var protocolDisplayNames = map[string]string{
	"ldp":       "LDP",
	"wireguard": "WireGuard", "openvpn": "OpenVPN", "ike": "IKE", "l2tp": "L2TP", "rip": "RIP", "nbt-dg": "NBT-DG", "onc-rpc": "ONC-RPC", "ipmi": "IPMI", "telnet": "Telnet",
	"tlsv1.0": "TLSv1.0", "tlsv1.1": "TLSv1.1", "tlsv1.2": "TLSv1.2", "tlsv1.3": "TLSv1.3",
	"http": "HTTP", "http2": "HTTP/2", "http3": "HTTP/3", "grpc": "gRPC",
	"tls": "TLS", "dtls": "DTLS", "dns": "DNS", "dot": "DoT", "doh": "DoH", "doq": "DoQ",
	"quic": "QUIC", "websocket": "WebSocket", "postgresql": "PostgreSQL", "mysql": "MySQL",
	"ssdp": "SSDP", "rdpudp": "RDPUDP",
	"redis": "Redis", "mongodb": "MongoDB", "kafka": "Kafka", "mqtt": "MQTT",
	"smb2": "SMB2", "smb3": "SMB3", "dcerpc": "DCE/RPC", "kerberos": "Kerberos",
	"ntlmssp": "NTLMSSP", "spnego": "SPNEGO", "mdns": "mDNS", "llmnr": "LLMNR",
	"snmp": "SNMP", "dhcp": "DHCP", "dhcpv6": "DHCPv6", "syslog": "Syslog",
}

func protocolName(protocol string) string {
	if name := protocolDisplayNames[strings.ToLower(protocol)]; name != "" {
		return name
	}
	return strings.ToUpper(safeText(protocol))
}

func eventValues(e *pcaputil.ProtocolEvent) map[string]any {
	if e.Session != nil {
		return e.Session
	}
	return e.Fields
}

func number(v any) uint64 {
	switch n := v.(type) {
	case uint8:
		return uint64(n)
	case uint16:
		return uint64(n)
	case uint32:
		return uint64(n)
	case uint64:
		return n
	case int:
		if n >= 0 {
			return uint64(n)
		}
	case int64:
		if n >= 0 {
			return uint64(n)
		}
	case float64:
		if n >= 0 && n < 1<<53 {
			return uint64(n)
		}
	case json.Number:
		i, _ := strconv.ParseUint(string(n), 10, 64)
		return i
	}
	return 0
}

func textValue(v any) string {
	if s, ok := v.(string); ok {
		return safeText(s)
	}
	return ""
}

func mapList(v any) []map[string]any {
	switch rows := v.(type) {
	case []map[string]any:
		return rows
	case []any:
		var out []map[string]any
		for _, row := range rows {
			if m, ok := row.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	}
	return nil
}

func tlsVersion(v any) string {
	switch number(v) {
	case 0x0301:
		return "TLSv1.0"
	case 0x0302:
		return "TLSv1.1"
	case 0x0303:
		return "TLSv1.2"
	case 0x0304:
		return "TLSv1.3"
	}
	return "TLS"
}

func eventProtocolName(e *pcaputil.ProtocolEvent) string {
	if e.Protocol == "pim" && number(eventValues(e)["Version"]) == 2 {
		return "PIMv2"
	}
	if e.Protocol == "hsrp" {
		switch number(eventValues(e)["Protocol Version"]) {
		case 1:
			return "HSRPv1"
		case 2:
			return "HSRPv2"
		}
	}
	if e.Protocol == "bfd" && number(eventValues(e)["Version"]) == 1 {
		return "BFDv1"
	}
	if e.Protocol == "rdpudp" && number(eventValues(e)["Protocol Version"]) == 0x101 {
		return "RDPUDP2"
	}
	if e.Protocol == "ike" {
		if v := number(eventValues(e)["Version"]); v == 0x10 || v == 0x20 {
			return fmt.Sprintf("IKEv%d", v>>4)
		}
	}
	if names := authenticationLayers(e); len(names) > 0 {
		return names[len(names)-1]
	}
	if e.Protocol == "smb2" && e.Session["Encrypted"] == true {
		return "SMB3"
	}
	if e.Protocol == "tls" {
		return tlsVersion(eventValues(e)["Negotiated Version"])
	}
	if e.Protocol == "dot" && e.SourceBytes.Kind != "decrypted" {
		return "DNS"
	}
	if e.Protocol == "http" {
		if version := textValue(e.Fields["Protocol"]); version == "HTTP/1.1" || version == "HTTP/1.0" {
			return version
		}
		message, _ := e.Fields["Message"].(map[string]any)
		for _, kind := range []string{"HTTP Request", "HTTP Response"} {
			part, _ := message[kind].(map[string]any)
			line, _ := part["FirstLine"].(map[string]any)
			if version := textValue(line["Version"]); version == "HTTP/1.1" || version == "HTTP/1.0" {
				return version
			}
		}
	}
	if e.Protocol == "rtp" && e.Entry == "RTCP" {
		return "RTCP"
	}
	return protocolName(e.Protocol)
}

func recordType(v any) string {
	switch number(v) {
	case 20:
		return "Change Cipher Spec"
	case 21:
		return "Alert"
	case 22:
		return "Handshake"
	case 23:
		return "Application Data"
	case 24:
		return "Heartbeat"
	}
	return fmt.Sprintf("Record type %d", number(v))
}

func handshakeType(v any) string {
	if name := map[uint64]string{1: "Client Hello", 2: "Server Hello", 4: "New Session Ticket", 8: "Encrypted Extensions", 11: "Certificate", 12: "Server Key Exchange", 13: "Certificate Request", 14: "Server Hello Done", 15: "Certificate Verify", 16: "Client Key Exchange", 20: "Finished", 24: "Key Update", 254: "Message Hash"}[number(v)]; name != "" {
		return name
	}
	return fmt.Sprintf("Handshake type %d", number(v))
}

func alertName(v any) string {
	if name := map[uint64]string{0: "close_notify", 10: "unexpected_message", 20: "bad_record_mac", 22: "record_overflow", 40: "handshake_failure", 42: "bad_certificate", 43: "unsupported_certificate", 44: "certificate_revoked", 45: "certificate_expired", 46: "certificate_unknown", 47: "illegal_parameter", 48: "unknown_ca", 49: "access_denied", 50: "decode_error", 51: "decrypt_error", 70: "protocol_version", 71: "insufficient_security", 80: "internal_error", 90: "user_canceled", 109: "missing_extension", 110: "unsupported_extension", 112: "unrecognized_name", 116: "certificate_required", 120: "no_application_protocol"}[number(v)]; name != "" {
		return name
	}
	return fmt.Sprintf("alert_%d", number(v))
}

func tlsKeyStatus(info map[string]any) string {
	switch textValue(info["Secret Lookup Status"]) {
	case "missing-key":
		return "key unavailable"
	case "missing-context":
		return "handshake context missing"
	case "unsupported-suite":
		return "cipher suite not supported for decryption"
	case "found":
		if info["Authentication Verified"] == true {
			return "record AEAD verified"
		}
	case "recovery-exhausted":
		return "decryption recovery budget exhausted; encrypted record only"
	}
	return ""
}

func secureTransportSummary(e *pcaputil.ProtocolEvent) string {
	v := eventValues(e)
	if e.Protocol == "ssh" {
		parts := []string{textValue(v["Packet Name"])}
		for _, key := range []string{"Kex Algorithm", "Host Key Algorithm", "Host Key Fingerprint", "Identification"} {
			if value := textValue(v[key]); value != "" {
				parts = append(parts, value)
			}
		}
		if v["Encrypted"] == true {
			parts = append(parts, "encrypted transport; plaintext unavailable")
		}
		return strings.Join(parts, " · ")
	}
	if e.Protocol == "ssdp" {
		name := textValue(v["Method"])
		if name == "" {
			name = "200 OK"
		}
		parts := []string{name}
		for _, key := range []string{"Notification Subtype", "Discovery Target"} {
			if s := textValue(v[key]); s != "" {
				parts = append(parts, s)
			}
		}
		if age, ok := v["Maximum Age Seconds"]; ok {
			parts = append(parts, fmt.Sprintf("max-age=%d", number(age)))
		}
		if location := textValue(v["Location"]); location != "" {
			parts = append(parts, location)
		}
		return strings.Join(parts, " · ")
	}
	if e.Protocol == "quic" || e.Protocol == "http3" || e.Protocol == "doq" {
		parts := []string{textValue(v["Packet Name"])}
		if _, ok := v["Packet Number"]; ok {
			parts = append(parts, fmt.Sprintf("pn=%d", number(v["Packet Number"])))
		}
		if _, ok := v["HTTP3 Stream ID"]; ok {
			parts = append(parts, fmt.Sprintf("stream=%d", number(v["HTTP3 Stream ID"])))
		}
		frames := mapList(v["HTTP3 Frames"])
		for i, frame := range frames {
			if i >= 3 {
				break
			}
			name := textValue(frame["Frame Type"])
			if name == "DATA" {
				name += fmt.Sprintf(" %d bytes", number(frame["DATA Length"]))
			}
			if name != "" {
				parts = append(parts, name)
			}
		}
		if len(frames) == 0 && textValue(v["HTTP3 Stream Kind"]) == "request" {
			parts = append(parts, "HTTP/3 frame reassembly")
		}
		for _, h := range mapList(v["Headers"]) {
			if name := textValue(h["Name"]); name == ":method" || name == ":path" || name == ":status" {
				parts = append(parts, textValue(h["Value"]))
			}
		}
		if dns, ok := v["DoQ Message"].(map[string]any); ok {
			parts = append(parts, "DNS="+textValue(dns["QNAME"]))
		}
		if v["Authentication Verified"] == true {
			parts = append(parts, "packet AEAD verified")
		} else if v["Encrypted"] == true {
			parts = append(parts, "encrypted")
		}
		if v["Retired Destination CID"] == true {
			parts = append(parts, "late packet · retired CID")
		}
		return strings.Join(parts, " · ")
	}
	var parts []string
	for _, h := range mapList(v["Handshake Messages"]) {
		name := handshakeType(h["Type"])
		if h["Hello Retry Request"] == true {
			name = "Hello Retry Request"
		}
		parts = append(parts, name)
	}
	if len(parts) == 0 {
		if _, ok := v["Alert Description"]; ok {
			level := "warning"
			if number(v["Alert Level"]) == 2 {
				level = "fatal"
			}
			parts = append(parts, "Alert · "+level+" · "+alertName(v["Alert Description"]))
		} else if textValue(v["Content Visibility"]) == "encrypted" {
			parts = append(parts, "Encrypted record")
		} else if typ, ok := v["Inner Content Type"]; ok && v["Authentication Verified"] == true {
			parts = append(parts, recordType(typ))
		} else if typ, ok := v["Record Type"]; ok {
			parts = append(parts, recordType(typ))
		} else {
			parts = append(parts, safeText(e.Summary))
		}
	}
	if v["Handshake Incomplete"] == true {
		parts = append(parts, "handshake reassembly incomplete")
	}
	if status := tlsKeyStatus(v); status != "" {
		parts = append(parts, status)
	}
	if v["Record Recovery Verified"] == true {
		parts = append(parts, "record recovered · earlier plaintext unavailable")
	} else if v["Authentication Verified"] == true && textValue(v["Application Decoding"]) == "unavailable across prior plaintext gap" {
		parts = append(parts, "application gap · authenticated record bytes available")
	}
	if name := textValue(v["SNI"]); name != "" {
		parts = append(parts, "SNI="+name)
	}
	if alpn := textValue(v["ALPN"]); alpn != "" {
		parts = append(parts, "ALPN="+alpn)
	}
	if e.Error != "" {
		parts = append(parts, safeText(e.Error))
	}
	return strings.Join(parts, " · ")
}

func http2Frame(v any) string {
	if name := map[uint64]string{0: "DATA", 1: "HEADERS", 2: "PRIORITY", 3: "RST_STREAM", 4: "SETTINGS", 5: "PUSH_PROMISE", 6: "PING", 7: "GOAWAY", 8: "WINDOW_UPDATE", 9: "CONTINUATION"}[number(v)]; name != "" {
		return name
	}
	return fmt.Sprintf("frame %d", number(v))
}

func applicationEventSummary(e *pcaputil.ProtocolEvent) string {
	if e.Protocol == "pim" {
		v := eventValues(e)
		parts := []string{textValue(v["Packet Name"])}
		address := func(key string) string {
			m, _ := v[key].(map[string]any)
			return textValue(m["Address"])
		}
		switch number(v["Type"]) {
		case 0:
			for _, o := range mapList(v["Options"]) {
				if hold, ok := o["Hold Time"]; ok {
					label := fmt.Sprintf("advertised hold=%d s", number(hold))
					if number(hold) == 65535 {
						label = "advertised hold=infinite"
					}
					if number(hold) == 0 {
						label = "goodbye (hold=0)"
					}
					parts = append(parts, label)
				}
				if id, ok := o["Generation ID"]; ok {
					parts = append(parts, fmt.Sprintf("GenID=0x%08x", number(id)))
				}
				if priority, ok := o["DR Priority"]; ok {
					parts = append(parts, fmt.Sprintf("advertised DR priority=%d", number(priority)))
				}
			}
		case 1:
			m, _ := v["Register IP Header"].(map[string]any)
			parts = append(parts, fmt.Sprintf("(S,G)=(%s,%s)", textValue(m["Source"]), textValue(m["Group"])))
			if v["Null Register"] == true {
				parts[0] = "Null-Register"
				parts = append(parts, "dummy header; advertised addresses only")
			}
		case 3, 6, 7:
			var joins, prunes uint64
			for _, g := range mapList(v["Groups"]) {
				joins += number(g["Join Count"])
				prunes += number(g["Prune Count"])
			}
			parts = append(parts, fmt.Sprintf("upstream=%s · groups=%d · joins=%d prunes=%d", address("Upstream Neighbor"), number(v["Group Count"]), joins, prunes))
		case 2, 5, 9:
			parts = append(parts, fmt.Sprintf("(S,G)=(%s,%s)", address("Source"), address("Group")))
		case 4:
			parts = append(parts, fmt.Sprintf("BSR=%s · fragment=0x%04x · RP-set not assembled", address("BSR"), number(v["Fragment Tag"])))
		case 8:
			parts = append(parts, fmt.Sprintf("advertised RP=%s · groups=%d", address("RP"), number(v["Prefix Count"])))
		}
		return strings.Join(parts, " · ")
	}
	if e.Protocol == "ldp" {
		v := eventValues(e)
		parts := []string{fmt.Sprintf("LSR %s:%d", textValue(v["LSR ID"]), number(v["Label Space ID"]))}
		messages := mapList(v["Messages"])
		if len(messages) > 0 && number(messages[0]["Message Type"]) != 0x100 {
			for i, m := range messages {
				if i == 3 {
					parts = append(parts, fmt.Sprintf("%d more messages", len(messages)-i))
					break
				}
				parts = append(parts, textValue(m["Packet Name"]))
			}
			return strings.Join(parts, " · ")
		}
		if len(messages) > 0 {
			for _, tlv := range mapList(messages[0]["TLVs"]) {
				if number(tlv["TLV Type"]) == 0x0400 {
					kind := "Link Hello"
					if tlv["Targeted"] == true {
						kind = "Targeted Hello"
					}
					hold := fmt.Sprintf("%d s", number(tlv["Hold Time"]))
					if number(tlv["Hold Time"]) == 0 {
						hold = "default"
					}
					if number(tlv["Hold Time"]) == 65535 {
						hold = "infinite"
					}
					parts = append(parts, kind, "advertised hold="+hold)
				}
				for _, key := range []string{"IPv4 Transport Address", "IPv6 Transport Address"} {
					if addr := textValue(tlv[key]); addr != "" {
						parts = append(parts, "transport="+addr)
					}
				}
			}
		}
		if len(messages) > 1 {
			parts = append(parts, fmt.Sprintf("%d Hello messages; first shown", len(messages)))
		}
		return strings.Join(parts, " · ")
	}
	if e.Protocol == "hsrp" || e.Protocol == "glbp" || e.Protocol == "bfd" {
		v := eventValues(e)
		parts := []string{eventProtocolName(e) + " " + textValue(v["Packet Name"])}
		if state, exists := v["State"]; exists {
			label := fmt.Sprint(state)
			if e.Protocol == "bfd" {
				label = map[uint64]string{0: "AdminDown", 1: "Down", 2: "Init", 3: "Up"}[number(state)]
			} else if number(v["Protocol Version"]) == 2 {
				label = map[uint64]string{0: "Disabled", 1: "Init", 2: "Learn", 3: "Listen", 4: "Speak", 5: "Standby", 6: "Active"}[number(state)]
			} else {
				label = map[uint64]string{0: "Initial", 1: "Learn", 2: "Listen", 4: "Speak", 8: "Standby", 16: "Active"}[number(state)]
			}
			parts = append(parts, "advertises "+label)
		}
		for _, key := range []string{"Group", "Virtual IP", "My Discriminator", "Your Discriminator", "Detect Multiplier", "Active Groups", "Passive Groups"} {
			if value, exists := v[key]; exists {
				parts = append(parts, fmt.Sprintf("%s=%v", key, value))
			}
		}
		if v["Poll"] == true {
			parts = append(parts, "Poll")
		}
		if v["Final"] == true {
			parts = append(parts, "Final")
		}
		if e.Protocol == "glbp" {
			if rows, ok := v["TLVs"].([]map[string]any); ok {
				for _, row := range rows {
					if number(row["Type"]) == 1 {
						parts = append(parts, fmt.Sprintf("gateway state=%v · VIP=%v", row["Gateway State"], row["Virtual IP"]))
					}
					if number(row["Type"]) == 2 && len(parts) < 6 {
						parts = append(parts, fmt.Sprintf("forwarder=%v · state=%v · weight=%v", row["Forwarder"], row["Forwarder State"], row["Weight"]))
					}
				}
			}
		}
		return strings.Join(parts, " · ")
	}
	if e.Protocol == "bgp" || e.Protocol == "ajp" || e.Protocol == "rtmp" || e.Protocol == "amf0" {
		v := eventValues(e)
		parts := []string{safeText(e.Summary)}
		keys := []string{"Method", "URI", "Status Code", "Reuse", "My AS", "BGP Identifier", "Hold Time", "Message Stream ID", "Chunk Stream ID", "Timestamp", "Observed Message Bytes", "Message Length"}
		for _, key := range keys {
			if value, ok := v[key]; ok {
				parts = append(parts, fmt.Sprintf("%s=%v", key, value))
			}
			if len(parts) >= 6 {
				break
			}
		}
		return strings.Join(parts, " · ")
	}
	if e.Protocol == "tls" || e.Protocol == "quic" || e.Protocol == "http3" || e.Protocol == "doq" || e.Protocol == "ssdp" || e.Protocol == "ssh" {
		return secureTransportSummary(e)
	}
	v := eventValues(e)
	if e.Protocol == "wireguard" || e.Protocol == "openvpn" {
		parts := []string{textValue(v["Packet Name"])}
		for _, key := range []string{"Sender Index", "Receiver Index", "Counter", "Key ID"} {
			if value, ok := v[key]; ok {
				parts = append(parts, fmt.Sprintf("%s=%d", key, number(value)))
			}
		}
		if v["Content Visibility"] == "encrypted" {
			parts = append(parts, "encrypted")
		}
		parts = append(parts, "authentication unverified")
		return strings.Join(parts, " · ")
	}
	if names := authenticationLayers(e); len(names) > 0 {
		auth, _ := e.Session["Authentication"].(map[string]any)
		parts := []string{"SESSION_SETUP", strings.Join(names, " › ")}
		if child, ok := auth["Mechanism"].(map[string]any); ok {
			auth = child
		}
		for _, key := range []string{"Message Name", "Token Kind", "User Name", "Domain", "Target Name"} {
			if text := textValue(auth[key]); text != "" {
				parts = append(parts, key+"="+text)
			}
		}
		return strings.Join(append(parts, "authentication not verified"), " · ")
	}
	if e.Protocol == "http2" || e.Protocol == "grpc" {
		parts := []string{http2Frame(v["Frame Type"]), fmt.Sprintf("stream=%d", number(v["Stream ID"]))}
		for _, h := range mapList(v["Headers"]) {
			name, value := textValue(h["Name"]), textValue(h["Value"])
			switch name {
			case ":method", ":path":
				parts = append(parts, value)
			case ":status":
				parts = append(parts, "status="+value)
			case "grpc-status":
				parts = append(parts, "grpc-status="+value)
			}
		}
		if ms := mapList(v["GRPC Messages"]); len(ms) > 0 {
			parts = append(parts, fmt.Sprintf("%d message(s)", len(ms)))
			for i, m := range ms {
				if i < 3 {
					data, _ := grpcMessageData(m)
					parts = append(parts, fmt.Sprintf("%d bytes", len(data)))
				}
			}
		}
		if v["Payload Decrypted"] == true {
			parts = append(parts, "TLS decrypted")
		}
		return strings.Join(parts, " · ")
	}
	parts := []string{}
	if summary := safeText(e.Summary); summary != "" && !strings.EqualFold(summary, e.Protocol) {
		parts = append(parts, summary)
	}
	for _, key := range []string{"Packet Name", "Message Name", "Command", "QNAME", "QTYPE Name", "Method", "Request URI", "Status", "Reply Code", "File Name", "Topic", "SQL", "Query", "Security"} {
		value, ok := v[key]
		if !ok {
			value, ok = e.Fields[key]
		}
		if !ok {
			continue
		}
		if text, ok := value.(string); ok {
			if text != "" {
				parts = append(parts, key+"="+clipText(text, 128))
			}
		} else if scalarNumber(value) {
			parts = append(parts, fmt.Sprintf("%s=%v", key, value))
		}
		if len(parts) >= 6 {
			break
		}
	}
	if len(parts) == 0 {
		return safeText(e.Summary)
	}
	return strings.Join(parts, " · ")
}

func scalarNumber(v any) bool {
	switch v.(type) {
	case uint, uint8, uint16, uint32, uint64, int, int8, int16, int32, int64, float32, float64, json.Number:
		return true
	}
	return false
}

func bytesValue(v any) []byte {
	if b, ok := v.([]byte); ok {
		return b
	}
	if s, ok := v.(string); ok {
		b, err := base64.StdEncoding.DecodeString(s)
		if err == nil {
			return b
		}
	}
	return nil
}

func grpcMessageData(m map[string]any) ([]byte, string) {
	if value, ok := m["Decoded Payload"]; ok {
		return bytesValue(value), "decompressed"
	}
	if m["Compressed"] == true {
		return bytesValue(m["Payload"]), "compressed payload (not decompressed)"
	}
	return bytesValue(m["Payload"]), "decoded payload"
}

func eventPriority(e *pcaputil.ProtocolEvent) int {
	p := 20
	if e.Protocol == "pim" {
		p = 5 // preserve the control layer without hiding admitted inner data
	}
	if e.Protocol == "tls" || e.Protocol == "quic" || e.Protocol == "dtls" {
		p = 10
	}
	if len(e.CarrierLayers) > 0 {
		p = 20 + 5*len(e.CarrierLayers)
	}
	if e.Protocol == "grpc" {
		p = 30
	}
	if e.SourceBytes.Kind == "decrypted" || e.Session["Payload Decrypted"] == true {
		p += 100
	}
	return p
}

func (p *packetPresentation) add(e *pcaputil.ProtocolEvent) {
	if e == nil {
		return
	}
	// These parsers validated a complete outer message before refusing inner
	// decryption/extensions. A missing key/profile must not erase that proven
	// identity, and it must not turn the inner payload into decoded fields.
	if e.Status == "context-required" && e.Protocol == "snmp" && e.ExpertCode == string(pcaputil.ErrEncrypted) && e.Session["Encrypted"] == true && number(e.Session["Version"]) == 3 {
		p.protocol, p.priority, p.status, p.visibility = "SNMP", 20, e.Status, "encrypted"
		p.evidence, p.summary = "validated SNMPv3 USM envelope", "SNMPv3 · encrypted scoped PDU · decryption key unavailable; authentication not verified"
		p.addLayer("SNMP")
		return
	}
	if e.Status == "context-required" && e.Protocol == "ntp" && e.Completeness == "headers" && e.Session["Trailing Content"] == "extension-or-authentication-unparsed" {
		p.protocol, p.priority, p.status, p.visibility = "NTP", 20, e.Status, "headers"
		p.evidence = "validated NTP base header; trailing extensions/authentication unparsed"
		p.summary = fmt.Sprintf("NTPv%d %s · stratum %d · %d trailing bytes; extensions/authentication unparsed", number(e.Session["Version"]), textValue(e.Session["Packet Name"]), number(e.Session["Stratum"]), number(e.Session["Trailing Bytes"]))
		p.addLayer("NTP")
		return
	}
	if protectedSSHEvent(e) {
		p.protocol, p.status, p.visibility = "SSH", "encrypted", "encrypted"
		p.evidence, p.summary = "observed SSH NEWKEYS", "SSH encrypted transport · NEWKEYS observed; plaintext unavailable"
		p.addLayer("SSH")
		return
	}
	if (e.Status == "decoded" || e.Status == "deferred") && !credibleTLSIdentity(e) {
		// The complete record envelope is useful evidence, but not a negotiated
		// session or authenticated ciphertext. Give that evidence its own label.
		if p.priority <= 10 {
			v := eventValues(e)
			p.protocol, p.priority, p.status, p.visibility = "TLS?", 10, "unconfirmed", "encrypted"
			p.evidence = "complete TLS record envelope; session identity unconfirmed"
			p.summary = fmt.Sprintf("%s record · %d bytes · session unconfirmed (handshake not captured)", recordType(v["Record Type"]), number(v["Record Length"]))
			if strongTLSRecord(e) {
				p.protocol, p.status = "TLS", "record-only"
				p.evidence = "complete protected TLS record framing; negotiated version unavailable"
				p.summary = fmt.Sprintf("%s record · %d bytes · handshake not captured; version unknown", recordType(v["Record Type"]), number(v["Record Length"]))
			}
			p.addLayer(p.protocol)
		}
		return
	}
	if protectedConversationEvent(e) {
		p.addLayer("QUIC")
		if p.priority > 10 {
			p.diagnostic = "Additional QUIC packet remains encrypted"
			return
		}
		p.protocol, p.priority, p.visibility, p.status, p.evidence = "QUIC", 10, "encrypted", e.Status, "observed QUIC CID"
		p.addLayer("QUIC")
		p.summary = textValue(e.Session["Packet Name"]) + " · protected payload · " + textValue(e.Session["Key Reason"])
		return
	}
	if e.Status != "decoded" && e.Status != "deferred" || e.Error != "" {
		if e.Error != "" || e.ExpertCode != "" {
			p.diagnostic = clipText(protocolName(e.Protocol)+" · "+e.ExpertCode+" · "+e.Error, 256)
			if p.protocol == "" {
				p.status = e.Status
			}
			if e.Protocol == "tls" && e.ExpertCode == string(pcaputil.ErrAuthenticationFailed) && p.protocol == "" {
				p.protocol, p.visibility = eventProtocolName(e), "encrypted"
				p.addLayer(p.protocol)
				p.summary = "Record authentication failed · plaintext unavailable"
			}
			if e.Protocol == "tls" && e.ExpertCode == string(pcaputil.ErrAuthenticationFailed) {
				p.rejectedTLS = true
			}
			if e.Protocol == "quic" && e.FlowID != 0 && e.ExpertCode == string(pcaputil.ErrAuthenticationFailed) && p.protocol == "" {
				p.protocol, p.visibility, p.evidence = "QUIC", "encrypted", "observed connection CID; packet authentication failed"
				p.addLayer("QUIC")
				p.summary = "Packet authentication failed · plaintext unavailable"
			}
			if e.Protocol == "quic" && e.FlowID != 0 && e.Admission == "authenticated-wire-and-observed-CID" && p.protocol == "" {
				p.protocol, p.visibility, p.evidence = "QUIC", "unavailable", "observed QUIC connection CID; protocol diagnostic"
				p.addLayer("QUIC")
				p.summary = clipText(e.Error, 256)
			}
		}
		return
	}
	if e.Protocol == "" {
		return
	}
	label := eventProtocolName(e)
	for _, layer := range eventCarriers(e) {
		p.addLayer(layer)
	}
	p.addLayer(label)
	rank := eventPriority(e)
	if rank < p.priority {
		return
	}
	info := clipText(applicationEventSummary(e), 384)
	if rank > p.priority || p.protocol != label {
		p.summary, p.count = info, 1
	} else {
		p.count++
		if p.count <= 3 && info != "" && !strings.Contains(p.summary, info) {
			p.summary += " | " + info
		}
	}
	p.protocol, p.priority, p.status = label, rank, e.Status
	p.evidence = ""
	p.visibility = e.SourceBytes.Kind
	if v := textValue(eventValues(e)["Content Visibility"]); v != "" {
		p.visibility = v
	}
}

func (p *packetPresentation) addLayer(label string) {
	for i, old := range p.stack {
		if old == label {
			return
		}
		if strings.HasPrefix(label, "TLSv") && old == "TLS" {
			p.stack[i] = label
			return
		}
		if label == "TLS" && strings.HasPrefix(old, "TLSv") {
			return
		}
		// SYN uses the legacy header before the selected RDPUDP2 wire format.
		// This is one evolving carrier, not RDPUDP nested inside RDPUDP2.
		if label == "RDPUDP2" && old == "RDPUDP" {
			p.stack[i] = label
			return
		}
		if label == "RDPUDP" && old == "RDPUDP2" {
			return
		}
	}
	if len(p.stack) < 12 {
		p.stack = append(p.stack, label)
	}
}

func presentEvents(events []*pcaputil.ProtocolEvent) packetPresentation {
	var p packetPresentation
	for _, e := range events {
		p.add(e)
	}
	return p
}

func clipText(s string, limit int) string {
	runes := []rune(safeText(s))
	if len(runes) > limit {
		return string(runes[:limit]) + "…"
	}
	return string(runes)
}

func (p packetPresentation) apply(row packetSummary) packetSummary {
	if p.protocol == "" {
		if p.diagnostic != "" {
			row.Info = p.diagnostic + " · " + row.Info
			row.ProtocolStatus, row.ProtocolEvidence = p.status, "protocol parser diagnostic"
			if row.Protocol == "TLS" {
				row.Protocol = strings.ToUpper(row.Transport)
			}
		}
		return row
	}
	row.Protocol = p.protocol
	row.ProtocolStack = append([]string(nil), row.Layers...)
	if row.TransportView != nil {
		row.ProtocolStack = append([]string(nil), row.TransportView.Layers...)
	}
	if row.MessageTransport != "" {
		carrier := strings.ToUpper(row.MessageTransport)
		present := false
		for _, name := range row.ProtocolStack {
			if name == carrier {
				present = true
			}
		}
		if !present {
			network := "IPv4"
			if strings.HasPrefix(row.MessageSource, "[") {
				network = "IPv6"
			}
			row.ProtocolStack = append(row.ProtocolStack, network, carrier)
		}
	}
	// A native control event can describe a layer already exposed by capture
	// decoding (PIM, ARP, etc.). Do not append it a second time. Preserve the
	// original chain itself: repeated IP headers are real tunnel layers.
	for _, semantic := range p.stack {
		present := false
		for _, physical := range row.ProtocolStack {
			if semantic == physical {
				present = true
				break
			}
		}
		if !present {
			row.ProtocolStack = append(row.ProtocolStack, semantic)
		}
	}
	row.ProtocolScope = "protocol message"
	if row.Transport == "tcp" {
		row.ProtocolScope = "reassembled message"
	}
	if row.Transport == "udp" {
		row.ProtocolScope = "datagram message"
	}
	if p.visibility == "decrypted" || p.visibility == "authenticated-plaintext" && p.priority >= 100 {
		row.ProtocolScope = "decrypted message"
	}
	row.ProtocolEvidence = "validated protocol events"
	if p.evidence != "" {
		row.ProtocolEvidence = p.evidence
		row.ProtocolScope = "connection envelope"
		if p.protocol == "TLS?" || p.status == "record-only" {
			row.ProtocolScope = "candidate record"
			if p.status == "record-only" {
				row.ProtocolScope = "record envelope"
			}
		}
	}
	row.Visibility, row.ProtocolStatus = p.visibility, p.status
	row.Info = p.summary
	if p.rejectedTLS && p.count > 0 {
		row.Info = "Includes rejected TLS record · " + row.Info
		row.ProtocolStatus = "mixed-record-outcomes"
	}
	if p.count > 3 {
		row.Info += fmt.Sprintf(" (+%d further PDUs)", p.count-3)
	}
	if p.diagnostic != "" {
		row.Info += " · " + p.diagnostic
	}
	if row.Info == "" {
		row.Info = p.protocol
	}
	return row
}

func tlsDisplayValue(name string, value any) string {
	n := number(value)
	switch name {
	case "Negotiated Version":
		if n == 0 {
			return "unknown · negotiation not captured yet"
		}
		return fmt.Sprintf("%s (0x%04x)", tlsVersion(value), n)
	case "Record Version":
		return fmt.Sprintf("0x%04x · record legacy version", n)
	case "Cipher Suite":
		if n == 0 {
			return "not selected yet"
		}
		return fmt.Sprintf("%s (0x%04x)", tls.CipherSuiteName(uint16(n)), n)
	case "Record Type", "Inner Content Type":
		return fmt.Sprintf("%s (%d)", recordType(value), n)
	case "Alert Description":
		return fmt.Sprintf("%s (%d)", alertName(value), n)
	case "Alert Level":
		if n == 1 {
			return "warning (1)"
		}
		if n == 2 {
			return "fatal (2)"
		}
	}
	return formatFieldValue(name, value)
}
