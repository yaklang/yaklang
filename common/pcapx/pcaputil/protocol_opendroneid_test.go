package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

func odidControls(t *testing.T) []bsapControl {
	t.Helper()
	b, err := trafficfixture.ReadFile("opendroneid-v2/manifest.json")
	require.NoError(t, err)
	var m struct {
		Schema string
		Cases  []bsapControl
	}
	require.NoError(t, json.Unmarshal(b, &m))
	require.Equal(t, "pr5013-opendroneid-selected-v2/v1", m.Schema)
	require.Len(t, m.Cases, 63)
	all, err := trafficfixture.AllExpectations()
	require.NoError(t, err)
	bound := map[string]trafficfixture.FrozenCase{}
	for _, batch := range all {
		for _, c := range batch.Cases {
			bound[c.ID] = c
		}
	}
	for _, c := range m.Cases {
		v, ok := bound["opendroneid-v2/"+c.ID]
		require.True(t, ok)
		require.Equal(t, c.Capture, v.Input.File)
		require.Equal(t, c.SHA256, v.Input.SHA256)
		require.Equal(t, c.Packets, v.Facts.PacketCount)
		require.Len(t, v.Expectations, 1)
		var a struct {
			File string `json:"answer_file"`
			SHA  string `json:"answer_sha256"`
		}
		require.NoError(t, json.Unmarshal(v.Expectations[0].PayloadConstraints, &a))
		require.Equal(t, c.Answer, a.File)
		require.Equal(t, c.AnswerSHA, a.SHA)
	}
	return m.Cases
}
func odidInput(t *testing.T, c bsapControl) ([]byte, bsapAnswer) {
	t.Helper()
	b, err := trafficfixture.ReadFile("opendroneid-v2/" + c.Capture)
	require.NoError(t, err)
	require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(b)))
	ab, err := trafficfixture.ReadFile("opendroneid-v2/" + c.Answer)
	require.NoError(t, err)
	require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(ab)))
	var a bsapAnswer
	require.NoError(t, json.Unmarshal(ab, &a))
	require.Equal(t, c.Packets, a.Packets)
	require.Len(t, a.Events, c.Packets)
	r, err := NewCaptureReader(bytes.NewReader(b))
	require.NoError(t, err)
	for _, e := range a.Events {
		wire, ci, err := r.ReadPacketData()
		require.NoError(t, err)
		require.Equal(t, len(wire), ci.CaptureLength)
		require.Equal(t, ci.Length, ci.CaptureLength)
		p := gopacket.NewPacket(wire, r.LinkType(), gopacket.Default)
		u, ok := p.Layer(layers.LayerTypeUDP).(*layers.UDP)
		require.True(t, ok)
		require.Equal(t, e.Raw, hex.EncodeToString(u.Payload))
	}
	_, _, err = r.ReadPacketData()
	require.ErrorIs(t, err, io.EOF)
	return b, a
}
func TestOpenDroneIDSealedReplay(t *testing.T) {
	for _, c := range odidControls(t) {
		t.Run(c.ID, func(t *testing.T) {
			raw, want := odidInput(t, c)
			for _, workers := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observe := range []bool{false, true} {
						t.Run(fmt.Sprintf("w%d/d%t/o%t", workers, deferred, observe), func(t *testing.T) {
							var events []*ProtocolEvent
							var stats ProtocolStats
							var asm TCPReassemblyStats
							var seen atomic.Int64
							opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithProtocolDecodeAs("udp", 28400, "opendroneid"), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s }), WithOnTCPReassemblyStats(func(s TCPReassemblyStats) { asm = s })}
							if observe {
								opts = append(opts, WithEveryPacket(func(p gopacket.Packet) { require.NotEmpty(t, p.Data()); seen.Add(1) }))
							}
							owned := bytes.Clone(raw)
							require.NoError(t, ReplayPcap(bytes.NewReader(owned), opts...))
							clear(owned)
							require.Len(t, events, len(want.Events))
							require.Zero(t, stats.BufferedBytes)
							require.Zero(t, stats.CallbackPanics)
							require.Zero(t, asm.UnreassembledBytes)
							require.EqualValues(t, c.Packets, asm.CapturedPackets)
							if observe {
								require.EqualValues(t, c.Packets, seen.Load())
							}
							for i, e := range events {
								w := want.Events[i]
								require.Equal(t, "opendroneid", e.Protocol)
								require.Equal(t, "opendroneid-v2-basic-location", e.Profile)
								require.Equal(t, "udp", e.Transport)
								require.Equal(t, "explicit-decode-as", e.Admission)
								require.Zero(t, e.FlowID)
								require.Zero(t, e.ResponseTo)
								require.Zero(t, e.TransactionID)
								require.Zero(t, e.Direction) // No requester or aircraft role is inferred.
								src, dst := "192.0.2.10:39200", "192.0.2.20:28400"
								if w.Direction == 1 {
									src, dst = dst, src
								}
								require.Equal(t, src, e.Source)
								require.Equal(t, dst, e.Destination)
								require.Equal(t, w.Raw, hex.EncodeToString(e.Raw))
								require.Len(t, e.SourceBytes.PacketRefs, 1)
								require.EqualValues(t, i+1, e.SourceBytes.PacketRefs[0].Number)
								require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
								if w.Fields == nil {
									rocTypedError(t, w.Error, e.sessionError)
									require.NotEmpty(t, e.Error)
									require.Nil(t, e.Session)
									require.Nil(t, e.Structured)
									require.Nil(t, e.Fields)
									_, err := e.GetFields()
									require.Error(t, err)
									rocTypedError(t, w.Error, err)
									require.Equal(t, e.Status, e.Completeness)
								} else {
									status := "decoded"
									if deferred {
										status = "deferred"
									}
									require.Equal(t, status, e.Status)
									require.Equal(t, "message", e.Completeness)
									require.Empty(t, e.Error)
									rocEqualFields(t, w.Fields, e.Session)
									f, err := e.GetFields()
									require.NoError(t, err)
									rocEqualFields(t, w.Fields, f)
									ms := f["Messages"].([]map[string]any)
									ms[0]["Raw"].([]byte)[0] ^= 255
									f["Message Count"] = 999
									d, err := e.Decode()
									require.NoError(t, err)
									rocEqualFields(t, w.Fields, protocolFields(d))
									rocEqualFields(t, w.Fields, e.Session)
								}
							}
						})
					}
				}
			}
		})
	}
}
func TestOpenDroneIDBoundsOwnershipAndAdmission(t *testing.T) {
	var basic, pack []byte
	for _, c := range odidControls(t) {
		cap, a := odidInput(t, c)
		if c.ID == "basic-serial" {
			basic, _ = hex.DecodeString(a.Events[0].Raw)
		}
		if c.ID == "pack-two-basic-location" {
			pack, _ = hex.DecodeString(a.Events[0].Raw)
		}
		if a.Events[0].Fields == nil {
			continue
		}
		// This UDP test carrier has no content signature that can safely identify
		// an aircraft. No automatic OpenDroneID admission, even on the test port.
		var ev []*ProtocolEvent
		require.NoError(t, ReplayPcap(bytes.NewReader(cap), WithOnProtocolMessage(func(e *ProtocolEvent) { ev = append(ev, e) })))
		for _, e := range ev {
			require.NotEqual(t, "opendroneid", e.Protocol)
		}
	}
	require.Len(t, basic, 25)
	require.Len(t, pack, 78)
	for _, tc := range []struct {
		raw             []byte
		bytes, elements int
	}{{basic, 24, 9}, {basic, 25, 0}, {pack, 78, 2}} {
		f, err := decodeOpenDroneID(tc.raw, tc.bytes, tc.elements)
		rocTypedError(t, "ResourceExceeded", err)
		require.Nil(t, f)
	}
	f, err := decodeOpenDroneID(basic, 25, 1)
	require.NoError(t, err)
	original := bytes.Clone(basic)
	clear(basic)
	require.Equal(t, original, f["Messages"].([]map[string]any)[0]["Raw"])
	// A malformed later message cannot expose a successfully decoded prefix.
	bad := bytes.Clone(pack)
	bad[53] = 0x22
	f, err = decodeOpenDroneID(bad, len(bad), 9)
	rocTypedError(t, "UnsupportedFeature", err)
	require.Nil(t, f)
	s, err := NewProtocolSessionWithOptions(ParserBudget{MaxFrameBytes: 128, MaxMessageBytes: 128, MaxBufferedBytes: 700, ProbeBytes: 16}, WithSessionTransport("udp"), WithSessionPorts(39200, 28400))
	require.NoError(t, err)
	cs := s.(*captureSession)
	cs.f.a.datagramDecodeAs[28400] = "opendroneid"
	hold := &binFlow{a: cs.f.a}
	require.NoError(t, hold.reserveSession(64))
	g := s.Feed(0, time.Unix(1700041000, 0), original)
	rocTypedError(t, "ResourceExceeded", g.Err)
	require.Len(t, g.Events, 1)
	require.Nil(t, g.Events[0].Session)
	require.Zero(t, g.Events[0].ResponseTo)
	require.EqualValues(t, 64, s.Stats().BufferedBytes)
	hold.closeSession()
	require.Zero(t, s.Stats().BufferedBytes)
	g = s.Feed(1, time.Unix(1700041001, 0), original)
	require.Nil(t, g.Err)
	require.Len(t, g.Events, 1)
	require.Zero(t, g.Events[0].FlowID)
	require.Zero(t, g.Events[0].ResponseTo)
	require.Zero(t, s.Stats().BufferedBytes)
	require.LessOrEqual(t, s.Stats().PeakBufferedBytes, int64(700))
	for _, reason := range []string{"complete", "idempotent"} {
		s.Close(reason)
		require.Zero(t, s.Stats().BufferedBytes)
	}
}
