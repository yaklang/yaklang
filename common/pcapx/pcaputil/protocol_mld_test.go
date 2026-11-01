package pcaputil

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"net"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func nativeMLDReport(t *testing.T) []byte {
	t.Helper()
	w, err := hex.DecodeString("8f00123400000002" +
		"01010002ff3e0000000000000000000000001234" +
		"20010db8000000000000000000000001" +
		"00000000000000000000ffffc0000201deadbeef" +
		"06000000ff0200000000000000000000000000fb")
	require.NoError(t, err)
	return w
}

func nativeMLDFrame(t *testing.T, message []byte) []byte {
	t.Helper()
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{0x33, 0x33, 0, 0, 0, 0x16}, EthernetType: layers.EthernetTypeIPv6}
	ip := &layers.IPv6{Version: 6, HopLimit: 1, NextHeader: layers.IPProtocolICMPv6, SrcIP: net.ParseIP("fe80::1"), DstIP: net.ParseIP("ff02::16")}
	b := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true}, eth, ip, gopacket.Payload(message)))
	return bytes.Clone(b.Bytes())
}

func TestMLDNativeFieldsOwnEveryRepeatedSourceRange(t *testing.T) {
	w := nativeMLDFrame(t, nativeMLDReport(t))
	var events []*ProtocolEvent
	flows := 0
	require.NoError(t, ReplayPcap(bytes.NewReader(icmpFragmentCapture(t, [][]byte{w})), WithTCPReassemblyOptions(TCPReassemblyOptions{AllowIncomplete: true}), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnTrafficFlowCreated(func(*TrafficFlow) { flows++ })))
	require.Zero(t, flows)
	require.Len(t, events, 1)
	e := events[0]
	require.Equal(t, "icmpv6-mld", e.Profile)
	require.Equal(t, "decoded", e.Status)
	require.Contains(t, e.Summary, "MLDv2")
	require.Zero(t, e.FlowID)
	values := func() []DisplayFieldValue {
		var out []DisplayFieldValue
		for _, v := range e.DisplayFieldValues().Values {
			if v.Name == "icmpv6.mldr.mar.multicast_address" {
				out = append(out, v)
			}
		}
		return out
	}
	before := e.DisplayFieldValues()
	groups := values()
	require.Len(t, groups, 2)
	require.Equal(t, "ff3e::1234", groups[0].Value)
	require.Equal(t, "ff02::fb", groups[1].Value)
	require.Equal(t, &DisplayFieldRange{Offset: 66, Length: 16}, groups[0].Range)
	require.Equal(t, &DisplayFieldRange{Offset: 122, Length: 16}, groups[1].Range)
	// The map tree's ranges address the actual full native PDU, not ICMP offset0.
	m := e.Session["MLD"].(map[string]any)
	records := m["Address Records"].([]map[string]any)
	ranges := records[1]["Wire Field Ranges"].(map[string]any)
	require.Equal(t, []int{122, 138}, ranges["Multicast Address"])
	// Public edits cannot rewrite private captured values or ordinal ranges.
	e.Session = map[string]any{"MLD": map[string]any{"Version": 3}}
	e.Fields = nil
	e.Raw = []byte{1}
	after := e.DisplayFieldValues()
	require.Equal(t, before, after)
	// JSON cannot manufacture ingress ownership from a plausible label/tree.
	encoded, err := json.Marshal(e)
	require.NoError(t, err)
	var decoded ProtocolEvent
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	for _, v := range decoded.DisplayFieldValues().Values {
		require.NotContains(t, v.Name, "icmpv6.mld")
	}
}

func TestMLDNativeDeclaredV2QueryNeverBecomesV1AfterCaptureCut(t *testing.T) {
	query, err := hex.DecodeString("82001234ffff0000ff3e000000000000000000000000abcd" + "02ff0001" + "20010db8000000000000000000000005")
	require.NoError(t, err)
	w := nativeMLDFrame(t, query)
	var events []*ProtocolEvent
	require.NoError(t, ReplayPcap(bytes.NewReader(icmpFragmentCapture(t, [][]byte{w[:54+24]})), WithTCPReassemblyOptions(TCPReassemblyOptions{AllowIncomplete: true}), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) })))
	require.Len(t, events, 1)
	e := events[0]
	require.Equal(t, "incomplete", e.Status)
	require.Equal(t, uint8(2), e.Session["MLD"].(map[string]any)["Version"])
	for _, value := range e.DisplayFieldValues().Values {
		require.NotEqual(t, "icmpv6.mld.maximum_response_delay", value.Name)
		require.NotEqual(t, "icmpv6.mld.nb_sources", value.Name)
		if value.Range != nil {
			require.LessOrEqual(t, value.Range.Offset+value.Range.Length, len(w[:54+24]))
		}
	}
}
