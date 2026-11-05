package pcaputil

// These private intervals contain actual nonempty, admitted early STREAM
// bytes. An out-of-order early tail does not qualify the preceding hole.
func (st *quicStream) rememberEarlyBytes(dir int, lo, hi uint64, max int) error {
	if st.h3 != nil {
		lo = max64(lo, uint64(st.h3.parsed[dir]))
	}
	if lo >= hi {
		return nil
	}
	rs := st.earlyBytes[dir]
	i := 0
	for i < len(rs) && rs[i].end < lo {
		i++
	}
	j := i
	for j < len(rs) && rs[j].off <= hi {
		lo, hi = min64(lo, rs[j].off), max64(hi, rs[j].end)
		j++
	}
	if j == i {
		if len(rs) >= max {
			return protocolError(ErrResourceExceeded, "QUIC early-byte range collection budget")
		}
		rs = append(rs, quicRange{})
		copy(rs[i+1:], rs[i:])
		rs[i] = quicRange{lo, hi}
	} else {
		rs[i] = quicRange{lo, hi}
		copy(rs[i+1:], rs[j:])
		clear(rs[len(rs)-(j-i-1):])
		rs = rs[:len(rs)-(j-i-1)]
	}
	st.earlyBytes[dir] = rs
	return nil
}

func (st *quicStream) pruneEarlyBytes(dir int, before uint64) {
	rs := st.earlyBytes[dir]
	n := 0
	for _, r := range rs {
		if r.end > before {
			r.off = max64(r.off, before)
			rs[n] = r
			n++
		}
	}
	clear(rs[n:])
	st.earlyBytes[dir] = rs[:n]
	if n == 0 {
		st.earlyBytes[dir] = nil
	}
}

func (st *quicStream) overlapsEarlyBytes(dir int, lo, hi uint64) bool {
	for _, r := range st.earlyBytes[dir] {
		if r.off >= hi {
			break
		}
		if r.end > lo {
			return true
		}
	}
	return false
}

func (st *quicStream) earlyQPACKFieldPrefix(dir int, payload []byte, start uint64) bool {
	_, ric, err := qpackPrefixed(payload, 8)
	if err != nil || ric >= len(payload) {
		return false // the existing complete-prefix gate reports the structure error
	}
	_, base, err := qpackPrefixed(payload[ric:], 7)
	return err == nil && st.overlapsEarlyBytes(dir, start, start+uint64(ric+base))
}

// Qualification-only wire scan: existing applyEncoder validates syntax,
// indexes, Huffman, capacity, budgets and table transactions. This scan neither
// allocates nor guesses a table; len(b) naturally bounds all reads and loops.
func qpackEncoderInstructionExtent(b []byte) (int, bool) {
	if len(b) == 0 {
		return 0, false
	}
	stringEnd := func(at int, prefix uint) (int, bool) {
		length, n, err := qpackPrefixed(b[at:], prefix)
		if err != nil || n > len(b)-at || length > uint64(len(b)-at-n) {
			return 0, false
		}
		return at + n + int(length), true
	}
	switch {
	case b[0]&0x80 != 0:
		_, n, err := qpackPrefixed(b, 6)
		if err != nil || n > len(b) {
			return 0, false
		}
		return stringEnd(n, 7)
	case b[0]&0x40 != 0:
		n, ok := stringEnd(0, 5)
		if !ok {
			return 0, false
		}
		return stringEnd(n, 7)
	default:
		_, n, err := qpackPrefixed(b, 5)
		return n, err == nil
	}
}

// A complete ordinary instruction prefix can precede an early/mixed one.
// Do not let an early byte in a later instruction poison that valid prefix,
// and do not interpret an instruction whose continuation/value has early bytes.
func (st *quicStream) currentQPACKEncoderPrefix(dir int, buf []byte, start uint64) (int, bool) {
	for off := 0; off < len(buf); {
		n, complete := qpackEncoderInstructionExtent(buf[off:])
		if !complete {
			if st.overlapsEarlyBytes(dir, start+uint64(off), start+uint64(len(buf))) {
				return off, true
			}
			return len(buf), false
		}
		if st.overlapsEarlyBytes(dir, start+uint64(off), start+uint64(off+n)) {
			return off, true
		}
		off += n
	}
	return len(buf), false
}

func (st *quicStream) canResumeCurrentQPACKEncoder(dir int) bool {
	h3 := st.h3
	n, early := st.currentQPACKEncoderPrefix(dir, h3.buf[dir][h3.parsed[dir]:], uint64(h3.parsed[dir]))
	return !early || n > 0
}

func (q *binQUIC) waitEarlyQPACKEncoder(h3 *h3StreamState, dir int, fr, info map[string]any) error {
	h3.encoderWaiting[dir] = true
	if h3.encoderWaitPDU[dir] == 0 {
		h3.encoderWaitPDU[dir] = q.eventID
	}
	info["QPACK Decoder Context"], fr["QPACK Decoder Context"] = "0-RTT remembered SETTINGS unconfirmed", "0-RTT remembered SETTINGS unconfirmed"
	return &h3EncoderContextError{protocolError(ErrContextRequired, "qpack: 0-RTT encoder prefix requires qualified remembered peer SETTINGS")}
}
