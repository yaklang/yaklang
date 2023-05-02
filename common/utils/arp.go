package utils

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"
	"yaklang/common/log"
	"yaklang/common/utils/arptable"

	"github.com/ReneKroon/ttlcache"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"github.com/mdlayher/arp"
	"github.com/pkg/errors"

	_ "yaklang/common/utils/arptable"
)

func Arp(ifaceName string, target string) (net.HardwareAddr, error) {
	return ArpWithContext(TimeoutContext(5*time.Second), ifaceName, target)
}

func ArpWithTimeout(timeoutContext time.Duration, ifaceName string, target string) (net.HardwareAddr, error) {
	return ArpWithContext(TimeoutContext(timeoutContext), ifaceName, target)
}

var (
	TargetIsLoopback = Errorf("loopback")
)

type ConvertIfaceNameError struct {
	name string
}

func (e *ConvertIfaceNameError) Error() string {
	return fmt.Sprintf("convert iface name failed: %s", e.name)
}

func NewConvertIfaceNameError(name string) *ConvertIfaceNameError {
	return &ConvertIfaceNameError{
		name: name,
	}
}

func ArpWithContext(ctx context.Context, ifaceName string, target string) (net.HardwareAddr, error) {
	hw, _ := arptable.SearchHardware(target)
	if hw != nil && hw.String() != "" {
		if arpTableTTLCache != nil {
			arpTableTTLCache.Set(target, hw)
		}
		return hw, nil
	}

	if arpTableTTLCache != nil {
		if v, ok := arpTableTTLCache.Get(target); ok {
			if hw, ok := v.(net.HardwareAddr); ok {
				return hw, nil
			}
		}
	}

	r, err := ArpIPAddressesWithContext(ctx, ifaceName, target)
	if err != nil {
		return nil, err
	}

	if r != nil {
		res, ok := r[target]
		if ok {
			return res, nil
		}
	}
	return nil, Errorf("empty result")
}

var (
	arpTableTTLCacheCreateOnce = new(sync.Once)
	arpTableTTLCache           *ttlcache.Cache
)

func init() {
	arpTableTTLCacheCreateOnce.Do(func() {
		if arpTableTTLCache == nil {
			arpTableTTLCache = ttlcache.NewCache()
			arpTableTTLCache.SetTTL(30 * time.Minute)
		}
	})
}

func fetchLocalARPTable(addrs string) (map[string]net.HardwareAddr, error) {
	if IsLoopback(addrs) {
		return nil, TargetIsLoopback
	}
	results := make(map[string]net.HardwareAddr)

	for _, t := range ParseStringToHosts(addrs) {
		if IsLoopback(t) {
			continue
		}
		result, err := arptable.SearchHardware(t)
		if err != nil {
			log.Errorf("search-arp search hardware failed: %s", err)
			continue
		}
		results[t] = result
	}
	return results, nil
}

