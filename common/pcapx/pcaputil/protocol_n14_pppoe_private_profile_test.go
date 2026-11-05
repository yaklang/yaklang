package pcaputil

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func TestN14PPPoEProfilePrecedesKnownAndUnconfirmedOwner(t *testing.T) {
	for _, cooked := range []bool{false, true} {
		for _, fragmented := range []bool{false, true} {
			for _, kind := range []string{"version", "type", "zero-SID", "reserved-SID", "session-code", "discovery-session-code", "child-extent"} {
				t.Run(fmt.Sprintf("cooked%t/fragments%t/%s", cooked, fragmented, kind), func(t *testing.T) {
					prefix, body, _ := n14AuditTransport(t, false, 0x5a4b)
					inners := [][]byte{append(prefix, body...)}
					if fragmented {
						inners = n14AuditFragments(t, "destination-after-fragment", false, false, 0x11223344, 0x5a4b)
					}
					var frames [][]byte
					for _, inner := range inners {
						raw := n14AuditPPPoE(t, inner, 1, false, 0)
						switch kind {
						case "version":
							raw[14] = 0x21
						case "type":
							raw[14] = 0x12
						case "zero-SID":
							binary.BigEndian.PutUint16(raw[16:18], 0)
						case "reserved-SID":
							binary.BigEndian.PutUint16(raw[16:18], 0xffff)
						case "session-code":
							raw[15] = byte(layers.PPPoECodePADI)
						case "discovery-session-code":
							binary.BigEndian.PutUint16(raw[12:14], uint16(layers.EthernetTypePPPoEDiscovery))
						case "child-extent":
							binary.BigEndian.PutUint16(raw[18:20], binary.BigEndian.Uint16(raw[18:20])-1)
						}
						if cooked {
							raw = n14AuditCookedPPPoE(t, raw, layers.LinkTypeLinuxSLL)
							if kind == "discovery-session-code" {
								binary.BigEndian.PutUint16(raw[14:16], uint16(layers.EthernetTypePPPoEDiscovery))
							}
						}
						frames = append(frames, raw)
					}
					link := layers.LinkTypeEthernet
					if cooked {
						link = layers.LinkTypeLinuxSLL
					}
					var events []*ProtocolEvent
					var stats ProtocolStats
					var conf *CaptureConfig
					flows, rangeStates := 0, 0
					opts := []CaptureOption{func(c *CaptureConfig) error { conf = c; return nil }, WithTCPReassemblyOptions(TCPReassemblyOptions{AllowIncomplete: true, Stream: true}), WithOnTrafficFlowCreated(func(*TrafficFlow) { flows++ }), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s }), WithEveryPacket(func(gopacket.Packet) {
						rangeStates += len(conf.binParser.fragments.sets)
						require.Nil(t, conf.binParser.udpSessions)
					})}
					require.NoError(t, ReplayPcap(bytes.NewReader(n14AuditClassicCapture(t, frames, link)), opts...))
					require.Zero(t, flows)
					require.Zero(t, rangeStates)
					require.Zero(t, stats.Messages)
					require.Zero(t, stats.BufferedBytes)
					require.Len(t, events, len(frames))
					for i, e := range events {
						if kind == "version" || kind == "type" {
							require.Equal(t, "PPPoEHeaderProfile", e.ExpertCode)
							require.Equal(t, "limited", e.Status)
						} else {
							require.Equal(t, "PPPoEHeaderAdmission", e.ExpertCode, "profile failure precedes missing-owner context")
							require.Equal(t, "malformed", e.Status)
						}
						require.Empty(t, e.Raw)
						require.Zero(t, e.FlowID)
						require.Zero(t, e.TCPFlowID)
						require.Nil(t, e.SourceBytes.CompletedIn)
						view, ok := e.NativePPPoEHeaderObservation()
						require.True(t, ok)
						at := 14
						if cooked {
							at = 16
						}
						require.Equal(t, frames[i][at:at+6], view.Raw)
						require.True(t, e.NativePPPoEHeaderWireMatches(view.Raw))
					}
				})
			}
		}
	}
}

