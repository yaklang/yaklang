package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
)

// These are observations in one TLS connection, not a resumption cache. In
// particular, a selected PSK can be external and a legacy session-ID echo does
// not authenticate a previous session. Never retain raw tickets or binders here.
type tlsResumptionState struct{ context map[string]any }

func (s tlsResumptionState) storage() int64 {
	if len(s.context) == 0 {
		return 0
	}
	charge := func(m map[string]any) int64 {
		n := int64(256 * (1 + len(m)))
		for k, v := range m {
			n += int64(len(k))
			if text, ok := v.(string); ok {
				n += int64(len(text))
			}
		}
		return n
	}
	n := charge(s.context)
	if selection, ok := s.context["PSK Selection"].(map[string]any); ok {
		n += charge(selection)
	}
	return n
}
func (s tlsResumptionState) project(info map[string]any) {
	if len(s.context) > 0 {
		info["Session Establishment"] = cloneSession(s.context)
	}
}

func tlsOpaqueMetadata(b []byte) map[string]any {
	sum := sha256.Sum256(b)
	return map[string]any{"Length": len(b), "SHA256": hex.EncodeToString(sum[:])}
}

type tlsPSKIdentity struct {
	identity     []byte // borrowed only while validating the current message
	age          uint32
	binderLength int
}

type tlsPSKOffer struct {
	identities                      []tlsPSKIdentity
	identitiesLength, bindersLength int
}

// Validate the entire identities/binders grammar before applying collection
// limits or allocating result entries (RFC 8446 4.2.11). A short binder is a
// malformed vector, not merely an exhausted collection budget.
func tlsPSKOfferStructure(v []byte) (int, int, int, error) {
	bad := func() (int, int, int, error) {
		return 0, 0, 0, protocolError(ErrMalformedMessage, "TLS PSK identities/binders vector")
	}
	if len(v) < 2 {
		return bad()
	}
	n := int(binary.BigEndian.Uint16(v))
	if n < 7 || n > len(v)-2 {
		return bad()
	}
	end, count := 2+n, 0
	for p := 2; p < end; {
		if end-p < 2 {
			return bad()
		}
		l := int(binary.BigEndian.Uint16(v[p:]))
		p += 2
		if l == 0 || l > end-p-4 {
			return bad()
		}
		p += l + 4
		count++
	}
	if len(v)-end < 2 {
		return bad()
	}
	bn := int(binary.BigEndian.Uint16(v[end:]))
	p := end + 2
	if bn < 33 || bn != len(v)-p {
		return bad()
	}
	binders := 0
	for p < len(v) {
		l := int(v[p])
		p++
		if l < 32 || l > len(v)-p {
			return bad()
		}
		p += l
		binders++
	}
	if binders != count {
		return bad()
	}
	return count, n, bn, nil
}

func tlsPSKOfferValue(v []byte, limits ...int) (*tlsPSKOffer, error) {
	count, n, bn, err := tlsPSKOfferStructure(v)
	if err != nil {
		return nil, err
	}
	if len(limits) > 0 && count > limits[0] {
		return nil, protocolError(ErrResourceExceeded, "TLS PSK identity count")
	}
	o := &tlsPSKOffer{identities: make([]tlsPSKIdentity, count), identitiesLength: n, bindersLength: bn}
	for p, i := 2, 0; p < 2+n; i++ {
		l := int(binary.BigEndian.Uint16(v[p:]))
		p += 2
		o.identities[i] = tlsPSKIdentity{identity: v[p : p+l], age: binary.BigEndian.Uint32(v[p+l:])}
		p += l + 4
	}
	for p, i := 2+n+2, 0; p < len(v); i++ {
		l := int(v[p])
		p++
		o.identities[i].binderLength = l
		p += l
	}
	return o, nil
}

func (o *tlsPSKOffer) metadata() map[string]any {
	entries := make([]map[string]any, len(o.identities))
	for i, p := range o.identities {
		m := tlsOpaqueMetadata(p.identity)
		m["Index"], m["Obfuscated Ticket Age"], m["Binder Length"] = i, p.age, p.binderLength
		entries[i] = m
	}
	return map[string]any{"Identities Length": o.identitiesLength, "Binders Length": o.bindersLength,
		"Identities": entries, "Binder Verified": false, "Identity Origin": "external PSK or session ticket; unconfirmed"}
}

func tlsPSKModes(v []byte) error {
	if len(v) < 2 || int(v[0]) != len(v)-1 {
		return protocolError(ErrMalformedMessage, "TLS PSK key exchange modes vector")
	}
	// Unknown enum values are allowed for extensibility; a selected known
	// mode still has to be present in the actual client vector.
	return nil
}

