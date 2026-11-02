package pcaputil

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func clientOrderState(client int, suite uint16) *binTLS {
	s := newBinTLS()
	s.client, s.version, s.suite = client, 0x304, suite
	return s
}

func TestTLS13ObservedClientOrderRejectsRepetitionAndReversal(t *testing.T) {
	for _, client := range []int{0, 1} {
		for _, pair := range [][2]byte{{11, 11}, {15, 11}, {15, 15}} {
			t.Run(fmt.Sprintf("client%d/%d-then-%d", client, pair[0], pair[1]), func(t *testing.T) {
				s := clientOrderState(client, 0x1301)
				_, err := s.handshakeMessage(client, serverOrderMessage(t, pair[0], s.suite), true)
				require.NoError(t, err)
				s.recordGap[client], s.handshakeGap[client] = true, true
				before := *s
				m, err := s.handshakeMessage(client, serverOrderMessage(t, pair[1], s.suite), true)
				var typed *ProtocolError
				require.ErrorAs(t, err, &typed)
				require.Equal(t, ErrMalformedMessage, typed.Kind)
				require.Contains(t, err.Error(), "client handshake order")
				require.Nil(t, m)
				require.Equal(t, before, *s, "a gap cannot erase known order; refusal cannot commit peer state")
			})
		}
	}
}

func TestTLS13ObservedClientOrderAllowsObservationSubsequences(t *testing.T) {
	kinds := []byte{11, 15, 20}
	for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
		for _, client := range []int{0, 1} {
			for mask := 1; mask < 1<<len(kinds); mask++ {
				t.Run(fmt.Sprintf("suite%04x/client%d/mask%d", suite, client, mask), func(t *testing.T) {
					s := clientOrderState(client, suite)
					s.recordGap[client], s.handshakeGap[client] = true, true
					for i, kind := range kinds {
						if mask&(1<<i) != 0 {
							_, err := s.handshakeMessage(client, serverOrderMessage(t, kind, suite), true)
							require.NoError(t, err, "missing captured messages do not prove a violation")
						}
					}
					fresh := clientOrderState(client, suite)
					_, err := fresh.handshakeMessage(client, serverOrderMessage(t, 11, suite), true)
					require.NoError(t, err, "a new connection cannot inherit prior client order")
				})
			}
		}
	}
}

func TestTLS13ObservedClientOrderEmptyCertificateCannotSupplyVerify(t *testing.T) {
	for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
		for _, client := range []int{0, 1} {
			t.Run(fmt.Sprintf("suite%04x/client%d", suite, client), func(t *testing.T) {
				s := clientOrderState(client, suite)
				m, err := s.handshakeMessage(client, phaResponseTestCertificate(nil, true), true)
				require.NoError(t, err)
				// Public decoded data cannot replace the observed private fact.
				m["Certificates"] = []map[string]any{{"untrusted": "supplied"}}
				clear(m["Encoded Certificate"].([]byte))
				before := *s
				m, err = s.handshakeMessage(client, serverOrderMessage(t, 15, suite), true)
				require.ErrorContains(t, err, "empty client Certificate")
				require.Nil(t, m)
				require.Equal(t, before, *s)
				_, err = s.handshakeMessage(client, serverOrderMessage(t, 20, suite), true)
				require.NoError(t, err, "empty Certificate may be followed by Finished")
			})
		}
	}
}

func TestTLS13ObservedClientOrderAdmissionIsAtomic(t *testing.T) {
	for _, client := range []int{0, 1} {
		t.Run(fmt.Sprintf("client%d", client), func(t *testing.T) {
			s := clientOrderState(client, 0x1301)
			_, err := s.handshakeMessage(client, serverOrderMessage(t, 11, s.suite), true, 0)
			var typed *ProtocolError
			require.ErrorAs(t, err, &typed)
			require.Equal(t, ErrResourceExceeded, typed.Kind)
			_, err = s.handshakeMessage(client, reliabilityHS(11, []byte{0, 0, 0, 1}), true)
			require.Error(t, err)
			_, err = s.handshakeMessage(client, serverOrderMessage(t, 11, s.suite), true)
			require.NoError(t, err, "body/budget refusals cannot consume an observed stage")
			_, err = s.handshakeMessage(client, reliabilityHS(15, []byte{8, 7, 0, 2, 1}), true)
			require.Error(t, err)
			_, err = s.handshakeMessage(client, serverOrderMessage(t, 15, s.suite), true)
			require.NoError(t, err)
		})
	}
}

func TestTLS13ObservedClientOrderNonemptyCertificateIgnoresPublicMutation(t *testing.T) {
	for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
		for _, client := range []int{0, 1} {
			t.Run(fmt.Sprintf("suite%04x/client%d", suite, client), func(t *testing.T) {
				s := clientOrderState(client, suite)
				m, err := s.handshakeMessage(client, serverOrderMessage(t, 11, suite), true)
				require.NoError(t, err)
				m["Certificates"] = nil
				clear(m["Encoded Certificate"].([]byte))
				_, err = s.handshakeMessage(client, serverOrderMessage(t, 15, suite), true)
				require.NoError(t, err, "public edits cannot erase the privately admitted nonempty list")
			})
		}
	}
}

func TestTLS13ObservedClientOrderDoesNotBorrowUnknownOrOpaqueIdentity(t *testing.T) {
	for _, client := range []int{0, 1} {
		for _, profile := range []string{"unknown-role", "opaque-compressed-certificate", "opposite-server"} {
			t.Run(fmt.Sprintf("client%d/%s", client, profile), func(t *testing.T) {
				s := clientOrderState(client, 0x1301)
				dir, encoded := client, serverOrderMessage(t, 15, s.suite)
				switch profile {
				case "unknown-role":
					s.client = -1
				case "opaque-compressed-certificate":
					encoded = reliabilityHS(25, []byte{0xa5})
				case "opposite-server":
					dir = 1 - client
				}
				_, err := s.handshakeMessage(dir, encoded, true)
				require.NoError(t, err)
				s.client = client
				_, err = s.handshakeMessage(client, serverOrderMessage(t, 11, s.suite), true)
				require.NoError(t, err)
			})
		}
	}
}

