package pcaputil

import (
	"bytes"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// These complete, non-secret messages are extracted from the independent244
// original mTLS captures. Full captures and keys remain outside the repository.
func certificateRequestFixture(t *testing.T, tls13 bool) []byte {
	t.Helper()
	text := "0d000030030102400028040305030603080708080809080a080b0804080508060401050106010303030103020402050206020000"
	if tls13 {
		text = "0d00003100002e000d002a002809050906090404030503060308070808081a081b081c0809080a080b080408050806040105010601"
	}
	b, err := hex.DecodeString(text)
	require.NoError(t, err)
	require.Equal(t, len(b)-4, tlsU24(b[1:4]))
	return b
}

func TestTLSCertificateRequestGrammarBoundaries(t *testing.T) {
	for _, tls13 := range []bool{false, true} {
		wire := certificateRequestFixture(t, tls13)
		version := certificateRequestState(tls13).version
		for length := 0; length < len(wire); length++ {
			_, err := DecodeTLSCertificateRequest(wire[:length], version, 4096)
			require.Error(t, err, "cropped message %d/%d", length, len(wire))
		}
		count := map[bool]int{false: 23, true: 21}[tls13]
		_, err := DecodeTLSCertificateRequest(wire, version, count)
		require.NoError(t, err)
		_, err = DecodeTLSCertificateRequest(wire, version, count-1)
		var typed *ProtocolError
		require.ErrorAs(t, err, &typed)
		require.Equal(t, ErrResourceExceeded, typed.Kind)
		bad := bytes.Clone(wire)
		bad[4] = 0xff
		_, err = DecodeTLSCertificateRequest(bad, version, 0)
		require.ErrorAs(t, err, &typed)
		require.Equal(t, ErrMalformedMessage, typed.Kind, "vector framing precedes collection limits")
	}
	legacy := reliabilityHS(13, []byte{1, 1, 0, 0})
	for _, version := range []uint16{0x301, 0x302} {
		view, err := DecodeTLSCertificateRequest(legacy, version, 1)
		require.NoError(t, err)
		require.NotContains(t, view.Fields, "Signature Algorithms Length")
	}
	_, err := DecodeTLSCertificateRequest(legacy, 0x303, 4096)
	require.Error(t, err, "TLS 1.2 requires the signature vector")
	for _, version := range []uint16{0, 0x300, 0x305, 0xffff} {
		_, err := DecodeTLSCertificateRequest(legacy, version, 4096)
		var typed *ProtocolError
		require.ErrorAs(t, err, &typed)
		require.Equal(t, ErrUnsupportedVersion, typed.Kind)
	}
}

func certificateRequestExtension(typ uint16, data []byte) []byte {
	b := make([]byte, 4+len(data))
	binary.BigEndian.PutUint16(b, typ)
	binary.BigEndian.PutUint16(b[2:], uint16(len(data)))
	copy(b[4:], data)
	return b
}

func certificateRequest13(context, extensions []byte) []byte {
	b := append([]byte{byte(len(context))}, context...)
	b = binary.BigEndian.AppendUint16(b, uint16(len(extensions)))
	b = append(b, extensions...)
	return reliabilityHS(13, b)
}

func TestTLSCertificateRequestExtensionsAndNames(t *testing.T) {
	sig := certificateRequestExtension(13, []byte{0, 2, 8, 4})
	name, err := asn1.Marshal(pkix.Name{CommonName: "shark fixture"}.ToRDNSequence())
	require.NoError(t, err)
	dn := binary.BigEndian.AppendUint16(nil, uint16(len(name)))
	dn = append(dn, name...)
	authorities := binary.BigEndian.AppendUint16(nil, uint16(len(dn)))
	authorities = append(authorities, dn...)
	extensions := append(bytes.Clone(sig), certificateRequestExtension(47, authorities)...)
	extensions = append(extensions, certificateRequestExtension(50, []byte{0, 2, 8, 9})...)
	extensions = append(extensions, certificateRequestExtension(65000, []byte{0, 0xff})...)
	wire := certificateRequest13([]byte{1, 2, 3}, extensions)
	view, err := DecodeTLSCertificateRequest(wire, 0x304, 7)
	require.NoError(t, err)
	ext := view.Fields["Extensions"].([]map[string]any)
	require.Len(t, ext, 4)
	require.NotContains(t, view.Fields, "Certificate Types")
	ca := ext[1]["Certificate Authorities"].([]map[string]any)
	require.Equal(t, name, ca[0]["DER"])
	require.Equal(t, "CN=shark fixture", ca[0]["Distinguished Name"])
	require.Equal(t, []byte{0, 0xff}, ext[3]["Data"])
	require.Equal(t, "uninterpreted extension", ext[3]["Interpretation"])
	clear(wire)
	require.Equal(t, []byte{1, 2, 3}, view.Fields["Context"])
	require.Equal(t, name, ca[0]["DER"])
	require.Equal(t, []byte{0, 0xff}, ext[3]["Data"])
	for name, extensions := range map[string][]byte{
		"no-signatures":      nil,
		"duplicate":          append(bytes.Clone(sig), sig...),
		"empty-signatures":   certificateRequestExtension(13, []byte{0, 0}),
		"odd-signatures":     certificateRequestExtension(13, []byte{0, 1, 8}),
		"signature-tail":     certificateRequestExtension(13, []byte{0, 2, 8, 4, 0}),
		"empty-ca-extension": append(bytes.Clone(sig), certificateRequestExtension(47, []byte{0, 0})...),
		"zero-dn":            append(bytes.Clone(sig), certificateRequestExtension(47, []byte{0, 2, 0, 0})...),
		"dn-overrun":         append(bytes.Clone(sig), certificateRequestExtension(47, []byte{0, 3, 0, 2, 0})...),
		"bad-der":            append(bytes.Clone(sig), certificateRequestExtension(47, []byte{0, 3, 0, 1, 0xff})...),
		"partial-extension":  append(bytes.Clone(sig), 0),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := DecodeTLSCertificateRequest(certificateRequest13(nil, extensions), 0x304, 4096)
			var typed *ProtocolError
			require.ErrorAs(t, err, &typed)
			require.Equal(t, ErrMalformedMessage, typed.Kind)
		})
	}
}

