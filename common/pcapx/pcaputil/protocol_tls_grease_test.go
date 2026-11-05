package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func greaseTestHello(value uint16, kind, profile string) []byte {
	suite := uint16(0x1301)
	ext := append([]tlsResumptionExt{{43, []byte{3, 4}}}, tlsResumptionDHExtensions(false)...)
	if profile == "legacy" {
		suite, ext = 0xc02f, nil
	} else if profile == "retry" {
		ext[1].data = []byte{0, 23}
	}
	switch kind {
	case "version":
		if profile != "legacy" {
			ext[0].data = binary.BigEndian.AppendUint16(nil, value)
		}
	case "cipher suite":
		suite = value
	case "extension":
		ext = append(ext, tlsResumptionExt{value, []byte{1, 2, 3}})
	case "key_share group":
		ext[1].data = bytes.Clone(ext[1].data)
		binary.BigEndian.PutUint16(ext[1].data, value)
	case "ALPN":
		ext = append(ext, tlsResumptionExt{16, []byte{0, 3, 2, byte(value >> 8), byte(value)}})
	}
	body := tlsResumptionHello(false, nil, suite, ext...)
	if profile == "legacy" && kind == "version" {
		binary.BigEndian.PutUint16(body, value)
	}
	if profile == "retry" {
		copy(body[2:34], tlsRetryRandom[:])
	}
	return body
}

func TestTLSGREASEServerSelectionsAndLiteralRanges(t *testing.T) {
	for i := uint16(0); i < 16; i++ {
		value := uint16(0x0a0a) + i*0x1010
		for server := 0; server < 2; server++ {
			for _, profile := range []string{"final", "legacy", "retry"} {
				kinds := []string{"version", "cipher suite", "extension", "key_share group"}
				if profile == "legacy" {
					kinds[3] = "ALPN"
				}
				for _, kind := range kinds {
					t.Run(fmt.Sprintf("%04x/server%d/%s/%s", value, server, profile, kind), func(t *testing.T) {
						body := greaseTestHello(value, kind, profile)
						s := newBinTLS()
						before := *s
						m, err := s.handshakeMessage(server, reliabilityHS(2, body), false)
						var typed *ProtocolError
						require.ErrorAs(t, err, &typed, "RFC8701 reserved selections are provably invalid without a captured ClientHello")
						require.Equal(t, ErrMalformedMessage, typed.Kind)
						require.Contains(t, err.Error(), "reserved GREASE "+kind)
						require.Nil(t, m)
						require.Equal(t, before, *s)
						wire := helloObservationRecord(body)
						original := bytes.Clone(wire)
						o, observationErr := ObserveTLSGREASEServerHelloRecord(wire, 16)
						require.NoError(t, observationErr)
						require.Equal(t, kind, o.SelectionKind)
						require.Equal(t, value, o.Value)
						require.Equal(t, profile == "retry", o.HelloRetryRequest)
						require.Equal(t, err.Error(), o.RefusalReason)
						require.Equal(t, value, binary.BigEndian.Uint16(wire[o.Offset:o.Offset+2]))
						require.Equal(t, original, wire)
					})
				}
			}
		}
	}
}

