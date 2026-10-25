package pcaputil

import (
	"fmt"
	"net/netip"

	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

func (a *binParser) decodeRIPngDatagram(e *ProtocolEvent, w []byte, src, dst uint16, explicit string) bool {
	selected := explicit == "ripng"
	if !selected && (explicit != "" || src != 521 && dst != 521) {
		return false
	}
	source, srcErr := netip.ParseAddrPort(e.Source)
	destination, dstErr := netip.ParseAddrPort(e.Destination)
	ipv6 := srcErr == nil && dstErr == nil && source.Addr().Is6() && destination.Addr().Is6()
	// A four-byte header alone is too weak to automatically identify traffic.
	// An explicit selector can inspect an empty request, but cannot invent IPv6.
	if !selected && (!ipv6 || len(w) < 24) {
		return false
	}
	var fields map[string]any
	var err error
	if !ipv6 {
		err = protocolError(ErrUnsupportedFeature, "RIPng requires an observed IPv6 UDP carrier")
	} else {
		fields, err = stream_parser.DecodeRIPngWire(w, a.budget.MaxCollectionElements)
		err = observedWireError(err)
	}
	if err != nil && !selected {
		return false
	}
	e.Protocol, e.Rule, e.Entry = "ripng", "ripng", "RIPngFields"
	e.Profile, e.Admission, e.Completeness = "ripng-v1-independent-advertisement", "complete-RIPng-grammar-IPv6-and-service-hint", "message"
	e.nativeSessionMessage = true
	if selected {
		e.Admission = "explicit-decode-as"
	}
	if fields != nil {
		e.Session, e.semanticSession, e.semanticFields = fields, cloneSession(fields), cloneSession(fields)
	}
	a.finishProtocolDatagram(e, w, nil, err)
	if err != nil {
		e.Completeness = "limited"
	} else {
		e.Summary = fmt.Sprintf("%s · %v route(s), %v next-hop advisory(s)", fields["Packet Name"], fields["Route Count"], fields["Next Hop Count"])
	}
	return true
}
