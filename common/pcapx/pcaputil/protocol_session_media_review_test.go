package pcaputil

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func mediaReviewRTCPRR(reportBlocks int) []byte {
	wire := make([]byte, 8+24*reportBlocks)
	wire[0] = 0x80 | byte(reportBlocks)
	wire[1] = 201
	binary.BigEndian.PutUint16(wire[2:4], uint16(len(wire)/4-1))
	return wire
}

func mediaReviewRTCPBYE(sources int) []byte {
	wire := make([]byte, 4+4*sources)
	wire[0] = 0x80 | byte(sources)
	wire[1] = 203
	binary.BigEndian.PutUint16(wire[2:4], uint16(len(wire)/4-1))
	return wire
}

func requireResourceExceeded(t *testing.T, err error) {
	t.Helper()
	var pe *ProtocolError
	require.ErrorAs(t, err, &pe)
	require.Equal(t, ErrResourceExceeded, pe.Kind)
}

func TestProtocolSessionRTCPCollectionBudgets(t *testing.T) {
	rtp := &binRTP{sources: map[uint32]*rtpSource{}}
	_, err := rtp.consumeRTCPBounded(mediaReviewRTCPRR(2), 1)
	requireResourceExceeded(t, err)

	_, err = rtp.consumeRTCPBounded(mediaReviewRTCPBYE(2), 1)
	requireResourceExceeded(t, err)

	compound := append(rtcpSR(1, 1, 2, 3, 4), rtcpSR(5, 6, 7, 8, 9)...)
	_, err = rtp.consumeRTCPCompound(compound, 1)
	requireResourceExceeded(t, err)
}

func TestProtocolSessionRTPInstalledReservationCallbackChargesSourceHistory(t *testing.T) {
	const sourceStateBytes = int64(512 + 96 + 128*4)
	reserved := int64(0)
	sources := map[uint32]*rtpSource{}
	rtp := &binRTP{
		sources:          sources,
		maxBufferedBytes: int(sourceStateBytes),
		reserveSessionMemory: func(target int64) error {
			require.Empty(t, sources, "source history must be reserved before the source is allocated")
			reserved = target
			return nil
		},
	}

	_, err := rtp.consumeRTP(rtpPkt(96, 1, 0, 0x12345678), time.Unix(1, 0), 8)
	require.NoError(t, err)
	require.Equal(t, sourceStateBytes, reserved)
	require.Len(t, sources[0x12345678].recent, 1)
	require.Equal(t, 128, cap(sources[0x12345678].recent))
}

func TestProtocolSessionRTPAddsSDPMapsWithinRetainedMemoryBudget(t *testing.T) {
	s, err := NewProtocolSession(ParserBudget{MaxMessageBytes: 64, MaxBufferedBytes: 1200})
	require.NoError(t, err)
	parser := s.(*captureSession).f.a

	const (
		ssrc = uint32(0x12345678)
		src  = "192.0.2.10:5004"
		dst  = "192.0.2.20:5004"
	)
	ts := time.Unix(1, 0)
	first := &ProtocolEvent{Source: src, Destination: dst, Timestamp: ts}
	require.True(t, parser.decodeRTPDatagram(first, rtpPkt(96, 1, 0, ssrc), false, false, sipMediaMatch{}))
	require.Nil(t, first.sessionError, "%v", first.sessionError)
	require.EqualValues(t, 512+96+128*4, parser.buffered.Load(), "the pre-SDP source and its history fit the budget")

	var flow *binFlow
	for _, el := range parser.udpSessions.entries {
		flow = el.Value.(*binUDPEntry).flow
		break
	}
	require.NotNil(t, flow)
	require.EqualValues(t, 1, flow.rtp.sources[ssrc].received)

	second := &ProtocolEvent{Source: src, Destination: dst, Timestamp: ts.Add(time.Second)}
	match := sipMediaMatch{status: "matched", payload: 96, encoding: "opus", clockRate: 48000}
	require.True(t, parser.decodeRTPDatagram(second, rtpPkt(96, 2, 480, ssrc), false, false, match))
	require.NotNil(t, second.sessionError)
	requireResourceExceeded(t, second.sessionError)
	require.Nil(t, flow.rtp.payload, "a rejected map growth must not allocate or retain the payload map")
	require.Nil(t, flow.rtp.clocks, "a rejected map growth must not allocate or retain the clock map")
	require.EqualValues(t, 1, flow.rtp.sources[ssrc].received, "the rejected configuration does not consume RTP history")
	require.EqualValues(t, 512+96+128*4, parser.buffered.Load(), "the shared reservation stays at the pre-SDP high-water mark")
}
