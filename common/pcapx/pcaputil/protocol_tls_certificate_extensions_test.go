package pcaputil

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// A non-X.509 payload is intentional: extension framing must be admitted before
// certificate hashing/parsing. These are public grammar controls, not credentials.
func certificateExtensionsBody(context []byte, extensions ...[]byte) []byte {
	var entries []byte
	for _, ext := range extensions {
		entries = append(entries, 0, 0, 1, 0x30, byte(len(ext)>>8), byte(len(ext)))
		entries = append(entries, ext...)
	}
	body := append([]byte{byte(len(context))}, context...)
	body = append(body, byte(len(entries)>>16), byte(len(entries)>>8), byte(len(entries)))
	return append(body, entries...)
}

func TestTLSCertificateEntryExtensionFraming(t *testing.T) {
	unknown := certificateRequestExtension(0xff01, []byte{0, 1, 2})
	for _, control := range []struct {
		name string
		ext  []byte
	}{
		{"partial-type", []byte{0xff}},
		{"partial-header", []byte{0xff, 1, 0}},
		{"missing-value", []byte{0xff, 1, 0, 1}},
		{"value-overrun", []byte{0xff, 1, 0, 3, 1, 2}},
		{"duplicate-type", append(bytes.Clone(unknown), unknown...)},
		{"late-malformed", append(bytes.Clone(unknown), 0xff)},
	} {
		t.Run(control.name, func(t *testing.T) {
			for _, limit := range []int{0, 1, 4096} {
				_, err := tlsCertificates(certificateExtensionsBody(nil, control.ext), true, limit)
				var typed *ProtocolError
				require.ErrorAs(t, err, &typed)
				require.Equal(t, ErrMalformedMessage, typed.Kind, "framing precedes collection admission")
			}
		})
	}
	for _, control := range []struct {
		name string
		exts [][]byte
	}{
		{"empty", [][]byte{nil}},
		{"unknown-opaque", [][]byte{unknown}},
		{"different-types", [][]byte{append(bytes.Clone(unknown), certificateRequestExtension(0xff02, nil)...)}},
		{"same-type-different-certificates", [][]byte{unknown, unknown}},
	} {
		t.Run(control.name, func(t *testing.T) {
			body := certificateExtensionsBody([]byte{0, 0xff, 1}, control.exts...)
			before := bytes.Clone(body)
			certs, err := tlsCertificates(body, true, 4096)
			require.NoError(t, err)
			require.Len(t, certs, len(control.exts))
			require.Equal(t, before, body)
			for _, cert := range certs {
				require.Equal(t, 1, cert["DER Length"])
				require.Equal(t, "invalid X.509", cert["Parse Error"])
				require.Equal(t, "not-evaluated", cert["Trust"])
			}
		})
	}
}

func TestTLSCertificateEntryExtensionCollectionBudget(t *testing.T) {
	ext := certificateRequestExtension(0xff00, nil)
	other := certificateRequestExtension(0xffff, []byte{0x01})
	body := certificateExtensionsBody(nil, append(bytes.Clone(ext), other...), ext)
	// Two certificate entries plus three extensions share the configured budget.
	_, err := tlsCertificates(body, true, 5)
	require.NoError(t, err)
	_, err = tlsCertificates(body, true, 4)
	var typed *ProtocolError
	require.ErrorAs(t, err, &typed)
	require.Equal(t, ErrResourceExceeded, typed.Kind)
	_, err = tlsCertificates(certificateExtensionsBody(nil), true, 0)
	require.NoError(t, err, "an empty client certificate list has no collection entries")
}

func TestTLSCertificateEntryExtensionAuthenticatedAdmission(t *testing.T) {
	valid := certificateExtensionsBody(nil, certificateRequestExtension(0xff01, []byte{1, 2}))
	bad := certificateExtensionsBody(nil, []byte{0xff, 1, 0})
	for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
		t.Run(tlsCipherTestName(suite), func(t *testing.T) {
			for _, dir := range []int{0, 1} {
				f, state := boundaryTLSFlow(t, suite)
				// The server sends its certificate with the handshake keys;
				// client post-handshake certificates use application keys.
				state.app[1] = false
				epochs := state.app
				wire := boundaryTLSRecord(t, state, dir, 2, reliabilityHS(11, valid), 22, 0)
				e := &ProtocolEvent{Protocol: "tls", Raw: wire}
				_, err := f.consumeTLS(dir, e)
				require.NoError(t, err)
				require.Equal(t, true, e.Session["Authentication Verified"])
				message := e.Session["Handshake Messages"].([]map[string]any)[0]
				require.Equal(t, uint16(0x304), message["Certificate Version"])
				require.Equal(t, reliabilityHS(11, valid), message["Encoded Certificate"])
				require.Equal(t, byte(0), message["Certificate Body"].(map[string]any)["Context Length"])
				require.Equal(t, uint64(3), state.seq[dir])
				secret, iv, key := bytes.Clone(state.secret[dir]), bytes.Clone(state.iv[dir]), state.keys[dir]
				wire = boundaryTLSRecord(t, state, dir, 3, reliabilityHS(11, bad), 22, 0)
				e = &ProtocolEvent{Protocol: "tls", Raw: wire}
				_, err = f.consumeTLS(dir, e)
				var typed *ProtocolError
				require.ErrorAs(t, err, &typed)
				require.Equal(t, ErrMalformedMessage, typed.Kind)
				require.NotContains(t, e.Session, "Handshake Messages", "a rejected certificate is not a decoded message")
				require.Equal(t, secret, state.secret[dir])
				require.Equal(t, iv, state.iv[dir])
				require.Same(t, key, state.keys[dir])
				require.Equal(t, epochs, state.app)
			}
		})
	}
}

