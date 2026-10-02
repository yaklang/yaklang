package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

// These synthetic exchanges follow RFC 8484: DNS ID zero is recommended,
// correlation belongs to the HTTP exchange, and non-2xx HTTP bodies are not DNS
// answers. The generated PCAPs below exercise TCP reassembly, not a claimed
// capture from a resolver deployment.
func dohExchangeH2Start() []sessionStep {
	return []sessionStep{
		{0, append([]byte(binH2Preface), h2TestFrame(4, 0, 0, nil)...)},
		{1, h2TestFrame(4, 0, 0, nil)},
		{0, h2TestFrame(4, 1, 0, nil)},
		{1, h2TestFrame(4, 1, 0, nil)},
	}
}

func dohExchangeH2GET(t *testing.T, id uint32, name string) sessionStep {
	t.Helper()
	path := "/dns-query?dns=" + base64.RawURLEncoding.EncodeToString(dnsWire(dnsQuery(0, name, 1)))
	return sessionStep{0, h2TestFrame(1, 5, id, h2TestHeaders(t, ":method", "GET", ":scheme", "https", ":path", path, ":authority", "dns.example.test"))}
}

func dohExchangeH2Response(t *testing.T, id uint32, name string) []sessionStep {
	t.Helper()
	return []sessionStep{
		{1, h2TestFrame(1, 4, id, h2TestHeaders(t, ":status", "200", "content-type", "application/dns-message"))},
		{1, h2TestFrame(0, 1, id, dnsWire(dnsAResponse(0, name, [4]byte{1, 2, 3, 4})))},
	}
}

func TestDoHHTTP1ZeroIDPipelinedExchanges(t *testing.T) {
	first := dohGET("dns.example.test", "one.example", 0)
	ordinary := []byte("GET /health HTTP/1.1\r\nHost: dns.example.test\r\n\r\n")
	second := dohPOST("dns.example.test", dnsWire(dnsQuery(0, "two.example", 1)))
	requests := append(append(bytes.Clone(first), ordinary...), second...)
	responses := []byte("HTTP/1.1 103 Early Hints\r\n\r\n")
	responses = append(responses, dohHTTPResp(200, dnsWire(dnsAResponse(0, "one.example", [4]byte{1, 2, 3, 4})))...)
	responses = append(responses, []byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok")...)
	responses = append(responses, dohHTTPResp(200, dnsWire(dnsAResponse(0, "two.example", [4]byte{5, 6, 7, 8})))...)
	for _, deferred := range []bool{false, true} {
		for _, chunk := range []int{0, 1, 7, 64} {
			t.Run(fmt.Sprintf("deferred=%v/chunk=%d", deferred, chunk), func(t *testing.T) {
				events, _ := sessionTestFlow(t, "http", []sessionStep{{0, requests}, {1, responses}}, chunk, deferred)
				var matched []string
				for _, event := range events {
					require.Empty(t, event.Error)
					if event.Session["Packet Name"] == "Response" {
						matched = append(matched, event.Session["Matched Request"].(string))
						require.Equal(t, event.Session["QNAME"], event.Session["Matched Request"])
					}
				}
				require.Equal(t, []string{"one.example", "two.example"}, matched)
			})
		}
	}
}

func TestDoHHTTP1ErrorDequeuesItsExchange(t *testing.T) {
	for _, status := range []int{302, 415} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			events, _ := sessionTestFlow(t, "http", []sessionStep{
				{0, dohGET("dns.example.test", "one.example", 0)},
				{0, dohGET("dns.example.test", "two.example", 0)},
				{1, dohHTTPResp(status, nil)},
				{1, dohHTTPResp(200, dnsWire(dnsAResponse(0, "two.example", [4]byte{1, 2, 3, 4})))},
			}, 1, false)
			require.Len(t, events, 4)
			for _, e := range events {
				require.Empty(t, e.Error)
			}
			require.Equal(t, "http-error", events[2].Session["Association Status"])
			require.Equal(t, "two.example", events[3].Session["Matched Request"])
		})
	}
}

