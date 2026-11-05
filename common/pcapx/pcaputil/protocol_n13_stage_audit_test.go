package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Authenticated test bytes use the existing RFC 8446 record sealer. The
// admission oracle is the cited RFC cross-message rule, never decoder output.
type n13AuditSecrets struct {
	random []byte
	secret [2][]byte
	client int
}

func (p *n13AuditSecrets) LookupTLSSecret(label string, random []byte) ([]byte, bool) {
	if !bytes.Equal(p.random, random) {
		return nil, false
	}
	dir := -1
	if label == "CLIENT_HANDSHAKE_TRAFFIC_SECRET" {
		dir = p.client
	}
	if label == "SERVER_HANDSHAKE_TRAFFIC_SECRET" {
		dir = 1 - p.client
	}
	if dir < 0 {
		return nil, false
	}
	return bytes.Clone(p.secret[dir]), true
}

func n13AuditNewSession(t *testing.T, suite uint16, client int) (ProtocolSession, *binTLS) {
	t.Helper()
	session, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionPorts(42000, 443))
	require.NoError(t, err)
	size := 32
	if suite == 0x1302 {
		size = 48
	}
	provider := &n13AuditSecrets{random: bytes.Repeat([]byte{0x31}, 32), client: client}
	seal := newBinTLS()
	seal.client, seal.version, seal.suite = client, 0x304, suite
	for dir := 0; dir < 2; dir++ {
		provider.secret[dir] = bytes.Repeat([]byte{byte(0x61 + dir)}, size)
		require.NoError(t, seal.install13(dir, provider.secret[dir]))
	}
	session.(*captureSession).f.a.tlsSecrets = provider
	t.Cleanup(func() { session.Close("audit done"); require.Zero(t, session.Stats().BufferedBytes) })
	return session, seal
}

func n13AuditHelloPair(suite uint16, offerEarly, selectPSK bool, index uint16) ([]byte, []byte) {
	key := append([]byte{0, 29, 0, 32}, bytes.Repeat([]byte{7}, 32)...)
	ch := []tlsResumptionExt{{43, []byte{2, 3, 4}}, {10, []byte{0, 2, 0, 29}}, {51, append(binary.BigEndian.AppendUint16(nil, uint16(len(key))), key...)}, {45, []byte{1, 1}}}
	if offerEarly {
		ch = append(ch, tlsResumptionExt{42, nil})
	}
	binder := 32
	if suite == 0x1302 {
		binder = 48
	}
	ch = append(ch, tlsResumptionExt{41, tlsResumptionPSKs([][]byte{[]byte("audit identity A"), []byte("audit identity B")}, []uint32{1, 2}, []int{binder, binder})})
	sh := []tlsResumptionExt{{43, []byte{3, 4}}, {51, key}}
	if selectPSK {
		sh = append(sh, tlsResumptionExt{41, binary.BigEndian.AppendUint16(nil, index)})
	}
	return tlsResumptionHello(true, nil, suite, ch...), tlsResumptionHello(false, nil, suite, sh...)
}

func n13AuditFeed(t *testing.T, s ProtocolSession, dir int, wire []byte) FeedResult {
	t.Helper()
	return s.Feed(dir, time.Unix(100, 0), bytes.Clone(wire))
}

// A fatal refusal may dispose the TLS owner. Both disposal and a retained
// owner without an application child satisfy the non-admission contract.
func n13AuditNoChild(t *testing.T, s ProtocolSession) {
	t.Helper()
	if state := s.(*captureSession).f.tls; state != nil {
		require.Nil(t, state.child)
	}
}

