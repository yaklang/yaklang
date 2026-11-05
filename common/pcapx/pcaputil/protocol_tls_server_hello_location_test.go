package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The location oracle is RFC8446 table1/4.1.3 and RFC9001 8.2. The
// extension values are authored declarations, not copied capture/key material.
func serverHelloLocationCases() []tlsResumptionExt {
	return []tlsResumptionExt{
		{0, nil}, {1, []byte{1}}, {5, nil}, {10, []byte{0, 2, 0, 29}},
		{13, []byte{0, 2, 4, 3}}, {16, []byte{0, 3, 2, 'h', '2'}},
		{18, []byte{0, 3, 0, 1, 0}}, {19, []byte{0}}, {20, []byte{0}},
		{21, nil}, {23, nil}, {27, []byte{2, 0, 1}}, {28, []byte{0, 64}},
		{35, nil}, {44, []byte{0, 1, 7}}, {47, []byte{0, 4, 0, 2, 0x30, 0}},
		{48, []byte{0, 0}}, {50, []byte{0, 2, 4, 3}}, {57, nil},
	}
}

func serverHelloLocationBody(suite uint16, sid []byte, extra tlsResumptionExt) []byte {
	ext := append([]tlsResumptionExt{{43, []byte{3, 4}}}, tlsResumptionDHExtensions(false)...)
	return tlsResumptionHello(false, sid, suite, append(ext, extra)...)
}

func serverHelloLocationClient(suite uint16, sid []byte) []byte {
	ext := append([]tlsResumptionExt{{43, []byte{2, 3, 4}}, {16, []byte{0, 3, 2, 'h', '2'}}}, tlsResumptionDHExtensions(true)...)
	return tlsResumptionHello(true, sid, suite, ext...)
}

func TestTLS13ServerHelloKnownExtensionLocations(t *testing.T) {
	for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
		for server := 0; server < 2; server++ {
			for _, known := range []bool{false, true} {
				for _, extra := range serverHelloLocationCases() {
					t.Run(fmt.Sprintf("%04x/server%d/client%t/extension%d", suite, server, known, extra.kind), func(t *testing.T) {
						s := newBinTLS()
						sid := []byte("same compatibility ID")
						if known {
							_, err := s.handshakeMessage(1-server, reliabilityHS(1, serverHelloLocationClient(suite, sid)), false)
							require.NoError(t, err)
						}
						before := *s
						firstHello := bytes.Clone(s.firstHello)
						body := serverHelloLocationBody(suite, sid, extra)
						m, err := s.handshakeMessage(server, reliabilityHS(2, body), false)
						var typed *ProtocolError
						require.ErrorAs(t, err, &typed)
						require.Equal(t, ErrMalformedMessage, typed.Kind)
						require.Contains(t, err.Error(), "ServerHello contains misplaced")
						require.Nil(t, m)
						require.Equal(t, before, *s, "a bad location cannot commit ALPN, cipher, epoch or hello/PSK observations")
						require.Equal(t, firstHello, s.firstHello, "retained ClientHello bytes must remain immutable")
						legal := tlsResumptionHello(false, sid, suite, append([]tlsResumptionExt{{43, []byte{3, 4}}}, tlsResumptionDHExtensions(false)...)...)
						_, err = s.handshakeMessage(server, reliabilityHS(2, legal), false)
						require.NoError(t, err, "refused trial must not obstruct a later valid declaration")
						require.Empty(t, s.alpn, "only the later EE can select TLS1.3 ALPN")
					})
				}
			}
		}
	}
}

