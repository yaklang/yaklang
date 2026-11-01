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

// This is a local passive-receiver policy, not an advertised exporter timeout.
// Capture time is required; processing delay or another exporter's clock never
// extends a lease. A sequence gap may have hidden a schema replacement.
const netflowV9LeaseLifetime = 5 * time.Minute

type netflowV9Key struct {
	domain              CaptureDomain
	source, destination string
	sourceID            uint32
}

type netflowV9Lease struct {
	definition stream_parser.NetFlowV9Template
	received   time.Time
	refs       []PacketReference
}

type netflowV9Exporter struct {
	key               netflowV9Key
	charge            *binFlow
	templates, before map[uint16]netflowV9Lease
	sequence, uptime  uint32
	hash              [32]byte
	time              time.Time
}

type netflowV9Store struct {
	entries map[netflowV9Key]*list.Element
	lru     list.List
}

// Run before any UDP dissector can claim a message. An unrelated successful
// codec or a bad carrier must not hide a tuple-reuse boundary from this cache.
func (a *binParser) observeNetFlowV9Boundary(e *ProtocolEvent, w []byte, badCarrier bool) {
	if !badCarrier && len(w) >= 20 && binary.BigEndian.Uint16(w) == 9 {
		return
	}
	a.netflowV9Mu.Lock()
	defer a.netflowV9Mu.Unlock()
	if a.netflowV9 == nil {
		return
	}
	for el := a.netflowV9.lru.Front(); el != nil; {
		next := el.Next()
		key := el.Value.(*netflowV9Exporter).key
		if key.domain == e.Domain && key.source == e.Source && key.destination == e.Destination {
			a.dropNetFlowV9Locked(el)
		}
		el = next
	}
}

func (a *binParser) dropNetFlowV9Locked(el *list.Element) {
	if el == nil || a.netflowV9 == nil {
		return
	}
	v := el.Value.(*netflowV9Exporter)
	// charge is accounting only: no captured flow or protocol session was
	// allocated, so generic FTP/TLS/session teardown must not be invoked.
	a.buffered.Add(-v.charge.sessionBytes)
	v.charge.sessionBytes = 0
	delete(a.netflowV9.entries, v.key)
	a.netflowV9.lru.Remove(el)
}

func (a *binParser) closeNetFlowV9Locked() {
	if a.netflowV9 != nil {
		for el := a.netflowV9.lru.Front(); el != nil; {
			next := el.Next()
			a.dropNetFlowV9Locked(el)
			el = next
		}
		a.netflowV9 = nil
	}
}

func (a *binParser) closeNetFlowV9() {
	a.netflowV9Mu.Lock()
	defer a.netflowV9Mu.Unlock()
	a.closeNetFlowV9Locked()
}

func (a *binParser) refuseNetFlowV9Oversize(e *ProtocolEvent, w []byte, src, dst uint16) bool {
	explicit := a.datagramDecodeAs[dst]
	if explicit == "" {
		explicit = a.datagramDecodeAs[src]
	}
	selected := explicit == "netflow-v9"
	matched := false
	a.netflowV9Mu.Lock()
	if a.netflowV9 != nil {
		for el := a.netflowV9.lru.Front(); el != nil; {
			next := el.Next()
			key := el.Value.(*netflowV9Exporter).key
			if key.domain == e.Domain && key.source == e.Source && key.destination == e.Destination {
				matched = matched || len(w) >= 20 && binary.BigEndian.Uint16(w) == 9 && binary.BigEndian.Uint32(w[16:]) == key.sourceID
				a.dropNetFlowV9Locked(el)
			}
			el = next
		}
	}
	a.netflowV9Mu.Unlock()
	if !selected && !matched {
		return false
	}
	e.Length = len(w)
	admission := "observed-directed-exporter-context"
	if selected {
		admission = "explicit-decode-as"
	}
	a.finishNetFlowV9Datagram(e, nil, nil, protocolError(ErrResourceExceeded, "NetFlow v9 UDP byte budget exceeded; exporter templates invalidated"), admission)
	a.messageBytes.Add(uint64(len(w)))
	a.limited.Add(uint64(len(w)))
	return true
}

func freshNetFlowV9Leases(in map[uint16]netflowV9Lease, now time.Time) map[uint16]netflowV9Lease {
	out := map[uint16]netflowV9Lease{}
	if now.IsZero() {
		return out
	}
	for id, lease := range in {
		if !lease.received.IsZero() && !now.Before(lease.received) && now.Sub(lease.received) < netflowV9LeaseLifetime {
			out[id] = lease
		}
	}
	return out
}