func TestN13EarlyDataSelectionMustMatchCompleteHelloEvidence(t *testing.T) {
	for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
		for _, server := range []int{0, 1} {
			for _, mode := range []string{"first-selected-offered", "unoffered", "PSK-unselected", "second-selected", "no-acceptance"} {
				t.Run(fmt.Sprintf("suite%04x/server%d/%s", suite, server, mode), func(t *testing.T) {
					s, seal := n13AuditNewSession(t, suite, 1-server)
					index := uint16(0)
					if mode == "second-selected" {
						index = 1
					}
					ch, sh := n13AuditHelloPair(suite, mode != "unoffered", mode != "PSK-unselected", index)
					require.Nil(t, n13AuditFeed(t, s, 1-server, carrierTestRecord(22, reliabilityHS(1, ch))).Err)
					require.Nil(t, n13AuditFeed(t, s, server, carrierTestRecord(22, reliabilityHS(2, sh))).Err)
					body := eeScopeBody(tlsResumptionExt{42, nil})
					if mode == "no-acceptance" {
						body = eeScopeBody()
					}
					encoded := reliabilityHS(8, body)
					wire := boundaryTLSRecord(t, seal, server, 0, encoded, 22, 0)
					result := n13AuditFeed(t, s, server, wire)
					require.Len(t, result.Events, 1)
					e := result.Events[0]
					require.Equal(t, wire, e.Raw)
					require.Equal(t, true, e.Session["Authentication Verified"])
					refuse := mode == "unoffered" || mode == "PSK-unselected" || mode == "second-selected"
					if refuse {
						require.NotNil(t, result.Err, "RFC8446 4.2.10: empty syntax alone cannot qualify early_data acceptance")
						require.Equal(t, ErrMalformedMessage, result.Err.Kind)
						require.NotContains(t, e.Session, "Handshake Messages")
						require.Empty(t, e.DisplayFieldValues().FieldMap()["tls.handshake.extensions.early_data"])
						_, err := e.GetFields()
						require.Error(t, err)
					} else {
						require.Nil(t, result.Err)
						require.Len(t, e.Session["Handshake Messages"], 1)
					}
					n13AuditNoChild(t, s)
				})
			}
		}
	}
}

func TestN13CompleteTLS13ServerHelloRequiresSelectedKeyExchange(t *testing.T) {
	for _, knownCH := range []bool{false, true} {
		for _, mode := range []string{"missing-both", "key-share", "PSK-only", "key-share-empty", "key-share-length"} {
			t.Run(fmt.Sprintf("knownCH%t/%s", knownCH, mode), func(t *testing.T) {
				s, _ := n13AuditNewSession(t, 0x1301, 0)
				ch, _ := n13AuditHelloPair(0x1301, false, true, 0)
				// Both documented PSK exchange modes are offered; this test
				// isolates a complete ServerHello's necessary structure.
				h, err := tlsHello(ch, true)
				require.NoError(t, err)
				var chExt []tlsResumptionExt
				for _, kind := range h.order {
					value := h.ext[kind]
					if kind == 45 {
						value = []byte{2, 0, 1}
					}
					chExt = append(chExt, tlsResumptionExt{kind, value})
				}
				if knownCH {
					require.Nil(t, n13AuditFeed(t, s, 0, carrierTestRecord(22, reliabilityHS(1, tlsResumptionHello(true, nil, 0x1301, chExt...)))).Err)
				}
				sh := []tlsResumptionExt{{43, []byte{3, 4}}}
				switch mode {
				case "key-share":
					sh = append(sh, tlsResumptionExt{51, append([]byte{0, 29, 0, 32}, bytes.Repeat([]byte{9}, 32)...)})
				case "PSK-only":
					sh = append(sh, tlsResumptionExt{41, []byte{0, 0}})
				case "key-share-empty":
					sh = append(sh, tlsResumptionExt{51, nil})
				case "key-share-length":
					sh = append(sh, tlsResumptionExt{51, []byte{0, 29, 0, 2, 1}})
				}
				wire := carrierTestRecord(22, reliabilityHS(2, tlsResumptionHello(false, nil, 0x1301, sh...)))
				result := n13AuditFeed(t, s, 1, wire)
				require.Len(t, result.Events, 1)
				if mode == "key-share" || mode == "PSK-only" {
					require.Nil(t, result.Err)
				} else {
					require.NotNil(t, result.Err, "RFC8446 4.1.3: a complete final SH requires key_share or pre_shared_key")
					require.Equal(t, ErrMalformedMessage, result.Err.Kind)
					if state := s.(*captureSession).f.tls; state != nil {
						require.False(t, state.finalHello, "a refused complete SH cannot commit a selected epoch")
					}
					require.NotContains(t, result.Events[0].Session, "Handshake Messages")
				}
				require.Equal(t, wire, result.Events[0].Raw)
				n13AuditNoChild(t, s)
			})
		}
	}
}

