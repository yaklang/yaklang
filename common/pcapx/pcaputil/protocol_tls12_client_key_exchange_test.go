package pcaputil

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func ckeTestBody(profile string) []byte {
	if profile == "ECDHEClient" {
		return append([]byte{32}, bytes.Repeat([]byte{7}, 32)...)
	}
	if profile == "DHEClient" {
		return []byte{0, 2, 4, 5}
	}
	return []byte{0, 3, 1, 2, 3}
}

func TestTLS12ClientKeyExchangeNativeAdmissionAndOwnedAliases(t *testing.T) {
	for server := 0; server < 2; server++ {
		for _, c := range []struct {
			profile string
			suite   uint16
			key     string
			alias   string
		}{
			{"ECDHEClient", 0xc02f, "Client EC Point", "tls.handshake.client_point"},
			{"DHEClient", 0x009e, "Client DH Public Value", "tls.handshake.yc"},
			{"RSAClient", 0x009c, "Encrypted Premaster", "tls.handshake.epms"},
		} {
			t.Run(fmt.Sprintf("server%d/%s", server, c.profile), func(t *testing.T) {
				s := skeTestSession(t, server, c.suite)
				wire := reliabilityHS(16, ckeTestBody(c.profile))
				m, err := s.handshakeMessage(1-server, wire, false)
				require.NoError(t, err)
				body, ok := m["Client Key Exchange"].(map[string]any)
				require.True(t, ok, "a complete native CKE needs its selected body, not only Type 16")
				require.Equal(t, c.profile, m["Client Key Exchange Profile"])
				require.Equal(t, uint16(0x303), m["Client Key Exchange Version"])
				require.Equal(t, c.suite, m["Client Key Exchange Cipher Suite"])
				value := bytes.Clone(body[c.key].([]byte))
				require.True(t, s.clientKeyExchangeObserved)
				wire[len(wire)-1] ^= 0xff
				require.Equal(t, value, body[c.key], "caller mutation cannot change the admitted public value")
				require.Equal(t, value[len(value)-1], m["Encoded Client Key Exchange"].([]byte)[len(wire)-1])
				e := &ProtocolEvent{Protocol: "tls", Session: map[string]any{"Handshake Messages": []map[string]any{m}}}
				fields := e.DisplayFieldValues()
				require.Equal(t, []any{hex.EncodeToString(value)}, fields.FieldMap()[c.alias])
				for _, f := range fields.Values {
					require.Nil(t, f.Range, "reassembled handshake cannot borrow a physical frame range")
				}
				require.Contains(t, m["Client Key Exchange Status"], "not evaluated")
			})
		}
	}
}

func TestTLS12ClientKeyExchangeRejectionIsAtomicAndRecoverable(t *testing.T) {
	for server := 0; server < 2; server++ {
		for _, profile := range []string{"ECDHEClient", "DHEClient", "RSAClient"} {
			suite := map[string]uint16{"ECDHEClient": 0xc02f, "DHEClient": 0x009e, "RSAClient": 0x009c}[profile]
			good := ckeTestBody(profile)
			for _, kind := range []string{"truncated", "trailing", "server-direction", "TLS13", "after-CCS", "authenticated", "after-Finished", "duplicate"} {
				t.Run(fmt.Sprintf("server%d/%s/%s", server, profile, kind), func(t *testing.T) {
					s := skeTestSession(t, server, suite)
					body, dir, authenticated := bytes.Clone(good), 1-server, false
					switch kind {
					case "truncated":
						body = body[:len(body)-1]
					case "trailing":
						body = append(body, 0)
					case "server-direction":
						dir = server
					case "TLS13":
						s.version, authenticated = 0x304, true
					case "after-CCS":
						s.ccs[dir] = true
					case "authenticated":
						authenticated = true
					case "after-Finished":
						s.app[dir] = true
					case "duplicate":
						_, err := s.handshakeMessage(dir, reliabilityHS(16, good), false)
						require.NoError(t, err)
					}
					before := *s
					m, err := s.handshakeMessage(dir, reliabilityHS(16, body), authenticated)
					require.Error(t, err)
					require.Nil(t, m)
					require.Equal(t, before, *s, "refusal cannot consume the valid CKE or advance the epoch")
					if kind == "truncated" || kind == "trailing" || kind == "server-direction" {
						m, err = s.handshakeMessage(1-server, reliabilityHS(16, good), false)
						require.NoError(t, err)
						require.NotNil(t, m["Client Key Exchange"])
					}
				})
			}
		}
	}
}

