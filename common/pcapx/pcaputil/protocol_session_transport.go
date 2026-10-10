package pcaputil

import (
	"encoding/binary"
	"fmt"
	"net"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
)

// ProtocolSessionOption supplies observed carrier information. The no-option
// constructor retains its historical byte-session behavior; new callers should
// select TCP or UDP rather than treating a datagram as a stream fragment.
type ProtocolSessionOption func(*captureSession) error

// WithSessionTransport selects an observed carrier. A UDP Feed is exactly one
// datagram. TCP admission excludes signatures belonging only to UDP profiles.
func WithSessionTransport(transport string) ProtocolSessionOption {
	return func(s *captureSession) error {
		if transport != "tcp" && transport != "udp" {
			return fmt.Errorf("session transport must be tcp or udp")
		}
		s.transport = transport
		s.f.captureTCP = transport == "tcp"
		return nil
	}
}

// WithSessionPorts supplies observed endpoint ports as hints. Wire validation
// still determines admission; ports never establish a negotiated context.
func WithSessionPorts(first, second uint16) ProtocolSessionOption {
	return func(s *captureSession) error {
		s.f.ports = [2]uint16{first, second}
		s.f.endpoints = [2]string{fmt.Sprintf("192.0.2.1:%d", first), fmt.Sprintf("192.0.2.2:%d", second)}
		return nil
	}
}

func (s *captureSession) feedDatagram(dir int, ts time.Time, raw []byte) FeedResult {
	s.events = nil
	before := s.f.a.input.Load()
	if len(raw) > 65527 {
		return FeedResult{State: "undetected", Err: &ProtocolError{Kind: ErrMalformedMessage, Message: "UDP payload exceeds datagram length"}}
	}
	source, dest := net.IPv4(192, 0, 2, 1), net.IPv4(192, 0, 2, 2)
	if dir == 1 {
		source, dest = dest, source
	}
	ip := &layers.IPv4{Version: 4, SrcIP: source, DstIP: dest}
	udp := &layers.UDP{SrcPort: layers.UDPPort(s.f.ports[dir]), DstPort: layers.UDPPort(s.f.ports[1-dir]), Length: uint16(len(raw) + 8), BaseLayer: layers.BaseLayer{Payload: raw}}
	s.f.a.datagramFields(ip, udp, gopacket.CaptureInfo{Timestamp: ts, CaptureLength: len(raw), Length: len(raw)}, false)
	out := FeedResult{Consumed: int(s.f.a.input.Load() - before), Events: s.events, State: "undetected"}
	for _, e := range out.Events {
		e.Direction = dir
		if e.Protocol != "" {
			out.State = e.Protocol
		}
	}
	out.Err = sessionErrorFromEvents(out.Events)
	s.events = nil
	return out
}

// Complete DNS structure is strong content evidence on a nonstandard port.
// This deliberately does not use the permissive header hint needed to report
// malformed DNS on a caller-selected/default DNS endpoint.
func dnsDatagramEvidence(raw []byte) bool {
	if !dnsHeader(raw) || raw[3]&0x40 != 0 {
		return false
	}
	count := 0
	for at := 4; at < 12; at += 2 {
		count += int(binary.BigEndian.Uint16(raw[at : at+2]))
	}
	if count == 0 {
		return false
	}
	_, err := DecodeDNSMessage(raw, 4096)
	return err == nil
}

func probeDatagram(raw []byte, maxBytes int) ProbeResult {
	if len(raw) > maxBytes {
		return ProbeResult{Verdict: ProbeReject, Reason: "datagram exceeds configured byte limit"}
	}
	if p := probeDLMS(raw, maxBytes); p.Verdict == ProbeAccept {
		return p
	}
	if slmpRequestEvidence(raw) {
		return probeAccept("slmp", "slmp-binary-self-test", 98)
	}
	if bsapEvidence(raw) {
		if _, err := decodeBSAPMessage(raw, DefaultParserBudget().MaxCollectionElements); err == nil {
			return probeAccept("bsap", "serial-local-rdb-name-2022", 98)
		}
	}
	if rtpsDatagramHeader(raw) {
		if _, err := decodeRTPSDatagram(raw, DefaultParserBudget().MaxCollectionElements); err == nil {
			return probeAccept("rtps", "spdp-parameter-list", 98)
		}
	}
	if p := probeMAVLink(raw, maxBytes); p.Verdict == ProbeAccept {
		if _, err := decodeMAVLinkDatagram(raw, maxBytes, 4096); err == nil {
			return p
		}
	}
	if len(raw) > 12 && raw[3] == 0 && validSemtechDatagram(raw) {
		return probeAccept("semtech-udp", "v2-push-data", 98)
	}
	if validNMEADatagram(raw) {
		return probeAccept("nmea", "gga-rmc", 98)
	}
	if dnsDatagramEvidence(raw) {
		return probeAccept("dns", "rfc1035", 98)
	}
	if _, ok := inspectBACnetIP(raw); ok {
		return probeAccept("bacnet", "bvlc-npdu", 98)
	}
	if len(raw) >= 13 && (raw[0] >= 20 && raw[0] <= 24) && raw[1] == 0xfe && (raw[2] == 0xfd || raw[2] == 0xff) {
		if _, err := dtlsRecords(raw, 4096); err == nil {
			return probeAccept("dtls", "record", 98)
		}
	}
	if p := probeSNMP(raw, len(raw)); p.Verdict == ProbeAccept {
		if _, err := (&binSNMP{}).consume(raw, 4096, 0); err == nil {
			return p
		}
	}
	if radiusDatagramEvidence(raw, "", "") {
		return probeAccept("radius", "rfc2865", 98)
	}
	for _, probe := range []func([]byte, int) ProbeResult{probeSTUN, probeDHCP, probeNTP, probeCoAP, probeSIP} {
		if p := probe(raw, len(raw)); p.Verdict == ProbeAccept {
			return p
		}
	}
	return ProbeResult{Verdict: ProbeReject, Reason: "no complete datagram profile"}
}