func TestTLSCertificateRequestContextAndDirection(t *testing.T) {
	wire := certificateRequestFixture(t, true)
	for _, knownVersion := range []bool{false, true} {
		s := certificateRequestState(true)
		if !knownVersion {
			s.version = 0
		}
		before := *s
		m, err := s.handshakeMessage(0, wire, true, 128)
		require.Error(t, err)
		require.Nil(t, m)
		require.Equal(t, before, *s)
	}
	s := certificateRequestState(true)
	s.finalHello = false
	m, err := s.handshakeMessage(1, wire, false, 128)
	require.Error(t, err)
	require.Nil(t, m)
	s = newBinTLS()
	m, err = s.handshakeMessage(1, wire, false, 128)
	require.NoError(t, err)
	require.Contains(t, m["Certificate Request Status"], "unavailable")
	require.NotContains(t, m, "Certificate Request")
	require.NotContains(t, m, "Encoded Certificate Request")
	require.Zero(t, s.version)
}

func TestTLSCertificateRequestRecordFragmentsAndDisplayFields(t *testing.T) {
	for _, tls13 := range []bool{false, true} {
		wire := certificateRequestFixture(t, tls13)
		for split := 1; split < len(wire); split++ {
			t.Run(fmt.Sprintf("TLS13=%t/split=%d", tls13, split), func(t *testing.T) {
				var f *binFlow
				var state *binTLS
				if tls13 {
					f, state = boundaryTLSFlow(t, 0x1302)
				} else {
					a, _ := remainingParser(t)
					state = certificateRequestState(false)
					f = &binFlow{a: a, id: 1, protocol: "tls", tls: state, transportFlow: &TrafficFlow{}}
					t.Cleanup(func() { f.closeSession(); require.Zero(t, a.stats().BufferedBytes) })
				}
				record := func(seq uint64, data []byte) []byte {
					if tls13 {
						return boundaryTLSRecord(t, state, 1, seq, data, 22, 3)
					}
					return tlsResumptionRecord(data)
				}
				e := &ProtocolEvent{Protocol: "tls", Raw: record(2, wire[:split])}
				_, err := f.consumeTLS(1, e)
				require.NoError(t, err)
				require.Equal(t, true, e.Session["Handshake Incomplete"])
				require.Empty(t, e.Session["Handshake Messages"])
				e = &ProtocolEvent{Protocol: "tls", Raw: record(3, wire[split:])}
				_, err = f.consumeTLS(1, e)
				require.NoError(t, err)
				require.Equal(t, false, e.Session["Handshake Incomplete"])
				messages := e.Session["Handshake Messages"].([]map[string]any)
				require.Len(t, messages, 1)
				require.Equal(t, wire, messages[0]["Encoded Certificate Request"])
				fields := e.DisplayFieldValues()
				count := 0
				for _, item := range fields.Values {
					if item.Name == "tls.handshake.sig_hash_alg" {
						count++
						require.Equal(t, "derived", item.Origin)
						require.Nil(t, item.Range, "a reassembled/decoded handshake is not the outer ciphertext PDU")
					}
				}
				require.Equal(t, 20, count)
				if tls13 {
					require.NotContains(t, fields.FieldMap(), "tls.handshake.dnames_len")
				}
			})
		}
	}
}