// No allocation or state mutation: usable in tlsHello's framing pass before
// extension/inner-collection admission. Cross-extension checks follow below.
func tlsResumptionExtensionStructure(kind uint16, v []byte, client, last bool) error {
	switch kind {
	case 41:
		if client {
			if _, _, _, err := tlsPSKOfferStructure(v); err != nil {
				return err
			}
			if !last {
				return protocolError(ErrMalformedMessage, "TLS client PSK extension is not last")
			}
		} else if len(v) != 2 {
			return protocolError(ErrMalformedMessage, "TLS selected PSK identity length")
		}
	case 45:
		if !client {
			return protocolError(ErrMalformedMessage, "TLS server PSK key exchange modes")
		}
		return tlsPSKModes(v)
	case 42:
		if !client || len(v) != 0 {
			return protocolError(ErrMalformedMessage, "TLS early_data hello extension placement or length")
		}
	case 35:
		if !client && len(v) != 0 {
			return protocolError(ErrMalformedMessage, "TLS server session ticket extension length")
		}
	}
	return nil
}

func tlsHelloPSK(h *tlsHelloView, client bool, limits ...int) (*tlsPSKOffer, error) {
	for i, kind := range h.order {
		if err := tlsResumptionExtensionStructure(kind, h.ext[kind], client, i == len(h.order)-1); err != nil {
			return nil, err
		}
	}
	v, present := h.ext[41]
	if !client || !present {
		return nil, nil
	}
	if err := tlsPSKClientRequirements(h.ext[43], h.ext[45]); err != nil {
		return nil, err
	}
	if len(limits) > 0 && len(h.ext[45])-1 > limits[0] {
		return nil, protocolError(ErrResourceExceeded, "TLS PSK key exchange mode count")
	}
	return tlsPSKOfferValue(v, limits...)
}

func tlsPSKClientRequirements(versions, modes []byte) error {
	if modes == nil {
		return protocolError(ErrMalformedMessage, "TLS PSK offer requires key exchange modes")
	}
	if len(versions) < 3 || int(versions[0]) != len(versions)-1 || (len(versions)-1)%2 != 0 || !tlsContains16(versions[1:], 0x304) {
		return protocolError(ErrMalformedMessage, "TLS PSK offer requires TLS 1.3 supported version")
	}
	return nil
}

func tlsPSKHashSize(suite uint16) int {
	switch suite {
	case 0x1302:
		return 48
	case 0x1301, 0x1303, 0x1304, 0x1305:
		return 32
	}
	return 0
}

func tlsPSKRetryCompatible(first, second *tlsPSKOffer, suite uint16) error {
	if first == nil && second == nil {
		return nil
	}
	if first == nil {
		return protocolError(ErrMalformedMessage, "TLS retry added PSK identities")
	}
	// Identities may be removed after HRR and their ages/binders recomputed;
	// retained identities keep their order and hash (hence binder length).
	at := 0
	want := tlsPSKHashSize(suite)
	var identities []tlsPSKIdentity
	if second != nil {
		identities = second.identities
	}
	for _, p := range identities {
		for at < len(first.identities) && !bytes.Equal(first.identities[at].identity, p.identity) {
			if want != 0 && first.identities[at].binderLength == want {
				return protocolError(ErrMalformedMessage, "TLS retry removed compatible PSK identity")
			}
			at++
		}
		if at == len(first.identities) || first.identities[at].binderLength != p.binderLength {
			return protocolError(ErrMalformedMessage, "TLS retry changed or reordered PSK identities")
		}
		at++
	}
	for ; at < len(first.identities); at++ {
		if want != 0 && first.identities[at].binderLength == want {
			return protocolError(ErrMalformedMessage, "TLS retry removed compatible PSK identity")
		}
	}
	return nil
}