func TestDoHHTTP2ZeroIDParallelExchanges(t *testing.T) {
	steps := dohExchangeH2Start()
	steps = append(steps, dohExchangeH2GET(t, 1, "one.example"))
	steps = append(steps, sessionStep{0, h2TestFrame(1, 4, 3, h2TestHeaders(t, ":method", "POST", ":scheme", "https", ":path", "/dns-query", ":authority", "dns.example.test", "content-type", "application/dns-message"))})
	steps = append(steps, sessionStep{0, h2TestFrame(0, 1, 3, dnsWire(dnsQuery(0, "two.example", 1)))})
	// The second stream completes first; DNS ID zero cannot identify either one.
	steps = append(steps, dohExchangeH2Response(t, 3, "two.example")...)
	steps = append(steps, dohExchangeH2Response(t, 1, "one.example")...)
	for _, chunk := range []int{0, 1, 7, 64} {
		for _, deferred := range []bool{false, true} {
			t.Run(fmt.Sprintf("chunk=%d/deferred=%v", chunk, deferred), func(t *testing.T) {
				events, _ := sessionTestFlow(t, "http2", steps, chunk, deferred)
				var matched []string
				for _, e := range events {
					require.Empty(t, e.Error)
					if e.Session["Packet Name"] == "Response" {
						matched = append(matched, e.Session["Matched Request"].(string))
						require.Equal(t, e.Session["QNAME"], e.Session["Matched Request"])
					}
				}
				require.Equal(t, []string{"two.example", "one.example"}, matched)
			})
		}
	}
}

func TestDoHHTTP2ErrorBodyKeepsOtherStreamsAlive(t *testing.T) {
	for _, ending := range []string{"data", "trailers", "headers"} {
		for _, contentType := range []string{"text/plain", "application/dns-message"} {
			t.Run(ending+"/"+contentType, func(t *testing.T) {
				steps := dohExchangeH2Start()
				steps = append(steps, dohExchangeH2GET(t, 1, "one.example"))
				flags := byte(4)
				if ending == "headers" {
					flags = 5
				}
				steps = append(steps, sessionStep{1, h2TestFrame(1, flags, 1, h2TestHeaders(t, ":status", "415", "content-type", contentType))})
				if ending != "headers" {
					flags = 1
					if ending == "trailers" {
						flags = 0
					}
					steps = append(steps, sessionStep{1, h2TestFrame(0, flags, 1, []byte("error"))})
					if ending == "trailers" {
						steps = append(steps, sessionStep{1, h2TestFrame(1, 5, 1, h2TestHeaders(t, "x-end", "yes"))})
					}
				}
				steps = append(steps, dohExchangeH2GET(t, 3, "two.example"))
				steps = append(steps, dohExchangeH2Response(t, 3, "two.example")...)
				for _, chunk := range []int{0, 1, 7} {
					// Explicitly synthetic PCAP; DATA/HEADERS boundaries are distinct from
					// TCP segment boundaries, including a one-byte-segment replay.
					raw := sessionTestPCAP(t, steps, layers.TCPPort(8080), chunk, false, true)
					var events []*ProtocolEvent
					require.NoError(t, ReplayPcap(bytes.NewReader(raw), WithTCPReassemblyWorkers(1), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) })))
					var lastError, response *ProtocolEvent
					for _, e := range events {
						require.Empty(t, e.Error, "chunk=%d %s", chunk, e.Summary)
						if e.Session["Packet Name"] == "Error" {
							lastError = e
						}
						if e.Session["Packet Name"] == "Response" {
							response = e
						}
					}
					require.NotNil(t, lastError)
					require.Equal(t, "415", lastError.Session["HTTP Status"])
					if ending != "headers" {
						require.Equal(t, []byte("error"), lastError.Session["Error Body"])
					}
					require.NotNil(t, response)
					require.Equal(t, "two.example", response.Session["Matched Request"])
				}
			})
		}
	}
}

