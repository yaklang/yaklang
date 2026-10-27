package pcaputil

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRTSPFlushFieldsFragmentationAndCSeqReuse(t *testing.T) {
	request := "FLUSH rtsp://example.test/audio RTSP/1.0\r\nCSeq: 7\r\nSession: audio\r\nRTP-Info: seq=65535;rtptime=4294967295\r\nContent-Length: 3\r\n\r\nabc"
	response := "RTSP/1.0 200 OK\r\nCSeq: 7\r\nSession: audio\r\n\r\n"
	steps := []sessionStep{{0, []byte(request)}, {1, []byte(response)}, {0, []byte(request)}, {1, []byte(response)}}
	for _, deferred := range []bool{false, true} {
		for _, chunk := range []int{0, 1, 7} {
			t.Run(fmt.Sprintf("%v/%d", deferred, chunk), func(t *testing.T) {
				events, _ := sessionTestFlow(t, "rtsp", steps, chunk, deferred)
				require.Len(t, events, 4)
				assertSessionEvents(t, events, "rtsp", deferred)
				for _, e := range []*ProtocolEvent{events[0], events[2]} {
					require.Equal(t, "rtsp-extension-envelope", e.Profile)
					require.Equal(t, "FLUSH", e.Session["Extension Method"])
					require.Equal(t, false, e.Session["Media Effect Verified"])
					info := e.Session["RTP Info"].([]any)[0].(map[string]any)
					require.Equal(t, uint64(65535), info["seq"])
					require.Equal(t, uint64(4294967295), info["rtptime"])
					fields, err := e.GetFields()
					require.NoError(t, err)
					require.Equal(t, e.Session, fields)
				}
				for _, e := range []*ProtocolEvent{events[1], events[3]} {
					require.Equal(t, true, e.Session["Matched"])
					require.Equal(t, "FLUSH", e.Session["In Reply To"])
					require.Empty(t, e.Session["Phase"])
					require.NotContains(t, e.Session, "UDP Media Transport")
				}
			})
		}
	}
}

func TestRTSPFlushBoundsAndNoInventedMediaReset(t *testing.T) {
	s := &binRTSP{}
	_, err := s.consume(0, []byte("PLAY rtsp://example.test/a RTSP/1.0\r\nCSeq: 1\r\nSession: audio\r\n\r\n"), time.Time{}, 64)
	require.NoError(t, err)
	_, err = s.consume(1, []byte("RTSP/1.0 200 OK\r\nCSeq: 1\r\nSession: audio\r\n\r\n"), time.Time{}, 64)
	require.NoError(t, err)
	s.sessions["audio"] = "playing"
	_, err = s.consume(0, []byte("FLUSH rtsp://example.test/a RTSP/1.0\r\nCSeq: 2\r\nSession: audio\r\nRTP-Info: seq=10;rtptime=20\r\n\r\n"), time.Time{}, 64)
	require.NoError(t, err)
	_, err = s.consume(1, []byte("RTSP/1.0 200 OK\r\nCSeq: 2\r\nSession: audio\r\n\r\n"), time.Time{}, 64)
	require.NoError(t, err)
	require.Equal(t, "playing", s.sessions["audio"])
	for _, value := range []string{"seq=65536", "rtptime=4294967296", "seq=-1", "seq=+1", "seq=1;seq=2", "seq=", "garbage", ""} {
		state := &binRTSP{}
		_, err := state.consume(0, []byte("FLUSH rtsp://example.test/a RTSP/1.0\r\nCSeq: 7\r\nRTP-Info: "+value+"\r\n\r\n"), time.Time{}, 64)
		require.Error(t, err, value)
		require.Empty(t, state.pending)
	}
	for _, prefix := range []string{"FLUSH rtsp://example.test/a", "OPTIONS rtsp://example.test/a", "RTSP/1.0", "RTSP/1.0 200 OK\r\nCSeq: 1\r\n"} {
		require.NotEqual(t, ProbeAccept, probeRTSP([]byte(prefix), 32768).Verdict, prefix)
	}
	for _, invalid := range []string{"FLUSH x RTSP/1.0\r\n\r\n", "FLUSH x RTSP/1.0\r\nCSeq: 1\r\nCSeq: 2\r\n\r\n", "OPTIONS sip:x SIP/2.0\r\nCSeq: 1\r\n\r\n"} {
		require.Equal(t, ProbeReject, probeRTSP([]byte(invalid), 32768).Verdict, invalid)
	}
}