func (t *binTLS) observeResumptionHello(dir int, typ byte, body []byte, h *tlsHelloView, m map[string]any, limits ...int) error {
	client := typ == 1
	offer, err := tlsHelloPSK(h, client, limits...)
	if err != nil {
		return err
	}
	m["Session ID"] = tlsOpaqueMetadata(h.session)
	if client {
		if modes, present := h.ext[45]; present {
			if len(limits) > 0 && len(modes)-1 > limits[0] {
				return protocolError(ErrResourceExceeded, "TLS PSK key exchange mode count")
			}
			m["PSK Key Exchange Modes Length"] = len(modes) - 1
			m["PSK Key Exchange Modes"] = bytes.Clone(modes[1:])
		}
		if offer != nil {
			if binary.BigEndian.Uint16(body) != 0x303 || !bytes.Equal(h.compression, []byte{0}) {
				return protocolError(ErrMalformedMessage, "TLS 1.3 PSK ClientHello legacy parameters")
			}
			m["PSK Offer"] = offer.metadata()
		}
		if v, present := h.ext[35]; present && len(v) > 0 {
			m["Session Ticket Offer"] = tlsOpaqueMetadata(v)
		}
		if t.retrySeen && !t.retryClientSeen && t.firstHello != nil {
			first, err := tlsHello(t.firstHello, true, limits...)
			if err != nil {
				return err
			}
			old, err := tlsHelloPSK(first, true, limits...)
			if err != nil {
				return err
			}
			if err = tlsPSKRetryCompatible(old, offer, t.retrySuite); err != nil {
				return err
			}
		}
		// A new valid ClientHello starts a fresh observation scope. The outer
		// transaction discards this reset if renegotiation/HRR checks fail.
		t.resumption.context = nil
		return nil
	}
	version := binary.BigEndian.Uint16(body)
	if v, ok := h.ext[43]; ok {
		if len(v) != 2 {
			return protocolError(ErrMalformedMessage, "TLS selected version length")
		}
		version = binary.BigEndian.Uint16(v)
	}
	var current *tlsHelloView
	if t.firstHello != nil && t.client >= 0 {
		if dir == t.client {
			return protocolError(ErrMalformedMessage, "TLS ServerHello from client direction")
		}
		current, err = tlsHello(t.firstHello, true, limits...)
		if err != nil {
			return err
		}
	}
	ctx := map[string]any{"Evidence Scope": "current connection hello observations", "Certificate Trust": "not-evaluated"}
	if current != nil {
		echo := bytes.Equal(current.session, h.session)
		ctx["Session ID Echo"] = echo
		if version >= 0x300 && version <= 0x303 {
			ctx["Session ID Resumption Indicated"] = len(current.session) > 0 && len(h.session) > 0 && echo
		} else if version == 0x304 {
			ctx["Compatibility Session ID Echo"] = echo
		}
	}
	selected, present := h.ext[41]
	if version == 0x304 && current != nil {
		versions := current.ext[43]
		if len(versions) < 3 || int(versions[0]) != len(versions)-1 || (len(versions)-1)%2 != 0 || !tlsContains16(versions[1:], version) {
			return protocolError(ErrMalformedMessage, "TLS selected version was not offered")
		}
		if binary.BigEndian.Uint16(body) != 0x303 || !bytes.Equal(h.session, current.session) || h.compression[0] != 0 {
			return protocolError(ErrMalformedMessage, "TLS 1.3 ServerHello legacy parameters")
		}
	}
	if present {
		if version != 0x304 || bytes.Equal(body[2:34], tlsRetryRandom[:]) {
			return protocolError(ErrMalformedMessage, "TLS PSK selection outside final TLS 1.3 ServerHello")
		}
		index := binary.BigEndian.Uint16(selected)
		selection := map[string]any{"Selected Identity": index, "Offer Match": "unavailable-client-hello", "Binder Verified": false,
			"Ticket Association": "unconfirmed; selected PSK may be external", "Evidence Scope": "current ServerHello"}
		if current != nil {
			offer, err := tlsHelloPSK(current, true, limits...)
			if err != nil {
				return err
			}
			if offer == nil || int(index) >= len(offer.identities) {
				return protocolError(ErrMalformedMessage, "TLS selected PSK index was not offered")
			}
			id := offer.identities[index]
			want := tlsPSKHashSize(h.suite)
			if want != 0 && id.binderLength != want {
				return protocolError(ErrMalformedMessage, "TLS selected PSK binder hash length disagrees with cipher suite")
			}
			mode := byte(0)
			if share, hasShare := h.ext[51]; hasShare {
				mode = 1
				if len(share) < 5 || int(binary.BigEndian.Uint16(share[2:])) != len(share)-4 {
					return protocolError(ErrMalformedMessage, "TLS selected PSK key_share length")
				}
				shares, err := tlsClientShares(current.ext[51])
				if err != nil {
					return err
				}
				if !shares[binary.BigEndian.Uint16(share)] {
					return protocolError(ErrMalformedMessage, "TLS selected PSK key_share was not offered")
				}
			}
			if bytes.IndexByte(current.ext[45][1:], mode) < 0 {
				return protocolError(ErrMalformedMessage, "TLS selected PSK key exchange mode was not offered")
			}
			for k, v := range tlsOpaqueMetadata(id.identity) {
				selection[k] = v
			}
			selection["Offer Match"], selection["Key Exchange Mode"], selection["Binder Length"] = "matched-current-client-hello", mode, id.binderLength
			selection["Obfuscated Ticket Age"] = id.age
		}
		m["PSK Selection"] = selection
		ctx["PSK Selection"] = cloneSession(selection)
		ctx["PSK Selected"] = true
	} else if version == 0x304 && current != nil && !bytes.Equal(body[2:34], tlsRetryRandom[:]) {
		ctx["PSK Selected"] = false
	}
	if len(ctx) > 2 {
		t.resumption.context = ctx
	}
	return nil
}

