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
	"sync/atomic"
	"testing"
	"time"
)

type lldpAnswer struct {
	Wire, Error, Protocol string
	ReaderError           string `json:"reader_error"`
	Fields                map[string]any
	Packets               int
	Tags                  []int
}

func lldpControls(t *testing.T) []rocControl {
	t.Helper()
	b, err := trafficfixture.ReadFile("lldp-discovery/manifest.json")
	require.NoError(t, err)
	var m struct {
		Schema string
		Cases  []rocControl
	}
	require.NoError(t, json.Unmarshal(b, &m))
	require.Equal(t, "pr5013-lldp-discovery/v1", m.Schema)
	require.Len(t, m.Cases, 40)
	all, err := trafficfixture.AllExpectations()
	require.NoError(t, err)
	found := map[string]trafficfixture.FrozenCase{}
	for _, batch := range all {
		for _, c := range batch.Cases {
			found[c.ID] = c
		}
	}
	for _, c := range m.Cases {
		v, ok := found["lldp-discovery/"+c.ID]
		require.True(t, ok)
		require.Equal(t, c.Capture, v.Input.File)
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
func lldpInput(t *testing.T, c rocControl) ([]byte, lldpAnswer) {
	t.Helper()
	b, err := trafficfixture.ReadFile("lldp-discovery/" + c.Capture)
	require.NoError(t, err)
	require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(b)))
	a, err := trafficfixture.ReadFile("lldp-discovery/" + c.Answer)
	require.NoError(t, err)
	require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(a)))
	var want lldpAnswer
	require.NoError(t, json.Unmarshal(a, &want))
	return b, want
}
func lldpCheck(t *testing.T, want lldpAnswer, e *ProtocolEvent, deferred bool) {
	t.Helper()
	require.Equal(t, "lldp", e.Protocol)
	require.Equal(t, "lldp-ended-discovery", e.Profile)
	require.Equal(t, "ethernet", e.Transport)
	require.Equal(t, want.Wire, hex.EncodeToString(e.Raw))
	require.Zero(t, e.FlowID)
	require.Zero(t, e.TransactionID)
	require.Zero(t, e.ResponseTo)
	require.Equal(t, "02:00:00:00:00:01", e.Source)
	require.Equal(t, "01:80:c2:00:00:0e", e.Destination)
	require.Len(t, e.SourceBytes.PacketRefs, 1)
	require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
	require.EqualValues(t, 1, e.SourceBytes.PacketRefs[0].Number)
	enc := ""
	for _, tag := range want.Tags {
		enc += fmt.Sprintf("/vlan:%d", tag)
	}
	require.Equal(t, enc, e.Domain.Encapsulation)
	if want.Fields == nil {
		rocTypedError(t, want.Error, e.sessionError)
		require.NotEmpty(t, e.Error)
		require.Nil(t, e.Session)
		require.Nil(t, e.Structured)
		return
	}
	require.Empty(t, e.Error)
	status := "decoded"
	if deferred {
		status = "deferred"
	}
	require.Equal(t, status, e.Status)
	rocEqualFields(t, want.Fields, e.Session)
	fields, err := e.GetFields()
	require.NoError(t, err)
	rocEqualFields(t, want.Fields, fields)
	decoded, err := e.Decode()
	require.NoError(t, err)
	rocEqualFields(t, want.Fields, protocolFields(decoded))
	fields["TLVs"].([]map[string]any)[0]["Value"].([]byte)[0] ^= 0xff
	fields["Chassis ID"].(map[string]any)["ID"].([]byte)[0] ^= 0xff
	rocEqualFields(t, want.Fields, e.Session)
	// Fields/Structured are caller-owned cached trees; their mutation must
	// not reach the independent session or semantic snapshot.
	rocEqualFields(t, want.Fields, e.semanticFields)
}
func TestLLDPDiscoverySealedReplay(t *testing.T) {
	for _, c := range lldpControls(t) {
		t.Run(c.ID, func(t *testing.T) {
			raw, want := lldpInput(t, c)
			for _, workers := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observe := range []bool{false, true} {
						t.Run(fmt.Sprintf("w%d/d%v/o%v", workers, deferred, observe), func(t *testing.T) {
							var events []*ProtocolEvent
							var stats ProtocolStats
							var asm TCPReassemblyStats
							var seen atomic.Int64
							opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s }), WithOnTCPReassemblyStats(func(s TCPReassemblyStats) { asm = s })}
							if observe {
								opts = append(opts, WithEveryPacket(func(p gopacket.Packet) { require.Equal(t, raw[40:], p.Data()); seen.Add(1) }))
							}
							err := ReplayPcap(bytes.NewReader(raw), opts...)
							if want.ReaderError != "" {
								require.EqualError(t, err, want.ReaderError)
								require.EqualValues(t, 1, asm.DecodeErrors)
							} else {
								require.NoError(t, err)
							}
							require.EqualValues(t, want.Packets, asm.CapturedPackets)
							if observe {
								require.EqualValues(t, want.Packets, seen.Load())
							}
							require.Zero(t, stats.BufferedBytes)
							require.Zero(t, asm.UnreassembledBytes)
							require.Zero(t, asm.UnreassembledSegments)
							if want.Protocol == "" {
								for _, e := range events {
									require.NotEqual(t, "lldp", e.Protocol)
								}
								return
							}
							require.Len(t, events, 1)
							lldpCheck(t, want, events[0], deferred)
						})
					}
				}
			}
		})
	}
}
func TestLLDPDiscoveryBoundariesAndOwnership(t *testing.T) {
	for _, c := range lldpControls(t) {
		_, a := lldpInput(t, c)
		if a.Fields == nil {
			continue
		}
		w, err := hex.DecodeString(a.Wire)
		require.NoError(t, err)
		for n := 0; n < int(a.Fields["PDU Length"].(float64)); n++ {
			f, err := decodeLLDP(w[:n], 4096)
			require.Error(t, err, "%s cut %d", c.ID, n)
			require.Nil(t, f)
		}
		f, err := decodeLLDP(w, 4096)
		require.NoError(t, err)
		for i := range w {
			w[i] ^= 0xff
		}
		rocEqualFields(t, a.Fields, f)
	}
}
func TestLLDPDiscoveryResourcesAndDomains(t *testing.T) {
	controls := lldpControls(t)
	var wire []byte
	var want lldpAnswer
	for _, c := range controls {
		if c.ID == "minimal" {
			_, want = lldpInput(t, c)
			wire, _ = hex.DecodeString(want.Wire)
		}
	}
	require.NotEmpty(t, wire)
	_, err := decodeLLDP(wire, 3)
	rocTypedError(t, "ResourceExceeded", err)
	f, err := decodeLLDP(wire, 4)
	require.NoError(t, err)
	rocEqualFields(t, want.Fields, f)
	for _, limit := range []int{len(wire) - 1, len(wire)} {
		budget := DefaultParserBudget()
		budget.MaxFrameBytes = limit
		budget.MaxMessageBytes = limit
		var events []*ProtocolEvent
		a := &binParser{budget: budget, config: BinParserConfig{OnEvent: func(e *ProtocolEvent) { events = append(events, e) }}}
		eth := &layers.Ethernet{SrcMAC: []byte{2, 0, 0, 0, 0, 1}, DstMAC: []byte{1, 128, 194, 0, 0, 14}}
		for _, iface := range []int{1, 2} {
			ci := gopacket.CaptureInfo{Timestamp: time.Unix(1, 0), CaptureLength: 14 + len(wire), Length: 14 + len(wire), InterfaceIndex: iface}
			ref := PacketReference{Number: uint64(iface), Domain: CaptureDomain{Interface: iface}}
			a.decodeLLDPEthernet(eth, wire, captureEvidence{Ref: ref}, ci)
		}
		require.Len(t, events, 2)
		for i, e := range events {
			require.Equal(t, i+1, e.Domain.Interface)
			if limit < len(wire) {
				rocTypedError(t, "ResourceExceeded", e.sessionError)
				require.Nil(t, e.Raw)
				require.Nil(t, e.Session)
			} else {
				rocEqualFields(t, want.Fields, e.Session)
			}
		}
		a.decodeLLDPEthernet(eth, wire, captureEvidence{}, gopacket.CaptureInfo{CaptureLength: len(wire), Length: len(wire) + 1})
		require.Len(t, events, 3)
		rocTypedError(t, "NeedMore", events[2].sessionError)
		require.Nil(t, events[2].Raw)
		require.Nil(t, events[2].Session)
		require.Zero(t, a.stats().BufferedBytes)
	}
}
