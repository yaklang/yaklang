package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func espNativeFrame(t *testing.T, w []byte, v6 bool, next layers.IPProtocol) []byte {
	t.Helper()
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeIPv4}
	var ip gopacket.SerializableLayer = &layers.IPv4{Version: 4, TTL: 64, Protocol: next, SrcIP: net.ParseIP("192.0.2.1"), DstIP: net.ParseIP("192.0.2.2")}
	if v6 {
		eth.EthernetType = layers.EthernetTypeIPv6
		ip = &layers.IPv6{Version: 6, HopLimit: 64, NextHeader: next, SrcIP: net.ParseIP("2001:db8::1"), DstIP: net.ParseIP("2001:db8::2")}
	}
	b := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, gopacket.Payload(w)))
	return bytes.Clone(b.Bytes())
}

func espNativeReplay(t *testing.T, frames [][]byte, limit int, truncated bool) ([]*ProtocolEvent, *binParser) {
	t.Helper()
	var out []*ProtocolEvent
	c := NewDefaultConfig()
	require.NoError(t, WithOnProtocolMessage(func(e *ProtocolEvent) { out = append(out, e) })(c))
	require.NoError(t, c.prepareBinParser())
	if limit > 0 {
		c.binParser.budget.MaxFrameBytes = limit
	}
	for i, w := range frames {
		p := gopacket.NewPacket(w, CaptureLinkDecoder(layers.LinkTypeEthernet), gopacket.Default)
		ci := gopacket.CaptureInfo{Timestamp: time.Unix(100+int64(i), 0), CaptureLength: len(w), Length: len(w)}
		if truncated {
			ci.Length += 17
		}
		p.Metadata().CaptureInfo = withEvidence(ci, captureEvidence{Ref: PacketReference{Number: uint64(i + 1), Domain: CaptureDomain{Interface: 3}}})
		c.binParser.networkPacket(protocolAnalysisPacket(p, nil))
	}
	c.binParser.closeUDPSessions()
	return out, c.binParser
}

func TestESPNativeExplicitHeaderScopeAndPrivateQueries(t *testing.T) {
	w := []byte{0x12, 0x34, 0x56, 0x78, 0, 0, 0, 0, 0x45, 0, 0, 20, 23, 3, 3, 0}
	for _, v6 := range []bool{false, true} {
		events, a := espNativeReplay(t, [][]byte{espNativeFrame(t, w, v6, layers.IPProtocolESP)}, 0, false)
		require.Len(t, events, 1)
		e := events[0]
		require.Equal(t, "esp", e.Protocol)
		require.Equal(t, "limited", e.Status)
		require.Equal(t, "headers", e.Completeness)
		require.Equal(t, w, e.Raw)
		require.Equal(t, "ESPHeaderFields", e.Entry)
		require.Zero(t, e.FlowID)
		require.Zero(t, e.TCPFlowID)
		require.Equal(t, uint64(0), a.flows.Load())
		require.Equal(t, uint64(0), a.panics.Load())
		before, err := e.GetFields()
		require.NoError(t, err)
		require.Equal(t, uint32(0x12345678), before["SPI"])
		require.Equal(t, w[8:], before["Opaque Body"])
		aliases := e.DisplayFieldValues()
		require.Equal(t, []any{uint64(0x12345678)}, aliases.FieldMap()["esp.spi"])
		for _, f := range aliases.Values {
			if f.Name == "esp.sequence" {
				require.Equal(t, &DisplayFieldRange{Offset: 4, Length: 4}, f.Range)
				require.Equal(t, uint64(0), f.Value)
			}
		}
		e.Fields = map[string]any{"SPI": uint32(1)}
		e.Session = map[string]any{"SPI": uint32(2)}
		e.Protocol = "http"
		e.Status = "decoded"
		e.Structured = map[string]any{"fields": e.Fields}
		after, err := e.GetFields()
		require.NoError(t, err)
		require.Equal(t, before, after)
		require.Equal(t, aliases.Values, e.DisplayFieldValues().Values)
		e.Raw[4] ^= 0xff
		require.NotContains(t, e.DisplayFields(), "esp.sequence", "public raw edits cannot keep a forged wire alias")
		require.Contains(t, e.DisplayFields(), "esp.spi", "independent unmodified word keeps its span")
	}
}

