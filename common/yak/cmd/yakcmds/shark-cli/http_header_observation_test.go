package sharkcli

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func httpHeaderObservationFixture(t *testing.T) (*pcaputil.ProtocolEvent, []byte) {
	t.Helper()
	s, err := pcaputil.NewProtocolSession(pcaputil.DefaultParserBudget())
	require.NoError(t, err)
	header := "HTTP/1.1 200 OK\r\nContent-Length: 2\r\nX-Repeat: one\r\nX-Repeat: two\r\n\r\n"
	tail := []byte("OKHTTP/1.1 204 No Content\r\n\r\n\xff\x00")
	r := s.Feed(1, time.Unix(100, 0), append([]byte(header), tail...))
	require.Len(t, r.Events, 1)
	s.Close("eof")
	e := r.Events[0]
	e.Fields, err = e.GetFields()
	require.NoError(t, err)
	return e, tail
}

func TestHTTPHeaderObservationHasProgressiveOwnedSources(t *testing.T) {
	e, tail := httpHeaderObservationFixture(t)
	p := presentEvents([]*pcaputil.ProtocolEvent{e})
	require.Equal(t, "HTTP/1.1", p.protocol)
	require.Equal(t, "headers", p.visibility)
	require.Equal(t, "context-required", p.status)
	require.Contains(t, p.summary, "200 OK")
	require.Contains(t, p.summary, "header only")
	require.Contains(t, p.summary, "request method unavailable")
	fs, ss := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
	for name, d := range map[string]packetDetail{
		"direct": {fields: fs, sources: ss}, "worker": workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, false),
		"stream-worker": workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, true),
	} {
		t.Run(name, func(t *testing.T) {
			if name != "direct" {
				require.Equal(t, "HTTP/1.1", d.protocol, "the worker title retains the qualified partial-header identity")
			}
			text := detailLines(d.fields)
			require.Contains(t, text, "HTTP/1.1 response header · observation")
			require.Contains(t, text, "Body boundary: unknown · request method unavailable")
			require.NotContains(t, text, "HTTP · PDU")
			require.NotContains(t, text, "Message bytes complete")
			headerSource, tailSource := -1, -1
			for i, source := range d.sources {
				switch {
				case strings.HasPrefix(source.Name, "HTTP/1.1 response header"):
					headerSource = i + 1
					require.Equal(t, e.Raw, source.Data)
					require.NotContains(t, source.Name, "PDU")
				case strings.HasPrefix(source.Name, "Unframed HTTP stream bytes"):
					tailSource = i + 1
					require.Equal(t, tail, source.Data)
					require.NotContains(t, source.Name, "body")
				}
			}
			require.Positive(t, headerSource)
			require.Positive(t, tailSource)
			require.NotEqual(t, headerSource, tailSource)
			for _, line := range []string{"X-Repeat: one", "X-Repeat: two"} {
				found := false
				for _, f := range d.fields {
					if f.text == line {
						found = true
						require.Equal(t, headerSource, f.source)
						require.True(t, f.hasRange)
						require.Equal(t, line, string(d.sources[f.source-1].Data[f.start:f.end]))
					}
				}
				require.True(t, found, "duplicate header lines retain their own literal range")
			}
		})
	}
}

