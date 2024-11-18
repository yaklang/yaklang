package arpx

import (
	"context"
	"github.com/gopacket/gopacket"
	"net"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/arptable"

	"github.com/gopacket/gopacket/layers"
	"github.com/mdlayher/arp"
	"github.com/pkg/errors"

	_ "github.com/yaklang/yaklang/common/utils/arptable"
)

func Arp(ifaceName string, target string) (net.HardwareAddr, error) {
	return ArpWithContext(utils.TimeoutContext(5*time.Second), ifaceName, target)
}

func ArpWithTimeout(timeoutContext time.Duration, ifaceName string, target string) (net.HardwareAddr, error) {
	return ArpWithContext(utils.TimeoutContext(timeoutContext), ifaceName, target)
}

var (
	arpTableTTLCache = utils.NewTTLCache[net.HardwareAddr](30 * time.Minute)

	TargetIsLoopback = utils.Error("loopback")
	LinkTypeIsNull   = utils.Error("link type is null")
)

func ArpWithContext(ctx context.Context, ifaceName string, target string) (net.HardwareAddr, error) {
	if arpTableTTLCache != nil {
		if hw, ok := arpTableTTLCache.Get(target); ok {
			return hw, nil
		}
	}

	hw, _ := arptable.SearchHardware(target)
	if hw != nil && hw.String() != "" {
		arpTableTTLCache.Set(target, hw)
		return hw, nil
	}

	r, err := ArpIPAddressesWithContext(ctx, ifaceName, target)
	if err != nil {
		return nil, err
	}

	if r != nil {
		res, ok := r[target]
		if ok {
			arpTableTTLCache.Set(target, res)
			return res, nil
		}
	}
	return nil, utils.Error("empty result")
}

func arpDial(ctx context.Context, ifaceName string, addrs string) (map[string]net.HardwareAddr, error) {
	ddl, ok := ctx.Deadline()
	if !ok {
		ddl = time.Now().Add(5 * time.Second)
	}

	// 获取 iface，针对这个 iface 创建一个 arpx 客户端
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return nil, err
	}
	client, err := arp.Dial(iface)
	if err != nil {
		return nil, utils.Errorf("ARP Dial error: %v", err)
	}
	defer client.Close()
	_ = client.SetDeadline(ddl)

	// 并发获取 arpx 包
	results := new(sync.Map)
	wg := new(sync.WaitGroup)
	for _, target := range utils.ParseStringToHosts(addrs) {
		target := target
		wg.Add(1)
		go func() {
			defer wg.Done()

			if hw, ok := arpTableTTLCache.Get(target); ok {
				results.Store(target, hw)
				return
			}

			hwAddr, err := arptable.SearchHardware(target)
			if err != nil {
				log.Debugf("")
			}
			if hwAddr != nil {
				results.Store(target, hwAddr)
				arpTableTTLCache.Set(target, hwAddr)
				return
			}

			targetIp := net.ParseIP(target)
			if targetIp == nil {
				log.Debugf("invalid target: %s", targetIp)
				return
			}

			hw, err := client.Resolve(targetIp)
			if err != nil {
				log.Debugf("resolve arpx for %v failed: %s", targetIp.String(), err)
			}
			if hw != nil {
				results.Store(target, hw)
				arpTableTTLCache.Set(target, hw)
				return
			}
		}()
	}
	wg.Wait()
	//for {
	//	select {
	//	case <-time.Tick(1 * time.Second):
	//		hw, _ := client.Resolve(targetIp)
	//		if hw != nil {
	//			return hw, nil
	//		}
	//	case <-newCtx.Done():
	//		return nil, Errorf("cannot found hw for %s", targetIp)
	//	}
	//}
	finalResult := make(map[string]net.HardwareAddr)
	results.Range(func(key, value interface{}) bool {
		finalResult[key.(string)] = value.(net.HardwareAddr)
		return true
	})
	return finalResult, nil
}

func ArpIPAddressesWithContext(ctx context.Context, ifaceName string, addrs string) (map[string]net.HardwareAddr, error) {
	var resultsMap = make(map[string]net.HardwareAddr)
	var err error
	if runtime.GOOS != "windows" {
		resultsMap, err = arpDial(ctx, ifaceName, addrs)
		if err != nil {
			log.Errorf("use arpx.Dial for send packet failed: %s", err)
		}
		if resultsMap != nil && len(resultsMap) > 0 {
			for ip, hw := range resultsMap {
				arpTableTTLCache.Set(ip, hw)
			}
			return resultsMap, nil
		}
	}
	resultsMap, err = ArpWithPcap(ctx, ifaceName, addrs)
	if err != nil {
		log.Errorf("send arpx request with pcap failed: %s", err)
	}
	if resultsMap != nil && len(resultsMap) > 0 {
		for ip, hw := range resultsMap {
			arpTableTTLCache.Set(ip, hw)
		}
		return resultsMap, nil
	}
	return nil, utils.Errorf("cannot fetch (%v) %v 's mac address", ifaceName, addrs)
}

var ipLoopback = make(map[string]interface{})

func init() {
	addrs, err := net.Interfaces()
	if err != nil {
		return
	}
	for _, i := range addrs {
		ret, _ := i.Addrs()
		for _, addr := range ret {
			ipNet, ok := addr.(*net.IPNet)
			if ok {
				ipLoopback[ipNet.IP.String()] = ipNet
			}
		}
	}
}