func TestTLS13ServerHelloLocationNativeFragmentedRecord(t *testing.T) {
	for _, extra := range serverHelloLocationCases() {
		for _, chunks := range []int{1, 2, 3} {
			t.Run(fmt.Sprintf("extension%d/chunks%d", extra.kind, chunks), func(t *testing.T) {
				s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"))
				require.NoError(t, err)
				t.Cleanup(func() { s.Close("done"); require.Zero(t, s.Stats().BufferedBytes) })
				wire := helloObservationRecord(serverHelloLocationBody(0x1301, nil, extra))
				var events []*ProtocolEvent
				for i := 0; i < chunks; i++ {
					lo, hi := i*len(wire)/chunks, (i+1)*len(wire)/chunks
					r := s.Feed(1, time.Unix(100+int64(i), 0), bytes.Clone(wire[lo:hi]))
					events = append(events, r.Events...)
					if i == chunks-1 {
						require.NotNil(t, r.Err)
						require.Equal(t, ErrMalformedMessage, r.Err.Kind)
					}
				}
				require.Len(t, events, 1)
				e := events[0]
				require.Equal(t, wire, e.Raw)
				require.Equal(t, "malformed", e.Status)
				require.NotContains(t, e.Session, "Handshake Messages")
				require.Zero(t, e.Session["Negotiated Version"])
				_, err = e.GetFields()
				require.Error(t, err)
				n13AuditNoChild(t, s)
			})
		}
	}
}

func TestTLS13ServerHelloLocationObservationKeepsLiteralRanges(t *testing.T) {
	for _, extra := range serverHelloLocationCases() {
		t.Run(fmt.Sprintf("extension%d", extra.kind), func(t *testing.T) {
			wire := helloObservationRecord(serverHelloLocationBody(0x1302, []byte("offset shift"), extra))
			original := bytes.Clone(wire)
			o, err := ObserveTLSMisplacedServerHelloExtensionRecord(wire, 16)
			require.NoError(t, err)
			require.Equal(t, extra.kind, o.ExtensionType)
			require.NotEmpty(t, o.ExtensionName)
			require.Equal(t, len(extra.data), o.ExtensionDataLength)
			at := o.ExtensionOffset
			require.Equal(t, extra.kind, binary.BigEndian.Uint16(wire[at:at+2]))
			require.True(t, bytes.Equal(extra.data, wire[at+4:at+4+len(extra.data)]))
			require.Equal(t, original, wire)
			_, err = ObserveTLSMisplacedServerHelloExtensionRecord(wire, 1)
			var typed *ProtocolError
			require.ErrorAs(t, err, &typed)
			require.Equal(t, ErrResourceExceeded, typed.Kind)
		})
	}
}

func TestQUICServerHelloLocationRecoveryDoesNotCommitTLS(t *testing.T) {
	for server := 0; server < 2; server++ {
		for _, extra := range []tlsResumptionExt{{16, []byte{0, 3, 2, 'h', '2'}}, {44, []byte{0, 1, 7}}, {57, nil}} {
			for _, delivery := range []string{"whole", "split", "reordered", "duplicate-tail"} {
				t.Run(fmt.Sprintf("server%d/extension%d/%s", server, extra.kind, delivery), func(t *testing.T) {
					q := &binQUIC{native: true, byteLimit: 1 << 20}
					ch := reliabilityHS(1, serverHelloLocationClient(0x1301, nil))
					require.NoError(t, q.feedCrypto(1-server, quicSpaceInitial, 0, ch, map[string]any{}, 4096))
					before := *q.tls
					first := bytes.Clone(q.tls.firstHello)
					bad := reliabilityHS(2, serverHelloLocationBody(0x1301, nil, extra))
					middle := len(bad) / 2
					if delivery == "split" {
						require.NoError(t, q.feedCrypto(server, quicSpaceInitial, 0, bad[:middle], map[string]any{}, 4096))
					}
					if delivery == "reordered" || delivery == "duplicate-tail" {
						require.NoError(t, q.feedCrypto(server, quicSpaceInitial, uint64(middle), bad[middle:], map[string]any{}, 4096))
						if delivery == "duplicate-tail" {
							require.NoError(t, q.feedCrypto(server, quicSpaceInitial, uint64(middle), bad[middle:], map[string]any{}, 4096))
						}
					}
					info := map[string]any{}
					offset, suffix := uint64(0), bad
					if delivery == "split" {
						offset, suffix = uint64(middle), bad[middle:]
					}
					if delivery == "reordered" || delivery == "duplicate-tail" {
						suffix = bad[:middle]
					}
					err := q.feedCrypto(server, quicSpaceInitial, offset, suffix, info, 4096)
					require.ErrorContains(t, err, "ServerHello contains misplaced")
					require.Equal(t, before, *q.tls)
					require.Equal(t, first, q.tls.firstHello)
					require.Zero(t, q.cryptoRead[server][quicSpaceInitial])
					require.NotContains(t, info, "TLS Handshakes")
					require.NotContains(t, info, "ALPN")
					require.Empty(t, q.alpn)
					require.False(t, q.loadedKeys)
				})
			}
		}
	}
}

