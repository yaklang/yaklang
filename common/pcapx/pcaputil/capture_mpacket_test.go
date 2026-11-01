package pcaputil

import (
	"encoding/binary"
	"hash/crc32"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func TestMPacketExpressAdmission(t *testing.T) {
	eth := make([]byte, 60)
	copy(eth, []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 2, 0, 0, 0, 0, 1, 0x00, 0x2e})
	for _, preamble := range [][]byte{{0xd5}, {0x55, 0x55, 0xd5}, {0x50, 0x55, 0xd5}} {
		wire := append(append([]byte(nil), preamble...), eth...)
		wire = binary.LittleEndian.AppendUint32(wire, crc32.ChecksumIEEE(eth))
		p := gopacket.NewPacket(wire, CaptureLinkDecoder(274), gopacket.Default)
		require.NotNil(t, p.Layer(layers.LayerTypeEthernet))
		require.Equal(t, eth[:14], p.Layer(layers.LayerTypeEthernet).LayerContents())
		wire[len(wire)-1] ^= 1
		p = gopacket.NewPacket(wire, CaptureLinkDecoder(274), gopacket.Default)
		require.NotNil(t, p.ErrorLayer())
		require.Nil(t, p.Layer(layers.LayerTypeEthernet))
	}
	for _, wire := range [][]byte{{}, {0x55}, {0x55, 0xe6, 0}, {0xd5, 0, 0}} {
		p := gopacket.NewPacket(wire, CaptureLinkDecoder(274), gopacket.Default)
		require.NotNil(t, p.ErrorLayer())
		require.Nil(t, p.Layer(layers.LayerTypeEthernet))
	}
}
