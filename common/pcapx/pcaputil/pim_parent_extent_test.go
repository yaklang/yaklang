package pcaputil

import (
	"bytes"
	"encoding/binary"
	"sync"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
)

func TestPIMRegisterCannotAdmitChildrenFromAnIncompleteParent(t *testing.T) {
	for _, v6 := range []bool{false, true} {
		for _, whole := range []bool{false, true} {
			for _, declaredDelta := range []int{0, 7} {
				wire := bytes.Clone(pimTestFrame(t, v6, false, whole))
				h := wire[14:]
				if v6 {
					if declaredDelta == 0 {
						// A zero IPv6 payload length selects the separate jumbo
						// context; it does not authorize this direct PIM profile.
						binary.BigEndian.PutUint16(h[4:6], 0)
					} else {
						binary.BigEndian.PutUint16(h[4:6], binary.BigEndian.Uint16(h[4:6])+uint16(declaredDelta))
					}
				} else {
					if declaredDelta == 0 {
						binary.BigEndian.PutUint16(h[2:4], 0)
					} else {
						binary.BigEndian.PutUint16(h[2:4], binary.BigEndian.Uint16(h[2:4])+uint16(declaredDelta))
					}
					h[10], h[11] = 0, 0
					binary.BigEndian.PutUint16(h[10:12], pimChecksum(h[:20], nil))
				}
				p := gopacket.NewPacket(wire, CaptureLinkDecoder(layers.LinkTypeEthernet), gopacket.Default)
				require.Nil(t, p.Layer(layers.LayerTypeUDP), "an inner UDP-shaped prefix is not an admitted packet")
				require.NotContains(t, PacketDomain(wire, gopacket.CaptureInfo{}, layers.LinkTypeEthernet).Encapsulation, "/pim-register:")
				var capture bytes.Buffer
				writer := pcapgo.NewWriterNanos(&capture)
				require.NoError(t, writer.WriteFileHeader(65535, layers.LinkTypeEthernet))
				require.NoError(t, writer.WritePacket(gopacket.CaptureInfo{Timestamp: time.Unix(1700000000, 0), CaptureLength: len(wire), Length: len(wire)}, wire))
				var events []*ProtocolEvent
				var mu sync.Mutex
				err := ReplayPcap(bytes.NewReader(capture.Bytes()), WithOnProtocolMessage(func(e *ProtocolEvent) {
					mu.Lock()
					defer mu.Unlock()
					events = append(events, e)
				}))
				if v6 && declaredDelta == 0 {
					require.ErrorContains(t, err, "IPv6 zero payload length/jumbogram", "a real parent refusal remains observable")
				} else {
					require.NoError(t, err)
					require.NotEmpty(t, events, "retain the observed parent/field diagnostic")
				}
				for _, event := range events {
					require.NotContains(t, []string{"dns", "mdns", "udp"}, event.Protocol)
					require.Zero(t, event.FlowID)
					require.NotEqual(t, "decoded", event.Status)
					require.NotContains(t, event.Domain.Encapsulation, "/pim-register:")
				}
			}
		}
	}
}

func TestPIMHelloUnknownIPv4LengthIsNotRepairedByTheDecoder(t *testing.T) {
	wire := pimControlFrame(t, []byte{0x20, 0, 0, 0, 0, 1, 0, 2, 0, 105}, false)
	// Remove Ethernet padding, then change only the original total-length word
	// and its checksum. The complete Hello prefix and checksum remain intact.
	wire = wire[:14+int(binary.BigEndian.Uint16(wire[16:18]))]
	wire[16], wire[17], wire[24], wire[25] = 0, 0, 0, 0
	binary.BigEndian.PutUint16(wire[24:26], pimChecksum(wire[14:34], nil))
	p := gopacket.NewPacket(wire, CaptureLinkDecoder(layers.LinkTypeEthernet), gopacket.Default)
	ip := p.Layer(layers.LayerTypeIPv4).(*layers.IPv4)
	require.NotZero(t, ip.Length, "the dependency replaces zero with the available byte count")
	require.Equal(t, -1, pimDeclaredPayloadBytes(ip), "the literal wire boundary stays unknown")
	events := pimReplay(t, wire)
	require.Len(t, events, 1)
	require.Equal(t, "context-required", events[0].Status)
	require.Equal(t, false, events[0].Session["Checksum Verified"])
	require.Equal(t, []any{uint64(105)}, events[0].DisplayFields()["pim.holdtime"])
}
