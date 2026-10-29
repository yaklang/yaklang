package pcaputil

// A passive capture can omit an entire key epoch, including its ACKs. Search
// only a bounded number of successors of this connection's traffic secret.
// RFC 9001 section 6 keeps header protection unchanged across those updates.
const quicForwardEpochLimit = 4
const quicPreviousEpochLimit = 4

type quicPriorKey struct {
	key      *quicTrafficKeys
	phase    int
	lower    uint64 // exclusive: accepted packet from an earlier epoch
	lowerSet bool
	upper    uint64 // exclusive: accepted packet from a newer epoch
}

type quicPacketProtection struct {
	plain                  []byte
	pn                     uint64
	pnLen                  int
	first                  byte
	phase, steps           int
	previous, invalidOrder bool
	unboundPhase           bool
	priorIndex             int
	key                    *quicTrafficKeys
	prior                  []quicPriorKey
}

// Shared by connection ownership and delivery. No keys, PN history, streams,
// counters or key-phase state may be committed while testing a candidate.
func (q *binQUIC) unprotectEpochs(dir int, wire []byte, h quicHdr, hp *quicTrafficKeys, keys *quicKeyring) (quicPacketProtection, error) {
	largest := q.spaces[dir][h.space].largest
	var phases [2]*quicTrafficKeys
	if !h.long && keys != nil {
		phases = keys.app[dir]
	}
	plain, pn, width, first, phase, err := quicUnprotect(wire, h, hp, phases, largest)
	if err == nil {
		result := quicPacketProtection{plain: plain, pn: pn, pnLen: width, first: first, phase: phase, key: hp}
		if !h.long {
			result.key = phases[phase]
			if result.key == nil {
				// Low-level unprotect falls back to the HP key when a phase
				// slot is absent. The tag still proves ownership, but cannot
				// authorize installing an unconfigured phase or a nil key.
				result.key, result.unboundPhase = hp, true
				return result, nil
			}
			if phase != q.keyPhase[dir] {
				result.steps = 1
				result.prior = []quicPriorKey{{key: phases[q.keyPhase[dir]], phase: q.keyPhase[dir], upper: pn}}
			}
		}
		return result, nil
	}
	if h.long || keys == nil {
		return quicPacketProtection{}, err
	}
	try := func(key *quicTrafficKeys, expectedPhase int) (quicPacketProtection, bool) {
		if key == nil {
			return quicPacketProtection{}, false
		}
		p, number, width, first, phase, e := quicUnprotect(wire, h, hp, [2]*quicTrafficKeys{key, key}, largest)
		if e != nil || phase != expectedPhase {
			return quicPacketProtection{}, false
		}
		return quicPacketProtection{plain: p, pn: number, pnLen: width, first: first, phase: phase, key: key}, true
	}
	for i := len(q.previousKeys[dir]) - 1; i >= 0; i-- {
		old := q.previousKeys[dir][i]
		if result, ok := try(old.key, old.phase); ok {
			result.previous, result.priorIndex = true, i
			result.invalidOrder = result.pn >= old.upper || old.lowerSet && result.pn <= old.lower
			return result, nil
		}
	}
	current := phases[q.keyPhase[dir]]
	if current == nil {
		return quicPacketProtection{}, err
	}
	prior := []quicPriorKey{{key: current, phase: q.keyPhase[dir]}}
	next := phases[1-q.keyPhase[dir]]
	for steps := 2; steps <= quicForwardEpochLimit && next != nil; steps++ {
		prior = append(prior, quicPriorKey{key: next, phase: (q.keyPhase[dir] + steps - 1) & 1})
		var e error
		next, e = quicNextPhase(next)
		if e != nil {
			return quicPacketProtection{}, e
		}
		if result, ok := try(next, (q.keyPhase[dir]+steps)&1); ok {
			result.steps = steps
			result.prior = prior
			for i := range result.prior {
				result.prior[i].upper = result.pn
			}
			return result, nil
		}
	}
	return quicPacketProtection{}, err
}

// Called after reserved bits, frame grammar/role, transport state, duplicate and
// PN-order validation. Application-layer semantic errors remain distinct from
// transport packet protection and are reported by their own dissectors.
func (q *binQUIC) commitProtectionEpoch(dir int, candidate *binQUIC, result quicPacketProtection, info map[string]any) error {
	if result.previous {
		// Delayed intermediate generations also constrain one another.
		for i := range q.previousKeys[dir] {
			old := &q.previousKeys[dir][i]
			if i < result.priorIndex {
				old.upper = min(old.upper, result.pn)
			} else if i > result.priorIndex && (!old.lowerSet || old.lower < result.pn) {
				old.lower, old.lowerSet = result.pn, true
			}
		}
		if !q.keyFloorSet[dir] || result.pn > q.keyFloor[dir] {
			q.keyFloor[dir], q.keyFloorSet[dir] = result.pn, true
		}
		return nil
	}
	if result.steps > 0 {
		if result.key == nil {
			return protocolError(ErrContextRequired, "QUIC candidate traffic key unavailable")
		}
		next, err := quicNextPhase(result.key)
		if err != nil {
			return err
		}
		retained := append([]quicPriorKey(nil), q.previousKeys[dir]...)
		maximum, observed := q.acceptedEpochMaximum(dir)
		for i, previous := range result.prior {
			if previous.key != nil {
				previous.upper = result.pn
				previous.lower, previous.lowerSet = maximum, observed
				if i == 0 {
					previous.lower, previous.lowerSet = q.keyFloor[dir], q.keyFloorSet[dir]
				}
				retained = append(retained, previous)
			}
		}
		if len(retained) > quicPreviousEpochLimit {
			retained = retained[len(retained)-quicPreviousEpochLimit:]
		}
		q.previousKeys[dir] = retained
		q.keyFloor[dir], q.keyFloorSet[dir] = maximum, observed
		q.keyFirst[dir], q.keyFirstSet[dir] = result.pn, true
		q.keyLast[dir] = result.pn
		candidate.keys.app[dir][result.phase] = result.key
		candidate.keys.app[dir][1-result.phase] = next
		q.keyPhase[dir] = result.phase
		if result.steps > 1 {
			info["Key Update Recovery"] = "authenticated forward traffic-secret derivation"
			info["Key Update Steps"] = uint64(result.steps)
			info["Unobserved Key Epochs"] = uint64(result.steps - 1)
		}
	} else {
		if !q.keyFirstSet[dir] || result.pn < q.keyFirst[dir] {
			q.keyFirst[dir], q.keyFirstSet[dir] = result.pn, true
			for i := range q.previousKeys[dir] {
				q.previousKeys[dir][i].upper = min(q.previousKeys[dir][i].upper, result.pn)
			}
		}
		q.keyLast[dir] = max(q.keyLast[dir], result.pn)
	}
	return nil
}

func (q *binQUIC) acceptedEpochMaximum(dir int) (uint64, bool) {
	if q.keyFirstSet[dir] {
		return q.keyLast[dir], true
	}
	// Preserve callers that supply an already authenticated PN context before
	// the first packet is consumed. Once observed, rejected future packets in
	// the global PN window cannot pretend to belong to the old accepted key.
	space := q.spaces[dir][quicSpaceApplication]
	if !space.init {
		return 0, false
	}
	if _, rejected := space.rejected[space.largest]; !rejected {
		return space.largest, true
	}
	var maximum uint64
	observed := false
	for _, pn := range space.seen {
		if _, rejected := space.rejected[pn]; !rejected {
			maximum, observed = max(maximum, pn), true
		}
	}
	return maximum, observed
}