func TestTLSGREASEClientOffersAndOrdinaryUnknownSelections(t *testing.T) {
	for i := uint16(0); i < 16; i++ {
		value := uint16(0x0a0a) + i*0x1010
		t.Run(fmt.Sprintf("client-offer%04x", value), func(t *testing.T) {
			ext := append([]tlsResumptionExt{{43, []byte{4, byte(value >> 8), byte(value), 3, 4}}, {value, []byte{7}},
				{16, eeScopeALPN(string([]byte{byte(value >> 8), byte(value)}), "h2")}}, tlsResumptionDHExtensions(true)...)
			ext = certificatePSKTestChange(ext, 10, []byte{0, 4, byte(value >> 8), byte(value), 0, 29}, false)
			share := append([]byte{byte(value >> 8), byte(value), 0, 1, 7}, tlsResumptionDHExtensions(false)[0].data...)
			ext = certificatePSKTestChange(ext, 51, append(binary.BigEndian.AppendUint16(nil, uint16(len(share))), share...), false)
			body := tlsResumptionHello(true, nil, 0x1301, ext...)
			// Add a GREASE cipher before the ordinary cipher, preserving a
			// complete authored cipher vector and every later declaration.
			body = append(append(append(bytes.Clone(body[:35]), 0, 4, byte(value>>8), byte(value)), body[37:39]...), body[39:]...)
			s := newBinTLS()
			_, err := s.handshakeMessage(0, reliabilityHS(1, body), false)
			require.NoError(t, err, "GREASE proposals must remain interoperable")
			clear(body)
			_, err = s.handshakeMessage(1, reliabilityHS(2, keyShareSelectionServer(0x1301)), false)
			require.NoError(t, err)
			_, err = s.handshakeMessage(1, reliabilityHS(8, eeScopeBody(tlsResumptionExt{16, eeScopeALPN("h2")})), true)
			require.NoError(t, err)
			require.Equal(t, "h2", s.alpn)
		})
	}
	for _, kind := range []string{"version", "cipher suite", "extension", "key_share group"} {
		t.Run("ordinary-unknown/"+kind, func(t *testing.T) {
			s := newBinTLS()
			body := greaseTestHello(0xffa5, kind, "final")
			_, err := s.handshakeMessage(1, reliabilityHS(2, body), false)
			require.NoError(t, err, "the reserved-value rule does not reject all future or private declarations")
			o, err := ObserveTLSGREASEServerHelloRecord(helloObservationRecord(body), 16)
			require.Error(t, err)
			require.Nil(t, o)
		})
	}
}

func TestTLSGREASEProtectedMessageAdmissionAndLegalServerOffers(t *testing.T) {
	for i := uint16(0); i < 16; i++ {
		value := uint16(0x0a0a) + i*0x1010
		for _, kind := range []string{"EE-extension", "EE-ALPN", "Certificate-extension", "CertificateVerify", "CertificateRequest-offer", "NewSessionTicket-offer"} {
			t.Run(fmt.Sprintf("%04x/%s", value, kind), func(t *testing.T) {
				s := newBinTLS()
				s.version, s.suite, s.client = 0x304, 0x1301, 0
				var encoded []byte
				switch kind {
				case "EE-extension":
					encoded = reliabilityHS(8, eeScopeBody(tlsResumptionExt{value, []byte{7}}))
				case "EE-ALPN":
					encoded = reliabilityHS(8, eeScopeBody(tlsResumptionExt{16, []byte{0, 3, 2, byte(value >> 8), byte(value)}}))
				case "Certificate-extension":
					encoded = reliabilityHS(11, certificateExtensionsBody(nil, certificateRequestExtension(value, []byte{7})))
					literal, err := DecodeTLSCertificate(encoded, 0x304, 16)
					require.NoError(t, err, "role-neutral literal decoding remains separate from native selection admission")
					require.NotEmpty(t, literal.Ranges)
				case "CertificateVerify":
					encoded = reliabilityHS(15, []byte{byte(value >> 8), byte(value), 0, 1, 7})
					literal, err := DecodeTLSCertificateVerify(encoded, 0x304)
					require.NoError(t, err)
					require.Equal(t, value, literal.Fields["Signature Algorithm"])
				case "CertificateRequest-offer":
					ext := append(certificateRequestExtension(13, []byte{0, 4, byte(value >> 8), byte(value), 8, 4}), certificateRequestExtension(value, []byte{7})...)
					encoded = certificateRequest13(nil, ext)
				case "NewSessionTicket-offer":
					s.app[1] = true
					encoded = reliabilityHS(4, []byte{0, 0, 0, 60, 0, 0, 0, 1, 0, 0, 1, 7, 0, 5, byte(value >> 8), byte(value), 0, 1, 7})
				}
				before := *s
				m, err := s.handshakeMessage(1, encoded, true, 16)
				if kind == "CertificateRequest-offer" || kind == "NewSessionTicket-offer" {
					require.NoError(t, err, "RFC8701 permits GREASE proposals at these server-initiated extension points")
					require.NotNil(t, m)
				} else {
					var typed *ProtocolError
					require.ErrorAs(t, err, &typed)
					require.Equal(t, ErrMalformedMessage, typed.Kind)
					require.Contains(t, err.Error(), "reserved GREASE")
					require.Nil(t, m)
					require.Equal(t, before, *s)
				}
			})
		}
	}
}

