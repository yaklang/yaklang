package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

// UseNumber is essential: GPS_RAW_INT's time_usec exceeds binary64's exact
// integer range. JSONEq alone could accept a silently rounded timestamp.
func mavlinkGPSEqual(t *testing.T, want, got map[string]any) {
	t.Helper()
	normalize := func(v map[string]any) any {
		b, err := json.Marshal(v)
		require.NoError(t, err)
		d := json.NewDecoder(bytes.NewReader(b))
		d.UseNumber()
		var x any
		require.NoError(t, d.Decode(&x))
		return x
	}
	require.Equal(t, normalize(want), normalize(got))
}

func mavlinkGPSControls(t *testing.T) []mavlinkAttitudeControl {
	t.Helper()
	b, err := trafficfixture.ReadFile("mavlink-gps/controls.json")
	require.NoError(t, err)
	var doc struct {
		Schema string
		Cases  []mavlinkAttitudeControl
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	require.NoError(t, d.Decode(&doc))
	require.Equal(t, "owned-mavlink-gps/v1", doc.Schema)
	require.Len(t, doc.Cases, 28)
	all, err := trafficfixture.AllExpectations()
	require.NoError(t, err)
	for _, c := range doc.Cases {
		b, err := trafficfixture.ReadFile("mavlink-gps/" + c.Answer)
		require.NoError(t, err)
		require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(b)))
		var answer mavlinkAttitudeControl
		d := json.NewDecoder(bytes.NewReader(b))
		d.UseNumber()
		require.NoError(t, d.Decode(&answer))
		answer.AnswerSHA = c.AnswerSHA
		require.Equal(t, c, answer)
		bound := 0
		for _, batch := range all {
			for _, v := range batch.Cases {
				if v.ID != "mavlink-gps/"+c.ID {
					continue
				}
				require.Equal(t, c.SHA256, v.Input.SHA256)
				require.Equal(t, c.InputAlias, v.Input.OriginalPath)
				require.Equal(t, c.Packets, v.Facts.PacketCount)
				for _, e := range v.Expectations {
					var ref struct {
						File string `json:"answer_file"`
						SHA  string `json:"answer_sha256"`
					}
					require.NoError(t, json.Unmarshal(e.PayloadConstraints, &ref))
					require.Equal(t, c.Answer, ref.File)
					require.Equal(t, c.AnswerSHA, ref.SHA)
					bound++
				}
			}
		}
		require.Equal(t, 1, bound, c.ID)
	}
	return doc.Cases
}

func mavlinkGPSAssert(t *testing.T, c mavlinkAttitudeControl, events []*ProtocolEvent) {
	t.Helper()
	n := len(c.Events)
	if c.Error != "" {
		n = 1
	}
	require.Len(t, events, n)
	ids := map[uint64]bool{}
	for i, e := range events {
		require.Equal(t, "mavlink", e.Protocol)
		require.Equal(t, c.Profile, e.Profile)
		require.Equal(t, "udp", e.Transport)
		require.Zero(t, e.ResponseTo)
		require.Zero(t, e.TransactionID)
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
			assertOwnedProtocolRefusal(t, e, c.Error, err)
			continue
		}
		w := c.Events[i]
		require.Empty(t, e.Error)
		require.Equal(t, "message", e.Completeness)
		require.Equal(t, w.Raw, hex.EncodeToString(e.Raw))
		require.Equal(t, w.Offset, e.Offset)
		require.Equal(t, len(w.Raw)/2, e.Length)
		require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(e.Raw)), e.SourceBytes.SHA256)
		mavlinkGPSEqual(t, w.Fields, f)
		mavlinkGPSEqual(t, w.Fields, e.Session)
		tree, err := e.Decode()
		require.NoError(t, err)
		mavlinkGPSEqual(t, w.Fields, protocolFields(tree))
		protocolFields(tree)["Message Fields"].(map[string]any)["alt"] = int32(1)
		wantTime, err := strconv.ParseUint(string(w.Fields["Message Fields"].(map[string]any)["time_usec"].(json.Number)), 10, 64)
		require.NoError(t, err)
		mf := f["Message Fields"].(map[string]any)
		require.IsType(t, uint64(0), mf["time_usec"])
		require.Equal(t, wantTime, mf["time_usec"])
		mf["time_usec"], mf["lat"] = uint64(1), int32(1)
		e.Session["Message Fields"].(map[string]any)["lon"] = int32(1)
		if s, ok := f["Signature"].([]byte); ok {
			clear(s)
		}
		again, err := e.GetFields()
		require.NoError(t, err)
		mavlinkGPSEqual(t, w.Fields, again)
	}
}

