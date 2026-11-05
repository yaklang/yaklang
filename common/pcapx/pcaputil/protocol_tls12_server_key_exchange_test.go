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

func skeTestBody(ec bool, group, scheme uint16) []byte {
	var body []byte
	if ec {
		body = append(binary.BigEndian.AppendUint16([]byte{3}, group), 32)
		body = append(body, bytes.Repeat([]byte{7}, 32)...)
	} else {
		for _, value := range [][]byte{{0x01, 0x03}, {2}, {0x04, 0x05}} {
			body = binary.BigEndian.AppendUint16(body, uint16(len(value)))
			body = append(body, value...)
		}
	}
	body = binary.BigEndian.AppendUint16(body, scheme)
	body = binary.BigEndian.AppendUint16(body, 3)
	return append(body, 1, 2, 3)
}

func skeTestSession(t *testing.T, server int, suite uint16, ext ...tlsResumptionExt) *binTLS {
	t.Helper()
	s := newBinTLS()
	_, err := s.handshakeMessage(1-server, reliabilityHS(1, tlsResumptionHello(true, nil, suite, ext...)), false)
	require.NoError(t, err)
	_, err = s.handshakeMessage(server, reliabilityHS(2, tlsResumptionHello(false, nil, suite)), false)
	require.NoError(t, err)
	require.Nil(t, s.firstHello, "offer evidence must survive final ServerHello without retaining the full ClientHello")
	return s
}

func TestTLS12ServerKeyExchangeNativeAdmission(t *testing.T) {
	for server := 0; server < 2; server++ {
		for _, c := range []struct {
			name          string
			suite, scheme uint16
			ec            bool
		}{{"ECDHE-RSA-PSS", 0xc02f, 0x0804, true}, {"ECDHE-ECDSA", 0xc02b, 0x0403, true}, {"DHE-RSA", 0x009e, 0x0401, false}, {"DHE-DSS", 0x00a2, 0x0402, false}} {
			t.Run(fmt.Sprintf("server%d/%s", server, c.name), func(t *testing.T) {
				s := skeTestSession(t, server, c.suite, tlsResumptionExt{10, []byte{0, 2, 0, 29}},
					tlsResumptionExt{13, []byte{0, 2, byte(c.scheme >> 8), byte(c.scheme)}})
				wire := reliabilityHS(12, skeTestBody(c.ec, 29, c.scheme))
				m, err := s.handshakeMessage(server, wire, false)
				require.NoError(t, err)
				body, ok := m["Server Key Exchange"].(map[string]any)
				require.True(t, ok, "complete native body must be available, not just Type 12")
				require.Equal(t, c.scheme, body["Signature Algorithm"])
				require.Equal(t, []byte{1, 2, 3}, body["Signature"])
				require.Equal(t, wire, m["Encoded Server Key Exchange"])
				require.Equal(t, uint16(0x303), m["Server Key Exchange Version"])
				require.Equal(t, c.suite, m["Server Key Exchange Cipher Suite"])
				require.Contains(t, m["Server Key Exchange Status"], "signature not evaluated")
				wire[len(wire)-1] = 99
				require.Equal(t, []byte{1, 2, 3}, body["Signature"], "published bytes must be owned")
				require.Equal(t, byte(3), m["Encoded Server Key Exchange"].([]byte)[len(wire)-1])
			})
		}
	}
}

func TestTLS12ServerKeyExchangeRefusalAtomic(t *testing.T) {
	good := skeTestBody(true, 29, 0x0804)
	for server := 0; server < 2; server++ {
		for _, c := range []struct {
			name               string
			body               []byte
			client, tls13, rsa bool
		}{{name: "truncated-signature", body: good[:len(good)-1]},
			{name: "trailing-data", body: append(bytes.Clone(good), 0)},
			{name: "empty-point", body: []byte{3, 0, 29, 0, 8, 4, 0, 0}},
			{name: "unknown-curve-type", body: append([]byte{2}, good[1:]...)},
			{name: "GREASE-group", body: skeTestBody(true, 0x1a1a, 0x0804)},
			{name: "GREASE-signature", body: skeTestBody(true, 29, 0x2a2a)},
			{name: "unoffered-group", body: skeTestBody(true, 23, 0x0804)},
			{name: "unoffered-signature", body: skeTestBody(true, 29, 0x0401)},
			{name: "client-direction", body: good, client: true},
			{name: "TLS13-prohibited", body: good, tls13: true},
			{name: "RSA-prohibited", body: good, rsa: true}} {
			t.Run(fmt.Sprintf("server%d/%s", server, c.name), func(t *testing.T) {
				s := skeTestSession(t, server, 0xc02f, tlsResumptionExt{10, []byte{0, 2, 0, 29}}, tlsResumptionExt{13, []byte{0, 2, 8, 4}})
				dir := server
				if c.client {
					dir = 1 - server
				}
				if c.tls13 {
					s.version = 0x304
				}
				if c.rsa {
					s.suite = 0x009c
				}
				before := *s
				m, err := s.handshakeMessage(dir, reliabilityHS(12, c.body), c.tls13)
				require.Error(t, err)
				require.Nil(t, m)
				require.Equal(t, before, *s, "rejected message cannot mutate connection state")
				if !c.tls13 && !c.rsa {
					m, err = s.handshakeMessage(server, reliabilityHS(12, good), false)
					require.NoError(t, err, "later complete observed message remains recoverable")
					require.NotNil(t, m["Server Key Exchange"])
				}
			})
		}
	}
}

