package pcaputil

import (
	"encoding/binary"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func TestVLANIEEE8023STP(t *testing.T) {
	// A generated configuration BPDU: IEEE 802.3 length, LLC, then STP.
	bpdu := make([]byte, 35)
	copy(bpdu[5:], []byte{0x80, 0, 0, 1, 2, 3, 4, 5})
	copy(bpdu[17:], []byte{0x80, 0, 0, 1, 2, 3, 4, 6})
	copy(bpdu[25:], []byte{0x80, 1, 0, 0, 20, 0, 2, 0, 15, 0})
	llc := append([]byte{0x42, 0x42, 3}, bpdu...)
	eth := []byte{1, 0x80, 0xc2, 0, 0, 0, 2, 0, 0, 0, 0, 1, 0x81, 0}
	wire := append(append(eth, 0, 123, 0, byte(len(llc))), llc...)
	for _, decode := range []gopacket.Decoder{CaptureLinkDecoder(1), protocolPacketDecoder{decoder: layers.LinkTypeEthernet}} {
		p := gopacket.NewPacket(wire, decode, gopacket.Default)
		require.Nil(t, p.ErrorLayer())
		require.NotNil(t, p.Layer(layers.LayerTypeSTP))
		require.Equal(t, "/vlan:123", packetEvidence(p).Ref.Domain.Encapsulation)
	}
	bad := append([]byte(nil), wire...)
	binary.BigEndian.PutUint16(bad[16:], 200)
	p := gopacket.NewPacket(bad, CaptureLinkDecoder(1), gopacket.Default)
	require.NotNil(t, p.ErrorLayer())
	require.Nil(t, p.Layer(layers.LayerTypeSTP))
}
