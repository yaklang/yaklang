package pcaputil

import (
	"bytes"
	"container/list"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"time"
)

// Native QUIC uses capture-domain/CID evidence, never a port or a payload guess.
func (a *binParser) decodeQUICDatagram(base *ProtocolEvent, wire []byte) ([]*ProtocolEvent, bool) {
	if len(wire) < 1 || wire[0]&0x40 == 0 {
		return nil, false
	}
	long := wire[0]&0x80 != 0
	var first quicHdr
	if long {
		if len(wire) < 7 || !quicSupportedVersion(binary.BigEndian.Uint32(wire[1:5])) {
			return nil, false
		}
		var err error
		first, err = quicParseHeader(wire, true)
		if err != nil {
			return nil, false
		}
	}
	a.udpMu.Lock()
	defer a.udpMu.Unlock()
	s := a.udpSessions
	if s == nil {
		if !long || first.typeName != "Initial" {
			return nil, false
		}
		s = &binUDPStore{entries: map[binUDPKey]*list.Element{}}
		a.udpSessions = s
	}
	retireMatchedIdle := func(v *binUDPEntry) {
		// Unknown CIDs and foreign capture domains cannot age an observed
		// connection. Shared UDP LRU order is not a per-connection clock.
		if !base.Timestamp.IsZero() && !v.touched.IsZero() && base.Timestamp.Sub(v.touched) >= 10*time.Minute {
			v.flow.retireQUICUDP()
		}
	}

	var el, retired *list.Element
	type zeroTupleRoute struct {
		element *list.Element
		dir     int
	}
	var zeroTuples []zeroTupleRoute
	dir, retiredDir := 0, 0
	retiredCID, retiredAmbiguous := false, false
	retiredProfile := ""
	selectRetired := func(candidate *list.Element, d int, byCID bool) {
		profile := quicNativeProfile(candidate.Value.(*binUDPEntry).flow.quic.effectiveVersion())
		if retiredProfile == "" {
			retiredProfile = profile
		} else if retiredProfile != profile {
			retiredProfile = "quic-native"
		}
		if retired != nil && retired != candidate {
			retiredAmbiguous = true
		}
		retired, retiredDir = candidate, d
		retiredCID = retiredCID || byCID
	}
	for _, candidate := range s.entries {
		v := candidate.Value.(*binUDPEntry)
		q := v.flow.quic
		if q == nil || v.key.domain != base.Domain {
			continue
		}
		// Zero-length CIDs cannot migrate: retain the observed endpoint tuple.
		for d := 0; d < 2; d++ {
			if q.zeroCID[1-d] && (!long || len(first.dcid) == 0) && base.Source == v.flow.endpoints[d] && base.Destination == v.flow.endpoints[1-d] {
				retireMatchedIdle(v)
				if q.retired {
					selectRetired(candidate, d, false)
					continue
				}
				// An endpoint tuple is not an epoch identifier, even when the
				// old close packet was lost. Resolve competing tuple/CID routes
				// by packet protection after collecting all candidates.
				zeroTuples = append(zeroTuples, zeroTupleRoute{candidate, d})
			}
		}
		for cid, owner := range q.cids {
			match := long && string(first.dcid) == cid || !long && len(wire) >= 1+len(cid) && string(wire[1:1+len(cid)]) == cid
			if match {
				retireMatchedIdle(v)
				if q.retired {
					selectRetired(candidate, 1-owner, true)
					continue
				}
				if el != nil && el != candidate {
					return nil, false
				}
				el = candidate
				dir = 1 - owner
			}
		}
	}
	if len(zeroTuples) == 1 && el == nil && !zeroTuples[0].element.Value.(*binUDPEntry).flow.quic.closed {
		el, dir = zeroTuples[0].element, zeroTuples[0].dir
		zeroTuples = nil // One active captured tuple has no competing epoch.
	}
	if retired != nil && (retiredCID || el == nil && len(zeroTuples) == 0) {
		// After retirement, packet protection with public Initial keys cannot
		// establish a fresh captured generation. Preserve the entire refused
		// datagram without pretending its coalesced packets were decoded.
		e := *base
		e.Protocol, e.Profile = "quic", retiredProfile
		if long {
			e.Profile = quicNativeProfile(first.version)
		}
		e.nativeQUICObservation = true
		e.Admission, e.Completeness = "observed-retired-CID", "datagram"
		e.ID, e.Direction, e.FlowID = a.ids.Add(1), retiredDir, 0
		e.ResponseTo, e.TransactionID = 0, 0
		if !retiredAmbiguous {
			e.FlowID = retired.Value.(*binUDPEntry).flow.id
		} else {
			e.Direction = 0 // No unique captured endpoint role; never depend on map iteration.
		}
		e.Length, e.Session = len(wire), nil
		a.finishProtocolDatagram(&e, wire, nil, protocolError(ErrContextRequired, "QUIC captured connection ID was retired"))
		return []*ProtocolEvent{&e}, true
	}
	historical := false
	if el == nil && !long {
		for _, candidate := range s.entries {
			v := candidate.Value.(*binUDPEntry)
			q := v.flow.quic
			if q == nil || q.retired || q.keys == nil || v.key.domain != base.Domain {
				continue
			}
			for owner, retired := range q.cidRetired {
				if q.keys.app[1-owner][q.keyPhase[1-owner]] == nil {
					continue
				}
				for _, cid := range retired {
					if len(wire) >= 1+len(cid) && string(wire[1:1+len(cid)]) == cid {
						if el != nil && el != candidate {
							return nil, false
						}
						el, dir, historical = candidate, 1-owner, true
					}
				}
			}
		}
	}
	if len(zeroTuples) > 0 {
		// Old zero-CID packets can be reordered past both CONNECTION_CLOSE and
		// a new handshake on the same tuple, or the close can be lost entirely.
		// Probe protection without changing PN/key/stream state or leases,
		// then deliver to exactly one owner.
		var owner *list.Element
		ownerDir, matches := 0, 0
		selectOwner := func(candidate *list.Element, d int) {
			if candidate.Value.(*binUDPEntry).flow.quic.matchesExistingWireContext(d, wire) {
				if owner == candidate && ownerDir == d {
					return
				}
				owner, ownerDir, matches = candidate, d, matches+1
			}
		}
		if el != nil {
			selectOwner(el, dir)
		}
		for _, route := range zeroTuples {
			selectOwner(route.element, route.dir)
		}
		if matches != 1 {
			e := *base
			e.Protocol, e.Profile = "quic", ""
			mergeProfile := func(candidate *list.Element) {
				profile := quicNativeProfile(candidate.Value.(*binUDPEntry).flow.quic.effectiveVersion())
				if e.Profile == "" {
					e.Profile = profile
				} else if e.Profile != profile {
					e.Profile = "quic-native"
				}
			}
			if el != nil {
				mergeProfile(el)
			}
			for _, route := range zeroTuples {
				mergeProfile(route.element)
			}
			if long {
				e.Profile = quicNativeProfile(first.version)
			}
			e.nativeQUICObservation = true
			e.Admission, e.Completeness = "unresolved-zero-CID-generation", "datagram"
			e.ID, e.Direction, e.FlowID = a.ids.Add(1), 0, 0
			e.ResponseTo, e.TransactionID, e.Session = 0, 0, nil
			e.Length = len(wire)
			a.finishProtocolDatagram(&e, wire, nil, protocolError(ErrContextRequired, "QUIC reused zero-CID tuple has no unique authenticated captured generation"))
			return []*ProtocolEvent{&e}, true
		}
		if el != owner {
			historical = false
		}
		el, dir = owner, ownerDir
	}
	var f *binFlow
	key := binUDPKey{}
	if el != nil {
		f = el.Value.(*binUDPEntry).flow
	} else {
		if !long || first.typeName != "Initial" {
			return nil, false
		}
		key = binUDPKey{"quic/" + base.Source + "/" + hex.EncodeToString(first.dcid), base.Destination, base.Domain}
		f = &binFlow{a: a, protocol: "quic", quic: &binQUIC{native: true, byteLimit: a.budget.MaxMessageBytes, provider: a.tlsSecrets}, endpoints: [2]string{base.Source, base.Destination}}
	}
	var events []*ProtocolEvent
	var datagramCID []byte
	datagramAuthenticated, lastLongBoundary := false, false
	for off := 0; off < len(wire); {
		// RFC9312 2.4 also describes unused UDP payload after an Initial.
		// Observe only an all-zero suffix at a declared long-packet boundary
		// following authenticated wire in this datagram. It is not a protected
		// QUIC PADDING frame and grants no packet/application authentication.
		if datagramAuthenticated && lastLongBoundary && el != nil && wire[off] == 0 && bytes.Count(wire[off:], []byte{0}) == len(wire)-off {
			e := *base
			e.ID, e.FlowID, e.ResponseTo, e.TransactionID = a.ids.Add(1), f.id, 0, 0
			e.Protocol, e.Profile, e.Direction = "quic", quicNativeProfile(f.quic.effectiveVersion()), dir
			e.Length = len(wire) - off
			e.nativeQUICObservation = true
			e.Completeness, e.Admission = "datagram-padding", "observed-datagram-padding"
			e.Session = map[string]any{"Packet Name": "Datagram padding", "Datagram Offset": off, "Datagram Padding Length": uint64(len(wire) - off), "Authentication Verified": false, "Peer Identity Verified": false, "Payload Delivery": "unprotected UDP bytes; not delivered to QUIC packet or application state", "Wire Field Ranges": map[string]any{"Datagram Padding": []any{0, len(wire) - off}}}
			e.semanticSession = cloneSession(e.Session)
			e.semanticFields = map[string]any{"Datagram Padding": bytes.Clone(wire[off:])}
			var paddingError error
			if len(events) >= a.budget.MaxCollectionElements {
				e.Session, e.semanticSession, e.semanticFields = nil, nil, nil
				e.Completeness, e.Admission = "datagram", "unparsed-datagram-remainder"
				paddingError = protocolError(ErrResourceExceeded, "QUIC packet/conversation budget")
			}
			a.finishProtocolDatagram(&e, wire[off:], nil, paddingError)
			e.Summary = fmt.Sprintf("Datagram padding · offset=%d · %d bytes · unprotected UDP bytes", off, len(wire)-off)
			if paddingError != nil {
				e.Summary = fmt.Sprintf("Unparsed QUIC datagram remainder · offset=%d · %d bytes · authentication not verified", off, len(wire)-off)
				f.retireQUICUDP()
			}
			events = append(events, &e)
			break
		}
		e := *base
		e.Protocol = "quic"
		e.Profile = quicNativeProfile(f.quic.effectiveVersion())
		e.nativeQUICObservation = true
		e.Admission = "unverified-QUIC-candidate"
		// Native CID selection owns the association; a refused new candidate
		// cannot inherit a carrier projection's unrelated flow identifier.
		e.ID, e.FlowID, e.ResponseTo, e.TransactionID = a.ids.Add(1), 0, 0, 0
		f.quic.eventID = e.ID
		// Temporary immutable input; retained ranges clone their bounded refs.
		f.quic.eventRefs = e.SourceBytes.PacketRefs
		e.Direction = dir
		h, err := f.quic.wireHeader(wire[off:], dir)
		if err == nil && h.long {
			e.Profile = quicNativeProfile(h.version)
		}
		if err != nil {
			if _, typed := classifySessionError(err); typed == nil {
				err = protocolError(ErrMalformedMessage, "invalid coalesced QUIC header: %v", err)
			}
		}
		size := len(wire) - off
		if err == nil && (h.size <= 0 || h.size > size) {
			err = protocolError(ErrMalformedMessage, "invalid coalesced QUIC length")
		}
		bounded := err == nil
		if bounded {
			size = h.size
			if off == 0 {
				datagramCID = h.dcid
			}
		}
		// RFC9000 12.2 permits only one destination CID in a datagram.
		// Preserve mismatched bytes as a diagnostic without consuming them in
		// the selected connection; later correctly framed packets stay usable.
		cidMismatch := bounded && off > 0 && !bytes.Equal(datagramCID, h.dcid)
		rejectedCID := false
		raw := wire[off : off+size]
		e.Length = size
		if err == nil && (len(events) >= a.budget.MaxCollectionElements || el == nil && len(s.entries) >= sessionCollectionLimit(a.budget.MaxCollectionElements)) {
			err = protocolError(ErrResourceExceeded, "QUIC packet/conversation budget")
		}
		if err == nil && cidMismatch {
			rejectedCID = true
			err = protocolError(ErrMalformedMessage, "coalesced QUIC destination CID differs from first packet")
		}
		if err == nil {
			err = f.reserveSession(f.quic.initialMemory() + int64(len(raw))*32)
		}
		if err == nil {
			e.Session, err = f.quic.consume(dir, raw, a.budget.MaxCollectionElements)
		}
		f.quic.eventRefs = nil
		if historical && e.Session["Authentication Verified"] != true {
			return nil, false
		}
		if historical {
			e.Session["Retired Destination CID"] = true
			e.Session["CID Association"] = "authenticated late packet on retired CID"
		}
		if err == nil || isRecoverableH3ObservationError(err) {
			if budgetError := f.reserveSession(f.quic.initialMemory()); budgetError != nil {
				err = budgetError
			}
		}
		if el == nil && err == nil && e.Session["Authentication Verified"] == true {
			f.id = a.flows.Add(1)
			el = s.lru.PushBack(&binUDPEntry{key, f, base.Timestamp})
			s.entries[key] = el
		}
		if el != nil && !rejectedCID {
			v := el.Value.(*binUDPEntry)
			// Closed-generation evidence may be inspected, but must not keep
			// obsolete keys alive indefinitely through late packets.
			if !f.quic.closed {
				if base.Timestamp.After(v.touched) {
					v.touched = base.Timestamp
				}
				s.lru.MoveToBack(el)
			}
			e.FlowID = f.id
		}
		if e.Session != nil {
			e.Session["Observation Scope"] = "UDP capture domain and observed CID"
			e.Session["Datagram Offset"] = off
			e.Session["Native Carrier"] = true
			e.Session["ALPN"] = f.quic.alpn
			// Duplicate/protected packets can return before finishFrames. Their
			// connection context still includes an earlier observed HTTP3 fault.
			f.quic.annotateH3ConnectionFault(e.Session)
		}
		if e.Session != nil {
			var transaction uint64
			multiple := false
			frames, _ := e.Session["Frames"].([]map[string]any)
			for _, fr := range frames {
				if id, ok := fr["Request PDU ID"].(uint64); ok && id != 0 {
					if transaction != 0 && transaction != id {
						multiple = true
					}
					transaction = id
				}
			}
			if !multiple && transaction != 0 {
				e.TransactionID = transaction
				if dir == 1 {
					e.ResponseTo = transaction
				}
			}
			if e.Session["Encrypted"] == true {
				e.Completeness = "encrypted"
				e.Session["Content Visibility"] = "encrypted"
				e.Admission = "observed-CID-protected-payload"
			}
		}
		if e.Session["HTTP3"] == true {
			e.Protocol = "http3"
			e.Profile = "http3-native"
		}
		if e.Session["DoQ"] == true {
			e.Protocol = "doq"
			e.Profile = "doq-native"
			if d, ok := e.Session["DoQ Message"].(map[string]any); ok {
				e.Session["DNS"] = d["DNS"]
			}
		}
		// A failed AEAD or unavailable key does not invalidate the long-header
		// length (RFC9000 12.2). Never scan ciphertext for a guessed boundary.
		// Other failures, notably resource refusal, stop processing and retain
		// the entire unprocessed remainder in one bounded diagnostic.
		_, typed := classifySessionError(err)
		recoverable := bounded && typed != nil && (typed.Kind == ErrAuthenticationFailed || typed.Kind == ErrEncrypted || rejectedCID && typed.Kind == ErrMalformedMessage || isRecoverableH3ObservationError(err))
		if err != nil && !recoverable {
			// Keep an existing single-packet error envelope. It cannot describe
			// an aggregated remainder or a refused resource reservation.
			if size != len(wire)-off || typed != nil && typed.Kind == ErrResourceExceeded {
				e.Session = nil
				e.ResponseTo, e.TransactionID = 0, 0
			}
			raw = wire[off:]
			size, e.Length = len(raw), len(raw)
		}
		// Admission describes these retained bytes, not the earlier packet that
		// selected this connection. In particular, an invalid trailing header or
		// an aggregated refusal is not authenticated by its datagram neighbour.
		switch {
		case !bounded || size != h.size:
			e.Admission = "unparsed-datagram-remainder"
		case rejectedCID:
			e.Admission = "rejected-coalesced-CID"
		case e.Session["Authentication Verified"] == true:
			e.Admission = "authenticated-wire-and-observed-CID"
		case e.FlowID != 0 && e.Session["Encrypted"] == true:
			e.Admission = "observed-CID-protected-payload"
		case e.FlowID != 0:
			e.Admission = "observed-CID-unverified-payload"
		default:
			e.Admission = "unverified-QUIC-candidate"
		}
		// The authenticated session already parsed this header. Preserve the wire
		// field view without executing a YAML operator for every ciphertext octet.
		e.nativeQUICWire = err == nil
		if bounded && size == h.size && e.Session != nil {
			observed := h
			if !h.long {
				// The session resolves a short header's CID size from observed
				// receiver context; the stateless outer header has no size byte.
				observed.dcid, _ = e.Session["DCID"].([]byte)
			}
			e.Session["Wire Field Ranges"] = quicHeaderWireRanges(observed, raw)
		}
		// Query identity and passive context are private even on a refused
		// native event; mutable public labels/caches cannot grant success.
		e.semanticSession = cloneSession(e.Session)
		a.finishProtocolDatagram(&e, raw, a.specs["application-layer.quic/QUIC"], err)
		datagramAuthenticated = datagramAuthenticated || e.Admission == "authenticated-wire-and-observed-CID" && e.Session["Authentication Verified"] == true
		lastLongBoundary = bounded && h.long && size == h.size && !rejectedCID
		if e.Admission == "unparsed-datagram-remainder" {
			e.Summary = fmt.Sprintf("Unparsed QUIC datagram remainder · offset=%d · %d bytes · authentication not verified", off, size)
		}
		events = append(events, &e)
		if e.sessionError != nil && e.sessionError.Kind == ErrResourceExceeded && el != nil {
			f.retireQUICUDP()
		}
		off += size
		if err != nil && !recoverable {
			break
		}
	}
	if el == nil {
		f.closeSession()
	}
	return events, true
}