func netflowV9LeaseCost(m map[uint16]netflowV9Lease) int64 {
	cost := int64(0)
	for _, lease := range m {
		cost += 512 + int64(len(lease.definition.Fields))*64 + int64(len(lease.refs))*128
	}
	return cost
}

func netflowV9LeaseRefs(refs []PacketReference) []map[string]any {
	out := make([]map[string]any, 0, len(refs))
	for _, ref := range refs {
		out = append(out, map[string]any{"Packet": ref.Number, "Section": ref.Domain.Section,
			"Interface": ref.Domain.Interface, "Encapsulation": ref.Domain.Encapsulation})
	}
	return out
}

func (a *binParser) decodeNetFlowV9Datagram(e *ProtocolEvent, w []byte, src, dst uint16, explicit string) bool {
	selected := explicit == "netflow-v9"
	if !selected && (explicit != "" || !controlPort([]uint16{2055, 2056, 9995, 9996}, src) && !controlPort([]uint16{2055, 2056, 9995, 9996}, dst)) {
		return false
	}
	a.netflowV9Mu.Lock()
	// Callbacks and field projection happen after releasing receiver state.
	if len(w) < 20 || binary.BigEndian.Uint16(w) != 9 {
		if a.netflowV9 != nil {
			for el := a.netflowV9.lru.Front(); el != nil; {
				next := el.Next()
				key := el.Value.(*netflowV9Exporter).key
				if key.domain == e.Domain && key.source == e.Source && key.destination == e.Destination {
					a.dropNetFlowV9Locked(el)
				}
				el = next
			}
		}
		a.netflowV9Mu.Unlock()
		if !selected {
			return false
		}
		result, err := stream_parser.DecodeNetFlowV9Wire(w, nil, a.budget.MaxCollectionElements)
		a.finishNetFlowV9Datagram(e, w, result.Fields, observedWireError(err), "explicit-decode-as")
		return true
	}
	key := netflowV9Key{e.Domain, e.Source, e.Destination, binary.BigEndian.Uint32(w[16:])}
	var el *list.Element
	if a.netflowV9 != nil {
		el = a.netflowV9.entries[key]
	}
	var state *netflowV9Exporter
	var leases map[uint16]netflowV9Lease
	sequence, uptime := binary.BigEndian.Uint32(w[12:]), binary.BigEndian.Uint32(w[4:])
	hash := sha256.Sum256(w)
	duplicate, reset := false, ""
	if el != nil {
		state = el.Value.(*netflowV9Exporter)
		duplicate = sequence == state.sequence && hash == state.hash
		switch {
		case e.Timestamp.IsZero() || e.Timestamp.Before(state.time):
			reset = "capture clock unavailable or regressed; prior templates quarantined"
			duplicate = false
		case duplicate:
			leases = freshNetFlowV9Leases(state.before, e.Timestamp)
		case sequence != state.sequence+1:
			reset = "nonconsecutive/reused export sequence; prior templates quarantined"
		case int32(uptime-state.uptime) < 0:
			reset = "exporter uptime regressed; prior templates quarantined, restart not authenticated"
		default:
			leases = freshNetFlowV9Leases(state.templates, e.Timestamp)
		}
	}
	previous := make(map[uint16]stream_parser.NetFlowV9Template, len(leases))
	for id, lease := range leases {
		previous[id] = lease.definition
	}
	result, wireErr := stream_parser.DecodeNetFlowV9Wire(w, previous, a.budget.MaxCollectionElements)
	fields := result.Fields
	if fields != nil {
		fields["Template Lease Policy"] = "local 300-second capture-time lease; consecutive directed exporter/domain sequence; no exporter restart/authentication proof"
		if reset != "" {
			fields["Context Reset"] = reset
		}
		if duplicate {
			fields["Duplicate Observation"] = "identical last export sequence and bytes; original input schemas, no lease refresh"
		}
		for _, row := range netflowV9Rows(fields["FlowSets"]) {
			if value, ok := row["Template PDU SHA256"].(string); ok {
				refs := e.SourceBytes.PacketRefs
				if value != hex.EncodeToString(hash[:]) {
					refs = leases[uint16(routingNumber(row, "FlowSet ID"))].refs
				}
				row["Template Packet References"] = netflowV9LeaseRefs(refs)
			}
		}
	}
	admission := "complete-NetFlow-v9-framing-and-service-hint"
	if selected {
		admission = "explicit-decode-as"
	} else if state != nil {
		admission = "observed-directed-exporter-context"
	}
	// A new automatic admission needs every literal FlowSet boundary. A partial
	// version/header signature or service port cannot establish the protocol.
	admit := selected || state != nil || fields != nil && fields["Wire Framing Complete"] == true &&
		(routingNumber(fields, "Template Record Count") > 0 || fields["Known FlowSet Namespace"] == true && routingNumber(fields, "Data FlowSet Count") > 0)
	if wireErr != nil {
		a.dropNetFlowV9Locked(el)
	} else if !duplicate && !e.Timestamp.IsZero() {
		if a.netflowV9 == nil {
			a.netflowV9 = &netflowV9Store{entries: map[netflowV9Key]*list.Element{}}
		}
		if state == nil {
			if len(a.netflowV9.entries) >= min(sessionCollectionLimit(a.budget.MaxCollectionElements), 128) {
				a.dropNetFlowV9Locked(a.netflowV9.lru.Front())
			}
			state = &netflowV9Exporter{key: key, charge: &binFlow{a: a}}
		}
		next := make(map[uint16]netflowV9Lease, len(result.Templates))
		for id, definition := range result.Templates {
			lease := leases[id]
			if definition.PDUSHA256 == hex.EncodeToString(hash[:]) {
				lease = netflowV9Lease{definition: definition, received: e.Timestamp, refs: append([]PacketReference(nil), e.SourceBytes.PacketRefs...)}
			}
			next[id] = lease
		}
		cost := 8192 + int64(4*(len(key.source)+len(key.destination)+len(key.domain.Encapsulation))) + netflowV9LeaseCost(next) + netflowV9LeaseCost(leases)
		if err := state.charge.reserveSession(cost); err != nil {
			fields["Context Retention Warning"] = "template cache budget exceeded; no partial schema update retained"
			a.dropNetFlowV9Locked(el)
		} else {
			state.templates, state.before = next, leases
			state.sequence, state.uptime, state.hash, state.time = sequence, uptime, hash, e.Timestamp
			if el == nil {
				el = a.netflowV9.lru.PushBack(state)
				a.netflowV9.entries[key] = el
			} else {
				a.netflowV9.lru.MoveToBack(el)
			}
		}
	} else if wireErr == nil && e.Timestamp.IsZero() {
		fields["Context Retention Warning"] = "capture clock unavailable; no cross-message template lease retained"
	}
	a.netflowV9Mu.Unlock()
	if !admit {
		return false
	}
	a.finishNetFlowV9Datagram(e, w, fields, observedWireError(wireErr), admission)
	return true
}

