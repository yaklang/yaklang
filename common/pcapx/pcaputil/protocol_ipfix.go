package pcaputil

import (
	"container/list"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

const ipfixLeaseLifetime = 5 * time.Minute // explicit local passive receiver policy
type ipfixLease struct {
	definition stream_parser.IPFIXTemplate
	received   time.Time
	refs       []PacketReference
}
type ipfixExporter struct {
	key                        netflowV9Key // directed physical endpoints, capture domain, Observation Domain
	charge                     *binFlow
	templates, before          map[uint16]ipfixLease
	sequence, expected, export uint32
	expectedKnown              bool
	hash                       [32]byte
	time                       time.Time
}
type ipfixStore struct {
	entries map[netflowV9Key]*list.Element
	lru     list.List
}

func (a *binParser) dropIPFIXLocked(el *list.Element) {
	if el == nil || a.ipfix == nil {
		return
	}
	s := el.Value.(*ipfixExporter)
	a.buffered.Add(-s.charge.sessionBytes)
	s.charge.sessionBytes = 0
	delete(a.ipfix.entries, s.key)
	a.ipfix.lru.Remove(el)
}
func (a *binParser) closeIPFIX() {
	a.ipfixMu.Lock()
	defer a.ipfixMu.Unlock()
	if a.ipfix != nil {
		for el := a.ipfix.lru.Front(); el != nil; {
			next := el.Next()
			a.dropIPFIXLocked(el)
			el = next
		}
		a.ipfix = nil
	}
}
func (a *binParser) invalidateIPFIXTuple(e *ProtocolEvent) bool {
	matched := false
	if a.ipfix != nil {
		for el := a.ipfix.lru.Front(); el != nil; {
			next := el.Next()
			key := el.Value.(*ipfixExporter).key
			if key.domain == e.Domain && key.source == e.Source && key.destination == e.Destination {
				matched = true
				a.dropIPFIXLocked(el)
			}
			el = next
		}
	}
	return matched
}

// Constant-space pre-dispatch guard: a version prefix must not hide tuple reuse.
func ipfixCompleteSetFraming(w []byte) bool {
	if len(w) < 16 || len(w) > 65535 || binary.BigEndian.Uint16(w) != 10 || int(binary.BigEndian.Uint16(w[2:])) != len(w) {
		return false
	}
	for at := 16; at < len(w); {
		if len(w)-at < 4 {
			return false
		}
		n := int(binary.BigEndian.Uint16(w[at+2:]))
		if n < 4 || n > len(w)-at {
			return false
		}
		at += n
	}
	return true
}
func (a *binParser) observeIPFIXBoundary(e *ProtocolEvent, w []byte, badCarrier bool) {
	if !badCarrier && ipfixCompleteSetFraming(w) {
		return
	}
	a.ipfixMu.Lock()
	defer a.ipfixMu.Unlock()
	a.invalidateIPFIXTuple(e)
}
func ipfixLeaseCost(m map[uint16]ipfixLease) int64 {
	var n int64
	for _, t := range m {
		n += 512 + int64(len(t.definition.Fields))*80 + int64(len(t.refs))*128
	}
	return n
}
func freshIPFIXLeases(in map[uint16]ipfixLease, now time.Time) map[uint16]ipfixLease {
	out := map[uint16]ipfixLease{}
	if now.IsZero() {
		return out
	}
	for id, t := range in {
		if !t.received.IsZero() && !now.Before(t.received) && now.Sub(t.received) < ipfixLeaseLifetime {
			out[id] = t
		}
	}
	return out
}
func (a *binParser) refuseIPFIXOversize(e *ProtocolEvent, w []byte, src, dst uint16) bool {
	explicit := a.datagramDecodeAs[dst]
	if explicit == "" {
		explicit = a.datagramDecodeAs[src]
	}
	a.ipfixMu.Lock()
	matched := a.invalidateIPFIXTuple(e)
	a.ipfixMu.Unlock()
	if explicit != "ipfix" && !matched {
		return false
	}
	e.Length = len(w)
	admission := "observed-directed-exporter-context"
	if explicit == "ipfix" {
		admission = "explicit-decode-as"
	}
	a.finishIPFIXDatagram(e, nil, nil, protocolError(ErrResourceExceeded, "IPFIX UDP byte budget exceeded; templates invalidated"), admission)
	a.messageBytes.Add(uint64(len(w)))
	a.limited.Add(uint64(len(w)))
	return true
}
func (a *binParser) decodeIPFIXDatagram(e *ProtocolEvent, w []byte, src, dst uint16, explicit string) bool {
	selected := explicit == "ipfix"
	if !selected && (explicit != "" || !controlPort([]uint16{4739, 2055, 2056, 9995, 9996}, src) && !controlPort([]uint16{4739, 2055, 2056, 9995, 9996}, dst)) {
		return false
	}
	a.ipfixMu.Lock()
	if len(w) < 16 || binary.BigEndian.Uint16(w) != 10 {
		a.invalidateIPFIXTuple(e)
		a.ipfixMu.Unlock()
		if !selected {
			return false
		}
		result, err := stream_parser.DecodeIPFIXWire(w, nil, "udp", a.budget.MaxCollectionElements)
		a.finishIPFIXDatagram(e, w, result.Fields, observedWireError(err), "explicit-decode-as")
		return true
	}
	key := netflowV9Key{e.Domain, e.Source, e.Destination, binary.BigEndian.Uint32(w[12:])}
	var el *list.Element
	if a.ipfix != nil {
		el = a.ipfix.entries[key]
	}
	seq, export := binary.BigEndian.Uint32(w[8:]), binary.BigEndian.Uint32(w[4:])
	hash := sha256.Sum256(w)
	hashText := hex.EncodeToString(hash[:])
	var state *ipfixExporter
	var leases map[uint16]ipfixLease
	duplicate, reset := false, ""
	if el != nil {
		state = el.Value.(*ipfixExporter)
		duplicate = seq == state.sequence && hash == state.hash
		switch {
		case e.Timestamp.IsZero() || e.Timestamp.Before(state.time):
			reset = "capture clock unavailable/regressed; templates quarantined"
			duplicate = false
		case duplicate:
			leases = freshIPFIXLeases(state.before, e.Timestamp)
		case !state.expectedKnown:
			reset = "prior Data Set record count unknown; sequence continuity unproved; templates quarantined"
		case seq != state.expected:
			reset = "data-record sequence gap/reorder/reuse; templates quarantined"
		case int32(export-state.export) < 0:
			reset = "export time regressed; templates quarantined; restart not authenticated"
		default:
			leases = freshIPFIXLeases(state.templates, e.Timestamp)
		}
	}
	previous := map[uint16]stream_parser.IPFIXTemplate{}
	for id, t := range leases {
		previous[id] = t.definition
	}
	result, wireErr := stream_parser.DecodeIPFIXWire(w, previous, "udp", a.budget.MaxCollectionElements)
	fields := result.Fields
	if fields != nil {
		fields["Template Lease Policy"] = "local300-second capture-time lease; data-record sequence continuity; last observed schema, not authenticated; unobserved template-only loss cannot be excluded"
		if reset != "" {
			fields["Context Reset"] = reset
		}
		if duplicate {
			fields["Duplicate Observation"] = "identical adjacent message; original input schemas; no lease refresh"
		}
		for _, row := range netflowV9Rows(fields["FlowSets"]) {
			if value, ok := row["Template PDU SHA256"].(string); ok {
				refs := e.SourceBytes.PacketRefs
				if value != hashText {
					refs = leases[uint16(routingNumber(row, "FlowSet ID"))].refs
				}
				row["Template Packet References"] = netflowV9LeaseRefs(refs)
			}
		}
	}
	admission := "complete-IPFIX-framing-and-service-hint"
	if selected {
		admission = "explicit-decode-as"
	} else if state != nil {
		admission = "observed-directed-exporter-context"
	}
	admit := selected || state != nil || fields != nil && fields["Wire Framing Complete"] == true && fields["Known Set Namespace"] == true && len(netflowV9Rows(fields["FlowSets"])) > 0 && (wireErr == nil || result.Templates != nil)
	if result.Templates == nil {
		a.dropIPFIXLocked(el)
	} else if !duplicate && !e.Timestamp.IsZero() && admit {
		if a.ipfix == nil {
			a.ipfix = &ipfixStore{entries: map[netflowV9Key]*list.Element{}}
		}
		if state == nil {
			if len(a.ipfix.entries) >= min(sessionCollectionLimit(a.budget.MaxCollectionElements), 128) {
				a.dropIPFIXLocked(a.ipfix.lru.Front())
			}
			state = &ipfixExporter{key: key, charge: &binFlow{a: a}}
		}
		next := map[uint16]ipfixLease{}
		for id, definition := range result.Templates {
			lease := leases[id]
			if definition.PDUSHA256 == hashText {
				lease = ipfixLease{definition, e.Timestamp, append([]PacketReference(nil), e.SourceBytes.PacketRefs...)}
			}
			next[id] = lease
		}
		cost := 8192 + int64(4*(len(key.source)+len(key.destination)+len(key.domain.Encapsulation))) + ipfixLeaseCost(next) + ipfixLeaseCost(leases)
		if err := state.charge.reserveSession(cost); err != nil {
			fields["Context Retention Warning"] = "template cache budget exceeded; no partial schema retained"
			a.dropIPFIXLocked(el)
		} else {
			state.templates, state.before = next, leases
			state.sequence, state.expected, state.export = seq, seq+uint32(routingNumber(fields, "Data Record Count")), export
			state.expectedKnown = fields["Record Count Verified"] == true
			state.hash, state.time = hash, e.Timestamp
			if el == nil {
				el = a.ipfix.lru.PushBack(state)
				a.ipfix.entries[key] = el
			} else {
				a.ipfix.lru.MoveToBack(el)
			}
		}
	} else if result.Templates != nil && e.Timestamp.IsZero() {
		fields["Context Retention Warning"] = "capture clock unavailable; no cross-message schemas retained"
	}
	a.ipfixMu.Unlock()
	if !admit {
		return false
	}
	a.finishIPFIXDatagram(e, w, fields, observedWireError(wireErr), admission)
	return true
}
func (a *binParser) finishIPFIXDatagram(e *ProtocolEvent, w []byte, fields map[string]any, err error, admission string) {
	e.Protocol, e.Rule, e.Entry = "ipfix", "ipfix", "IPFIXFields"
	e.FlowID, e.ResponseTo, e.TransactionID, e.Direction = 0, 0, 0, 0
	e.Profile, e.Admission, e.Completeness = "ipfix-directed-UDP-template-observer", admission, "message"
	e.nativeNetFlowObservation, e.nativeSessionMessage = true, true
	if fields != nil {
		e.Session, e.semanticFields, e.semanticSession = fields, cloneSession(fields), cloneSession(fields)
	}
	a.finishProtocolDatagram(e, w, nil, err)
	if err != nil || fields["Unselected Field Semantics"] == true || fields["Context Retention Warning"] != nil {
		e.Completeness = "limited"
	}
	if fields != nil {
		e.Summary = fmt.Sprintf("IPFIX · domain=%v seq=%v · %v template(s), %v data record(s)", fields["Observation Domain ID"], fields["Sequence Number"], fields["Template Record Count"], fields["Data Record Count"])
		if err != nil {
			e.Summary = err.Error() + " · " + e.Summary
		}
	}
}
