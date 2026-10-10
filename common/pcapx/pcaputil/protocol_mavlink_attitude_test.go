package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

type mavlinkAttitudeControl struct {
	ID, Capture, Answer, SHA256, Error, Raw, Profile string
	AnswerSHA                                        string `json:"answer_sha256"`
	InputAlias                                       string `json:"input_alias"`
	Packets                                          int
	Events                                           []struct {
		Raw    string
		Offset uint64
		Fields map[string]any
	}
}

func mavlinkAttitudeControls(t *testing.T) []mavlinkAttitudeControl {
	t.Helper()
	b, err := trafficfixture.ReadFile("mavlink-attitude/controls.json")
	require.NoError(t, err)
	var doc struct {
		Schema string
		Cases  []mavlinkAttitudeControl
	}
	require.NoError(t, json.Unmarshal(b, &doc))
	require.Equal(t, "owned-mavlink-attitude/v1", doc.Schema)
	require.Len(t, doc.Cases, 17)
	all, err := trafficfixture.AllExpectations()
	require.NoError(t, err)
	for _, c := range doc.Cases {
		answer, err := trafficfixture.ReadFile("mavlink-attitude/" + c.Answer)
		require.NoError(t, err)
		require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(answer)))
		var a mavlinkAttitudeControl
		require.NoError(t, json.Unmarshal(answer, &a))
		a.AnswerSHA = c.AnswerSHA
		require.Equal(t, c, a)
		bound := 0
		for _, batch := range all {
			for _, v := range batch.Cases {
				if v.ID != "mavlink-attitude/"+c.ID {
					continue
				}
				require.Equal(t, c.SHA256, v.Input.SHA256)
				require.Equal(t, c.InputAlias, v.Input.OriginalPath)
				require.Equal(t, c.Packets, v.Facts.PacketCount)
				for _, e := range v.Expectations {
					var x struct {
						File string `json:"answer_file"`
						SHA  string `json:"answer_sha256"`
					}
					require.NoError(t, json.Unmarshal(e.PayloadConstraints, &x))
					require.Equal(t, c.Answer, x.File)
					require.Equal(t, c.AnswerSHA, x.SHA)
					bound++
				}
			}
		}
		require.Equal(t, 1, bound, c.ID)
	}
	return doc.Cases
}
func mavlinkAttitudeAssert(t *testing.T, c mavlinkAttitudeControl, events []*ProtocolEvent) {
	t.Helper()
	want := len(c.Events)
	if c.Error != "" {
		want = 1
	}
	require.Len(t, events, want)
	ids := map[uint64]bool{}
	for i, e := range events {
		require.Equal(t, "mavlink", e.Protocol)
		require.Equal(t, c.Profile, e.Profile)
		require.Equal(t, "udp", e.Transport)
		require.Zero(t, e.TransactionID)
		require.Zero(t, e.ResponseTo)
		require.NotZero(t, e.ID)
		require.False(t, ids[e.ID])
		ids[e.ID] = true
		f, err := e.GetFields()
		rocTypedError(t, c.Error, err)
		if c.Error != "" {
			require.Nil(t, f)
			require.Nil(t, e.Session)
			require.Nil(t, e.Structured)
			require.Equal(t, c.Raw, hex.EncodeToString(e.Raw))
			require.Zero(t, e.Offset)
			return
		}
		w := c.Events[i]
		require.Empty(t, e.Error)
		require.Equal(t, "message", e.Completeness)
		require.Equal(t, w.Raw, hex.EncodeToString(e.Raw))
		require.Equal(t, w.Offset, e.Offset)
		require.Equal(t, len(w.Raw)/2, e.Length)
		require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(e.Raw)), e.SourceBytes.SHA256)
		rocEqualFields(t, w.Fields, f)
		rocEqualFields(t, w.Fields, e.Session)
		tree, err := e.Decode()
		require.NoError(t, err)
		rocEqualFields(t, w.Fields, protocolFields(tree))
		// JSON projection must keep non-finite values safe and retain signed-zero bits.
		_, err = json.Marshal(f)
		require.NoError(t, err)
		floats := f["Message Fields"].(map[string]any)
		for _, key := range []string{"roll", "pitch", "yaw", "rollspeed", "pitchspeed", "yawspeed"} {
			x := floats[key].(map[string]any)
			if x["float_class"] == "zero" {
				require.Equal(t, x["negative"], math.Signbit(x["value"].(float64)))
			}
			x["ieee754_bits"] = uint32(0xdeadbeef)
		}
		e.Session["Message Fields"].(map[string]any)["roll"].(map[string]any)["value"] = "mutated"
		if s, ok := f["Signature"].([]byte); ok {
			clear(s)
		}
		again, err := e.GetFields()
		require.NoError(t, err)
		rocEqualFields(t, w.Fields, again)
	}
}
func testMAVLinkAttitudeMatrix(t *testing.T) {
	for _, c := range mavlinkAttitudeControls(t) {
		t.Run(c.ID, func(t *testing.T) {
			raw, err := trafficfixture.ReadFile(strings.TrimPrefix(c.InputAlias, "common/pcapx/pcaputil/"))
			require.NoError(t, err)
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
			for _, workers := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observe := range []bool{false, true} {
						t.Run(fmt.Sprintf("w%d/d%t/o%t", workers, deferred, observe), func(t *testing.T) {
							var events []*ProtocolEvent
							var stats ProtocolStats
							var seen atomic.Int64
							opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
							if observe {
								opts = append(opts, WithEveryPacket(func(p gopacket.Packet) { seen.Add(1) }))
							}
							owned := bytes.Clone(raw)
							require.NoError(t, ReplayPcap(bytes.NewReader(owned), opts...))
							clear(owned)
							mavlinkAttitudeAssert(t, c, events)
							require.EqualValues(t, len(events), stats.Messages)
							require.Zero(t, stats.BufferedBytes)
							require.Zero(t, stats.CallbackPanics)
							if observe {
								require.EqualValues(t, 1, seen.Load())
							}
							for _, e := range events {
								require.Equal(t, time.Unix(1, 0), e.Timestamp)
								require.Equal(t, "192.0.2.1:40000", e.Source)
								require.Equal(t, "192.0.2.2:14550", e.Destination)
								require.Equal(t, "captured", e.SourceBytes.Kind)
								require.Zero(t, e.Direction)
								require.Len(t, e.SourceBytes.PacketRefs, 1)
								require.EqualValues(t, 1, e.SourceBytes.PacketRefs[0].Number)
								require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
							}
						})
					}
				}
			}
		})
	}
}
func testMAVLinkAttitudeBudgets(t *testing.T) {
	controls := mavlinkAttitudeControls(t)
	var c mavlinkAttitudeControl
	for _, v := range controls {
		if v.ID == "copacked-v1-v2" {
			c = v
		}
	}
	raw, err := hex.DecodeString(c.Raw)
	require.NoError(t, err)
	for name, budget := range map[string]ParserBudget{"collection": {MaxCollectionElements: 1}, "frame": {MaxFrameBytes: 20}, "depth": {MaxRecursionDepth: 2}} {
		t.Run(name, func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(budget, WithSessionTransport("udp"), WithSessionPorts(40000, 14550))
			require.NoError(t, err)
			defer s.Close("cleanup")
			out := s.Feed(0, time.Unix(1, 0), raw)
			require.Len(t, out.Events, 1)
			f, err := out.Events[0].GetFields()
			rocTypedError(t, "ResourceExceeded", err)
			require.Nil(t, f)
			require.Nil(t, out.Events[0].Session)
			require.Zero(t, out.Events[0].ResponseTo)
			require.Zero(t, out.Events[0].TransactionID)
			require.Empty(t, s.Close("complete"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
	// At the exact projection boundary both complete messages survive unchanged.
	s, err := NewProtocolSessionWithOptions(ParserBudget{MaxRecursionDepth: 3}, WithSessionTransport("udp"), WithSessionPorts(40000, 14550))
	require.NoError(t, err)
	defer s.Close("cleanup")
	input := bytes.Clone(raw)
	out := s.Feed(0, time.Unix(1, 0), input)
	clear(input)
	mavlinkAttitudeAssert(t, c, out.Events)
	require.Empty(t, s.Close("complete"))
	require.Zero(t, s.Stats().BufferedBytes)
	// No split UDP packet may acquire bytes from its neighbor or expose a prefix.
	for cut := 1; cut < len(raw)/2; cut++ {
		one := raw[:36]
		if cut >= len(one) {
			continue
		}
		s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 14550))
		require.NoError(t, err)
		for _, part := range [][]byte{one[:cut], one[cut:]} {
			out := s.Feed(0, time.Unix(1, 0), part)
			for _, e := range out.Events {
				f, err := e.GetFields()
				require.Error(t, err)
				require.Nil(t, f)
				require.Nil(t, e.Session)
				require.Zero(t, e.ResponseTo)
			}
		}
		require.Empty(t, s.Close("complete"))
		require.Zero(t, s.Stats().BufferedBytes)
	}
}

func testMAVLinkExistingQueries(t *testing.T) {
	wire := mavlinkTestFrame(1, 0, mavlinkTestPayload(0), 0, 50)
	for _, deferred := range []bool{false, true} {
		t.Run(fmt.Sprintf("deferred%t", deferred), func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 14550))
			require.NoError(t, err)
			defer s.Close("cleanup")
			s.(*captureSession).f.a.config.Deferred = deferred
			e := s.Feed(0, time.Unix(1, 0), wire).Events[0]
			f, err := e.GetFields()
			require.NoError(t, err)
			before := cloneSession(f)
			f["Message Fields"].(map[string]any)["custom_mode"] = uint32(999)
			e.Session["Message Fields"].(map[string]any)["custom_mode"] = uint32(888)
			again, err := e.GetFields()
			require.NoError(t, err)
			require.Equal(t, before, again)
			bad := bytes.Clone(wire)
			bad[len(bad)-1] ^= 1
			out := s.Feed(1, time.Unix(2, 0), bad)
			require.Len(t, out.Events, 1)
			f, err = out.Events[0].GetFields()
			rocTypedError(t, "MalformedMessage", err)
			require.Nil(t, f)
			rocTypedError(t, "MalformedMessage", out.Err)
			assertOwnedProtocolRefusal(t, out.Events[0], "MalformedMessage", err)
			require.Zero(t, out.Events[0].TransactionID)
			require.Zero(t, out.Events[0].ResponseTo)
			require.Empty(t, s.Close("complete"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}