func TestTLS12ServerKeyExchangeGrammarAndContextBoundaries(t *testing.T) {
	for _, ec := range []bool{false, true} {
		suite := uint16(0x009e)
		if ec {
			suite = 0xc02f
		}
		wire := reliabilityHS(12, skeTestBody(ec, 29, 0x0804))
		for n := 0; n < len(wire); n++ {
			_, err := DecodeTLS12ServerKeyExchange(wire[:n], 0x303, suite)
			require.Error(t, err, "cropped complete handshake %d", n)
		}
		_, err := DecodeTLS12ServerKeyExchange(append(bytes.Clone(wire), 1), 0x303, suite)
		require.Error(t, err)
		view, err := DecodeTLS12ServerKeyExchange(wire, 0x303, suite)
		require.NoError(t, err)
		for _, span := range view.Ranges {
			require.Len(t, span.Path, 1)
			require.GreaterOrEqual(t, span.Offset, 4)
			require.LessOrEqual(t, span.Offset+span.Length, len(wire))
		}
		s := newBinTLS()
		s.version, s.suite = 0x303, suite
		m, err := s.handshakeMessage(1, wire, false)
		require.NoError(t, err, "absent ClientHello is not evidence of an unoffered value")
		require.Contains(t, m["Server Key Exchange Status"], "ClientHello offer context unavailable")
		e := &ProtocolEvent{Protocol: "tls", Session: map[string]any{"Handshake Messages": []map[string]any{m}}}
		fields := e.DisplayFieldValues()
		require.Equal(t, []any{uint64(0x0804)}, fields.FieldMap()["tls.handshake.sig_hash_alg"])
		require.Equal(t, []any{uint64(3)}, fields.FieldMap()["tls.handshake.sig_len"])
		require.Equal(t, []any{hex.EncodeToString([]byte{1, 2, 3})}, fields.FieldMap()["tls.handshake.sig"])
		for _, f := range fields.Values {
			require.Nil(t, f.Range, "reassembled body literals cannot borrow physical offsets")
		}
	}
	for _, version := range []uint16{0, 0x301, 0x302, 0x304} {
		s := newBinTLS()
		s.version, s.suite = version, 0xc02f
		m, err := s.handshakeMessage(1, reliabilityHS(12, skeTestBody(true, 29, 0x0804)), version == 0x304)
		if version == 0x304 {
			require.Error(t, err)
			continue
		}
		require.NoError(t, err)
		require.NotContains(t, m, "Server Key Exchange")
		require.Contains(t, m["Server Key Exchange Status"], "uninterpreted")
	}
}

func TestTLS12ServerKeyExchangeAuthenticationFamily(t *testing.T) {
	for _, suite := range []uint16{0xc02f, 0xc02b, 0x009e, 0x00a2} {
		for _, algorithm := range []uint16{0x0804, 0x0401, 0x0403, 0x0807, 0x0402} {
			t.Run(fmt.Sprintf("%04x/%04x", suite, algorithm), func(t *testing.T) {
				s := newBinTLS()
				s.version, s.suite = 0x303, suite
				ec := suite == 0xc02f || suite == 0xc02b
				_, err := s.handshakeMessage(1, reliabilityHS(12, skeTestBody(ec, 29, algorithm)), false)
				want := (suite == 0xc02b && (algorithm == 0x0403 || algorithm == 0x0807)) ||
					(suite == 0x00a2 && algorithm == 0x0402) ||
					((suite == 0xc02f || suite == 0x009e) && (algorithm == 0x0804 || algorithm == 0x0401))
				if want {
					require.NoError(t, err)
				} else {
					require.Error(t, err)
				}
			})
		}
	}
}

