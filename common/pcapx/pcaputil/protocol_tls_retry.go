package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
)

var tlsRetryRandom = sha256.Sum256([]byte("HelloRetryRequest"))

type tlsHelloView struct {
	prefix, session, suites, compression []byte
	suite                                uint16
	ext                                  map[uint16][]byte
	order                                []uint16
}

func tlsHello(body []byte, client bool, limits ...int) (*tlsHelloView, error) {
	bad := func() (*tlsHelloView, error) { return nil, protocolError(ErrMalformedMessage, "TLS hello structure") }
	if len(body) < 35 || body[34] > 32 {
		return bad()
	}
	p := 35 + int(body[34])
	if p > len(body) {
		return bad()
	}
	h := &tlsHelloView{session: body[35:p]}
	if client {
		if p+2 > len(body) {
			return bad()
		}
		n := int(binary.BigEndian.Uint16(body[p:]))
		p += 2
		if n < 2 || n%2 != 0 || n > len(body)-p {
			return bad()
		}
		h.suites = body[p : p+n]
		p += n
		if p == len(body) {
			return bad()
		}
		n = int(body[p])
		p++
		if n == 0 || n > len(body)-p {
			return bad()
		}
		h.compression = body[p : p+n]
		p += n
	} else {
		if len(body)-p < 3 {
			return bad()
		}
		h.suite = binary.BigEndian.Uint16(body[p:])
		h.compression = body[p+2 : p+3]
		p += 3
	}
	h.prefix = body[:p]
	if p == len(body) {
		h.ext = map[uint16][]byte{}
		return h, nil
	}
	if len(body)-p < 2 || int(binary.BigEndian.Uint16(body[p:])) != len(body)-p-2 {
		return bad()
	}
	p += 2
	if len(limits) > 0 {
		// Validate the whole extension vector before admitting its collection.
		// The fixed bitset detects duplicate 16-bit types without allocating
		// the output map/order or retaining a rejected Hello as retry state.
		var seen [1024]uint64
		count := 0
		var psk, versions, modes []byte
		for q := p; q < len(body); {
			if len(body)-q < 4 {
				return bad()
			}
			kind, n := binary.BigEndian.Uint16(body[q:]), int(binary.BigEndian.Uint16(body[q+2:]))
			q += 4
			bit := uint64(1) << (kind % 64)
			if n > len(body)-q || seen[kind/64]&bit != 0 {
				return bad()
			}
			if err := tlsHelloExtensionDataStructure(kind, body[q:q+n], client, q+n == len(body)); err != nil {
				return nil, err
			}
			seen[kind/64] |= bit
			switch kind {
			case 41:
				psk = body[q : q+n]
			case 43:
				versions = body[q : q+n]
			case 45:
				modes = body[q : q+n]
			}
			count++
			q += n
		}
		if client && psk != nil {
			if err := tlsPSKClientRequirements(versions, modes); err != nil {
				return nil, err
			}
			if binary.BigEndian.Uint16(body) != 0x303 || !bytes.Equal(h.compression, []byte{0}) {
				return nil, protocolError(ErrMalformedMessage, "TLS 1.3 PSK ClientHello legacy parameters")
			}
		}
		if count > limits[0] {
			return nil, protocolError(ErrResourceExceeded, "TLS hello extension count")
		}
	}
	h.ext = map[uint16][]byte{}
	for p < len(body) {
		if len(body)-p < 4 {
			return bad()
		}
		kind, n := binary.BigEndian.Uint16(body[p:]), int(binary.BigEndian.Uint16(body[p+2:]))
		p += 4
		if _, exists := h.ext[kind]; exists || n > len(body)-p {
			return bad()
		}
		h.ext[kind] = body[p : p+n]
		h.order = append(h.order, kind)
		p += n
	}
	return h, nil
}

func tlsSNIName(v []byte) ([]byte, error) {
	if len(v) < 5 || int(binary.BigEndian.Uint16(v)) != len(v)-2 || v[2] != 0 || int(binary.BigEndian.Uint16(v[3:])) != len(v)-5 {
		return nil, protocolError(ErrMalformedMessage, "TLS SNI length")
	}
	return v[5:], nil
}

// Validate known extension grammar before applying a new inner collection cap.
// Resource refusal must not hide an existing structural error in another entry.
func tlsHelloExtensionStructure(h *tlsHelloView, client bool) error {
	for i, kind := range h.order {
		if err := tlsHelloExtensionDataStructure(kind, h.ext[kind], client, i == len(h.order)-1); err != nil {
			return err
		}
	}
	return nil
}

