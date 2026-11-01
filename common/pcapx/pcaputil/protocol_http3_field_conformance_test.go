package pcaputil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Literal QPACK encoder for declared malformed-header controls. It does not use
// the native decoder or a normalizing HTTP client to construct expected bytes.
func h3LiteralFields(fields ...[2]string) []byte {
	block := []byte{0, 0}
	integer := func(first byte, bits uint, n int) []byte {
		max := (1 << bits) - 1
		if n < max {
			return []byte{first | byte(n)}
		}
		out := []byte{first | byte(max)}
		for n -= max; n >= 128; n >>= 7 {
			out = append(out, byte(n)|128)
		}
		return append(out, byte(n))
	}
	for _, h := range fields {
		block = append(block, integer(0x20, 3, len(h[0]))...)
		block = append(block, h[0]...)
		block = append(block, integer(0, 7, len(h[1]))...)
		block = append(block, h[1]...)
	}
	return h3Frame(1, block)
}

func TestNativeHTTP3MalformedFieldsKeepWireAndIsolateStream(t *testing.T) {
	request := [][2]string{{":method", "GET"}, {":scheme", "https"}, {":path", "/"}, {":authority", "example.com"}}
	tests := []struct {
		name   string
		dir    int
		fields [][2]string
		prior  int
	}{
		{"missing-method", 0, request[1:], 0},
		{"missing-path", 0, append(append([][2]string{}, request[:2]...), request[3]), 0},
		{"duplicate-method", 0, append(append([][2]string{}, request...), request[0]), 0},
		{"request-status", 0, append(append([][2]string{}, request...), [2]string{":status", "200"}), 0},
		{"pseudo-after-field", 0, append([][2]string{{"x-test", "ok"}}, request...), 0},
		{"unknown-pseudo", 0, append(append([][2]string{}, request...), [2]string{":unknown", "x"}), 0},
		{"uppercase", 0, append(append([][2]string{}, request...), [2]string{"X-Test", "ok"}), 0},
		{"invalid-token", 0, append(append([][2]string{}, request...), [2]string{"x(test", "ok"}), 0},
		{"control-value", 0, append(append([][2]string{}, request...), [2]string{"x-test", "a\x01b"}), 0},
		{"edge-whitespace", 0, append(append([][2]string{}, request...), [2]string{"x-test", " leading"}), 0},
		{"connection", 0, append(append([][2]string{}, request...), [2]string{"connection", "close"}), 0},
		{"TE-gzip", 0, append(append([][2]string{}, request...), [2]string{"te", "gzip"}), 0},
		{"missing-status", 1, [][2]string{{"server", "test"}}, 0},
		{"invalid-status", 1, [][2]string{{":status", "20x"}}, 0},
		{"request-pseudo-in-response", 1, request, 0},
		{"response-TE", 1, [][2]string{{":status", "200"}, {"te", "trailers"}}, 0},
		{"pseudo-in-trailers", 0, [][2]string{{":path", "/second-request"}}, 1},
		{"third-header-section", 0, [][2]string{{"x-trailer", "extra"}}, 2},
		{"CONNECT-with-path", 0, [][2]string{{":method", "CONNECT"}, {":authority", "example.com:443"}, {":path", ""}}, 0},
		{"CONNECT-without-port", 0, [][2]string{{":method", "CONNECT"}, {":authority", "example.com"}}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3"}
			off := uint64(0)
			feed := func(sid uint64, w []byte) (map[string]any, map[string]any, error) {
				q.eventID++
				q.eventRefs = []PacketReference{{Number: q.eventID}}
				info := map[string]any{}
				fr := map[string]any{"Frame Type": "STREAM", "Stream ID": sid, "Offset": off, "Stream Data": w, "FIN": false}
				err := q.applyFrames(tt.dir, quicSpaceApplication, []map[string]any{fr}, info, 1024)
				off += uint64(len(w))
				return fr, info, err
			}
			for i := 0; i < tt.prior; i++ {
				w := h3LiteralFields()
				if i == 0 {
					w = h3RequestHeaders()
				}
				_, _, err := feed(0, w)
				require.NoError(t, err)
			}
			bad := h3LiteralFields(tt.fields...)
			fr, info, err := feed(0, bad)
			require.Error(t, err, "decodable QPACK does not certify a valid HTTP message")
			require.Equal(t, true, info["HTTP3"], "confirmed HTTP3 remains identifiable on malformed messages")
			hf := fr["HTTP3 Frames"].([]map[string]any)[0]
			status := "malformed"
			if tt.name == "unknown-pseudo" {
				status = "unsupported"
			}
			require.Equal(t, status, hf["Header Conformance Status"])
			header := 1 + 1<<(bad[1]>>6)
			require.Equal(t, bad[header:], hf["QPACK Block"], "retain original field-section bytes for inspection")
			require.Equal(t, uint64(len(bad)-header), hf["Payload Length"])
			_, _, err = feed(0, h3LiteralFields(request...))
			require.Error(t, err, "a later valid header must not erase this stream's violation")
			off = 0
			good := h3RequestHeaders()
			if tt.dir == 1 {
				good = h3ResponseHeaders()
			}
			_, _, err = feed(4, good)
			require.NoError(t, err, "another stream on the same connection remains usable")
		})
	}
}

