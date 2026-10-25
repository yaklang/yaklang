package pcaputil

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func stpNativeWire() []byte {
	return []byte{0, 0, 2, 2, 0x3c, 0x80, 1, 2, 0, 0, 0, 0, 1, 0, 0, 0, 19, 0x90, 1, 2, 0, 0, 0, 0, 2, 0x80, 3, 0, 128, 20, 0, 2, 0, 15, 0, 0}
}
func stpNativeFrame(w []byte, pvst, vlan bool) []byte {
	carrier := []byte{0x42, 0x42, 3}
	if pvst {
		carrier = []byte{0xaa, 0xaa, 3, 0, 0, 12, 1, 11}
	}
	carrier = append(carrier, w...)
	frame := []byte{1, 0x80, 0xc2, 0, 0, 0, 2, 0, 0, 0, 0, 7}
	if vlan {
		frame = append(frame, 0x81, 0, 0, 7)
	}
	frame = append(frame, byte(len(carrier)>>8), byte(len(carrier)))
	frame = append(frame, carrier...)
	for len(frame) < 64 {
		frame = append(frame, 0xee)
	} // padding is not a BPDU trailer
	return frame
}

func TestSTPNativeCarrierFieldsAndNoAdvertisementInheritance(t *testing.T) {
	for _, pvst := range []bool{false, true} {
		for _, vlan := range []bool{false, true} {
			for _, deferred := range []bool{false, true} {
				c := NewDefaultConfig()
				var events []*ProtocolEvent
				require.NoError(t, WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) })(c))
				require.NoError(t, WithProtocolDeferred(deferred)(c))
				require.NoError(t, c.prepareBinParser())
				decode := func(w []byte, cut bool) bool {
					p := gopacket.NewPacket(w, CaptureLinkDecoder(layers.LinkTypeEthernet), gopacket.Default)
					ci := gopacket.CaptureInfo{Timestamp: time.Unix(7, 0), Length: len(w), CaptureLength: len(w)}
					if cut {
						ci.Length++
					}
					p.Metadata().CaptureInfo = withEvidence(ci, captureEvidence{Ref: PacketReference{Number: uint64(len(events) + 1), Domain: CaptureDomain{Interface: 7}}})
					_, handled := c.binParser.networkPacket(p)
					return handled
				}
				wire := stpNativeWire()
				if pvst {
					wire = append(wire, 0, 0, 0, 2, 0, 121)
				}
				require.True(t, decode(stpNativeFrame(wire, pvst, vlan), false))
				require.Len(t, events, 1)
				e := events[0]
				require.Contains(t, []string{"decoded", "deferred"}, e.Status, e.Error)
				require.Zero(t, e.FlowID)
				require.Equal(t, wire, e.Raw)
				require.Equal(t, "02:00:00:00:00:01", e.Session["Root MAC"])
				require.EqualValues(t, 128, e.Session["Message Age Ticks"])
				fields, err := e.GetFields()
				require.NoError(t, err)
				fields["Root MAC"] = "changed"
				e.Session["Root MAC"] = "changed"
				fields, err = e.GetFields()
				require.NoError(t, err)
				require.Equal(t, "02:00:00:00:00:01", fields["Root MAC"])
				// Display projection is read from the immutable owned semantic view.
				filter, err := CompileDisplayFilter("stp.version == 2 and stp.msg_age.ticks == 128")
				require.NoError(t, err)
				v := filter.EvaluateEvents([]*ProtocolEvent{e})
				require.True(t, v.Matched)
				require.True(t, v.Complete)
				require.True(t, decode(stpNativeFrame([]byte{0, 0, 0, 128}, pvst, vlan), false))
				require.NotContains(t, events[1].Session, "Root MAC")
				require.True(t, decode(stpNativeFrame(wire[:5], pvst, vlan), false))
				require.Equal(t, "malformed", events[2].Status)
				require.Empty(t, events[2].Session)
				require.True(t, decode(stpNativeFrame(wire, pvst, vlan), true))
				require.Equal(t, "incomplete", events[3].Status)
				require.Empty(t, events[3].Session)
				for _, bad := range []byte{0x40, 0x43, 0xff} {
					f := stpNativeFrame(wire, pvst, vlan)
					at := 14
					if vlan {
						at += 4
					}
					f[at+1] = bad
					before := len(events)
					decode(f, false)
					require.Len(t, events, before, "wrong SSAP cannot borrow a valid prior BPDU")
				}
				c.binParser.closeFragments()
				c.binParser.closeUDPSessions()
			}
		}
	}
}

func TestSTPCarrierLengthAndSNAPIdentityCannotBeGuessed(t *testing.T) {
	for _, vlan := range []bool{false, true} {
		wire := append(stpNativeWire(), 0, 0, 0, 2, 0, 121)
		frame := stpNativeFrame(wire, true, vlan)
		for _, index := range []int{3, 4, 5, 6, 7} {
			bad := bytes.Clone(frame)
			at := 14
			if vlan {
				at += 4
			}
			bad[at+index] ^= 1
			p := gopacket.NewPacket(bad, CaptureLinkDecoder(layers.LinkTypeEthernet), gopacket.Default)
			eth := p.Layer(layers.LayerTypeEthernet).(*layers.Ethernet)
			_, _, selected, _ := stpEthernetCarrier(eth)
			require.False(t, selected)
		}
		for _, length := range []uint16{2, 7, 500} {
			bad := bytes.Clone(frame)
			at := 12
			if vlan {
				at += 4
			}
			binary.BigEndian.PutUint16(bad[at:], length)
			p := gopacket.NewPacket(bad, CaptureLinkDecoder(layers.LinkTypeEthernet), gopacket.Default)
			eth := p.Layer(layers.LayerTypeEthernet).(*layers.Ethernet)
			w, _, selected, err := stpEthernetCarrier(eth)
			require.True(t, !selected || err != nil || len(w) < 4)
		}
	}
}

func TestSTPExactSecondsEmptyRegionAndLimitedByteAccounting(t *testing.T) {
	e := &ProtocolEvent{Protocol: "stp", Status: "decoded", Session: map[string]any{"MST Configuration Name": "", "Message Age Seconds": float64(1) / 256, "Message Age Ticks": uint64(1)}}
	v := e.DisplayFields()
	require.Equal(t, []any{""}, v["mstp.config_name"], "a present empty region name is not absent")
	require.Equal(t, []any{"0.00390625"}, v["stp.msg_age"])
	filter, err := CompileDisplayFilter(`mstp.config_name == "" and stp.msg_age.ticks == 1`)
	require.NoError(t, err)
	result := filter.EvaluateEvents([]*ProtocolEvent{e})
	require.True(t, result.Complete)
	require.True(t, result.Matched)
	var events []*ProtocolEvent
	c := NewDefaultConfig()
	require.NoError(t, WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) })(c))
	require.NoError(t, c.prepareBinParser())
	c.binParser.config.MaxMessageBytes = 16
	w := stpNativeWire()
	p := gopacket.NewPacket(stpNativeFrame(w, false, false), CaptureLinkDecoder(layers.LinkTypeEthernet), gopacket.Default)
	_, handled := c.binParser.networkPacket(p)
	require.True(t, handled)
	require.Len(t, events, 1)
	require.Equal(t, "limited", events[0].Status)
	require.Empty(t, events[0].Raw)
	require.Equal(t, len(w), events[0].Length)
	require.EqualValues(t, len(w), c.binParser.messageBytes.Load())
	require.EqualValues(t, len(w), c.binParser.limited.Load())
}
