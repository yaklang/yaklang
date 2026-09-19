package pcaputil

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRTSPInterleavingAndHTTPIsolation(t *testing.T) {
	steps := []sessionStep{{0, []byte("OPTIONS * RTSP/1.0\r\nCSeq: 1\r\n\r\n")}, {1, []byte("RTSP/1.0 200 OK\r\nCSeq: 1\r\n\r\n")}, {0, []byte("SETUP rtsp://example.test/a RTSP/1.0\r\nCSeq: 2\r\nTransport: RTP/AVP/TCP;interleaved=0-1\r\n\r\n")}, {1, []byte("RTSP/1.0 200 OK\r\nCSeq: 2\r\nSession: abc\r\nTransport: RTP/AVP/TCP;interleaved=0-1\r\n\r\n")}}
	rtp := rtpPkt(0, 1, 0, 123)
	packet := []byte{'$', 0, byte(len(rtp) >> 8), byte(len(rtp))}
	packet = append(packet, rtp...)
	steps = append(steps, sessionStep{1, packet})
	for _, chunk := range []int{0, 1, 7} {
		for _, deferred := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d/%v", chunk, deferred), func(t *testing.T) {
				events, _ := sessionTestFlow(t, "rtsp", steps, chunk, deferred)
				require.Len(t, events, 5)
				assertSessionEvents(t, events, "rtsp", deferred)
				require.Equal(t, true, events[4].Session["Matched"])
				require.Equal(t, "abc", events[4].Session["Session ID"])
			})
		}
	}
	_, _, _, _, err := rtspHeader([]byte("PLAY x RTSP/1.0\r\nCSeq: 1\r\nContent-Length: 1\r\nContent-Length: 2\r\n\r\n"), 8)
	require.Error(t, err)
}
func ippTestWire(op byte) []byte {
	return []byte{1, 1, 0, op, 0, 0, 0, 1, 1, 0x47, 0, 18, 'a', 't', 't', 'r', 'i', 'b', 'u', 't', 'e', 's', '-', 'c', 'h', 'a', 'r', 's', 'e', 't', 0, 5, 'u', 't', 'f', '-', '8', 3}
}
func TestIPPMixedHTTPAndMalformed(t *testing.T) {
	req := ippTestWire(0xb)
	resp := ippTestWire(0)
	steps := []sessionStep{{0, append([]byte(fmt.Sprintf("POST /ipp HTTP/1.1\r\nHost: example.test\r\nContent-Type: application/ipp\r\nContent-Length: %d\r\n\r\n", len(req))), req...)}, {1, []byte("HTTP/1.1 103 Early Hints\r\n\r\n")}, {1, append([]byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: application/ipp\r\nContent-Length: %d\r\n\r\n", len(resp))), resp...)}, {0, []byte("GET / HTTP/1.1\r\nHost: example.test\r\n\r\n")}, {1, []byte("HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n")}}
	for _, chunk := range []int{0, 1, 7} {
		for _, deferred := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d/%v", chunk, deferred), func(t *testing.T) {
				events, _ := sessionTestFlow(t, "ipp", steps, chunk, deferred)
				require.Len(t, events, 5)
				require.Equal(t, "ipp", events[0].Protocol)
				require.Equal(t, "ipp", events[2].Protocol)
				require.Equal(t, true, events[2].Session["Matched"])
				require.Equal(t, "http", events[4].Protocol)
				for _, e := range events {
					require.Empty(t, e.Error)
					_, err := e.Decode()
					require.NoError(t, err)
				}
			})
		}
	}
	for _, bad := range [][]byte{req[:8], req[:len(req)-1], {1, 1, 0, 1, 0, 0, 0, 1, 1, 0x21, 0, 1, 'a', 0, 1, 1, 3}} {
		_, err := ippFields(bad, 4, 4)
		require.Error(t, err)
	}
}
func TestRTSPIPPUpstreamCaptureReplay(t *testing.T) {
	for _, tc := range []struct{ protocol, path string }{{"rtsp", "testdata/protocol-sessions/upstream/rtsp.pcap"}, {"ipp", "../../bin-parser/testdata/protocol-corpus/captures/ndpi/ndpi-ipp.pcap"}} {
		for _, workers := range []int{1, 2} {
			t.Run(fmt.Sprintf("%s/%d", tc.protocol, workers), func(t *testing.T) {
				raw, err := os.ReadFile(tc.path)
				require.NoError(t, err)
				events, stats, err := binReplay(t, raw, workers)
				require.NoError(t, err)
				counts := map[string]int{}
				for _, e := range events {
					if e.Protocol == tc.protocol {
						counts[e.Status]++
						if e.Error != "" {
							t.Logf("%s %s", e.Entry, e.Error)
						}
					}
				}
				require.Positive(t, counts["decoded"])
				require.Zero(t, stats.BufferedBytes)
				t.Logf("counts %v", counts)
			})
		}
	}
}