func TestTLS13ServerHelloLocationPreservesOtherProfiles(t *testing.T) {
	for _, version := range []uint16{0x303, 0x305} {
		t.Run(fmt.Sprintf("selected-version%04x", version), func(t *testing.T) {
			ext := []tlsResumptionExt{{16, []byte{0, 3, 2, 'h', '2'}}}
			suite := uint16(0xc02f)
			if version == 0x305 {
				ext = append(ext, tlsResumptionExt{43, []byte{3, 5}})
				suite = 0x1301
			}
			s := newBinTLS()
			body := tlsResumptionHello(false, nil, suite, ext...)
			_, err := s.handshakeMessage(1, reliabilityHS(2, body), false)
			require.NoError(t, err, "a TLS1.3 location rule cannot be imposed on legacy or future profiles")
			require.Equal(t, version, s.version)
		})
	}
	for _, kind := range []uint16{0x0a0a, 0x7a7a, 0xffa5} {
		t.Run(fmt.Sprintf("opaque-extension%04x", kind), func(t *testing.T) {
			s := newBinTLS()
			body := serverHelloLocationBody(0x1301, nil, tlsResumptionExt{kind, []byte{1, 2}})
			_, err := s.handshakeMessage(1, reliabilityHS(2, body), false)
			if kind == 0x0a0a || kind == 0x7a7a {
				require.ErrorContains(t, err, "reserved GREASE extension", "RFC8701 forbids a server selecting these reserved proposal values")
			} else {
				require.NoError(t, err)
			}
			_, err = ObserveTLSMisplacedServerHelloExtensionRecord(helloObservationRecord(body), 16)
			require.Error(t, err)
		})
	}
}

func FuzzTLS13ServerHelloLocationIsolation(f *testing.F) {
	for _, seed := range []struct {
		which, server, sid byte
		cut                uint16
	}{
		{0, 0, 0, 0xffff}, {5, 1, 16, 0xffff}, {14, 1, 32, 0xffff},
		{18, 0, 0, 0xffff}, {19, 1, 1, 0xffff}, {5, 0, 32, 50},
	} {
		f.Add(seed.which, seed.server, seed.sid, seed.cut)
	}
	f.Fuzz(func(t *testing.T, which, server, sid byte, cut uint16) {
		cases := append(serverHelloLocationCases(), tlsResumptionExt{0xffa5, []byte{1, 2}})
		i := int(which) % len(cases)
		body := serverHelloLocationBody(0x1301, bytes.Repeat([]byte{7}, int(sid)%33), cases[i])
		s, before := newBinTLS(), newBinTLS()
		m, err := s.handshakeMessage(int(server&1), reliabilityHS(2, body), false)
		if i < len(cases)-1 {
			require.Error(t, err)
			require.Nil(t, m)
			require.Equal(t, *before, *s)
		} else {
			require.NoError(t, err)
		}
		wire := helloObservationRecord(body)
		if cut != 0xffff {
			wire = wire[:int(cut)%(len(wire)+1)]
		}
		original := bytes.Clone(wire)
		o, err := ObserveTLSMisplacedServerHelloExtensionRecord(wire, 16)
		require.Equal(t, original, wire)
		if err == nil {
			require.Less(t, i, len(cases)-1)
			require.Equal(t, cases[i].kind, o.ExtensionType)
			require.True(t, bytes.Equal(cases[i].data, wire[o.ExtensionOffset+4:o.ExtensionOffset+4+o.ExtensionDataLength]))
		} else {
			require.Nil(t, o)
		}
	})
}