func TestTLS13ObservedClientOrderPostHandshakeResponsesRemainIndependent(t *testing.T) {
	for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
		for _, server := range []int{0, 1} {
			t.Run(fmt.Sprintf("suite%04x/server%d", suite, server), func(t *testing.T) {
				f, s := phaResponseTestFlow(t, suite, server)
				client := 1 - server
				for _, context := range [][]byte{{0xa1}, {0xb2}} {
					_, err := phaResponseTestSend(t, f, s, server, phaContextTestRequest(context), 22)
					require.NoError(t, err)
					for _, encoded := range [][]byte{phaResponseTestCertificate(context, false), serverOrderMessage(t, 15, suite), serverOrderMessage(t, 20, suite)} {
						_, err = phaResponseTestSend(t, f, s, client, encoded, 22)
						require.NoError(t, err, "PHA owns its independent observed context and order")
					}
				}
			})
		}
	}
}

func TestTLS13ObservedClientOrderAuthenticatedRecords(t *testing.T) {
	for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
		for _, client := range []int{0, 1} {
			for _, profile := range []string{"duplicate-certificate", "verify-then-certificate", "fragmented-duplicate-verify", "verify-after-empty", "coalesced-reversal"} {
				t.Run(fmt.Sprintf("suite%04x/client%d/%s", suite, client, profile), func(t *testing.T) {
					f, s := eeRecordFlow(t, suite, 1-client, true)
					first, next := byte(11), byte(11)
					if profile == "verify-then-certificate" {
						first = 15
					} else if profile == "fragmented-duplicate-verify" {
						first, next = 15, 15
					} else if profile == "verify-after-empty" {
						next = 15
					}
					encoded := serverOrderMessage(t, first, suite)
					if profile == "verify-after-empty" {
						encoded = phaResponseTestCertificate(nil, true)
					}
					e := &ProtocolEvent{Protocol: "tls", Raw: boundaryTLSRecord(t, s, client, s.seq[client], encoded, 22, 1)}
					_, err := f.consumeTLS(client, e)
					require.NoError(t, err)
					require.Equal(t, true, e.Session["Authentication Verified"])
					key, iv, secret, peerSeq := s.keys[client], bytes.Clone(s.iv[client]), bytes.Clone(s.secret[client]), s.seq[1-client]
					encoded = serverOrderMessage(t, next, suite)
					if profile == "coalesced-reversal" {
						encoded = append(serverOrderMessage(t, 15, suite), encoded...)
					}
					pieces := [][]byte{encoded}
					if profile == "fragmented-duplicate-verify" {
						pieces = [][]byte{encoded[:3], encoded[3:]}
					}
					for i, piece := range pieces {
						seq := s.seq[client]
						e = &ProtocolEvent{Protocol: "tls", Raw: boundaryTLSRecord(t, s, client, seq, piece, 22, 2)}
						_, err = f.consumeTLS(client, e)
						require.Equal(t, true, e.Session["Authentication Verified"])
						require.Equal(t, seq+1, s.seq[client])
						if i < len(pieces)-1 {
							require.NoError(t, err)
						} else {
							var typed *ProtocolError
							require.ErrorAs(t, err, &typed)
							require.Equal(t, ErrMalformedMessage, typed.Kind)
							if profile == "verify-after-empty" {
								require.Contains(t, err.Error(), "empty client Certificate")
							} else {
								require.Contains(t, err.Error(), "client handshake order")
							}
							require.NotContains(t, e.Session, "Handshake Messages")
						}
					}
					require.Same(t, key, s.keys[client])
					require.Equal(t, iv, s.iv[client])
					require.Equal(t, secret, s.secret[client])
					require.Equal(t, peerSeq, s.seq[1-client])
				})
			}
		}
	}
}

func FuzzTLS13ObservedClientOrder(f *testing.F) {
	for _, seed := range [][]byte{{1, 1}, {2, 1}, {2, 2}, {0, 2}, {0, 3, 2}, {1, 2, 3}} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, program []byte) {
		if len(program) > 64 {
			program = program[:64]
		}
		s := clientOrderState(0, 0x1301)
		last, empty := byte(0), false
		for _, operation := range program {
			op := operation & 3
			kind, stage := byte(11), byte(1)
			encoded := phaResponseTestCertificate(nil, op == 0)
			if op == 2 {
				kind, stage = 15, 2
				encoded = serverOrderMessage(t, kind, s.suite)
			} else if op == 3 {
				kind, stage = 25, 0
				encoded = reliabilityHS(kind, []byte{0xa5})
			}
			refuse := stage != 0 && (stage <= last || kind == 15 && empty)
			before := *s
			message, err := s.handshakeMessage(0, encoded, true)
			if refuse {
				var typed *ProtocolError
				require.ErrorAs(t, err, &typed)
				require.Equal(t, ErrMalformedMessage, typed.Kind)
				require.Nil(t, message)
				require.Equal(t, before, *s)
			} else {
				require.NoError(t, err)
				if stage != 0 {
					last = stage
				}
				if kind == 11 {
					empty = op == 0
					message["Certificates"] = nil
					clear(message["Encoded Certificate"].([]byte))
				}
			}
		}
		_, err := s.handshakeMessage(0, serverOrderMessage(t, 20, s.suite), true)
		require.NoError(t, err)
	})
}
