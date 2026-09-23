package pcaputil

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func consumeT20RTSP(t *testing.T, state *binRTSP, dir int, message string) map[string]any {
	t.Helper()
	fields, err := state.consume(dir, []byte(message), time.Unix(1_790_000_000, 0), DefaultParserBudget().MaxCollectionElements)
	require.NoError(t, err)
	return fields
}

func TestT20RTSPInterleavedRequiresOfferedPair(t *testing.T) {
	cases := []struct {
		name             string
		requestTransport string
		responsePair     string
		wantStatus       string
	}{
		{name: "no-request-offer", responsePair: "0-1", wantStatus: "request-interleaved-offer-not-observed"},
		{name: "response-selects-unoffered-pair", requestTransport: "RTP/AVP/TCP;interleaved=0-1", responsePair: "1-2", wantStatus: "interleaved-channel-offer-mismatch"},
		{name: "request-transport-is-ambiguous", requestTransport: "RTP/AVP/TCP;interleaved=0-1, RTP/AVP/TCP;interleaved=2-3", responsePair: "0-1", wantStatus: "request-interleaved-offer-not-observed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state := &binRTSP{}
			request := "SETUP rtsp://camera.example/live RTSP/1.0\r\nCSeq: 1\r\n"
			if tc.requestTransport != "" {
				request += "Transport: " + tc.requestTransport + "\r\n"
			}
			request += "\r\n"
			consumeT20RTSP(t, state, 0, request)

			response := "RTSP/1.0 200 OK\r\nCSeq: 1\r\nSession: camera\r\nTransport: RTP/AVP/TCP;interleaved=" + tc.responsePair + "\r\n\r\n"
			fields := consumeT20RTSP(t, state, 1, response)
			require.Equal(t, tc.wantStatus, fields["Interleaved Transport Status"])
			require.Empty(t, state.channels, "an unoffered channel pair must not be installed")
		})
	}
}

func TestT20RTSPInterleavedChannelReuseIsAtomic(t *testing.T) {
	state := &binRTSP{}
	setup := func(seq int, pair string) {
		request := "SETUP rtsp://camera.example/live RTSP/1.0\r\nCSeq: " + strconv.Itoa(seq) + "\r\nTransport: RTP/AVP/TCP;interleaved=" + pair + "\r\n\r\n"
		consumeT20RTSP(t, state, 0, request)
		response := "RTSP/1.0 200 OK\r\nCSeq: " + strconv.Itoa(seq) + "\r\nSession: camera\r\nTransport: RTP/AVP/TCP;interleaved=" + pair + "\r\n\r\n"
		consumeT20RTSP(t, state, 1, response)
	}
	setup(1, "0-1")
	require.Equal(t, rtspChannel{session: "camera", rtcp: false}, state.channels[0])
	require.Equal(t, rtspChannel{session: "camera", rtcp: true}, state.channels[1])

	request := "SETUP rtsp://camera.example/audio RTSP/1.0\r\nCSeq: 2\r\nTransport: RTP/AVP/TCP;interleaved=1-2\r\n\r\n"
	consumeT20RTSP(t, state, 0, request)
	response := "RTSP/1.0 200 OK\r\nCSeq: 2\r\nSession: camera\r\nTransport: RTP/AVP/TCP;interleaved=1-2\r\n\r\n"
	_, err := state.consume(1, []byte(response), time.Unix(1_790_000_001, 0), DefaultParserBudget().MaxCollectionElements)
	require.Error(t, err, "reusing an already assigned RTSP channel must fail")
	require.Len(t, state.channels, 2)
	require.Equal(t, rtspChannel{session: "camera", rtcp: true}, state.channels[1], "failed setup must preserve the old channel type")
	_, exists := state.channels[2]
	require.False(t, exists, "the new channel must not be partially installed")
}

