package pcaputil

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	yaklang "github.com/yaklang/yaklang/common/yak/antlr4yak"
)

func TestProtocolBudgetLargeHTTP(t *testing.T) {
	body := strings.Repeat("x", 2<<20)
	request := fmt.Sprintf("POST /upload HTTP/1.1\r\nHost: example.test\r\nContent-Length: %d\r\n\r\n%s", len(body), body)
	response := "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nOK"
	steps := []tcpStep{{seq: 0, syn: true}, {seq: 0, syn: true, reverse: true}}
	for at := 0; at < len(request); at += 8192 {
		steps = append(steps, tcpStep{seq: 1 + uint32(at), data: request[at:min(at+8192, len(request))]})
	}
	steps = append(steps, tcpStep{seq: 1, data: response, reverse: true})
	pcap := binTestPcap(t, steps, 80, false, false)
	for _, workers := range []int{1, 4} {
		for _, deferred := range []bool{false, true} {
			for _, small := range []bool{false, true} {
				t.Run(fmt.Sprintf("workers%d/deferred%v/small%v", workers, deferred, small), func(t *testing.T) {
					opts := []CaptureOption{WithProtocolDeferred(deferred)}
					if small {
						opts = append(opts, WithProtocolBudget(1<<20, 32<<20))
					}
					events, stats, err := binReplay(t, pcap, workers, opts...)
					require.NoError(t, err)
					for _, e := range events {
						t.Log(e.DumpLine())
					}
					require.Len(t, events, 2)
					requestEvent, responseEvent := events[0], events[1]
					expected := "decoded"
					if deferred {
						expected = "deferred"
					}
					require.Equal(t, expected, responseEvent.Status, responseEvent.DumpLine())
					require.Equal(t, requestEvent.ID, responseEvent.ResponseTo, "limited requests still supply response framing context")
					require.Zero(t, stats.BufferedBytes, "teardown must release message and request queue reservations")
					require.LessOrEqual(t, stats.PeakBufferedBytes, int64(128<<20))
					require.Zero(t, stats.ContextRequired)
					if small {
						require.Equal(t, "limited", requestEvent.Status)
						require.Equal(t, "HTTPPayloadOmitted", requestEvent.ExpertCode)
						require.Contains(t, requestEvent.Summary, fmt.Sprintf("message_bytes=%d", len(request)))
						require.Contains(t, requestEvent.Summary, "max_message_bytes=1048576")
						require.Equal(t, int64(len(request)), requestEvent.Session["Declared Message Bytes"])
						require.Less(t, len(requestEvent.Raw), 1024)
						require.Equal(t, "headers", requestEvent.Completeness)
						require.Equal(t, "POST", requestEvent.Fields["Method"])
					} else {
						require.Equal(t, expected, requestEvent.Status, requestEvent.DumpLine())
						require.Equal(t, request, string(requestEvent.Raw))
						require.Zero(t, stats.LimitedBytes)
						if !deferred {
							require.NotEmpty(t, requestEvent.Fields)
						}
					}
				})
			}
		}
	}
}

func TestProtocolBudgetOptionComposition(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		c := NewDefaultConfig()
		opts := []CaptureOption{WithProtocolBudget(32<<20, 256<<20), WithOnProtocolMessage(func(*ProtocolEvent) {}), WithOnProtocolStats(func(ProtocolStats) {}), WithProtocolDeferred(true)}
		if reverse {
			for i, j := 0, len(opts)-1; i < j; i, j = i+1, j-1 {
				opts[i], opts[j] = opts[j], opts[i]
			}
		}
		for _, opt := range opts {
			require.NoError(t, opt(c))
		}
		require.NoError(t, c.prepareBinParser())
		require.Equal(t, 32<<20, c.binParser.config.MaxMessageBytes)
		require.Equal(t, 256<<20, c.binParser.config.MaxBufferedBytes)
		require.True(t, c.binParser.config.Deferred)
		require.NotNil(t, c.binParser.config.OnEvent)
		require.NotNil(t, c.binParser.config.OnStats)
		require.Zero(t, c.binParser.buffered.Load(), "configuration must not preallocate budget")
	}
	for _, limits := range [][2]int{{-1, 128}, {0, 128}, {63, 128}, {65 << 20, 128 << 20}, {1024, 512}} {
		require.Error(t, WithProtocolBudget(limits[0], limits[1])(NewDefaultConfig()))
	}
	c := NewDefaultConfig()
	require.NoError(t, WithProtocolBudget(64<<20, 256<<20)(c))
	require.NoError(t, c.prepareBinParser())
	require.Nil(t, c.binParser, "budget alone does not subscribe to parsing")
	engine := yaklang.New()
	engine.SetVars(map[string]any{"pcapx": Exports, "check": func(opt CaptureOption) { require.NoError(t, opt(c)) }})
	require.NoError(t, engine.SafeEval(context.Background(), `check(pcapx.pcap_protocolBudget(32*1024*1024,256*1024*1024))`))
	require.Equal(t, 32<<20, c.binParserConfig.MaxMessageBytes)
}

