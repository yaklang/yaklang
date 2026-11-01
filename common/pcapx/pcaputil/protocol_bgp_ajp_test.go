package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/gopacket/gopacket/layers"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

func bgpTestWire(typ byte, body []byte) []byte {
	w := bytes.Repeat([]byte{255}, 16)
	w = binary.BigEndian.AppendUint16(w, uint16(19+len(body)))
	return append(append(w, typ), body...)
}
func bgpTestOpen(as4 bool) []byte {
	b := []byte{4, 0xfd, 0xe8, 0, 90, 192, 0, 2, 1, 0}
	if as4 {
		caps := []byte{2, 6, 65, 4, 0, 1, 0, 1}
		b[9] = byte(len(caps))
		b = append(b, caps...)
	}
	return bgpTestWire(1, b)
}
func observedTestSession(t *testing.T, port uint16) ProtocolSession {
	t.Helper()
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionPorts(40000, port), WithSessionClientDirection(0))
	require.NoError(t, err)
	t.Cleanup(func() { s.Close("test close"); require.Zero(t, s.Stats().BufferedBytes) })
	return s
}
func TestBGPFragmentationAndNegotiatedASWidth(t *testing.T) {
	w := bgpTestOpen(true)
	for cut := 1; cut < len(w); cut++ {
		t.Run(fmt.Sprint(cut), func(t *testing.T) {
			s := observedTestSession(t, 179)
			require.Empty(t, s.Feed(0, time.Unix(1, 0), w[:cut]).Events)
			r := s.Feed(0, time.Unix(2, 0), w[cut:])
			require.Len(t, r.Events, 1)
			require.Equal(t, "decoded", r.Events[0].Status, r.Events[0].Error)
			r = s.Feed(1, time.Unix(3, 0), w)
			require.Len(t, r.Events, 1)
			require.Equal(t, 4, r.Events[0].Session["AS Width Context"])
			// Four-octet AS_PATH, extended-length community, and a /24 NLRI.
			attrs := []byte{0x40, 1, 1, 0, 0x40, 2, 6, 2, 1, 0, 1, 0, 1, 0xd0, 8, 0, 4, 0xfd, 0xe8, 0, 1}
			b := []byte{0, 0}
			b = binary.BigEndian.AppendUint16(b, uint16(len(attrs)))
			b = append(b, attrs...)
			b = append(b, 24, 192, 0, 2)
			r = s.Feed(0, time.Unix(4, 0), bgpTestWire(2, b))
			require.Len(t, r.Events, 1)
			e := r.Events[0]
			require.Empty(t, e.Error)
			asns := e.Session["Path Attributes"].([]map[string]any)[1]["Segments"].([]map[string]any)[0]["ASNs"]
			require.Equal(t, []uint32{65537}, asns)
			want, err := e.GetFields()
			require.NoError(t, err)
			e.Session = map[string]any{"Type": 999}
			require.Equal(t, []any{uint64(2)}, e.DisplayFields()["bgp.type"])
			got, err := e.GetFields()
			require.NoError(t, err)
			require.Equal(t, want, got)
		})
	}
}
func TestBGPRepeatedOPENAndMalformedMessageDoNotInventNegotiation(t *testing.T) {
	s := observedTestSession(t, 179)
	s.Feed(0, time.Unix(1, 0), bgpTestOpen(true))
	s.Feed(1, time.Unix(2, 0), bgpTestOpen(true))
	w := append(bgpTestOpen(false), bgpTestWire(4, nil)...)
	r := s.Feed(0, time.Unix(3, 0), w)
	require.Len(t, r.Events, 2)
	require.Equal(t, "context-required", r.Events[0].Status)
	require.Equal(t, 0, r.Events[1].Session["AS Width Context"])
	bad := bgpTestWire(2, []byte{0, 0, 0, 3, 0x40, 1, 9})
	w = append(bad, bgpTestWire(4, nil)...)
	r = s.Feed(0, time.Unix(4, 0), w)
	require.Len(t, r.Events, 2)
	require.Equal(t, "malformed", r.Events[0].Status)
	require.Empty(t, r.Events[1].Error)
}
func TestBGPDoesNotGuessAmbiguousPrefixOrASWidth(t *testing.T) {
	attrs := []byte{0x40, 2, 6, 2, 1, 0, 1, 0, 1}
	b := []byte{0, 0, 0, byte(len(attrs))}
	b = append(b, attrs...)
	f, err := stream_parser.DecodeBGPMessage(bgpTestWire(2, b), stream_parser.BGPWireContext{}, 4096)
	require.NoError(t, err)
	a := f["Path Attributes"].([]map[string]any)[0]
	require.Contains(t, a, "Context Required")
	require.NotContains(t, a, "Segments")
	for _, w := range [][]byte{[]byte("arbitrary tcp"), {255, 255, 0}, bgpTestWire(4, []byte{0})} {
		require.NotEqual(t, ProbeAccept, probeBGP(w, 4096).Verdict)
	}
}