func TestHTTP3BaseFieldConformanceValidForms(t *testing.T) {
	request := [][2]string{{":method", "GET"}, {":scheme", "https"}, {":path", "/"}, {":authority", "example.com"}}
	for _, tt := range []struct {
		name   string
		fields [][2]string
		dir    int
		prior  int
		kind   string
	}{
		{"ordinary-request", request, 0, 0, "request"},
		{"Host-instead-of-authority", [][2]string{{":method", "GET"}, {":scheme", "https"}, {":path", "/"}, {"host", "example.com"}}, 0, 0, "request"},
		{"OPTIONS-star", [][2]string{{":method", "OPTIONS"}, {":scheme", "https"}, {":path", "*"}, {":authority", "example.com"}}, 0, 0, "request"},
		{"CONNECT", [][2]string{{":method", "CONNECT"}, {":authority", "example.com:443"}}, 0, 0, "request"},
		{"CONNECT-IPv6", [][2]string{{":method", "CONNECT"}, {":authority", "[2001:db8::1]:443"}}, 0, 0, "request"},
		{"CONNECT-port-zero-is-not-a-policy-refusal", [][2]string{{":method", "CONNECT"}, {":authority", "example.com:0"}}, 0, 0, "request"},
		{"opaque-and-repeated-regular-fields", append(append([][2]string{}, request...), [2]string{"x-opaque", "\x80\xff"}, [2]string{"cookie", "a=b"}, [2]string{"cookie", "c=d"}, [2]string{"te", "Trailers"}), 0, 0, "request"},
		{"informational", [][2]string{{":status", "103"}}, 1, 0, "informational"},
		{"final", [][2]string{{":status", "200"}}, 1, 0, "response"},
		{"request-trailers", [][2]string{{"x-checksum", "ok"}}, 0, 1, "trailers"},
		{"response-trailers", [][2]string{{"grpc-status", "0"}}, 1, 1, "trailers"},
		{"empty-trailers", nil, 0, 1, "trailers"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			headers := make([]map[string]any, 0, len(tt.fields))
			for _, h := range tt.fields {
				headers = append(headers, map[string]any{"Name": h[0], "Value": h[1]})
			}
			kind, err := h3ValidateFieldSection(headers, tt.dir, tt.prior)
			require.NoError(t, err)
			require.Equal(t, tt.kind, kind)
		})
	}
}

func TestHTTP3MalformedHeaderAfterOutOfOrderCompletionKeepsAllSources(t *testing.T) {
	q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3"}
	bad := h3LiteralFields([2]string{":method", "GET"}, [2]string{":scheme", "https"}, [2]string{":authority", "example.com"})
	feed := func(pdu, off uint64, b []byte) (map[string]any, error) {
		q.eventID, q.eventRefs = pdu, []PacketReference{{Number: pdu}}
		fr := map[string]any{"Frame Type": "STREAM", "Stream ID": uint64(0), "Offset": off, "Stream Data": b, "FIN": false}
		err := q.applyFrames(0, quicSpaceApplication, []map[string]any{fr}, map[string]any{}, 1024)
		return fr, err
	}
	_, err := feed(1, 8, bad[8:])
	require.NoError(t, err, "an incomplete field section does not prove missing mandatory fields")
	fr, err := feed(2, 0, bad[:8])
	require.Error(t, err)
	hf := fr["HTTP3 Frames"].([]map[string]any)[0]
	require.Equal(t, []uint64{1, 2}, hf["Byte Source"].(map[string]any)["Parent PDUs"])
	require.Equal(t, "malformed", hf["Header Conformance Status"])
	require.Zero(t, q.streams[0].requestPDU, "bad request cannot become a confirmed response association")
}

