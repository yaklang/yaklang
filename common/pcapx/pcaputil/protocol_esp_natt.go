package pcaputil

import (
	"encoding/binary"
	"fmt"

	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

// RFC3948 multiplexes ESP, marked IKE and one-byte keepalives on the same UDP
// endpoints. This receiver runs only after UDP carrier validation and the QUIC
// and RDPUDP receivers. It never installs a tuple/SPI association: each datagram
// must provide its own complete clear header and nonzero SPI. The opaque bytes
// cannot establish an SA, algorithm, authenticated identity or replay result.
func (a *binParser) decodeESPNATTDatagram(e *ProtocolEvent, wire []byte, src, dst uint16, explicit string) bool {
	selected := explicit == "esp-natt"
	if src == 0 || dst == 0 || !selected && (explicit != "" || src != 4500 && dst != 4500) {
		return false
	}
	// Match nat_t.yaml's selected ESP envelope, including one opaque word.
	// Zero SPI belongs to the Non-ESP Marker grammar; short prefixes and the
	// single FF keepalive do not acquire an ESP identity, even with DecodeAs.
	if len(wire) < 12 || len(wire) > 65527 || binary.BigEndian.Uint32(wire[:4]) == 0 {
		return false
	}
	e.Protocol, e.Rule, e.Entry = "esp", "ipsec_esp", "ESPHeaderFields"
	e.Profile, e.Admission = "esp-natt-observed-header", "observed-UDP4500-nonzero-SPI"
	if selected {
		e.Admission = "explicit-decode-as"
	}
	e.Status, e.Completeness, e.nativeESPObservation = "limited", "headers", true
	a.messages.Add(1)
	a.messageBytes.Add(uint64(len(wire)))
	limit := min(a.config.MaxMessageBytes, a.budget.MaxFrameBytes)
	if len(wire) > limit {
		e.Status, e.Completeness = "limited", "limited"
		_, e.sessionError = classifySessionError(protocolError(ErrResourceExceeded, "ESP UDP carrier/header observation exceeds parser limit"))
		e.ExpertCode, e.Error = string(ErrResourceExceeded), e.sessionError.Error()
		e.Summary = "ESP · UDP encapsulation · observation omitted by resource limit"
		a.limited.Add(uint64(len(wire)))
		return true
	}
	fields, err := stream_parser.DecodeESPHeaderWire(wire, limit)
	if err != nil {
		// The admitted envelope supplies the complete fixed header. Keep a
		// typed refusal if its canonical projection ever rejects it.
		e.Status, e.sessionError = classifySessionError(observedWireError(err))
		e.Completeness = e.Status
		e.ExpertCode, e.Error = string(e.sessionError.Kind), e.sessionError.Error()
		e.Summary = "ESP · UDP encapsulation · clear-header projection refused"
		a.malformed.Add(1)
		return true
	}
	e.Raw = append([]byte(nil), wire...)
	e.Session, e.semanticFields, e.semanticSession = cloneSession(fields), cloneSession(fields), cloneSession(fields)
	e.Summary = fmt.Sprintf("ESP · UDP encapsulation · SPI=0x%08x · seq=%d · opaque=%d bytes; SA context unavailable", binary.BigEndian.Uint32(wire[:4]), binary.BigEndian.Uint32(wire[4:8]), len(wire)-8)
	a.limited.Add(uint64(len(wire)))
	return true
}
