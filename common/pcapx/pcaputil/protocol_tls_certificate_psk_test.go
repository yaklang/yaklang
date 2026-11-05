package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// The independently constructed vectors assert only RFC 8773 flag/companion
// declarations. They do not prove that an opaque PSK is external, or verify a
// binder, certificate signature, Finished MAC or transcript.
func certificatePSKTestHellos(suite uint16) ([]tlsResumptionExt, []tlsResumptionExt) {
	key := append([]byte{0, 29, 0, 32}, bytes.Repeat([]byte{0x31}, 32)...)
	shares := append(binary.BigEndian.AppendUint16(nil, uint16(len(key))), key...)
	binderLength := 32
	if suite == 0x1302 {
		binderLength = 48
	}
	return []tlsResumptionExt{{43, []byte{2, 3, 4}}, {51, shares}, {45, []byte{2, 0, 1}}, {33, nil},
			{41, tlsResumptionPSKs([][]byte{[]byte("external identity declaration")}, []uint32{7}, []int{binderLength})}},
		[]tlsResumptionExt{{43, []byte{3, 4}}, {51, key}, {33, nil}, {41, []byte{0, 0}}}
}

func certificatePSKTestChange(exts []tlsResumptionExt, kind uint16, value []byte, remove bool) []tlsResumptionExt {
	out := make([]tlsResumptionExt, 0, len(exts))
	for _, e := range exts {
		if e.kind == kind {
			if remove {
				continue
			}
			e.data = value
		}
		out = append(out, e)
	}
	return out
}

func certificatePSKTestRefusal(t *testing.T, state *binTLS, dir int, kind byte, body []byte, limits ...int) {
	t.Helper()
	before := *state
	m, err := state.handshakeMessage(dir, reliabilityHS(kind, body), false, limits...)
	var typed *ProtocolError
	require.ErrorAs(t, err, &typed)
	require.Equal(t, ErrMalformedMessage, typed.Kind)
	require.Nil(t, m)
	require.Equal(t, before, *state, "refused hello cannot admit negotiation, retry, identity or authentication mode")
}

