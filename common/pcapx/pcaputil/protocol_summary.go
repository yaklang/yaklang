package pcaputil

import (
	"fmt"
	"strings"

	"github.com/gopacket/gopacket/layers"
)

// Summaries describe captured evidence. Unknown payloads keep their diagnostic
// status; conventional ports never turn them into an identified protocol.
func (e *ProtocolEvent) summarizeProtocol() {
	if e.Status == "unrecognized" && e.Transport == "udp" && e.Protocol == "" {
		e.ExpertCode = "ProtocolNotRecognized"
		e.Summary = fmt.Sprintf("UDP application protocol not recognized (%s -> %s, %d bytes)", e.Source, e.Destination, e.Length)
		return
	}
	if e.Status != "decoded" && e.Status != "deferred" || e.Error != "" {
		return
	}
	// Preserve summaries already supplied by a more specific protocol decoder.
	if e.Summary != "" && e.Summary != e.Protocol {
		return
	}
	switch e.Protocol {
	case "arp":
		op := fmt.Sprintf("operation %v", e.Session["Operation"])
		switch e.Session["Operation"] {
		case uint16(1):
			op = "request"
		case uint16(2):
			op = "reply"
		}
		e.Summary = fmt.Sprintf("ARP %s %s -> %s sender_mac=%v", op, e.Source, e.Destination, e.Session["Sender MAC"])
	case "icmp", "icmpv6":
		typ, ok := e.Session["Type"].(uint8)
		code, codeOK := e.Session["Code"].(uint8)
		if !ok || !codeOK {
			return
		}
		name := layers.CreateICMPv4TypeCode(typ, code).String()
		if e.Protocol == "icmpv6" {
			name = layers.CreateICMPv6TypeCode(typ, code).String()
		}
		e.Summary = fmt.Sprintf("%s %s", strings.ToUpper(e.Protocol), name)
		if target, ok := e.Session["Target"].(string); ok {
			e.Summary += " target=" + target
		}
	case "tls":
		typ, ok := e.Session["Record Type"].(byte)
		if !ok {
			return
		}
		name := "Record"
		switch typ {
		case 20:
			name = "ChangeCipherSpec"
		case 21:
			name = "Alert"
		case 22:
			name = "Handshake"
		case 23:
			name = "Application Data"
		}
		if messages, ok := e.Session["Handshake Messages"].([]map[string]any); ok && len(messages) > 0 {
			switch messages[0]["Type"] {
			case byte(1):
				name = "ClientHello"
			case byte(2):
				name = "ServerHello"
			}
		}
		e.Summary = fmt.Sprintf("TLS %s (%v bytes, %v)", name, e.Session["Record Length"], e.Session["Content Visibility"])
		if sni, ok := e.Session["SNI"].(string); ok && sni != "" {
			e.Summary += " SNI=" + sni
		}
	case "dns", "mdns", "llmnr":
		dns, ok := e.Session["DNS"].(map[string]any)
		if !ok {
			return
		}
		name := strings.ToUpper(e.Protocol)
		if e.Protocol == "mdns" {
			name = "mDNS"
		}
		role := "query"
		if dns["Response"] == true {
			role = "response"
		}
		questions, _ := dns["Questions"].([]map[string]any)
		answers, _ := dns["Answers"].([]map[string]any)
		e.Summary = fmt.Sprintf("%s %s (questions=%d answers=%d)", name, role, len(questions), len(answers))
		rows := questions
		if len(rows) == 0 {
			rows = answers
		}
		if len(rows) > 0 {
			e.Summary += fmt.Sprintf(" %v", rows[0]["Name"])
			if typ, ok := rows[0]["Type"].(uint16); ok {
				e.Summary += " " + layers.DNSType(typ).String()
			}
		}
	}
}
