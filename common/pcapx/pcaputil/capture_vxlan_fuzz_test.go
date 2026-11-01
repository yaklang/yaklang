package pcaputil

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func FuzzVXLANHeaderAdmissionNeverInventsChild(f *testing.F) {
	header := []byte{8, 0, 0, 0, 0, 0, 42, 0}
	f.Add([]byte{})
	for size := 1; size <= 8; size++ {
		f.Add(bytes.Clone(header[:size]))
	}
	for _, flags := range []byte{0, 8, 0x88, 0xf7, 0xff} {
		seed := []byte{flags, 0x48, 0x12, 0x34, 0xab, 0xcd, 0xef, 0xff}
		f.Add(append(seed, make([]byte, 14)...))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 4096 {
			return
		}
		packet := gopacket.NewPacket(data, protocolPacketDecoder{decoder: layers.LayerTypeVXLAN}, gopacket.Default)
		v, ok := packet.Layer(layers.LayerTypeVXLAN).(*layers.VXLAN)
		require.True(t, ok)
		if len(data) >= 7 {
			require.Equal(t, uint32(data[4])<<16|uint32(data[5])<<8|uint32(data[6]), v.VNI)
		} else {
			require.Zero(t, v.VNI, "an incomplete identifier cannot inherit prior VNI state")
		}
		if len(data) > 0 && data[0]&0x80 != 0 && len(data) >= 4 {
			require.True(t, v.GBPExtension)
			require.Equal(t, data[1]&8 != 0, v.GBPApplied)
			require.Equal(t, data[1]&0x40 != 0, v.GBPDontLearn)
			require.Equal(t, binary.BigEndian.Uint16(data[2:4]), v.GBPGroupPolicyID)
		} else {
			require.False(t, v.GBPApplied)
			require.False(t, v.GBPDontLearn)
			require.Zero(t, v.GBPGroupPolicyID)
		}
		if len(data) < 8 || data[0]&8 == 0 || len(data)-8 < 14 {
			var failure *vxlanHeaderError
			require.NotNil(t, packet.ErrorLayer())
			require.ErrorAs(t, packet.ErrorLayer().Error(), &failure)
			require.Same(t, v, failure.layer)
			require.Nil(t, packet.Layer(layers.LayerTypeEthernet))
			require.Nil(t, packet.NetworkLayer())
			require.Nil(t, packet.TransportLayer())
			return
		}
		// An arbitrary inner frame need not be valid. Nevertheless, changing
		// receive-ignored reserved fields must not change its decoder path or
		// errors; GBP observation must not become forwarding-policy execution.
		plain := bytes.Clone(data)
		plain[0], plain[1], plain[2], plain[3], plain[7] = 8, 0, 0, 0, 0
		control := gopacket.NewPacket(plain, protocolPacketDecoder{decoder: layers.LayerTypeVXLAN}, gopacket.Default)
		var actualLayers, controlLayers []gopacket.LayerType
		for _, layer := range packet.Layers() {
			actualLayers = append(actualLayers, layer.LayerType())
		}
		for _, layer := range control.Layers() {
			controlLayers = append(controlLayers, layer.LayerType())
		}
		require.Equal(t, controlLayers, actualLayers)
		if failure := control.ErrorLayer(); failure != nil {
			require.NotNil(t, packet.ErrorLayer())
			require.Equal(t, failure.Error().Error(), packet.ErrorLayer().Error().Error())
		} else {
			require.Nil(t, packet.ErrorLayer())
		}
	})
}
