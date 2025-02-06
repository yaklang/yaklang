package netstackvm

import (
	"errors"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func (vm *NetStackVirtualMachine) DisallowTCP(destinationAddr string) {
	vm.driver.DisallowTCP(destinationAddr)
}

func (vm *NetStackVirtualMachine) AllowTCP(destinationAddr string) {
	vm.driver.AllowTCP(destinationAddr)
}

func (vm *NetStackVirtualMachine) DisallowTCPWithSrc(destinationAddr string, srcAddr string) {
	vm.driver.DisallowTCPWithSrc(destinationAddr, srcAddr)
}

func (vm *NetStackVirtualMachine) AllowTCPWithSrc(destinationAddr string, srcAddr string) {
	vm.driver.AllowTCPWithSrc(destinationAddr, srcAddr)
}

func (driver *PCAPEndpoint) generateRSTFromPacket(pkt gopacket.Packet) (bool, error) {
	tcpLayerRaw := pkt.Layer(layers.LayerTypeTCP)
	if tcpLayerRaw == nil {
		return false, errors.New("tcp layer not found")
	}
	tcpLayer, ok := tcpLayerRaw.(*layers.TCP)
	if !ok {
		return false, errors.New("tcp layer not found")
	}

	networkLayerRaw := pkt.Layer(layers.LayerTypeIPv4)
	if networkLayerRaw == nil {
		return false, errors.New("ip layer not found")
	}
	networkLayer, ok := networkLayerRaw.(*layers.IPv4)
	if !ok {
		return false, errors.New("ip layer not found")
	}

	srcIP := networkLayer.SrcIP.String()
	srcPort := int(tcpLayer.SrcPort)
	dstIP := networkLayer.DstIP.String()
	dstPort := int(tcpLayer.DstPort)

	hashes := map[string]struct{}{}
	for _, h := range driver.generateKillTCPHash(utils.HostPort(dstIP, dstPort), utils.HostPort(srcIP, srcPort)) {
		hashes[h] = struct{}{}
	}
	for _, h := range driver.generateKillTCPHash(utils.HostPort(dstIP, dstPort), "") {
		hashes[h] = struct{}{}
	}

	driver.tcpKillMutex.RLock()
	defer driver.tcpKillMutex.RUnlock()

	matched := false
	for h := range hashes {
		_, ok := driver.tcpKillMap[h]
		if ok {
			matched = true
			break
		}
	}
	if !matched {
		return false, nil
	}

	// 生成 RST 包
	rst := &layers.TCP{
		SrcPort: tcpLayer.DstPort,
		DstPort: tcpLayer.SrcPort,
		Seq:     tcpLayer.Ack,
		Ack:     tcpLayer.Seq,
		RST:     true,
		Window:  0,
		Urgent:  0,
		Options: []layers.TCPOption{},
	}

	ip := &layers.IPv4{
		Version:  4,
		TTL:      64,
		SrcIP:    networkLayer.DstIP,
		DstIP:    networkLayer.SrcIP,
		Protocol: layers.IPProtocolTCP,
	}

	linkLayer := pkt.Layer(layers.LayerTypeEthernet)
	if linkLayer == nil {
		return false, errors.New("ethernet layer not found")
	}
	eth := linkLayer.(*layers.Ethernet)

	newEth := &layers.Ethernet{
		SrcMAC:       eth.DstMAC,
		DstMAC:       eth.SrcMAC,
		EthernetType: eth.EthernetType,
	}

	// 序列化数据包
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{
		ComputeChecksums: true,
		FixLengths:       true,
	}

	// 计算 TCP 校验和
	rst.SetNetworkLayerForChecksum(ip)

	if err := gopacket.SerializeLayers(buf, opts, newEth, ip, rst); err != nil {
		log.Errorf("序列化 RST 数据包失败: %v", err)
		return false, err
	}

	// 发送 RST 数据包
	if err := driver.adaptor.WritePacketData(buf.Bytes()); err != nil {
		log.Errorf("发送 RST 数据包失败: %v", err)
		return false, err
	}

	return true, nil
}