func TestHTTP3FieldErrorDoesNotDiscardOtherStreamsInSameQUICPacket(t *testing.T) {
	q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3", eventID: 1, eventRefs: []PacketReference{{Number: 1}}}
	bad := map[string]any{"Frame Type": "STREAM", "Stream ID": uint64(0), "Offset": uint64(0), "Stream Data": h3LiteralFields([2]string{":method", "GET"}), "FIN": true}
	good := map[string]any{"Frame Type": "STREAM", "Stream ID": uint64(4), "Offset": uint64(0), "Stream Data": h3RequestHeaders(), "FIN": true}
	info := map[string]any{}
	err := q.applyFrames(0, quicSpaceApplication, []map[string]any{bad, good}, info, 1024)
	require.Error(t, err, "the bad request still has a diagnostic")
	require.NotEmpty(t, good["HTTP3 Frames"], "a stream-local HTTP error must not discard another STREAM frame in the same authenticated packet")
	require.Equal(t, 1, q.streams[4].h3.reqHeaders)
	require.Equal(t, uint64(1), q.streams[4].requestPDU)
}

func TestHTTP3ResponseDoesNotClaimAssociationAfterMalformedRequestTrailers(t *testing.T) {
	q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3", eventID: 1, eventRefs: []PacketReference{{Number: 1}}}
	w := append(h3RequestHeaders(), h3LiteralFields([2]string{":path", "/another"})...)
	fr := map[string]any{"Frame Type": "STREAM", "Stream ID": uint64(0), "Offset": uint64(0), "Stream Data": w, "FIN": true}
	require.Error(t, q.applyFrames(0, quicSpaceApplication, []map[string]any{fr}, map[string]any{}, 1024))
	q.eventID, q.eventRefs = 2, []PacketReference{{Number: 2}}
	fr = map[string]any{"Frame Type": "STREAM", "Stream ID": uint64(0), "Offset": uint64(0), "Stream Data": h3ResponseHeaders(), "FIN": true}
	require.NoError(t, q.applyFrames(1, quicSpaceApplication, []map[string]any{fr}, map[string]any{}, 1024))
	h := fr["HTTP3 Frames"].([]map[string]any)[0]
	require.NotEqual(t, true, h["Associated Request"])
	require.Zero(t, fr["Request PDU ID"])
}

func FuzzHTTP3FieldErrorStreamIsolation(f *testing.F) {
	f.Add([]byte("valid-name\x00value"))
	f.Add([]byte{0, 255, 13, 10, 0, 1, 128})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 256 {
			data = data[:256]
		}
		fields := [][2]string{{":method", "GET"}, {":scheme", "https"}, {":path", "/"}, {":authority", "example.com"}}
		for at := 0; at < len(data); {
			n := min(1+int(data[at])%12, len(data)-at)
			name := string(data[at : at+n])
			at += n
			n = min(16, len(data)-at)
			fields = append(fields, [2]string{name, string(data[at : at+n])})
			at += n
		}
		q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3", eventID: 1, eventRefs: []PacketReference{{Number: 1}}}
		bad := map[string]any{"Frame Type": "STREAM", "Stream ID": uint64(0), "Offset": uint64(0), "Stream Data": h3LiteralFields(fields...), "FIN": true}
		good := map[string]any{"Frame Type": "STREAM", "Stream ID": uint64(4), "Offset": uint64(0), "Stream Data": h3RequestHeaders(), "FIN": true}
		err := q.applyFrames(0, quicSpaceApplication, []map[string]any{bad, good}, map[string]any{}, 1024)
		if err != nil && !isH3FieldStreamError(err) {
			t.Fatalf("literal QPACK field construction caused a connection error: %v", err)
		}
		if good["HTTP3 Frames"] == nil || q.streams[4].h3.reqHeaders != 1 {
			t.Fatal("arbitrary field content discarded another stream")
		}
	})
}