func TestBGPDirectionalAddPathRequiresObservedCapabilityPair(t *testing.T) {
	open := func(mode byte) []byte {
		w := bgpTestOpen(false)
		cap := []byte{2, 6, 69, 4, 0, 1, 1, mode}
		w[28] = byte(len(cap))
		w = append(w, cap...)
		binary.BigEndian.PutUint16(w[16:], uint16(len(w)))
		return w
	}
	s := observedTestSession(t, 179)
	s.Feed(0, time.Unix(1, 0), open(2))
	s.Feed(1, time.Unix(2, 0), open(1))
	update := bgpTestWire(2, []byte{0, 0, 0, 0, 0, 0, 0, 7, 24, 192, 0, 2})
	r := s.Feed(0, time.Unix(3, 0), update)
	require.Len(t, r.Events, 1)
	require.Empty(t, r.Events[0].Error)
	nlri := r.Events[0].Session["NLRI"].(map[string]any)
	require.Equal(t, true, nlri["Add Path"])
	prefix := nlri["Prefixes"].([]map[string]any)[0]
	require.Equal(t, uint32(7), prefix["Path ID"])
	require.Equal(t, "192.0.2.0/24", prefix["Prefix"])
	// Reverse direction has no negotiated send capability. A valid plain NLRI
	// must not inherit the forward direction's Path ID interpretation.
	r = s.Feed(1, time.Unix(4, 0), bgpTestWire(2, []byte{0, 0, 0, 0, 24, 198, 51, 100}))
	require.Empty(t, r.Events[0].Error)
	require.Equal(t, false, r.Events[0].Session["NLRI"].(map[string]any)["Add Path"])
}

func TestAJPPrematureBodyEOFIsIncompleteAndCannotAuthorizeReuse(t *testing.T) {
	s := observedTestSession(t, 8009)
	s.Feed(0, time.Unix(1, 0), ajpTestForward("/eof", 4))
	r := s.Feed(0, time.Unix(2, 0), ajpTestWire(true, []byte{0, 0}))
	require.Len(t, r.Events, 1)
	require.Equal(t, "incomplete", r.Events[0].Status)
	r = s.Feed(1, time.Unix(3, 0), ajpTestWire(false, []byte{5, 1}))
	require.Zero(t, r.Events[0].ResponseTo)
	r = s.Feed(0, time.Unix(4, 0), ajpTestForward("/next", 0))
	require.Equal(t, "context-required", r.Events[0].Status)
}