func kerberosWireEvidence(raw []byte, maxBytes int) bool {
	if len(raw) < 2 || !kerberosTag(raw[0]) || len(raw) > maxBytes {
		return false
	}
	n, h, err := berLength(raw[1:])
	if err != nil || h == 0 || n+1+h != len(raw) {
		return false
	}
	body := raw[1+h:]
	if len(body) < 2 || body[0] != 0x30 {
		return false
	}
	n, h, err = berLength(body[1:])
	if err != nil || h == 0 || n+1+h != len(body) {
		return false
	}
	fields := body[1+h:]
	first, second := byte(0xa1), byte(0xa2)
	if raw[0] == 0x7e {
		first, second = 0xa0, 0xa1
	}
	// RFC 4120 pvno=5 and msg-type matching the APPLICATION tag.
	for _, want := range []struct{ tag, value byte }{{first, 5}, {second, raw[0] & 31}} {
		if len(fields) < 5 || fields[0] != want.tag || fields[1] != 3 || fields[2] != 2 || fields[3] != 1 || fields[4] != want.value {
			return false
		}
		fields = fields[5:]
	}
	used := 0
	return validateLDAPBER(body, 0, DefaultParserBudget(), &used) == nil
}

func cassandraRequestEvidence(w []byte, maxBytes int) bool {
	if !cassandraInitialExchangeHeader(w, maxBytes) || w[0]&0x80 != 0 || w[1] != 0 {
		return false
	}
	n := 9 + int(binary.BigEndian.Uint32(w[5:9]))
	if len(w) < n {
		return false
	}
	if w[4] == 5 {
		return n == 9
	}
	if w[4] != 1 {
		return false
	}
	b := w[9:n]
	if len(b) < 2 {
		return false
	}
	count := int(binary.BigEndian.Uint16(b))
	b = b[2:]
	if count < 1 || count > 32 {
		return false
	}
	version := false
	for i := 0; i < count; i++ {
		var values [2]string
		for j := 0; j < 2; j++ {
			if len(b) < 2 {
				return false
			}
			l := int(binary.BigEndian.Uint16(b))
			b = b[2:]
			if l > len(b) {
				return false
			}
			values[j] = string(b[:l])
			b = b[l:]
		}
		if values[0] == "CQL_VERSION" {
			if version || values[1] != "3.0.0" {
				return false
			}
			version = true
		}
	}
	return len(b) == 0 && version
}

func initialProtocolNeedMore(w []byte, maxBytes int) ProbeResult {
	if p := probeDLMS(w, maxBytes); p.Verdict == ProbeNeedMore {
		return p
	}
	if p := probeGenisys(w, maxBytes); p.Verdict == ProbeNeedMore {
		return p
	}
	if p := probeROCPlus(w, maxBytes); p.Verdict == ProbeNeedMore {
		return p
	}
	if len(w) >= 9 && cassandraInitialExchangeHeader(w, maxBytes) && w[0]&0x80 == 0 && w[1] == 0 {
		n := 9 + int(binary.BigEndian.Uint32(w[5:9]))
		if len(w) < n {
			return probeNeed("cassandra", "initial", len(w), n)
		}
	}
	if len(w) >= 6 && kerberosTag(w[4]) {
		n := int(binary.BigEndian.Uint32(w[:4])) + 4
		if n >= 6 && n <= maxBytes && len(w) < n {
			// Preserve a bounded candidate until its complete BER body can establish
			// content evidence; this is a hold, never positive protocol admission.
			return probeNeed("kerberos", "rfc4120", len(w), n)
		}
	}
	return ProbeResult{Verdict: ProbeReject}
}

// WithSessionClientDirection supplies a caller-observed TCP initiator role.
// It is separate from the first captured endpoint, which can be midstream.
func WithSessionClientDirection(dir int) ProtocolSessionOption {
	return func(s *captureSession) error {
		if dir != 0 && dir != 1 {
			return fmt.Errorf("TCP client direction must be 0 or 1")
		}
		s.f.tcpClientKnown, s.f.tcpClientDir = true, dir
		return nil
	}
}
func (f *binFlow) needsMoreSSHServerPreamble(dir int, wire []byte) bool {
	return f.tcpClientKnown && dir != f.tcpClientDir && len(wire) < min(f.a.budget.MaxFrameBytes, sshMaxPreBytes+255) && probeSSHServerPreamble(wire, f.a.budget.MaxFrameBytes).Verdict == ProbeNeedMore
}

func radiusDatagramEvidence(w []byte, src, dst string) bool {
	if probeRADIUS(w, len(w)).Verdict != ProbeAccept || int(binary.BigEndian.Uint16(w[2:4])) != len(w) {
		return false
	}
	// A known carrier still reports malformed attributes; arbitrary ports need
	// complete attribute spans rather than a four-byte length coincidence.
	_, sp, _ := net.SplitHostPort(src)
	_, dp, _ := net.SplitHostPort(dst)
	if sp == "1812" || sp == "1813" || dp == "1812" || dp == "1813" {
		return true
	}
	for at := 20; at < len(w); {
		if at+2 > len(w) || w[at+1] < 2 || at+int(w[at+1]) > len(w) {
			return false
		}
		at += int(w[at+1])
	}
	return true
}