func TestN13EarlyDataCannotBeAcceptedAfterObservedHRR(t *testing.T) {
	for _, server := range []int{0, 1} {
		t.Run(fmt.Sprintf("server%d", server), func(t *testing.T) {
			s, seal := n13AuditNewSession(t, 0x1301, 1-server)
			first, _ := n13AuditHelloPair(0x1301, true, true, 0)
			h, err := tlsHello(first, true)
			require.NoError(t, err)
			var firstExt []tlsResumptionExt
			for _, kind := range h.order {
				value := h.ext[kind]
				if kind == 10 {
					value = []byte{0, 4, 0, 29, 0, 23}
				}
				firstExt = append(firstExt, tlsResumptionExt{kind, value})
			}
			require.Nil(t, n13AuditFeed(t, s, 1-server, carrierTestRecord(22, reliabilityHS(1, tlsResumptionHello(true, nil, 0x1301, firstExt...)))).Err)
			retry := tlsResumptionHello(false, nil, 0x1301, tlsResumptionExt{43, []byte{3, 4}}, tlsResumptionExt{51, []byte{0, 23}})
			copy(retry[2:34], tlsRetryRandom[:])
			require.Nil(t, n13AuditFeed(t, s, server, carrierTestRecord(22, reliabilityHS(2, retry))).Err)
			var secondExt []tlsResumptionExt
			share := append([]byte{0, 23, 0, 65}, append([]byte{4}, bytes.Repeat([]byte{6}, 64)...)...)
			for _, e := range firstExt {
				if e.kind == 42 {
					continue
				}
				if e.kind == 51 {
					e.data = append(binary.BigEndian.AppendUint16(nil, uint16(len(share))), share...)
				}
				secondExt = append(secondExt, e)
			}
			require.Nil(t, n13AuditFeed(t, s, 1-server, carrierTestRecord(22, reliabilityHS(1, tlsResumptionHello(true, nil, 0x1301, secondExt...)))).Err)
			sh := tlsResumptionHello(false, nil, 0x1301, tlsResumptionExt{43, []byte{3, 4}}, tlsResumptionExt{51, share}, tlsResumptionExt{41, []byte{0, 0}})
			require.Nil(t, n13AuditFeed(t, s, server, carrierTestRecord(22, reliabilityHS(2, sh))).Err)
			wire := boundaryTLSRecord(t, seal, server, 0, reliabilityHS(8, eeScopeBody(tlsResumptionExt{42, nil})), 22, 0)
			r := n13AuditFeed(t, s, server, wire)
			require.NotNil(t, r.Err, "RFC8446 4.2.10: observed HRR prohibits subsequent early_data acceptance")
			require.Equal(t, ErrMalformedMessage, r.Err.Kind)
			require.Len(t, r.Events, 1)
			require.Equal(t, true, r.Events[0].Session["Authentication Verified"])
			require.NotContains(t, r.Events[0].Session, "Handshake Messages")
			n13AuditNoChild(t, s)
		})
	}
}

func TestN13EarlyDataUnknownHelloContextRemainsUnconfirmed(t *testing.T) {
	state := newBinTLS()
	state.version, state.suite, state.client = 0x304, 0x1301, 0
	// Private offer/selection provenance is absent. Negotiated public-looking
	// fields alone cannot manufacture either a missing offer or a selected PSK.
	_, err := state.handshakeMessage(1, reliabilityHS(8, eeScopeBody(tlsResumptionExt{42, nil})), true)
	require.NoError(t, err)
}

