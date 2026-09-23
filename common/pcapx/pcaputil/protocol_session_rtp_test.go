package pcaputil

import (
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func rtpPkt(pt uint8, seq uint16, ts, ssrc uint32) []byte {
	b := make([]byte, 12)
	b[0] = 0x80
	b[1] = pt
	binary.BigEndian.PutUint16(b[2:4], seq)
	binary.BigEndian.PutUint32(b[4:8], ts)
	binary.BigEndian.PutUint32(b[8:12], ssrc)
	return b
}

func rtcpSR(ssrc, ntp, rtpTS, packets, octets uint32) []byte {
	b := make([]byte, 28)
	b[0] = 0x80
	b[1] = 200
	binary.BigEndian.PutUint16(b[2:4], 6)
	binary.BigEndian.PutUint32(b[4:8], ssrc)
	binary.BigEndian.PutUint32(b[8:12], ntp)
	binary.BigEndian.PutUint32(b[16:20], rtpTS)
	binary.BigEndian.PutUint32(b[20:24], packets)
	binary.BigEndian.PutUint32(b[24:28], octets)
	return b
}

func rtcpRR(reporter, source uint32, frac byte, lost uint32) []byte {
	b := make([]byte, 32)
	b[0] = 0x81
	b[1] = 201
	binary.BigEndian.PutUint16(b[2:4], 7)
	binary.BigEndian.PutUint32(b[4:8], reporter)
	binary.BigEndian.PutUint32(b[8:12], source)
	b[12] = frac
	b[13] = byte(lost >> 16)
	b[14] = byte(lost >> 8)
	b[15] = byte(lost)
	return b
}

func rtcpRRWithExtendedHighest(reporter, source uint32, frac byte, lost, extendedHighest uint32) []byte {
	b := rtcpRR(reporter, source, frac, lost)
	binary.BigEndian.PutUint32(b[16:20], extendedHighest)
	return b
}

func TestProtocolSessionRTPSequenceJitterAndRTCP(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ssrc := uint32(0x12345678)
	p := s.Probe(rtpPkt(0, 1, 0, ssrc))
	require.Equal(t, ProbeAccept, p.Verdict)
	require.Equal(t, "rtp", p.Protocol)
	require.Equal(t, "3550", p.Version)

	r := s.Feed(0, time.Unix(1, 0), rtpPkt(0, 1, 0, ssrc))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "RTP", r.Events[0].Session["Packet Name"])
	require.Equal(t, "PCMU", r.Events[0].Session["Payload Type Name"])
	require.Equal(t, uint16(1), r.Events[0].Session["Sequence"])
	require.Equal(t, ssrc, r.Events[0].Session["SSRC"])
	require.Equal(t, false, r.Events[0].Session["Network Loss"])

	r = s.Feed(0, time.Unix(1, 20_000_000), rtpPkt(0, 2, 160, ssrc))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, uint16(2), r.Events[0].Session["Sequence"])

	r = s.Feed(0, time.Unix(1, 60_000_000), rtpPkt(0, 4, 480, ssrc))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, true, r.Events[0].Session["Gap"])
	require.Equal(t, uint32(1), r.Events[0].Session["Missing Count"])
	require.Equal(t, true, r.Events[0].Session["Capture Missing"])
	require.Equal(t, false, r.Events[0].Session["Network Loss"])
	require.Equal(t, "capture-missing", r.Events[0].Session["Loss Kind"])

	r = s.Feed(0, time.Unix(1, 80_000_000), rtpPkt(0, 3, 320, ssrc))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, true, r.Events[0].Session["Out of Order"])

	r = s.Feed(0, time.Unix(1, 100_000_000), rtpPkt(0, 4, 480, ssrc))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, true, r.Events[0].Session["Duplicate"])

	r = s.Feed(1, time.Unix(1, 120_000_000), rtcpSR(ssrc, 0x11121418, 480, 4, 80))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "SR", r.Events[0].Session["Packet Name"])
	require.Equal(t, true, r.Events[0].Session["Associated RTP"])
	require.Equal(t, uint32(4), r.Events[0].Session["Packet Count"])

	r = s.Feed(1, time.Unix(1, 140_000_000), rtcpRR(0xabcdef01, ssrc, 0x20, 1))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "RR", r.Events[0].Session["Packet Name"])
	blocks, _ := r.Events[0].Session["Report Blocks"].([]map[string]any)
	require.NotEmpty(t, blocks)
	require.Equal(t, ssrc, blocks[0]["Source SSRC"])
	require.Equal(t, true, blocks[0]["Associated RTP"])
	require.Equal(t, true, blocks[0]["Reported Network Loss"])
	require.Equal(t, false, r.Events[0].Session["Network Loss"])

	negative := s.Feed(1, time.Unix(1, 160_000_000), rtcpRR(0xabcdef01, ssrc, 0, 0xffffff))
	require.Nil(t, negative.Err, "%v", negative.Err)
	negativeBlocks, _ := negative.Events[0].Session["Report Blocks"].([]map[string]any)
	require.Len(t, negativeBlocks, 1)
	require.Equal(t, int32(-1), negativeBlocks[0]["Cumulative Packets Lost"], "RFC 3550 cumulative loss is signed 24-bit")
	require.Equal(t, false, negativeBlocks[0]["Reported Network Loss"], "negative cumulative loss is not positive network loss")
}