func tlsHelloExtensionDataStructure(kind uint16, v []byte, client, last bool) error {
	if err := tlsResumptionExtensionStructure(kind, v, client, last); err != nil {
		return err
	}
	switch kind {
	case 0:
		if client {
			_, err := tlsSNIName(v)
			return err
		}
	case 16:
		_, err := tlsALPNName(v, !client)
		return err
	case 43:
		if !client && len(v) != 2 {
			return protocolError(ErrMalformedMessage, "TLS selected version length")
		}
	}
	return nil
}

// tlsALPNName admits the complete vector before exposing its first name.
// The client advertises a list; the server must select exactly one name.
func tlsALPNName(v []byte, server bool, limits ...int) ([]byte, error) {
	if len(v) < 3 || int(binary.BigEndian.Uint16(v)) != len(v)-2 {
		return nil, protocolError(ErrMalformedMessage, "TLS ALPN vector")
	}
	var first []byte
	count := 0
	for p := 2; p < len(v); {
		n := int(v[p])
		p++
		if n == 0 || n > len(v)-p {
			return nil, protocolError(ErrMalformedMessage, "TLS ALPN length")
		}
		if count == 0 {
			first = v[p : p+n]
		}
		count++
		p += n
	}
	if server && count != 1 {
		return nil, protocolError(ErrMalformedMessage, "TLS selected ALPN count")
	}
	if len(limits) > 0 && count > limits[0] {
		return nil, protocolError(ErrResourceExceeded, "TLS ALPN protocol count")
	}
	return first, nil
}

func tlsContains16(v []byte, n uint16) bool {
	for len(v) >= 2 {
		if binary.BigEndian.Uint16(v) == n {
			return true
		}
		v = v[2:]
	}
	return false
}

func tlsClientShares(v []byte) (map[uint16]bool, error) {
	groups := map[uint16]bool{}
	if v == nil {
		return groups, nil
	}
	if len(v) < 2 || int(binary.BigEndian.Uint16(v)) != len(v)-2 {
		return nil, protocolError(ErrMalformedMessage, "TLS client key_share length")
	}
	v = v[2:]
	for len(v) > 0 {
		if len(v) < 4 {
			return nil, protocolError(ErrMalformedMessage, "TLS client key_share entry")
		}
		g, n := binary.BigEndian.Uint16(v), int(binary.BigEndian.Uint16(v[2:]))
		v = v[4:]
		if n == 0 || n > len(v) || groups[g] {
			return nil, protocolError(ErrMalformedMessage, "TLS client key_share entry length")
		}
		groups[g] = true
		v = v[n:]
	}
	return groups, nil
}