func FuzzTLSCertificateRequestBody(f *testing.F) {
	f.Add(uint16(0x303), []byte{13, 0, 0, 8, 1, 1, 0, 2, 4, 3, 0, 0})
	f.Add(uint16(0x304), certificateRequest13(nil, certificateRequestExtension(13, []byte{0, 2, 8, 4})))
	f.Add(uint16(0x304), certificateRequest13([]byte{1}, append(certificateRequestExtension(13, []byte{0, 2, 8, 4}), certificateRequestExtension(47, []byte{0, 4, 0, 2, 0x30, 0})...)))
	f.Fuzz(func(t *testing.T, version uint16, encoded []byte) {
		if len(encoded) > 1<<16 {
			return
		}
		check := func(input []byte, version uint16) {
			original := bytes.Clone(input)
			view, err := DecodeTLSCertificateRequest(input, version, 64)
			require.Equal(t, original, input)
			if err != nil {
				require.Nil(t, view)
				return
			}
			for _, span := range view.Ranges {
				require.GreaterOrEqual(t, span.Offset, 4)
				require.Positive(t, span.Length)
				require.LessOrEqual(t, span.Offset+span.Length, len(input))
				var value any = view.Fields
				for _, component := range span.Path {
					switch parent := value.(type) {
					case map[string]any:
						value = parent[component]
					case []map[string]any:
						i, err := strconv.Atoi(component)
						require.NoError(t, err)
						require.GreaterOrEqual(t, i, 0)
						require.Less(t, i, len(parent))
						value = parent[i]
					default:
						t.Fatalf("range path has no parent: %v", span.Path)
					}
				}
				raw := input[span.Offset : span.Offset+span.Length]
				switch literal := value.(type) {
				case byte:
					require.Equal(t, []byte{literal}, raw)
				case uint16:
					require.Equal(t, binary.BigEndian.AppendUint16(nil, literal), raw)
				case []byte:
					require.Equal(t, literal, raw)
				default:
					t.Fatalf("range attached to a non-literal %T", value)
				}
			}
			second, err := DecodeTLSCertificateRequest(input, version, 64)
			require.NoError(t, err)
			require.Equal(t, view, second)
			before, err := json.Marshal(view.Fields)
			require.NoError(t, err)
			clear(input)
			after, err := json.Marshal(view.Fields)
			require.NoError(t, err)
			require.Equal(t, before, after, "published body octets must not alias the caller")
		}
		check(bytes.Clone(encoded), version)
		// Keep framing/version fuzzing, and independently force a complete
		// envelope so mutations also reach nested body and ASN.1 boundaries.
		body := encoded
		if len(body) >= 4 {
			body = body[4:]
		}
		check(reliabilityHS(13, body), 0x301+version%4)
	})
}

