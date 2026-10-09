package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
	"io"
	"sync/atomic"
	"testing"
)

func industrialControls(t *testing.T) []bsapControl {
	t.Helper()
	b, err := trafficfixture.ReadFile("industrial-link-core/manifest.json")
	require.NoError(t, err)
	var m struct {
		Schema string
		Cases  []bsapControl
	}
	require.NoError(t, json.Unmarshal(b, &m))
	require.Equal(t, "pr5013-link-and-generation/v1", m.Schema)
	require.Len(t, m.Cases, 61)
	all, err := trafficfixture.AllExpectations()
	require.NoError(t, err)
	bound := map[string]trafficfixture.FrozenCase{}
	for _, batch := range all {
		for _, c := range batch.Cases {
			bound[c.ID] = c
		}
	}
	for _, c := range m.Cases {
		v, ok := bound["industrial-link-core/"+c.ID]
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
func industrialInput(t *testing.T, id string) ([]byte, []byte) {
	t.Helper()
	for _, c := range industrialControls(t) {
		if c.ID == id {
			b, e := trafficfixture.ReadFile("industrial-link-core/" + c.Capture)
			require.NoError(t, e)
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(b)))
			a, e := trafficfixture.ReadFile("industrial-link-core/" + c.Answer)
			require.NoError(t, e)
			require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(a)))
			return b, a
		}
	}
	t.Fatalf("unbound industrial input %s", id)
	return nil, nil
}
func TestIndustrialLinkSealedReplay(t *testing.T) {
	count := 0
	for _, c := range industrialControls(t) {
		if c.Packets != 1 {
			continue
		}
		count++
		t.Run(c.ID, func(t *testing.T) {
			raw, ab := industrialInput(t, c.ID)
			var a struct {
				Fields      map[string]any
				Error, Wire string
			}
			require.NoError(t, json.Unmarshal(ab, &a))
			w, err := hex.DecodeString(a.Wire)
			require.NoError(t, err)
			r, err := NewCaptureReader(bytes.NewReader(raw))
			require.NoError(t, err)
			frame, ci, err := r.ReadPacketData()
			require.NoError(t, err)
			require.Equal(t, ci.CaptureLength, ci.Length)
			require.Equal(t, w, frame[14:])
			_, _, err = r.ReadPacketData()
			require.ErrorIs(t, err, io.EOF)
			protocol, profile, rule, entry := "sv", "iec61850-sv-raw-asdu", "iec61850", "SampledValues"
			if binary.BigEndian.Uint16(frame[12:]) == 0x88a4 {
				protocol, profile, rule, entry = "ethercat", "ethercat-datagram-chain", "ethercat", "EtherCAT"
			}
			for _, workers := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observe := range []bool{false, true} {
						t.Run(fmt.Sprintf("w%d/d%t/o%t", workers, deferred, observe), func(t *testing.T) {
							var events []*ProtocolEvent
							var stats ProtocolStats
							var seen atomic.Int64
							opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
							if observe {
								opts = append(opts, WithEveryPacket(func(p gopacket.Packet) { require.Equal(t, frame, p.Data()); seen.Add(1) }))
							}
							owned := bytes.Clone(raw)
							require.NoError(t, ReplayPcap(bytes.NewReader(owned), opts...))
							clear(owned)
							require.Len(t, events, 1)
							e := events[0]
							require.Equal(t, protocol, e.Protocol)
							require.Equal(t, profile, e.Profile)
							require.Equal(t, rule, e.Rule)
							require.Equal(t, entry, e.Entry)
							require.Equal(t, "ethernet", e.Transport)
							require.Zero(t, e.FlowID)
							require.Zero(t, e.ResponseTo)
							require.Zero(t, e.TransactionID)
							require.Equal(t, "02:00:00:00:00:01", e.Source)
							require.Equal(t, "01:0c:cd:04:00:01", e.Destination)
							require.Equal(t, "captured", e.SourceBytes.Kind)
							require.Len(t, e.SourceBytes.PacketRefs, 1)
							require.EqualValues(t, 1, e.SourceBytes.PacketRefs[0].Number)
							require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
							require.EqualValues(t, 1, stats.Messages)
							require.Zero(t, stats.BufferedBytes)
							require.Zero(t, stats.CallbackPanics)
							if observe {
								require.EqualValues(t, 1, seen.Load())
							}
							f, err := e.GetFields()
							if a.Error != "" {
								rocTypedError(t, a.Error, err)
								rocTypedError(t, a.Error, e.sessionError)
								require.Nil(t, f)
								require.Nil(t, e.Session)
								require.Equal(t, e.Status, e.Completeness)
								if a.Error == "NeedMore" {
									require.EqualValues(t, 1, stats.Incomplete)
									require.Zero(t, stats.Malformed)
								}
								return
							}
							require.NoError(t, err)
							require.Empty(t, e.Error)
							require.Equal(t, "message", e.Completeness)
							rocEqualFields(t, a.Fields, f)
							rocEqualFields(t, a.Fields, e.Session)
							length := int(binary.BigEndian.Uint16(w[2:]))
							if protocol == "ethercat" {
								length = int(binary.LittleEndian.Uint16(w)&2047) + 2
							}
							require.Equal(t, w[:length], e.Raw)
							f["Length"] = 0
							if rows, ok := f["ASDUs"].([]map[string]any); ok && len(rows) > 0 {
								rows[0]["Sample Counter"] = 0
							}
							if rows, ok := f["Datagrams"].([]map[string]any); ok && len(rows) > 0 {
								clear(rows[0]["Data"].([]byte))
							}
							e.Session["Length"] = 0
							again, err := e.GetFields()
							require.NoError(t, err)
							rocEqualFields(t, a.Fields, again)
							tree, err := e.Decode()
							require.NoError(t, err)
							rocEqualFields(t, a.Fields, protocolFields(tree))
						})
					}
				}
			}
		})
	}
	require.Equal(t, 45, count)
}
func TestIndustrialLinkBudgetsAndVLAN(t *testing.T) {
	for _, id := range []string{"sv-two-ASDU", "ecat-logical-roundtrip-chain"} {
		raw, _ := industrialInput(t, id)
		r, err := NewCaptureReader(bytes.NewReader(raw))
		require.NoError(t, err)
		frame, _, err := r.ReadPacketData()
		require.NoError(t, err)
		kind := layers.EthernetType(binary.BigEndian.Uint16(frame[12:]))
		wire := bytes.Clone(frame[14:])
		for _, budget := range []ParserBudget{{MaxBufferedBytes: 512, MaxMessageBytes: 128, MaxFrameBytes: 128}, {MaxCollectionElements: 1}, {MaxRecursionDepth: 1}, {MaxFrameBytes: 12}} {
			t.Run(fmt.Sprintf("%s/%+v", id, budget), func(t *testing.T) {
				var stats ProtocolStats
				session, err := NewProtocolSession(budget)
				require.NoError(t, err)
				a := session.(*captureSession).f.a
				var events []*ProtocolEvent
				a.config.OnEvent = func(e *ProtocolEvent) { events = append(events, e) }
				a.decodeIndustrialEthernet(&layers.Ethernet{SrcMAC: frame[6:12], DstMAC: frame[:6]}, kind, wire, captureEvidence{}, gopacket.CaptureInfo{})
				session.Close("test")
				session.Close("idempotent")
				stats = session.Stats()
				require.Len(t, events, 1)
				rocTypedError(t, "ResourceExceeded", events[0].sessionError)
				require.Nil(t, events[0].Session)
				require.Zero(t, stats.BufferedBytes)
				if budget.MaxBufferedBytes > 0 {
					require.LessOrEqual(t, stats.PeakBufferedBytes, int64(budget.MaxBufferedBytes))
					require.Empty(t, events[0].Raw)
				}
			})
		}
		t.Run(id+"/oversized-link-padding", func(t *testing.T) {
			session, err := NewProtocolSession(ParserBudget{MaxMessageBytes: 128, MaxFrameBytes: 128})
			require.NoError(t, err)
			a := session.(*captureSession).f.a
			var event *ProtocolEvent
			a.config.OnEvent = func(e *ProtocolEvent) { event = e }
			a.decodeIndustrialEthernet(&layers.Ethernet{SrcMAC: frame[6:12], DstMAC: frame[:6]}, kind, append(bytes.Clone(wire), make([]byte, 129)...), captureEvidence{}, gopacket.CaptureInfo{})
			session.Close("test")
			require.NotNil(t, event)
			require.NotNil(t, event.sessionError)
			rocTypedError(t, "ResourceExceeded", event.sessionError)
			require.Nil(t, event.Session)
			require.Empty(t, event.Raw)
			require.Zero(t, session.Stats().PeakBufferedBytes)
			require.Zero(t, session.Stats().BufferedBytes)
		})
		for _, prefix := range [][]byte{{0, 7, byte(kind >> 8), byte(kind)}, {0, 7, 0x81, 0, 0, 8, byte(kind >> 8), byte(kind)}} {
			typ := layers.EthernetTypeDot1Q
			if len(prefix) > 4 {
				typ = layers.EthernetTypeQinQ
			}
			events := replayGOOSECapture(t, oneFrameEthernetCapture(t, typ, append(bytes.Clone(prefix), wire...)))
			require.Len(t, events, 1)
			require.Empty(t, events[0].Error)
			wantDomain := "/vlan:7"
			if len(prefix) > 4 {
				wantDomain += "/vlan:8"
			}
			require.Equal(t, wantDomain, events[0].Domain.Encapsulation)
		}
		// Every truncated carrier is independent. A later complete packet cannot
		// provide bytes, context or completion for the earlier one.
		n := int(binary.BigEndian.Uint16(wire[2:]))
		if kind == 0x88a4 {
			n = int(binary.LittleEndian.Uint16(wire)&2047) + 2
		}
		for cut := 0; cut < n; cut++ {
			truncated := bytes.Clone(raw[:54+cut])
			binary.LittleEndian.PutUint32(truncated[32:], uint32(14+cut))
			binary.LittleEndian.PutUint32(truncated[36:], uint32(14+cut))
			events := replayGOOSECapture(t, truncated)
			for _, e := range events {
				require.NotEqual(t, "message", e.Completeness, "cut%d", cut)
			}
		}
	}
}
