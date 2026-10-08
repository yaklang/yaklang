package pcaputil

import (
	"bytes"
	"compress/gzip"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestFirstBatchT04(t *testing.T) {
	e := &ProtocolEvent{ID: 1, Status: "decoded", Protocol: "dns", Raw: []byte{1, 2}, Session: map[string]any{"DNS": map[string]any{"ID": uint16(7), "Questions": []map[string]any{{"Name": "a.example"}}}}, semanticFields: map[string]any{"Value": []byte{3}}, SourceBytes: ByteSource{PacketRefs: []PacketReference{{Number: 4}}, ParentPDUs: []uint64{5}}}
	v, err := NewProtocolInspector(2, 8192)
	require.NoError(t, err)
	v.OnEvent(e)
	e.Raw[0] = 99
	e.SourceBytes.PacketRefs[0].Number = 99
	e.semanticFields["Value"].([]byte)[0] = 99
	d, err := v.Details(1)
	require.NoError(t, err)
	require.Equal(t, byte(1), d.Raw[0])
	require.Equal(t, uint64(4), d.SourceBytes.PacketRefs[0].Number)
	require.Equal(t, []byte{3}, d.Fields["Value"])
	d.Fields["Value"].([]byte)[0] = 0
	d, err = v.Details(1)
	require.NoError(t, err)
	require.Equal(t, []byte{3}, d.Fields["Value"])
	tiny, err := NewProtocolInspector(1, 64)
	require.NoError(t, err)
	tiny.OnEvent(e)
	require.Equal(t, uint64(1), tiny.Evicted())
	require.Empty(t, tiny.Rows("", 0))
}
func TestFirstBatchT05Filters(t *testing.T) {
	e := &ProtocolEvent{Protocol: "dns", ID: 1<<60 + 7, Source: "192.0.2.1:1000", Destination: "[2001:db8::1]:53", Session: map[string]any{"DNS": map[string]any{"ID": uint16(7), "Questions": []map[string]any{{"Name": "a.example", "Type": uint16(1)}}}}}
	for _, expr := range []string{`dns.qry.name`, `dns.qry.name == "a.example" and dns.id >= 7`, `ip.addr in 192.0.2.0/24`, `ip.addr in 2001:db8::/32`, `not (protocol == "http") and dns.qry.name contains "example"`, `pdu.id == 1152921504606846983`} {
		f, err := CompileDisplayFilter(expr)
		require.NoError(t, err, expr)
		require.True(t, f.Match(e), expr)
	}
	for _, expr := range []string{`dns.id != 7`, `tls.version != 0`, `dns.qry.name == "b"`, `pdu.id == 1152921504606846984`} {
		f, err := CompileDisplayFilter(expr)
		require.NoError(t, err)
		require.False(t, f.Match(e), expr)
	}
	for _, expr := range []string{`unknown == 1`, `dns.id ==`, `(dns.id == 7`, `dns.id === 7`} {
		_, err := CompileDisplayFilter(expr)
		require.Error(t, err, expr)
	}
}
func TestFirstBatchT11(t *testing.T) {
	var compressed bytes.Buffer
	gz := gzip.NewWriter(&compressed)
	_, err := gz.Write([]byte{10, 3, 'm', '1', 'x'})
	require.NoError(t, err)
	require.NoError(t, gz.Close())
	steps := append([]sessionStep{}, h2TestFixtures(t)[:4]...)
	steps = append(steps, sessionStep{0, h2TestFrame(1, 4, 1, h2TestHeaders(t, ":method", "POST", ":scheme", "http", ":path", "/M1/Bidi", "content-type", "application/grpc", "grpc-encoding", "gzip"))})
	payload := append([]byte{1, 0, 0, 0, byte(compressed.Len())}, compressed.Bytes()...)
	steps = append(steps, sessionStep{0, h2TestFrame(0, 1, 1, payload)}, sessionStep{1, h2TestFrame(1, 4, 1, h2TestHeaders(t, ":status", "200", "content-type", "application/grpc"))}, sessionStep{1, h2TestFrame(1, 5, 1, h2TestHeaders(t, "grpc-status", "0"))})
	for _, chunk := range []int{0, 1, 2, 7} {
		events, stats := sessionTestFlow(t, "http2", steps, chunk, false)
		require.Zero(t, stats.a.stats().BufferedBytes)
		messages, status := 0, 0
		for _, e := range events {
			require.Empty(t, e.Error)
			if ms, ok := e.Session["GRPC Messages"].([]map[string]any); ok {
				require.Len(t, ms, 1)
				require.Equal(t, []byte{10, 3, 'm', '1', 'x'}, ms[0]["Decoded Payload"])
				messages++
			}
			if e.Session["GRPC Status"] == "0" {
				status++
			}
		}
		require.Equal(t, 1, messages)
		require.Equal(t, 1, status)
	}
	t.Run("stream-error-does-not-poison-next", func(t *testing.T) {
		s := reviewH2(t, "application/grpc")
		r := s.Feed(0, time.Unix(1, 0), h2TestFrame(0, 0, 1, []byte{2, 0, 0, 0, 0}))
		require.NotEmpty(t, r.Events)
		require.Equal(t, "stream", r.Events[0].Session["Error Scope"])
		r = s.Feed(0, time.Unix(1, 0), h2TestFrame(1, 5, 3, h2TestHeaders(t, ":method", "GET", ":scheme", "http", ":path", "/")))
		require.Nil(t, r.Err)
		require.Empty(t, r.Events[0].Error)
		s.Close("end")
		require.Zero(t, s.Stats().BufferedBytes)
	})
}
func FuzzFirstBatchM1(f *testing.F) {
	f.Add(m1DNSQuery(), "dns.id == 4660")
	f.Add([]byte{1, 0, 0, 1}, "not tls.version")
	f.Fuzz(func(t *testing.T, b []byte, expr string) {
		if len(b) > 65536 || len(expr) > 8192 {
			return
		}
		DecodeDNSMessage(b, 32)
		decodeDHCPv6(b, 0, 32)
		decodeDHCP4(b, 32)
		CompileDisplayFilter(expr)
		ParseTLSKeyLog(string(b))
	})
}

func TestFirstBatchT11BodySemantics(t *testing.T) {
	for _, tc := range []struct {
		name, method, status, length, body string
		bad                                bool
	}{
		{"exact", "POST", "200", "3", "abc", false},
		{"short", "POST", "200", "4", "abc", true},
		{"long", "POST", "200", "2", "abc", true},
		{"head-metadata-length", "HEAD", "200", "100", "", false},
		{"head-with-data", "HEAD", "200", "100", "x", true},
		{"204-with-data", "GET", "204", "", "x", true},
		{"304-metadata-length", "GET", "304", "100", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			steps := append([]sessionStep{}, h2TestFixtures(t)[:4]...)
			steps = append(steps, sessionStep{0, h2TestFrame(1, 5, 1, h2TestHeaders(t, ":method", tc.method, ":scheme", "http", ":path", "/"))})
			hs := []string{":status", tc.status}
			if tc.length != "" {
				hs = append(hs, "content-length", tc.length)
			}
			steps = append(steps, sessionStep{1, h2TestFrame(1, 4, 1, h2TestHeaders(t, hs...))}, sessionStep{1, h2TestFrame(0, 1, 1, []byte(tc.body))})
			s := newReviewSession(t, ParserBudget{})
			for i, step := range steps {
				r := s.Feed(step.dir, time.Time{}, step.wire)
				if i == len(steps)-1 && tc.bad {
					require.NotNil(t, r.Err)
					require.Equal(t, "stream", r.Events[0].Session["Error Scope"])
				} else {
					require.Nil(t, r.Err)
				}
			}
			s.Close("end")
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
	fields, err := DecodeProtobufWire([]byte{8, 150, 1, 18, 3, 'f', 'o', 'o'}, 16)
	require.NoError(t, err)
	require.Len(t, fields, 2)
	require.Equal(t, uint64(1), fields[0]["Number"])
	require.Equal(t, uint64(150), fields[0]["Value"])
	require.Equal(t, uint64(2), fields[1]["Wire Type"])
	require.NotContains(t, fields[1], "Name")
	_, err = DecodeProtobufWire([]byte{18, 255}, 16)
	require.Error(t, err)
}
func TestFirstBatchT10DecompressionBudget(t *testing.T) {
	var compressed bytes.Buffer
	gz := gzip.NewWriter(&compressed)
	_, err := gz.Write(bytes.Repeat([]byte{'x'}, 1<<20))
	require.NoError(t, err)
	require.NoError(t, gz.Close())
	_, err = DecodeBody(compressed.Bytes(), "gzip", 1024)
	require.Error(t, err)
	plain, err := DecodeBody(compressed.Bytes(), "gzip", 1<<20)
	require.NoError(t, err)
	require.Len(t, plain, 1<<20)
}

func TestFirstBatchT11PushIsolation(t *testing.T) {
	s := reviewH2(t, "application/grpc")
	// A pushed stream remains visibly unsupported, but its HPACK block must be
	// consumed so ordinary response streams can continue on the connection.
	push := append([]byte{0, 0, 0, 2}, h2TestHeaders(t, ":method", "GET", ":scheme", "https", ":authority", "example.test", ":path", "/push")...)
	r := s.Feed(1, time.Time{}, h2TestFrame(5, 4, 1, push))
	require.NotNil(t, r.Err)
	require.Equal(t, "stream", r.Events[0].Session["Error Scope"])
	r = s.Feed(1, time.Time{}, h2TestFrame(1, 5, 2, h2TestHeaders(t, ":status", "200")))
	require.NotNil(t, r.Err)
	require.Equal(t, "stream", r.Events[0].Session["Error Scope"])
	r = s.Feed(0, time.Time{}, h2TestFrame(1, 5, 3, h2TestHeaders(t, ":method", "GET", ":scheme", "http", ":path", "/ordinary")))
	require.Nil(t, r.Err)
	r = s.Feed(1, time.Time{}, h2TestFrame(1, 5, 3, h2TestHeaders(t, ":status", "200")))
	require.Nil(t, r.Err)
	require.Empty(t, r.Events[0].Error)
	s.Close("end")
	require.Zero(t, s.Stats().BufferedBytes)
}
