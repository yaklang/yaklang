package pcaputil

import (
	"bytes"
	"fmt"
	"sync"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func TestIPv6OptionsStayInsideDeclaredHeaderAndKeepFinalPad1(t *testing.T) {
	w := []byte{0x12, 0x34, 0x56, 0x78, 0, 0, 0, 1, 1, 2, 3, 4, 5, 6, 7, 8}
	cases := []struct {
		name   string
		header []byte
		valid  bool
	}{
		{"Pad1", []byte{50, 0, 0, 0, 0, 0, 0, 0}, true},
		{"PadN", []byte{50, 0, 1, 4, 0, 0, 0, 0}, true},
		{"unknown-and-final-Pad1", []byte{50, 0, 0x1e, 3, 1, 2, 3, 0}, true},
		{"crossing", []byte{50, 0, 1, 7, 0, 0, 0, 0}, false},
		{"value-truncated", []byte{50, 0, 1, 255, 0, 0, 0, 0}, false},
		{"length-in-next-header", []byte{50, 0, 0, 0, 0, 0, 0, 0x1e}, false},
	}
	for _, c := range cases {
		for _, next := range []layers.IPProtocol{layers.IPProtocolIPv6HopByHop, layers.IPProtocolIPv6Destination} {
			t.Run(fmt.Sprintf("%s/%d", c.name, next), func(t *testing.T) {
				frame := espNativeFrame(t, append(bytes.Clone(c.header), w...), true, next)
				observer := gopacket.NewPacket(frame, CaptureLinkDecoder(layers.LinkTypeEthernet), gopacket.Default)
				native := protocolAnalysisPacket(observer, nil)
				for _, packet := range []gopacket.Packet{observer, native} {
					if c.valid {
						require.Nil(t, packet.ErrorLayer())
						require.NotNil(t, packet.Layer(layers.LayerTypeIPSecESP))
					} else {
						require.NotNil(t, packet.ErrorLayer())
						require.Nil(t, packet.Layer(layers.LayerTypeIPSecESP))
					}
				}
				events, _ := espNativeReplay(t, [][]byte{frame}, 0, false)
				require.Len(t, events, 1)
				if c.valid {
					require.Equal(t, "esp", events[0].Protocol)
					require.Equal(t, w, events[0].Raw)
				} else {
					require.Equal(t, "ip", events[0].Protocol)
					require.Equal(t, "IPv6OptionsHeaderBounds", events[0].ExpertCode)
				}
			})
		}
	}
	// Pad1 at EOF is one byte, not a truncated two-byte TLV.
	for _, header := range [][]byte{cases[0].header, cases[2].header} {
		var feedback protocolDecodeFeedback
		n, options, err := boundedIPv6Options(header, &feedback)
		require.NoError(t, err)
		require.Equal(t, 8, n)
		require.False(t, feedback.truncated)
		require.Equal(t, 8, options[len(options)-1].end)
	}
}

func TestIPsecRefusalsPreserveStrictCaptureAPIAndNoOuterUDPDispatch(t *testing.T) {
	w := []byte{0x12, 0x34, 0x56, 0x78, 0, 0, 0, 1, 1, 2, 3, 4, 5, 6, 7, 8}
	badAH := append([]byte{50, 1, 0, 0, 0, 0, 0, 7, 0, 0, 0, 8}, w...)
	badOption := append([]byte{50, 0, 1, 7, 0, 0, 0, 0}, w...)
	zeroSPI := bytes.Clone(w)
	clear(zeroSPI[:4])
	cases := []struct {
		name        string
		frame       []byte
		decodeError bool
	}{
		{"short-ESP", espNativeFrame(t, w[:3], false, layers.IPProtocolESP), true},
		{"crossing-option", espNativeFrame(t, badOption, true, layers.IPProtocolIPv6Destination), true},
		{"AH-IPv6-12", espNativeFrame(t, badAH, true, layers.IPProtocolAH), true},
		{"SPI-zero-semantic-only", espNativeFrame(t, zeroSPI, false, layers.IPProtocolESP), false},
		{"opaque-header", espNativeFrame(t, w, false, layers.IPProtocolESP), false},
	}
	for _, c := range cases {
		for _, tunnel := range []bool{false, true} {
			for _, protocols := range []bool{false, true} {
				for _, allow := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/tunnel%v/protocols%v/allow%v", c.name, tunnel, protocols, allow), func(t *testing.T) {
						frame := c.frame
						if tunnel {
							frame = vxlanProfileTestOuter(t, append(vxlanProfileTestHeader(8, 0, 0, 42, 0), frame...), 55000, 4789, false)
						}
						var stats TCPReassemblyStats
						var events []*ProtocolEvent
						var mu sync.Mutex
						observed, flows := 0, 0
						opts := []CaptureOption{WithTCPReassemblyOptions(TCPReassemblyOptions{AllowIncomplete: allow, Stream: true}), WithOnTCPReassemblyStats(func(s TCPReassemblyStats) { stats = s }), WithEveryPacket(func(gopacket.Packet) { mu.Lock(); defer mu.Unlock(); observed++ }), WithOnTrafficFlowCreated(func(*TrafficFlow) { mu.Lock(); defer mu.Unlock(); flows++ })}
						if protocols {
							opts = append(opts, WithOnProtocolMessage(func(e *ProtocolEvent) { mu.Lock(); defer mu.Unlock(); events = append(events, e) }))
						}
						err := ReplayPcap(bytes.NewReader(icmpFragmentCapture(t, [][]byte{frame})), opts...)
						if c.decodeError && !allow {
							require.Error(t, err)
						} else {
							require.NoError(t, err)
						}
						if c.decodeError {
							require.EqualValues(t, 1, stats.DecodeErrors)
						} else {
							require.Zero(t, stats.DecodeErrors)
						}
						require.Equal(t, 1, observed)
						require.Zero(t, flows)
						if protocols {
							require.Len(t, events, 1)
							require.NotEqual(t, "udp", events[0].Transport, "a refused inner carrier cannot be retried as the outer UDP application")
						}
					})
				}
			}
		}
	}
}