// This is an ownership probe, not frame delivery. In particular it never loads
// application secrets, advances a packet-number/key epoch, or accepts a Retry.
func (q *binQUIC) authenticatesExistingWire(dir int, wire []byte) bool {
	if q == nil || q.retired || q.keys == nil || dir < 0 || dir > 1 {
		return false
	}
	h, err := q.wireHeader(wire, dir)
	if err != nil || h.retry || h.vn || h.size <= 0 || h.size > len(wire) {
		return false
	}
	probe, _, err := q.protectionCandidate(dir, h)
	if err != nil {
		return false
	}
	tk, _ := probe.keysFor(dir, h)
	if tk == nil {
		return false
	}
	var phases [2]*quicTrafficKeys
	if !h.long {
		phases = probe.keys.app[dir]
	}
	_, _, _, _, _, err = quicUnprotect(wire, h, tk, phases, q.spaces[dir][h.space].largest)
	if err != nil && !h.long && q.previousKey[dir] != nil {
		phases[1-q.keyPhase[dir]] = q.previousKey[dir]
		_, _, _, _, _, err = quicUnprotect(wire, h, tk, phases, q.spaces[dir][h.space].largest)
	}
	// Authenticated reserved-bit or frame violations still identify the captured
	// key epoch. The delivery path reports those violations without committing
	// their frames. Refusing them here would lose the correct connection and its
	// inspectable plaintext whenever a reused tuple has zero-length CIDs.
	return err == nil
}

