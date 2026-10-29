package pcaputil

import (
	"strings"

	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

func (a *binParser) decodeGTPv2Datagram(e *ProtocolEvent, wire []byte, src, dst uint16, explicit string) bool {
	selected := explicit == "gtpv2"
	if !selected && (explicit != "" || src != 2123 && dst != 2123 || len(wire) < 8 || wire[0]>>5 != 2) {
		return false
	}
	fields, wireErr := stream_parser.DecodeGTPv2Wire(wire, a.budget.MaxCollectionElements)
	// A port, partial header or earlier TEID cannot establish current identity.
	// Explicit selection retains malformed/partial literal fields for inspection.
	if !selected && (wireErr != nil || fields == nil || fields["Wire Grammar Complete"] != true) {
		return false
	}
	e.Protocol, e.Rule, e.Entry = "gtpv2", "gtpv2_observed", "GTPv2ControlFields"
	e.Profile, e.Admission, e.Completeness = "gtpv2-bounded-UDP-observation", "complete-selected-wire-grammar-and-service-hint", "message"
	if selected {
		e.Admission = "explicit-decode-as"
	}
	e.FlowID, e.TCPFlowID, e.TransactionID, e.ResponseTo, e.Direction = 0, 0, 0, 0, 0
	e.nativeSessionMessage = true
	if fields != nil {
		e.Session, e.semanticFields, e.semanticSession = cloneSession(fields), cloneSession(fields), cloneSession(fields)
	}
	e.retainNativeGTPv2Fields(wire)
	err := observedWireError(wireErr)
	a.finishProtocolDatagram(e, wire, nil, err)
	if err != nil {
		e.Completeness = "limited"
	} else {
		var names []string
		for _, row := range gtpv2Rows(fields["Messages"]) {
			if name, ok := row["Message Name"].(string); ok {
				names = append(names, name)
			}
		}
		e.Summary = "GTPv2-C " + strings.Join(names, " + ") + " · wire observation; bearer/session unverified"
	}
	return true
}