func TestT20RTSPUDPAssociationLinksRequestAndResponseEvidence(t *testing.T) {
	state := &binRTSP{}
	var requestEvidenceCharge int64
	state.reserveRequestEvidence = func(delta int64) error {
		requestEvidenceCharge += delta
		return nil
	}
	parser := &binParser{
		budget:    DefaultParserBudget(),
		config:    BinParserConfig{MaxMessageBytes: DefaultParserBudget().MaxMessageBytes, MaxBufferedBytes: DefaultParserBudget().MaxBufferedBytes},
		rtspMedia: &rtspMediaRegistry{index: make(map[rtspMediaTuple][]rtspMediaBinding)},
	}
	domain := CaptureDomain{Interface: 3}
	ts := time.Unix(1_790_000_000, 0)
	requestMessage := "SETUP rtsp://camera.example/live RTSP/1.0\r\nCSeq: 1\r\nTransport: RTP/AVP/UDP;unicast;client_port=5000-5001\r\n\r\n"
	requestFields := consumeT20RTSP(t, state, 0, requestMessage)
	requestEvent := &ProtocolEvent{
		ID: 41, Protocol: "rtsp", Domain: domain, Timestamp: ts,
		Source: "192.0.2.1:554", Destination: "192.0.2.2:50000", Session: requestFields,
		SourceBytes: ByteSource{PacketRefs: []PacketReference{{Number: 101, Domain: domain}, {Number: 102, Domain: domain}}},
	}
	parser.observeRTSPMedia(requestEvent)
	require.Equal(t, "retained", requestEvent.Session["RTSP Request Evidence Status"])
	require.EqualValues(t, 56, requestEvidenceCharge, "request event id and both packet references are charged")
	require.EqualValues(t, 56, state.requestEvidenceBytes)

	responseMessage := "RTSP/1.0 200 OK\r\nCSeq: 1\r\nSession: udp-camera\r\nTransport: RTP/AVP/UDP;unicast;client_port=5000-5001;server_port=6000-6001\r\n\r\n"
	responseFields := consumeT20RTSP(t, state, 1, responseMessage)
	responseEvent := &ProtocolEvent{
		ID: 42, Protocol: "rtsp", Domain: domain, Timestamp: ts.Add(time.Millisecond),
		Source: "192.0.2.2:554", Destination: "192.0.2.1:50000", Session: responseFields,
		SourceBytes: ByteSource{PacketRefs: []PacketReference{{Number: 103, Domain: domain}}},
	}
	parser.observeRTSPMedia(responseEvent)
	require.Equal(t, []uint64{41}, responseEvent.Session["RTSP SETUP Request Event IDs"])
	require.Equal(t, []PacketReference{{Number: 101, Domain: domain}, {Number: 102, Domain: domain}}, responseEvent.Session["RTSP SETUP Request Packet References"])
	require.Equal(t, "mapped", responseEvent.Session["UDP Media Association Status"])
	require.Len(t, parser.rtspMedia.entries, 1)
	association := parser.rtspMedia.entries[0]
	require.Equal(t, []uint64{41, 42}, association.eventIDs)
	require.Equal(t, []PacketReference{{Number: 101, Domain: domain}, {Number: 102, Domain: domain}, {Number: 103, Domain: domain}}, association.packetRefs)
	require.Equal(t, int64(256+len("udp-camera")+len("192.0.2.2:5000")+len("192.0.2.2:5001")+len("192.0.2.1:6000")+len("192.0.2.1:6001")+3*24+2*8), association.cost)

	match := parser.matchRTSPMedia(domain, "192.0.2.1:5000", "192.0.2.2:6000", ts.Add(2*time.Millisecond))
	mediaEvent := &ProtocolEvent{Domain: domain, Source: "192.0.2.1:5000", Destination: "192.0.2.2:6000", Timestamp: ts.Add(2 * time.Millisecond)}
	require.True(t, parser.decodeRTSPMediaDatagram(mediaEvent, rtspUDPTestRTP(96, 1, 0x12345678), match))
	require.Equal(t, []uint64{41, 42}, mediaEvent.Session["RTSP SETUP Event IDs"])
	require.Equal(t, association.packetRefs, mediaEvent.Session["RTSP SETUP Packet References"])
}