func TestHTTPHeaderObservationDoesNotPromoteUnqualifiedMetadata(t *testing.T) {
	for name, mutate := range map[string]func(*pcaputil.ProtocolEvent){
		"wrong-protocol":  func(e *pcaputil.ProtocolEvent) { e.Protocol = "tls" },
		"wrong-carrier":   func(e *pcaputil.ProtocolEvent) { e.Transport = "udp" },
		"wrong-profile":   func(e *pcaputil.ProtocolEvent) { e.Profile = "http" },
		"wrong-admission": func(e *pcaputil.ProtocolEvent) { e.Admission = "port" },
		"decoded":         func(e *pcaputil.ProtocolEvent) { e.Status = "decoded" },
		"complete":        func(e *pcaputil.ProtocolEvent) { e.Completeness = "message" },
		"wrong-hash":      func(e *pcaputil.ProtocolEvent) { e.SourceBytes.SHA256 = strings.Repeat("0", 64) },
		"partial-header":  func(e *pcaputil.ProtocolEvent) { e.Raw = e.Raw[:len(e.Raw)-1] },
		"extra-body":      func(e *pcaputil.ProtocolEvent) { e.Raw = append(e.Raw, 'X') },
		"request-context": func(e *pcaputil.ProtocolEvent) { e.Session["Request Context Available"] = true },
		"body-known":      func(e *pcaputil.ProtocolEvent) { e.Session["Body Boundary Known"] = true },
		"method":          func(e *pcaputil.ProtocolEvent) { e.Session["Method"] = "GET" },
		"response-to":     func(e *pcaputil.ProtocolEvent) { e.ResponseTo = 17 },
		"completion":      func(e *pcaputil.ProtocolEvent) { e.SourceBytes.CompletedIn = &pcaputil.PacketReference{Number: 17} },
		"bad-count":       func(e *pcaputil.ProtocolEvent) { e.Session["Unframed Stream Bytes"] = -1 },
	} {
		t.Run(name, func(t *testing.T) {
			e, _ := httpHeaderObservationFixture(t)
			mutate(e)
			fields, _ := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
			require.NotContains(t, detailLines(fields), "HTTP/1.1 response header · observation")
			p := presentEvents([]*pcaputil.ProtocolEvent{e})
			require.NotEqual(t, "headers", p.visibility, "unqualified metadata cannot gain the header observation contract")
		})
	}
}

func TestHTTPHeaderObservationRejectsUnboundTailSources(t *testing.T) {
	for name, mutate := range map[string]func(map[string]any){
		"hash":   func(m map[string]any) { m["Prefix SHA256"] = strings.Repeat("0", 64) },
		"length": func(m map[string]any) { m["Prefix Bytes"] = 1 },
		"offset": func(m map[string]any) { m["Offset"] = 0 },
		"kind":   func(m map[string]any) { m["Source Kind"] = "captured" },
		"base64-looking-text": func(m map[string]any) {
			m["Unframed Stream Prefix"] = "T0s="
		},
		"full-hash": func(m map[string]any) { m["Observed SHA256"] = strings.Repeat("0", 64) },
	} {
		t.Run(name, func(t *testing.T) {
			e, tail := httpHeaderObservationFixture(t)
			mutate(e.Session["Unframed Stream Observation"].(map[string]any))
			_, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
			for _, source := range sources {
				require.False(t, strings.HasPrefix(source.Name, "Unframed HTTP stream"), fmt.Sprintf("invalid observation source %s accepted", name))
			}
			require.NotEmpty(t, tail)
		})
	}
}

func TestHTTPHeaderObservationDoesNotMutateNativeBytes(t *testing.T) {
	e, tail := httpHeaderObservationFixture(t)
	header := bytes.Clone(e.Raw)
	fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
	require.NotEmpty(t, fields)
	for i := range sources {
		if len(sources[i].Data) > 0 {
			sources[i].Data[0] ^= 0xff
		}
	}
	require.Equal(t, header, e.Raw)
	require.Equal(t, tail, e.Session["Unframed Stream Observation"].(map[string]any)["Unframed Stream Prefix"])
}

func TestHTTPLaterUnframedBytesUseHeaderContextWithoutMessages(t *testing.T) {
	s, err := pcaputil.NewProtocolSession(pcaputil.DefaultParserBudget())
	require.NoError(t, err)
	r := s.Feed(1, time.Unix(100, 0), []byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\n"))
	require.Len(t, r.Events, 1)
	headerID := r.Events[0].ID
	tail := []byte("OKHTTP/1.1 204 No Content\r\n\r\n\xff\x00")
	r = s.Feed(1, time.Unix(101, 0), tail)
	require.Len(t, r.Events, 1)
	e := r.Events[0]
	p := presentEvents([]*pcaputil.ProtocolEvent{e})
	require.Equal(t, "HTTP/1.1", p.protocol)
	require.Equal(t, "unframed-stream", p.visibility)
	require.Contains(t, p.summary, "unframed stream bytes")
	require.Contains(t, p.scope, "header context")
	fs, ss := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
	for name, d := range map[string]packetDetail{"direct": {fields: fs, sources: ss}, "worker": workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, false), "stream": workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, true)} {
		t.Run(name, func(t *testing.T) {
			if name != "direct" {
				require.Equal(t, "HTTP/1.1", d.protocol, "stream title must retain the explicitly qualified header context")
			}
			text := detailLines(d.fields)
			require.Contains(t, text, "HTTP response context · unframed observation")
			require.Contains(t, text, fmt.Sprintf("Observed header PDU: #%d", headerID))
			require.NotContains(t, text, "PDU #"+fmt.Sprint(e.ID))
			require.NotContains(t, text, "Status Code: 204")
			require.NotContains(t, text, "Message bytes complete")
			found := false
			for _, source := range d.sources {
				if strings.HasPrefix(source.Name, "Unframed HTTP stream bytes") {
					found = true
					require.Equal(t, tail, source.Data)
				}
			}
			require.True(t, found)
		})
	}
	s.Close("eof")
}

