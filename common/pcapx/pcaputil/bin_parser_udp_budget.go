package pcaputil

// The shared UDP byte limit precedes semantic decoding. It must still retire
// association state for an admitted conversation: the denied on-wire request or
// reply did not disappear. Admission here examines only bounded headers/ports or
// an already observed exact endpoint/capture-domain key, never an oversized body.
func (a *binParser) refuseUDPOversizeAssociation(e *ProtocolEvent, w []byte, src, dst uint16) bool {
	explicit := a.datagramDecodeAs[dst]
	if explicit == "" {
		explicit = a.datagramDecodeAs[src]
	}
	prefix, protocol, profile := "", "", ""
	ttl := slmpIdleTTL
	switch {
	case explicit == "slmp" || explicit == "" && slmpStart(w):
		prefix, protocol, profile = "slmp/", "slmp", "slmp-binary-self-test"
	case explicit == "dlms-wrapper" || explicit == "" && len(w) >= 8 && w[0] == 0 && w[1] == 1:
		prefix, protocol, profile = "wrapper/", "dlms-wrapper", wrapperProfile(w)
		ttl = wrapperIdleTTL
	default:
		return false
	}
	key := binUDPKey{prefix + e.Source, prefix + e.Destination, e.Domain}
	if key.a > key.b {
		key.a, key.b = key.b, key.a
	}
	a.udpMu.Lock()
	store := a.udpSessions
	if store != nil {
		if e.Timestamp.After(store.clock) {
			store.clock = e.Timestamp
		}
		for el := store.lru.Front(); el != nil; {
			next := el.Next()
			v := el.Value.(*binUDPEntry)
			applicable := protocol == "slmp" && v.flow.slmp != nil || protocol == "dlms-wrapper" && v.flow.wrapper != nil
			if applicable && store.clock.Sub(v.touched) >= ttl {
				v.flow.closeSession()
				delete(store.entries, v.key)
				store.lru.Remove(el)
			}
			el = next
		}
	}
	existing := store != nil && store.entries[key] != nil
	if explicit == "" && !existing && !(protocol == "dlms-wrapper" && (src == 4059 || dst == 4059)) {
		a.udpMu.Unlock()
		return false
	}
	if existing {
		el := store.entries[key]
		v := el.Value.(*binUDPEntry)
		f := v.flow
		e.FlowID = f.id
		if e.Source != f.endpoints[0] {
			e.Direction = 1
		}
		var retained int64
		if f.slmp != nil {
			f.slmp.pending = nil
			f.slmp.ambiguous = true
			retained = 512 + 64*int64(len(f.slmp.seen))
		} else if f.wrapper != nil {
			f.wrapper.invalidate()
			retained = f.wrapper.storage()
		}
		// reserveSession is a high-water allocator. Removed wire/projection graphs
		// must release their ledger charge while bounded ambiguity markers remain.
		if retained < f.sessionBytes {
			a.buffered.Add(retained - f.sessionBytes)
			f.sessionBytes = retained
		}
		v.touched = store.clock
		store.lru.MoveToBack(el)
	}
	a.udpMu.Unlock()
	e.Protocol, e.Profile, e.Admission, e.Completeness = protocol, profile, "observed-conversation", "message"
	if explicit != "" {
		e.Admission = "explicit-decode-as"
	} else if !existing {
		e.Admission = "wire-and-port-hint"
	}
	if e.ID == 0 {
		e.ID = a.ids.Add(1)
	}
	a.finishProtocolDatagram(e, nil, nil, protocolError(ErrResourceExceeded, "%s UDP datagram exceeds message byte budget", protocol))
	e.Completeness = e.Status
	a.limited.Add(uint64(len(w)))
	return true
}
