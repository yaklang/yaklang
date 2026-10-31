package pcaputil

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type tlsResumptionExt struct {
	kind uint16
	data []byte
}

func tlsResumptionExtensions(exts ...tlsResumptionExt) []byte {
	var out []byte
	for _, e := range exts {
		out = binary.BigEndian.AppendUint16(out, e.kind)
		out = binary.BigEndian.AppendUint16(out, uint16(len(e.data)))
		out = append(out, e.data...)
	}
	return out
}

func tlsResumptionHello(client bool, sid []byte, suite uint16, exts ...tlsResumptionExt) []byte {
	b := []byte{3, 3}
	random := byte(0x52)
	if client {
		random = 0x31
	}
	b = append(b, bytes.Repeat([]byte{random}, 32)...)
	b = append(b, byte(len(sid)))
	b = append(b, sid...)
	if client {
		b = append(b, 0, 2, byte(suite>>8), byte(suite), 1, 0)
	} else {
		b = append(b, byte(suite>>8), byte(suite), 0)
	}
	ext := tlsResumptionExtensions(exts...)
	b = binary.BigEndian.AppendUint16(b, uint16(len(ext)))
	return append(b, ext...)
}

func tlsResumptionPSKs(ids [][]byte, ages []uint32, lengths []int) []byte {
	var identities, binders []byte
	for i, id := range ids {
		identities = binary.BigEndian.AppendUint16(identities, uint16(len(id)))
		identities = append(identities, id...)
		identities = binary.BigEndian.AppendUint32(identities, ages[i])
		binders = append(binders, byte(lengths[i]))
		binders = append(binders, bytes.Repeat([]byte{byte(i + 1)}, lengths[i])...)
	}
	b := binary.BigEndian.AppendUint16(nil, uint16(len(identities)))
	b = append(b, identities...)
	b = binary.BigEndian.AppendUint16(b, uint16(len(binders)))
	return append(b, binders...)
}

func tlsResumptionPSKClient(sid, offer []byte, modes ...byte) []byte {
	v := append([]byte{byte(len(modes))}, modes...)
	return tlsResumptionHello(true, sid, 0x1301, tlsResumptionExt{43, []byte{2, 3, 4}}, tlsResumptionExt{45, v}, tlsResumptionExt{41, offer})
}

func tlsResumptionPSKServer(sid []byte, index uint16, exts ...tlsResumptionExt) []byte {
	ext := []tlsResumptionExt{{43, []byte{3, 4}}, {41, []byte{byte(index >> 8), byte(index)}}}
	return tlsResumptionHello(false, sid, 0x1301, append(ext, exts...)...)
}

func tlsResumptionContext(t *testing.T, s *binTLS) map[string]any {
	t.Helper()
	m := map[string]any{}
	s.resumption.project(m)
	v, ok := m["Session Establishment"].(map[string]any)
	require.True(t, ok)
	return v
}

func TestTLSResumptionSessionIDEvidenceIsVersionScoped(t *testing.T) {
	for _, client := range []int{0, 1} {
		for _, version := range []uint16{0x303, 0x304} {
			for _, sid := range [][]byte{nil, bytes.Repeat([]byte{0xa5}, 32)} {
				t.Run(fmt.Sprintf("client%d/version%x/sid%d", client, version, len(sid)), func(t *testing.T) {
					s := newBinTLS()
					suite := uint16(0xc02b)
					var ch, sh []tlsResumptionExt
					if version == 0x304 {
						suite = 0x1301
						ch = []tlsResumptionExt{{43, []byte{2, 3, 4}}}
						sh = []tlsResumptionExt{{43, []byte{3, 4}}}
					}
					m, err := s.handshakeMessage(client, reliabilityHS(1, tlsResumptionHello(true, sid, suite, ch...)), false)
					require.NoError(t, err)
					require.Equal(t, len(sid), m["Session ID"].(map[string]any)["Length"])
					_, err = s.handshakeMessage(1-client, reliabilityHS(2, tlsResumptionHello(false, sid, suite, sh...)), false)
					require.NoError(t, err)
					ctx := tlsResumptionContext(t, s)
					require.Equal(t, true, ctx["Session ID Echo"])
					if version == 0x303 {
						require.Equal(t, len(sid) > 0, ctx["Session ID Resumption Indicated"])
						require.NotContains(t, ctx, "PSK Selected")
					} else {
						require.NotContains(t, ctx, "Session ID Resumption Indicated")
						require.Equal(t, true, ctx["Compatibility Session ID Echo"])
						require.Equal(t, false, ctx["PSK Selected"])
					}
					ctx["Session ID Echo"] = false
					require.Equal(t, true, tlsResumptionContext(t, s)["Session ID Echo"], "caller edits cannot change retained context")
				})
			}
		}
	}
	s := newBinTLS()
	_, err := s.handshakeMessage(0, reliabilityHS(1, tlsResumptionHello(true, []byte{1}, 0xc02b)), false)
	require.NoError(t, err)
	_, err = s.handshakeMessage(1, reliabilityHS(2, tlsResumptionHello(false, []byte{2}, 0xc02b)), false)
	require.NoError(t, err)
	require.Equal(t, false, tlsResumptionContext(t, s)["Session ID Resumption Indicated"])
}