func TestProtocolSessionRTPSourceBudgetIncludesFixedRecentWindow(t *testing.T) {
	const requiredBytes = 512 + 96 + 128*4
	reserved := int64(0)
	rtp := &binRTP{
		sources:          map[uint32]*rtpSource{},
		maxBufferedBytes: requiredBytes - 1,
		reserveSessionMemory: func(target int64) error {
			reserved = target
			return nil
		},
	}
	_, err := rtp.consumeRTP(rtpPkt(0, 1, 0, 0x12345678), time.Unix(1, 0), 8)
	require.Error(t, err)
	var pe *ProtocolError
	require.ErrorAs(t, err, &pe)
	require.Equal(t, ErrResourceExceeded, pe.Kind)
	require.Empty(t, rtp.sources, "source and recent-history storage are reserved before allocation")
	require.Zero(t, reserved, "a rejected reservation does not reach the shared allocator")

	rtp.maxBufferedBytes = requiredBytes
	for seq := uint16(1); seq <= 140; seq++ {
		_, err = rtp.consumeRTP(rtpPkt(0, seq, uint32(seq)*160, 0x12345678), time.Unix(1, int64(seq)*20_000_000), 8)
		require.NoError(t, err)
	}
	source := rtp.sources[0x12345678]
	require.NotNil(t, source)
	require.Len(t, source.recent, 128)
	require.Equal(t, 128, cap(source.recent), "the rolling duplicate window must not grow beyond its reserved size")
	require.EqualValues(t, requiredBytes, reserved)
}

func TestProtocolSessionRTPWrapMultipleSSRCAndSDPMap(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	a, b := uint32(0x11111111), uint32(0x22222222)
	require.Nil(t, s.Feed(0, time.Unix(1, 0), rtpPkt(0, 65535, 0, a)).Err)
	r := s.Feed(0, time.Unix(1, 20_000_000), rtpPkt(0, 0, 160, a))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Nil(t, r.Events[0].Session["Gap"])
	require.Nil(t, r.Events[0].Session["Duplicate"])

	r = s.Feed(1, time.Unix(1, 0), rtpPkt(8, 1, 0, b))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "PCMA", r.Events[0].Session["Payload Type Name"])
	require.Equal(t, b, r.Events[0].Session["SSRC"])

	cs := s.(*captureSession)
	rtpApplySDPMap(cs.f.rtp, map[uint8]string{96: "opus"})
	r = s.Feed(0, time.Unix(1, 40_000_000), rtpPkt(96, 1, 0, 99))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "opus", r.Events[0].Session["Payload Type Name"])
	require.Equal(t, "dynamic-97", rtpPayloadName(97, nil))
}