func TestAHAlignmentUsesInnermostIPv6AcrossOptions(t *testing.T) {
	w := []byte{0x12, 0x34, 0x56, 0x78, 0, 0, 0, 1, 1, 2, 3, 4, 5, 6, 7, 8}
	for _, size := range []int{12, 16, 20, 24} {
		for _, option := range []bool{false, true} {
			ah := make([]byte, size)
			ah[0] = 50
			ah[1] = byte(size/4 - 2)
			ah[7] = 7
			body := append(ah, w...)
			next := layers.IPProtocolAH
			if option {
				body = append([]byte{51, 0, 0, 0, 0, 0, 0, 0}, body...)
				next = layers.IPProtocolIPv6Destination
			}
			frame := espNativeFrame(t, body, true, next)
			observer := gopacket.NewPacket(frame, CaptureLinkDecoder(layers.LinkTypeEthernet), gopacket.Default)
			for _, packet := range []gopacket.Packet{observer, protocolAnalysisPacket(observer, nil)} {
				if size%8 == 0 {
					require.Nil(t, packet.ErrorLayer())
					require.NotNil(t, packet.Layer(layers.LayerTypeIPSecESP))
				} else {
					require.NotNil(t, packet.ErrorLayer())
					require.Nil(t, packet.Layer(layers.LayerTypeIPSecESP))
				}
			}
		}
	}
}