func TestDoHHTTP2MalformedDNSIsStreamScoped(t *testing.T) {
	steps := dohExchangeH2Start()
	steps = append(steps, dohExchangeH2GET(t, 1, "one.example"))
	steps = append(steps,
		sessionStep{1, h2TestFrame(1, 4, 1, h2TestHeaders(t, ":status", "200", "content-type", "application/dns-message"))},
		sessionStep{1, h2TestFrame(0, 1, 1, []byte("bad DNS"))},
		dohExchangeH2GET(t, 3, "two.example"),
	)
	steps = append(steps, dohExchangeH2Response(t, 3, "two.example")...)
	events, _ := sessionTestFlow(t, "http2", steps, 1, false)
	malformed, responses := 0, 0
	for _, e := range events {
		if e.Status == "malformed" {
			malformed++
			require.Equal(t, "stream", e.Session["Error Scope"])
			continue
		}
		require.Empty(t, e.Error)
		if e.Session["Packet Name"] == "Response" {
			responses++
			require.Equal(t, "two.example", e.Session["Matched Request"])
		}
	}
	require.Equal(t, 1, malformed)
	require.Equal(t, 1, responses)
}

func TestDoHHTTP2TrailersAndReset(t *testing.T) {
	steps := dohExchangeH2Start()
	steps = append(steps, dohExchangeH2GET(t, 1, "reset.example"))
	steps = append(steps, sessionStep{1, h2TestFrame(3, 0, 1, []byte{0, 0, 0, 0})})
	steps = append(steps, dohExchangeH2GET(t, 3, "two.example"))
	steps = append(steps,
		sessionStep{1, h2TestFrame(1, 4, 3, h2TestHeaders(t, ":status", "200", "content-type", "application/dns-message"))},
		sessionStep{1, h2TestFrame(0, 0, 3, dnsWire(dnsAResponse(0, "two.example", [4]byte{1, 2, 3, 4})))},
		sessionStep{1, h2TestFrame(1, 5, 3, h2TestHeaders(t, "x-end", "yes"))},
	)
	events, _ := sessionTestFlow(t, "http2", steps, 7, false)
	for _, e := range events {
		require.Empty(t, e.Error)
	}
	require.Equal(t, "Response", events[len(events)-1].Session["Packet Name"])
	require.Equal(t, "two.example", events[len(events)-1].Session["Matched Request"])
}

func TestDoHClickHouseRecognitionRegressionCaptures(t *testing.T) {
	root := filepath.Join("..", "..", "bin-parser", "testdata", "protocol-recognition-regressions", "doh-clickhouse")
	raw, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	require.NoError(t, err)
	var manifest struct {
		Kind  string                          `json:"kind"`
		Files []struct{ File, SHA256 string } `json:"files"`
	}
	require.NoError(t, json.Unmarshal(raw, &manifest))
	require.Equal(t, "synthetic", manifest.Kind)
	require.Len(t, manifest.Files, 6)
	for _, fixture := range manifest.Files {
		t.Run(fixture.File, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(root, fixture.File))
			require.NoError(t, err)
			require.Equal(t, fixture.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
			var events []*ProtocolEvent
			require.NoError(t, ReplayPcap(bytes.NewReader(raw), WithTCPReassemblyWorkers(1), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) })))
			if fixture.File == "doh-h2-dns-semantics.pcapng" {
				assertDoHDNSSemanticsEvents(t, events)
				return
			}
			var matched, names []string
			for _, event := range events {
				if fixture.File == "clickhouse-server-hello-pong-ambiguous.pcapng" {
					require.Equal(t, "context-required", event.Status)
					require.Empty(t, event.Fields["Role"])
					continue
				}
				require.Empty(t, event.Error, "%s", event.Summary)
				if name, ok := event.Fields["Packet Name"].(string); ok {
					names = append(names, name)
				}
				if event.Session["Packet Name"] == "Response" {
					matched = append(matched, event.Session["Matched Request"].(string))
				}
			}
			switch fixture.File {
			case "doh-h1-id-zero-pipeline.pcapng":
				require.Equal(t, []string{"one.example", "two.example"}, matched)
			case "doh-h2-id-zero-reordered.pcapng":
				require.Equal(t, []string{"two.example", "one.example"}, matched)
			case "doh-h2-error-followed-valid.pcapng":
				require.Equal(t, []string{"two.example"}, matched)
			case "clickhouse-client-first-coalesced.pcapng":
				require.Equal(t, []string{"Hello", "Ping", "Hello", "Pong"}, names)
			case "clickhouse-server-hello-pong-ambiguous.pcapng":
				require.Len(t, events, 1)
			}
		})
	}
}

