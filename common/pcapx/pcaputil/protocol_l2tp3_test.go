package pcaputil

import (
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func l2tp3NativeFrame(t *testing.T, w []byte, udp, v6 bool) []byte {
	t.Helper()
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeIPv4}
	ip4 := &layers.IPv4{Version: 4, TTL: 64, Protocol: 115, SrcIP: net.ParseIP("192.0.2.1"), DstIP: net.ParseIP("192.0.2.2")}
	ip6 := &layers.IPv6{Version: 6, HopLimit: 64, NextHeader: 115, SrcIP: net.ParseIP("2001:db8::1"), DstIP: net.ParseIP("2001:db8::2")}
	var ip gopacket.SerializableLayer = ip4
	if v6 {
		eth.EthernetType = layers.EthernetTypeIPv6
		ip = ip6
	}
	ls := []gopacket.SerializableLayer{eth, ip}
	if udp {
		ip4.Protocol = layers.IPProtocolUDP
		ip6.NextHeader = layers.IPProtocolUDP
		u := &layers.UDP{SrcPort: 1701, DstPort: 1701}
		if v6 {
			require.NoError(t, u.SetNetworkLayerForChecksum(ip6))
		} else {
			require.NoError(t, u.SetNetworkLayerForChecksum(ip4))
		}
		ls = append(ls, u)
	}
	ls = append(ls, gopacket.Payload(w))
	b := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, ls...))
	return b.Bytes()
}
func l2tp3NativeReplay(t *testing.T, w []byte, limit int, truncated bool) ([]*ProtocolEvent, *binParser) {
	t.Helper()
	var events []*ProtocolEvent
	c := NewDefaultConfig()
	require.NoError(t, WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) })(c))
	require.NoError(t, c.prepareBinParser())
	a := c.binParser
	if limit > 0 {
		a.budget.MaxFrameBytes = limit
	}
	p := gopacket.NewPacket(w, CaptureLinkDecoder(layers.LinkTypeEthernet), gopacket.Default)
	ci := gopacket.CaptureInfo{Timestamp: time.Unix(10, 0), CaptureLength: len(w), Length: len(w)}
	if truncated {
		ci.Length += 10
	}
	p.Metadata().CaptureInfo = withEvidence(ci, captureEvidence{Ref: PacketReference{Number: 7, Domain: CaptureDomain{Interface: 3}}})
	_, handled := a.networkPacket(p)
	require.True(t, handled)
	a.closeUDPSessions()
	return events, a
}

func TestL2TPv3NativeIPUDPControlAndTypedIdentity(t *testing.T) {
	for _, udp := range []bool{false, true} {
		for _, v6 := range []bool{false, true} {
			w := []byte{0xc8, 3, 0, 20, 0x12, 0x34, 0x56, 0x78, 0xff, 0xfe, 0, 5, 0x80, 8, 0, 0, 0, 0, 0, 6}
			if !udp {
				w = append([]byte{0, 0, 0, 0}, w...)
			}
			es, a := l2tp3NativeReplay(t, l2tp3NativeFrame(t, w, udp, v6), 0, false)
			require.Len(t, es, 1)
			e := es[0]
			require.Equal(t, "decoded", e.Status, e.Error)
			require.Equal(t, w, e.Raw, "Ethernet minimum padding is not an AVP")
			require.Equal(t, uint64(0), e.FlowID, "HELLO without setup cannot establish a connection")
			require.Equal(t, uint64(7), e.SourceBytes.PacketRefs[0].Number)
			require.Equal(t, 3, e.Domain.Interface)
			require.Equal(t, []any{uint64(0x12345678)}, e.DisplayFields()["l2tp.ccid"])
			require.Equal(t, []any{uint64(0xfffe)}, e.DisplayFields()["l2tp.Ns"])
			require.Empty(t, e.DisplayFields()["l2tp.session"], "v3 SID must never alias the 16-bit v2 field")
			if !udp {
				require.Equal(t, []any{uint64(0)}, e.DisplayFields()["l2tp.sid"], "retain the actual IP zero Session ID marker as Wireshark does")
			}
			require.Zero(t, a.buffered.Load())
		}
	}
}

func TestL2TPv3MidstreamDataNeverGuessesPayload(t *testing.T) {
	w := make([]byte, 38)
	binary.BigEndian.PutUint32(w, 0xfedcba98)
	copy(w[4:], []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 8, 0, 0x45})
	es, _ := l2tp3NativeReplay(t, l2tp3NativeFrame(t, w, false, false), 0, false)
	e := es[0]
	require.Equal(t, "decoded", e.Status)
	require.Equal(t, "limited", e.Completeness)
	require.False(t, e.Session["Payload Decoded"].(bool))
	require.NotContains(t, e.Session, "Ethernet Source")
	require.Equal(t, []any{uint64(0xfedcba98)}, e.DisplayFields()["l2tp.sid"])
	require.Empty(t, e.DisplayFields()["l2tp.version"], "v3 is inferred from IP115; no version bits occur in a data header")
	f, err := CompileDisplayFilter("not l2tp.cookie")
	require.NoError(t, err)
	require.False(t, f.EvaluateEvents(es).Complete, "unknown cookie layout is not proved field absence")
	decoded, err := e.Decode()
	require.NoError(t, err)
	e.Session["Session ID"] = uint64(1)
	require.Equal(t, uint64(0xfedcba98), decoded["session"].(map[string]any)["Session ID"])
	require.Equal(t, []any{uint64(0xfedcba98)}, e.DisplayFields()["l2tp.sid"], "editable Session map is not parser authority")
}