// Validate the HRR branch separately from the final ServerHello. This remains a
// passive record decoder, not a certificate or PSK-binder verifier (RFC 8446 4.1).
func (t *binTLS) observeHello(dir int, typ byte, body []byte, limits ...int) (bool, error) {
	h, err := tlsHello(body, typ == 1, limits...)
	if err != nil {
		return false, err
	}
	bad := func(why string) (bool, error) { return false, protocolError(ErrMalformedMessage, "TLS retry: "+why) }
	if err := tlsHelloExtensionStructure(h, typ == 1); err != nil {
		return false, err
	}
	if alpn, present := h.ext[16]; present {
		if _, err := tlsALPNName(alpn, typ == 2, limits...); err != nil {
			return false, err
		}
	}
	if typ == 1 {
		if t.retrySeen {
			if t.retryClientSeen || dir != t.client {
				return bad("unexpected ClientHello2")
			}
			first, err := tlsHello(t.firstHello, true, limits...)
			if err != nil {
				return false, err
			}
			if !bytes.Equal(first.prefix, h.prefix) {
				return bad("ClientHello2 fixed fields changed")
			}
			if h.ext[42] != nil {
				return bad("early_data after HelloRetryRequest")
			}
			if !bytes.Equal(h.ext[44], t.retryCookie) {
				return bad("cookie mismatch")
			}
			if t.retryGroup != 0 {
				shares, err := tlsClientShares(h.ext[51])
				if err != nil {
					return false, err
				}
				if len(shares) != 1 || !shares[t.retryGroup] {
					return bad("ClientHello2 selected group mismatch")
				}
			} else if !bytes.Equal(first.ext[51], h.ext[51]) {
				return bad("unsolicited key_share change")
			}
			stable := func(v *tlsHelloView) []uint16 {
				var out []uint16
				for _, k := range v.order {
					if k != 21 && k != 41 && k != 42 && k != 44 && k != 51 {
						out = append(out, k)
					}
				}
				return out
			}
			x, y := stable(first), stable(h)
			if len(x) != len(y) {
				return bad("ClientHello2 extension set changed")
			}
			for i, k := range x {
				if k != y[i] || !bytes.Equal(first.ext[k], h.ext[k]) {
					return bad("ClientHello2 extension changed")
				}
			}
			for _, c := range h.ext[21] {
				if c != 0 {
					return bad("invalid padding")
				}
			}
			t.retryClientSeen = true
			// Final ServerHello PSK indices refer to ClientHello2, which may
			// remove incompatible identities and recompute ages/binders.
			t.firstHello = bytes.Clone(body)
		} else if t.firstHello == nil {
			t.firstHello = bytes.Clone(body)
		}
		return false, nil
	}
	retry := bytes.Equal(body[2:34], tlsRetryRandom[:])
	if !retry && !t.retrySeen && t.firstHello != nil && t.client >= 0 {
		// A complete owned ClientHello makes these observations provable.
		// Server-only captures retain their existing partial-context behavior.
		first, err := tlsHello(t.firstHello, true, limits...)
		if err != nil {
			return false, err
		}
		if dir == t.client {
			return bad("ServerHello from observed client direction")
		}
		if !tlsContains16(first.suites, h.suite) {
			return bad("ServerHello cipher was not offered")
		}
		if bytes.IndexByte(first.compression, h.compression[0]) < 0 {
			return bad("ServerHello compression was not offered")
		}
		if binary.BigEndian.Uint16(body) > binary.BigEndian.Uint16(t.firstHello) {
			return bad("ServerHello version exceeds observed client version")
		}
		if alpn, present := h.ext[16]; present {
			selected, err := tlsALPNName(alpn, true, limits...)
			if err != nil {
				return false, err
			}
			offered, present := first.ext[16]
			if !present {
				return bad("ServerHello ALPN was not offered")
			}
			if _, err := tlsALPNName(offered, false, limits...); err != nil {
				return false, err
			}
			found := false
			for v := offered[2:]; len(v) > 0; {
				n := int(v[0])
				v = v[1:]
				found = found || bytes.Equal(v[:n], selected)
				v = v[n:]
			}
			if !found {
				return bad("ServerHello ALPN was not offered")
			}
		}
	}
	if retry || t.retrySeen {
		if t.firstHello == nil || t.client < 0 {
			return false, protocolError(ErrContextRequired, "TLS HRR requires first ClientHello")
		}
		first, err := tlsHello(t.firstHello, true, limits...)
		if err != nil {
			return false, err
		}
		if dir == t.client || binary.BigEndian.Uint16(body) != 0x303 || h.prefix[len(h.prefix)-1] != 0 || !bytes.Equal(h.session, first.session) || !tlsContains16(first.suites, h.suite) || !bytes.Equal(h.ext[43], []byte{3, 4}) {
			return bad("ServerHello parameters")
		}
		if retry {
			if t.retrySeen {
				return bad("repeated HelloRetryRequest")
			}
			versions := first.ext[43]
			if len(versions) < 3 || int(versions[0]) != len(versions)-1 || (len(versions)-1)%2 != 0 || !tlsContains16(versions[1:], 0x304) {
				return bad("TLS 1.3 was not offered")
			}
			for k := range h.ext {
				if k != 43 && k != 44 && k != 51 {
					return bad("HelloRetryRequest extension")
				}
			}
			cookie, cookiePresent := h.ext[44]
			if cookiePresent && (len(cookie) < 3 || int(binary.BigEndian.Uint16(cookie)) != len(cookie)-2) {
				return bad("cookie length")
			}
			share, sharePresent := h.ext[51]
			if !cookiePresent && !sharePresent {
				return bad("HelloRetryRequest makes no change")
			}
			if sharePresent {
				if len(share) != 2 {
					return bad("selected group length")
				}
				group := binary.BigEndian.Uint16(share)
				groups := first.ext[10]
				if len(groups) < 2 || int(binary.BigEndian.Uint16(groups)) != len(groups)-2 || (len(groups)-2)%2 != 0 || !tlsContains16(groups[2:], group) {
					return bad("selected group was not offered")
				}
				shares, err := tlsClientShares(first.ext[51])
				if err != nil {
					return false, err
				}
				if shares[group] {
					return bad("selected share was already offered")
				}
				t.retryGroup = group
			}
			t.retrySeen, t.retrySuite = true, h.suite
			t.retryCookie = bytes.Clone(cookie)
			return true, nil
		}
		if !t.retryClientSeen || h.suite != t.retrySuite {
			return bad("final ServerHello before ClientHello2 or different suite")
		}
		if _, ok := h.ext[44]; ok {
			return bad("cookie in final ServerHello")
		}
		if t.retryGroup != 0 {
			share := h.ext[51]
			if len(share) < 5 || binary.BigEndian.Uint16(share) != t.retryGroup || int(binary.BigEndian.Uint16(share[2:])) != len(share)-4 {
				return bad("final ServerHello key_share")
			}
		}
	}
	t.finalHello = true
	t.firstHello, t.retryCookie = nil, nil
	return false, nil
}
