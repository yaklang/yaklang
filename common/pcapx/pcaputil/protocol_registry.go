package pcaputil

import "fmt"

// ProtocolProfile describes the native ingress implemented by this registry.
// Probe evidence and user DecodeAs selection are separate event properties.
type ProtocolProfile struct{ Protocol, Transport, Framing, Context string }

func NativeProtocolProfiles() []ProtocolProfile {
	return []ProtocolProfile{
		{"dns", "udp", "datagram", "question and endpoint transaction"}, {"mdns", "udp", "datagram", "multicast observation"}, {"llmnr", "udp", "datagram", "question and endpoint transaction"}, {"dhcp", "udp", "datagram", "domain/client/xid observation"}, {"dhcpv6", "udp", "datagram", "domain/relay/client/xid observation"},
		{"arp", "l2", "ethernet", "unverified neighbor"}, {"icmp", "network", "IP", "quoted packet observation"}, {"icmpv6", "network", "IPv6", "unverified neighbor"},
		{"http", "tcp", "ordered bytes", "observed request method"}, {"websocket", "tcp", "ordered bytes", "validated HTTP upgrade"}, {"tls", "tcp", "records", "ClientRandom/direction/epoch"}, {"http2", "tcp", "frames", "preface and HPACK state"}, {"grpc", "tcp", "HTTP2 DATA", "content-type and stream"},
	}
}

// WithProtocolDecodeAs selects a native UDP codec on a port. Selection does not
// bypass validation or grant authenticated/decrypted status. Only listed native
// datagram profiles are accepted; legacy TCP Bindings remain source compatible.
func WithProtocolDecodeAs(transport string, port uint16, protocol string) CaptureOption {
	return func(c *CaptureConfig) error {
		if transport != "udp" || port == 0 {
			return fmt.Errorf("DecodeAs requires UDP and a nonzero port")
		}
		switch protocol {
		case "dns", "mdns", "llmnr", "dhcp", "dhcpv6":
		default:
			return fmt.Errorf("unsupported native DecodeAs profile")
		}
		if c.datagramDecodeAs == nil {
			c.datagramDecodeAs = map[uint16]string{}
		}
		if old := c.datagramDecodeAs[port]; old != "" && old != protocol {
			return fmt.Errorf("conflicting DecodeAs profile")
		}
		c.datagramDecodeAs[port] = protocol
		return nil
	}
}

type nativeDatagramProfile struct {
	name     string
	ports    []uint16
	validate func([]byte) bool
	decode   func([]byte, int) (map[string]any, error)
}

var nativeDatagrams = []nativeDatagramProfile{
	{"mdns", []uint16{5353}, dnsHeader, DecodeDNSMessage},
	{"llmnr", []uint16{5355}, dnsHeader, DecodeDNSMessage},
	{"dns", []uint16{53}, dnsHeader, DecodeDNSMessage},
	{"dhcpv6", []uint16{546, 547}, func(w []byte) bool { return len(w) >= 4 && w[0] >= 1 && w[0] <= 13 }, func(w []byte, n int) (map[string]any, error) { return decodeDHCPv6(w, 0, n) }},
	{"dhcp", nil, func(w []byte) bool { return len(w) >= 240 && probeDHCP(w, len(w)).Verdict == ProbeAccept }, decodeDHCP4},
}

func (a *binParser) decodeNativeDatagram(e *ProtocolEvent, w []byte, src, dst uint16) bool {
	explicit := a.datagramDecodeAs[dst]
	if explicit == "" {
		explicit = a.datagramDecodeAs[src]
	}
	for _, p := range nativeDatagrams {
		hint := len(p.ports) == 0
		for _, port := range p.ports {
			hint = hint || src == port || dst == port
		}
		if explicit != "" {
			if explicit != p.name {
				continue
			}
		} else if !hint || !p.validate(w) {
			continue
		}
		e.Protocol = p.name
		e.Profile = p.name + "-native"
		e.Admission = "wire-and-port-hint"
		if len(p.ports) == 0 {
			e.Admission = "wire-signature"
		}
		if explicit != "" {
			e.Admission = "explicit-decode-as"
		}
		e.Raw = append([]byte(nil), w...)
		fields, err := p.decode(w, a.budget.MaxCollectionElements)
		e.Session = fields
		e.Completeness = "message"
		if err == nil {
			switch p.name {
			case "dns", "mdns", "llmnr":
				e.Session = map[string]any{}
				err = a.dnsEventDecoded(e, fields)
			case "dhcp":
				a.dhcpObservation(e)
			case "dhcpv6":
				a.dhcp6Observation(e)
			}
		}
		e.semanticFields = cloneSession(fields)
		e.Structured = map[string]any{"fields": e.semanticFields}
		e.Status = "decoded"
		e.Summary = p.name
		a.messages.Add(1)
		a.messageBytes.Add(uint64(len(w)))
		if err != nil {
			e.Status, e.sessionError = classifySessionError(err)
			e.Error = err.Error()
			a.malformed.Add(1)
		} else if a.config.Deferred {
			e.Status = "deferred"
			e.Structured = nil
			a.deferred.Add(1)
		} else {
			a.decoded.Add(1)
		}
		return true
	}
	return false
}