func TestTLS12ClientKeyExchangeGrammarAndMissingContext(t *testing.T) {
	for _, c := range []struct {
		suite   uint16
		profile string
	}{{0xc02b, "ECDHEClient"}, {0x009f, "DHEClient"}, {0x009d, "RSAClient"}} {
		wire := reliabilityHS(16, ckeTestBody(c.profile))
		for n := 0; n < len(wire); n++ {
			_, err := DecodeTLS12ClientKeyExchange(wire[:n], 0x303, c.suite)
			require.Error(t, err, "truncated complete encoding %d", n)
		}
		_, err := DecodeTLS12ClientKeyExchange(append(bytes.Clone(wire), 0), 0x303, c.suite)
		require.Error(t, err)
		for _, context := range []string{"suite-only", "server-only", "unknown-suite", "TLS10", "TLS11", "unknown-version"} {
			t.Run(fmt.Sprintf("%s/%s", c.profile, context), func(t *testing.T) {
				s := newBinTLS()
				s.version, s.suite = 0x303, c.suite
				switch context {
				case "server-only":
					s = newBinTLS()
					_, err := s.handshakeMessage(1, reliabilityHS(2, tlsResumptionHello(false, nil, c.suite)), false)
					require.NoError(t, err)
				case "unknown-suite":
					s.suite = 0x00a8 // PSK has a different body; never guess from its prefix.
				case "TLS10":
					s.version = 0x301
				case "TLS11":
					s.version = 0x302
				case "unknown-version":
					s.version = 0
				}
				m, err := s.handshakeMessage(0, wire, false)
				require.NoError(t, err)
				if context == "server-only" || context == "suite-only" {
					require.Equal(t, c.profile, m["Client Key Exchange Profile"])
					if context == "suite-only" {
						require.Contains(t, m["Client Key Exchange Status"], "sender role unavailable")
					}
				} else {
					require.NotContains(t, m, "Client Key Exchange")
					require.Contains(t, m["Client Key Exchange Status"], "uninterpreted")
					require.False(t, s.clientKeyExchangeObserved)
				}
			})
		}
	}
	_, err := DecodeTLS12ClientKeyExchange(reliabilityHS(16, []byte{0}), 0x303, 0xc02f)
	require.Error(t, err, "an ECPoint cannot be empty")
	_, err = DecodeTLS12ClientKeyExchange(reliabilityHS(16, []byte{0, 0}), 0x303, 0x009e)
	require.Error(t, err, "explicit DHE client public value cannot be empty")
	rsa, err := DecodeTLS12ClientKeyExchange(reliabilityHS(16, []byte{0, 0}), 0x303, 0x009c)
	require.NoError(t, err, "RSA ciphertext vector has a zero lower wire bound, not a validity assertion")
	require.Empty(t, rsa.Fields["Encrypted Premaster"])
	require.Len(t, rsa.Ranges, 1, "zero-length ciphertext has no fabricated selectable range")
}

func TestTLS12ClientKeyExchangeSessionFragmentationAndReuse(t *testing.T) {
	for server := 0; server < 2; server++ {
		for _, profile := range []string{"ECDHEClient", "DHEClient", "RSAClient"} {
			suite := map[string]uint16{"ECDHEClient": 0xc02f, "DHEClient": 0x009e, "RSAClient": 0x009c}[profile]
			alias := map[string]string{"ECDHEClient": "tls.handshake.client_point", "DHEClient": "tls.handshake.yc", "RSAClient": "tls.handshake.epms"}[profile]
			wire := reliabilityHS(16, ckeTestBody(profile))
			for split := 1; split < len(wire); split++ {
				t.Run(fmt.Sprintf("server%d/%s/%d", server, profile, split), func(t *testing.T) {
					s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionPorts(40000, 443))
					require.NoError(t, err)
					defer s.Close("cleanup")
					for _, step := range []struct {
						dir  int
						wire []byte
					}{
						{1 - server, reliabilityHS(1, tlsResumptionHello(true, nil, suite))},
						{server, reliabilityHS(2, tlsResumptionHello(false, nil, suite))},
					} {
						r := s.Feed(step.dir, time.Unix(10, 0), tlsResumptionRecord(step.wire))
						require.Nil(t, r.Err)
					}
					first := s.Feed(1-server, time.Unix(11, 0), tlsResumptionRecord(wire[:split]))
					require.Nil(t, first.Err)
					for _, e := range first.Events {
						require.NotContains(t, e.DisplayFields(), alias, "partial vector cannot publish a complete public value")
					}
					lastWire := tlsResumptionRecord(wire[split:])
					last := s.Feed(1-server, time.Unix(12, 0), lastWire)
					require.Nil(t, last.Err)
					require.Len(t, last.Events, 1)
					e := last.Events[0]
					before := e.DisplayFields()
					require.Contains(t, before, alias)
					status := e.Status
					clear(lastWire)
					tlsDTLSPoison(e, "caller")
					e.Status = status
					require.Equal(t, before, e.DisplayFields())
					fresh, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionPorts(40000, 443))
					require.NoError(t, err)
					defer fresh.Close("cleanup")
					freshResult := fresh.Feed(1-server, time.Unix(13, 0), tlsResumptionRecord(wire))
					require.Nil(t, freshResult.Err)
					for _, e := range freshResult.Events {
						require.NotContains(t, e.DisplayFields(), alias, "new owner cannot inherit the old cipher layout")
					}
				})
			}
		}
	}
}

