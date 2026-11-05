package pcaputil

import "bytes"

// Identity is private and issuer-scoped. Parsed frame slices and editable
// event metadata cannot overwrite a previously observed reset token.
func (q *binQUIC) applyNativeNewCID(dir int, frame map[string]any, max int) error {
	wireCID, cidOK := frame["CID"].([]byte)
	token, tokenOK := frame["Reset Token"].([]byte)
	seq, seqOK := frame["Sequence"].(uint64)
	retire, retireOK := frame["Retire Prior To"].(uint64)
	if !cidOK || len(wireCID) < 1 || len(wireCID) > 20 || !tokenOK || len(token) != 16 || !seqOK || !retireOK || retire > seq {
		return protocolError(ErrMalformedMessage, "QUIC NEW_CONNECTION_ID requires complete CID, sequence and reset token")
	}
	cid := string(wireCID)
	if old, ok := q.cidSequences[dir][seq]; ok && old != cid {
		return protocolError(ErrMalformedMessage, "QUIC CID sequence changed")
	}
	if old, ok := q.cidRetired[dir][seq]; ok && old != cid {
		return protocolError(ErrMalformedMessage, "QUIC retired CID sequence changed")
	}
	for _, history := range []map[uint64]string{q.cidSequences[dir], q.cidRetired[dir]} {
		for number, old := range history {
			if old == cid && number != seq {
				return protocolError(ErrMalformedMessage, "QUIC CID reused with a different sequence")
			}
		}
	}
	var ownedToken [16]byte
	copy(ownedToken[:], token)
	if old, known := q.cidTokens[dir][seq]; known && old != ownedToken {
		return protocolError(ErrMalformedMessage, "QUIC CID reset token changed")
	}
	floor := max64(q.cidRetiredBelow[dir], retire)
	_, retired := q.cidRetired[dir][seq]
	retired = retired || seq < floor
	// Account for retirement before enforcing the active route limit, but do
	// not mutate the real route/floor if the replacement still exceeds it.
	count := q.nativeCIDRouteCount()
	present := q.hasNativeCID(dir, cid)
	for number, old := range q.cidSequences[dir] {
		if number < floor {
			if q.hasNativeCID(dir, old) {
				count--
				if old == cid {
					present = false
				}
			}
		}
	}
	if !retired && !present && count >= max {
		return protocolError(ErrResourceExceeded, "QUIC CID budget")
	}
	q.cidRetiredBelow[dir] = floor
	for number, old := range q.cidSequences[dir] {
		if number < floor {
			q.rememberRetiredCID(dir, number, old, max)
			q.dropNativeCID(dir, old)
			delete(q.cidSequences[dir], number)
		}
	}
	if retired {
		frame["Already Retired"] = true
		return nil
	}
	if q.cidSequences[dir] == nil {
		q.cidSequences[dir] = map[uint64]string{}
	}
	if q.cidTokens[dir] == nil {
		q.cidTokens[dir] = map[uint64][16]byte{}
	}
	if q.cids == nil {
		q.cids = map[string]int{}
	}
	q.cidSequences[dir][seq], q.cidTokens[dir][seq] = cid, ownedToken
	q.addNativeCID(dir, cid)
	return nil
}

func (q *binQUIC) cloneCIDState() binQUIC {
	probe := *q
	if q.cids != nil {
		probe.cids = make(map[string]int, len(q.cids))
		for cid, owner := range q.cids {
			probe.cids[cid] = owner
		}
	}
	for d := 0; d < 2; d++ {
		if q.cidSequences[d] != nil {
			probe.cidSequences[d] = cloneQUICCIDSequences(q.cidSequences[d])
		}
		if q.cidRetired[d] != nil {
			probe.cidRetired[d] = cloneQUICCIDSequences(q.cidRetired[d])
		}
		if q.cidTokens[d] != nil {
			probe.cidTokens[d] = make(map[uint64][16]byte, len(q.cidTokens[d]))
			for sequence, token := range q.cidTokens[d] {
				probe.cidTokens[d][sequence] = token
			}
		}
	}
	return probe
}