func testMAVLinkGPSMatrix(t *testing.T) {
	for _, c := range mavlinkGPSControls(t) {
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
							input := bytes.Clone(raw)
							require.NoError(t, ReplayPcap(bytes.NewReader(input), opts...))
							clear(input)
							mavlinkGPSAssert(t, c, events)
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
								require.Zero(t, e.Direction)
								require.Equal(t, "captured", e.SourceBytes.Kind)
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

func testMAVLinkGPSBudgets(t *testing.T) {
	var normal, packed mavlinkAttitudeControl
	for _, c := range mavlinkGPSControls(t) {
		if c.ID == "v2-signed-position" {
			normal = c
		}
		if c.ID == "copacked-v1-v2" {
			packed = c
		}
	}
	wire, err := hex.DecodeString(normal.Raw)
	require.NoError(t, err)
	copacked, err := hex.DecodeString(packed.Raw)
	require.NoError(t, err)
	for name, budget := range map[string]ParserBudget{"frame": {MaxFrameBytes: len(wire) - 1}, "collection": {MaxCollectionElements: 1}, "depth": {MaxRecursionDepth: 1}} {
		t.Run(name, func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(budget, WithSessionTransport("udp"), WithSessionPorts(40000, 14550))
			require.NoError(t, err)
			defer s.Close("cleanup")
			out := s.Feed(0, time.Unix(1, 0), copacked)
			require.Len(t, out.Events, 1)
			rocTypedError(t, "ResourceExceeded", out.Err)
			f, err := out.Events[0].GetFields()
			rocTypedError(t, "ResourceExceeded", err)
			require.Nil(t, f)
			require.Nil(t, out.Events[0].Session)
			require.Zero(t, out.Events[0].TransactionID)
			require.Zero(t, out.Events[0].ResponseTo)
			require.Empty(t, s.Close("complete"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
	// The exact scalar-field projection depth admits both complete frames.
	s, err := NewProtocolSessionWithOptions(ParserBudget{MaxRecursionDepth: 2, MaxCollectionElements: 2, MaxFrameBytes: len(wire)}, WithSessionTransport("udp"), WithSessionPorts(40000, 14550))
	require.NoError(t, err)
	defer s.Close("cleanup")
	out := s.Feed(0, time.Unix(1, 0), copacked)
	require.Nil(t, out.Err)
	mavlinkGPSAssert(t, packed, out.Events)
	require.Empty(t, s.Close("complete"))
	require.Zero(t, s.Stats().BufferedBytes)
	for cut := 1; cut < len(wire); cut++ {
		t.Run(fmt.Sprintf("datagram-cut-%d", cut), func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 14550))
			require.NoError(t, err)
			defer s.Close("cleanup")
			for _, part := range [][]byte{wire[:cut], wire[cut:]} {
				for _, e := range s.Feed(0, time.Unix(1, 0), part).Events {
					f, err := e.GetFields()
					require.Error(t, err)
					require.Nil(t, f)
					require.Nil(t, e.Session)
					require.Zero(t, e.ResponseTo)
					require.Zero(t, e.TransactionID)
				}
			}
			owned := bytes.Clone(wire)
			out := s.Feed(1, time.Unix(2, 0), owned)
			clear(owned)
			mavlinkGPSAssert(t, normal, out.Events)
			require.Empty(t, s.Close("complete"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}
