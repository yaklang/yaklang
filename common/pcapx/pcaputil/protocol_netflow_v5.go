package pcaputil

import (
	"encoding/binary"
	"fmt"

	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

func (a *binParser) decodeNetFlowV5Datagram(e *ProtocolEvent, w []byte, src, dst uint16, explicit string) bool {
	selected := explicit == "netflow-v5"
	ports := []uint16{2055, 2056, 9995, 9996}
	if !selected && (explicit != "" || !controlPort(ports, src) && !controlPort(ports, dst) || len(w) < 24 || binary.BigEndian.Uint16(w) != 5) {
		return false
	}
	fields, err := stream_parser.DecodeNetFlowV5Wire(w, a.budget.MaxCollectionElements)
	err = observedWireError(err)
	if err != nil && !selected && (fields == nil || fields["Wire Grammar Complete"] != true) {
		return false
	}
	e.Protocol, e.Rule, e.Entry = "netflow-v5", "netflow_v5", "NetFlowV5Fields"
	e.Profile, e.Admission, e.Completeness = "netflow-v5-independent-UDP-export", "complete-fixed-record-grammar-and-service-hint", "message"
	if selected {
		e.Admission = "explicit-decode-as"
	}
	e.FlowID, e.ResponseTo, e.TransactionID, e.Direction = 0, 0, 0, 0
	e.nativeNetFlowObservation, e.nativeSessionMessage = true, true
	if fields != nil {
		e.Session, e.semanticFields, e.semanticSession = fields, cloneSession(fields), cloneSession(fields)
	}
	a.finishProtocolDatagram(e, w, nil, err)
	if err != nil {
		e.Completeness = "limited"
	} else {
		e.Summary = fmt.Sprintf("NetFlow v5 · engine=%v/%v seq=%v · %v reported record(s)", fields["Engine Type"], fields["Engine ID"], fields["Sequence Number"], fields["Data Record Count"])
	}
	return true
}