func TestTLSCertificatePSKHelloCompanions(t *testing.T) {
	for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
		for _, client := range []int{0, 1} {
			for _, fault := range []string{"flag-data", "missing-psk", "missing-share", "missing-modes", "no-dhe-mode", "early-data", "share-length", "share-duplicate"} {
				t.Run(fmt.Sprintf("%04x/client%d/CH/%s", suite, client, fault), func(t *testing.T) {
					ch, sh := certificatePSKTestHellos(suite)
					bad := ch
					switch fault {
					case "flag-data":
						bad = certificatePSKTestChange(ch, 33, []byte{0}, false)
					case "missing-psk":
						bad = certificatePSKTestChange(ch, 41, nil, true)
					case "missing-share":
						bad = certificatePSKTestChange(ch, 51, nil, true)
					case "missing-modes":
						bad = certificatePSKTestChange(ch, 45, nil, true)
					case "no-dhe-mode":
						bad = certificatePSKTestChange(ch, 45, []byte{1, 0}, false)
					case "early-data":
						bad = append(append([]tlsResumptionExt{}, ch[:len(ch)-1]...), tlsResumptionExt{42, nil}, ch[len(ch)-1])
					case "share-length":
						bad = certificatePSKTestChange(ch, 51, []byte{0, 1, 0}, false)
					case "share-duplicate":
						key := sh[1].data
						bad = certificatePSKTestChange(ch, 51, append(binary.BigEndian.AppendUint16(nil, uint16(2*len(key))), append(bytes.Clone(key), key...)...), false)
					}
					state := newBinTLS()
					certificatePSKTestRefusal(t, state, client, 1, tlsResumptionHello(true, nil, suite, bad...))
					_, err := state.handshakeMessage(client, reliabilityHS(1, tlsResumptionHello(true, nil, suite, ch...)), false)
					require.NoError(t, err)
					_, err = state.handshakeMessage(1-client, reliabilityHS(2, tlsResumptionHello(false, nil, suite, sh...)), false)
					require.NoError(t, err, "a rejected candidate cannot poison the later complete declaration")
					require.Equal(t, tlsPSKAuthenticationCertificateExtension, state.initialPSKAuthentication)
				})
			}
			for _, known := range []bool{false, true} {
				for _, fault := range []string{"flag-data", "missing-psk", "missing-share", "TLS12", "share-length"} {
					t.Run(fmt.Sprintf("%04x/client%d/SH/CH%v/%s", suite, client, known, fault), func(t *testing.T) {
						ch, sh := certificatePSKTestHellos(suite)
						state := newBinTLS()
						if known {
							_, err := state.handshakeMessage(client, reliabilityHS(1, tlsResumptionHello(true, nil, suite, ch...)), false)
							require.NoError(t, err)
						}
						bad := sh
						switch fault {
						case "flag-data":
							bad = certificatePSKTestChange(sh, 33, []byte{0xff}, false)
						case "missing-psk":
							bad = certificatePSKTestChange(sh, 41, nil, true)
						case "missing-share":
							bad = certificatePSKTestChange(sh, 51, nil, true)
						case "TLS12":
							bad = certificatePSKTestChange(certificatePSKTestChange(sh, 41, nil, true), 43, []byte{3, 3}, false)
						case "share-length":
							bad = certificatePSKTestChange(sh, 51, []byte{0, 29, 0, 0}, false)
						}
						certificatePSKTestRefusal(t, state, 1-client, 2, tlsResumptionHello(false, nil, suite, bad...))
						_, err := state.handshakeMessage(1-client, reliabilityHS(2, tlsResumptionHello(false, nil, suite, sh...)), false)
						require.NoError(t, err)
						require.Equal(t, tlsPSKAuthenticationCertificateExtension, state.initialPSKAuthentication)
					})
				}
			}
			t.Run(fmt.Sprintf("%04x/client%d/unsolicited-flag", suite, client), func(t *testing.T) {
				ch, sh := certificatePSKTestHellos(suite)
				ch = certificatePSKTestChange(ch, 33, nil, true)
				state := newBinTLS()
				_, err := state.handshakeMessage(client, reliabilityHS(1, tlsResumptionHello(true, nil, suite, ch...)), false)
				require.NoError(t, err)
				certificatePSKTestRefusal(t, state, 1-client, 2, tlsResumptionHello(false, nil, suite, sh...))
				sh = certificatePSKTestChange(sh, 33, nil, true)
				_, err = state.handshakeMessage(1-client, reliabilityHS(2, tlsResumptionHello(false, nil, suite, sh...)), false)
				require.NoError(t, err)
				require.Equal(t, tlsPSKAuthenticationOrdinary, state.initialPSKAuthentication)
				_, err = state.handshakeMessage(1-client, serverOrderMessage(t, 11, suite), true)
				require.ErrorContains(t, err, "PSK forbids initial", "unsolicited extension cannot disable the private PSK rule")
			})
		}
	}
}

func TestTLSCertificatePSKPartialObservationAndFallback(t *testing.T) {
	for _, client := range []int{0, 1} {
		for _, mode := range []string{"server-only", "declined-flag", "empty-client-shares", "unknown-mode", "PSK-only-mode-one"} {
			t.Run(fmt.Sprintf("client%d/%s", client, mode), func(t *testing.T) {
				ch, sh := certificatePSKTestHellos(0x1302)
				state := newBinTLS()
				switch mode {
				case "declined-flag":
					sh = certificatePSKTestChange(sh, 33, nil, true)
				case "empty-client-shares":
					ch = certificatePSKTestChange(ch, 51, []byte{0, 0}, false)
				case "unknown-mode":
					ch = certificatePSKTestChange(ch, 45, []byte{2, 1, 0xff}, false)
				case "PSK-only-mode-one":
					ch = certificatePSKTestChange(ch, 45, []byte{1, 1}, false)
				}
				if mode != "server-only" {
					_, err := state.handshakeMessage(client, reliabilityHS(1, tlsResumptionHello(true, nil, 0x1302, ch...)), false)
					require.NoError(t, err)
				}
				if mode == "empty-client-shares" {
					return
				} // HRR selection needs separate supported_groups evidence.
				m, err := state.handshakeMessage(1-client, reliabilityHS(2, tlsResumptionHello(false, nil, 0x1302, sh...)), false)
				require.NoError(t, err)
				want := tlsPSKAuthenticationCertificateExtension
				if mode == "declined-flag" {
					want = tlsPSKAuthenticationOrdinary
				}
				require.Equal(t, want, state.initialPSKAuthentication)
				if mode == "server-only" {
					require.Equal(t, "unavailable-client-hello", m["PSK Selection"].(map[string]any)["Offer Match"])
					require.Equal(t, -1, state.client, "partial observation cannot invent an offer or peer role")
				}
			})
		}
	}
}