func TestHTTPHeaderObservationSupportsEntireNativeHeaderLimit(t *testing.T) {
	for _, length := range []int{(16 << 10) + 1024, 32 << 10} {
		t.Run(fmt.Sprint(length), func(t *testing.T) {
			s, err := pcaputil.NewProtocolSession(pcaputil.DefaultParserBudget())
			require.NoError(t, err)
			prefix, suffix := "HTTP/1.1 200 OK\r\nX-Long: ", "\r\n\r\n"
			header := []byte(prefix + strings.Repeat("x", length-len(prefix)-len(suffix)) + suffix)
			r := s.Feed(1, time.Unix(100, 0), header)
			require.Len(t, r.Events, 1)
			e := r.Events[0]
			require.Equal(t, "HTTP/1.1", presentEvents([]*pcaputil.ProtocolEvent{e}).protocol)
			fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
			require.Contains(t, detailLines(fields), "HTTP/1.1 response header · observation")
			require.Len(t, sources, 1)
			require.Equal(t, header, sources[0].Data)
			s.Close("eof")
		})
	}
}

func TestHTTPUnframedArrivalCannotBePromotedByPublicMetadata(t *testing.T) {
	for name, mutate := range map[string]func(*pcaputil.ProtocolEvent){
		"decoded":        func(e *pcaputil.ProtocolEvent) { e.Status, e.Error = "decoded", "" },
		"complete":       func(e *pcaputil.ProtocolEvent) { e.Completeness = "message" },
		"wrong-kind":     func(e *pcaputil.ProtocolEvent) { e.SourceBytes.Kind = "captured" },
		"wrong-hash":     func(e *pcaputil.ProtocolEvent) { e.SourceBytes.SHA256 = strings.Repeat("0", 64) },
		"body-known":     func(e *pcaputil.ProtocolEvent) { e.Session["Body Boundary Known"] = true },
		"method":         func(e *pcaputil.ProtocolEvent) { e.Session["Method"] = "GET" },
		"header-id":      func(e *pcaputil.ProtocolEvent) { e.Session["Observed Header PDU"] = 0 },
		"header-version": func(e *pcaputil.ProtocolEvent) { e.Session["Observed Header Protocol"] = "HTTP/2" },
		"whole-hash":     func(e *pcaputil.ProtocolEvent) { e.Session["Observed SHA256"] = strings.Repeat("0", 64) },
		"count":          func(e *pcaputil.ProtocolEvent) { e.Session["Observed Bytes"] = 1 },
		"response-to":    func(e *pcaputil.ProtocolEvent) { e.ResponseTo = 3 },
		"completed-in":   func(e *pcaputil.ProtocolEvent) { e.SourceBytes.CompletedIn = &pcaputil.PacketReference{Number: 3} },
	} {
		t.Run(name, func(t *testing.T) {
			s, err := pcaputil.NewProtocolSession(pcaputil.DefaultParserBudget())
			require.NoError(t, err)
			s.Feed(1, time.Unix(100, 0), []byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\n"))
			r := s.Feed(1, time.Unix(101, 0), []byte("OKHTTP/1.1 204 No Content\r\n\r\n"))
			require.Len(t, r.Events, 1)
			e := r.Events[0]
			mutate(e)
			e.Fields = map[string]any{"Status Code": 204, "Body": "forged"}
			p := presentEvents([]*pcaputil.ProtocolEvent{e})
			require.Empty(t, p.protocol, "a partial observation cannot gain a full-message identity")
			fields, _ := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
			text := detailLines(fields)
			require.NotContains(t, text, "HTTP · PDU")
			require.NotContains(t, text, "forged")
			require.NotContains(t, text, "Status Code: 204")
			s.Close("eof")
		})
	}
}
