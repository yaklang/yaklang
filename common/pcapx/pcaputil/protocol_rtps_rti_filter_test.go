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
	"strings"
	"testing"
	"time"
)

func rtps20RTIFilterCases(t *testing.T) []rtpsControl {
	root := "rtps20-rti-filter"
	count := 82
	t.Helper()
	var raw []byte
	var err error
	if dir := os.Getenv("RTPS20_RTI_FILTER_TEST_ROOT"); dir != "" {
		raw, err = os.ReadFile(filepath.Join(dir, "manifest.json"))
	} else {
		raw, err = trafficfixture.ReadFile(root + "/manifest.json")
	}
	require.NoError(t, err)
	var m struct {
		SchemaVersion int `json:"schema_version"`
		Cases         []rtpsControl
	}
	require.NoError(t, json.Unmarshal(raw, &m))
	require.Equal(t, 1, m.SchemaVersion)
	require.Len(t, m.Cases, count)
	if os.Getenv("RTPS20_RTI_FILTER_TEST_ROOT") != "" {
		for i := range m.Cases {
			m.Cases[i].Root = "rtps20-rti-filter"
		}
		return m.Cases
	}
	all, err := trafficfixture.AllExpectations()
	require.NoError(t, err)
	found := map[string]trafficfixture.FrozenCase{}
	for _, b := range all {
		for _, c := range b.Cases {
			found[c.ID] = c
		}
	}
	for i := range m.Cases {
		c := &m.Cases[i]
		c.Root = root
		if strings.HasPrefix(c.Capture, "common/") {
			continue
		} // Hash-pinned historical capture reused by this new manifest.
		v, ok := found[root+"/"+c.ID]
		require.True(t, ok)
		require.Equal(t, c.Capture, strings.TrimPrefix(v.Input.File, root+"/"))
		require.Equal(t, c.SHA256, v.Input.SHA256)
		require.Equal(t, c.Packets, v.Facts.PacketCount)
		require.Len(t, v.Expectations, 1)
		var binding struct {
			AnswerFile string `json:"answer_file"`
			AnswerSHA  string `json:"answer_sha256"`
		}
		require.NoError(t, json.Unmarshal(v.Expectations[0].PayloadConstraints, &binding))
		require.Equal(t, c.Answer, binding.AnswerFile)
		require.Equal(t, c.AnswerSHA, binding.AnswerSHA)
	}
	return m.Cases
}
func rtps20RTIFilterInput(t *testing.T, c rtpsControl) ([]byte, rtpsControlAnswer) {
	t.Helper()
	read := func(file string) []byte {
		var b []byte
		var err error
		if root := os.Getenv("RTPS20_RTI_FILTER_TEST_ROOT"); root != "" {
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
func newRTPS20RTIFilterSession(t *testing.T, b ParserBudget) ProtocolSession {
	t.Helper()
	s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(38000, 28400), WithSessionRTPS20RTIFilter(28400))
	require.NoError(t, err)
	return s
}
func TestRTPS20RTIFilterSealedSession(t *testing.T) {
	for _, c := range rtps20RTIFilterCases(t) {
		t.Run(c.ID, func(t *testing.T) {
			_, a := rtps20RTIFilterInput(t, c)
			s := newRTPS20RTIFilterSession(t, DefaultParserBudget())
			for i, encoded := range a.Wires {
				w, err := hex.DecodeString(encoded)
				require.NoError(t, err)
				pure, err := decodeRTPS20RTIFilter(w, DefaultParserBudget())
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
				require.Equal(t, "rtps20-rti-filter-native", e.Profile)
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
func TestRTPS20RTIFilterSealedReplay(t *testing.T) {
	for _, c := range rtps20RTIFilterCases(t) {
		t.Run(c.ID, func(t *testing.T) {
			raw, a := rtps20RTIFilterInput(t, c)
			for _, w := range []int{1, 2, 4} {
				for _, d := range []bool{false, true} {
					for _, o := range []bool{false, true} {
						t.Run(fmt.Sprintf("w%d/deferred%v/observer%v", w, d, o), func(t *testing.T) {
							var events []*ProtocolEvent
							var stats ProtocolStats
							opts := []CaptureOption{WithTCPReassemblyWorkers(w), WithProtocolDeferred(d), WithProtocolDecodeAs("udp", 28400, "rtps20-rti-filter"), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
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
								require.Equal(t, "rtps20-rti-filter-native", e.Profile)
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

func rtps20RTIFilterCase(t *testing.T, id string) ([]byte, rtpsControlAnswer) {
	t.Helper()
	for _, c := range rtps20RTIFilterCases(t) {
		if c.ID == id {
			return rtps20RTIFilterInput(t, c)
		}
	}
	t.Fatalf("missing %s", id)
	return nil, rtpsControlAnswer{}
}
func TestRTPS20RTIFilterSharedBudgetsAndRecovery(t *testing.T) {
	_, normal := rtps20RTIFilterCase(t, "le-zero-channels")
	_, input := rtps20RTIFilterCase(t, "le-received-layout")
	_, repeated := rtps20RTIFilterCase(t, "le-repeated")
	for _, tc := range []struct {
		name            string
		input           rtpsControlAnswer
		depth, elements int
	}{{"locator-depth", input, 9, 1000}, {"channel-depth", input, 7, 1000}, {"shared-elements", input, 64, 4}, {"shared-repeated", repeated, 64, 7}} {
		t.Run(tc.name, func(t *testing.T) {
			b := DefaultParserBudget()
			b.MaxRecursionDepth = tc.depth
			b.MaxCollectionElements = tc.elements
			s := newRTPS20RTIFilterSession(t, b)
			w, err := hex.DecodeString(tc.input.Wires[0])
			require.NoError(t, err)
			before := s.Stats()
			require.Equal(t, ProbeReject, s.Probe(w).Verdict)
			require.Equal(t, before, s.Stats())
			r := s.Feed(0, time.Unix(1, 0), w)
			require.Len(t, r.Events, 1)
			f, err := r.Events[0].GetFields()
			require.Nil(t, f)
			assertOwnedProtocolRefusal(t, r.Events[0], "ResourceExceeded", err)
			require.Nil(t, r.Events[0].Session)
			require.Zero(t, s.Stats().BufferedBytes)
			w, err = hex.DecodeString(normal.Wires[0])
			require.NoError(t, err)
			r = s.Feed(1, time.Unix(2, 0), w)
			f, err = r.Events[0].GetFields()
			require.NoError(t, err)
			rocEqualFields(t, normal.Events[0].Fields, f)
			rocEqualFields(t, normal.Events[0].Fields, r.Events[0].Session)
			require.Zero(t, r.Events[0].ResponseTo)
			require.Zero(t, r.Events[0].TransactionID)
			require.Empty(t, s.Close("end"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
	b := DefaultParserBudget()
	b.MaxCollectionElements = 5
	s := newRTPS20RTIFilterSession(t, b)
	w, err := hex.DecodeString(input.Wires[0])
	require.NoError(t, err)
	r := s.Feed(0, time.Unix(1, 0), w)
	f, err := r.Events[0].GetFields()
	require.NoError(t, err)
	rocEqualFields(t, input.Events[0].Fields, f)
	require.Empty(t, s.Close("end"))
	require.Zero(t, s.Stats().BufferedBytes)
}
func TestRTPS20RTIFilterSharedProjectionBudget(t *testing.T) {
	_, a := rtps20RTIFilterCase(t, "le-received-layout")
	w, err := hex.DecodeString(a.Wires[0])
	require.NoError(t, err)
	window := 32768 + 512*len(w)
	s := newRTPS20RTIFilterSession(t, ParserBudget{MaxFrameBytes: 4096, MaxMessageBytes: 4096, MaxBufferedBytes: window + 100}).(*captureSession)
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

func TestRTPS20RTIFilterSelectionAndCarrier(t *testing.T) {
	for _, opts := range [][]ProtocolSessionOption{{WithSessionRTPS20RTIFilter(28400)}, {WithSessionTransport("tcp"), WithSessionRTPS20RTIFilter(28400)}, {WithSessionTransport("udp"), WithSessionRTPS20RTIFilter(0)}, {WithSessionTransport("udp"), WithSessionRTPSWire(28400), WithSessionRTPS20RTIFilter(28400)}} {
		_, err := NewProtocolSessionWithOptions(DefaultParserBudget(), opts...)
		require.Error(t, err)
	}
	raw, normal := rtps20RTIFilterCase(t, "le-received-layout")
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(38000, 28400), WithSessionRTPS20RTIFilter(38000), WithSessionRTPSWire(28400))
	require.NoError(t, err)
	w, err := hex.DecodeString(normal.Wires[0])
	require.NoError(t, err)
	r := s.Feed(0, time.Unix(1, 0), w)
	require.Len(t, r.Events, 1)
	require.NotEqual(t, "rtps20-rti-filter-native", r.Events[0].Profile)
	s.Close("end")
	// No automatic legacy admission or cross-datagram join is introduced.
	var events []*ProtocolEvent
	require.NoError(t, ReplayPcap(bytes.NewReader(raw), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) })))
	require.Len(t, events, 1)
	require.NotEqual(t, "rtps20-rti-filter-native", events[0].Profile)
	s = newRTPS20RTIFilterSession(t, DefaultParserBudget())
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
		{"invalid-udp-length", func(p []byte) []byte { p[24+16+20+4] = 0; p[24+16+20+5] = 7; return p }, 1 << 20, "NeedMore"},
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
				require.ErrorContains(t, ReplayPcap(bytes.NewReader(p), WithProtocolDecodeAs("udp", 28400, "rtps20-rti-filter")), "capture contains truncated packets")
				live := newRTPS20RTIFilterSession(t, DefaultParserBudget()).(*captureSession)
				packet := gopacket.NewPacket(p[40:], layers.LayerTypeIPv4, gopacket.Default)
				ip := packet.Layer(layers.LayerTypeIPv4).(*layers.IPv4)
				udp := packet.Layer(layers.LayerTypeUDP).(*layers.UDP)
				live.f.a.datagramFields(ip, udp, gopacket.CaptureInfo{Timestamp: time.Unix(1, 0), CaptureLength: len(p) - 40, Length: len(raw) - 40}, true)
				require.Len(t, live.events, 1)
				e := live.events[0]
				require.Equal(t, "rtps20-rti-filter-native", e.Profile)
				require.Empty(t, e.Raw)
				f, err := e.GetFields()
				require.Nil(t, f)
				assertOwnedProtocolRefusal(t, e, "NeedMore", err)
				require.Empty(t, live.Close("end"))
				require.Zero(t, live.Stats().BufferedBytes)
				return
			}
			require.NoError(t, ReplayPcap(bytes.NewReader(p), WithProtocolDecodeAs("udp", 28400, "rtps20-rti-filter"), WithProtocolBudget(tc.bytes, DefaultParserBudget().MaxBufferedBytes), WithOnProtocolMessage(func(e *ProtocolEvent) { es = append(es, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })))
			require.Len(t, es, 1)
			require.Equal(t, "rtps20-rti-filter-native", es[0].Profile)
			require.Empty(t, es[0].Raw)
			f, err := es[0].GetFields()
			require.Nil(t, f)
			assertOwnedProtocolRefusal(t, es[0], tc.want, err)
			require.Zero(t, stats.BufferedBytes)
		})
	}
}