func TestProtocolBudgetHTTPHugeDeclaration(t *testing.T) {
	steps := []tcpStep{{seq: 0, syn: true}, {seq: 0, syn: true, reverse: true}, {seq: 1, data: "POST / HTTP/1.1\r\nHost: example.test\r\nContent-Length: 9223372036854775807\r\n\r\n"}, {seq: 1, reverse: true, data: "HTTP/1.1 413 Content Too Large\r\nContent-Length: 0\r\n\r\n"}}
	events, stats, err := binReplay(t, binTestPcap(t, steps, 80, false, false), 1)
	require.NoError(t, err)
	for _, e := range events {
		t.Log(e.DumpLine())
	}
	require.Len(t, events, 3)
	require.Equal(t, "limited", events[0].Status)
	require.Contains(t, events[0].Summary, "message_bytes=9223372036854775807")
	require.Equal(t, "decoded", events[1].Status)
	require.Equal(t, events[0].ID, events[1].ResponseTo)
	require.Zero(t, stats.BufferedBytes)
}

func TestProtocolBudgetHTTPOmittedPipeline(t *testing.T) {
	body := strings.Repeat("x", 4096)
	fixed := fmt.Sprintf("POST /large HTTP/1.1\r\nHost: example.test\r\nContent-Length: %d\r\n\r\n%s", len(body), body)
	chunked := fmt.Sprintf("POST /large HTTP/1.1\r\nHost: example.test\r\nTransfer-Encoding: chunked\r\n\r\n%x\r\n%s\r\n0\r\nX-End: yes\r\n\r\n", len(body), body)
	next := "GET /after HTTP/1.1\r\nHost: example.test\r\n\r\n"
	responses := "HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\nHTTP/1.1 204 No Content\r\n\r\n"
	for _, request := range []string{fixed, chunked} {
		for _, split := range []int{1, 37, 8192} {
			steps := []tcpStep{{seq: 0, syn: true}, {seq: 0, syn: true, reverse: true}}
			wire := request + next
			for at := 0; at < len(wire); at += split {
				steps = append(steps, tcpStep{seq: 1 + uint32(at), data: wire[at:min(at+split, len(wire))]})
			}
			steps = append(steps, tcpStep{seq: 1, reverse: true, data: responses})
			events, stats, err := binReplay(t, binTestPcap(t, steps, 80, false, false), 4, WithProtocolBudget(1024, 16384))
			require.NoError(t, err)
			require.Len(t, events, 4)
			require.Equal(t, "limited", events[0].Status)
			require.Equal(t, "headers", events[0].Completeness)
			require.Equal(t, "POST", events[0].Fields["Method"])
			fields, err := events[0].GetFields()
			require.NoError(t, err)
			require.Equal(t, "/large", fields["Request URI"])
			view, err := NewProtocolInspector()
			require.NoError(t, err)
			view.OnEvent(events[0])
			detail, err := view.Details(events[0].ID)
			require.NoError(t, err)
			require.Equal(t, "POST", detail.Fields["Method"])
			require.Equal(t, "decoded", events[1].Status)
			require.Equal(t, uint64(len(request)), events[1].Offset)
			require.Contains(t, events[1].Summary, "GET /after")
			require.Equal(t, events[0].ID, events[2].ResponseTo)
			require.Equal(t, events[1].ID, events[3].ResponseTo)
			require.Zero(t, stats.BufferedBytes)
			require.LessOrEqual(t, stats.PeakBufferedBytes, int64(16384))
			require.Zero(t, stats.Incomplete)
			require.Zero(t, stats.Malformed)
			require.Zero(t, stats.Unknown)
		}
	}
}

