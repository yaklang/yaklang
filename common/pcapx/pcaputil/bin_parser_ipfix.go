package pcaputil

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

type ipfixTCPDomain struct {
	templates map[uint16]ipfixLease
	expected  uint32
	known     bool
}
type binIPFIX struct{ domains [2]map[uint32]ipfixTCPDomain }

func probeIPFIX(w []byte, limit int) ProbeResult {
	if len(w) > 0 && w[0] != 0 {
		return ProbeResult{Verdict: ProbeReject}
	}
	n, err := stream_parser.IPFIXWireLength(w)
	if err != nil || n > limit {
		return ProbeResult{Verdict: ProbeReject}
	}
	if n == 0 {
		return probeNeed("ipfix", "10", len(w), 4)
	}
	if len(w) < n {
		return probeNeed("ipfix", "10", len(w), n)
	}
	x, err := stream_parser.DecodeIPFIXWire(w[:n], nil, "tcp", 4096)
	if (err == nil || x.Templates != nil) && x.Fields["Known Set Namespace"] == true && len(netflowV9Rows(x.Fields["FlowSets"])) > 0 {
		return probeAccept("ipfix", "10", 98)
	}
	return ProbeResult{Verdict: ProbeReject}
}
func (f *binFlow) frameIPFIX(w []byte) (int, *binSpec, error) {
	if f.ipfixTCP == nil {
		if err := f.reserveSession(8192); err != nil {
			return 0, nil, err
		}
		f.ipfixTCP = &binIPFIX{}
	}
	n, err := stream_parser.IPFIXWireLength(w)
	return n, f.a.specs["ipfix/IPFIXFields"], observedWireError(err)
}
func (f *binFlow) consumeIPFIX(dir int, e *ProtocolEvent) error {
	if f.ipfixTCP == nil {
		f.ipfixTCP = &binIPFIX{}
	}
	e.nativeSessionMessage, e.nativeNetFlowObservation = true, true
	e.Profile, e.Admission, e.Completeness = "ipfix-observed-TCP-template-receiver", "complete-service-hinted-IPFIX-message", "message"
	if len(e.Raw) < 16 {
		return protocolError(ErrMalformedMessage, "IPFIX complete header required")
	}
	id, seq := binary.BigEndian.Uint32(e.Raw[12:]), binary.BigEndian.Uint32(e.Raw[8:])
	s := f.ipfixTCP
	previous, exists := s.domains[dir][id]
	leases := previous.templates
	reset := ""
	if exists && (!previous.known || seq != previous.expected) {
		leases = map[uint16]ipfixLease{}
		for id, t := range previous.templates {
			if t.definition.Quarantined {
				leases[id] = t
			}
		}
		reset = "reported data-record sequence discontinuity; prior schemas quarantined"
	}
	templates := map[uint16]stream_parser.IPFIXTemplate{}
	for id, t := range leases {
		templates[id] = t.definition
	}
	result, err := stream_parser.DecodeIPFIXWire(e.Raw, templates, "tcp", f.a.budget.MaxCollectionElements)
	m := result.Fields
	if m == nil {
		m = map[string]any{}
	}
	m["Error Scope"] = "message"
	m["Template Scope"] = "captured TCP generation / direction / Observation Domain; ordered withdrawals; no UDP cache or lease"
	if reset != "" {
		m["Context Reset"] = reset
	}
	digest := sha256.Sum256(e.Raw)
	hash := hex.EncodeToString(digest[:])
	for _, row := range netflowV9Rows(m["FlowSets"]) {
		if value, ok := row["Template PDU SHA256"].(string); ok {
			refs := e.SourceBytes.PacketRefs
			if value != hash {
				refs = leases[uint16(routingNumber(row, "FlowSet ID"))].refs
			}
			row["Template Packet References"] = netflowV9LeaseRefs(refs)
		}
	}
	if result.Templates == nil {
		// A bad complete message can hide template management; retain only blocked
		// identifiers so another unwithdrawn definition cannot clear the ambiguity.
		blocked := map[uint16]ipfixLease{}
		for id, t := range previous.templates {
			t.definition.Quarantined = true
			blocked[id] = t
		}
		if len(blocked) > 0 {
			s.domains[dir][id] = ipfixTCPDomain{templates: blocked}
		} else {
			delete(s.domains[dir], id)
		}
	} else {
		next := map[uint16]ipfixLease{}
		for id, t := range result.Templates {
			lease := leases[id]
			if t.PDUSHA256 == hash {
				lease = ipfixLease{definition: t, refs: append([]PacketReference(nil), e.SourceBytes.PacketRefs...)}
			}
			next[id] = lease
		}
		cost := int64(8192) + ipfixLeaseCost(next)
		domains := 1
		for d, entries := range s.domains {
			for k, v := range entries {
				if d != dir || k != id {
					cost += 512 + ipfixLeaseCost(v.templates)
					domains++
				}
			}
		}
		if domains > min(sessionCollectionLimit(f.a.budget.MaxCollectionElements), 128) {
			err = protocolError(ErrResourceExceeded, "IPFIX TCP Observation Domain budget exceeded")
			delete(s.domains[dir], id)
		} else if reserveErr := f.reserveSession(cost); reserveErr != nil {
			err = reserveErr
			delete(s.domains[dir], id)
		} else {
			if s.domains[dir] == nil {
				s.domains[dir] = map[uint32]ipfixTCPDomain{}
			}
			s.domains[dir][id] = ipfixTCPDomain{next, seq + uint32(routingNumber(m, "Data Record Count")), m["Record Count Verified"] == true}
		}
	}
	e.Session, e.semanticFields, e.semanticSession = m, cloneSession(m), cloneSession(m)
	e.Summary = fmt.Sprintf("IPFIX · domain=%v seq=%v · %v template(s), %v data record(s)", m["Observation Domain ID"], m["Sequence Number"], m["Template Record Count"], m["Data Record Count"])
	if err != nil {
		e.Summary = err.Error() + " · " + e.Summary
	}
	if err != nil || m["Unselected Field Semantics"] == true {
		e.Completeness = "limited"
	}
	return observedWireError(err)
}