func TestTLSCertificateBodyOwnedVectorsAndRanges(t *testing.T) {
	context := []byte{0xff, 0, 0xff}
	ext := certificateRequestExtension(0xff01, []byte{0xff, 0})
	wire := reliabilityHS(11, certificateExtensionsBody(context, ext, ext))
	view, err := DecodeTLSCertificate(wire, 0x304, 4)
	require.NoError(t, err)
	require.Len(t, view.Ranges, 15)
	verifyTLSCertificateRanges(t, view, wire)
	require.Equal(t, byte(3), view.Fields["Context Length"])
	require.Equal(t, context, view.Fields["Context"])
	entries := view.Fields["Certificate Entries"].([]map[string]any)
	require.Len(t, entries, 2)
	require.Equal(t, uint32(24), view.Fields["Certificate List Length"])
	for i, entry := range entries {
		require.Equal(t, uint32(1), entry["Certificate Length"])
		require.Equal(t, []byte{0x30}, entry["Certificate Data"])
		require.Equal(t, uint16(6), entry["Extensions Length"])
		x := entry["Extensions"].([]map[string]any)
		require.Len(t, x, 1)
		require.Equal(t, uint16(0xff01), x[0]["Type"])
		require.Equal(t, uint16(2), x[0]["Length"])
		require.Equal(t, []byte{0xff, 0}, x[0]["Data"])
		want := 11 + i*12
		for _, span := range view.Ranges {
			if fmt.Sprint(span.Path) == fmt.Sprintf("[Certificate Entries %d Certificate Data]", i) {
				require.Equal(t, want+3, span.Offset)
				require.Equal(t, 1, span.Length)
			}
		}
	}
	for _, span := range view.Ranges {
		require.Positive(t, span.Length)
		require.GreaterOrEqual(t, span.Offset, 4)
		require.LessOrEqual(t, span.Offset+span.Length, len(wire))
	}
	before, err := json.Marshal(view)
	require.NoError(t, err)
	clear(wire)
	after, err := json.Marshal(view)
	require.NoError(t, err)
	require.Equal(t, before, after, "context, entry bytes and extension payloads are owned")
}

func verifyTLSCertificateRanges(t *testing.T, view *TLSCertificateView, wire []byte) {
	t.Helper()
	covered := make([]bool, len(wire))
	for i := 0; i < 4; i++ {
		covered[i] = true
	}
	for _, span := range view.Ranges {
		var value any = view.Fields
		for _, key := range span.Path {
			switch node := value.(type) {
			case map[string]any:
				value = node[key]
			case []map[string]any:
				i, err := strconv.Atoi(key)
				require.NoError(t, err)
				require.GreaterOrEqual(t, i, 0)
				require.Less(t, i, len(node))
				value = node[i]
			default:
				t.Fatalf("nonexistent literal path %v", span.Path)
			}
		}
		require.Positive(t, span.Length)
		require.GreaterOrEqual(t, span.Offset, 4)
		require.LessOrEqual(t, span.Offset+span.Length, len(wire))
		raw := wire[span.Offset : span.Offset+span.Length]
		switch literal := value.(type) {
		case byte:
			require.Equal(t, []byte{literal}, raw)
		case uint16:
			require.Equal(t, binary.BigEndian.AppendUint16(nil, literal), raw)
		case uint32:
			require.Equal(t, []byte{byte(literal >> 16), byte(literal >> 8), byte(literal)}, raw)
		case []byte:
			require.Equal(t, literal, raw)
		default:
			t.Fatalf("non-literal range value %T", value)
		}
		for at := span.Offset; at < span.Offset+span.Length; at++ {
			require.False(t, covered[at], "overlapping or borrowed range at %d", at)
			covered[at] = true
		}
	}
	for at, covered := range covered {
		require.True(t, covered, "unmapped complete vector byte at %d", at)
	}
}