func TestTLSCertificateRequestNativeOwnershipAndFreshConnection(t *testing.T) {
	for _, deferred := range []bool{false, true} {
		s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionPorts(40000, 443))
		require.NoError(t, err)
		defer s.Close("cleanup")
		s.(*captureSession).f.a.config.Deferred = deferred
		for dir, typ := range []byte{1, 2} {
			hello := make([]byte, 35)
			hello[0], hello[1] = 3, 3
			if dir == 0 {
				hello = append(hello, 0, 2, 0xc0, 0x2f, 1, 0, 0, 0)
			} else {
				hello = append(hello, 0xc0, 0x2f, 0, 0, 0)
			}
			result := s.Feed(dir, time.Unix(10, 0), tlsResumptionRecord(reliabilityHS(typ, hello)))
			require.Nil(t, result.Err)
			require.Len(t, result.Events, 1)
		}
		input := tlsResumptionRecord(certificateRequestFixture(t, false))
		result := s.Feed(1, time.Unix(11, 0), input)
		require.Nil(t, result.Err)
		require.Len(t, result.Events, 1)
		e := result.Events[0]
		fields, err := e.GetFields()
		require.NoError(t, err)
		before := e.DisplayFields()
		require.Len(t, before["tls.handshake.sig_hash_alg"], 20)
		clear(input)
		tlsDTLSPoison(e, "caller")
		if deferred {
			e.Status = "deferred"
		} else {
			e.Status = "decoded"
		}
		require.Equal(t, before, e.DisplayFields())
		after, err := e.GetFields()
		require.NoError(t, err)
		require.Equal(t, fields, after)
		fresh, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionPorts(40000, 443))
		require.NoError(t, err)
		defer fresh.Close("cleanup")
		result = fresh.Feed(1, time.Unix(12, 0), tlsResumptionRecord(certificateRequestFixture(t, false)))
		require.Nil(t, result.Err)
		require.Len(t, result.Events, 1)
		require.NotContains(t, result.Events[0].DisplayFields(), "tls.handshake.sig_hash_alg", "same endpoints do not transfer a previous negotiated grammar")
	}
}

func TestTLSCertificateRequestQUICCryptoGapAndDuplicates(t *testing.T) {
	wire := certificateRequestFixture(t, true)
	for split := 1; split < len(wire); split++ {
		q := &binQUIC{tls: certificateRequestState(true), byteLimit: 4096}
		info := map[string]any{}
		require.NoError(t, q.feedCrypto(1, quicSpaceHandshake, uint64(split), wire[split:], info, 128))
		require.NotContains(t, info, "TLS Handshakes", "a permanent CRYPTO prefix gap cannot publish a complete message")
		require.Equal(t, true, info["CRYPTO Incomplete"])
		require.NoError(t, q.feedCrypto(1, quicSpaceHandshake, 0, wire[:split], info, 128))
		messages := info["TLS Handshakes"].([]map[string]any)
		require.Len(t, messages, 1)
		require.Equal(t, wire, messages[0]["Encoded Certificate Request"])
		for _, protocol := range []string{"quic", "http3", "doq"} {
			e := &ProtocolEvent{Protocol: protocol, Session: map[string]any{"TLS Handshakes": messages}}
			if protocol != "quic" {
				e.nativeQUICObservation, e.semanticSession = true, cloneSession(e.Session)
			}
			require.Len(t, e.DisplayFields()["tls.handshake.sig_hash_alg"], 20)
		}
		info = map[string]any{}
		require.NoError(t, q.feedCrypto(1, quicSpaceHandshake, 0, wire, info, 128))
		require.NotContains(t, info, "TLS Handshakes", "retransmitted CRYPTO bytes are not a second CertificateRequest")
		bad := bytes.Clone(wire)
		bad[len(bad)-1] ^= 1
		require.Error(t, q.feedCrypto(1, quicSpaceHandshake, 0, bad, map[string]any{}, 128))
	}
}

