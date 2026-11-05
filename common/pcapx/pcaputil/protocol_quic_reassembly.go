package pcaputil

import (
	"bytes"
	"encoding/hex"
	"sort"
)

type quicPiece struct {
	off  uint64
	data []byte
}
type quicAssembler struct {
	data                   []byte
	pending                []quicPiece
	final                  uint64
	hasFinal, deliveredFIN bool
}

func (r *quicAssembler) memory() int64 {
	n := int64(cap(r.data))
	for _, p := range r.pending {
		n += int64(cap(p.data) + 32)
	}
	return n
}
func (r *quicAssembler) feed(off uint64, data []byte, fin bool, limit, max int) ([]byte, uint64, bool, error) {
	end := off + uint64(len(data))
	if end < off || end > uint64(limit) {
		return nil, 0, false, protocolError(ErrResourceExceeded, "QUIC reassembly byte limit")
	}
	if r.hasFinal && (end > r.final || fin && end != r.final) {
		return nil, 0, false, protocolError(ErrMalformedMessage, "QUIC final size changed")
	}
	if fin {
		if end < uint64(len(r.data)) {
			return nil, 0, false, protocolError(ErrMalformedMessage, "QUIC final size before received bytes")
		}
		for _, p := range r.pending {
			if p.off+uint64(len(p.data)) > end {
				return nil, 0, false, protocolError(ErrMalformedMessage, "QUIC final size before buffered range")
			}
		}
	}
	check := func(po uint64, pd []byte) bool {
		lo, hi := max64(off, po), min64(end, po+uint64(len(pd)))
		return lo >= hi || bytes.Equal(data[lo-off:hi-off], pd[lo-po:hi-po])
	}
	if !check(0, r.data) {
		return nil, 0, false, protocolError(ErrMalformedMessage, "QUIC conflicting retransmission")
	}
	for _, p := range r.pending {
		if !check(p.off, p.data) {
			return nil, 0, false, protocolError(ErrMalformedMessage, "QUIC conflicting buffered range")
		}
	}
	if fin {
		r.final = end
		r.hasFinal = true
	}
	start := uint64(len(r.data))
	if end > start {
		if off < start {
			data = data[start-off:]
			off = start
		}
		if off == start {
			r.data = append(r.data, data...)
		} else {
			for _, p := range r.pending {
				if off >= p.off && end <= p.off+uint64(len(p.data)) {
					return nil, start, false, nil
				}
			}
			if r.memory()+int64(len(data))+32 > int64(limit) {
				return nil, 0, false, protocolError(ErrResourceExceeded, "QUIC retained range byte budget")
			}
			if len(r.pending) >= max {
				return nil, 0, false, protocolError(ErrResourceExceeded, "QUIC range count limit")
			}
			r.pending = append(r.pending, quicPiece{off, bytes.Clone(data)})
			sort.Slice(r.pending, func(i, j int) bool { return r.pending[i].off < r.pending[j].off })
		}
	}
	for len(r.pending) > 0 && r.pending[0].off <= uint64(len(r.data)) {
		p := r.pending[0]
		n := uint64(len(r.data)) - p.off
		if n < uint64(len(p.data)) {
			r.data = append(r.data, p.data[n:]...)
		}
		r.pending[0] = quicPiece{}
		r.pending = r.pending[1:]
	}
	if len(r.pending) == 0 {
		r.pending = nil
	}
	done := r.hasFinal && r.final == uint64(len(r.data)) && !r.deliveredFIN
	if done {
		r.deliveredFIN = true
	}
	return r.data[start:], start, done, nil
}
func max64(a, b uint64) uint64 {
	if a > b {
		return a
	}
	return b
}
func min64(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}
func (q *binQUIC) wireHeader(raw []byte, dir int) (quicHdr, error) {
	h, err := quicParseHeader(raw, true)
	if err != nil || h.long || !q.native {
		return h, err
	}
	// A short header carries no CID length. A prefix match alone cannot
	// choose between two observed issuer-qualified lengths, including history.
	// There are at most twenty nonzero lengths; deduplicate retired/active
	// copies without depending on map iteration or preferring the longest CID.
	var candidates []quicHdr
	var seen [21]bool
	add := func(cid string) {
		n := len(cid)
		if n < 1 || n > 20 || seen[n] || len(raw) < 1+n || string(raw[1:1+n]) != cid {
			return
		}
		seen[n] = true
		candidate := h
		candidate.dcid, candidate.payloadOff = []byte(cid), 1+n
		candidates = append(candidates, candidate)
	}
	for cid, owners := range q.cids {
		if owners&(1<<(1-dir)) != 0 {
			add(cid)
		}
	}
	for _, cid := range q.cidRetired[1-dir] {
		add(cid)
	}
	if q.zeroCID[1-dir] {
		candidate := h
		candidate.payloadOff = 1
		candidates = append(candidates, candidate)
	}
	if len(candidates) == 1 {
		return candidates[0], nil
	}
	if len(candidates) == 0 {
		return h, sessionContext("QUIC short header requires observed destination CID")
	}
	var selected quicHdr
	matches := 0
	for _, candidate := range candidates {
		if q.authenticatesParsedWire(dir, raw, candidate) {
			selected, matches = candidate, matches+1
		}
	}
	if matches == 1 {
		return selected, nil
	}
	return h, sessionContext("QUIC short header CID length is ambiguous without unique packet protection")
}

