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
	for cid, owner := range q.cids {
		if owner == 1-dir && len(raw) >= 1+len(cid) && string(raw[1:1+len(cid)]) == cid {
			h.dcid = []byte(cid)
			h.payloadOff = 1 + len(cid)
			return h, nil
		}
	}
	if q.zeroCID[1-dir] {
		h.payloadOff = 1
		return h, nil
	}
	return h, sessionContext("QUIC short header requires observed destination CID")
}
func (q *binQUIC) feedCrypto(dir, space int, off uint64, data []byte, info map[string]any, max int) error {
	a := &q.cryptoData[dir][space]
	_, _, _, err := a.feed(off, data, false, q.byteLimit, max)
	if err != nil {
		return err
	}
	if q.tls == nil {
		q.tls = &binTLS{client: -1}
	}
	at := q.cryptoRead[dir][space]
	var messages []map[string]any
	for len(a.data)-at >= 4 {
		b := a.data[at:]
		n := 4 + (int(b[1]) << 16) + (int(b[2]) << 8) + int(b[3])
		if n > q.byteLimit {
			return protocolError(ErrResourceExceeded, "QUIC handshake size")
		}
		if len(b) < n {
			break
		}
		// Initial protection authenticates packet bytes, not peer identity.
		m, err := q.tls.handshakeMessage(dir, b[:n], space != quicSpaceInitial)
		if err != nil {
			return err
		}
		messages = append(messages, m)
		at += n
	}
	q.cryptoRead[dir][space] = at
	if len(messages) > 0 {
		info["TLS Handshakes"] = messages
	}
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
		case 0x1303:
			aead = quicAEADChaCha20Poly1305
		default:
			return protocolError(ErrUnsupportedFeature, "QUIC traffic cipher unsupported")
		}
		secrets := map[string][]byte{}
		for _, label := range []string{"CLIENT_HANDSHAKE_TRAFFIC_SECRET", "SERVER_HANDSHAKE_TRAFFIC_SECRET", "CLIENT_TRAFFIC_SECRET_0", "SERVER_TRAFFIC_SECRET_0"} {
			if v, ok := q.provider.LookupTLSSecret(label, q.tls.random); ok {
				secrets[label] = v
			}
		}
		ring, err := quicLoadKeys(QUICKeyMaterial{Secrets: secrets, AEAD: aead})
		if err != nil {
			return err
		}
		ring.initial = q.keys.initial
		q.keys = ring
		q.loadedKeys = len(secrets) == 4
	}
	return nil
}