func TestProtocolSessionRTCPReportLossBoundariesAndSequenceCycles(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ssrc := uint32(0x12345678)
	ts := time.Unix(6, 0)

	first := s.Feed(0, ts, rtpPkt(0, 65535, 0, ssrc))
	require.Nil(t, first.Err, "%v", first.Err)
	wrapped := s.Feed(0, ts.Add(20*time.Millisecond), rtpPkt(0, 0, 160, ssrc))
	require.Nil(t, wrapped.Err, "%v", wrapped.Err)
	require.Equal(t, uint16(0), wrapped.Events[0].Session["Sequence"])
	require.Nil(t, wrapped.Events[0].Session["Gap"], "the sequence number immediately after 65535 is contiguous")

	positive := s.Feed(1, ts.Add(40*time.Millisecond), rtcpRRWithExtendedHighest(0xabcdef01, ssrc, 0, 0x7fffff, 0x00010000))
	require.Nil(t, positive.Err, "%v", positive.Err)
	positiveBlocks, ok := positive.Events[0].Session["Report Blocks"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, positiveBlocks, 1)
	require.Equal(t, int32(0x7fffff), positiveBlocks[0]["Cumulative Packets Lost"], "RFC 3550 cumulative loss is signed 24-bit")
	require.Equal(t, true, positiveBlocks[0]["Reported Network Loss"])
	require.Equal(t, uint32(0x00010000), positiveBlocks[0]["Extended Highest Sequence"], "one sequence cycle plus low sequence zero is 65536")
	require.Equal(t, true, positiveBlocks[0]["Associated RTP"])

	negative := s.Feed(1, ts.Add(60*time.Millisecond), rtcpRRWithExtendedHighest(0xabcdef01, ssrc, 0, 0x800000, 0x00010000))
	require.Nil(t, negative.Err, "%v", negative.Err)
	negativeBlocks, ok := negative.Events[0].Session["Report Blocks"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, negativeBlocks, 1)
	require.Equal(t, int32(-0x800000), negativeBlocks[0]["Cumulative Packets Lost"], "the 24-bit sign boundary must extend to -8388608")
	require.Equal(t, false, negativeBlocks[0]["Reported Network Loss"], "negative cumulative loss is not positive network loss")
	require.Equal(t, uint32(0x00010000), negativeBlocks[0]["Extended Highest Sequence"])
}

func TestProtocolSessionRTPFailClosedAndProbe(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	require.Equal(t, ProbeNeedMore, s.Probe([]byte{0x80, 0x00}).Verdict)
	require.NotEqual(t, "rtp", s.Probe([]byte{0x00, 0x00, 0x00, 0x00}).Protocol)
	require.NotEqual(t, "rtp", s.Probe([]byte("INVITE sip:a SIP/2.0\r\n")).Protocol)

	r := s.Feed(0, ts, []byte{0x40, 0x00, 0x00, 0x01, 0, 0, 0, 2, 0, 0, 0, 3})
	require.True(t, r.Err != nil || r.State == "undetected" || len(r.Events) == 0 || r.Events[0].Status != "decoded")

	s2, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	cut := s2.Feed(0, ts, rtpPkt(0, 1, 0, 1)[:6])
	require.True(t, cut.NeedMore)
	require.Equal(t, ErrNeedMore, cut.Err.Kind)

	s3, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	bad := []byte{0x80, 100, 0, 0, 0, 0, 0, 1}
	r = s3.Feed(0, ts, bad)
	require.True(t, r.Err != nil || r.State == "undetected" || len(r.Events) == 0 || r.Events[0].Status != "decoded")
}

func TestProtocolSessionRTPFragmentation(t *testing.T) {
	ssrc := uint32(0x12345678)
	steps := []sessionStep{
		{0, rtpPkt(0, 1, 0, ssrc)},
		{0, rtpPkt(0, 2, 160, ssrc)},
		{0, rtpPkt(0, 3, 320, ssrc)},
		{1, rtcpSR(ssrc, 1, 320, 3, 0)},
		{1, rtcpRR(0xabcdef01, ssrc, 0, 0)},
	}
	assertFragmentation(t, steps, func(chunk int) []string {
		s, err := NewProtocolSession(DefaultParserBudget())
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
					if e.Status == "decoded" || e.Status == "deferred" {
						names = append(names, fmt.Sprintf("%v:%v", e.Session["Packet Name"], e.Session["SSRC"]))
					}
				}
				w = w[n:]
			}
		}
		return names
	})
}
