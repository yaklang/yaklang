package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// These tests validate negotiated message roles and vectors, not DH/binders,
// signatures, Finished MACs or certificate trust.
func pskAuthenticationHello(t *testing.T, suite uint16, client int, mode string, states ...*binTLS) (*binTLS, map[string]any) {
	t.Helper()
	s := newBinTLS()
	if len(states) > 0 {
		require.Len(t, states, 1)
		s = states[0]
		s.client, s.version = -1, 0
	}
	key := append([]byte{0, 29, 0, 32}, bytes.Repeat([]byte{0x31}, 32)...)
	shares := append(binary.BigEndian.AppendUint16(nil, uint16(len(key))), key...)
	ch := []tlsResumptionExt{{43, []byte{2, 3, 4}}, {10, []byte{0, 2, 0, 29}}, {51, shares}, {13, []byte{0, 2, 8, 7}}, {45, []byte{2, 0, 1}}, {49, nil}}
	sh := []tlsResumptionExt{{43, []byte{3, 4}}, {51, key}}
	if mode == "certificate-with-external-PSK" {
		ch = append(ch, tlsResumptionExt{33, nil})
		sh = append(sh, tlsResumptionExt{33, nil})
	}
	ch = append(ch, tlsResumptionExt{41, tlsResumptionPSKs([][]byte{[]byte("test identity")}, []uint32{0}, []int{phaResponseTestHashSize(suite)})})
	if mode != "offered-not-selected" {
		sh = append(sh, tlsResumptionExt{41, []byte{0, 0}})
	}
	if mode != "missing-client-hello" {
		_, err := s.handshakeMessage(client, reliabilityHS(1, tlsResumptionHello(true, nil, suite, ch...)), false)
		require.NoError(t, err)
	}
	if mode == "client-hello-only" {
		return s, nil
	}
	m, err := s.handshakeMessage(1-client, reliabilityHS(2, tlsResumptionHello(false, nil, suite, sh...)), false)
	require.NoError(t, err)
	require.Equal(t, mode != "offered-not-selected", tlsResumptionContext(t, s)["PSK Selected"])
	return s, m
}

func pskAuthenticationMessage(t *testing.T, kind byte, suite uint16) []byte {
	t.Helper()
	if kind == 25 {
		// The currently opaque body is not asserted to be a valid compression.
		return reliabilityHS(kind, []byte{0xa5})
	}
	return serverOrderMessage(t, kind, suite)
}

func TestTLS13PSKAuthenticationRejectsInitialCertificateMessages(t *testing.T) {
	for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
		for _, client := range []int{0, 1} {
			for _, sender := range []int{client, 1 - client} {
				for _, kind := range []byte{13, 11, 15, 25} {
					if kind == 13 && sender == client {
						continue // existing role refusal, not this PSK contract
					}
					t.Run(fmt.Sprintf("suite%04x/client%d/sender%d/type%d", suite, client, sender, kind), func(t *testing.T) {
						s, _ := pskAuthenticationHello(t, suite, client, "ordinary")
						s.recordGap[sender], s.handshakeGap[sender] = true, true
						before := *s
						m, err := s.handshakeMessage(sender, pskAuthenticationMessage(t, kind, suite), true)
						var typed *ProtocolError
						require.ErrorAs(t, err, &typed)
						require.Equal(t, ErrMalformedMessage, typed.Kind)
						require.Contains(t, err.Error(), "PSK")
						require.Nil(t, m)
						require.Equal(t, before, *s, "refusal cannot commit request, order, key or negotiation state")
					})
				}
			}
		}
	}
}

func TestTLS13PSKAuthenticationAllowsNegotiatedCertificateAndFallback(t *testing.T) {
	for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
		for _, client := range []int{0, 1} {
			for _, mode := range []string{"ordinary", "certificate-with-external-PSK", "offered-not-selected"} {
				t.Run(fmt.Sprintf("suite%04x/client%d/%s", suite, client, mode), func(t *testing.T) {
					s, _ := pskAuthenticationHello(t, suite, client, mode)
					server := 1 - client
					kinds := []byte{8, 13, 11, 15, 20}
					if mode == "ordinary" {
						kinds = []byte{8, 20}
					}
					for _, kind := range kinds {
						_, err := s.handshakeMessage(server, serverOrderMessage(t, kind, suite), true)
						require.NoError(t, err)
					}
					kinds = []byte{11, 15, 20}
					if mode == "ordinary" {
						kinds = []byte{20}
					}
					for _, kind := range kinds {
						_, err := s.handshakeMessage(client, serverOrderMessage(t, kind, suite), true)
						require.NoError(t, err)
					}
					require.Equal(t, [2]bool{true, true}, s.app)
				})
			}
		}
	}
}