func netflowV9Rows(v any) []map[string]any {
	rows, _ := v.([]map[string]any)
	return rows
}

func (a *binParser) finishNetFlowV9Datagram(e *ProtocolEvent, w []byte, fields map[string]any, err error, admission string) {
	e.Protocol, e.Rule, e.Entry = "netflow-v9", "netflow_v9", "NetFlowV9Fields"
	e.FlowID, e.ResponseTo, e.TransactionID, e.Direction = 0, 0, 0, 0
	e.Profile, e.Admission, e.Completeness = "netflow-v9-directed-UDP-template-observer", admission, "message"
	e.nativeNetFlowObservation = true
	e.nativeSessionMessage = true // typed snapshots also use the private owned session
	if fields != nil {
		e.Session, e.semanticFields, e.semanticSession = fields, cloneSession(fields), cloneSession(fields)
	}
	a.finishProtocolDatagram(e, w, nil, err)
	if err != nil || fields["Unselected Field Semantics"] == true || fields["Context Retention Warning"] != nil {
		e.Completeness = "limited"
	}
	if fields != nil {
		e.Summary = fmt.Sprintf("exporter domain=%v seq=%v · %v reported record(s)", fields["Source ID"], fields["Sequence Number"], fields["Header Record Count"])
		if fields["Record Count Verified"] == true {
			e.Summary += fmt.Sprintf(" · %v template(s), %v data record(s)", fields["Template Record Count"], fields["Data Record Count"])
		}
		if err != nil {
			e.Summary = err.Error() + " · " + e.Summary
		}
		e.Summary = "NetFlow v9 · " + e.Summary
	}
}