func TestTLSCertificatePSKEncryptedExtensionsPlacement(t *testing.T) {
	for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
		for _, server := range []int{0, 1} {
			for _, fragmented := range []bool{false, true} {
				t.Run(fmt.Sprintf("%04x/server%d/fragmented%v", suite, server, fragmented), func(t *testing.T) {
					f, s := eeRecordFlow(t, suite, server, false)
					encoded := reliabilityHS(8, eeScopeBody(tlsResumptionExt{33, nil}))
					view, err := DecodeTLSEncryptedExtensions(encoded, 0x304, 0)
					var typed *ProtocolError
					require.ErrorAs(t, err, &typed)
					require.Equal(t, ErrMalformedMessage, typed.Kind)
					require.Nil(t, view)
					pieces := [][]byte{encoded}
					if fragmented {
						pieces = [][]byte{encoded[:7], encoded[7:]}
					}
					for i, part := range pieces {
						before := s.seq[server]
						e := &ProtocolEvent{Protocol: "tls", Raw: boundaryTLSRecord(t, s, server, before, part, 22, 3)}
						_, err = f.consumeTLS(server, e)
						require.Equal(t, before+1, s.seq[server])
						require.Equal(t, true, e.Session["Authentication Verified"])
						if i+1 < len(pieces) {
							require.NoError(t, err)
							continue
						}
						require.ErrorAs(t, err, &typed)
						require.Equal(t, ErrMalformedMessage, typed.Kind)
						require.NotContains(t, e.Session, "Handshake Messages")
						require.Equal(t, tlsPSKAuthenticationUnconstrained, s.initialPSKAuthentication)
					}
				})
			}
		}
	}
}

func FuzzTLSCertificatePSKHelloCompanions(f *testing.F) {
	for _, seed := range [][]byte{{0, 0, 0}, {1, 1, 1, 0xff}, {2, 0, 2}, {3, 1, 3}, {4, 0, 4}, {5, 1, 5}, {6, 0, 6}} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input []byte) {
		var program [3]byte
		copy(program[:], input)
		client := int(program[0] & 1)
		suite := []uint16{0x1301, 0x1302, 0x1303}[program[1]%3]
		ch, sh := certificatePSKTestHellos(suite)
		state := newBinTLS()
		_, err := state.handshakeMessage(client, reliabilityHS(1, tlsResumptionHello(true, nil, suite, ch...)), false)
		require.NoError(t, err)
		fault := program[2] % 7
		switch fault {
		case 1:
			data := []byte{0xff}
			if len(input) > 3 {
				data = input[3:min(len(input), 1027)]
			}
			sh = certificatePSKTestChange(sh, 33, data, false)
		case 2:
			sh = certificatePSKTestChange(sh, 41, nil, true)
		case 3:
			sh = certificatePSKTestChange(sh, 51, nil, true)
		case 4:
			sh = certificatePSKTestChange(sh, 51, []byte{0, 29, 0, 0}, false)
		case 5:
			sh = certificatePSKTestChange(sh, 43, []byte{3, 3}, false)
		case 6:
			// Same owned client bytes except this extension was never offered.
			state = newBinTLS()
			_, err = state.handshakeMessage(client, reliabilityHS(1, tlsResumptionHello(true, nil, suite, certificatePSKTestChange(ch, 33, nil, true)...)), false)
			require.NoError(t, err)
		}
		before := *state
		m, err := state.handshakeMessage(1-client, reliabilityHS(2, tlsResumptionHello(false, nil, suite, sh...)), false)
		if fault == 0 {
			require.NoError(t, err)
			require.NotNil(t, m)
			return
		}
		var typed *ProtocolError
		require.ErrorAs(t, err, &typed)
		require.Equal(t, ErrMalformedMessage, typed.Kind)
		require.Nil(t, m)
		require.Equal(t, before, *state)
	})
}

func TestTLSCertificatePSKStructuralAdmission(t *testing.T) {
	for _, client := range []bool{false, true} {
		for _, fault := range []uint16{33, 41, 51} {
			t.Run(fmt.Sprintf("client%v/fault%d", client, fault), func(t *testing.T) {
				ch, sh := certificatePSKTestHellos(0x1301)
				exts := sh
				if client {
					exts = ch
				}
				exts = certificatePSKTestChange(exts, fault, []byte{1}, fault != 33)
				body := tlsResumptionHello(client, nil, 0x1301, exts...)
				_, err := tlsHello(body, client, 0)
				var typed *ProtocolError
				require.ErrorAs(t, err, &typed)
				require.Equal(t, ErrMalformedMessage, typed.Kind, "structural refusal must precede the extension collection cap")
			})
		}
	}
}
