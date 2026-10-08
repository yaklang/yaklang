package pcaputil

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestProtocolSessionHTTP3ControlRequestResponse(t *testing.T) {
	s, err := NewDecryptedQUICSession(DefaultParserBudget(), "synthetic algorithm fixture; not wire evidence")
	require.NoError(t, err)
	dcid, scid := quicTestDCID(), quicTestSCID()
	ts := time.Unix(1, 0)
	require.Nil(t, s.Feed(0, ts, quicLongPacket(0, 1, dcid, nil, nil, 0, quicCryptoFrame(0, []byte("CHLO")))).Err)

	ctrl := h3ControlSETTINGS(0x01, 128, 0x07, 1)
	r := s.Feed(0, ts, quicLongPacket(1, 1, dcid, scid, nil, 0, quicStreamFrame(2, 0, false, ctrl)))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "http3", r.Events[0].Protocol)
	require.Equal(t, true, r.Events[0].Session["HTTP3"])
	require.Equal(t, "control", r.Events[0].Session["HTTP3 Stream Kind"])
	require.Equal(t, uint64(2), r.Events[0].Session["HTTP3 Stream ID"])
	require.Equal(t, "quic->http3", r.Events[0].Session["Protocol Transition"])
	h3f, _ := r.Events[0].Session["HTTP3 Frames"].([]map[string]any)
	require.Equal(t, "SETTINGS", h3f[0]["Frame Type"])
	settings, _ := h3f[0]["Settings"].([]map[string]any)
	require.Equal(t, "QPACK_MAX_TABLE_CAPACITY", settings[0]["Name"])
	require.Equal(t, uint64(128), settings[0]["Value"])
	require.Equal(t, "QPACK_BLOCKED_STREAMS", settings[1]["Name"])

	req := append(h3RequestHeaders(), h3Data([]byte("hello"))...)
	r = s.Feed(0, ts, quicLongPacket(1, 1, dcid, scid, nil, 1, quicStreamFrame(0, 0, true, req)))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "http3", r.Events[0].Protocol)
	require.Equal(t, "request", r.Events[0].Session["HTTP3 Stream Kind"])
	h3f, _ = r.Events[0].Session["HTTP3 Frames"].([]map[string]any)
	require.Equal(t, "HEADERS", h3f[0]["Frame Type"])
	require.Equal(t, "request", h3f[0]["Header Kind"])
	require.Equal(t, "DATA", h3f[1]["Frame Type"])
	require.Equal(t, []byte("hello"), h3f[1]["Payload"])
	require.Equal(t, "half-closed", r.Events[0].Session["HTTP3 Stream State"])

	r = s.Feed(1, ts, quicLongPacket(1, 1, dcid, scid, nil, 0, quicStreamFrame(0, 0, true, h3ResponseHeaders())))
	require.Nil(t, r.Err, "%v", r.Err)
	h3f, _ = r.Events[0].Session["HTTP3 Frames"].([]map[string]any)
	require.Equal(t, "HEADERS", h3f[0]["Frame Type"])
	require.Equal(t, "response", h3f[0]["Header Kind"])
	require.Equal(t, true, h3f[0]["Associated Request"])
	require.Equal(t, "closed", r.Events[0].Session["HTTP3 Stream State"])
}