func ajpTestString(s string) []byte {
	return append(append(binary.BigEndian.AppendUint16(nil, uint16(len(s))), []byte(s)...), 0)
}
func ajpTestWire(proxy bool, b []byte) []byte {
	w := []byte{0x12, 0x34}
	if !proxy {
		w = []byte{0x41, 0x42}
	}
	w = binary.BigEndian.AppendUint16(w, uint16(len(b)))
	return append(w, b...)
}
func ajpTestForward(uri string, length int) []byte {
	b := []byte{2, 4}
	for _, s := range []string{"HTTP/1.1", uri, "192.0.2.1", "", "example.test"} {
		b = append(b, ajpTestString(s)...)
	}
	b = append(b, 0, 80, 0, 0, 1, 0xa0, 8)
	b = append(b, ajpTestString(fmt.Sprint(length))...)
	return ajpTestWire(true, append(b, 255))
}
func TestAJPRequestBodyIsNotASecondForwardAndConnectionReuses(t *testing.T) {
	for _, cut := range []int{1, 3, 4, 9, 21} {
		t.Run(fmt.Sprint(cut), func(t *testing.T) {
			s := observedTestSession(t, 8009)
			first := ajpTestForward("/upload", 514)
			require.Empty(t, s.Feed(0, time.Unix(1, 0), first[:cut]).Events)
			r := s.Feed(0, time.Unix(2, 0), first[cut:])
			require.Len(t, r.Events, 1)
			request := r.Events[0].ID
			require.NotZero(t, request)
			// Body length 0x0202 begins with the FORWARD_REQUEST type byte.
			data := bytes.Repeat([]byte{'x'}, 514)
			body := binary.BigEndian.AppendUint16(nil, 514)
			body = append(body, data...)
			r = s.Feed(0, time.Unix(3, 0), ajpTestWire(true, body))
			require.Len(t, r.Events, 1)
			require.Equal(t, "Request Body", r.Events[0].Session["Packet Name"])
			require.Equal(t, data, r.Events[0].Session["Body"])
			require.Equal(t, request, r.Events[0].TransactionID)
			h := []byte{4, 0, 200}
			h = append(h, ajpTestString("OK")...)
			h = append(h, 0, 0)
			r = s.Feed(1, time.Unix(4, 0), append(ajpTestWire(false, h), ajpTestWire(false, []byte{5, 1})...))
			require.Len(t, r.Events, 2)
			require.Equal(t, request, r.Events[1].ResponseTo)
			r = s.Feed(0, time.Unix(5, 0), ajpTestForward("/two", 0))
			require.Len(t, r.Events, 1)
			require.NotEqual(t, request, r.Events[0].TransactionID)
			r = s.Feed(1, time.Unix(6, 0), ajpTestWire(false, []byte{5, 0}))
			require.Len(t, r.Events, 1)
			r = s.Feed(0, time.Unix(7, 0), ajpTestForward("/closed", 0))
			require.Len(t, r.Events, 1)
			require.Equal(t, "context-required", r.Events[0].Status)
			require.Zero(t, r.Events[0].TransactionID)
		})
	}
}
func TestAJPConflictingRequestsAndPartialFrames(t *testing.T) {
	s := observedTestSession(t, 8009)
	s.Feed(0, time.Unix(1, 0), ajpTestForward("/one", 0))
	r := s.Feed(0, time.Unix(2, 0), ajpTestForward("/overlap", 0))
	require.Len(t, r.Events, 1)
	require.Equal(t, "context-required", r.Events[0].Status)
	r = s.Feed(1, time.Unix(3, 0), ajpTestWire(false, []byte{5, 1}))
	require.Zero(t, r.Events[0].ResponseTo)
	// Full grammar, not two matching prefix bytes, establishes identity.
	w := ajpTestForward("/a", 0)
	for cut := 0; cut < len(w); cut++ {
		require.NotEqual(t, ProbeAccept, probeAJP(w[:cut], 4096).Verdict)
	}
	bad := bytes.Clone(w)
	bad[len(bad)-1] = 0
	require.Equal(t, ProbeReject, probeAJP(bad, 4096).Verdict)
	_, err := stream_parser.DecodeAJPMessage(ajpTestWire(true, []byte{2, 2, 1, 1}), true, 4096)
	require.Error(t, err)
}
func TestBGP_AJP_RTMPTransportReorderRetransmissionAndNewGeneration(t *testing.T) {
	for _, tc := range []struct {
		name string
		port layers.TCPPort
		wire []byte
	}{{"bgp", 179, bgpTestOpen(true)}, {"ajp", 8009, ajpTestWire(true, []byte{10})}, {"rtmp", 1935, rtmpTestInitial()}} {
		t.Run(tc.name, func(t *testing.T) {
			w := string(tc.wire)
			cut := min(8, len(w)-1)
			steps := []tcpStep{{seq: 99, syn: true}, {seq: uint32(100 + cut), data: w[cut:]}, {seq: 100, data: w[:cut]}, {seq: 100, data: w[:cut]}, {seq: uint32(100 + len(w)), rst: true}, {seq: 9000, syn: true}, {seq: 19000, ack: 9001, syn: true, synack: true, reverse: true}, {seq: 9001, ack: 19001, data: w}}
			es, stats, err := binReplay(t, binTestPcap(t, steps, tc.port, false, false), 1, WithTCPReassemblyOptions(TCPReassemblyOptions{AllowIncomplete: true}))
			require.NoError(t, err)
			var good []*ProtocolEvent
			for _, e := range es {
				if e.Protocol == tc.name && e.Status == "decoded" {
					good = append(good, e)
				}
			}
			require.Len(t, good, 2)
			require.NotEqual(t, good[0].FlowID, good[1].FlowID)
			require.Zero(t, stats.BufferedBytes)
		})
	}
}