func TestTLSResumptionPSKSelectionPreservesUncertaintyAndIsolation(t *testing.T) {
	id := []byte("private-ticket-identity-not-for-output")
	offer := tlsResumptionPSKs([][]byte{id}, []uint32{0xfedcba98}, []int{32})
	s := newBinTLS()
	ch, err := s.handshakeMessage(0, reliabilityHS(1, tlsResumptionPSKClient([]byte{4}, offer, 0)), false, 8)
	require.NoError(t, err)
	sh, err := s.handshakeMessage(1, reliabilityHS(2, tlsResumptionPSKServer([]byte{4}, 0)), false, 8)
	require.NoError(t, err)
	p := sh["PSK Selection"].(map[string]any)
	require.Equal(t, "matched-current-client-hello", p["Offer Match"])
	require.Equal(t, false, p["Binder Verified"])
	require.Equal(t, "unconfirmed; selected PSK may be external", p["Ticket Association"])
	require.Equal(t, tlsOpaqueMetadata(id)["SHA256"], p["SHA256"])
	require.Equal(t, uint32(0xfedcba98), p["Obfuscated Ticket Age"])
	encoded, err := json.Marshal([]map[string]any{ch, sh})
	require.NoError(t, err)
	require.NotContains(t, string(encoded), string(id))
	require.NotContains(t, string(encoded), "01010101010101010101010101010101")
	clear(p)
	require.Equal(t, true, tlsResumptionContext(t, s)["PSK Selected"])
	require.Equal(t, "matched-current-client-hello", tlsResumptionContext(t, s)["PSK Selection"].(map[string]any)["Offer Match"])
	fresh := newBinTLS()
	m, err := fresh.handshakeMessage(1, reliabilityHS(2, tlsResumptionPSKServer([]byte{4}, 0)), false, 8)
	require.NoError(t, err)
	unknown := m["PSK Selection"].(map[string]any)
	require.Equal(t, "unavailable-client-hello", unknown["Offer Match"])
	require.NotContains(t, unknown, "SHA256")
	require.NotContains(t, unknown, "Length")
	require.NotContains(t, tlsResumptionContext(t, fresh), "Session ID Echo")
	bare := newBinTLS()
	plain := map[string]any{}
	bare.resumption.project(plain)
	require.Empty(t, plain, "a reused transport tuple starts without another connection's metadata")
}

func TestTLSResumptionUnknownVersionDoesNotInferSessionReuse(t *testing.T) {
	for _, version := range []uint16{0, 0x2ff, 0x305} {
		t.Run(fmt.Sprintf("%x", version), func(t *testing.T) {
			s := newBinTLS()
			sid := []byte{1, 2, 3}
			versions := []byte{4, 3, 4, byte(version >> 8), byte(version)}
			_, err := s.handshakeMessage(0, reliabilityHS(1, tlsResumptionHello(true, sid, 0x1301, tlsResumptionExt{43, versions})), false)
			require.NoError(t, err)
			_, err = s.handshakeMessage(1, reliabilityHS(2, tlsResumptionHello(false, sid, 0x1301, tlsResumptionExt{43, []byte{byte(version >> 8), byte(version)}})), false)
			require.NoError(t, err)
			ctx := tlsResumptionContext(t, s)
			require.Equal(t, true, ctx["Session ID Echo"], "literal byte equality remains observable")
			require.NotContains(t, ctx, "Session ID Resumption Indicated")
			require.NotContains(t, ctx, "Compatibility Session ID Echo")
			require.NotContains(t, ctx, "PSK Selected")
		})
	}
}