func TestN14PPPoEDeclaredLengthCannotBorrowBackingBytes(t *testing.T) {
	prefix, body, _ := n14AuditTransport(t, true, 0)
	raw := n14AuditPPPoE(t, append(prefix, body...), 1, false, 0)
	declared := binary.BigEndian.Uint16(raw[18:20])
	binary.BigEndian.PutUint16(raw[18:20], declared+1)
	var flows int
	var stats TCPReassemblyStats
	var events []*ProtocolEvent
	require.NoError(t, ReplayPcap(bytes.NewReader(n14AuditClassicCapture(t, [][]byte{raw}, layers.LinkTypeEthernet)), WithTCPReassemblyOptions(TCPReassemblyOptions{AllowIncomplete: true, Stream: true}), WithOnTrafficFlowCreated(func(*TrafficFlow) { flows++ }), WithOnTCPReassemblyStats(func(s TCPReassemblyStats) { stats = s }), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) })))
	require.Zero(t, flows)
	require.EqualValues(t, 1, stats.DecodeErrors, "existing capacity/strict decoder failure stays explicit")
	require.Zero(t, stats.DeliveredBytes)
	for _, e := range events {
		require.NotContains(t, []string{"tcp", "udp", "http", "dns"}, e.Protocol)
		require.Zero(t, e.FlowID)
		require.Zero(t, e.TCPFlowID)
	}
}

func TestN14PPPoEDiscoveryNeverSelectsPPPSession(t *testing.T) {
	raw := make([]byte, 20)
	copy(raw[:12], []byte{2, 0, 0, 0, 0, 1, 2, 0, 0, 0, 0, 2})
	binary.BigEndian.PutUint16(raw[12:14], uint16(layers.EthernetTypePPPoEDiscovery))
	copy(raw[14:], []byte{0x11, byte(layers.PPPoECodePADI), 0, 0, 0, 0})
	var events []*ProtocolEvent
	flows := 0
	require.NoError(t, ReplayPcap(bytes.NewReader(n14AuditClassicCapture(t, [][]byte{raw}, layers.LinkTypeEthernet)), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnTrafficFlowCreated(func(*TrafficFlow) { flows++ })))
	require.Zero(t, flows)
	for _, e := range events {
		require.NotContains(t, []string{"tcp", "udp", "http", "dns"}, e.Protocol)
		require.Zero(t, e.FlowID)
	}
}

func TestN14PPPoEPrivateHeaderObservationSurvivesPublicPoisonButNotDTO(t *testing.T) {
	prefix, body, _ := n14AuditTransport(t, false, 0x5a4b)
	raw := n14AuditCookedPPPoE(t, n14AuditPPPoE(t, append(prefix, body...), 1, false, 0), layers.LinkTypeLinuxSLL)
	var events []*ProtocolEvent
	require.NoError(t, ReplayPcap(bytes.NewReader(n14AuditClassicCapture(t, [][]byte{raw}, layers.LinkTypeLinuxSLL)), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) })))
	require.Len(t, events, 1)
	e := events[0]
	view, ok := e.NativePPPoEHeaderObservation()
	require.True(t, ok)
	want := bytes.Clone(view.Raw)
	clear(view.Raw)
	view.Fields["PPPoE Session ID"] = uint16(9)
	view.SourceBytes.PacketRefs[0].Number = 999
	e.Protocol, e.Profile, e.Status, e.ExpertCode = "http", "forged", "decoded", ""
	e.Raw = []byte("forged PDU")
	e.Session = map[string]any{"PPPoE Session Owner": "confirmed", "PPPoE Session ID": uint16(9)}
	e.Fields = map[string]any{"PPPoE Session ID": uint16(9)}
	e.SourceBytes = ByteSource{Kind: "captured", PacketRefs: []PacketReference{{Number: 999}}}
	got, ok := e.NativePPPoEHeaderObservation()
	require.True(t, ok)
	require.Equal(t, want, got.Raw)
	require.Equal(t, uint16(30), got.Fields["PPPoE Session ID"])
	require.Equal(t, "context-required", got.Status)
	require.EqualValues(t, 1, got.SourceBytes.PacketRefs[0].Number)
	require.True(t, e.NativePPPoEHeaderWireMatches(raw[16:22]))
	require.False(t, e.NativePPPoEHeaderWireMatches(make([]byte, 6)))
	fields, err := e.GetFields()
	require.NoError(t, err)
	require.Equal(t, uint16(30), fields["PPPoE Session ID"])
	encoded, err := json.Marshal(e)
	require.NoError(t, err)
	var dto ProtocolEvent
	require.NoError(t, json.Unmarshal(encoded, &dto))
	_, ok = dto.NativePPPoEHeaderObservation()
	require.False(t, ok)
	require.False(t, dto.NativePPPoEHeaderWireMatches(want))
}