func TestProtocolSessionHTTP3ResetAndFailClosed(t *testing.T) {
	s, err := NewDecryptedQUICSession(DefaultParserBudget(), "synthetic algorithm fixture; not wire evidence")
	require.NoError(t, err)
	dcid, scid := quicTestDCID(), quicTestSCID()
	ts := time.Unix(1, 0)
	require.Nil(t, s.Feed(0, ts, quicLongPacket(0, 1, dcid, nil, nil, 0, quicCryptoFrame(0, []byte("CHLO")))).Err)
	require.Nil(t, s.Feed(0, ts, quicLongPacket(1, 1, dcid, scid, nil, 0, quicStreamFrame(2, 0, false, h3ControlSETTINGS()))).Err)

	r := s.Feed(0, ts, quicLongPacket(1, 1, dcid, scid, nil, 1, quicStreamFrame(2, uint64(len(h3ControlSETTINGS())), false, h3Data([]byte("nope")))))
	require.NotNil(t, r.Err)
	require.Equal(t, ErrMalformedMessage, r.Err.Kind)

	s2, err := NewDecryptedQUICSession(DefaultParserBudget(), "synthetic algorithm fixture; not wire evidence")
	require.NoError(t, err)
	require.Nil(t, s2.Feed(0, ts, quicLongPacket(0, 1, dcid, nil, nil, 0, quicCryptoFrame(0, []byte("CHLO")))).Err)
	require.Nil(t, s2.Feed(0, ts, quicLongPacket(1, 1, dcid, scid, nil, 0, quicStreamFrame(0, 0, false, h3RequestHeaders()))).Err)
	r = s2.Feed(0, ts, quicLongPacket(1, 1, dcid, scid, nil, 1, quicStreamFrame(0, uint64(len(h3RequestHeaders())), false, h3Frame(0x04, nil))))
	require.NotNil(t, r.Err)
	require.Equal(t, ErrMalformedMessage, r.Err.Kind)

	s3, err := NewDecryptedQUICSession(DefaultParserBudget(), "synthetic algorithm fixture; not wire evidence")
	require.NoError(t, err)
	require.Nil(t, s3.Feed(0, ts, quicLongPacket(0, 1, dcid, nil, nil, 0, quicCryptoFrame(0, []byte("CHLO")))).Err)
	require.Nil(t, s3.Feed(0, ts, quicLongPacket(1, 1, dcid, scid, nil, 0, quicStreamFrame(0, 0, false, h3RequestHeaders()))).Err)
	r = s3.Feed(1, ts, quicLongPacket(1, 1, dcid, scid, nil, 0, quicResetStream(0, 0x010c, 0)))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "http3", r.Events[0].Protocol)
	require.Equal(t, "reset", r.Events[0].Session["HTTP3 Stream State"])
	require.Equal(t, true, r.Events[0].Session["HTTP3"])
}

func TestProtocolSessionHTTP3Fragmentation(t *testing.T) {
	dcid, scid := quicTestDCID(), quicTestSCID()
	req := append(h3RequestHeaders(), h3Data([]byte("hi"))...)
	steps := []sessionStep{
		{0, quicLongPacket(0, 1, dcid, nil, nil, 0, quicCryptoFrame(0, []byte("CHLO")))},
		{0, quicLongPacket(1, 1, dcid, scid, nil, 0, quicStreamFrame(2, 0, false, h3ControlSETTINGS(0x01, 0)))},
		{0, quicLongPacket(1, 1, dcid, scid, nil, 1, quicStreamFrame(0, 0, true, req))},
		{1, quicLongPacket(1, 1, dcid, scid, nil, 0, quicStreamFrame(0, 0, true, h3ResponseHeaders()))},
	}
	assertFragmentation(t, steps, func(chunk int) []string {
		s, err := NewDecryptedQUICSession(DefaultParserBudget(), "synthetic algorithm fixture; not wire evidence")
		require.NoError(t, err)
		ts := time.Unix(1, 0)
		var names []string
		for _, st := range steps {
			for w := st.wire; len(w) > 0; {
				n := len(w)
				if chunk > 0 {
					n = min(n, chunk)
				}
				r := s.Feed(st.dir, ts, w[:n])
				for _, e := range r.Events {
					if e.Protocol == "http3" && e.Session["HTTP3 Frames"] != nil {
						for _, hf := range e.Session["HTTP3 Frames"].([]map[string]any) {
							names = append(names, fmt.Sprintf("%v:%v:%v", e.Session["HTTP3 Stream ID"], hf["Frame Type"], hf["Header Kind"]))
						}
					}
				}
				w = w[n:]
			}
		}
		return names
	})
}