func TestTLSResumptionPSKGrammarAndBudgetAreTransactional(t *testing.T) {
	valid := tlsResumptionPSKs([][]byte{{'a'}, {'b'}}, []uint32{1, 2}, []int{32, 32})
	badCount := bytes.Clone(valid)
	badCount = badCount[:len(badCount)-33]
	binary.BigEndian.PutUint16(badCount[2+int(binary.BigEndian.Uint16(badCount)):], 33)
	short := tlsResumptionPSKs([][]byte{{'a'}}, []uint32{1}, []int{31})
	cases := []struct {
		name  string
		body  []byte
		limit int
		kind  string
	}{
		{"valid-over-cap", tlsResumptionPSKClient(nil, valid, 0), 1, "ResourceExceeded"},
		{"binder-count-before-cap", tlsResumptionPSKClient(nil, badCount, 0), 1, "MalformedMessage"},
		{"short-binder-before-cap", tlsResumptionPSKClient(nil, short, 0), 0, "MalformedMessage"},
		{"missing-modes-before-cap", tlsResumptionHello(true, nil, 0x1301, tlsResumptionExt{43, []byte{2, 3, 4}}, tlsResumptionExt{41, valid}), 0, "MalformedMessage"},
		{"empty-modes-before-cap", tlsResumptionPSKClient(nil, valid), 0, "MalformedMessage"},
		{"missing-version-before-cap", tlsResumptionHello(true, nil, 0x1301, tlsResumptionExt{45, []byte{1, 0}}, tlsResumptionExt{41, valid}), 0, "MalformedMessage"},
		{"psk-not-last", tlsResumptionHello(true, nil, 0x1301, tlsResumptionExt{43, []byte{2, 3, 4}}, tlsResumptionExt{45, []byte{1, 0}}, tlsResumptionExt{41, valid}, tlsResumptionExt{21, nil}), 1, "MalformedMessage"},
		{"other-malformed-extension-before-psk-cap", tlsResumptionHello(true, nil, 0x1301, tlsResumptionExt{43, []byte{2, 3, 4}}, tlsResumptionExt{45, []byte{1, 0}}, tlsResumptionExt{0, []byte{0}}, tlsResumptionExt{41, valid}), 1, "MalformedMessage"},
		{"duplicate-psk", tlsResumptionHello(true, nil, 0x1301, tlsResumptionExt{41, valid}, tlsResumptionExt{41, valid}), 0, "MalformedMessage"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newBinTLS()
			before := *s
			_, err := s.handshakeMessage(0, reliabilityHS(1, c.body), false, c.limit)
			rocTypedError(t, c.kind, err)
			require.Equal(t, before, *s, "refused hello cannot publish state")
			_, err = s.handshakeMessage(0, reliabilityHS(1, tlsResumptionPSKClient(nil, valid, 0)), false, 8)
			require.NoError(t, err)
		})
	}
	for _, c := range []struct {
		name  string
		index uint16
		ext   []tlsResumptionExt
		offer []byte
		modes []byte
	}{
		{"index", 2, nil, valid, []byte{0}},
		{"missing-dhe-mode", 0, []tlsResumptionExt{{51, []byte{0, 29, 0, 1, 1}}}, valid, []byte{0}},
		{"dhe-without-client-share", 0, []tlsResumptionExt{{51, []byte{0, 29, 0, 1, 1}}}, valid, []byte{1}},
		{"missing-share", 0, nil, valid, []byte{1}},
		{"hash-size", 0, nil, tlsResumptionPSKs([][]byte{{'a'}}, []uint32{0}, []int{48}), []byte{0}},
	} {
		t.Run("server-"+c.name, func(t *testing.T) {
			s := newBinTLS()
			_, err := s.handshakeMessage(0, reliabilityHS(1, tlsResumptionPSKClient(nil, c.offer, c.modes...)), false, 8)
			require.NoError(t, err)
			before := *s
			_, err = s.handshakeMessage(1, reliabilityHS(2, tlsResumptionPSKServer(nil, c.index, c.ext...)), false, 8)
			rocTypedError(t, "MalformedMessage", err)
			require.Equal(t, before, *s)
		})
	}
}

