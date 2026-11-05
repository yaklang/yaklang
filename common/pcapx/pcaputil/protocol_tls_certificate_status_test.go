package pcaputil

import (
	"bytes"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func statusTestSession(t *testing.T, server int, offer, ack bool) *binTLS {
	t.Helper()
	s := newBinTLS()
	var ch, sh []tlsResumptionExt
	if offer {
		ch = append(ch, tlsResumptionExt{5, []byte{1, 0, 0, 0, 0}})
	}
	if ack {
		sh = append(sh, tlsResumptionExt{5, nil})
	}
	_, err := s.handshakeMessage(1-server, reliabilityHS(1, tlsResumptionHello(true, nil, 0xc02f, ch...)), false)
	require.NoError(t, err)
	_, err = s.handshakeMessage(server, reliabilityHS(2, tlsResumptionHello(false, nil, 0xc02f, sh...)), false)
	require.NoError(t, err)
	_, err = s.handshakeMessage(server, reliabilityHS(11, []byte{0, 0, 4, 0, 0, 1, 0x30}), false)
	require.NoError(t, err)
	return s
}

func TestTLSCertificateStatusNativeWireAndRefusal(t *testing.T) {
	good := reliabilityHS(22, []byte{1, 0, 0, 5, 0x30, 3, 0x0a, 1, 3})
	for server := 0; server < 2; server++ {
		t.Run(fmt.Sprintf("server%d/owned-body", server), func(t *testing.T) {
			s := statusTestSession(t, server, true, true)
			m, err := s.handshakeMessage(server, good, false)
			require.NoError(t, err)
			body, ok := m["Certificate Status"].(map[string]any)
			require.True(t, ok, "Type 22 must have complete typed status body")
			require.Equal(t, byte(1), body["Status Type"])
			require.Equal(t, uint32(5), body["OCSP Response Length"])
			require.Equal(t, []byte{0x30, 3, 0x0a, 1, 3}, body["OCSP Response"])
			require.Equal(t, good, m["Encoded Certificate Status"])
			require.Contains(t, m["Certificate Status Validation"], "not evaluated")
			e := &ProtocolEvent{Protocol: "tls", Session: map[string]any{"Handshake Messages": []map[string]any{m}}}
			f := e.DisplayFieldValues()
			require.Equal(t, []any{uint64(5)}, f.FieldMap()["tls.handshake.ocsp_response_len"])
		})
		for _, kind := range []string{"zero-length", "truncated", "trailing", "client-sender", "after-CCS", "authenticated", "duplicate", "after-SKE", "no-client-offer", "no-server-ack", "TLS13"} {
			t.Run(fmt.Sprintf("server%d/%s", server, kind), func(t *testing.T) {
				s := statusTestSession(t, server, kind != "no-client-offer", kind != "no-server-ack")
				dir, auth := server, false
				b := bytes.Clone(good)
				switch kind {
				case "zero-length":
					b = reliabilityHS(22, []byte{1, 0, 0, 0})
				case "truncated":
					b = reliabilityHS(22, []byte{1, 0, 0, 5, 0x30, 3, 0x0a, 1})
				case "trailing":
					b = reliabilityHS(22, []byte{1, 0, 0, 4, 0x30, 3, 0x0a, 1, 3})
				case "client-sender":
					dir = 1 - server
				case "after-CCS":
					s.ccs[dir] = true
				case "authenticated":
					auth = true
				case "duplicate":
					_, err := s.handshakeMessage(server, good, false)
					require.NoError(t, err)
				case "after-SKE":
					_, err := s.handshakeMessage(server, reliabilityHS(12, skeTestBody(true, 29, 0x0804)), false)
					require.NoError(t, err)
				case "TLS13":
					s.version = 0x304
					auth = true
				}
				before := *s
				m, err := s.handshakeMessage(dir, b, auth)
				require.Error(t, err)
				require.Nil(t, m)
				require.Equal(t, before, *s, "rejected status must not consume the valid phase")
				if kind == "zero-length" || kind == "truncated" || kind == "trailing" || kind == "client-sender" {
					m, err = s.handshakeMessage(server, good, false)
					require.NoError(t, err)
					require.NotNil(t, m["Certificate Status"])
				}
			})
		}
	}
}

func TestTLSCertificateStatusMissingContextAndOpaqueResponse(t *testing.T) {
	wire := reliabilityHS(22, []byte{1, 0, 0, 1, 0xff})
	s := newBinTLS()
	s.version = 0x303
	m, err := s.handshakeMessage(1, wire, false)
	require.NoError(t, err)
	body := m["Certificate Status"].(map[string]any)
	require.Equal(t, false, body["OCSP DER Decoded"])
	require.Equal(t, false, body["OCSP Response Validated"])
	require.Contains(t, m["Certificate Status Validation"], "Hello negotiation unavailable")
	require.Contains(t, m["Certificate Status Validation"], "sender role unavailable")
	wire[8] = 0
	require.Equal(t, []byte{0xff}, body["OCSP Response"], "native body must own its response")
	for _, version := range []uint16{0, 0x300, 0x305} {
		s := newBinTLS()
		s.version = version
		m, err := s.handshakeMessage(1, reliabilityHS(22, []byte{1, 0, 0, 1, 7}), false)
		require.NoError(t, err)
		require.NotContains(t, m, "Certificate Status")
		require.Contains(t, m["Certificate Status Validation"], "unavailable")
	}
	s = newBinTLS()
	s.version = 0x303
	m, err = s.handshakeMessage(1, reliabilityHS(22, []byte{2, 7}), false)
	require.NoError(t, err)
	require.NotContains(t, m, "Certificate Status")
	require.Contains(t, m["Certificate Status Validation"], "unsupported status_type")
	s = statusTestSession(t, 1, true, true)
	before := *s
	m, err = s.handshakeMessage(1, reliabilityHS(22, []byte{2, 7}), false)
	require.ErrorContains(t, err, "differs from observed OCSP request")
	require.Nil(t, m)
	require.Equal(t, before, *s)
	s = statusTestSession(t, 1, true, true)
	s.certificateStatus.v2Offered, s.certificateStatus.v2Selected = true, true
	m, err = s.handshakeMessage(1, reliabilityHS(22, []byte{2, 7}), false)
	require.NoError(t, err)
	require.NotContains(t, m, "Certificate Status")
	require.Contains(t, m["Certificate Status Validation"], "status_request_v2 negotiation unsupported")
}

func TestTLSCertificateStatusHelloVectorsAndGapOrder(t *testing.T) {
	for _, v := range [][]byte{nil, {1}, {1, 0, 2, 0, 0, 0, 0}, {1, 0, 3, 0, 2, 7, 0, 0}, {1, 0, 0, 0, 1}, {1, 0, 0, 0, 0, 7}} {
		s := newBinTLS()
		before := *s
		m, err := s.handshakeMessage(0, reliabilityHS(1, tlsResumptionHello(true, nil, 0xc02f, tlsResumptionExt{5, v})), false)
		require.Error(t, err)
		require.Nil(t, m)
		require.Equal(t, before, *s)
	}
	s := newBinTLS()
	_, err := s.handshakeMessage(0, reliabilityHS(1, tlsResumptionHello(true, nil, 0xc02f, tlsResumptionExt{5, []byte{1, 0, 0, 0, 0}})), false)
	require.NoError(t, err)
	before := *s
	m, err := s.handshakeMessage(1, reliabilityHS(2, tlsResumptionHello(false, nil, 0xc02f, tlsResumptionExt{5, []byte{0}})), false)
	require.ErrorContains(t, err, "acknowledgement must be empty")
	require.Nil(t, m)
	require.Equal(t, before, *s)
	_, err = s.handshakeMessage(1, reliabilityHS(2, tlsResumptionHello(false, nil, 0xc02f, tlsResumptionExt{5, nil})), false)
	require.NoError(t, err)
	good := reliabilityHS(22, []byte{1, 0, 0, 1, 7})
	before = *s
	m, err = s.handshakeMessage(1, good, false)
	require.ErrorContains(t, err, "immediately follow server Certificate")
	require.Nil(t, m)
	require.Equal(t, before, *s)
	s.handshakeGap[1] = true
	m, err = s.handshakeMessage(1, good, false)
	require.NoError(t, err)
	require.Contains(t, m["Certificate Status Validation"], "preceding message order unavailable")
	s = statusTestSession(t, 1, true, true)
	_, err = s.handshakeMessage(1, reliabilityHS(12, skeTestBody(true, 29, 0x0804)), false)
	require.NoError(t, err)
	s.handshakeGap[1] = true
	before = *s
	m, err = s.handshakeMessage(1, good, false)
	require.Error(t, err)
	require.Nil(t, m)
	require.Equal(t, before, *s, "a gap cannot erase an observed later message")
}

func FuzzTLSCertificateStatusEnvelope(f *testing.F) {
	for _, b := range [][]byte{{22, 0, 0, 9, 1, 0, 0, 5, 0x30, 3, 0x0a, 1, 3}, {22, 0, 0, 5, 1, 0, 0, 1, 7}, {22, 0, 0, 4, 1, 0, 0, 0}, {22, 0, 0, 5, 1, 0, 0, 2, 7}, {22, 0, 0, 5, 2, 0, 0, 1, 7}, {22, 0, 0, 0}, {}} {
		f.Add(b)
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 2<<20 {
			t.Skip()
		}
		v, err := DecodeTLSCertificateStatus(b, 0x303)
		if err != nil {
			require.Nil(t, v)
			return
		}
		require.Equal(t, byte(22), b[0])
		require.Equal(t, len(b)-4, tlsU24(b[1:4]))
		require.Equal(t, byte(1), b[4])
		require.Equal(t, len(b)-8, tlsU24(b[5:8]))
		require.Greater(t, len(b), 8)
		owned := bytes.Clone(b[8:])
		require.Equal(t, owned, v.Fields["OCSP Response"])
		for _, r := range v.Ranges {
			require.GreaterOrEqual(t, r.Offset, 4)
			require.Greater(t, r.Length, 0)
			require.LessOrEqual(t, r.Offset+r.Length, len(b))
		}
		s := newBinTLS()
		s.version = 0x303
		_, err = s.handshakeMessage(1, b, false)
		require.NoError(t, err)
		before := *s
		m, err := s.handshakeMessage(1, b, false)
		require.Error(t, err)
		require.Nil(t, m)
		require.Equal(t, before, *s)
	})
}
