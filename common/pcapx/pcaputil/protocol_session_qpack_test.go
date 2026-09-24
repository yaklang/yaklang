package pcaputil

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func h3EncoderStream(inst []byte) []byte { return append([]byte{0x02}, inst...) }
func h3DecoderStream(inst []byte) []byte { return append([]byte{0x03}, inst...) }

func TestProtocolSessionQPACKStaticAndDynamic(t *testing.T) {
	s, err := NewDecryptedQUICSession(DefaultParserBudget(), "synthetic algorithm fixture; not wire evidence")
	require.NoError(t, err)
	dcid, scid := quicTestDCID(), quicTestSCID()
	ts := time.Unix(1, 0)
	require.Nil(t, s.Feed(0, ts, quicLongPacket(0, 1, dcid, nil, nil, 0, quicCryptoFrame(0, []byte("CHLO")))).Err)

	r := s.Feed(0, ts, quicLongPacket(1, 1, dcid, scid, nil, 0, quicStreamFrame(0, 0, false, h3RequestHeaders())))
	require.Nil(t, r.Err, "%v", r.Err)
	headers, _ := r.Events[0].Session["Headers"].([]map[string]any)
	require.Equal(t, ":method", headers[0]["Name"])
	require.Equal(t, "GET", headers[0]["Value"])
	require.Equal(t, ":scheme", headers[1]["Name"])
	require.Equal(t, "https", headers[1]["Value"])
	require.Equal(t, ":path", headers[2]["Name"])
	require.Equal(t, "/", headers[2]["Value"])
	require.Equal(t, ":authority", headers[3]["Name"])
	require.Equal(t, "example.com", headers[3]["Value"])

	s2, err := NewDecryptedQUICSession(DefaultParserBudget(), "synthetic algorithm fixture; not wire evidence")
	require.NoError(t, err)
	require.Nil(t, s2.Feed(0, ts, quicLongPacket(0, 1, dcid, nil, nil, 0, quicCryptoFrame(0, []byte("CHLO")))).Err)
	require.Nil(t, s2.Feed(1, ts, quicLongPacket(1, 1, dcid, scid, nil, 0, quicStreamFrame(3, 0, false, h3ControlSETTINGS(0x01, 220)))).Err)

	b2 := rfcHex("3fbd01c00f7777772e6578616d706c652e636f6dc10c2f73616d706c652f70617468")
	r = s2.Feed(0, ts, quicLongPacket(1, 1, dcid, scid, nil, 0, quicStreamFrame(6, 0, false, h3EncoderStream(b2))))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "encoder", r.Events[0].Session["HTTP3 Stream Kind"])
	require.Equal(t, uint64(2), r.Events[0].Session["QPACK Insert Count"])
	require.Equal(t, uint64(220), r.Events[0].Session["QPACK Capacity"])

	r = s2.Feed(0, ts, quicLongPacket(1, 1, dcid, scid, nil, 1, quicStreamFrame(0, 0, true, h3Frame(0x01, rfcHex("03811011")))))
	require.Nil(t, r.Err, "%v", r.Err)
	headers, _ = r.Events[0].Session["Headers"].([]map[string]any)
	require.Equal(t, ":authority", headers[0]["Name"])
	require.Equal(t, "www.example.com", headers[0]["Value"])
	require.Equal(t, ":path", headers[1]["Name"])
	require.Equal(t, "/sample/path", headers[1]["Value"])
	h3f, _ := r.Events[0].Session["HTTP3 Frames"].([]map[string]any)
	require.Equal(t, uint64(2), h3f[0]["Required Insert Count"])
}