func TestTLSCertificateNativeBodyCacheFragmentsAndFreshConnection(t *testing.T) {
	for _, deferred := range []bool{false, true} {
		for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
			for _, chunk := range []int{1, 7, 4096} {
				t.Run(fmt.Sprintf("deferred%v/%04x/chunk%d", deferred, suite, chunk), func(t *testing.T) {
					session, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionPorts(40000, 443))
					require.NoError(t, err)
					defer session.Close("cleanup")
					capture := session.(*captureSession)
					capture.f.a.config.Deferred = deferred
					// Prime only the supported application epoch with public test
					// secrets. Real handshakes/keys are covered by external captures.
					state := newBinTLS()
					state.version, state.suite, state.client = 0x304, suite, 0
					state.random = bytes.Repeat([]byte{0x44}, 32)
					state.app, state.seq = [2]bool{true, true}, [2]uint64{2, 2}
					n := 32
					if suite == 0x1302 {
						n = 48
					}
					for dir := 0; dir < 2; dir++ {
						require.NoError(t, state.install13(dir, bytes.Repeat([]byte{byte(0x41 + dir)}, n)))
					}
					capture.f.protocol, capture.f.tls = "tls", state
					body := certificateExtensionsBody([]byte{0xff, 0, 0xff}, certificateRequestExtension(0xff01, []byte{1, 2}))
					handshake := reliabilityHS(11, body)
					wire := boundaryTLSRecord(t, state, 0, 2, handshake, 22, 0)
					var events []*ProtocolEvent
					for at := 0; at < len(wire); at += chunk {
						x := session.Feed(0, time.Unix(10, 0), wire[at:min(at+chunk, len(wire))])
						if x.Err != nil {
							require.Equal(t, ErrNeedMore, x.Err.Kind)
						}
						events = append(events, x.Events...)
					}
					require.Len(t, events, 1)
					event := events[0]
					fields, err := event.GetFields()
					require.NoError(t, err)
					before := event.DisplayFieldValues()
					aliases := before.FieldMap()
					require.Equal(t, []any{uint64(3)}, aliases["tls.handshake.certificate_request_context_length"])
					require.Equal(t, []any{"ff00ff"}, aliases["tls.handshake.certificate_request_context"])
					require.Equal(t, []any{uint64(12)}, aliases["tls.handshake.certificates_length"])
					require.Equal(t, []any{uint64(1)}, aliases["tls.handshake.certificate_length"])
					require.Equal(t, []any{"30"}, aliases["tls.handshake.certificate"])
					for _, value := range before.Values {
						if value.Origin == "derived" {
							require.Nil(t, value.Range)
						}
					}
					clear(wire)
					require.Equal(t, before, event.DisplayFieldValues(), "input buffers are not retained")
					tlsDTLSPoison(event, "caller")
					if deferred {
						event.Status = "deferred"
					} else {
						event.Status = "decoded"
					}
					poisoned := event.DisplayFieldValues()
					require.Equal(t, before.FieldMap(), poisoned.FieldMap())
					for _, value := range poisoned.Values {
						if value.Origin == "derived" || value.Origin == "pdu" {
							require.Nil(t, value.Range, "edited outer bytes cannot retain a ciphertext span")
						}
					}
					after, err := event.GetFields()
					require.NoError(t, err)
					require.Equal(t, fields, after)
					fresh, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionPorts(40000, 443))
					require.NoError(t, err)
					defer fresh.Close("cleanup")
					x := fresh.Feed(0, time.Unix(11, 0), boundaryTLSRecord(t, state, 0, 2, handshake, 22, 0))
					require.Len(t, x.Events, 1)
					for _, event := range x.Events {
						require.NotContains(t, event.DisplayFields(), "tls.handshake.certificate", "same endpoints do not inherit certificate plaintext or keys")
					}
				})
			}
		}
	}
}