func (t *binTLS) newSessionTicket(dir int, body []byte, authenticated bool, limits ...int) (map[string]any, error) {
	bad := func(why string) (map[string]any, error) {
		return nil, protocolError(ErrMalformedMessage, "TLS NewSessionTicket "+why)
	}
	if t.version == 0 {
		return nil, protocolError(ErrContextRequired, "TLS NewSessionTicket requires observed negotiated version")
	}
	if len(body) < 6 {
		return bad("length")
	}
	m := map[string]any{"Lifetime Hint": binary.BigEndian.Uint32(body), "Record Authentication Verified": authenticated,
		"Ticket Association": "not tracked across connections", "Evidence Scope": "this NewSessionTicket message"}
	if t.version != 0x304 {
		if t.version < 0x301 || t.version > 0x303 {
			return nil, protocolError(ErrUnsupportedFeature, "TLS NewSessionTicket version")
		}
		n := int(binary.BigEndian.Uint16(body[4:]))
		if n != len(body)-6 {
			return bad("ticket length")
		}
		m["Ticket"] = tlsOpaqueMetadata(body[6:])
	} else {
		if len(body) < 13 {
			return bad("TLS 1.3 length")
		}
		lifetime := binary.BigEndian.Uint32(body)
		m["Lifetime Within Server Limit"] = lifetime <= 604800
		// Preserve the exact wire value. This is a specification diagnostic,
		// not a ticket cache or a claim that this ticket was accepted.
		if lifetime > 604800 {
			m["Lifetime Diagnostic"] = "server lifetime exceeds RFC 8446 seven-day limit; ticket acceptance is unconfirmed"
		}
		m["Maximum Client Cache Seconds"] = lifetime
		if lifetime > 604800 {
			m["Maximum Client Cache Seconds"] = uint32(604800)
		}
		m["Age Add"] = binary.BigEndian.Uint32(body[4:])
		nonceLength := int(body[8])
		p := 9
		if nonceLength > len(body)-p-4 {
			return bad("nonce length")
		}
		m["Nonce"] = tlsOpaqueMetadata(body[p : p+nonceLength])
		p += nonceLength
		n := int(binary.BigEndian.Uint16(body[p:]))
		p += 2
		if n == 0 || n > len(body)-p-2 {
			return bad("ticket length")
		}
		m["Ticket"] = tlsOpaqueMetadata(body[p : p+n])
		p += n
		if len(body)-p < 2 || int(binary.BigEndian.Uint16(body[p:])) != len(body)-p-2 {
			return bad("extensions length")
		}
		p += 2
		start, count := p, 0
		var seen [1024]uint64
		for p < len(body) {
			if len(body)-p < 4 {
				return bad("extension header")
			}
			kind, n := binary.BigEndian.Uint16(body[p:]), int(binary.BigEndian.Uint16(body[p+2:]))
			p += 4
			bit := uint64(1) << (kind % 64)
			if n > len(body)-p || seen[kind/64]&bit != 0 {
				return bad("duplicate or truncated extension")
			}
			seen[kind/64] |= bit
			count++
			if kind == 42 && n != 4 {
				return bad("early_data length")
			}
			p += n
		}
		if err := t.newSessionTicketContext(dir, authenticated); err != nil {
			return nil, err
		}
		if len(limits) > 0 && count > limits[0] {
			return nil, protocolError(ErrResourceExceeded, "TLS NewSessionTicket extension count")
		}
		exts := make([]map[string]any, 0, count)
		for p = start; p < len(body); {
			kind, n := binary.BigEndian.Uint16(body[p:]), int(binary.BigEndian.Uint16(body[p+2:]))
			p += 4
			e := map[string]any{"Type": kind, "Length": n}
			if kind == 42 {
				e["Maximum Early Data Size"] = binary.BigEndian.Uint32(body[p:])
			}
			exts = append(exts, e)
			p += n
		}
		m["Extensions"] = exts
	}
	if err := t.newSessionTicketContext(dir, authenticated); err != nil {
		return nil, err
	}
	return m, nil
}

func (t *binTLS) newSessionTicketContext(dir int, authenticated bool) error {
	if t.client < 0 {
		return protocolError(ErrContextRequired, "TLS NewSessionTicket server direction is unavailable")
	}
	if dir == t.client {
		return protocolError(ErrMalformedMessage, "TLS NewSessionTicket from client direction")
	}
	if t.version == 0x304 {
		if !authenticated {
			return protocolError(ErrMalformedMessage, "TLS 1.3 NewSessionTicket is not authenticated")
		}
		if !t.app[dir] {
			return protocolError(ErrContextRequired, "TLS 1.3 NewSessionTicket requires server application epoch")
		}
	}
	return nil
}