// quicWireEnvelope preserves the QUIC YAML field shape and scalar types. Binary
// identifiers/tokens own raw bytes; strings would lose invalid UTF-8 in JSON IPC.
// It is only selected after native wire authentication, never as a plaintext probe.
func quicWireEnvelope(w []byte) (map[string]any, error) {
	h, err := quicParseHeader(w, true)
	if err != nil {
		return nil, err
	}
	if h.size != len(w) {
		return nil, protocolError(ErrMalformedMessage, "QUIC envelope length mismatch")
	}
	fields := map[string]any{"First Byte": w[0]}
	off := 1
	if h.long {
		fields["Version"] = h.version
		fields["DCID Length"], fields["SCID Length"] = uint8(len(h.dcid)), uint8(len(h.scid))
		if len(h.dcid) > 0 {
			fields["DCID"] = bytes.Clone(h.dcid)
		}
		if len(h.scid) > 0 {
			fields["SCID"] = bytes.Clone(h.scid)
		}
		off = 7 + len(h.dcid) + len(h.scid)
		if h.typeName == "Initial" {
			length, width, err := quicVarint(w[off:])
			if err != nil {
				return nil, err
			}
			token := map[string]any{"Token Length": uint8(length)}
			if width > 1 {
				token["Token Length"], token["Length Encoding"] = length, uint8(w[off]>>6)
			}
			off += width
			if len(h.token) > 0 {
				token["Token"] = bytes.Clone(h.token)
				off += len(h.token)
			}
			fields["QUICToken"] = token
		}
	}
	if off < len(w) {
		payload := make([]any, len(w)-off)
		for i, b := range w[off:] {
			payload[i] = b
		}
		fields["Protected Payload"] = payload
	}
	return fields, nil
}

// Idle or refused wire is not a new-generation proof. Release key material,
// TLS/stream graphs and PN history, but keep already charged CID/zero-CID facts.
// Their bounded memory is owned by this capture and released on Close.
func (f *binFlow) retireQUICUDP() {
	q := f.quic
	if q == nil || q.retired {
		return
	}
	history, zero, version := q.cids, q.zeroCID, q.effectiveVersion()
	// Keep the already allocated base structure and its existing base charge.
	// Moving the bounded CID map needs no second allocation or reservation.
	*q = binQUIC{native: true, retired: true, cids: history, zeroCID: zero, version: version}
	retained := q.initialMemory()
	if f.sessionBytes > retained {
		f.a.buffered.Add(retained - f.sessionBytes)
		f.sessionBytes = retained
	}
}
