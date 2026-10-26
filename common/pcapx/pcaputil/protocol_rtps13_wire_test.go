package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func rtps13Cases(t *testing.T) []rtpsControl {
	t.Helper()
	if root := os.Getenv("RTPS13_TEST_ROOT"); root != "" {
		b, err := os.ReadFile(filepath.Join(root, "manifest.json"))
		require.NoError(t, err)
		var m struct{ Cases []rtpsControl }
		require.NoError(t, json.Unmarshal(b, &m))
		require.Len(t, m.Cases, 190)
		return m.Cases
	}
	return rtpsBatchControls(t, "rtps13-wire", 190)
}
func rtps13Input(t *testing.T, c rtpsControl) ([]byte, rtpsControlAnswer) {
	t.Helper()
	read := func(file string) []byte {
		var b []byte
		var err error
		if root := os.Getenv("RTPS13_TEST_ROOT"); root != "" {
			b, err = os.ReadFile(filepath.Join(root, file))
		} else {
			b, err = trafficfixture.ReadFile(rtpsControlPath(t, c, file))
		}
		require.NoError(t, err)
		return b
	}
	raw, ans := read(c.Capture), read(c.Answer)
	require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
	require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(ans)))
	var a rtpsControlAnswer
	require.NoError(t, json.Unmarshal(ans, &a))
	require.Len(t, a.Wires, c.Packets)
	require.Len(t, a.Events, c.Packets)
	require.Len(t, a.PureErrors, c.Packets)
	r, err := NewCaptureReader(bytes.NewReader(raw))
	require.NoError(t, err)
	for _, w := range a.Wires {
		b, _, err := r.ReadPacketData()
		require.NoError(t, err)
		p := gopacket.NewPacket(b, r.LinkType(), gopacket.Default)
		u := p.Layer(layers.LayerTypeUDP).(*layers.UDP)
		require.Equal(t, w, hex.EncodeToString(u.Payload))
	}
	_, _, err = r.ReadPacketData()
	require.ErrorIs(t, err, io.EOF)
	return raw, a
}
func newRTPS13Session(t *testing.T, b ParserBudget) ProtocolSession {
	t.Helper()
	s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(38000, 28400), WithSessionRTPS13Wire(28400))
	require.NoError(t, err)
	return s
}
func TestRTPS13WireSealedSession(t *testing.T) {
	for _, c := range rtps13Cases(t) {
		t.Run(c.ID, func(t *testing.T) {
			_, a := rtps13Input(t, c)
			s := newRTPS13Session(t, DefaultParserBudget())
			for i, encoded := range a.Wires {
				w, err := hex.DecodeString(encoded)
				require.NoError(t, err)
				pure, err := decodeRTPS13Wire(w, DefaultParserBudget())
				rocTypedError(t, a.PureErrors[i], err)
				if err == nil {
					rocEqualFields(t, a.Events[i].Fields, pure)
				} else {
					require.Nil(t, pure)
				}
				before := s.Stats()
				for j := 0; j < 2; j++ {
					p := s.Probe(w)
					verdict := ProbeAccept
					if a.PureErrors[i] != "" {
						verdict = ProbeReject
					}
					require.Equal(t, verdict, p.Verdict)
					require.Equal(t, before, s.Stats())
				}
				r := s.Feed(i%2, time.Unix(1700000000, int64(i)), w)
				clear(w)
				require.Len(t, r.Events, 1)
				e := r.Events[0]
				require.EqualValues(t, i+1, e.ID)
				require.Equal(t, "rtps", e.Protocol)
				require.Equal(t, "rtps13-wire-native", e.Profile)
				require.Zero(t, e.ResponseTo)
				require.Zero(t, e.TransactionID)
				require.Equal(t, a.Events[i].Raw, hex.EncodeToString(e.Raw))
				f, err := e.GetFields()
				if a.PureErrors[i] != "" {
					require.Nil(t, f)
					assertOwnedProtocolRefusal(t, e, a.PureErrors[i], err)
					require.Nil(t, e.Session)
				} else {
					require.NoError(t, err)
					rocEqualFields(t, a.Events[i].Fields, f)
					rocEqualFields(t, a.Events[i].Fields, e.Session)
					mutateRTPSQuery(f)
					mutateRTPSQuery(e.Session)
					clear(e.Raw)
					testNativeObservationSessionQueries(t, e, a.Events[i].Fields)
				}
				require.Zero(t, s.Stats().BufferedBytes)
			}
			require.Empty(t, s.Close("end"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
			r := s.Feed(0, time.Unix(1, 0), []byte("late"))
			require.Empty(t, r.Events)
			require.Zero(t, r.Consumed)
			rocTypedError(t, "FatalSessionError", r.Err)
		})
	}
}
func TestRTPS13WireSealedReplay(t *testing.T) {
	for _, c := range rtps13Cases(t) {
		t.Run(c.ID, func(t *testing.T) {
			raw, a := rtps13Input(t, c)
			for _, w := range []int{1, 2, 4} {
				for _, d := range []bool{false, true} {
					for _, o := range []bool{false, true} {
						t.Run(fmt.Sprintf("w%d/deferred%v/observer%v", w, d, o), func(t *testing.T) {
							var events []*ProtocolEvent
							var stats ProtocolStats
							opts := []CaptureOption{WithTCPReassemblyWorkers(w), WithProtocolDeferred(d), WithProtocolDecodeAs("udp", 28400, "rtps13-wire"), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
							seen := 0
							if o {
								opts = append(opts, WithEveryPacket(func(gopacket.Packet) { seen++ }))
							}
							input := bytes.Clone(raw)
							require.NoError(t, ReplayPcap(bytes.NewReader(input), opts...))
							clear(input)
							require.Len(t, events, len(a.Events))
							require.Zero(t, stats.BufferedBytes)
							require.Zero(t, stats.CallbackPanics)
							if o {
								require.Equal(t, c.Packets, seen)
							}
							for i, e := range events {
								want := a.Events[i]
								require.Equal(t, "rtps", e.Protocol)
								require.Equal(t, "rtps13-wire-native", e.Profile)
								require.Equal(t, "udp", e.Transport)
								require.EqualValues(t, i+1, e.ID)
								require.Equal(t, want.Raw, hex.EncodeToString(e.Raw))
								require.Zero(t, e.ResponseTo)
								require.Zero(t, e.TransactionID)
								require.Equal(t, "192.0.2.10:38000", e.Source)
								require.Equal(t, "192.0.2.20:28400", e.Destination)
								require.Len(t, e.SourceBytes.PacketRefs, 1)
								require.EqualValues(t, i+1, e.SourceBytes.PacketRefs[0].Number)
								require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
								require.Empty(t, e.SourceBytes.ParentPDUs)
								f, err := e.GetFields()
								if want.Error != "" {
									require.Nil(t, f)
									assertOwnedProtocolRefusal(t, e, want.Error, err)
									require.Nil(t, e.Session)
								} else {
									require.NoError(t, err)
									rocEqualFields(t, want.Fields, f)
									rocEqualFields(t, want.Fields, e.Session)
									mutateRTPSQuery(f)
									mutateRTPSQuery(e.Session)
									testNativeObservationSessionQueries(t, e, want.Fields)
								}
							}
						})
					}
				}
			}
		})
	}
}
func rtps13Case(t *testing.T, id string) ([]byte, rtpsControlAnswer) {
	t.Helper()
	for _, c := range rtps13Cases(t) {
		if c.ID == id {
			return rtps13Input(t, c)
		}
	}
	t.Fatalf("missing%s", id)
	return nil, rtpsControlAnswer{}
}
func TestRTPS13WireResourceAndRecovery(t *testing.T) {
	_, normal := rtps13Case(t, "le-data-hFalseqFalsedFalse")
	_, qos := rtps13Case(t, "le-parameters-size1")
	_, bitmap := rtps13Case(t, "le-bitmap-unused-bits")
	_, big := rtps13Case(t, "le-data-hTrueqTruedTrue")
	for _, tc := range []struct {
		name                    string
		a                       rtpsControlAnswer
		depth, elements, delta  int
		limitBytes, limitMemory bool
		success                 bool
	}{
		{"depth-low", normal, 3, 4096, 0, false, false, false}, {"depth-exact", normal, 4, 4096, 0, false, false, true}, {"qos-depth-low", qos, 4, 4096, 0, false, false, false}, {"qos-depth-exact", qos, 5, 4096, 0, false, false, true},
		{"qos-count-low", qos, 64, 3, 0, false, false, false}, {"qos-count-exact", qos, 64, 4, 0, false, false, true},
		{"bitmap-count-low", bitmap, 64, 2, 0, false, false, false}, {"bitmap-count-exact", bitmap, 64, 3, 0, false, false, true},
		{"wire-low", big, 64, 4096, -1, true, false, false}, {"wire-exact", big, 64, 4096, 0, true, false, true},
		{"memory-low", normal, 64, 4096, -1, false, true, false}, {"memory-exact", normal, 64, 4096, 0, false, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, err := hex.DecodeString(tc.a.Wires[0])
			require.NoError(t, err)
			b := DefaultParserBudget()
			b.MaxRecursionDepth, b.MaxCollectionElements = tc.depth, tc.elements
			if tc.limitBytes {
				b.MaxFrameBytes, b.MaxMessageBytes = len(w)+tc.delta, len(w)+tc.delta
			}
			if tc.limitMemory {
				b.MaxBufferedBytes = 32768 + 512*len(w) + tc.delta
				b.MaxMessageBytes, b.MaxFrameBytes = 4096, 4096
			}
			s := newRTPS13Session(t, b)
			before := s.Stats()
			p := s.Probe(w)
			require.Equal(t, before, s.Stats())
			verdict := ProbeReject
			if tc.success {
				verdict = ProbeAccept
			}
			require.Equal(t, verdict, p.Verdict)
			r := s.Feed(0, time.Unix(1, 0), w)
			require.Len(t, r.Events, 1)
			require.Equal(t, "rtps13-wire-native", r.Events[0].Profile)
			f, err := r.Events[0].GetFields()
			if tc.success {
				require.NoError(t, err)
				rocEqualFields(t, tc.a.Events[0].Fields, f)
				rocEqualFields(t, tc.a.Events[0].Fields, r.Events[0].Session)
			} else {
				require.Nil(t, f)
				assertOwnedProtocolRefusal(t, r.Events[0], "ResourceExceeded", err)
			}
			require.Zero(t, s.Stats().BufferedBytes)
			require.LessOrEqual(t, s.Stats().PeakBufferedBytes, int64(b.MaxBufferedBytes))
			require.Empty(t, s.Close("end"))
			require.Empty(t, s.Close("again"))
		})
	}
	// A refused stateless datagram cannot install a logical source or timestamp.
	_, a := rtps13Case(t, "le-old-logical-0-data")
	s := newRTPS13Session(t, DefaultParserBudget())
	for i, raw := range a.Wires {
		w, err := hex.DecodeString(raw)
		require.NoError(t, err)
		r := s.Feed(0, time.Unix(int64(i), 0), w)
		f, err := r.Events[0].GetFields()
		if i == 0 {
			require.Nil(t, f)
			assertOwnedProtocolRefusal(t, r.Events[0], "ContextRequired", err)
		} else {
			require.NoError(t, err)
			rocEqualFields(t, a.Events[i].Fields, f)
		}
	}
	s.Close("end")
	require.Zero(t, s.Stats().BufferedBytes)
}
func TestRTPS13WireSharedProjectionBudget(t *testing.T) {
	_, a := rtps13Case(t, "le-data-hFalseqFalsedFalse")
	w, err := hex.DecodeString(a.Wires[0])
	require.NoError(t, err)
	window := 32768 + 512*len(w)
	s := newRTPS13Session(t, ParserBudget{MaxFrameBytes: 4096, MaxMessageBytes: 4096, MaxBufferedBytes: window + 100}).(*captureSession)
	require.True(t, s.f.a.reserveEvidence(101))
	before := s.Stats()
	require.Equal(t, ProbeReject, s.Probe(w).Verdict)
	require.Equal(t, before, s.Stats())
	r := s.Feed(0, time.Unix(1, 0), w)
	f, err := r.Events[0].GetFields()
	require.Nil(t, f)
	assertOwnedProtocolRefusal(t, r.Events[0], "ResourceExceeded", err)
	require.EqualValues(t, 101, s.Stats().BufferedBytes)
	s.f.a.buffered.Add(-101)
	r = s.Feed(0, time.Unix(2, 0), w)
	f, err = r.Events[0].GetFields()
	require.NoError(t, err)
	rocEqualFields(t, a.Events[0].Fields, f)
	require.Empty(t, s.Close("end"))
	require.Zero(t, s.Stats().BufferedBytes)
	require.LessOrEqual(t, s.Stats().PeakBufferedBytes, int64(window+100))
}

func TestRTPS13WireSelectionAndCarrier(t *testing.T) {
	for _, opts := range [][]ProtocolSessionOption{{WithSessionRTPS13Wire(28400)}, {WithSessionTransport("tcp"), WithSessionRTPS13Wire(28400)}, {WithSessionTransport("udp"), WithSessionRTPS13Wire(0)}, {WithSessionTransport("udp"), WithSessionRTPSWire(28400), WithSessionRTPS13Wire(28400)}} {
		_, err := NewProtocolSessionWithOptions(DefaultParserBudget(), opts...)
		require.Error(t, err)
	}
	raw, normal := rtps13Case(t, "le-data-hTrueqTruedTrue")
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(38000, 28400), WithSessionRTPS13Wire(38000), WithSessionRTPSWire(28400))
	require.NoError(t, err)
	w, err := hex.DecodeString(normal.Wires[0])
	require.NoError(t, err)
	r := s.Feed(0, time.Unix(1, 0), w)
	require.Len(t, r.Events, 1)
	require.NotEqual(t, "rtps13-wire-native", r.Events[0].Profile)
	s.Close("end")
	// No automatic legacy admission or cross-datagram join is introduced.
	var events []*ProtocolEvent
	require.NoError(t, ReplayPcap(bytes.NewReader(raw), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) })))
	require.Len(t, events, 1)
	require.NotEqual(t, "rtps13-wire-native", events[0].Profile)
	s = newRTPS13Session(t, DefaultParserBudget())
	r = s.Feed(2, time.Unix(1, 0), w)
	require.Empty(t, r.Events)
	require.Zero(t, r.Consumed)
	rocTypedError(t, "MalformedMessage", r.Err)
	require.Zero(t, s.Stats().Messages)
	require.Empty(t, s.Close("end"))
	// Actual carrier validation precedes interpreting a truncated application.
	for _, tc := range []struct {
		name  string
		alter func([]byte) []byte
		bytes int
		want  string
	}{
		{"capture-truncation", func(p []byte) []byte { return p[:len(p)-1] }, 1 << 20, "NeedMore"},
		{"invalid-udp-length", func(p []byte) []byte { p[24+16+20+4] = 0; p[24+16+20+5] = 7; return p }, 1 << 20, "MalformedMessage"},
		{"message-limit", func(p []byte) []byte { return p }, len(w) - 1, "ResourceExceeded"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := tc.alter(bytes.Clone(raw))
			if tc.name == "capture-truncation" {
				p = bytes.Clone(raw)
				p = p[:len(p)-1] /* preserve reported record length but adjust captured length */
				p[32]--
			}
			var es []*ProtocolEvent
			var stats ProtocolStats
			if tc.name == "capture-truncation" {
				// Strict offline reader rejection is preserved. Independently
				// exercise the live/shared carrier ingress with real decoded
				// truncated packet bytes and their capture metadata.
				require.ErrorContains(t, ReplayPcap(bytes.NewReader(p), WithProtocolDecodeAs("udp", 28400, "rtps13-wire")), "capture contains truncated packets")
				live := newRTPS13Session(t, DefaultParserBudget()).(*captureSession)
				packet := gopacket.NewPacket(p[40:], layers.LayerTypeIPv4, gopacket.Default)
				ip := packet.Layer(layers.LayerTypeIPv4).(*layers.IPv4)
				udp := packet.Layer(layers.LayerTypeUDP).(*layers.UDP)
				live.f.a.datagramFields(ip, udp, gopacket.CaptureInfo{Timestamp: time.Unix(1, 0), CaptureLength: len(p) - 40, Length: len(raw) - 40}, true)
				require.Len(t, live.events, 1)
				e := live.events[0]
				require.Equal(t, "rtps13-wire-native", e.Profile)
				require.Empty(t, e.Raw)
				f, err := e.GetFields()
				require.Nil(t, f)
				assertOwnedProtocolRefusal(t, e, "NeedMore", err)
				require.Empty(t, live.Close("end"))
				require.Zero(t, live.Stats().BufferedBytes)
				return
			}
			require.NoError(t, ReplayPcap(bytes.NewReader(p), WithProtocolDecodeAs("udp", 28400, "rtps13-wire"), WithProtocolBudget(tc.bytes, DefaultParserBudget().MaxBufferedBytes), WithOnProtocolMessage(func(e *ProtocolEvent) { es = append(es, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })))
			require.Len(t, es, 1)
			require.Equal(t, "rtps13-wire-native", es[0].Profile)
			require.Empty(t, es[0].Raw)
			f, err := es[0].GetFields()
			require.Nil(t, f)
			assertOwnedProtocolRefusal(t, es[0], tc.want, err)
			require.Zero(t, stats.BufferedBytes)
		})
	}
}