func TestIPsecErrorBehindUnselectedParentDoesNotOverrideAdmission(t *testing.T) {
	inner := espNativeFrame(t, []byte{1, 2, 3}, false, layers.IPProtocolESP)
	outer := vxlanProfileTestOuter(t, append([]byte{0, 0x80, 0x65, 0x58, 0, 0, 7, 0}, inner...), 55001, 6081, false)
	var events []*ProtocolEvent
	var stats TCPReassemblyStats
	require.NoError(t, ReplayPcap(bytes.NewReader(icmpFragmentCapture(t, [][]byte{outer})), WithTCPReassemblyOptions(TCPReassemblyOptions{AllowIncomplete: false}), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnTCPReassemblyStats(func(s TCPReassemblyStats) { stats = s })))
	require.Zero(t, stats.DecodeErrors)
	require.Len(t, events, 1)
	require.Equal(t, "EncapsulationDepthOrProfile", events[0].ExpertCode)
	require.Equal(t, "ip", events[0].Protocol)
}

func TestBadHopByHopKeepsValidatedFixedParentAndCapturedSource(t *testing.T) {
	w := append([]byte{50, 0, 1, 7, 0, 0, 0, 0}, []byte{0x12, 0x34, 0x56, 0x78, 0, 0, 0, 1, 1, 2, 3, 4, 5, 6, 7, 8}...)
	inner := espNativeFrame(t, w, true, layers.IPProtocolIPv6HopByHop)
	for _, nested := range []bool{false, true} {
		frame := inner
		if nested {
			frame = espNativeFrame(t, inner[14:], false, layers.IPProtocolIPv6)
		}
		observer := gopacket.NewPacket(frame, CaptureLinkDecoder(layers.LinkTypeEthernet), gopacket.Default)
		for _, packet := range []gopacket.Packet{observer, protocolAnalysisPacket(observer, nil)} {
			var parent *layers.IPv6
			for _, layer := range packet.Layers() {
				if ip, ok := layer.(*layers.IPv6); ok {
					parent = ip
				}
			}
			require.NotNil(t, parent)
			require.Equal(t, "2001:db8::1", parent.SrcIP.String())
			require.Equal(t, "2001:db8::2", parent.DstIP.String())
			require.Len(t, parent.Contents, 40)
			require.Equal(t, w, parent.Payload)
			require.Nil(t, parent.HopByHop)
			require.NotNil(t, packet.ErrorLayer())
		}
		events, _ := espNativeReplay(t, [][]byte{frame}, 0, false)
		require.Len(t, events, 1)
		require.Equal(t, "ip", events[0].Protocol)
		require.Equal(t, "IPv6OptionsHeaderBounds", events[0].ExpertCode)
		require.Equal(t, "2001:db8::1", events[0].Source)
		require.Equal(t, "2001:db8::2", events[0].Destination)
		require.Equal(t, []PacketReference{{Number: 1, Domain: events[0].Domain}}, events[0].SourceBytes.PacketRefs)
	}
}

func FuzzIPv6OptionsHeaderBounds(f *testing.F) {
	for _, w := range [][]byte{nil, {50}, {50, 0, 0, 0, 0, 0, 0, 0}, {50, 0, 1, 7, 0, 0, 0, 0, 0, 0, 0, 0}, {50, 0, 0x1e, 3, 1, 2, 3, 0}} {
		f.Add(w)
	}
	f.Fuzz(func(t *testing.T, w []byte) {
		var feedback protocolDecodeFeedback
		n, options, err := boundedIPv6Options(w, &feedback)
		if err != nil {
			require.Zero(t, n)
			require.Nil(t, options)
			return
		}
		require.GreaterOrEqual(t, len(w), n)
		require.Equal(t, (int(w[1])+1)*8, n)
		at := 2
		for _, option := range options {
			require.Equal(t, at, option.start)
			require.LessOrEqual(t, option.end, n)
			require.Greater(t, option.end, option.start)
			if option.kind == 0 {
				require.Equal(t, at+1, option.end)
			} else {
				require.Equal(t, at+2+int(w[at+1]), option.end)
			}
			at = option.end
		}
		require.Equal(t, n, at)
	})
}