func (q *binQUIC) rememberRetiredCID(owner int, seq uint64, cid string, max int) {
	if max < 1 {
		return
	}
	if q.cidRetired[owner] == nil {
		q.cidRetired[owner] = map[uint64]string{}
	}
	if _, exists := q.cidRetired[owner][seq]; !exists && len(q.cidRetired[owner]) >= max {
		oldest := ^uint64(0)
		for n := range q.cidRetired[owner] {
			if n < oldest {
				oldest = n
			}
		}
		delete(q.cidRetired[owner], oldest)
		if _, active := q.cidSequences[owner][oldest]; !active {
			delete(q.cidTokens[owner], oldest)
		}
	}
	q.cidRetired[owner][seq] = cid
}
func (q *binQUIC) feedCrypto(dir, space int, off uint64, data []byte, info map[string]any, max int) error {
	if dir < 0 || dir > 1 || space < quicSpaceInitial || space > quicSpaceApplication {
		return protocolError(ErrMalformedMessage, "QUIC CRYPTO direction or encryption level")
	}
	a := &q.cryptoData[dir][space]
	_, _, _, err := a.feed(off, data, false, q.byteLimit, max)
	if err != nil {
		return err
	}
	// All complete messages made available by this delivery form one TLS
	// transaction. A valid prefix followed by a refused message must not leave
	// a ClientHello/ALPN/key epoch committed while cryptoRead remains unchanged.
	next := binTLS{client: -1}
	if q.tls != nil {
		next = *q.tls
	}
	at := q.cryptoRead[dir][space]
	nextEarly := q.earlyContext
	var messages []map[string]any
	for len(a.data)-at >= 4 {
		if space == quicSpaceHandshake && next.app[dir] {
			return protocolError(ErrMalformedMessage, "QUIC handshake CRYPTO extends beyond observed Finished")
		}
		b := a.data[at:]
		n := 4 + (int(b[1]) << 16) + (int(b[2]) << 8) + int(b[3])
		if n > q.byteLimit {
			return protocolError(ErrResourceExceeded, "QUIC handshake size")
		}
		if len(b) < n {
			break
		}
		if err := quicTLSMessageLevel(b[0], space); err != nil {
			return err
		}
		if err := quicTLSHelloVersion(b[:n], max); err != nil {
			return err
		}
		if b[0] == 4 && next.version == 0 {
			return sessionContext("QUIC NewSessionTicket needs the observed negotiated TLS version")
		}
		// Initial protection authenticates packet bytes, not peer identity.
		m, err := next.handshakeMessage(dir, b[:n], space != quicSpaceInitial, max)
		if err != nil {
			return err
		}
		if err := quicTLSTicketValues(m); err != nil {
			return err
		}
		if err := nextEarly.observe(&next, b[0], b[4:n], max); err != nil {
			return err
		}
		messages = append(messages, m)
		at += n
	}
	if space == quicSpaceHandshake && next.app[dir] && (at < len(a.data) || len(a.pending) > 0) {
		// Even an incomplete header or an out-of-order tail cannot extend
		// this old level once its complete Finished is observed. Refuse the
		// entire TLS batch rather than committing just the Finished prefix.
		return protocolError(ErrMalformedMessage, "QUIC handshake CRYPTO extends beyond observed Finished")
	}
	if q.tls == nil {
		q.tls = &next
	} else {
		*q.tls = next
	}
	q.cryptoRead[dir][space] = at
	q.earlyContext = nextEarly
	if len(messages) > 0 {
		info["TLS Handshakes"] = messages
	}
	q.tls.resumption.project(info)
	info["CRYPTO Incomplete"] = at < len(a.data) || len(a.pending) > 0
	if len(q.tls.random) == 32 {
		info["Client Random"] = hex.EncodeToString(q.tls.random)
	}
	if q.tls.alpn != "" && space != quicSpaceInitial {
		q.alpn = q.tls.alpn
		info["ALPN"] = q.alpn
	}
	if !q.loadedKeys && q.provider != nil && len(q.tls.random) == 32 && q.tls.suite != 0 {
		aead := ""
		switch q.tls.suite {
		case 0x1301:
			aead = quicAEADAES128GCM
		case 0x1302:
			aead = quicAEADAES256GCM
		case 0x1303:
			aead = quicAEADChaCha20Poly1305
		default:
			return protocolError(ErrUnsupportedFeature, "QUIC traffic cipher unsupported")
		}
		secrets := map[string][]byte{}
		for _, label := range []string{"CLIENT_HANDSHAKE_TRAFFIC_SECRET", "SERVER_HANDSHAKE_TRAFFIC_SECRET", "CLIENT_TRAFFIC_SECRET_0", "SERVER_TRAFFIC_SECRET_0", "CLIENT_EARLY_TRAFFIC_SECRET"} {
			if v, ok := q.provider.LookupTLSSecret(label, q.tls.random); ok {
				secrets[label] = v
			}
		}
		ring, err := quicLoadKeys(QUICKeyMaterial{Secrets: secrets, AEAD: aead, Version: q.effectiveVersion()})
		if err != nil {
			return err
		}
		ring.initial = q.keys.initial
		q.keys = ring
		// The optional early secret must neither be omitted nor make five
		// provider entries look like incomplete regular keys. Replacing the
		// ring again would discard already established application key epochs.
		q.loadedKeys = ring.handshake[0] != nil && ring.handshake[1] != nil && ring.app[0][0] != nil && ring.app[1][0] != nil
	}
	return nil
}

func (q *binQUIC) isRetiredNativeCID(owner int, cid []byte) bool {
	if len(cid) == 0 {
		return false
	}
	for _, old := range q.cidRetired[owner] {
		if old == string(cid) {
			return true
		}
	}
	return false
}