func TestTLS12ServerKeyExchangeRecordReassemblyOwnershipAndNewSession(t *testing.T) {
	for _, ec := range []bool{false, true} {
		for server := 0; server < 2; server++ {
			for _, deferred := range []bool{false, true} {
				suite := uint16(0x009e)
				if ec {
					suite = 0xc02f
				}
				wire := reliabilityHS(12, skeTestBody(ec, 29, 0x0804))
				for _, split := range []int{1, 3, 4, 5, 6, 7, 9, len(wire) - 4, len(wire) - 2, len(wire) - 1} {
					t.Run(fmt.Sprintf("%04x/server%d/deferred%v/split%d", suite, server, deferred, split), func(t *testing.T) {
						s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionPorts(40000, 443))
						require.NoError(t, err)
						defer s.Close("cleanup")
						s.(*captureSession).f.a.config.Deferred = deferred
						for _, step := range []struct {
							dir int
							hs  []byte
						}{
							{1 - server, reliabilityHS(1, tlsResumptionHello(true, nil, suite, tlsResumptionExt{10, []byte{0, 2, 0, 29}}, tlsResumptionExt{13, []byte{0, 2, 8, 4}}))},
							{server, reliabilityHS(2, tlsResumptionHello(false, nil, suite))},
						} {
							r := s.Feed(step.dir, time.Unix(10, 0), tlsResumptionRecord(step.hs))
							require.Nil(t, r.Err)
							require.Len(t, r.Events, 1)
						}
						first := s.Feed(server, time.Unix(11, 0), tlsResumptionRecord(wire[:split]))
						require.Nil(t, first.Err)
						for _, e := range first.Events {
							require.NotContains(t, e.DisplayFields(), "tls.handshake.sig", "partial body cannot publish a complete signature")
						}
						lastWire := tlsResumptionRecord(wire[split:])
						last := s.Feed(server, time.Unix(12, 0), lastWire)
						require.Nil(t, last.Err)
						require.Len(t, last.Events, 1)
						e := last.Events[0]
						fields, err := e.GetFields()
						require.NoError(t, err)
						before := e.DisplayFields()
						status := e.Status
						require.Equal(t, []any{"010203"}, before["tls.handshake.sig"])
						require.Equal(t, []any{uint64(0x0804)}, before["tls.handshake.sig_hash_alg"])
						clear(lastWire)
						tlsDTLSPoison(e, "caller")
						e.Status = status // the helper deliberately changes this public lifecycle label
						require.Equal(t, before, e.DisplayFields())
						after, err := e.GetFields()
						require.NoError(t, err)
						require.Equal(t, fields, after)
						fresh, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionPorts(40000, 443))
						require.NoError(t, err)
						defer fresh.Close("cleanup")
						r := fresh.Feed(server, time.Unix(13, 0), tlsResumptionRecord(wire))
						require.Nil(t, r.Err)
						for _, e := range r.Events {
							require.NotContains(t, e.DisplayFields(), "tls.handshake.sig", "a fresh flow cannot borrow the prior negotiated layout")
						}
					})
				}
			}
		}
	}
}

func TestTLS12ServerKeyExchangeOfferBoundsAndServerOnlyDirection(t *testing.T) {
	for _, groups := range [][]byte{{0, 2, 0, 29}, {0, 4, 0, 29}, {0, 0}} {
		s := skeTestSession(t, 1, 0xc02f, tlsResumptionExt{10, groups}, tlsResumptionExt{13, []byte{0, 2, 8, 4}})
		wire := reliabilityHS(12, skeTestBody(true, 29, 0x0804))
		before := *s
		_, err := s.handshakeMessage(1, wire, false, 0)
		require.Error(t, err)
		require.Equal(t, before, *s)
		var typed *ProtocolError
		require.ErrorAs(t, err, &typed)
		if bytes.Equal(groups, []byte{0, 2, 0, 29}) {
			require.Equal(t, ErrResourceExceeded, typed.Kind)
		} else {
			require.Equal(t, ErrMalformedMessage, typed.Kind)
		}
	}
	s := newBinTLS()
	_, err := s.handshakeMessage(1, reliabilityHS(2, tlsResumptionHello(false, nil, 0xc02f)), false)
	require.NoError(t, err)
	_, err = s.handshakeMessage(0, reliabilityHS(12, skeTestBody(true, 29, 0x0804)), false)
	require.Error(t, err, "a captured server-only hello still establishes sender direction")
	_, err = s.handshakeMessage(1, reliabilityHS(12, skeTestBody(true, 29, 0x0804)), false)
	require.NoError(t, err, "missing ClientHello cannot invent an offer mismatch")
}

func FuzzTLS12ServerKeyExchangeAdmission(f *testing.F) {
	for _, ec := range []bool{false, true} {
		for _, algorithm := range []uint16{0x0804, 0x0403, 0x1a1a} {
			f.Add(reliabilityHS(12, skeTestBody(ec, 29, algorithm)), ec)
		}
	}
	f.Fuzz(func(t *testing.T, wire []byte, ec bool) {
		if len(wire) == 0 || len(wire) > 262154 || wire[0] != 12 {
			return
		}
		suite := uint16(0x009e)
		if ec {
			suite = 0xc02f
		}
		s := newBinTLS()
		s.version, s.suite = 0x303, suite
		before := *s
		_, err := s.handshakeMessage(1, wire, false)
		if err != nil {
			require.Equal(t, before, *s)
			return
		}
		view, err := DecodeTLS12ServerKeyExchange(wire, 0x303, suite)
		require.NoError(t, err)
		for _, r := range view.Ranges {
			require.GreaterOrEqual(t, r.Offset, 4)
			require.LessOrEqual(t, r.Offset+r.Length, len(wire))
		}
	})
}