func TestProtocolSessionQPACKBlockedDecoderAndBudget(t *testing.T) {
	s, err := NewDecryptedQUICSession(DefaultParserBudget(), "synthetic algorithm fixture; not wire evidence")
	require.NoError(t, err)
	dcid, scid := quicTestDCID(), quicTestSCID()
	ts := time.Unix(1, 0)
	require.Nil(t, s.Feed(0, ts, quicLongPacket(0, 1, dcid, nil, nil, 0, quicCryptoFrame(0, []byte("CHLO")))).Err)
	require.Nil(t, s.Feed(1, ts, quicLongPacket(1, 1, dcid, scid, nil, 0, quicStreamFrame(3, 0, false, h3ControlSETTINGS(0x01, 220)))).Err)

	r := s.Feed(0, ts, quicLongPacket(1, 1, dcid, scid, nil, 0, quicStreamFrame(0, 0, false, h3Frame(0x01, rfcHex("03811011")))))
	require.NotNil(t, r.Err)
	require.Equal(t, ErrContextRequired, r.Err.Kind)
	require.Contains(t, r.Err.Message, "blocked")

	s2, err := NewDecryptedQUICSession(DefaultParserBudget(), "synthetic algorithm fixture; not wire evidence")
	require.NoError(t, err)
	require.Nil(t, s2.Feed(0, ts, quicLongPacket(0, 1, dcid, nil, nil, 0, quicCryptoFrame(0, []byte("CHLO")))).Err)
	require.Nil(t, s2.Feed(1, ts, quicLongPacket(1, 1, dcid, scid, nil, 0, quicStreamFrame(3, 0, false, h3ControlSETTINGS(0x01, 100)))).Err)
	r = s2.Feed(0, ts, quicLongPacket(1, 1, dcid, scid, nil, 0, quicStreamFrame(6, 0, false, h3EncoderStream(rfcHex("3fbd01")))))
	require.NotNil(t, r.Err)
	require.Equal(t, ErrResourceExceeded, r.Err.Kind)

	s3, err := NewDecryptedQUICSession(DefaultParserBudget(), "synthetic algorithm fixture; not wire evidence")
	require.NoError(t, err)
	require.Nil(t, s3.Feed(0, ts, quicLongPacket(0, 1, dcid, nil, nil, 0, quicCryptoFrame(0, []byte("CHLO")))).Err)
	r = s3.Feed(1, ts, quicLongPacket(1, 1, dcid, scid, nil, 0, quicStreamFrame(7, 0, false, h3DecoderStream([]byte{0x84, 0x01, 0x48}))))
	require.Nil(t, r.Err, "%v", r.Err)
	inst, _ := r.Events[0].Session["QPACK Instructions"].([]map[string]any)
	require.Equal(t, "Section Acknowledgment", inst[0]["Instruction"])
	require.Equal(t, uint64(4), inst[0]["Value"])
	require.Equal(t, "Insert Count Increment", inst[1]["Instruction"])
	require.Equal(t, "Stream Cancellation", inst[2]["Instruction"])
	require.Equal(t, uint64(8), inst[2]["Value"])
}

func TestProtocolSessionQPACKFragmentation(t *testing.T) {
	dcid, scid := quicTestDCID(), quicTestSCID()
	b2 := rfcHex("3fbd01c00f7777772e6578616d706c652e636f6dc10c2f73616d706c652f70617468")
	steps := []sessionStep{
		{0, quicLongPacket(0, 1, dcid, nil, nil, 0, quicCryptoFrame(0, []byte("CHLO")))},
		{1, quicLongPacket(1, 1, dcid, scid, nil, 0, quicStreamFrame(3, 0, false, h3ControlSETTINGS(0x01, 220)))},
		{0, quicLongPacket(1, 1, dcid, scid, nil, 0, quicStreamFrame(6, 0, false, h3EncoderStream(b2)))},
		{0, quicLongPacket(1, 1, dcid, scid, nil, 1, quicStreamFrame(0, 0, true, h3Frame(0x01, rfcHex("03811011"))))},
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
					if hs, ok := e.Session["Headers"].([]map[string]any); ok {
						for _, h := range hs {
							names = append(names, fmt.Sprintf("%v=%v", h["Name"], h["Value"]))
						}
					}
					if inst, ok := e.Session["QPACK Instructions"].([]map[string]any); ok {
						for _, it := range inst {
							names = append(names, fmt.Sprint(it["Instruction"]))
						}
					}
				}
				w = w[n:]
			}
		}
		return names
	})
}
