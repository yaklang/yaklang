package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"time"
)

// Capture-lifetime history is deliberately not expired: UDP has no observed
// generation boundary that can make a reused sequence safe from late fragments.
type ntpControlKey struct {
	requester, server string
	domain            CaptureDomain
	version           byte
	sequence          uint16
}
type ntpControlPart struct {
	offset, length int
	pdu            uint64
	hash           [32]byte
	refs           []PacketReference
}
type ntpControlObservation struct {
	request          uint64
	requestHash      [32]byte
	requestTime      time.Time
	opcode           byte
	association      uint16
	orphan, complete bool
	refused          string
	status           uint16
	errorFlag        bool
	responseSeen     bool
	final            int
	data, seen       []byte
	parts            []ntpControlPart
}
type ntpControlStore struct {
	entries  map[ntpControlKey]*ntpControlObservation
	bytes    int64
	disabled bool
}

func (a *binParser) observeNTPControl(e *ProtocolEvent, w []byte, m map[string]any) {
	a.udpMu.Lock()
	defer a.udpMu.Unlock()
	m["Association"], m["Control Reassembly"], m["Reassembly Complete"] = "unverified-request", "not-assembled", false
	seq, assoc := binary.BigEndian.Uint16(w[2:4]), binary.BigEndian.Uint16(w[6:8])
	op, response := w[1]&31, w[1]&128 != 0
	if seq == 0 || op == 7 || ntpControlOpcode(op) == "Reserved" {
		m["Association"] = "unselected-sequence-or-opcode"
		return
	}
	key := ntpControlKey{e.Source, e.Destination, e.Domain, w[0] >> 3 & 7, seq}
	if response {
		key.requester, key.server = key.server, key.requester
	}
	if a.ntpControl == nil {
		a.ntpControl = &ntpControlStore{entries: map[ntpControlKey]*ntpControlObservation{}}
	}
	s := a.ntpControl
	o := s.entries[key]
	reserve := func(n int64) bool {
		if !a.reserveEvidence(n) {
			return false
		}
		s.bytes += n
		return true
	}
	if o == nil {
		if s.disabled || len(s.entries) >= sessionCollectionLimit(a.budget.MaxCollectionElements) || !reserve(512+int64(len(key.requester)+len(key.server)+len(key.domain.Encapsulation))) {
			s.disabled = true
			m["Association"], m["Context Retention Warning"] = "untracked-control-history", "capture control history budget exhausted; new identities cannot establish association"
			return
		}
		o = &ntpControlObservation{opcode: op, association: assoc, final: -1, orphan: response}
		s.entries[key] = o
	}
	refuse := func(why string) {
		if o.refused == "" {
			o.refused = why
		}
		m["Association"], m["Control Reassembly"], m["Context Retention Warning"] = "ambiguous-sequence", "refused", o.refused
		e.ResponseTo, e.TransactionID = 0, 0
	}
	if o.refused != "" {
		refuse(o.refused)
		return
	}
	if o.orphan {
		m["Association"] = "response-before-request-history"
		return
	}
	if o.opcode != op || o.association != assoc {
		refuse("sequence reused with a different opcode or association")
		return
	}
	if e.Timestamp.IsZero() || o.request != 0 && (e.Timestamp.Before(o.requestTime) || e.Timestamp.Sub(o.requestTime) > 30*time.Second) {
		refuse("capture clock unavailable/regressed or local30-second request lease expired; sequence history retained")
		return
	}
	hash := sha256.Sum256(w)
	if !response {
		if o.request == 0 {
			o.request, o.requestHash = e.ID, hash
			o.requestTime = e.Timestamp
			m["Association"] = "observed-request"
		} else if o.requestHash == hash && !o.complete {
			m["Association"], m["Observed Duplicate PDU"] = "identical-active-request", o.request
		} else {
			refuse("request sequence reused after completion or with different content")
			return
		}
		e.TransactionID = o.request
		m["Request PDU ID"] = o.request
		return
	}
	if o.request == 0 {
		m["Association"] = "missing-request"
		return
	}
	if o.complete {
		for _, p := range o.parts {
			if p.hash == hash {
				m["Association"], m["Observed Duplicate PDU"], m["Control Reassembly"] = "identical-observed-response", p.pdu, "duplicate-no-replay"
				return
			}
		}
		refuse("different response after a completed sequence")
		return
	}
	status, errorFlag := binary.BigEndian.Uint16(w[4:6]), w[1]&64 != 0
	if o.responseSeen && (o.status != status || o.errorFlag != errorFlag) {
		refuse("inconsistent response status across fragments")
		return
	}
	o.status, o.errorFlag, o.responseSeen = status, errorFlag, true
	off, n, more := int(binary.BigEndian.Uint16(w[8:10])), int(binary.BigEndian.Uint16(w[10:12])), w[1]&32 != 0
	end := off + n
	if more && n == 0 {
		refuse("nonfinal empty control fragment cannot establish progress")
		return
	}
	if end > min(65536, a.config.MaxMessageBytes) {
		refuse("control reassembly exceeds configured byte limit")
		return
	}
	if o.final >= 0 && end > o.final || !more && o.final >= 0 && o.final != end {
		refuse("inconsistent final control extent")
		return
	}
	if !more {
		for _, v := range o.parts {
			if v.offset+v.length > end {
				refuse("final extent excludes an observed fragment")
				return
			}
		}
		o.final = end
	}
	newBytes := 0
	for i, b := range w[12 : 12+n] {
		at := off + i
		if at < len(o.seen) && o.seen[at] != 0 {
			if o.data[at] != b {
				refuse("conflicting control fragment overlap")
				return
			}
		} else {
			newBytes++
		}
	}
	partNeeded := newBytes > 0 || n == 0
	if partNeeded && (len(o.parts) >= min(128, sessionCollectionLimit(a.budget.MaxCollectionElements)) || len(e.SourceBytes.PacketRefs) > 32) {
		refuse("control byte-source witness budget exhausted")
		return
	}
	capacity := max(len(o.data), end)
	cost := int64(2 * (capacity - len(o.data)))
	if partNeeded {
		cost += 256 + int64(len(e.SourceBytes.PacketRefs))*160
		for _, r := range e.SourceBytes.PacketRefs {
			cost += int64(len(r.Domain.Encapsulation))
		}
	}
	if !reserve(cost) {
		refuse("capture control reassembly byte budget exhausted")
		return
	}
	if capacity > len(o.data) {
		data, seen := make([]byte, capacity), make([]byte, capacity)
		copy(data, o.data)
		copy(seen, o.seen)
		o.data, o.seen = data, seen
	}
	for i, b := range w[12 : 12+n] {
		o.data[off+i], o.seen[off+i] = b, 1
	}
	if partNeeded {
		o.parts = append(o.parts, ntpControlPart{off, n, e.ID, hash, append([]PacketReference(nil), e.SourceBytes.PacketRefs...)})
	}
	e.ResponseTo, e.TransactionID = o.request, o.request
	m["Association"], m["Request PDU ID"], m["Control Reassembly"] = "observed-request-key", o.request, "pending-gap-or-final"
	m["Association Scope"] = "observed endpoint/domain/version/sequence/opcode/association within local30-second capture-time lease; not authenticated invocation or peer delivery"
	if o.final < 0 {
		return
	}
	for _, b := range o.seen[:o.final] {
		if b == 0 {
			return
		}
	}
	o.complete = true
	m["Control Reassembly"], m["Reassembly Complete"] = "complete", true
	m["Reassembled Control Data"] = bytes.Clone(o.data[:o.final])
	m["Reassembled Control SHA256"] = fmt.Sprintf("%x", sha256.Sum256(o.data[:o.final]))
	m["Reassembled Byte Source"] = ntpControlByteSource(o, e.Domain)
	if op == 1 && assoc == 0 && !errorFlag && o.final%4 != 0 {
		o.refused = "read-status association list is not a sequence of four-byte entries"
		m["Control Payload Failure"] = o.refused
		m["Association"] = "malformed-control-payload"
		delete(m, "Request PDU ID")
		e.ResponseTo, e.TransactionID = 0, 0
		return
	}
	if op == 1 && assoc == 0 && !errorFlag && o.final%4 == 0 && o.final/4 <= sessionCollectionLimit(a.budget.MaxCollectionElements) {
		rows := make([]map[string]any, 0, o.final/4)
		for i := 0; i < o.final; i += 4 {
			rows = append(rows, map[string]any{"Association ID": binary.BigEndian.Uint16(o.data[i : i+2]), "Reported Status Word": binary.BigEndian.Uint16(o.data[i+2 : i+4])})
		}
		m["Reported Associations"] = rows
	}
}

func ntpControlByteSource(o *ntpControlObservation, domain CaptureDomain) map[string]any {
	ranges := make([]map[string]any, 0, len(o.parts))
	parents := make([]uint64, 0, len(o.parts))
	physical := true
	for _, p := range o.parts {
		if p.length == 0 {
			continue
		}
		parents = append(parents, p.pdu)
		refs := make([]map[string]any, 0, len(p.refs))
		if len(p.refs) == 0 {
			physical = false
		}
		for _, r := range p.refs {
			refs = append(refs, map[string]any{"Packet": r.Number, "Section": r.Domain.Section, "Interface": r.Domain.Interface, "Encapsulation": r.Domain.Encapsulation})
		}
		ranges = append(ranges, map[string]any{"Offset": p.offset, "Length": p.length, "Parent PDU": p.pdu, "Packet References": refs})
	}
	coverage := "complete"
	if !physical {
		coverage = "partial-physical-witnesses"
	}
	return map[string]any{"Kind": "reassembled-ntp-control", "Offset": 0, "Length": o.final, "Capture Domain": domain, "Byte Coverage Complete": true, "Packet Coverage Complete": physical, "Coverage": coverage, "Parent PDUs": parents, "Ranges": ranges}
}