func TestN13EarlyDataOfferRequiresPSKInSameCompleteClientHello(t *testing.T) {
	s, _ := n13AuditNewSession(t, 0x1301, 0)
	ch := tlsResumptionHello(true, nil, 0x1301, tlsResumptionExt{43, []byte{2, 3, 4}}, tlsResumptionExt{42, nil})
	r := n13AuditFeed(t, s, 0, carrierTestRecord(22, reliabilityHS(1, ch)))
	require.NotNil(t, r.Err, "RFC8446 4.2.10: a complete early_data offer requires pre_shared_key")
	require.Equal(t, ErrMalformedMessage, r.Err.Kind)
	require.Len(t, r.Events, 1)
	if state := s.(*captureSession).f.tls; state != nil {
		require.False(t, state.finalHello)
		require.Empty(t, state.random)
	}
}

func TestN13TLS13AlertAndCCSUseSenderDirectionEvidence(t *testing.T) {
	for _, server := range []int{0, 1} {
		for _, mode := range []string{"server-alert-after-SH", "client-alert-after-SH-only", "client-alert-after-server-Finished", "client-alert-after-own-Finished", "CCS-before-own-Finished", "CCS-after-own-Finished"} {
			t.Run(fmt.Sprintf("server%d/%s", server, mode), func(t *testing.T) {
				s, seal := n13AuditNewSession(t, 0x1301, 1-server)
				ch, sh := n13AuditHelloPair(0x1301, false, true, 0)
				require.Nil(t, n13AuditFeed(t, s, 1-server, carrierTestRecord(22, reliabilityHS(1, ch))).Err)
				require.Nil(t, n13AuditFeed(t, s, server, carrierTestRecord(22, reliabilityHS(2, sh))).Err)
				sender := 1 - server
				if mode == "server-alert-after-SH" {
					sender = server
				}
				if mode == "client-alert-after-own-Finished" || mode == "CCS-after-own-Finished" || mode == "client-alert-after-server-Finished" || mode == "CCS-before-own-Finished" {
					finishedDir := sender
					if mode == "client-alert-after-server-Finished" || mode == "CCS-before-own-Finished" {
						finishedDir = server
					}
					finish := boundaryTLSRecord(t, seal, finishedDir, 0, finishedTestMessage(32), 22, 0)
					r := n13AuditFeed(t, s, finishedDir, finish)
					require.Nil(t, r.Err)
					require.Equal(t, true, r.Events[0].Session["Authentication Verified"])
				}
				wire := carrierTestRecord(21, []byte{2, 40})
				if mode == "CCS-before-own-Finished" || mode == "CCS-after-own-Finished" {
					wire = carrierTestRecord(20, []byte{1})
				}
				r := n13AuditFeed(t, s, sender, wire)
				require.Len(t, r.Events, 1)
				require.Equal(t, wire, r.Events[0].Raw)
				refuse := mode == "server-alert-after-SH" || mode == "client-alert-after-own-Finished" || mode == "CCS-after-own-Finished"
				if refuse {
					require.NotNil(t, r.Err, "sender's own admitted state requires a protected record")
					require.Equal(t, ErrMalformedMessage, r.Err.Kind)
				} else {
					require.Nil(t, r.Err, "cross-direction capture order does not prove receipt of ServerHello or sender Finished")
				}
				n13AuditNoChild(t, s)
			})
		}
	}
}

func TestN13ProtectedHelloReentryIsAlreadyRefused(t *testing.T) {
	for _, kind := range []byte{1, 2} {
		for _, sender := range []int{0, 1} {
			t.Run(fmt.Sprintf("type%d/sender%d", kind, sender), func(t *testing.T) {
				s, seal := n13AuditNewSession(t, 0x1301, 0)
				ch, sh := n13AuditHelloPair(0x1301, false, true, 0)
				require.Nil(t, n13AuditFeed(t, s, 0, carrierTestRecord(22, reliabilityHS(1, ch))).Err)
				require.Nil(t, n13AuditFeed(t, s, 1, carrierTestRecord(22, reliabilityHS(2, sh))).Err)
				state := s.(*captureSession).f.tls
				client, version, suite, mode := state.client, state.version, state.suite, state.initialPSKAuthentication
				body := ch
				if kind == 2 {
					body = sh
				}
				wire := boundaryTLSRecord(t, seal, sender, 0, reliabilityHS(kind, body), 22, 0)
				r := n13AuditFeed(t, s, sender, wire)
				require.NotNil(t, r.Err, "an authenticated repeated hello is not admitted")
				require.Contains(t, []ProtocolErrorKind{ErrUnsupportedFeature, ErrMalformedMessage}, r.Err.Kind)
				require.Len(t, r.Events, 1)
				require.Equal(t, true, r.Events[0].Session["Authentication Verified"])
				require.NotContains(t, r.Events[0].Session, "Handshake Messages")
				require.Equal(t, client, state.client)
				require.Equal(t, version, state.version)
				require.Equal(t, suite, state.suite)
				require.Equal(t, mode, state.initialPSKAuthentication)
				require.Zero(t, state.serverHandshakeStage)
				require.Zero(t, state.clientHandshakeStage)
				require.Equal(t, uint64(1), state.seq[sender])
				require.Nil(t, state.child)
			})
		}
	}
}