func TestTLSCertificateSharedAliasesKeepMessageAndEntryOrder(t *testing.T) {
	wire := reliabilityHS(11, certificateExtensionsBody([]byte{0xff, 0}, nil, nil))
	view, err := DecodeTLSCertificate(wire, 0x304, 2)
	require.NoError(t, err)
	for _, protocol := range []string{"tls", "quic"} {
		t.Run(protocol, func(t *testing.T) {
			root := "Handshake Messages"
			if protocol == "quic" {
				root = "TLS Handshakes"
			}
			event := &ProtocolEvent{Protocol: protocol, Session: map[string]any{root: []map[string]any{
				{"Certificate Request": map[string]any{"Context Length": byte(1), "Context": []byte{1}, "Extensions Length": uint16(4), "Extensions": []map[string]any{{"Type": uint16(0xfefe), "Length": uint16(0)}}}},
				{"Certificate Body": view.Fields},
				{"Certificate Request": map[string]any{"Context Length": byte(0), "Context": []byte{}, "Extensions Length": uint16(8), "Extensions": []map[string]any{{"Type": uint16(0xffff), "Length": uint16(4)}}}},
			}}}
			fields := event.DisplayFieldValues().FieldMap()
			require.Equal(t, []any{uint64(1), uint64(2), uint64(0)}, fields["tls.handshake.certificate_request_context_length"])
			require.Equal(t, []any{"01", "ff00", ""}, fields["tls.handshake.certificate_request_context"])
			require.Equal(t, []any{uint64(4), uint64(0), uint64(0), uint64(8)}, fields["tls.handshake.extensions_length"])
			require.Equal(t, []any{uint64(0xfefe), uint64(0xffff)}, fields["tls.handshake.extension.type"])
			require.Equal(t, []any{"30", "30"}, fields["tls.handshake.certificate"])
		})
	}
}

func TestTLSCertificateBodyRequiresCompleteVersionedEncoding(t *testing.T) {
	wire := reliabilityHS(11, certificateExtensionsBody(nil, nil))
	for n := 0; n < len(wire); n++ {
		_, err := DecodeTLSCertificate(wire[:n], 0x304, 4096)
		require.Error(t, err)
	}
	_, err := DecodeTLSCertificate(append(bytes.Clone(wire), 0), 0x304, 4096)
	require.Error(t, err)
	for _, version := range []uint16{0, 0x300, 0x305, 0xffff} {
		_, err = DecodeTLSCertificate(wire, version, 4096)
		var typed *ProtocolError
		require.ErrorAs(t, err, &typed)
		require.Equal(t, ErrUnsupportedVersion, typed.Kind)
	}
	legacy := reliabilityHS(11, []byte{0, 0, 4, 0, 0, 1, 0x30})
	for _, version := range []uint16{0x301, 0x302, 0x303} {
		view, err := DecodeTLSCertificate(legacy, version, 1)
		require.NoError(t, err)
		require.NotContains(t, view.Fields, "Context")
		require.NotContains(t, view.Fields["Certificate Entries"].([]map[string]any)[0], "Extensions")
	}
	empty, err := DecodeTLSCertificate(reliabilityHS(11, certificateExtensionsBody(nil)), 0x304, 0)
	require.NoError(t, err)
	require.Empty(t, empty.Fields["Certificate Entries"])
	for _, span := range empty.Ranges {
		require.NotEqual(t, "Context", span.Path[len(span.Path)-1], "an empty context has no invented byte range")
	}
}

func FuzzTLSCertificateEntryExtensionFraming(f *testing.F) {
	f.Add([]byte{0xff, 1, 0, 0}, uint16(2))
	f.Add([]byte{0xff, 1, 0}, uint16(4096))
	f.Add([]byte{0xff, 1, 0, 1, 0}, uint16(1))
	f.Fuzz(func(t *testing.T, ext []byte, budget uint16) {
		if len(ext) > 65535 {
			return
		}
		valid := true
		seen := map[uint16]bool{}
		count := 1 // One certificate entry.
		for remaining := ext; len(remaining) != 0; {
			if len(remaining) < 4 {
				valid = false
				break
			}
			typ, n := binary.BigEndian.Uint16(remaining), int(binary.BigEndian.Uint16(remaining[2:]))
			remaining = remaining[4:]
			if seen[typ] || n > len(remaining) {
				valid = false
				break
			}
			seen[typ] = true
			count++
			remaining = remaining[n:]
		}
		body := certificateExtensionsBody(nil, ext)
		certs, err := tlsCertificates(body, true, int(budget))
		if valid && count <= int(budget) {
			require.NoError(t, err)
			require.Len(t, certs, 1)
			wire := reliabilityHS(11, body)
			view, err := DecodeTLSCertificate(wire, 0x304, int(budget))
			require.NoError(t, err)
			verifyTLSCertificateRanges(t, view, wire)
			before, err := json.Marshal(view)
			require.NoError(t, err)
			clear(wire)
			after, err := json.Marshal(view)
			require.NoError(t, err)
			require.Equal(t, before, after)
		} else {
			var typed *ProtocolError
			require.ErrorAs(t, err, &typed)
			want := ErrResourceExceeded
			if !valid {
				want = ErrMalformedMessage
			}
			require.Equal(t, want, typed.Kind)
		}
	})
}