func TestTLS13PSKAuthenticationSelectionMustComeFromCompleteServerHello(t *testing.T) {
	for _, client := range []int{0, 1} {
		t.Run(fmt.Sprintf("missing-client%d", client), func(t *testing.T) {
			s, _ := pskAuthenticationHello(t, 0x1301, client, "missing-client-hello")
			_, err := s.handshakeMessage(1-client, serverOrderMessage(t, 13, s.suite), true)
			require.ErrorContains(t, err, "PSK")
		})
		t.Run(fmt.Sprintf("offer-only-client%d", client), func(t *testing.T) {
			s, _ := pskAuthenticationHello(t, 0x1301, client, "client-hello-only")
			m, err := s.handshakeMessage(1-client, certificateRequestFixture(t, true), true)
			require.NoError(t, err)
			require.Contains(t, m, "Certificate Request Status", "a client offer cannot establish selected PSK or a body grammar")
		})
	}
}

func TestTLS13PSKAuthenticationIgnoresPublicContextEdits(t *testing.T) {
	for _, client := range []int{0, 1} {
		for _, mode := range []string{"ordinary", "offered-not-selected"} {
			t.Run(fmt.Sprintf("client%d/%s", client, mode), func(t *testing.T) {
				s, hello := pskAuthenticationHello(t, 0x1301, client, mode)
				if selection, ok := hello["PSK Selection"].(map[string]any); ok {
					clear(selection)
				}
				ctx := tlsResumptionContext(t, s)
				ctx["PSK Selected"] = mode != "ordinary"
				m, err := s.handshakeMessage(1-client, serverOrderMessage(t, 11, s.suite), true)
				if mode == "ordinary" {
					require.ErrorContains(t, err, "PSK")
					require.Nil(t, m)
				} else {
					require.NoError(t, err)
				}
			})
		}
	}
}

func TestTLS13PSKAuthenticationRejectedHelloCannotLatchMode(t *testing.T) {
	for _, client := range []int{0, 1} {
		for _, profile := range []string{"unoffered-same-hash-suite", "collection-budget"} {
			t.Run(fmt.Sprintf("client%d/%s", client, profile), func(t *testing.T) {
				s, _ := pskAuthenticationHello(t, 0x1301, client, "client-hello-only")
				before := *s
				badSuite := uint16(0x1303) // same binder hash, but not in this ClientHello
				if profile == "collection-budget" {
					badSuite = 0x1301
				}
				bad := reliabilityHS(2, tlsResumptionHello(false, nil, badSuite, tlsResumptionExt{43, []byte{3, 4}}, tlsResumptionExt{41, []byte{0, 0}}))
				limits := []int(nil)
				if profile == "collection-budget" {
					limits = []int{0}
				}
				_, err := s.handshakeMessage(1-client, bad, false, limits...)
				require.Error(t, err)
				require.Equal(t, before, *s)
				key := append([]byte{0, 29, 0, 32}, bytes.Repeat([]byte{0x31}, 32)...)
				fallback := reliabilityHS(2, tlsResumptionHello(false, nil, 0x1301, tlsResumptionExt{43, []byte{3, 4}}, tlsResumptionExt{51, key}))
				_, err = s.handshakeMessage(1-client, fallback, false)
				require.NoError(t, err)
				require.Equal(t, false, tlsResumptionContext(t, s)["PSK Selected"])
				_, err = s.handshakeMessage(1-client, serverOrderMessage(t, 11, s.suite), true)
				require.NoError(t, err, "a refused selected-PSK candidate cannot poison a later certificate fallback")
			})
		}
	}
}

func TestTLS13PSKAuthenticationPostHandshakeRemainsIndependent(t *testing.T) {
	for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
		for _, server := range []int{0, 1} {
			t.Run(fmt.Sprintf("suite%04x/server%d", suite, server), func(t *testing.T) {
				f, original := phaResponseTestFlow(t, suite, server)
				s, _ := pskAuthenticationHello(t, suite, 1-server, "ordinary")
				// Original test crypto represents authenticated application keys;
				// PSK selection comes from the complete separate hello program.
				s.keys, s.iv, s.secret, s.seq = original.keys, original.iv, original.secret, original.seq
				s.app, s.phaInitialFinished = [2]bool{true, true}, [2]bool{true, true}
				f.tls = s
				context := []byte{0xa1}
				_, err := phaResponseTestSend(t, f, s, server, phaContextTestRequest(context), 22)
				require.NoError(t, err)
				for _, encoded := range [][]byte{phaResponseTestCertificate(context, false), serverOrderMessage(t, 15, suite), serverOrderMessage(t, 20, suite)} {
					_, err = phaResponseTestSend(t, f, s, 1-server, encoded, 22)
					require.NoError(t, err, "selected PSK does not prohibit separately negotiated application-key PHA")
				}
			})
		}
	}
}

