package pcaputil

import (
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func feedT20RTSPSetup(t *testing.T, s *captureSession, seq int, sid string, requestTransport, responseTransport []string, ts time.Time) *ProtocolEvent {
	t.Helper()
	var request, response string
	request = fmt.Sprintf("SETUP rtsp://camera.example/live RTSP/1.0\r\nCSeq: %d\r\n", seq)
	for _, value := range requestTransport {
		request += "Transport: " + value + "\r\n"
	}
	request += "\r\n"
	requestResult := s.Feed(0, ts, []byte(request))
	if requestResult.Err != nil {
		if len(requestResult.Events) > 0 {
			t.Fatalf("request error=%T %v status=%s event-error=%q session=%#v", requestResult.Err, requestResult.Err, requestResult.Events[0].Status, requestResult.Events[0].Error, requestResult.Events[0].Session)
		}
		t.Fatalf("request error=%T %v", requestResult.Err, requestResult.Err)
	}
	require.Len(t, requestResult.Events, 1)
	require.Equal(t, "SETUP", requestResult.Events[0].Session["Packet Name"])

	response = fmt.Sprintf("RTSP/1.0 200 OK\r\nCSeq: %d\r\nSession: %s\r\n", seq, sid)
	for _, value := range responseTransport {
		response += "Transport: " + value + "\r\n"
	}
	response += "\r\n"
	responseResult := s.Feed(1, ts.Add(time.Millisecond), []byte(response))
	if responseResult.Err != nil {
		if len(responseResult.Events) > 0 {
			t.Fatalf("response error=%T %v status=%s event-error=%q session=%#v", responseResult.Err, responseResult.Err, responseResult.Events[0].Status, responseResult.Events[0].Error, responseResult.Events[0].Session)
		}
		t.Fatalf("response error=%T %v", responseResult.Err, responseResult.Err)
	}
	require.Len(t, responseResult.Events, 1)
	return responseResult.Events[0]
}

func rtspUDPTestRTP(pt byte, seq uint16, ssrc uint32) []byte {
	w := make([]byte, 13)
	w[0], w[1] = 0x80, pt
	binary.BigEndian.PutUint16(w[2:4], seq)
	binary.BigEndian.PutUint32(w[4:8], uint32(seq)*160)
	binary.BigEndian.PutUint32(w[8:12], ssrc)
	w[12] = 0x55
	return w
}

func rtspUDPTestRTCP() []byte {
	w := make([]byte, 28)
	w[0], w[1] = 0x80, 200 // Version 2, Sender Report.
	binary.BigEndian.PutUint16(w[2:4], 6)
	binary.BigEndian.PutUint32(w[4:8], 0x12345678)
	return w
}

func decodeT20RTSPMedia(a *binParser, source, destination string, wire []byte, domain CaptureDomain, ts time.Time) (*ProtocolEvent, bool) {
	e := &ProtocolEvent{Domain: domain, Transport: "udp", Source: source, Destination: destination, Timestamp: ts}
	_, sourcePort, err := net.SplitHostPort(source)
	if err != nil {
		return e, false
	}
	_, destinationPort, err := net.SplitHostPort(destination)
	if err != nil {
		return e, false
	}
	srcPort, err := strconv.ParseUint(sourcePort, 10, 16)
	if err != nil {
		return e, false
	}
	dstPort, err := strconv.ParseUint(destinationPort, 10, 16)
	if err != nil {
		return e, false
	}
	handled := a.decodeNativeDatagram(e, wire, uint16(srcPort), uint16(dstPort))
	return e, handled
}

func testT20RTSPUDPTransport(t *testing.T) {
	ts := time.Unix(1_790_000_000, 0)
	domain := CaptureDomain{}
	requestTransport := []string{"RTP/AVP/UDP;unicast;client_port=5000-5001"}
	responseTransport := []string{"RTP/AVP/UDP;unicast;client_port=5000-5001;server_port=6000-6001"}

	t.Run("bidirectional-client-server-port-map-and-rtcp", func(t *testing.T) {
		session, err := NewProtocolSession(DefaultParserBudget())
		require.NoError(t, err)
		capture := session.(*captureSession)
		setupEvent := feedT20RTSPSetup(t, capture, 1, "udp-session", requestTransport, responseTransport, ts)
		require.Equal(t, "mapped", setupEvent.Session["UDP Media Transport Status"])
		require.Equal(t, "mapped", setupEvent.Session["UDP Media Association Status"])
		require.Equal(t, "RTP/AVP/UDP", setupEvent.Session["UDP Media Transport"].(map[string]any)["Profile"])
		require.Len(t, capture.f.a.rtspMedia.entries, 1)

		packets := []struct {
			src, dst string
			wire     []byte
			name     string
			dir      string
		}{
			{"192.0.2.1:5000", "192.0.2.2:6000", rtspUDPTestRTP(96, 1, 0x12345678), "RTP", "client-to-server"},
			{"192.0.2.2:6000", "192.0.2.1:5000", rtspUDPTestRTP(96, 2, 0x12345678), "RTP", "server-to-client"},
			{"192.0.2.1:5001", "192.0.2.2:6001", rtspUDPTestRTCP(), "SR", "client-to-server"},
			{"192.0.2.2:6001", "192.0.2.1:5001", rtspUDPTestRTCP(), "SR", "server-to-client"},
		}
		for _, packet := range packets {
			event, handled := decodeT20RTSPMedia(capture.f.a, packet.src, packet.dst, packet.wire, domain, ts.Add(2*time.Millisecond))
			require.True(t, handled, "%s -> %s", packet.src, packet.dst)
			require.Equal(t, "rtp", event.Protocol)
			require.Equal(t, "captured-rtsp-setup-port-mapping", event.Admission)
			require.Equal(t, "decoded", event.Status)
			require.Equal(t, packet.name, event.Session["Packet Name"])
			require.Equal(t, "udp-session", event.Session["Associated RTSP Session ID"])
			require.Equal(t, packet.dir, event.Session["RTSP Media Direction"])
			require.Equal(t, "matched-rtsp-transport", event.Session["Association Status"])
			require.Empty(t, event.Session["Associated Call-ID"], "RTSP port mapping must not invent SIP identity")
			if packet.name == "RTP" {
				require.Equal(t, "clock-rate-unknown", event.Session["Timing Status"], "RTSP Transport does not negotiate payload clock rates")
				require.NotContains(t, event.Session, "Clock Rate")
			}
		}

		wrongPort, handled := decodeT20RTSPMedia(capture.f.a, "192.0.2.1:5002", "192.0.2.2:6002", rtspUDPTestRTP(96, 3, 0x12345678), domain, ts.Add(3*time.Millisecond))
		require.False(t, handled, "an unnegotiated UDP tuple must not inherit the RTSP mapping")
		require.Empty(t, wrongPort.Protocol)
		wrongDomain, handled := decodeT20RTSPMedia(capture.f.a, "192.0.2.1:5000", "192.0.2.2:6000", rtspUDPTestRTP(96, 4, 0x12345678), CaptureDomain{Interface: 5}, ts.Add(3*time.Millisecond))
		require.False(t, handled, "the association must not cross capture interfaces")
		require.Empty(t, wrongDomain.Protocol)
	})

	t.Run("fail-closed-ambiguous-or-incomplete-transport", func(t *testing.T) {
		cases := []struct {
			name     string
			request  []string
			response []string
		}{
			{"multiple-alternatives", requestTransport, []string{"RTP/AVP/UDP;unicast;client_port=5000-5001;server_port=6000-6001, RTP/AVP/TCP;unicast;interleaved=0-1"}},
			{"duplicate-transport-fields", requestTransport, []string{"RTP/AVP/UDP;unicast;client_port=5000-5001;server_port=6000-6001", "RTP/AVP/UDP;unicast;client_port=5000-5001;server_port=6000-6001"}},
			{"duplicate-port-parameter", requestTransport, []string{"RTP/AVP/UDP;unicast;client_port=5000-5001;server_port=6000-6001;server_port=7000-7001"}},
			{"missing-server-port", requestTransport, []string{"RTP/AVP/UDP;unicast;client_port=5000-5001"}},
			{"invalid-same-port-pair", requestTransport, []string{"RTP/AVP/UDP;unicast;client_port=5000-5000;server_port=6000-6001"}},
			{"client-port-does-not-match-offer", requestTransport, []string{"RTP/AVP/UDP;unicast;client_port=7000-7001;server_port=6000-6001"}},
			{"missing-unicast", requestTransport, []string{"RTP/AVP/UDP;client_port=5000-5001;server_port=6000-6001"}},
			{"unsupported-mux", requestTransport, []string{"RTP/AVP/UDP;unicast;rtcp-mux;client_port=5000-5001;server_port=6000-6001"}},
			{"tcp-not-udp", []string{"RTP/AVP/TCP;unicast;interleaved=0-1"}, []string{"RTP/AVP/TCP;unicast;interleaved=0-1"}},
			{"request-port-pair-missing", []string{"RTP/AVP/UDP;unicast"}, responseTransport},
			{"duplicate-request-transport-fields", []string{"RTP/AVP/UDP;unicast;client_port=5000-5001", "RTP/AVP/UDP;unicast;client_port=5000-5001"}, responseTransport},
		}
		for i, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				session, err := NewProtocolSession(DefaultParserBudget())
				require.NoError(t, err)
				capture := session.(*captureSession)
				setup := feedT20RTSPSetup(t, capture, i+1, "bad-session", tc.request, tc.response, ts)
				require.NotEqual(t, "mapped", setup.Session["UDP Media Transport Status"])
				require.Empty(t, capture.f.a.rtspMedia.entries)
				event, handled := decodeT20RTSPMedia(capture.f.a, "192.0.2.1:5000", "192.0.2.2:6000", rtspUDPTestRTP(96, 1, 0xabcdef01), domain, ts.Add(time.Millisecond))
				require.False(t, handled, "invalid transport evidence must not admit RTP")
				require.Empty(t, event.Protocol)
			})
		}
	})

	t.Run("observed-media-address-override-and-session-teardown", func(t *testing.T) {
		session, err := NewProtocolSession(DefaultParserBudget())
		require.NoError(t, err)
		capture := session.(*captureSession)
		response := []string{"RTP/AVP/UDP;unicast;client_port=5000-5001;server_port=6000-6001;source=198.51.100.20;destination=198.51.100.10"}
		feedT20RTSPSetup(t, capture, 1, "teardown-session", requestTransport, response, ts)
		event, handled := decodeT20RTSPMedia(capture.f.a, "198.51.100.10:5000", "198.51.100.20:6000", rtspUDPTestRTP(96, 1, 0x01020304), domain, ts.Add(2*time.Millisecond))
		require.True(t, handled)
		require.Equal(t, "teardown-session", event.Session["Associated RTSP Session ID"])

		request := "TEARDOWN rtsp://camera.example/live RTSP/1.0\r\nCSeq: 2\r\nSession: teardown-session\r\n\r\n"
		teardownRequest := capture.Feed(0, ts.Add(3*time.Millisecond), []byte(request))
		if teardownRequest.Err != nil {
			if len(teardownRequest.Events) > 0 {
				t.Fatalf("teardown request error=%v status=%s event-error=%q session=%#v", teardownRequest.Err, teardownRequest.Events[0].Status, teardownRequest.Events[0].Error, teardownRequest.Events[0].Session)
			}
			t.Fatalf("teardown request error=%v", teardownRequest.Err)
		}
		responseMessage := "RTSP/1.0 200 OK\r\nCSeq: 2\r\nSession: teardown-session\r\n\r\n"
		teardown := capture.Feed(1, ts.Add(4*time.Millisecond), []byte(responseMessage))
		if teardown.Err != nil {
			if len(teardown.Events) > 0 {
				t.Fatalf("teardown response error=%T %#v event=%#v", teardown.Err, teardown.Err, teardown.Events[0])
			}
			t.Fatalf("teardown response error=%T %#v", teardown.Err, teardown.Err)
		}
		require.Len(t, teardown.Events, 1)
		require.Equal(t, true, teardown.Events[0].Session["RTSP Session Teardown"])
		require.Empty(t, capture.f.a.rtspMedia.entries)
		event, handled = decodeT20RTSPMedia(capture.f.a, "198.51.100.10:5000", "198.51.100.20:6000", rtspUDPTestRTP(96, 2, 0x01020304), domain, ts.Add(5*time.Millisecond))
		require.False(t, handled, "TEARDOWN removes the media tuple mapping")
		require.Empty(t, event.Protocol)
	})

	t.Run("colliding-port-mappings-remain-ambiguous", func(t *testing.T) {
		session, err := NewProtocolSession(DefaultParserBudget())
		require.NoError(t, err)
		capture := session.(*captureSession)
		feedT20RTSPSetup(t, capture, 1, "first-udp-session", requestTransport, responseTransport, ts)
		feedT20RTSPSetup(t, capture, 2, "second-udp-session", requestTransport, responseTransport, ts.Add(2*time.Millisecond))
		require.Len(t, capture.f.a.rtspMedia.entries, 2)

		event, handled := decodeT20RTSPMedia(capture.f.a, "192.0.2.1:5000", "192.0.2.2:6000", rtspUDPTestRTP(96, 1, 0xabcdef01), domain, ts.Add(4*time.Millisecond))
		require.True(t, handled, "the known tuple should be retained as ambiguous protocol evidence")
		require.Equal(t, "rtp", event.Protocol)
		require.Equal(t, "context-required", event.Status)
		require.Equal(t, "ambiguous-captured-rtsp-transport", event.Admission)
		require.Equal(t, "ambiguous-rtsp-transport", event.Session["Association Status"])
		require.Empty(t, event.Session["Associated RTSP Session ID"], "a colliding tuple must not pick either RTSP session")
	})

	t.Run("association-expiry-budget-and-cleanup", func(t *testing.T) {
		session, err := NewProtocolSession(DefaultParserBudget())
		require.NoError(t, err)
		capture := session.(*captureSession)
		feedT20RTSPSetup(t, capture, 1, "expiry-session", requestTransport, responseTransport, ts)
		parser := capture.f.a
		require.Len(t, parser.rtspMedia.entries, 1)
		cost := parser.rtspMedia.entries[0].cost
		before := parser.buffered.Load()
		match := parser.matchRTSPMedia(domain, "192.0.2.1:5000", "192.0.2.2:6000", ts.Add(sipMediaAssociationTTL+2*time.Millisecond))
		require.Nil(t, match.association, "expired SETUP state must not admit RTP")
		require.Empty(t, parser.rtspMedia.entries)
		require.Equal(t, before-cost, parser.buffered.Load(), "expired mapping releases its shared memory charge")

		limited := &binParser{budget: ParserBudget{MaxCollectionElements: 1}, config: BinParserConfig{MaxBufferedBytes: 100}}
		oversized := &rtspMediaAssociation{session: "too-large", clientRTP: "192.0.2.1:5000", clientRTCP: "192.0.2.1:5001", serverRTP: "192.0.2.2:6000", serverRTCP: "192.0.2.2:6001", expires: ts.Add(time.Hour), cost: 101}
		require.False(t, limited.addRTSPMediaAssociation(oversized, ts), "association memory must obey the capture budget")
		require.Zero(t, limited.buffered.Load())

		limited.config.MaxBufferedBytes = 2048
		first := *oversized
		first.cost, first.session = 64, "one"
		require.True(t, limited.addRTSPMediaAssociation(&first, ts))
		second := first
		second.session, second.clientRTP = "two", "192.0.2.1:7000"
		require.False(t, limited.addRTSPMediaAssociation(&second, ts), "association count must obey MaxCollectionElements")
		require.Len(t, limited.rtspMedia.entries, 1)
		limited.closeRTSPMedia()
		require.Zero(t, limited.buffered.Load(), "parser teardown releases RTSP association budget")
	})
}
