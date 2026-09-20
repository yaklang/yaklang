package sharkcli

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
)

// Identification uses contiguous reassembled prefixes. Ports remain labelled
// as hints; ciphertext or an arbitrary middle segment is not a new protocol.
func sniffStreamPayload(p []byte, src, dst uint16) string {
	if name := sniffPayload(p); name != "" {
		return name
	}
	if len(p) >= 8 && p[0] == 0 {
		if name := sniffPayload(p[4:]); name == "SMB" || name == "SMB2" {
			return name
		}
	}
	for prefix, name := range map[string]string{"AMQP": "AMQP", "RFB ": "VNC", "\x13BitTorrent protocol": "BitTorrent", "RTSP/1.": "RTSP"} {
		if bytes.HasPrefix(p, []byte(prefix)) {
			return name
		}
	}
	line := p[:min(len(p), 512)]
	if at := bytes.Index(line, []byte("\r\n")); at >= 0 {
		first := string(line[:at])
		if strings.HasSuffix(first, " RTSP/1.0") {
			return "RTSP"
		}
		if strings.HasPrefix(first, "220 ") && strings.Contains(strings.ToUpper(first), "SMTP") {
			return "SMTP"
		}
		if strings.HasPrefix(first, "220 ") && strings.Contains(strings.ToUpper(first), "FTP") {
			return "FTP"
		}
		if first == "+OK" || strings.HasPrefix(first, "-ERR ") {
			return "Redis"
		}
		if at > 1 && (p[0] == '*' || p[0] == '$' || p[0] == ':') {
			numeric := true
			for _, b := range p[1:at] {
				if (b < '0' || b > '9') && b != '-' {
					numeric = false
				}
			}
			if numeric && (p[0] == ':' || p[0] == '$' || at+2 < len(p) && p[at+2] == '$') {
				return "Redis"
			}
		}
	}
	if len(p) >= 10 && p[0] == 0x10 {
		offset := 2
		for offset <= 5 && offset <= len(p) && p[offset-1]&0x80 != 0 {
			offset++
		}
		if len(p) >= offset+6 && bytes.Equal(p[offset:offset+6], []byte{0, 4, 'M', 'Q', 'T', 'T'}) {
			return "MQTT"
		}
	}
	if len(p) >= 9 && p[3] == 0 && p[4] == 10 && bytes.IndexByte(p[5:min(len(p), 64)], 0) > 0 {
		return "MySQL"
	}
	if len(p) >= 8 && binary.BigEndian.Uint32(p[:4]) >= 8 {
		version := binary.BigEndian.Uint32(p[4:8])
		if version == 196608 || version == 80877103 || version == 80877102 {
			return "PostgreSQL"
		}
	}
	if (src == 53 || dst == 53) && len(p) >= 14 {
		n := int(binary.BigEndian.Uint16(p[:2]))
		if n >= 12 && n <= len(p)-2 {
			var dns layers.DNS
			if dns.DecodeFromBytes(p[2:2+n], gopacket.NilDecodeFeedback) == nil {
				return "DNS"
			}
		}
	}
	return ""
}

// Names emitted by the branch's YAML payload dispatch. A successful lower-layer
// parse alone must never be advertised as successful application identification.
var applicationNodes = map[string]string{
	"TLS": "TLS", "Transport Layer Security": "TLS", "HTTP": "HTTP", "HTTP2": "HTTP2", "HTTP2Preface": "HTTP2",
	"SSH": "SSH", "SSHPacket": "SSH", "Redis": "Redis", "RESP": "Redis", "MQTT": "MQTT", "FTP": "FTP", "FTPCommand": "FTP", "SMTP": "SMTP", "SMTPCommand": "SMTP",
	"DNS": "DNS", "SMB": "SMB", "SMB2": "SMB2", "SMB3Transform": "SMB3", "MySQL": "MySQL", "MySQLPacket": "MySQL", "PostgreSQL": "PostgreSQL", "PGStartup": "PostgreSQL", "PGSSLRequest": "PostgreSQL",
	"TDS": "TDS", "AJP": "AJP", "WebSocket": "WebSocket", "TNS": "TNS", "DCERPC": "DCERPC", "JavaSer": "Java serialization", "RDP": "RDP", "TPKT": "TPKT", "LDAPMessage": "LDAP",
	"Telnet": "Telnet", "POP3": "POP3", "IMAP": "IMAP", "BGP": "BGP", "VNC": "VNC", "MongoDB": "MongoDB", "Memcached": "Memcached", "AMQP": "AMQP", "Kafka": "Kafka", "RTSP": "RTSP", "RTMP": "RTMP", "BitTorrent": "BitTorrent", "Rsync": "Rsync", "Zabbix": "Zabbix",
}

func applyApplication(row packetSummary, protocol, evidence string, id uint64) packetSummary {
	row.StreamID = id
	if protocol != "" {
		if at := strings.Index(row.Info, " [stream: "); at >= 0 {
			row.Info = row.Info[:at]
		}
		if row.packetProtocol != "" {
			row.Protocol = row.packetProtocol
		}
		row.ProtocolEvidence = evidence
		if evidence == "port hint" {
			row.ProtocolScope = "port hint only"
		} else if row.packetProtocol != protocol {
			row.Protocol = "TCP/" + protocol
			row.ProtocolScope = "connection prefix, not this packet"
		} else {
			row.ProtocolScope = "packet"
		}
		if id != 0 {
			label := "prefix=" + protocol
			if evidence == "port hint" {
				label = "port hint=" + protocol
			}
			row.Info += " [stream: #" + fmt.Sprint(id) + " " + label + "; " + evidence + "]"
		}
	}
	return row
}

func (u *tui) packetRow(e *packetEntry) packetSummary {
	row := e.row()
	if protocol, evidence := u.streams.label(e.packet.streamID); protocol != "" {
		return applyApplication(row, protocol, evidence, e.packet.streamID)
	}
	return row
}