func TestTLSGREASELegacySignatureFieldIsNotAnOpaqueByteHeuristic(t *testing.T) {
	for _, version := range []uint16{0x301, 0x302, 0x303} {
		t.Run(fmt.Sprintf("version%04x", version), func(t *testing.T) {
			s := newBinTLS()
			s.version, s.suite, s.client = version, 0xc02f, 0
			body := []byte{0, 2, 0x7a, 0x7a} // legacy opaque signature bytes
			if version == 0x303 {
				body = []byte{0x7a, 0x7a, 0, 1, 7} // selected algorithm field
			}
			encoded := reliabilityHS(15, body)
			literal, err := DecodeTLSCertificateVerify(encoded, version)
			require.NoError(t, err)
			before := *s
			m, err := s.handshakeMessage(0, encoded, false)
			if version == 0x303 {
				require.ErrorContains(t, err, "reserved GREASE signature algorithm")
				require.Equal(t, uint16(0x7a7a), literal.Fields["Signature Algorithm"])
				require.Nil(t, m)
				require.Equal(t, before, *s)
			} else {
				require.NoError(t, err, "GREASE-looking bytes inside an opaque signature are not an algorithm selection")
				require.NotContains(t, literal.Fields, "Signature Algorithm")
			}
		})
	}
}

func TestTLSGREASEEncryptedExtensionsTailAndBudgetPrecedence(t *testing.T) {
	for _, tail := range []bool{false, true} {
		t.Run(fmt.Sprintf("malformedTail%t", tail), func(t *testing.T) {
			body := eeScopeBody(tlsResumptionExt{0xaaa9, nil}, tlsResumptionExt{0x7a7a, nil})
			if tail {
				body = append(body, 0xff)
				binary.BigEndian.PutUint16(body, uint16(len(body)-2))
			}
			s := newBinTLS()
			s.version, s.suite, s.client = 0x304, 0x1301, 0
			before := *s
			m, err := s.handshakeMessage(1, reliabilityHS(8, body), true, 1)
			want := "reserved GREASE extension"
			if tail {
				want = "TLS extension header"
			}
			require.ErrorContains(t, err, want, "complete structure precedes reserved selection, which precedes collection refusal")
			require.Nil(t, m)
			require.Equal(t, before, *s)
		})
	}
}

func TestTLSGREASEAuthenticatedRecordsRetainAuthenticationWithoutAdmission(t *testing.T) {
	for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
		for server := 0; server < 2; server++ {
			for _, chunks := range []int{1, 2} {
				t.Run(fmt.Sprintf("%04x/server%d/chunks%d", suite, server, chunks), func(t *testing.T) {
					f, s := eeRecordFlow(t, suite, server, true)
					message := reliabilityHS(8, eeScopeBody(tlsResumptionExt{0x7a7a, []byte{1}}))
					peerKey, peerSequence := s.keys[1-server], s.seq[1-server]
					for i := 0; i < chunks; i++ {
						piece := message[i*len(message)/chunks : (i+1)*len(message)/chunks]
						sequence := s.seq[server]
						e := &ProtocolEvent{Protocol: "tls", Raw: boundaryTLSRecord(t, s, server, sequence, piece, 22, 0)}
						e.Length = len(e.Raw)
						_, err := f.consumeTLS(server, e)
						require.Equal(t, true, e.Session["Authentication Verified"])
						require.Equal(t, sequence+1, s.seq[server])
						if i+1 == chunks {
							require.ErrorContains(t, err, "reserved GREASE extension")
							require.NotContains(t, e.Session, "Handshake Messages")
						} else {
							require.NoError(t, err)
							require.Equal(t, true, e.Session["Handshake Incomplete"])
						}
						require.Empty(t, s.alpn)
						require.Nil(t, s.child)
					}
					require.Same(t, peerKey, s.keys[1-server])
					require.Equal(t, peerSequence, s.seq[1-server])
				})
			}
		}
	}
}