func TestN13ArchivedAlertSetupWithoutKEXIsRefused(t *testing.T) {
	for _, c := range tlsAlertOriginalControls(t) {
		if len(c.Steps) < 2 || !bytes.HasPrefix([]byte(c.Name), []byte("known-tls13-")) {
			continue
		}
		t.Run(c.Name, func(t *testing.T) {
			s, _ := n13AuditNewSession(t, 0x1301, 0)
			require.Nil(t, n13AuditFeed(t, s, c.Steps[0].Direction, wrapperWire(t, c.Steps[0].Wire)).Err)
			wire := wrapperWire(t, c.Steps[1].Wire)
			r := n13AuditFeed(t, s, c.Steps[1].Direction, wire)
			require.NotNil(t, r.Err)
			require.Equal(t, ErrMalformedMessage, r.Err.Kind)
			require.Contains(t, r.Err.Message, "requires key_share or pre_shared_key")
			require.Len(t, r.Events, 1)
			require.Equal(t, wire, r.Events[0].Raw)
			require.NotContains(t, r.Events[0].Session, "Handshake Messages")
			n13AuditNoChild(t, s)
		})
	}
}

func FuzzTLS13EarlyDataObservedSelection(f *testing.F) {
	for _, seed := range []struct{ suite, server, flags byte }{{0, 0, 7}, {1, 1, 6}, {2, 0, 5}, {0, 1, 15}, {1, 0, 3}, {2, 1, 23}} {
		f.Add(seed.suite, seed.server, seed.flags)
	}
	f.Fuzz(func(t *testing.T, cipher, direction, flags byte) {
		suite := []uint16{0x1301, 0x1302, 0x1303}[int(cipher)%3]
		server := int(direction % 2)
		s, seal := n13AuditNewSession(t, suite, 1-server)
		offered, selected, acceptance := flags&1 != 0, flags&2 != 0, flags&4 != 0
		index := uint16((flags >> 3) & 1)
		ch, sh := n13AuditHelloPair(suite, offered, selected, index)
		require.Nil(t, n13AuditFeed(t, s, 1-server, carrierTestRecord(22, reliabilityHS(1, ch))).Err)
		require.Nil(t, n13AuditFeed(t, s, server, carrierTestRecord(22, reliabilityHS(2, sh))).Err)
		var ext []tlsResumptionExt
		if acceptance {
			var value []byte
			if flags&16 != 0 {
				value = []byte{1}
			}
			ext = []tlsResumptionExt{{42, value}}
		}
		wire := boundaryTLSRecord(t, seal, server, 0, reliabilityHS(8, eeScopeBody(ext...)), 22, 0)
		r := n13AuditFeed(t, s, server, wire)
		require.Len(t, r.Events, 1)
		require.Equal(t, wire, r.Events[0].Raw)
		require.Equal(t, true, r.Events[0].Session["Authentication Verified"])
		if acceptance && (!offered || !selected || index != 0 || flags&16 != 0) {
			require.NotNil(t, r.Err)
			require.Equal(t, ErrMalformedMessage, r.Err.Kind)
			require.NotContains(t, r.Events[0].Session, "Handshake Messages")
		} else {
			require.Nil(t, r.Err)
			require.Len(t, r.Events[0].Session["Handshake Messages"], 1)
		}
		n13AuditNoChild(t, s)
	})
}