func TestESPNativePartialHeaderBudgetAndNoPortGuess(t *testing.T) {
	w := []byte{0x12, 0x34, 0x56, 0x78, 0xff, 0xff, 0xff, 0xff, 1, 2, 3, 4}
	for _, v6 := range []bool{false, true} {
		for cut := 0; cut < 8; cut++ {
			t.Run(fmt.Sprintf("v6=%v/cut=%d", v6, cut), func(t *testing.T) {
				events, a := espNativeReplay(t, [][]byte{espNativeFrame(t, w[:cut], v6, layers.IPProtocolESP)}, 0, false)
				if v6 && cut == 0 {
					require.Empty(t, events, "zero-length IPv6 payload is an unselected native parent profile")
					return
				}
				require.Len(t, events, 1)
				e := events[0]
				require.Equal(t, "incomplete", e.Status)
				require.True(t, bytes.Equal(w[:cut], e.Raw))
				require.Zero(t, a.panics.Load())
				require.NotContains(t, e.DisplayFields(), "esp.sequence")
				if cut < 4 {
					require.NotContains(t, e.DisplayFields(), "esp.spi")
				} else {
					require.Contains(t, e.DisplayFields(), "esp.spi")
				}
				e.Protocol = "http"
				e.Session = map[string]any{"SPI": uint32(999)}
				e.Fields = e.Session
				e.Status = "decoded"
				_, err := e.GetFields()
				require.Error(t, err)
			})
		}
		for _, truncated := range []bool{false, true} {
			events, _ := espNativeReplay(t, [][]byte{espNativeFrame(t, w, v6, layers.IPProtocolESP)}, 0, truncated)
			require.Len(t, events, 1)
			if truncated {
				require.Equal(t, "incomplete", events[0].Status)
			} else {
				require.Equal(t, "limited", events[0].Status)
			}
		}
		events, _ := espNativeReplay(t, [][]byte{espNativeFrame(t, w, v6, layers.IPProtocolESP)}, 16, false)
		require.Len(t, events, 1)
		require.Nil(t, events[0].Raw)
		require.Nil(t, events[0].Session)
		require.Equal(t, "ResourceExceeded", events[0].ExpertCode)
		udp := make([]byte, 8)
		binary.BigEndian.PutUint16(udp[:2], 4500)
		binary.BigEndian.PutUint16(udp[2:4], 4500)
		binary.BigEndian.PutUint16(udp[4:6], uint16(8+len(w)))
		p := gopacket.NewPacket(espNativeFrame(t, append(udp, w...), v6, layers.IPProtocolUDP), CaptureLinkDecoder(layers.LinkTypeEthernet), gopacket.Default)
		_, _, selected := declaredESPWire(protocolAnalysisPacket(p, nil))
		require.False(t, selected)
	}
}