func arpDial(ctx context.Context, ifaceName string, addrs string) (map[string]net.HardwareAddr, error) {
	ddl, ok := ctx.Deadline()
	if !ok {
		ddl = time.Now().Add(5 * time.Second)
	}

	// 获取 iface，针对这个 iface 创建一个 arp 客户端
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return nil, err
	}
	client, err := arp.Dial(iface)
	if err != nil {
		return nil, Errorf("ARP Dial error: %v", err)
	}
	defer client.Close()
	_ = client.SetDeadline(ddl)

	// 并发获取 arp 包
	results := new(sync.Map)
	wg := new(sync.WaitGroup)
	for _, target := range ParseStringToHosts(addrs) {
		target := target
		wg.Add(1)
		go func() {
			defer wg.Done()

			if res, ok := arpTableTTLCache.Get(target); ok {
				results.Store(target, res.(net.HardwareAddr))
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
				//return nil,
			}

			hw, err := client.Resolve(targetIp)
			if err != nil {
				log.Debugf("resolve arp for %v failed: %s", targetIp.String(), err)
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
	resultsMap, err := arpDial(ctx, ifaceName, addrs)
	if err != nil {
		log.Errorf("use arp.Dial for send packet failed: %s", err)
	}
	if resultsMap != nil && len(resultsMap) > 0 {
		for ip, hw := range resultsMap {
			arpTableTTLCache.Set(ip, hw)
		}
		return resultsMap, nil
	}

	resultsMap, err = ARPWithPcap(ctx, ifaceName, addrs)
	if err != nil {
		log.Errorf("send arp request with pcap failed: %s", err)
	}
	if resultsMap != nil && len(resultsMap) > 0 {
		for ip, hw := range resultsMap {
			arpTableTTLCache.Set(ip, hw)
		}
		return resultsMap, nil
	}
	return nil, Errorf("cannot fetch (%v) %v 's mac address", ifaceName, addrs)
}

var (
	ipLoopback = make(map[string]interface{})
)

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
	ipInstance := net.ParseIP(FixForParseIP(t))
	if ipInstance != nil {
		if ipInstance.IsLoopback() {
			return true
		}
	}

	if strings.HasPrefix(FixForParseIP(t), "127.") {
		return true
	} else {
		_, ok := ipLoopback[FixForParseIP(t)]
		return ok
	}
}

func newArpARPPacket(iface *net.Interface, ip string) (gopacket.SerializeBuffer, error) {
	ipIns := net.ParseIP(ip)
	if ipIns == nil {
		return nil, Errorf("parse ip[%v] failed", ip)
	}

	srcIPS, err := iface.Addrs()
	if err != nil {
		return nil, Errorf("fetch src ip failed: %s", err)
	}
	var src net.IP
	haveSet := false
	for _, a := range srcIPS {
		ip, _, err := net.ParseCIDR(a.String())
		if err != nil {
			continue
		}
		if haveSet {
			break
		}
		if IsIPv4(ip.String()) {
			src = net.ParseIP(ip.String())
			haveSet = true
		}
	}
	if !haveSet {
		return nil, Errorf("iface[%v] 's ip cannot be found", iface.Name)
	}

	eth := &layers.Ethernet{SrcMAC: iface.HardwareAddr, DstMAC: net.HardwareAddr{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, EthernetType: layers.EthernetTypeARP}
	arp := &layers.ARP{
		AddrType:          layers.LinkTypeEthernet,
		Protocol:          layers.EthernetTypeIPv4,
		HwAddressSize:     6,
		ProtAddressSize:   4,
		Operation:         layers.ARPRequest,
		SourceHwAddress:   []byte(iface.HardwareAddr),
		SourceProtAddress: []byte(src.To4()[:4]),
		DstHwAddress:      []byte{0, 0, 0, 0, 0, 0},
		DstProtAddress:    []byte(ipIns.To4()[:4]),
	}
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{
		FixLengths:       true,
		ComputeChecksums: true,
	}
	err = gopacket.SerializeLayers(buf, opts, eth, arp)
	if err != nil {
		return nil, errors.Errorf("serialize arp packet failed: %s", err)
	}
	return buf, nil
}

func ARPWithPcap(ctx context.Context, ifaceName string, targets string) (map[string]net.HardwareAddr, error) {
	ifaceIns, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return nil, Errorf("find interface by name failed: %s", ifaceName)
	}
	pcapName, err := IfaceNameToPcapIfaceName(ifaceName)
	if err != nil {
		log.Errorf("find pcap name failed: %s", err)
		return nil, Errorf("find pcap name failed: %v", err)
	}

	hanldler, err := pcap.OpenLive(pcapName, 65535, false, pcap.BlockForever)
	if err != nil {
		return nil, Errorf("pcap open live %v failed: %s", pcapName, err)
	}

	err = hanldler.SetBPFFilter("arp")
	if err != nil {
		return nil, Errorf("bind bpf filter failed: %s", err)
	}

	results := make(map[string]net.HardwareAddr)

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	srcs := gopacket.NewPacketSource(hanldler, hanldler.LinkType())
	packets := srcs.Packets()

	targetsList := ParseStringToHosts(targets)
	if targetsList == nil {
		return nil, Errorf("cannot fetch hosts: %v", targets)
	}

	wg := new(sync.WaitGroup)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case packet, ok := <-packets:
				if !ok {
					return
				}

				arpPacket := packet.Layer(layers.LayerTypeARP)
				if arpPacket == nil {
					continue
				}

				arpIns, ok := arpPacket.(*layers.ARP)
				if !ok {
					continue
				}

				if arpIns.Operation != layers.ARPReply || bytes.Equal([]byte(ifaceIns.HardwareAddr), arpIns.SourceHwAddress) {
					continue
				}

				ipAddr := fmt.Sprintf("%v", net.IP(arpIns.SourceProtAddress))
				hwAddr := net.HardwareAddr(arpIns.SourceHwAddress)
				log.Debugf("IP[%v] 's mac addr: %v", ipAddr, hwAddr)
				results[ipAddr] = hwAddr
			}
		}
	}()

	for _, p := range targetsList {
		if IsIPv4(p) {
			buf, err := newArpARPPacket(ifaceIns, p)
			if err != nil {
				log.Errorf("create arp packet [%v for %v] failed: %s", ifaceName, p, err)
				continue
			}
			err = hanldler.WritePacketData(buf.Bytes())
			if err != nil {
				log.Errorf("write arp[%v] request packet to %v failed", p, ifaceName)
				continue
			}
		}
	}

	wg.Wait()
	return results, nil
}

func PcapInterfaceEqNetInterface(piface pcap.Interface, iface *net.Interface) bool {
	addrs, err := iface.Addrs()
	if err != nil {
		log.Errorf("fetch iface[%v] addrs failed: %s", iface.Name, err)
		return false
	}

	var pIfaceAddrs []string
	var ifaceAddrs []string

	for _, addr := range piface.Addresses {
		pIfaceAddrs = append(pIfaceAddrs, addr.IP.String())
	}

	for _, addr := range addrs {
		ipValue, _, err := net.ParseCIDR(addr.String())
		if err != nil {
			continue
		}
		ifaceAddrs = append(ifaceAddrs, ipValue.String())
	}

	if pIfaceAddrs == nil || ifaceAddrs == nil {
		log.Debugf("no iIfaceAddrs[pcap:%v] or ifaceAddrs[net:%v]", piface.Name, iface.Name)
		return false
	}

	sort.Strings(pIfaceAddrs)
	sort.Strings(ifaceAddrs)
	return CalcSha1(strings.Join(pIfaceAddrs, "|")) == CalcSha1(strings.Join(ifaceAddrs, "|"))
}

func IfaceNameToPcapIfaceName(name string) (string, error) {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return "", Errorf("fetch net.Interface failed: %s", err)
	}

	devs, err := pcap.FindAllDevs()
	if err != nil {
		return "", Errorf("find pcap dev failed: %s", err)
	}

	for _, dev := range devs {
		if PcapInterfaceEqNetInterface(dev, iface) {
			return dev.Name, nil
		}
	}
	return "", NewConvertIfaceNameError(name)
}
