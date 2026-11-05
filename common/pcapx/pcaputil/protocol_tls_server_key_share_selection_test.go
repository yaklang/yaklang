package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Authored public declarations; neither the one-byte keys nor a matching
// group prove a successful cryptographic handshake. The oracle is RFC8446
// 4.2.8: a final server share must match an actual current client share.
func keyShareSelectionClient(suite uint16, mode string) []byte {
	ext := append([]tlsResumptionExt{{43, []byte{2, 3, 4}}}, tlsResumptionDHExtensions(true)...)
	switch mode {
	case "missing-share":
		ext = certificatePSKTestChange(ext, 51, nil, true)
	case "empty-share":
		ext = certificatePSKTestChange(ext, 51, []byte{0, 0}, false)
	case "missing-groups":
		ext = certificatePSKTestChange(ext, 10, nil, true)
	case "unoffered-group":
		ext = certificatePSKTestChange(ext, 51, []byte{0, 5, 0, 23, 0, 1, 7}, false)
	case "unsupported-group":
		ext = certificatePSKTestChange(ext, 10, []byte{0, 2, 0, 23}, false)
	case "empty-groups":
		ext = certificatePSKTestChange(ext, 10, []byte{0, 0}, false)
	case "short-tail", "duplicate-tail", "zero-length-tail":
		v := bytes.Clone(ext[2].data)
		switch mode {
		case "short-tail":
			v = append(v, 0, 23, 0, 4, 7)
		case "duplicate-tail":
			v = append(v, 0, 29, 0, 1, 7)
		case "zero-length-tail":
			v = append(v, 0, 23, 0, 0)
		}
		binary.BigEndian.PutUint16(v, uint16(len(v)-2))
		ext = certificatePSKTestChange(ext, 51, v, false)
	}
	return tlsResumptionHello(true, nil, suite, ext...)
}

func keyShareSelectionServer(suite uint16) []byte {
	ext := append([]tlsResumptionExt{{43, []byte{3, 4}}}, tlsResumptionDHExtensions(false)...)
	return tlsResumptionHello(false, nil, suite, ext...)
}

func TestTLS13ServerKeyShareRequiresCurrentOffer(t *testing.T) {
	for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
		for server := 0; server < 2; server++ {
			for _, mode := range []string{"matching", "unknown-client", "missing-share", "empty-share", "unoffered-group", "missing-groups", "empty-groups", "unsupported-group", "short-tail", "duplicate-tail", "zero-length-tail"} {
				t.Run(fmt.Sprintf("%04x/server%d/%s", suite, server, mode), func(t *testing.T) {
					s := newBinTLS()
					if mode != "unknown-client" {
						_, err := s.handshakeMessage(1-server, reliabilityHS(1, keyShareSelectionClient(suite, mode)), false)
						require.NoError(t, err)
					}
					before, owned := *s, bytes.Clone(s.firstHello)
					m, err := s.handshakeMessage(server, reliabilityHS(2, keyShareSelectionServer(suite)), false)
					if mode == "matching" || mode == "unknown-client" {
						require.NoError(t, err, "missing capture context is unknown, never proof of a missing offer")
						require.NotNil(t, m)
						require.True(t, s.finalHello)
						return
					}
					var typed *ProtocolError
					require.ErrorAs(t, err, &typed)
					require.Equal(t, ErrMalformedMessage, typed.Kind)
					require.Nil(t, m)
					require.Equal(t, before, *s, "rejected selection cannot commit version, keys, ALPN or retry observations")
					require.Equal(t, owned, s.firstHello)
					fresh := newBinTLS()
					_, err = fresh.handshakeMessage(1-server, reliabilityHS(1, keyShareSelectionClient(suite, "matching")), false)
					require.NoError(t, err)
					_, err = fresh.handshakeMessage(server, reliabilityHS(2, keyShareSelectionServer(suite)), false)
					require.NoError(t, err, "a fresh connection cannot inherit another connection's rejection or offers")
				})
			}
		}
	}
}

func TestTLS13ServerKeyShareCollectionRefusalIsTransactional(t *testing.T) {
	for _, badTail := range []bool{false, true} {
		t.Run(fmt.Sprintf("malformedTail%t", badTail), func(t *testing.T) {
			var shares, groups []byte
			for _, group := range []uint16{29, 23, 24, 25} {
				shares = binary.BigEndian.AppendUint16(shares, group)
				shares = append(shares, 0, 1, 7)
				groups = binary.BigEndian.AppendUint16(groups, group)
			}
			if badTail {
				shares = append(shares, 0, 26, 0, 2, 7)
			}
			ext := []tlsResumptionExt{{43, []byte{2, 3, 4}}, {10, append(binary.BigEndian.AppendUint16(nil, uint16(len(groups))), groups...)}, {51, append(binary.BigEndian.AppendUint16(nil, uint16(len(shares))), shares...)}}
			s := newBinTLS()
			_, err := s.handshakeMessage(0, reliabilityHS(1, tlsResumptionHello(true, nil, 0x1301, ext...)), false)
			require.NoError(t, err)
			before, owned := *s, bytes.Clone(s.firstHello)
			m, err := s.handshakeMessage(1, reliabilityHS(2, keyShareSelectionServer(0x1301)), false, 3)
			var typed *ProtocolError
			require.ErrorAs(t, err, &typed)
			want := ErrResourceExceeded
			if badTail {
				want = ErrMalformedMessage
			}
			require.Equal(t, want, typed.Kind, "the entire vector is checked before applying its collection limit")
			require.Nil(t, m)
			require.Equal(t, before, *s)
			require.Equal(t, owned, s.firstHello)
		})
	}
}