func TestTLSGREASENativeTCPAndQUICRefusalOwnsNoNegotiation(t *testing.T) {
	for _, kind := range []string{"version", "cipher suite", "extension", "key_share group"} {
		for _, delivery := range []string{"whole", "split", "reordered", "duplicate-tail"} {
			t.Run(kind+"/"+delivery, func(t *testing.T) {
				body := greaseTestHello(0x7a7a, kind, "final")
				q := &binQUIC{native: true, byteLimit: 1 << 20}
				require.NoError(t, q.feedCrypto(0, quicSpaceInitial, 0, reliabilityHS(1, keyShareSelectionClient(0x1301, "matching")), map[string]any{}, 4096))
				wire := reliabilityHS(2, body)
				middle, offset, tail := len(wire)/2, uint64(0), wire
				if delivery != "whole" {
					offset, tail = uint64(middle), wire[middle:]
					if delivery == "split" {
						offset, tail = 0, wire[:middle]
					}
					require.NoError(t, q.feedCrypto(1, quicSpaceInitial, offset, tail, map[string]any{}, 4096))
					if delivery == "duplicate-tail" {
						require.NoError(t, q.feedCrypto(1, quicSpaceInitial, offset, tail, map[string]any{}, 4096))
					}
					offset, tail = 0, wire[:middle]
					if delivery == "split" {
						offset, tail = uint64(middle), wire[middle:]
					}
				}
				before, owned := *q.tls, bytes.Clone(q.tls.firstHello)
				info := map[string]any{}
				err := q.feedCrypto(1, quicSpaceInitial, offset, tail, info, 4096)
				require.ErrorContains(t, err, "reserved GREASE")
				require.Equal(t, before, *q.tls)
				require.Equal(t, owned, q.tls.firstHello)
				require.Zero(t, q.cryptoRead[1][quicSpaceInitial])
				require.NotContains(t, info, "TLS Handshakes")
				require.False(t, q.loadedKeys)
				s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"))
				require.NoError(t, err)
				t.Cleanup(func() { s.Close("done"); require.Zero(t, s.Stats().BufferedBytes) })
				r := s.Feed(1, time.Unix(1, 0), helloObservationRecord(body))
				require.NotNil(t, r.Err)
				require.Equal(t, ErrMalformedMessage, r.Err.Kind)
				require.Len(t, r.Events, 1)
				require.Zero(t, r.Events[0].Session["Negotiated Version"])
				n13AuditNoChild(t, s)
			})
		}
	}
}

func FuzzTLSGREASESelectionAndObservation(f *testing.F) {
	for _, seed := range []struct {
		value, kind, profile, server byte
		cut                          uint16
		data                         []byte
	}{
		{0, 0, 0, 0, 0xffff, nil}, {7, 1, 0, 1, 0xffff, []byte{0, 1}},
		{15, 2, 0, 1, 0xffff, []byte{0xff, 0}}, {5, 3, 0, 0, 0xffff, nil},
		{3, 3, 1, 0, 0xffff, []byte{1}}, {9, 3, 2, 1, 0xffff, nil},
		{7, 2, 0, 1, 50, []byte{0, 0, 0}},
	} {
		f.Add(seed.value, seed.kind, seed.profile, seed.server, seed.cut, seed.data)
	}
	f.Fuzz(func(t *testing.T, value, kind, profile, server byte, cut uint16, data []byte) {
		grease := uint16(0x0a0a) + uint16(value%16)*0x1010
		mode := []string{"final", "legacy", "retry"}[profile%3]
		kinds := []string{"version", "cipher suite", "extension", "key_share group"}
		if mode == "legacy" {
			kinds[3] = "ALPN"
		}
		body := greaseTestHello(grease, kinds[kind%4], mode)
		data = bytes.Clone(data[:min(len(data), 256)])
		extra := certificateRequestExtension(0xffa5, data)
		body = append(body, extra...)
		binary.BigEndian.PutUint16(body[38:], uint16(len(body)-40))
		s := newBinTLS()
		before := *s
		m, err := s.handshakeMessage(int(server&1), reliabilityHS(2, body), false, 8)
		var typed *ProtocolError
		require.ErrorAs(t, err, &typed)
		require.Equal(t, ErrMalformedMessage, typed.Kind)
		require.Nil(t, m)
		require.Equal(t, before, *s)
		wire := helloObservationRecord(body)
		if cut != 0xffff {
			wire = wire[:int(cut)%(len(wire)+1)]
		}
		original := bytes.Clone(wire)
		o, err := ObserveTLSGREASEServerHelloRecord(wire, 8)
		require.Equal(t, original, wire)
		if err == nil {
			require.Equal(t, grease, o.Value)
			require.Equal(t, grease, binary.BigEndian.Uint16(wire[o.Offset:o.Offset+2]))
		} else {
			require.Nil(t, o)
		}
	})
}