func TestESPNativeFragmentReorderAndCaptureIsolation(t *testing.T) {
	w := []byte{0x12, 0x34, 0x56, 0x78, 0, 0, 0, 8, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	for _, v6 := range []bool{false, true} {
		frame := espNativeFrame(t, w, v6, layers.IPProtocolESP)
		if !v6 {
			frame = frame[:14+20+len(w)] // Fragment the IP datagram, excluding Ethernet padding.
		}
		parts := icmpFragmentFrames(t, frame, 8, false)
		if v6 {
			for _, part := range parts {
				part[54] = 50
			}
		}
		for _, reverse := range []bool{false, true} {
			ordered := append([][]byte(nil), parts...)
			if reverse {
				ordered[0], ordered[1] = ordered[1], ordered[0]
			}
			events, _ := espNativeReplay(t, ordered, 0, false)
			var esp []*ProtocolEvent
			for _, e := range events {
				if e.Protocol == "esp" {
					esp = append(esp, e)
				}
			}
			require.Len(t, esp, 1)
			require.Equal(t, w, esp[0].Raw)
			require.Equal(t, "reassembled", esp[0].SourceBytes.Kind)
			require.Len(t, esp[0].SourceBytes.PacketRefs, 2)
		}
		lost, _ := espNativeReplay(t, parts[:1], 0, false)
		for _, e := range lost {
			require.NotEqual(t, "esp", e.Protocol, "missing fragment cannot publish a complete ESP header observation")
		}
	}
}

func TestIPsecDecoderAHGuardAndICMPQuoteBoundary(t *testing.T) {
	for _, v6 := range []bool{false, true} {
		for cut := 0; cut <= 12; cut++ {
			w := make([]byte, cut)
			if cut > 0 {
				w[0] = 50
			}
			p := gopacket.NewPacket(espNativeFrame(t, w, v6, layers.IPProtocolAH), CaptureLinkDecoder(layers.LinkTypeEthernet), gopacket.Default)
			require.NotNil(t, p.ErrorLayer())
			require.NotContains(t, p.ErrorLayer().Error().Error(), "panic")
			native := protocolAnalysisPacket(p, nil)
			require.NotNil(t, native.ErrorLayer())
			require.NotContains(t, native.ErrorLayer().Error().Error(), "panic")
		}
	}
	quoted := espNativeFrame(t, []byte{0x12, 0x34, 0x56, 0x78, 0, 0, 0, 1}, false, layers.IPProtocolESP)[14:]
	icmp := append([]byte{3, 1, 0, 0, 0, 0, 0, 0}, quoted...)
	p := gopacket.NewPacket(espNativeFrame(t, icmp, false, layers.IPProtocolICMPv4), CaptureLinkDecoder(layers.LinkTypeEthernet), gopacket.Default)
	_, _, selected := declaredESPWire(protocolAnalysisPacket(p, nil))
	require.False(t, selected)
}

func TestESPNativeRejectsErroredParentAndWireLengthZero(t *testing.T) {
	w := []byte{0x12, 0x34, 0x56, 0x78, 0, 0, 0, 1, 1, 2, 3, 4, 5, 6, 7, 8}
	badOption := append([]byte{50, 0, 1, 0xff, 0, 0, 0, 0}, w...)
	for _, next := range []layers.IPProtocol{layers.IPProtocolIPv6Destination, layers.IPProtocolIPv6HopByHop} {
		frame := espNativeFrame(t, badOption, true, next)
		p := gopacket.NewPacket(frame, CaptureLinkDecoder(layers.LinkTypeEthernet), gopacket.Default)
		native := protocolAnalysisPacket(p, nil)
		require.NotNil(t, native.ErrorLayer())
		_, _, selected := declaredESPWire(native)
		require.False(t, selected, "a partial option layer cannot admit its declared ESP child")
		events, _ := espNativeReplay(t, [][]byte{frame}, 0, false)
		for _, e := range events {
			require.NotEqual(t, "esp", e.Protocol)
		}
	}
	frame := espNativeFrame(t, w, false, layers.IPProtocolESP)
	frame[16], frame[17] = 0, 0 // Dependency treats zero IPv4 total length as all remaining capture bytes.
	events, _ := espNativeReplay(t, [][]byte{frame}, 0, false)
	require.Len(t, events, 1)
	require.Equal(t, "ip", events[0].Protocol)
	require.Equal(t, "ESPParentLengthInvalid", events[0].ExpertCode)
	require.NotContains(t, events[0].DisplayFields(), "esp.spi")
}

func TestESPNativeDeclaredExtensionsAndAHRemainOpaque(t *testing.T) {
	w := []byte{0x12, 0x34, 0x56, 0x78, 0, 0, 0, 1, 0x45, 0, 0, 20}
	for _, next := range []layers.IPProtocol{layers.IPProtocolIPv6Destination, layers.IPProtocolIPv6HopByHop} {
		body := append([]byte{50, 0, 0, 0, 0, 0, 0, 0}, w...)
		events, _ := espNativeReplay(t, [][]byte{espNativeFrame(t, body, true, next)}, 0, false)
		require.Len(t, events, 1)
		require.Equal(t, "esp", events[0].Protocol)
		require.Equal(t, w, events[0].Raw)
	}
	for _, v6 := range []bool{false, true} {
		ah := []byte{50, 1, 0, 0, 0, 0, 0, 7, 0, 0, 0, 8}
		if v6 {
			ah[1] = 2
			ah = append(ah, 0, 0, 0, 0)
		}
		body := append(ah, w...)
		events, _ := espNativeReplay(t, [][]byte{espNativeFrame(t, body, v6, layers.IPProtocolAH)}, 0, false)
		require.Len(t, events, 1)
		require.Equal(t, w, events[0].Raw)
		require.Equal(t, false, events[0].Session["Authentication Verified"])
	}
}