func TestTLS13PSKAuthenticationAuthenticatedRecordRefusals(t *testing.T) {
	for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
		for _, client := range []int{0, 1} {
			for _, sender := range []int{client, 1 - client} {
				for _, kind := range []byte{13, 11, 15, 25} {
					if kind == 13 && sender == client {
						continue
					}
					t.Run(fmt.Sprintf("suite%04x/client%d/sender%d/type%d", suite, client, sender, kind), func(t *testing.T) {
						f, s := eeRecordFlow(t, suite, 1-client, false)
						pskAuthenticationHello(t, suite, client, "ordinary", s)
						sequence, peerSequence := s.seq[sender], s.seq[1-sender]
						key, iv, secret := s.keys[sender], bytes.Clone(s.iv[sender]), bytes.Clone(s.secret[sender])
						serverStage, clientStage := s.serverHandshakeStage, s.clientHandshakeStage
						e := &ProtocolEvent{Protocol: "tls", Raw: boundaryTLSRecord(t, s, sender, sequence, pskAuthenticationMessage(t, kind, suite), 22, 1)}
						_, err := f.consumeTLS(sender, e)
						var typed *ProtocolError
						require.ErrorAs(t, err, &typed)
						require.Equal(t, ErrMalformedMessage, typed.Kind)
						require.Contains(t, err.Error(), "PSK")
						require.Equal(t, true, e.Session["Authentication Verified"])
						require.Equal(t, true, e.Session["Session Establishment"].(map[string]any)["PSK Selected"])
						require.NotContains(t, e.Session, "Handshake Messages")
						require.Equal(t, sequence+1, s.seq[sender])
						require.Equal(t, peerSequence, s.seq[1-sender])
						require.Same(t, key, s.keys[sender])
						require.Equal(t, iv, s.iv[sender])
						require.Equal(t, secret, s.secret[sender])
						require.Equal(t, serverStage, s.serverHandshakeStage)
						require.Equal(t, clientStage, s.clientHandshakeStage)
						require.Equal(t, [2]bool{}, s.app)
					})
				}
			}
		}
	}
}

func FuzzTLS13PSKAuthenticationScope(f *testing.F) {
	for _, seed := range [][]byte{{0, 0, 0}, {1, 1, 2}, {2, 2, 4}, {0, 0, 6}, {4, 0, 0, 1}, {5, 0, 0, 0, 1}} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input []byte) {
		var program [5]byte
		copy(program[:], input)
		suite := []uint16{0x1301, 0x1302, 0x1303}[program[0]%3]
		client := int(program[0]>>2) & 1
		mode := []string{"ordinary", "certificate-with-external-PSK", "offered-not-selected"}[program[1]%3]
		s, hello := pskAuthenticationHello(t, suite, client, mode)
		sender := 1 - client
		if program[2]&1 != 0 {
			sender = client
		}
		kind := []byte{11, 13, 15, 25}[(program[2]>>1)&3]
		s.app[sender] = program[3]&1 != 0
		authenticated := program[4]&1 == 0
		if selection, ok := hello["PSK Selection"].(map[string]any); ok {
			clear(selection)
		}
		public := tlsResumptionContext(t, s)
		public["PSK Selected"] = mode != "ordinary"
		before := *s
		message, err := s.handshakeMessage(sender, pskAuthenticationMessage(t, kind, suite), authenticated)
		refusePSK := mode == "ordinary" && !before.app[sender] && authenticated
		if refusePSK {
			var typed *ProtocolError
			require.ErrorAs(t, err, &typed)
			require.Equal(t, ErrMalformedMessage, typed.Kind)
			require.Contains(t, err.Error(), "PSK forbids initial")
			require.Nil(t, message)
		} else if err != nil {
			require.NotContains(t, err.Error(), "PSK forbids initial", "ordinary-PSK policy cannot override PHA, cleartext or peer-role rules")
		}
		if err != nil {
			require.Equal(t, before, *s)
		} else if kind == 25 {
			require.NotContains(t, message, "Encoded Certificate")
			require.NotContains(t, message, "Certificate Body")
		}
	})
}
