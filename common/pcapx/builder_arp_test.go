package pcapx

import (
	"fmt"
	"net"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/log"
)

func TestSmoking_Arp(t *testing.T) {
	// The host default route may use a VPN tunnel with no Ethernet address.
	// Give the auto builder a deterministic Ethernet route so this verifies
	// packet construction without relying on the developer machine's route.
	oldIface, oldGateway, oldAddress := PublicInterface, PublicGatewayAddress, PublicPreferredAddress
	PublicInterface = &net.Interface{HardwareAddr: net.HardwareAddr{2, 0, 0, 0, 0, 1}}
	PublicGatewayAddress = net.IPv4(192, 0, 2, 1)
	PublicPreferredAddress = net.IPv4(192, 0, 2, 2)
	t.Cleanup(func() {
		PublicInterface, PublicGatewayAddress, PublicPreferredAddress = oldIface, oldGateway, oldAddress
	})
	packets, err := PacketBuilder(
		WithArp_RequestAuto("8.8.8.8"),
	)
	if err != nil {
		panic(err)
	}
	packet := gopacket.NewPacket(packets, layers.LayerTypeEthernet, gopacket.Default)
	if packet.ErrorLayer() != nil {
		log.Infof("error layer: %v", packet.ErrorLayer().Error())
		panic(packet.ErrorLayer().Error())
	}
	if arp := packet.Layer(layers.LayerTypeARP); arp.LayerType() != layers.LayerTypeARP {
		panic("build arp error")
	}
	fmt.Println(packet.String())
}