func TestTLS13ServerKeyShareNativeFragmentationAndQUICRollback(t *testing.T) {
	for server := 0; server < 2; server++ {
		for _, chunks := range []int{1, 2, 3} {
			t.Run(fmt.Sprintf("TCP/server%d/chunks%d", server, chunks), func(t *testing.T) {
				s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"))
				require.NoError(t, err)
				t.Cleanup(func() { s.Close("done"); require.Zero(t, s.Stats().BufferedBytes) })
				client := helloObservationRecord(keyShareSelectionClient(0x1301, "empty-share"))
				client[5] = 1
				r := s.Feed(1-server, time.Unix(1, 0), client)
				require.Nil(t, r.Err)
				wire := helloObservationRecord(keyShareSelectionServer(0x1301))
				var events []*ProtocolEvent
				for i := 0; i < chunks; i++ {
					r = s.Feed(server, time.Unix(2+int64(i), 0), wire[i*len(wire)/chunks:(i+1)*len(wire)/chunks])
					events = append(events, r.Events...)
					if i+1 < chunks {
						require.NotNil(t, r.Err)
						require.Equal(t, ErrNeedMore, r.Err.Kind, "a partial record must remain incomplete, without accepting its selection")
						require.Empty(t, r.Events)
					}
				}
				require.NotNil(t, r.Err)
				require.Equal(t, ErrMalformedMessage, r.Err.Kind)
				require.Len(t, events, 1)
				require.Equal(t, wire, events[0].Raw)
				require.NotContains(t, events[0].Session, "Handshake Messages")
				require.Zero(t, events[0].Session["Negotiated Version"])
				n13AuditNoChild(t, s)
			})
		}
		for _, delivery := range []string{"whole", "split", "reordered", "duplicate-tail"} {
			t.Run(fmt.Sprintf("QUIC/server%d/%s", server, delivery), func(t *testing.T) {
				q := &binQUIC{native: true, byteLimit: 1 << 20}
				err := q.feedCrypto(1-server, quicSpaceInitial, 0, reliabilityHS(1, keyShareSelectionClient(0x1301, "empty-share")), map[string]any{}, 4096)
				require.NoError(t, err)
				wire := reliabilityHS(2, keyShareSelectionServer(0x1301))
				middle, offset, tail := len(wire)/2, uint64(0), wire
				if delivery != "whole" {
					offset, tail = uint64(middle), wire[middle:]
					if delivery == "split" {
						offset, tail = 0, wire[:middle]
					}
					require.NoError(t, q.feedCrypto(server, quicSpaceInitial, offset, tail, map[string]any{}, 4096))
					if delivery == "duplicate-tail" {
						require.NoError(t, q.feedCrypto(server, quicSpaceInitial, offset, tail, map[string]any{}, 4096))
					}
					offset, tail = 0, wire[:middle]
					if delivery == "split" {
						offset, tail = uint64(middle), wire[middle:]
					}
				}
				before, owned := *q.tls, bytes.Clone(q.tls.firstHello)
				info := map[string]any{}
				err = q.feedCrypto(server, quicSpaceInitial, offset, tail, info, 4096)
				require.ErrorContains(t, err, "was not offered in current ClientHello")
				require.Equal(t, before, *q.tls)
				require.Equal(t, owned, q.tls.firstHello)
				require.Zero(t, q.cryptoRead[server][quicSpaceInitial])
				require.NotContains(t, info, "TLS Handshakes")
				require.Empty(t, q.alpn)
				require.False(t, q.loadedKeys)
			})
		}
	}
}

func TestTLS13ServerKeyShareLiteralObservation(t *testing.T) {
	wire := helloObservationRecord(keyShareSelectionServer(0x1302))
	original := bytes.Clone(wire)
	o, err := ObserveTLSKeyShareServerHelloRecord(wire, 16)
	require.NoError(t, err)
	require.Equal(t, uint16(29), o.Group)
	require.Equal(t, 32, o.KeyExchangeLength)
	at := o.ExtensionOffset + 4
	require.Equal(t, o.Group, binary.BigEndian.Uint16(wire[at:at+2]))
	require.Equal(t, o.KeyExchangeLength, int(binary.BigEndian.Uint16(wire[at+2:at+4])))
	require.Equal(t, original, wire)
	for cut := 0; cut < len(wire); cut++ {
		partial, err := ObserveTLSKeyShareServerHelloRecord(wire[:cut], 16)
		require.Error(t, err)
		require.Nil(t, partial)
	}
	_, err = ObserveTLSKeyShareServerHelloRecord(wire, 1)
	var typed *ProtocolError
	require.ErrorAs(t, err, &typed)
	require.Equal(t, ErrResourceExceeded, typed.Kind)
}

