package pcaputil

import (
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

type bgpOpenObservation struct {
	seen, as4, extended, refresh bool
	asn                          uint16
	identifier                   string
	hash                         [32]byte
	addPath                      map[uint32]byte
}
type binBGP struct {
	opens           [2]bgpOpenObservation
	blocked, closed bool
}

func probeBGP(w []byte, limit int) ProbeResult {
	if len(w) == 0 || w[0] != 255 {
		return ProbeResult{Verdict: ProbeReject}
	}
	n, err := stream_parser.BGPWireLength(w)
	if err != nil {
		return ProbeResult{Verdict: ProbeReject}
	}
	if n == 0 {
		return probeNeed("bgp", "4", len(w), 19)
	}
	if n > limit {
		return ProbeResult{Verdict: ProbeReject}
	}
	if len(w) < n {
		return probeNeed("bgp", "4", len(w), n)
	}
	if _, err := stream_parser.DecodeBGPMessage(w[:n], stream_parser.BGPWireContext{}, 4096); err != nil {
		return ProbeResult{Verdict: ProbeReject}
	}
	return probeAccept("bgp", "4", 99)
}
func (s *binBGP) storage() int64 {
	return int64(1024 + (len(s.opens[0].addPath)+len(s.opens[1].addPath))*64)
}
func (f *binFlow) frameBGP(w []byte) (int, *binSpec, error) {
	if err := f.reserveSession(f.bgp.storage()); err != nil {
		return 0, nil, err
	}
	n, err := stream_parser.BGPWireLength(w)
	return n, f.a.specs["bgp/BGPMessageFields"], err
}
func observedWireError(err error) error {
	var typed *ProtocolError
	if errors.As(err, &typed) {
		return err
	}
	if err == nil {
		return nil
	}
	if errors.Is(err, stream_parser.ErrRSVPWireIntegrity) || errors.Is(err, stream_parser.ErrRSVPWireFormat) {
		return protocolError(ErrMalformedMessage, "%s", err)
	}
	if errors.Is(err, stream_parser.ErrOSPFv3WireIntegrity) || errors.Is(err, stream_parser.ErrOSPFv2WireIntegrity) || errors.Is(err, stream_parser.ErrOSPFv2WireFormat) || errors.Is(err, stream_parser.ErrGTPv2WireFormat) {
		return protocolError(ErrMalformedMessage, "%s", err)
	}
	if errors.Is(err, stream_parser.ErrObservedWireBudget) {
		return protocolError(ErrResourceExceeded, "%s", err)
	}
	if errors.Is(err, stream_parser.ErrObservedWireContext) {
		return protocolError(ErrContextRequired, "%s", err)
	}
	return protocolError(ErrMalformedMessage, "%s", err)
}
func (s *binBGP) context(dir int) stream_parser.BGPWireContext {
	c := stream_parser.BGPWireContext{}
	if !s.blocked && !s.closed {
		if s.opens[0].seen && !s.opens[0].as4 || s.opens[1].seen && !s.opens[1].as4 {
			c.ASWidth = 2
		} else if s.opens[0].seen && s.opens[1].seen {
			c.ASWidth = 4
		}
	}
	c.AddPath = func(afi uint16, safi byte) int {
		if s.blocked || s.closed {
			return -1
		}
		key := uint32(afi)<<8 | uint32(safi)
		send, receive := s.opens[dir], s.opens[1-dir]
		if send.seen && send.addPath[key]&2 == 0 || receive.seen && receive.addPath[key]&1 == 0 {
			return 0
		}
		if send.seen && receive.seen {
			return 1
		}
		return -1
	}
	return c
}
func (f *binFlow) consumeBGP(dir int, e *ProtocolEvent) error {
	s := f.bgp
	ctx := s.context(dir)
	fields, err := stream_parser.DecodeBGPMessage(e.Raw, ctx, f.a.budget.MaxCollectionElements)
	e.nativeSessionMessage = true
	e.Profile, e.Admission, e.Completeness = "bgp4-observed", "complete-marker-length-and-message", "message"
	e.Session = fields
	if fields == nil {
		return observedWireError(err)
	}
	fields["Error Scope"] = "message"
	if err == nil && len(e.Raw) > 4096 {
		peer := s.opens[1-dir]
		if s.blocked || s.closed || !peer.seen {
			err = protocolError(ErrContextRequired, "BGP extended message requires observed receiving-peer capability")
		} else if !peer.extended {
			err = protocolError(ErrMalformedMessage, "BGP extended message was not permitted by receiving peer")
		}
	}
	if err == nil && e.Raw[18] == 1 {
		if s.closed || s.opens[dir].seen {
			s.blocked = true
			err = protocolError(ErrContextRequired, "BGP repeated OPEN inside the same captured TCP generation; negotiation quarantined")
		} else {
			o := bgpOpenObservation{seen: true, asn: fields["My AS"].(uint16), identifier: fields["BGP Identifier"].(string), hash: sha256.Sum256(e.Raw), addPath: map[uint32]byte{}}
			for _, cap := range fields["Capabilities"].([]map[string]any) {
				switch cap["Code"].(byte) {
				case 65:
					o.as4 = true
				case 6:
					o.extended = true
				case 2:
					o.refresh = true
				case 69:
					for _, v := range cap["Families"].([]map[string]any) {
						key := uint32(v["AFI"].(uint16))<<8 | uint32(v["SAFI"].(byte))
						mode := v["Mode"].(byte)
						if previous, ok := o.addPath[key]; ok && previous != mode {
							err = protocolError(ErrMalformedMessage, "BGP conflicting ADD-PATH capability modes")
							break
						}
						o.addPath[key] = mode
					}
				}
			}
			if err == nil {
				if err = f.reserveSession(s.storage() + int64(len(o.addPath))*64 + 512); err == nil {
					s.opens[dir] = o
				}
			}
		}
	}
	if err == nil && e.Raw[18] == 3 {
		s.closed = true
	}
	state := "midstream; OPEN exchange incomplete"
	if s.opens[0].seen && s.opens[1].seen {
		state = "both OPEN messages observed"
	}
	if s.closed {
		state = "NOTIFICATION observed; protocol session closed"
	}
	if s.blocked {
		state = "negotiation quarantined"
	}
	fields["Connection State"], fields["AS Width Context"] = state, s.context(dir).ASWidth
	if s.opens[0].seen || s.opens[1].seen {
		var peers []map[string]any
		for i, o := range s.opens {
			if o.seen {
				peers = append(peers, map[string]any{"Direction": i, "My AS": o.asn, "BGP Identifier": o.identifier, "Four Octet AS Capability": o.as4, "Extended Message Receive Capability": o.extended})
			}
		}
		fields["Observed Peers"] = peers
	}
	if err != nil && e.Raw[18] == 1 {
		s.blocked = true
	}
	e.semanticFields = cloneSession(fields)
	e.semanticSession = cloneSession(fields)
	e.Summary = fmt.Sprintf("BGP %v", fields["Packet Name"])
	return observedWireError(err)
}