func TestProtocolBudgetHTTPOmittedCloseDelimited(t *testing.T) {
	request := "GET /large HTTP/1.1\r\nHost: example.test\r\n\r\n"
	response := "HTTP/1.0 200 OK\r\nContent-Type: text/plain\r\n\r\n" + strings.Repeat("x", 4096)
	steps := []tcpStep{{seq: 0, syn: true}, {seq: 0, syn: true, reverse: true}, {seq: 1, data: request}}
	for at := 0; at < len(response); at += 97 {
		steps = append(steps, tcpStep{seq: 1 + uint32(at), reverse: true, data: response[at:min(at+97, len(response))]})
	}
	steps = append(steps, tcpStep{seq: 1 + uint32(len(response)), reverse: true, fin: true}, tcpStep{seq: 1 + uint32(len(request)), fin: true})
	events, stats, err := binReplay(t, binTestPcap(t, steps, 80, false, false), 1, WithProtocolBudget(1024, 8192))
	require.NoError(t, err)
	require.Len(t, events, 2)
	require.Equal(t, "limited", events[1].Status)
	require.Equal(t, "headers", events[1].Completeness)
	require.Equal(t, 200, events[1].Fields["Status Code"])
	require.Equal(t, events[0].ID, events[1].ResponseTo)
	require.Zero(t, stats.Incomplete)
	require.Zero(t, stats.BufferedBytes)
}

func TestProtocolBudgetHTTPSharedBufferOmission(t *testing.T) {
	request := fmt.Sprintf("POST /large HTTP/1.1\r\nHost: example.test\r\nContent-Length: 7000\r\n\r\n%s", strings.Repeat("x", 7000))
	next := "GET /after HTTP/1.1\r\nHost: example.test\r\n\r\n"
	wire := request + next
	steps := []tcpStep{{seq: 0, syn: true}, {seq: 0, syn: true, reverse: true}}
	for at := 0; at < len(wire); at += 97 {
		steps = append(steps, tcpStep{seq: 1 + uint32(at), data: wire[at:min(at+97, len(wire))]})
	}
	steps = append(steps, tcpStep{seq: 1, reverse: true, data: "HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\nHTTP/1.1 204 No Content\r\n\r\n"})
	events, stats, err := binReplay(t, binTestPcap(t, steps, 80, false, false), 1, WithProtocolBudget(8192, 8192))
	require.NoError(t, err)
	require.Len(t, events, 4)
	require.Equal(t, "headers", events[0].Completeness)
	require.Contains(t, events[0].Summary, "capture buffer limit reached")
	require.Equal(t, "decoded", events[1].Status)
	require.Equal(t, uint64(len(request)), events[1].Offset)
	require.Equal(t, events[0].ID, events[2].ResponseTo)
	require.Equal(t, events[1].ID, events[3].ResponseTo)
	require.Equal(t, uint64(len(wire)+len(events[2].Raw)+len(events[3].Raw)), stats.InputBytes)
	require.Zero(t, stats.BufferedBytes)
	require.LessOrEqual(t, stats.PeakBufferedBytes, int64(8192))
}

func TestProtocolHTTPUnverifiedUpgradeKeepsHeaderBoundary(t *testing.T) {
	// This matches the nonstandard handshake observed in live traffic: both
	// Upgrade tokens are present, but version and accept proof are absent.
	request := "GET /socket HTTP/1.1\r\nHost: example.test\r\nConnection: Upgrade\r\nUpgrade: websocket\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n\r\n"
	response := "HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n"
	steps := []tcpStep{{seq: 0, syn: true}, {seq: 0, syn: true, reverse: true}, {seq: 1, data: request}, {seq: 1, data: response, reverse: true}, {seq: 1 + uint32(len(request)), data: strings.Repeat("x", 40000)}}
	events, stats, err := binReplay(t, binTestPcap(t, steps, 12010, false, false), 1)
	require.NoError(t, err)
	require.Len(t, events, 2, "opaque upgraded payload must not be retried as an HTTP header")
	require.Equal(t, "decoded", events[0].Status)
	require.Equal(t, "context-required", events[1].Status)
	require.Equal(t, "HTTPUpgradeUnverified", events[1].ExpertCode)
	require.Equal(t, "headers", events[1].Completeness)
	require.Equal(t, 101, events[1].Fields["Status Code"])
	require.Equal(t, response, string(events[1].Raw))
	require.Contains(t, events[1].Summary, "Sec-WebSocket-Version must be 13")
	require.Equal(t, events[0].ID, events[1].ResponseTo)
	require.Zero(t, stats.Malformed)
	require.Zero(t, stats.Incomplete)
	require.Zero(t, stats.BufferedBytes)
	require.GreaterOrEqual(t, stats.UnclassifiedBytes, uint64(40000))
}