func TestTLSResumptionRefusedHelloCannotPoisonEstablishedContext(t *testing.T) {
	s := newBinTLS()
	alpn := func(name string) []byte {
		return append([]byte{0, byte(len(name) + 1), byte(len(name))}, []byte(name)...)
	}
	ch := tlsResumptionHello(true, []byte{1}, 0xc02b, tlsResumptionExt{16, alpn("http/1.1")})
	_, err := s.handshakeMessage(0, reliabilityHS(1, ch), false)
	require.NoError(t, err)
	before := *s
	_, err = s.handshakeMessage(1, reliabilityHS(2, tlsResumptionHello(false, []byte{1}, 0xc02b, tlsResumptionExt{16, alpn("other")})), false)
	rocTypedError(t, "MalformedMessage", err)
	require.Equal(t, before, *s)
	_, err = s.handshakeMessage(1, reliabilityHS(2, tlsResumptionHello(false, []byte{1}, 0xc02b, tlsResumptionExt{16, alpn("http/1.1")})), false)
	require.NoError(t, err)
	before = *s
	_, err = s.handshakeMessage(0, reliabilityHS(1, ch), false)
	rocTypedError(t, "UnsupportedFeature", err)
	require.Equal(t, before, *s, "same-random duplicate ClientHello is not a new negotiation")
	_, err = s.handshakeMessage(1, reliabilityHS(2, tlsResumptionHello(false, []byte{2}, 0xc02b)), false)
	rocTypedError(t, "UnsupportedFeature", err)
	require.Equal(t, before, *s)
}

func TestTLSResumptionHRRUsesCurrentOfferAndRejectsIdentityPoisoning(t *testing.T) {
	makeCH := func(group byte, ids [][]byte, lengths []int, cookie bool) []byte {
		offer := tlsResumptionPSKs(ids, make([]uint32, len(ids)), lengths)
		ext := []tlsResumptionExt{{43, []byte{2, 3, 4}}, {10, []byte{0, 4, 0, 29, 0, 23}}, {51, []byte{0, 5, 0, group, 0, 1, 1}}, {45, []byte{1, 1}}}
		if cookie {
			ext = append(ext, tlsResumptionExt{44, []byte{0, 1, 'c'}})
		}
		ext = append(ext, tlsResumptionExt{41, offer})
		return tlsResumptionHello(true, nil, 0x1301, ext...)
	}
	a, b := []byte("sha384-ticket"), []byte("sha256-ticket")
	start := func(t *testing.T) *binTLS {
		s := newBinTLS()
		_, err := s.handshakeMessage(0, reliabilityHS(1, makeCH(29, [][]byte{a, b}, []int{48, 32}, false)), false, 16)
		require.NoError(t, err)
		hrr := tlsResumptionHello(false, nil, 0x1301, tlsResumptionExt{43, []byte{3, 4}}, tlsResumptionExt{51, []byte{0, 23}}, tlsResumptionExt{44, []byte{0, 1, 'c'}})
		copy(hrr[2:34], tlsRetryRandom[:])
		_, err = s.handshakeMessage(1, reliabilityHS(2, hrr), false, 16)
		require.NoError(t, err)
		require.NotContains(t, tlsResumptionContext(t, s), "PSK Selected", "HRR does not decide PSK selection")
		return s
	}
	s := start(t)
	_, err := s.handshakeMessage(0, reliabilityHS(1, makeCH(23, [][]byte{b}, []int{32}, true)), false, 16)
	require.NoError(t, err)
	m, err := s.handshakeMessage(1, reliabilityHS(2, tlsResumptionPSKServer(nil, 0, tlsResumptionExt{51, []byte{0, 23, 0, 1, 1}})), false, 16)
	require.NoError(t, err)
	require.Equal(t, tlsOpaqueMetadata(b)["SHA256"], m["PSK Selection"].(map[string]any)["SHA256"], "index zero refers to ClientHello2, not the original offer")
	for _, c := range []struct {
		name    string
		ids     [][]byte
		lengths []int
	}{
		{"added", [][]byte{a, b, []byte("attacker")}, []int{48, 32, 32}},
		{"reordered", [][]byte{b, a}, []int{32, 48}},
		{"changed-hash", [][]byte{a, b}, []int{32, 32}},
		{"removed-compatible", [][]byte{a}, []int{48}},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := start(t)
			before := *s
			_, err := s.handshakeMessage(0, reliabilityHS(1, makeCH(23, c.ids, c.lengths, true)), false, 16)
			rocTypedError(t, "MalformedMessage", err)
			require.Equal(t, before, *s)
		})
	}
}

