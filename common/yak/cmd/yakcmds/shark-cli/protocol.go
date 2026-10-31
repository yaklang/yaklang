package sharkcli

import (
	"fmt"
	"strings"
)

// Identification uses contiguous reassembled prefixes. Ports remain labelled
// as hints; ciphertext or an arbitrary middle segment is not a new protocol.
func sniffStreamPayload(p []byte, src, dst uint16) string {
	return sniffPayload(p)
}

// Names emitted by the branch's YAML payload dispatch. A successful lower-layer
// parse alone must never be advertised as successful application identification.
var applicationNodes = map[string]string{
	"SSDP": "SSDP",
	"TLS":  "TLS", "Transport Layer Security": "TLS", "HTTP": "HTTP", "HTTP2": "HTTP2", "HTTP2Preface": "HTTP2",
	"SSH": "SSH", "SSHPacket": "SSH", "Redis": "Redis", "RESP": "Redis", "MQTT": "MQTT", "FTP": "FTP", "FTPCommand": "FTP", "SMTP": "SMTP", "SMTPCommand": "SMTP",
	"DNS": "DNS", "SMB": "SMB", "SMB2": "SMB2", "SMB3Transform": "SMB3", "MySQL": "MySQL", "MySQLPacket": "MySQL", "PostgreSQL": "PostgreSQL", "PGStartup": "PostgreSQL", "PGSSLRequest": "PostgreSQL",
	"TDS": "TDS", "AJP": "AJP", "WebSocket": "WebSocket", "TNS": "TNS", "DCERPC": "DCERPC", "JavaSer": "Java serialization", "RDP": "RDP", "TPKT": "TPKT", "LDAPMessage": "LDAP",
	"Telnet": "Telnet", "POP3": "POP3", "IMAP": "IMAP", "BGP": "BGP", "VNC": "VNC", "MongoDB": "MongoDB", "Memcached": "Memcached", "AMQP": "AMQP", "Kafka": "Kafka", "RTSP": "RTSP", "RTMP": "RTMP", "BitTorrent": "BitTorrent", "Rsync": "Rsync", "Zabbix": "Zabbix",
}

func applyApplication(row packetSummary, protocol, evidence string, id uint64) packetSummary {
	if row.tcpHeaderFailure != nil {
		return row.tcpHeaderFailure.apply(row)
	}
	row.StreamID = id
	if row.closedTCPPayload || row.tcpUnconfirmed {
		row.StreamID = 0 // Keep the TCP namespace as evidence, not an old application-page association.
		if !row.connectionAnnotated {
			row.packetInfo, row.connectionAnnotated = row.Info, true
		}
		row.Protocol = row.packetProtocol
		row.ProtocolStack = append([]string(nil), row.Layers...)
		if row.TransportView != nil {
			row.ProtocolStack = append([]string(nil), row.TransportView.Layers...)
		}
		row.ConnectionProtocol, row.ConnectionStack = "", nil
		row.ConnectionEvidence = "native closed TCP observation; payload not admitted to reassembly"
		row.ProtocolEvidence = "application generation unconfirmed after observed TCP close"
		row.ProtocolScope, row.Visibility = "unadmitted closed TCP payload", "unassembled-stream"
		if row.ProtocolStatus == "" {
			row.ProtocolStatus = "context-required"
		}
		row.Info = "TCP payload after observed close; generation unconfirmed · " + row.packetInfo
		if row.payloadBytes == 0 {
			// The native callback retains a closed namespace for forensic
			// ownership. An ACK or reset after both directions closed does not
			// prove that its application belongs to that earlier generation.
			row.ConnectionEvidence = "native closed TCP observation; application generation unconfirmed"
			row.ProtocolScope, row.Visibility = "closed TCP observation", "header-only"
			row.Info = "TCP header after observed close; generation unconfirmed · " + row.packetInfo
		}
		return row
	}
	if protocol != "" {
		if row.connectionAnnotated {
			row.Info = row.packetInfo
		} else {
			row.packetInfo = row.Info
			row.connectionAnnotated = true
		}
		row.ConnectionEvidence = evidence
		label := protocolName(protocol)
		if evidence == "port hint" {
			row.ConnectionProtocol = ""
		} else {
			row.ConnectionProtocol = label
		}
		row = resolveTLSCandidate(row)
		if (row.Protocol == "TCP" || (row.ProtocolScope == "connection-associated bytes" || row.ProtocolScope == "connection-associated diagnostic") && row.packetProtocol == "TCP") && row.payloadBytes > 0 && row.ConnectionProtocol != "" && (evidence == "validated protocol events" || strings.HasPrefix(row.ConnectionProtocol, "TLS") || row.opaqueTransport) {
			row.Protocol = row.ConnectionProtocol
			if row.ProtocolEvidence == "protocol parser diagnostic" && (row.ProtocolStatus == "malformed" || row.ProtocolStatus == "limited") {
				// Connection identity cannot downgrade this packet's observed
				// failure to missing reassembly/context, including on redraw.
				row.ProtocolScope = "connection-associated diagnostic"
			} else {
				row.ProtocolStatus, row.ProtocolScope, row.Visibility = "context-required", "connection-associated bytes", "unassembled-stream"
				if strings.HasPrefix(row.ConnectionProtocol, "TLS") || row.opaqueTransport {
					row.Visibility = "encrypted"
				}
				row.ProtocolEvidence = "observed connection; this segment has no complete decoded message"
				row.Info = row.ConnectionProtocol + " stream bytes · no complete decoded message in this packet · " + row.Info
			}
			row.ProtocolStack = append(append([]string(nil), row.Layers...), row.ConnectionProtocol)
			if row.TransportView != nil {
				row.ProtocolStack = append(append([]string(nil), row.TransportView.Layers...), row.ConnectionProtocol)
			}
		}
		if id != 0 {
			if len(row.ConnectionStack) > 0 && evidence != "port hint" {
				label = strings.Join(row.ConnectionStack, " › ")
			}
			if evidence == "port hint" {
				label = "port hint=" + label
			}
			row.Info += " [connection #" + fmt.Sprint(id) + ": " + label + "]"
		}
	}
	return row
}

func resolveTLSCandidate(row packetSummary) packetSummary {
	if row.Protocol != "TLS?" || row.ConnectionProtocol == "" || !strings.HasPrefix(row.ConnectionProtocol, "TLS") {
		return row
	}
	row.Protocol = row.ConnectionProtocol
	row.ProtocolEvidence = "TLS record envelope in an observed TLS connection"
	row.ProtocolScope, row.ProtocolStatus = "connection envelope", "context-required"
	for i, layer := range row.ProtocolStack {
		if layer == "TLS?" {
			row.ProtocolStack[i] = row.Protocol
		}
	}
	row.Info = strings.ReplaceAll(row.Info, "session unconfirmed (handshake not captured)", "observed TLS connection; session metadata incomplete")
	return row
}

func (u *tui) packetRow(e *packetEntry) packetSummary {
	row := e.row()
	if e.packet.conversationID != 0 && e.packet.streamID == 0 {
		return u.streams.projectDatagram(row, e.packet)
	}
	return u.streams.projectConversation(row, e.packet.streamID)
}