func TestProtocolHTTPResponseWithoutRequestKeepsHeader(t *testing.T) {
	response := "HTTP/1.1 200 OK\r\nContent-Length: 0\r\nServer: " + strings.Repeat("x", 128) + "\r\n\r\n"
	steps := []tcpStep{{seq: 0, syn: true}, {seq: 1, data: response[:96]}, {seq: 97, data: response[96:]}}
	events, stats, err := binReplay(t, binTestPcap(t, steps, 80, false, false), 1)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, "context-required", events[0].Status)
	require.Equal(t, "headers", events[0].Completeness)
	require.Equal(t, 200, events[0].Fields["Status Code"])
	require.Equal(t, response, string(events[0].Raw))
	require.Len(t, events[0].SourceBytes.PacketRefs, 2, "every packet contributing to the retained header must remain attributable")
	require.Equal(t, uint64(2), events[0].SourceBytes.PacketRefs[0].Number)
	require.Equal(t, uint64(3), events[0].SourceBytes.PacketRefs[1].Number)
	require.Empty(t, events[0].Error)
	require.Zero(t, stats.BufferedBytes)
}

func TestProtocolHTTPFailureReleasesHeaderState(t *testing.T) {
	session, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	request := "GET /socket HTTP/1.1\r\nHost: example.test\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n"
	response := "HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n"
	requests := session.Feed(0, time.Unix(1, 0), []byte(request)).Events
	require.Len(t, requests, 1)
	responses := session.Feed(1, time.Unix(2, 0), []byte(response)).Events
	require.Len(t, responses, 1)
	flow := session.(*captureSession).f
	for _, direction := range flow.directions {
		require.True(t, direction.stopped)
		require.Nil(t, direction.http, "stopped directions must release header copies without waiting for TCP eviction")
	}
	require.Zero(t, flow.a.buffered.Load())
	fields, err := responses[0].GetFields()
	require.NoError(t, err)
	require.Equal(t, 101, fields["Status Code"], "emitted header evidence must survive flow-state release")
	require.Equal(t, response, string(responses[0].Raw))
	require.Equal(t, requests[0].ID, responses[0].ResponseTo)

	// A response seen without its request stops only the observed direction.
	// Its validated header must likewise move entirely to the emitted event.
	standalone, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	result := standalone.Feed(1, time.Unix(3, 0), []byte("HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n"))
	require.Len(t, result.Events, 1)
	require.Nil(t, standalone.(*captureSession).f.directions[1].http)
	require.Equal(t, 200, result.Events[0].Fields["Status Code"])
}

func TestProtocolAppleRTSPControlWithoutCSeq(t *testing.T) {
	request := "GET /info?txtAirPlay&txtRAOP RTSP/1.0\r\nX-Apple-QR: 1\r\n\r\n"
	response := "RTSP/1.0 200 OK\r\nServer: AirTunes/1.0\r\nContent-Type: application/x-apple-binary-plist\r\nContent-Length: 8\r\n\r\nbplist00"
	for _, deferred := range []bool{false, true} {
		steps := []tcpStep{{seq: 0, syn: true}, {seq: 0, syn: true, reverse: true}, {seq: 1, data: request}, {seq: 1, data: response[:17], reverse: true}, {seq: 18, data: response[17:], reverse: true}}
		events, stats, err := binReplay(t, binTestPcap(t, steps, 7000, false, false), 1, WithProtocolDeferred(deferred))
		require.NoError(t, err)
		require.Len(t, events, 2)
		for _, e := range events {
			require.Equal(t, "rtsp", e.Protocol)
			require.Equal(t, "rtsp-apple-control", e.Profile)
			require.Equal(t, "RTSPSequenceNotObserved", e.ExpertCode)
			require.Empty(t, e.Error)
			require.Equal(t, false, e.Session["CSeq Observed"])
			require.Equal(t, "unavailable-no-cseq", e.Session["Transaction Association"])
			require.Zero(t, e.ResponseTo, "a missing CSeq does not establish response association")
			_, err := e.GetFields()
			require.NoError(t, err)
		}
		require.Zero(t, stats.Malformed)
		require.Zero(t, stats.Incomplete)
		require.Zero(t, stats.BufferedBytes)
	}
}