func tlsResumptionNST13(lifetime uint32, nonce, ticket []byte, exts ...tlsResumptionExt) []byte {
	b := binary.BigEndian.AppendUint32(nil, lifetime)
	b = binary.BigEndian.AppendUint32(b, 0x10203040)
	b = append(b, byte(len(nonce)))
	b = append(b, nonce...)
	b = binary.BigEndian.AppendUint16(b, uint16(len(ticket)))
	b = append(b, ticket...)
	ext := tlsResumptionExtensions(exts...)
	b = binary.BigEndian.AppendUint16(b, uint16(len(ext)))
	return append(b, ext...)
}

func TestTLSResumptionNewSessionTicketsAreBoundedOpaqueMetadata(t *testing.T) {
	for _, version := range []uint16{0x303, 0x304} {
		for _, size := range []int{0, 1, 192, 65535} {
			if version == 0x304 && size == 0 {
				continue
			}
			t.Run(fmt.Sprintf("%x/%d", version, size), func(t *testing.T) {
				s := newBinTLS()
				s.version, s.client = version, 0
				s.app[1] = true
				ticket := bytes.Repeat([]byte{0xfa}, size)
				wire := binary.BigEndian.AppendUint32(nil, 3600)
				wire = binary.BigEndian.AppendUint16(wire, uint16(size))
				wire = append(wire, ticket...)
				if version == 0x304 {
					wire = tlsResumptionNST13(3600, []byte{1, 2}, ticket, tlsResumptionExt{42, []byte{0, 0, 0x40, 0}})
				}
				before := s.storage()
				m, err := s.handshakeMessage(1, reliabilityHS(4, wire), version == 0x304, 8)
				require.NoError(t, err)
				n := m["New Session Ticket"].(map[string]any)
				require.Equal(t, uint32(3600), n["Lifetime Hint"])
				require.Equal(t, size, n["Ticket"].(map[string]any)["Length"])
				require.Equal(t, tlsOpaqueMetadata(ticket)["SHA256"], n["Ticket"].(map[string]any)["SHA256"])
				require.Equal(t, before, s.storage(), "issuing arbitrarily many tickets cannot grow retained session state")
				if version == 0x304 {
					require.Equal(t, uint32(0x10203040), n["Age Add"])
					require.Equal(t, uint32(16384), n["Extensions"].([]map[string]any)[0]["Maximum Early Data Size"])
				}
			})
		}
	}
	s := newBinTLS()
	s.version, s.client, s.app[1] = 0x304, 0, true
	m, err := s.handshakeMessage(1, reliabilityHS(4, tlsResumptionNST13(0xffffffff, nil, []byte("sensitive-ticket"))), true, 8)
	require.NoError(t, err)
	n := m["New Session Ticket"].(map[string]any)
	require.Equal(t, uint32(0xffffffff), n["Lifetime Hint"])
	require.Equal(t, false, n["Lifetime Within Server Limit"])
	require.Equal(t, uint32(604800), n["Maximum Client Cache Seconds"])
	require.Contains(t, n, "Lifetime Diagnostic")
	encoded, err := json.Marshal(n)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "sensitive-ticket")
}