func IsLoopback(t string) bool {
	ipInstance := net.ParseIP(utils.FixForParseIP(t))
	if ipInstance != nil {
		if ipInstance.IsLoopback() {
			return true
		}
	}

	if strings.HasPrefix(utils.FixForParseIP(t), "127.") {
		return true
	} else {
		_, ok := ipLoopback[utils.FixForParseIP(t)]
		return ok
	}
}

func newArpARPPacket(iface *net.Interface, ip string) ([]byte, error) {
	ipIns := net.ParseIP(ip)
	if ipIns == nil {
		return nil, utils.Errorf("parse ip[%v] failed", ip)
	}

	addrs, err := iface.Addrs()
	if err != nil {
		return nil, utils.Errorf("fetch src ip failed: %s", err)
	}
	var src net.IP
	for _, addr := range addrs {
		ip := addr.(*net.IPNet).IP

		if utils.IsIPv6(ip.String()) {
			src = ip
		}
		if utils.IsIPv4(ip.String()) {
			src = ip
			break
		}
	}
	if src == nil {
		return nil, utils.Errorf("iface[%v] 's ip cannot be found", iface.Name)
	}

	srcMac := iface.HardwareAddr
	srcIP := src.To4()
	eth := &layers.Ethernet{
		SrcMAC:       srcMac,
		DstMAC:       net.HardwareAddr{0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
		EthernetType: layers.EthernetTypeARP,
	}
	arpFrame := &layers.ARP{
		AddrType:          layers.LinkTypeEthernet,
		Protocol:          layers.EthernetTypeIPv4,
		HwAddressSize:     6,
		ProtAddressSize:   4,
		Operation:         layers.ARPRequest,
		SourceHwAddress:   iface.HardwareAddr,
		SourceProtAddress: []byte(srcIP),
		DstHwAddress:      make([]byte, 6),
		DstProtAddress:    []byte(ipIns.To4()),
	}
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{
		FixLengths:       true,
		ComputeChecksums: true,
	}
	err = gopacket.SerializeLayers(buf, opts, eth, arpFrame)
	if err != nil {
		return nil, errors.Errorf("serialize arpx packet failed: %s", err)
	}
	return buf.Bytes(), nil
}

//func ARPWithPcap(ctx context.Context, ifaceName string, targets string) (map[string]net.HardwareAddr, error) {
//	ifaceIns, err := net.InterfaceByName(ifaceName)
//	if err != nil {
//		return nil, utils.Errorf("find interface by name failed: %s", ifaceName)
//	}
//	pcapName, err := _ifaceNameToPcapIfaceName(ifaceName)
//	if err != nil {
//		log.Errorf("find pcap name failed: %s", err)
//		return nil, utils.Errorf("find pcap name failed: %v", err)
//	}
//
//	handler, err := pcap.OpenLive(pcapName, 65535, true, pcap.BlockForever)
//	if err != nil {
//		return nil, utils.Errorf("pcap open live %v failed: %s", pcapName, err)
//	}
//
//	log.Infof(`Arp With Pcap in %v, LinkType: %v`, ifaceName, handler.LinkType())
//
//	expr := "arp"
//	err = handler.SetBPFFilter(expr)
//	if err != nil {
//		return nil, utils.Errorf("bind bpf(%v) filter failed: %s with name: %v", expr, err, "- "+ifaceName)
//	}
//
//	results := make(map[string]net.HardwareAddr)
//
//	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
//	defer cancel()
//	srcs := gopacket.NewPacketSource(handler, handler.LinkType())
//	packets := srcs.Packets()
//
//	targetsList := utils.ParseStringToHosts(targets)
//	if targetsList == nil {
//		return nil, utils.Errorf("cannot fetch hosts: %v", targets)
//	}
//
//	wg := new(sync.WaitGroup)
//	wg.Add(1)
//	go func() {
//		defer wg.Done()
//		for {
//			select {
//			case <-ctx.Done():
//				return
//			case packet, ok := <-packets:
//				if !ok {
//					return
//				}
//
//				arpPacket := packet.Layer(layers.LayerTypeARP)
//				if arpPacket == nil {
//					continue
//				}
//
//				arpIns, ok := arpPacket.(*layers.ARP)
//				if !ok {
//					continue
//				}
//
//				if arpIns.Operation != layers.ARPReply || bytes.Equal([]byte(ifaceIns.HardwareAddr), arpIns.SourceHwAddress) {
//					continue
//				}
//
//				ipAddr := fmt.Sprintf("%v", net.IP(arpIns.SourceProtAddress))
//				hwAddr := net.HardwareAddr(arpIns.SourceHwAddress)
//				log.Debugf("IP[%v] 's mac addr: %v", ipAddr, hwAddr)
//				results[ipAddr] = hwAddr
//			}
//		}
//	}()
//
//	for _, p := range targetsList {
//		if utils.IsIPv4(p) {
//			buf, err := newArpARPPacket(ifaceIns, p)
//			if err != nil {
//				log.Errorf("create arpx packet [%v for %v] failed: %s", ifaceName, p, err)
//				continue
//			}
//			err = handler.WritePacketData(buf.Bytes())
//			if err != nil {
//				log.Errorf("write arpx[%v] request packet to %v failed", p, ifaceName)
//				continue
//			}
//		}
//	}
//
//	wg.Wait()
//	return results, nil
//}
