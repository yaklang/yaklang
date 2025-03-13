package pcapx

import (
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

func TestTCPReSetter(t *testing.T) {
	packet := `f02f4b09df5994d9b31db46a0800450000867fbd40002e0607309d9466c2c0a800862ee3d97b7c75fe74e7165bc68018002aca7b00000101080a6bb8aa7eeaee7e474f26a250c14ffa176111b4c149bfc80f3436038f849392c6463f4ac86ab3e345b60502fb5becd1d30a40925473657a9f40ef123d7d86e57ffb5691e6873d04b17b098dbd0f40f18c013350847c586d0b189b`
	raw, _ := codec.DecodeHex(packet)
	results, err := GenerateTCPRST(raw)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, 2, len(results))
}
func TestReset(t *testing.T) {
	tmp := `38d57a2fbe7df84d8991af52080045000028000000004006f378c0a80304c0a803031f99eecf01a4a9f0bf19321d501010006d480000`
	raw, _ := codec.DecodeHex(tmp)
	packet := gopacket.NewPacket(raw, layers.LayerTypeEthernet, gopacket.DecodeOptions{
		// default config for packet
	})
	ether := packet.LinkLayer()
	ip := packet.NetworkLayer()
	tcp := packet.TransportLayer()
	// layerIp := ip.(*layers.IPv4)
	// // layerIp.SrcIP = net.ParseIP("192.168.3.3")
	// // layerIp.DstIP = net.ParseIP("192.168.3.4")
	// layerTcp := tcp.(*layers.TCP)
	// layerTcp.Seq = 2
	// layerTcp.SrcPort = 2080
	// layerTcp.DstPort = 56969
	// forged := forgeReset(packet)
	results, err := buildRST(ether.(*layers.Ethernet), ip.(*layers.IPv4), tcp.(*layers.TCP))
	if err != nil {
		t.Fatal(err)
	}
	result := results[0]
	packetData := gopacket.NewSerializeBuffer()
	gopacket.SerializeLayers(packetData, defaultGopacketSerializeOpt, result...)
	InjectRaw(packetData.Bytes(), WithIface("en0"))
}