func TestTLSResumptionNewSessionTicketGrammarContextAndLimits(t *testing.T) {
	valid := tlsResumptionNST13(0, nil, []byte{1}, tlsResumptionExt{42, []byte{0, 0, 0, 0}})
	for _, c := range []struct {
		name        string
		wire        []byte
		limit       int
		auth        bool
		client, dir int
		app         bool
		kind        string
	}{
		{"valid-cap", valid, 0, true, 0, 1, true, "ResourceExceeded"},
		{"short-before-cap", valid[:7], 0, true, 0, 1, true, "MalformedMessage"},
		{"empty-ticket", tlsResumptionNST13(1, nil, nil), 0, true, 0, 1, true, "MalformedMessage"},
		{"bad-early-before-cap", tlsResumptionNST13(1, nil, []byte{1}, tlsResumptionExt{42, []byte{0}}), 0, true, 0, 1, true, "MalformedMessage"},
		{"duplicate-before-cap", tlsResumptionNST13(1, nil, []byte{1}, tlsResumptionExt{42, []byte{0, 0, 0, 0}}, tlsResumptionExt{42, []byte{0, 0, 0, 0}}), 0, true, 0, 1, true, "MalformedMessage"},
		{"unauthenticated", valid, 8, false, 0, 1, true, "MalformedMessage"},
		{"wrong-direction", valid, 8, true, 0, 0, true, "MalformedMessage"},
		{"no-client", valid, 8, true, -1, 1, true, "ContextRequired"},
		{"no-app-epoch", valid, 8, true, 0, 1, false, "ContextRequired"},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := newBinTLS()
			s.version, s.client = 0x304, c.client
			s.app[c.dir] = c.app
			before := *s
			_, err := s.handshakeMessage(c.dir, reliabilityHS(4, c.wire), c.auth, c.limit)
			rocTypedError(t, c.kind, err)
			require.Equal(t, before, *s)
		})
	}
	s := newBinTLS()
	_, err := s.handshakeMessage(1, reliabilityHS(4, valid), true, 8)
	rocTypedError(t, "ContextRequired", err)
	for cut := 0; cut < len(valid); cut++ {
		s := newBinTLS()
		s.version, s.client, s.app[1] = 0x304, 0, true
		_, err := s.handshakeMessage(1, reliabilityHS(4, valid[:cut]), true, 8)
		rocTypedError(t, "MalformedMessage", err)
	}
}

func TestTLSResumptionFragmentAuthenticationRequiresEveryRecord(t *testing.T) {
	a, _ := remainingParser(t)
	s := newBinTLS()
	s.version, s.suite, s.client = 0x303, 0xc02b, 0
	s.random = bytes.Repeat([]byte{0x31}, 32)
	s.serverRandom = bytes.Repeat([]byte{0x52}, 32)
	block, err := aes.NewCipher(make([]byte, 16))
	require.NoError(t, err)
	s.keys[1], err = cipher.NewGCM(block)
	require.NoError(t, err)
	s.iv[1] = make([]byte, 4)
	f := &binFlow{a: a, id: 1, protocol: "tls", tls: s}
	defer f.closeSession()
	body := binary.BigEndian.AppendUint32(nil, 60)
	body = append(body, 0, 1, 0xab)
	hs := reliabilityHS(4, body)
	clearRecord := func(p []byte) []byte {
		h := []byte{22, 3, 3, 0, 0}
		binary.BigEndian.PutUint16(h[3:], uint16(len(p)))
		return append(h, p...)
	}
	_, err = f.consumeTLS(1, &ProtocolEvent{Raw: clearRecord(hs[:5])})
	require.NoError(t, err)
	s.ccs[1] = true
	plain := hs[5:]
	aad := make([]byte, 13)
	aad[8], aad[9], aad[10] = 22, 3, 3
	binary.BigEndian.PutUint16(aad[11:], uint16(len(plain)))
	explicit := make([]byte, 8)
	payload := append(explicit, s.keys[1].Seal(nil, make([]byte, 12), plain, aad)...)
	w := clearRecord(payload)
	e := &ProtocolEvent{Raw: w}
	_, err = f.consumeTLS(1, e)
	require.NoError(t, err)
	require.Equal(t, true, e.Session["Authentication Verified"])
	m := e.Session["Handshake Messages"].([]map[string]any)[0]
	require.Equal(t, false, m["Authenticated"], "an authenticated final fragment cannot authenticate earlier cleartext")
	require.Equal(t, false, m["New Session Ticket"].(map[string]any)["Record Authentication Verified"])
}