// Check the complete CID batch before packet delivery or speculative key-epoch
// installation. Other frame handlers are not run on this probe.
func (q *binQUIC) validateNativeCIDFrames(dir int, h quicHdr, frames []map[string]any, max int) error {
	var probe *binQUIC
	if h.long || q.cids == nil {
		copy := q.cloneCIDState()
		probe = &copy
		if err := probe.bindNativeCIDHeader(dir, h, max); err != nil {
			return err
		}
	}
	for _, frame := range frames {
		name, _ := frame["Frame Type"].(string)
		if name != "NEW_CONNECTION_ID" && name != "RETIRE_CONNECTION_ID" {
			continue
		}
		if probe == nil {
			copy := q.cloneCIDState()
			probe = &copy
		}
		if name == "RETIRE_CONNECTION_ID" {
			sequence, _ := frame["Sequence"].(uint64)
			for _, state := range []*binQUIC{q, probe} {
				for _, history := range []map[uint64]string{state.cidSequences[1-dir], state.cidRetired[1-dir]} {
					if cid, known := history[sequence]; known && bytes.Equal([]byte(cid), h.dcid) {
						return protocolError(ErrMalformedMessage, "QUIC RETIRE_CONNECTION_ID retires this packet's destination CID")
					}
				}
			}
		}
		copy := make(map[string]any, len(frame))
		for key, value := range frame {
			copy[key] = value
		}
		if err := probe.applyFrames(dir, h.space, []map[string]any{copy}, map[string]any{}, max); err != nil {
			return err
		}
	}
	return nil
}

// A CID identifies its receiving endpoint. Equal bytes selected by both peers
// denote two issuer-qualified routes, with independent sequences and tokens.
func (q *binQUIC) hasNativeCID(owner int, cid string) bool {
	return owner >= 0 && owner < 2 && q.cids[cid]&(1<<owner) != 0
}
func (q *binQUIC) addNativeCID(owner int, cid string) {
	if q.cids == nil {
		q.cids = map[string]int{}
	}
	q.cids[cid] |= 1 << owner
}
func (q *binQUIC) dropNativeCID(owner int, cid string) {
	left := q.cids[cid] &^ (1 << owner)
	if left == 0 {
		delete(q.cids, cid)
	} else {
		q.cids[cid] = left
	}
}
func (q *binQUIC) nativeCIDRouteCount() int {
	count := 0
	for _, owners := range q.cids {
		if owners&1 != 0 {
			count++
		}
		if owners&2 != 0 {
			count++
		}
	}
	return count
}

// Called first on a private CID probe before PN/key-epoch/frame commits. Repeat
// the qualified update on the live state only after packet protection succeeds.
func (q *binQUIC) bindNativeCIDHeader(dir int, h quicHdr, max int) error {
	if q.cids == nil {
		q.cids = map[string]int{}
		initial := q.initialDCID
		if len(initial) == 0 && dir == 0 && h.typeName == "Initial" {
			initial = h.dcid
		}
		if len(initial) > 0 {
			if q.nativeCIDRouteCount() >= max {
				return protocolError(ErrResourceExceeded, "QUIC CID budget")
			}
			q.addNativeCID(1, string(initial))
		}
	}
	if h.long && len(h.scid) == 0 {
		q.zeroCID[dir] = true
	}
	if len(h.scid) > 0 {
		cid := string(h.scid)
		if !q.hasNativeCID(dir, cid) && q.nativeCIDRouteCount() >= max {
			return protocolError(ErrResourceExceeded, "QUIC CID budget")
		}
		if q.cidSequences[dir] == nil {
			q.cidSequences[dir] = map[uint64]string{0: cid}
		}
		if _, retired := q.cidRetired[dir][0]; !retired && q.cidRetiredBelow[dir] == 0 {
			q.addNativeCID(dir, cid)
		}
	}
	return nil
}
func quicObservedTupleDirection(f *binFlow, base *ProtocolEvent, choices uint8) (int, bool) {
	direction, matches := 0, 0
	for d := 0; d < 2; d++ {
		if choices&(1<<d) != 0 && base.Source == f.endpoints[d] && base.Destination == f.endpoints[1-d] {
			direction = d
			matches++
		}
	}
	return direction, matches == 1
}