func FuzzTLS12ClientKeyExchangeAdmission(f *testing.F) {
	for i, profile := range []string{"ECDHEClient", "DHEClient", "RSAClient"} {
		f.Add(reliabilityHS(16, ckeTestBody(profile)), uint8(i))
		f.Add(reliabilityHS(16, []byte{0, 0}), uint8(i))
		if i < 2 {
			f.Add(reliabilityHS(16, nil), uint8(i))
		}
	}
	f.Fuzz(func(t *testing.T, wire []byte, choice uint8) {
		if len(wire) > 262154 {
			return
		}
		suite := []uint16{0xc02f, 0x009e, 0x009c}[int(choice)%3]
		s := newBinTLS()
		s.version, s.suite, s.client = 0x303, suite, 0
		before := *s
		m, err := s.handshakeMessage(0, wire, false)
		if err != nil {
			require.Equal(t, before, *s)
			return
		}
		if len(wire) < 1 || wire[0] != 16 {
			return
		}
		view, err := DecodeTLS12ClientKeyExchange(wire, 0x303, suite)
		if typed, ok := err.(*ProtocolError); ok && typed.Kind == ErrContextRequired {
			require.NotContains(t, m, "Client Key Exchange")
			require.Contains(t, m["Client Key Exchange Status"], "certificate context unavailable")
			require.True(t, s.clientKeyExchangeObserved)
			return
		}
		require.NoError(t, err)
		require.Equal(t, view.Fields, m["Client Key Exchange"])
		for _, r := range view.Ranges {
			require.GreaterOrEqual(t, r.Offset, 4)
			require.LessOrEqual(t, r.Offset+r.Length, len(wire))
		}
	})
}

// A deliberately valid two-byte length prefix is insufficient evidence of
// either DHE/RSA when the caller has not observed a supported suite.
func TestTLS12ClientKeyExchangeUnknownLayoutNeverGuessed(t *testing.T) {
	for _, suite := range []uint16{0, 0x0031, 0x00a8, 0xc035, 0x1a1a} {
		s := newBinTLS()
		s.version, s.suite = 0x303, suite
		body := binary.BigEndian.AppendUint16(nil, 3)
		body = append(body, 1, 2, 3)
		m, err := s.handshakeMessage(0, reliabilityHS(16, body), false)
		require.NoError(t, err)
		require.NotContains(t, m, "Client Key Exchange")
		require.False(t, s.clientKeyExchangeObserved)
	}
}

func TestTLS12ClientKeyExchangeImplicitValueNeedsCertificateContext(t *testing.T) {
	for server := 0; server < 2; server++ {
		for _, suite := range []uint16{0xc02f, 0x009e} {
			t.Run(fmt.Sprintf("server%d/suite%04x", server, suite), func(t *testing.T) {
				wire := reliabilityHS(16, nil)
				view, err := DecodeTLS12ClientKeyExchange(wire, 0x303, suite)
				require.Nil(t, view)
				var typed *ProtocolError
				require.ErrorAs(t, err, &typed)
				require.Equal(t, ErrContextRequired, typed.Kind, "unknown certificate selection is not proof of a malformed explicit vector")
				state := skeTestSession(t, server, suite)
				m, err := state.handshakeMessage(1-server, wire, false)
				require.NoError(t, err)
				require.NotContains(t, m, "Client Key Exchange")
				require.NotContains(t, m, "Encoded Client Key Exchange")
				require.Contains(t, m["Client Key Exchange Status"], "certificate context unavailable")
				require.True(t, state.clientKeyExchangeObserved)
				previous := *state
				_, err = state.handshakeMessage(1-server, reliabilityHS(16, ckeTestBody(tls12ClientKeyExchangeProfile(suite))), false)
				require.Error(t, err, "a second CKE is not a replacement for an observed opaque first CKE")
				require.Equal(t, previous, *state)
			})
		}
	}
	_, err := DecodeTLS12ClientKeyExchange(reliabilityHS(16, nil), 0x303, 0x009c)
	require.Error(t, err, "RSA still requires its two-byte ciphertext-vector length")
	for _, suite := range []uint16{0xc02f, 0x009e} {
		_, err := DecodeTLS12ClientKeyExchange([]byte{12, 0, 0, 0}, 0x303, suite)
		require.Error(t, err)
		_, err = DecodeTLS12ClientKeyExchange([]byte{16, 0, 0, 1}, 0x303, suite)
		require.Error(t, err, "a truncated nonempty body cannot become implicit")
	}
}