func TestTLSResumptionAuthenticatedTicketFragmentsRemainAuthenticated(t *testing.T) {
	a, _ := remainingParser(t)
	s := newBinTLS()
	s.version, s.suite, s.client = 0x304, 0x1301, 0
	s.app[1], s.finalHello = true, true
	s.random = bytes.Repeat([]byte{1}, 32)
	require.NoError(t, s.install13(1, bytes.Repeat([]byte{2}, 32)))
	f := &binFlow{a: a, id: 1, protocol: "tls", tls: s}
	defer f.closeSession()
	hs := reliabilityHS(4, tlsResumptionNST13(60, []byte{1}, []byte("ticket")))
	seal := func(sequence uint64, p []byte) []byte {
		plain := append(bytes.Clone(p), 22)
		header := []byte{23, 3, 3, 0, 0}
		binary.BigEndian.PutUint16(header[3:], uint16(len(plain)+16))
		nonce := bytes.Clone(s.iv[1])
		for i := 0; i < 8; i++ {
			nonce[11-i] ^= byte(sequence >> uint(8*i))
		}
		return append(header, s.keys[1].Seal(nil, nonce, plain, header)...)
	}
	first := &ProtocolEvent{Raw: seal(0, hs[:6])}
	_, err := f.consumeTLS(1, first)
	require.NoError(t, err)
	require.Equal(t, true, first.Session["Handshake Incomplete"])
	second := &ProtocolEvent{Raw: seal(1, hs[6:])}
	_, err = f.consumeTLS(1, second)
	require.NoError(t, err)
	m := second.Session["Handshake Messages"].([]map[string]any)[0]
	require.Equal(t, true, m["Authenticated"])
	require.Equal(t, true, m["New Session Ticket"].(map[string]any)["Record Authentication Verified"])
}

func TestTLSResumptionEmptyRecordIsNotHandshakeByteContributor(t *testing.T) {
	// An aggregation control with seeded buffered authenticated bytes. This
	// checks the zero-byte contributor contract, not an on-wire cipher epoch
	// transition or a claim that cleartext records authenticate peer identity.
	a, _ := remainingParser(t)
	s := newBinTLS()
	s.version, s.client = 0x303, 0
	s.handshake[1] = []byte{4, 0, 0, 7, 0}
	s.handshakeAuthenticated[1] = true
	f := &binFlow{a: a, id: 1, protocol: "tls", tls: s}
	defer f.closeSession()
	_, err := f.consumeTLS(1, &ProtocolEvent{Raw: []byte{22, 3, 3, 0, 0}})
	require.NoError(t, err)
	require.True(t, s.handshakeAuthenticated[1], "a record contributing no bytes cannot change the aggregate")
	require.Equal(t, []byte{4, 0, 0, 7, 0}, s.handshake[1])
}

func TestTLSResumptionRefusedRecordKeepsPrivateWireAliases(t *testing.T) {
	for _, deferred := range []bool{false, true} {
		t.Run(fmt.Sprint(deferred), func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionPorts(40000, 443))
			require.NoError(t, err)
			defer s.Close("cleanup")
			s.(*captureSession).f.a.config.Deferred = deferred
			ch := s.Feed(0, time.Unix(1, 0), tlsResumptionRecord(reliabilityHS(1, tlsResumptionHello(true, nil, 0xc02b))))
			require.Nil(t, ch.Err)
			bad := s.Feed(1, time.Unix(2, 0), []byte{22, 3, 3, 0, 4, 2, 0, 0, 0})
			rocTypedError(t, "MalformedMessage", bad.Err)
			require.Len(t, bad.Events, 1)
			e := bad.Events[0]
			require.Equal(t, "malformed", e.Status)
			require.NotEmpty(t, e.Error)
			want := map[string][]any{
				"tls.record.wire_type": {uint64(22)}, "tls.record.content_type": {uint64(22)},
				"tls.record.version": {uint64(0x303)}, "tls.record.length": {uint64(4)},
				"tls.record.authenticated": {false},
			}
			check := func() {
				fields := e.DisplayFields()
				for name, values := range want {
					require.Equal(t, values, fields[name], name)
				}
				queried, queryErr := e.GetFields()
				rocTypedError(t, "MalformedMessage", queryErr)
				require.Nil(t, queried)
				decoded, decodeErr := e.Decode()
				rocTypedError(t, "MalformedMessage", decodeErr)
				require.Nil(t, decoded)
			}
			check()
			for _, name := range []string{"radius", "tls", "redis", "caller"} {
				tlsDTLSPoison(e, name)
				check()
			}
		})
	}
}