func TestTLS13ServerKeyShareRetryUsesCurrentClientHello(t *testing.T) {
	for server := 0; server < 2; server++ {
		for _, requestGroup := range []bool{false, true} {
			for _, selected := range []uint16{23, 29} {
				t.Run(fmt.Sprintf("server%d/groupRequest%t/selected%d", server, requestGroup, selected), func(t *testing.T) {
					s := newBinTLS()
					ch := append([]tlsResumptionExt{{43, []byte{2, 3, 4}}}, tlsResumptionDHExtensions(true)...)
					ch = certificatePSKTestChange(ch, 10, []byte{0, 4, 0, 29, 0, 23}, false)
					_, err := s.handshakeMessage(1-server, reliabilityHS(1, tlsResumptionHello(true, nil, 0x1301, ch...)), false)
					require.NoError(t, err)
					cookie := []byte{0, 1, 7}
					hrr := []tlsResumptionExt{{43, []byte{3, 4}}, {44, cookie}}
					if requestGroup {
						hrr = append(hrr, tlsResumptionExt{51, []byte{0, 23}})
					}
					body := tlsResumptionHello(false, nil, 0x1301, hrr...)
					copy(body[2:34], tlsRetryRandom[:])
					_, err = s.handshakeMessage(server, reliabilityHS(2, body), false)
					require.NoError(t, err)
					currentGroup := uint16(29)
					if requestGroup {
						currentGroup = 23
						ch = certificatePSKTestChange(ch, 51, []byte{0, 5, 0, 23, 0, 1, 7}, false)
					}
					ch = append(ch, tlsResumptionExt{44, cookie})
					_, err = s.handshakeMessage(1-server, reliabilityHS(1, tlsResumptionHello(true, nil, 0x1301, ch...)), false)
					require.NoError(t, err)
					share := binary.BigEndian.AppendUint16(nil, selected)
					share = append(share, 0, 1, 7)
					before, owned := *s, bytes.Clone(s.firstHello)
					m, err := s.handshakeMessage(server, reliabilityHS(2, tlsResumptionHello(false, nil, 0x1301, tlsResumptionExt{43, []byte{3, 4}}, tlsResumptionExt{51, share})), false)
					if selected == currentGroup {
						require.NoError(t, err)
						require.NotNil(t, m)
					} else {
						require.ErrorContains(t, err, "was not offered in current ClientHello")
						require.Nil(t, m)
						require.Equal(t, before, *s)
						require.Equal(t, owned, s.firstHello)
					}
				})
			}
		}
	}
}

func FuzzTLS13ServerKeyShareSelectionIsolation(f *testing.F) {
	for _, seed := range []struct {
		vector []byte
		group  uint16
		flags  byte
	}{
		{[]byte{0, 5, 0, 29, 0, 1, 7}, 29, 0},
		{[]byte{0, 0}, 29, 1},
		{[]byte{0, 5, 0, 29, 0, 1, 7}, 0xfe00, 2},
		{[]byte{0, 10, 0, 29, 0, 1, 7, 0, 29, 0, 1, 7}, 29, 0},
		{[]byte{0, 10, 0, 29, 0, 1, 7, 0, 23, 0, 2, 7}, 29, 0},
	} {
		f.Add(seed.vector, seed.group, seed.flags)
	}
	f.Fuzz(func(t *testing.T, vector []byte, group uint16, flags byte) {
		vector = bytes.Clone(vector[:min(len(vector), 512)])
		s := newBinTLS()
		server := int(flags & 1)
		if flags&2 == 0 {
			groups := binary.BigEndian.AppendUint16([]byte{0, 2}, group)
			ch := tlsResumptionHello(true, nil, 0x1301, tlsResumptionExt{43, []byte{2, 3, 4}}, tlsResumptionExt{10, groups}, tlsResumptionExt{51, vector})
			_, err := s.handshakeMessage(1-server, reliabilityHS(1, ch), false)
			if err != nil {
				return
			}
		}
		share := append(binary.BigEndian.AppendUint16(nil, group), 0, 1, 7)
		body := tlsResumptionHello(false, nil, 0x1301, tlsResumptionExt{43, []byte{3, 4}}, tlsResumptionExt{51, share})
		before, owned := *s, bytes.Clone(s.firstHello)
		m, err := s.handshakeMessage(server, reliabilityHS(2, body), false, 8)
		if err != nil {
			require.Nil(t, m)
			require.Equal(t, before, *s)
			require.Equal(t, owned, s.firstHello)
		} else {
			require.True(t, s.finalHello)
		}
		wire := helloObservationRecord(body)
		original := bytes.Clone(wire)
		o, observationErr := ObserveTLSKeyShareServerHelloRecord(wire, 8)
		require.NoError(t, observationErr, "literal observation does not certify current-client agreement")
		require.Equal(t, group, o.Group)
		require.Equal(t, original, wire)
	})
}
