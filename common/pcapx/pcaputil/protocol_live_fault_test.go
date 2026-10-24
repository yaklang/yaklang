package pcaputil

import (
	"encoding/hex"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestQUICRepeatedCIDRetirementDoesNotInvalidateFollowingFrames(t *testing.T) {
	q := &binQUIC{native: true, byteLimit: 4096, cids: map[string]int{"server": 1}}
	q.cidSequences[1] = map[uint64]string{0: "server"}
	retire := func() map[string]any {
		return map[string]any{"Frame Type": "RETIRE_CONNECTION_ID", "Sequence": uint64(0)}
	}
	require.NoError(t, q.applyFrames(0, 2, []map[string]any{retire()}, map[string]any{}, 8))
	info := map[string]any{}
	frames := []map[string]any{retire(), {"Frame Type": "STREAM", "Stream ID": uint64(0), "Offset": uint64(0), "Stream Data": []byte("body"), "FIN": true}}
	require.NoError(t, q.applyFrames(0, 2, frames, info, 8))
	require.Equal(t, true, frames[0]["Already Retired"])
	require.Equal(t, []byte("body"), q.streams[0].reassembly[0].data)
	require.NotContains(t, q.cids, "server")
	// A retransmitted NEW_CONNECTION_ID cannot resurrect a retired route.
	newCID := map[string]any{"Frame Type": "NEW_CONNECTION_ID", "Sequence": uint64(0), "Retire Prior To": uint64(0), "CID": []byte("server")}
	require.NoError(t, q.applyFrames(1, 2, []map[string]any{newCID}, info, 8))
	require.NotContains(t, q.cids, "server")
	newCID["CID"] = []byte("changed")
	require.ErrorContains(t, q.applyFrames(1, 2, []map[string]any{newCID}, info, 8), "sequence changed")
	unknown := map[string]any{"Frame Type": "RETIRE_CONNECTION_ID", "Sequence": uint64(99)}
	require.ErrorContains(t, q.applyFrames(0, 2, []map[string]any{unknown}, info, 8), "unobserved")
}

func TestCoAPDatagramAdmissionNeedsAnObservedRequest(t *testing.T) {
	c := NewDefaultConfig()
	require.NoError(t, WithOnProtocolMessage(func(*ProtocolEvent) {})(c))
	require.NoError(t, c.prepareBinParser())
	a := c.binParser
	defer a.closeUDPSessions()
	// The next fresh run exposed a QUIC packet resembling DELETE, but its
	// supposed If-Match was twelve bytes (RFC 7252 permits at most eight).
	require.Equal(t, ProbeReject, probeCoAP([]byte{0x40, 4, 0, 1, 0x1c, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, 64).Verdict)
	require.False(t, a.decodeCoAPDatagram(&ProtocolEvent{}, []byte{0x40, 1, 0, 1}), "bare GET is insufficient automatic evidence")
	explicit, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	root := explicit.Feed(0, time.Unix(1, 0), []byte{0x40, 1, 0, 1})
	require.Len(t, root.Events, 1)
	require.Equal(t, "coap", root.Events[0].Protocol, "isolated caller-selected byte sessions preserve weak-envelope inspection")
	explicit.Close("EOF")
	// Minimized protected short-header datagrams from the fresh 2026-10-24
	// quic-go capture (source pcap SHA-256 recorded by live-audit). Both passed
	// the old structural probe, despite being authenticated QUIC wire.
	for _, h := range []string{"448ddbb71ebca4f7c2bd8ac91ad08feacd2557ee4284a0fc2a58", "588ddbb71e3ac5e1ffb929b31a947d66ea0f18cf7f76123667b62178c7eb"} {
		wire, err := hex.DecodeString(h)
		require.NoError(t, err)
		require.False(t, a.decodeCoAPDatagram(&ProtocolEvent{Source: "127.0.0.1:40000", Destination: "127.0.0.1:50000"}, wire))
	}
	request := []byte{0x41, 1, 0x12, 0x34, 0x99, 0xb1, 'a'}
	e := &ProtocolEvent{Source: "127.0.0.1:40000", Destination: "127.0.0.1:50000", Timestamp: time.Unix(1, 0)}
	require.True(t, a.decodeCoAPDatagram(e, request))
	require.Equal(t, "decoded", e.Status, e.Error)
	response := []byte{0x61, 69, 0x12, 0x34, 0x99, 0xff, 'x'}
	r := &ProtocolEvent{Source: e.Destination, Destination: e.Source, Timestamp: time.Unix(2, 0)}
	require.True(t, a.decodeCoAPDatagram(r, response))
	require.Equal(t, e.FlowID, r.FlowID)
	require.Equal(t, "GET", r.Session["In Reply To"])
	other := &ProtocolEvent{Source: e.Destination, Destination: e.Source, Domain: CaptureDomain{Interface: 1}, Timestamp: time.Unix(2, 0)}
	require.False(t, a.decodeCoAPDatagram(other, response))
	expired := &ProtocolEvent{Source: e.Destination, Destination: e.Source, Timestamp: time.Unix(1000, 0)}
	require.False(t, a.decodeCoAPDatagram(expired, response))
}

func TestHTTP2MissingPeerControlDoesNotBecomeMalformedDATA(t *testing.T) {
	for _, missing := range []bool{false, true} {
		h := newBinHTTP2(0)
		h.initial = [2]bool{true, true}
		h.peerCaptureGap = missing
		h.window[1] = 0
		h.streams[1] = &binH2Stream{headers: [2]bool{true, true}, window: [2]int64{65535, 0}}
		info, err := h.consume(1, h2TestFrame(0, 0, 1, []byte("reply")))
		if !missing {
			require.ErrorContains(t, err, "flow-control window exceeded")
		} else {
			require.NoError(t, err)
			require.Equal(t, "unavailable: peer TCP sequence gap", info["Flow Control Verification"])
		}
	}
}