func TestDoHHTTP2UnsupportedMediaIsStreamScoped(t *testing.T) {
	steps := dohExchangeH2Start()
	steps = append(steps, dohExchangeH2GET(t, 1, "one.example"))
	steps = append(steps,
		sessionStep{1, h2TestFrame(1, 4, 1, h2TestHeaders(t, ":status", "200", "content-type", "application/dns-json"))},
		sessionStep{1, h2TestFrame(0, 1, 1, []byte(`{"Status":0}`))},
		dohExchangeH2GET(t, 3, "two.example"),
	)
	steps = append(steps, dohExchangeH2Response(t, 3, "two.example")...)
	events, _ := sessionTestFlow(t, "http2", steps, 7, false)
	unsupported, responses := 0, 0
	for _, e := range events {
		if e.Status == "context-required" {
			unsupported++
			require.Contains(t, e.Error, string(ErrUnsupportedFeature))
			require.Equal(t, "stream", e.Session["Error Scope"])
			continue
		}
		require.Empty(t, e.Error)
		if e.Session["Packet Name"] == "Response" {
			responses++
			require.Equal(t, "two.example", e.Session["Matched Request"])
		}
	}
	require.Equal(t, 1, unsupported)
	require.Equal(t, 1, responses)
}

func TestDoHHTTP2EarlyErrorDoesNotRecreatePendingRequest(t *testing.T) {
	steps := dohExchangeH2Start()
	steps = append(steps,
		sessionStep{0, h2TestFrame(1, 4, 1, h2TestHeaders(t, ":method", "POST", ":scheme", "https", ":path", "/dns-query", ":authority", "dns.example.test", "content-type", "application/dns-message"))},
		sessionStep{1, h2TestFrame(1, 5, 1, h2TestHeaders(t, ":status", "415", "content-type", "text/plain"))},
		sessionStep{0, h2TestFrame(0, 1, 1, dnsWire(dnsQuery(0, "one.example", 1)))},
	)
	events, _ := sessionTestFlow(t, "http2", steps, 1, false)
	for _, e := range events {
		require.Empty(t, e.Error)
	}
	require.Equal(t, "Query", events[len(events)-1].Session["Packet Name"])
	require.Equal(t, false, events[len(events)-1].Session["Outstanding"])
}

func TestDoHHTTP2FailedStreamCannotBeReadmittedByResponseHeaders(t *testing.T) {
	steps := dohExchangeH2Start()
	steps = append(steps,
		sessionStep{0, h2TestFrame(1, 4, 1, h2TestHeaders(t, ":method", "POST", ":scheme", "https", ":path", "/dns-query", ":authority", "dns.example.test", "content-type", "application/dns-message"))},
		sessionStep{0, h2TestFrame(0, 1, 1, []byte("bad DNS"))},
	)
	steps = append(steps, dohExchangeH2Response(t, 1, "one.example")...)
	steps = append(steps, dohExchangeH2GET(t, 3, "two.example"))
	steps = append(steps, dohExchangeH2Response(t, 3, "two.example")...)
	events, _ := sessionTestFlow(t, "http2", steps, 1, false)
	malformed, invalidated, responses := 0, 0, 0
	for _, e := range events {
		if e.Status == "malformed" {
			malformed++
			require.Equal(t, "stream", e.Session["Error Scope"])
			continue
		}
		require.Empty(t, e.Error)
		if e.Session["DoH State"] == "invalid-message-stream" {
			invalidated++
			require.Equal(t, "http2", e.Protocol)
			require.Equal(t, uint32(1), e.Session["Stream ID"])
			require.Nil(t, e.Session["DNS"])
			require.Nil(t, e.Session["Matched Request"])
		}
		if e.Session["Packet Name"] == "Response" {
			responses++
			require.Equal(t, "two.example", e.Session["Matched Request"])
		}
	}
	require.Equal(t, 1, malformed)
	require.Equal(t, 2, invalidated)
	require.Equal(t, 1, responses)
}