func FuzzTLSResumptionHelloFailureIsAtomic(f *testing.F) {
	offer := tlsResumptionPSKs([][]byte{[]byte("identity")}, []uint32{1}, []int{32})
	f.Add(tlsResumptionPSKClient(nil, offer, 0))
	f.Add(tlsResumptionPSKServer(nil, 0))
	f.Add([]byte{3, 3})
	f.Fuzz(func(t *testing.T, body []byte) {
		if len(body) > 65535 {
			t.Skip()
		}
		s := newBinTLS()
		before := *s
		_, err := s.handshakeMessage(0, reliabilityHS(1, body), false, 4)
		if err != nil {
			require.Equal(t, before, *s, "malformed/budget-refused input must not alter retained state")
		}
	})
}

func TestTLSResumptionQUICMetadataFollowsAuthenticatedCIDEpoch(t *testing.T) {
	c := NewDefaultConfig()
	require.NoError(t, WithOnProtocolMessage(func(*ProtocolEvent) {})(c))
	require.NoError(t, c.prepareBinParser())
	a := c.binParser
	defer a.closeUDPSessions()
	base := &ProtocolEvent{Source: "client", Destination: "server", Timestamp: time.Unix(1, 0)}
	crypto := func(hs []byte) []byte {
		b := append([]byte{6, 0}, quicPutVarint(uint64(len(hs)))...)
		return append(b, hs...)
	}
	id := []byte("quic-external-or-ticket")
	offer := tlsResumptionPSKs([][]byte{id}, []uint32{0}, []int{32})
	var previous uint64
	for epoch := byte(1); epoch <= 2; epoch++ {
		seed := bytes.Repeat([]byte{epoch}, 8)
		ch := tlsResumptionPSKClient(nil, offer, 0)
		if epoch == 2 {
			ch = tlsResumptionHello(true, nil, 0x1301, tlsResumptionExt{43, []byte{2, 3, 4}})
		}
		events, ok := a.decodeQUICDatagram(base, retryTestInitialFrames(t, seed, seed, nil, nil, 0, 0, crypto(reliabilityHS(1, ch))))
		require.True(t, ok)
		require.Len(t, events, 1)
		require.NotContains(t, events[0].Session, "Session Establishment")
		flow := events[0].FlowID
		require.NotEqual(t, previous, flow)
		previous = flow
		reverse := *base
		reverse.Source, reverse.Destination = base.Destination, base.Source
		sh := tlsResumptionPSKServer(nil, 0)
		if epoch == 2 {
			sh = tlsResumptionHello(false, nil, 0x1301, tlsResumptionExt{43, []byte{3, 4}})
		}
		events, ok = a.decodeQUICDatagram(&reverse, retryTestInitialFrames(t, seed, nil, nil, nil, 0, 1, crypto(reliabilityHS(2, sh))))
		require.True(t, ok)
		require.Len(t, events, 1)
		require.Equal(t, flow, events[0].FlowID)
		ctx := events[0].Session["Session Establishment"].(map[string]any)
		require.Equal(t, epoch == 1, ctx["PSK Selected"])
		if epoch == 2 {
			require.NotContains(t, ctx, "PSK Selection")
		}
		// An ordinary subsequent authenticated PING has no CRYPTO frame.
		events, ok = a.decodeQUICDatagram(&reverse, retryTestProtectedInitial(t, seed, nil, nil, nil, 1, 1))
		require.True(t, ok)
		require.Len(t, events, 1)
		require.Equal(t, ctx, events[0].Session["Session Establishment"])
	}
}
