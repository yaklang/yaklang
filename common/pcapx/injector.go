package pcapx

import (
	"fmt"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakdns"
	"math/rand"
	"net"
	"sync"
	"time"
)

var (
	deviceToNet sync.Map
)

func getInjectorHandler(name string) (*pcap.Handle, error) {
	raw, ok := deviceToNet.Load(name)
	if !ok {
		var err error
		name, err = utils.IfaceNameToPcapIfaceName(name)
		if err != nil {
			return nil, utils.Errorf("fix iface name failed: %v", err)
		}
		handle, err := pcap.OpenLive(name, 65536, true, pcap.BlockForever)
		if err != nil {
			return nil, err
		}
		deviceToNet.Store(name, handle)
		return handle, nil
	}
	return raw.(*pcap.Handle), nil
}

func injectRaw(iface string, raw []byte) error {
	handler, err := getInjectorHandler(iface)
	if err != nil {
		return err
	}
	err = handler.WritePacketData(raw)
	if err != nil {
		return err
	}
	return nil
}

var defaultGopacketSerializeOpt = gopacket.SerializeOptions{
	FixLengths:       true,
	ComputeChecksums: true,
}

func createIPTCPHTTPRequest(isHttps bool, raw []byte) ([]byte, error) {
	raw = lowhttp.FixHTTPRequestOut(raw)
	_, _, localip, err := GetPublicRoute()
	if err != nil {
		return nil, utils.Errorf("get default route iface ip failed: %s", err)
	}

	u, err := lowhttp.ExtractURLFromHTTPRequestRaw(raw, isHttps)
	if err != nil {
		return nil, utils.Errorf("extract dst url failed: %s", err)
	}

	dst, port, err := utils.ParseStringToHostPort(u.String())
	if err != nil {
		return nil, err
	}

	src := localip.String() + fmt.Sprintf(":%v", 40000+rand.Intn(20000))
	ip, tcp, err := createIPTCPLayers(src, utils.HostPort(dst, port))
	if err != nil {
		return nil, utils.Errorf("create network layers failed: %s", err)
	}
	err = tcp.SetNetworkLayerForChecksum(ip)
	if err != nil {
		return nil, utils.Errorf("calc network layer for checksum failed: %s", err)
	}

	buf := gopacket.NewSerializeBuffer()
	err = gopacket.SerializeLayers(buf, defaultGopacketSerializeOpt, ip, tcp, gopacket.Payload(raw))
	if err != nil {
		return nil, utils.Errorf("serialize failed: %s", err)
	}
	return buf.Bytes(), nil
}

func ParseSrcNDstAddress(src, dst string) (net.IP, net.IP, uint16, uint16, error) {
	srcHost, srcPort, err := utils.ParseStringToHostPort(src)
	if err != nil {
		return nil, nil, 0, 0, utils.Errorf("parse src addr[%v] failed: %s", src, err)
	}

	dstHost, dstPort, err := utils.ParseStringToHostPort(dst)
	if err != nil {
		return nil, nil, 0, 0, utils.Errorf("parse dst addr[%v] failed: %s", dst, err)
	}

	if srcPort <= 0 || dstPort <= 0 {
		return nil, nil, 0, 0, utils.Errorf("addr cannot found port: %v -> %v", src, dst)
	}

	var (
		srcIP, dstIP net.IP
	)
	wg := new(sync.WaitGroup)
	wg.Add(2)
	go func() {
		defer func() {
			wg.Done()
		}()
		if !utils.IsIPv4(srcHost) && utils.IsValidDomain(srcHost) {
			targetIP := yakdns.LookupFirst(srcHost, yakdns.WithTimeout(3*time.Second))

			if targetIP != "" {
				srcIP = net.ParseIP(targetIP)
			}
		} else {
			srcIP = net.ParseIP(srcHost)
		}

		if srcIP == nil {
			srcIP = net.ParseIP(utils.GetRandomIPAddress())
		}
	}()
	go func() {
		defer func() {
			wg.Done()
		}()
		if !utils.IsIPv4(dstHost) && utils.IsValidDomain(dstHost) {
			targetIP := yakdns.LookupFirst(dstHost, yakdns.WithTimeout(5*time.Second))
			if targetIP != "" {
				dstIP = net.ParseIP(targetIP)
			}
		} else {
			dstIP = net.ParseIP(dstHost)
		}

		if dstIP == nil {
			dstIP = net.ParseIP(utils.GetRandomIPAddress())
		}
	}()
	wg.Wait()

	if dstIP == nil || srcIP == nil {
		return nil, nil, 0, 0, utils.Errorf("dstIP and srcIP failed: %s => %v", src, dst)
	}
	return srcIP, dstIP, uint16(srcPort), uint16(dstPort), nil
}

func createIPTCPLayers(src, dst string) (*layers.IPv4, *layers.TCP, error) {
	srcIP, dstIP, srcPort, dstPort, err := ParseSrcNDstAddress(src, dst)
	if err != nil {
		return nil, nil, err
	}

	ipLayer := &layers.IPv4{
		BaseLayer: layers.BaseLayer{},
		Version:   4,
		TTL:       64,
		Protocol:  layers.IPProtocolTCP,
		SrcIP:     srcIP,
		DstIP:     dstIP,
	}
	seq := 11050 + rand.Intn(10000)
	tcpLayer := &layers.TCP{
		SrcPort:    layers.TCPPort(srcPort),
		DstPort:    layers.TCPPort(dstPort),
		Seq:        uint32(seq),
		Ack:        uint32(seq - 123),
		DataOffset: 0,
		FIN:        false,
		SYN:        false,
		RST:        false,
		PSH:        true,
		ACK:        true,
		URG:        false,
		ECE:        false,
		CWR:        false,
		NS:         false,
		Window:     4096,
		Checksum:   0,
		Urgent:     0,
		Options:    nil,
		Padding:    nil,
	}

	err = tcpLayer.SetNetworkLayerForChecksum(ipLayer)
	if err != nil {
		return nil, nil, utils.Errorf("compute checksum failed: %s", err)
	}
	return ipLayer, tcpLayer, nil
}