func TestL2TPv3CarrierCutAndBudgetRefuseCompleteFields(t *testing.T) {
	w := []byte{0, 0, 0, 0, 0xc8, 3, 0, 20, 0, 0, 0, 1, 0, 0, 0, 0, 0x80, 8, 0, 0, 0, 0, 0, 6}
	frame := l2tp3NativeFrame(t, w, false, false)
	es, _ := l2tp3NativeReplay(t, frame, 0, true)
	require.Equal(t, "incomplete", es[0].Status)
	require.Empty(t, es[0].Session)
	es, a := l2tp3NativeReplay(t, frame, 8, false)
	require.Equal(t, "limited", es[0].Status)
	require.Empty(t, es[0].Raw)
	require.Equal(t, len(w), es[0].Length)
	require.Equal(t, uint64(len(w)), a.limited.Load())
}

func TestL2TPv3UDPOpaqueHeaderCannotAutoIdentify(t *testing.T) {
	w := []byte{0, 3, 0, 0, 0x12, 0x34, 0x56, 0x78, 1, 2, 3, 4}
	c := NewDefaultConfig()
	require.NoError(t, WithOnProtocolMessage(func(e *ProtocolEvent) { t.Fatal("port-only opaque UDP data identified as L2TPv3") })(c))
	require.NoError(t, c.prepareBinParser())
	p := gopacket.NewPacket(l2tp3NativeFrame(t, w, true, false), CaptureLinkDecoder(layers.LinkTypeEthernet), gopacket.Default)
	_, handled := c.binParser.networkPacket(p)
	require.False(t, handled)
	c.binParser.closeUDPSessions()
}

func TestL2TPv3InvalidIPZeroLengthCannotCreateDecodedData(t *testing.T) {
	frame := l2tp3NativeFrame(t, []byte{1, 2, 3, 4, 5, 6, 7, 8}, false, false)
	frame[16], frame[17] = 0, 0 // gopacket treats IPv4 Length0 as capture-rest; v3 must not.
	es, _ := l2tp3NativeReplay(t, frame, 0, false)
	require.Len(t, es, 1)
	require.Equal(t, "malformed", es[0].Status)
	require.Empty(t, es[0].Session)
	require.Contains(t, es[0].Error, "IPv4 total length")
	control := l2tp3TestControl(true, 100, 1, 1, 6)
	frame = l2tp3NativeFrame(t, control, true, false)
	frame[16], frame[17] = 0, 0
	es, _ = l2tp3NativeReplay(t, frame, 0, false)
	require.Equal(t, "malformed", es[0].Status, "the UDP entry must not bypass original IPv4 length validation")
	require.Empty(t, es[0].Session)
}

func TestL2TPv2TypedSessionKeepsItsOriginalRange(t *testing.T) {
	e := &ProtocolEvent{Protocol: "l2tp", Status: "decoded", Raw: make([]byte, 8), Session: map[string]any{"Session ID": uint64(0xabcd), "Wire Field Ranges": map[string]any{"Session ID": []any{uint64(6), uint64(8)}}}}
	for _, v := range e.DisplayFieldValues().Values {
		if v.Name == "l2tp.session" {
			require.Equal(t, uint64(0xabcd), v.Value)
			require.Equal(t, &DisplayFieldRange{Offset: 6, Length: 2}, v.Range)
			return
		}
	}
	t.Fatal("v2 session field disappeared during v3 registration")
}

func TestL2TPv3ExplicitUDPDataDoesNotClaimControlEvidence(t *testing.T) {
	var events []*ProtocolEvent
	c := NewDefaultConfig()
	require.NoError(t, WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) })(c))
	require.NoError(t, WithProtocolDecodeAs("udp", 1701, "l2tp")(c))
	require.NoError(t, c.prepareBinParser())
	p := gopacket.NewPacket(l2tp3NativeFrame(t, []byte{0, 3, 0, 0, 0x12, 0x34, 0x56, 0x78, 1, 2, 3, 4}, true, false), CaptureLinkDecoder(layers.LinkTypeEthernet), gopacket.Default)
	_, handled := c.binParser.networkPacket(p)
	require.True(t, handled)
	require.Len(t, events, 1)
	require.Equal(t, "explicit-decode-as", events[0].Admission)
	require.Equal(t, "limited", events[0].Completeness)
	require.False(t, events[0].Session["Payload Decoded"].(bool))
	c.binParser.closeUDPSessions()
}

func TestL2TPv3HiddenOptionalParameterIsNotProvedFieldAbsence(t *testing.T) {
	// HELLO followed by hidden optional Local Session ID. Its M bit is clear;
	// the encrypted value still cannot justify a negative field predicate.
	w := []byte{0, 0, 0, 0, 0xc8, 3, 0, 30, 0, 0, 0, 1, 0, 7, 0, 5, 0x80, 8, 0, 0, 0, 0, 0, 6, 0x40, 10, 0, 0, 0, 63, 1, 2, 3, 4}
	es, _ := l2tp3NativeReplay(t, l2tp3NativeFrame(t, w, false, false), 0, false)
	require.Len(t, es, 1)
	require.Equal(t, "decoded", es[0].Status)
	require.Equal(t, "limited", es[0].Completeness)
	f, err := CompileDisplayFilter("not l2tp.avp.local_session_id")
	require.NoError(t, err)
	require.False(t, f.EvaluateEvents(es).Complete)
}