func certificateRequestState(tls13 bool) *binTLS {
	s := newBinTLS()
	s.client, s.version, s.suite, s.finalHello = 0, 0x303, 0xc02f, true
	if tls13 {
		s.version, s.suite = 0x304, 0x1302
	}
	return s
}

func TestTLSCertificateRequestPublishesObservedBody(t *testing.T) {
	for _, tls13 := range []bool{false, true} {
		t.Run(map[bool]string{false: "TLS12", true: "TLS13"}[tls13], func(t *testing.T) {
			s := certificateRequestState(tls13)
			wire := certificateRequestFixture(t, tls13)
			original := bytes.Clone(wire)
			m, err := s.handshakeMessage(1, wire, tls13, 128)
			require.NoError(t, err)
			require.Equal(t, original, m["Encoded Certificate Request"])
			require.Equal(t, s.version, m["Certificate Request Version"])
			body, ok := m["Certificate Request"].(map[string]any)
			require.True(t, ok, "the complete request must publish more than its envelope")
			if tls13 {
				require.Equal(t, byte(0), body["Context Length"])
				require.Equal(t, []byte{}, body["Context"])
				require.Equal(t, uint16(46), body["Extensions Length"])
				extensions := body["Extensions"].([]map[string]any)
				require.Len(t, extensions, 1)
				require.Equal(t, uint16(13), extensions[0]["Type"])
				require.Equal(t, uint16(42), extensions[0]["Length"])
				require.Equal(t, uint16(40), extensions[0]["Signature Algorithms Length"])
				require.Len(t, extensions[0]["Signature Algorithms"], 20)
				require.NotContains(t, body, "Certificate Authorities Length", "absent extension is not an empty CA literal")
			} else {
				require.Equal(t, byte(3), body["Certificate Types Length"])
				require.Len(t, body["Certificate Types"], 3)
				require.Equal(t, uint16(40), body["Signature Algorithms Length"])
				require.Len(t, body["Signature Algorithms"], 20)
				require.Equal(t, uint16(0), body["Certificate Authorities Length"])
				require.Empty(t, body["Certificate Authorities"])
			}
			clear(wire)
			require.Equal(t, original, m["Encoded Certificate Request"])
		})
	}
}

func TestTLSCertificateRequestRejectsMalformedVectors(t *testing.T) {
	for _, tls13 := range []bool{false, true} {
		wire := certificateRequestFixture(t, tls13)
		for _, offset := range map[bool][]int{false: {4, 8, len(wire) - 2}, true: {4, 5, 9, 11}}[tls13] {
			t.Run(map[bool]string{false: "TLS12", true: "TLS13"}[tls13]+"/"+hex.EncodeToString([]byte{byte(offset)}), func(t *testing.T) {
				input := bytes.Clone(wire)
				input[offset] = 0xff
				s := certificateRequestState(tls13)
				m, err := s.handshakeMessage(1, input, tls13, 128)
				var typed *ProtocolError
				require.ErrorAs(t, err, &typed)
				require.Equal(t, ErrMalformedMessage, typed.Kind)
				require.Nil(t, m)
				require.Equal(t, certificateRequestState(tls13), s, "rejected messages must not change connection state")
			})
		}
	}
}
