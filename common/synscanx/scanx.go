package synscanx

import (
	"bytes"
	"context"
	"fmt"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/pkg/errors"
	"github.com/yaklang/pcap"
	"github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/pcapx/arpx"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/common/synscan"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/hostsparser"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Scannerx struct {
	Ctx    context.Context
	Cancel context.CancelFunc
	config *SynxConfig

	sample string
	hosts  *hostsparser.HostsParser
	ports  *utils.PortsFilter

	OpenPortHandlers func(ip net.IP, port int)

	macTable    *sync.Map
	MacHandlers func(ip net.IP, addr net.HardwareAddr)

	Handle *pcap.Handle

	startTime time.Time
}

func NewScannerx(ctx context.Context, config *SynxConfig) (*Scannerx, error) {
	s := &Scannerx{
		Ctx:       ctx,
		config:    config,
		startTime: time.Now(),
		macTable:  new(sync.Map),
	}
	var iface *net.Interface
	var err error
	// 支持 net interface name 和 pcap dev name
	iface, err = net.InterfaceByName(config.netInterface)
	if err != nil {
		iface, err = pcaputil.PcapIfaceNameToNetInterface(config.netInterface)
		if err != nil {
			return nil, errors.Errorf("get iface failed: %s", err)
		}
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return nil, err
	}
	var ifaceIp net.IP
	for _, addr := range addrs {
		ip := addr.(*net.IPNet).IP
		if utils.IsIPv6(ip.String()) {
			ifaceIp = ip
		}
		if utils.IsIPv4(ip.String()) {
			ifaceIp = ip
			break
		}
	}
	if ifaceIp == nil {
		return nil, errors.Errorf("iface: %s has no addrs", iface.Name)
	}

	s.config.Iface = iface
	s.config.SourceIP = ifaceIp

	err = s.initHandle()
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Scannerx) filterHost(host string) bool {
	if s.config.excludeHosts != nil {
		if s.config.excludeHosts.Contains(host) {
			return true
		}
	}

	return false
}

func (s *Scannerx) filterPort(port int) bool {
	if s.config.ExcludePorts != nil {
		if s.config.ExcludePorts.Exist(fmt.Sprint(port)) {
			return true
		}
	}

	return false
}

func (s *Scannerx) GetFilterPorts(ports string) []int {
	var filteredPorts []int
	var fp string
	for _, port := range utils.ParseStringToPorts(ports) {
		if s.filterPort(port) {
			continue
		}
		fp += strconv.Itoa(port) + ","
		filteredPorts = append(filteredPorts, port)
	}
	s.ports = utils.NewPortsFilter(fp)
	return filteredPorts
}

func (s *Scannerx) GetFilterHosts(targets string) []string {
	var filteredHosts []string

	for _, host := range utils.ParseStringToHosts(targets) {
		if s.filterHost(host) {
			continue
		}
		filteredHosts = append(filteredHosts, host)
	}
	s.hosts = hostsparser.NewHostsParser(s.Ctx, strings.Join(filteredHosts, ","))
	return filteredHosts
}

func (s *Scannerx) SubmitTask(targets, ports string, targetCh chan *SynxTarget) {
	filteredHosts := s.GetFilterHosts(targets)
	filtedPorts := s.GetFilterPorts(ports)
	var counter int
	for _, host := range filteredHosts {
		if s.sample == "" {
			s.sample = host
			if err := s.getGatewayMac(); err != nil {
				log.Errorf("getGatewayMac failed: %v", err)
				break
			}
		}
		for _, port := range filtedPorts {
			if s.config.rateLimitDelayMs > 0 && s.config.rateLimitDelayGap > 0 {
				if counter > s.config.rateLimitDelayGap {
					counter = 0
					s.sleepRateLimit()
				}
			}
			counter++
			proto, p := utils.ParsePortToProtoPort(port)
			target := &SynxTarget{
				Host: host,
				Port: p,
				Mode: TCP, // 默认 TCP
			}
			if proto == "udp" {
				target.Mode = UDP
			}
			select {
			case <-s.Ctx.Done():
				log.Infof("SubmitTask canceled")
				return
			case targetCh <- target:
			}
		}
	}
}

func (s *Scannerx) getGatewayMac() error {
	gateway := s.config.GatewayIP.String()
	if gateway != "" && gateway != "<nil>" && s.config.Iface != nil && s.config.Iface.HardwareAddr != nil {
		srcHw := s.config.Iface.HardwareAddr
		dstHw, err := arpx.ArpWithTimeout(5*time.Second, s.config.Iface.Name, gateway)
		if err != nil {
			log.Warnf("ArpWithTimeout cannot found dstHw: %v, target: %v, iface: %v, gateway: %v", err, s.sample, s.config.Iface.Name, gateway)
		}
		if dstHw != nil && srcHw != nil {
			s.config.RemoteMac = dstHw
			log.Infof("use arpx proto to fetch gateway's hw address: %s", dstHw.String())
			return nil
		}
	}
	macCh := make(chan net.HardwareAddr)

	wg := sync.WaitGroup{}

	ctx, cancel := context.WithTimeout(context.Background(), s.config.FetchGatewayHardwareAddressTimeout)

	wg.Add(1)
	go func() {
		defer wg.Done()
		err := pcaputil.Start(
			pcaputil.WithContext(ctx),
			pcaputil.WithDevice(s.config.Iface.Name),
			pcaputil.WithDisableAssembly(true),
			pcaputil.WithBPFFilter("udp dst port 65321"),
			pcaputil.WithEveryPacket(func(packet gopacket.Packet) {
				if ethLayer := packet.Layer(layers.LayerTypeEthernet); ethLayer != nil {
					if !bytes.Equal(ethLayer.(*layers.Ethernet).DstMAC, []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}) {
						log.Infof("MAC Address found: %s", ethLayer.(*layers.Ethernet).DstMAC)
						macCh <- ethLayer.(*layers.Ethernet).DstMAC
						cancel()
					}
				}
			}),
		)
		if err != nil {
			log.Errorf("pcaputil.Start failed: %v", err)
			return
		}

	}()

	connectUdp := func() error {
		conn, err := yaklib.ConnectUdp(s.sample, "65321")
		if err != nil {
			log.Errorf("connect udp failed: %v", err)
			return err
		}
		defer conn.Close()
		_, err = conn.Write([]byte("hello"))
		if err != nil {
			return err
		}

		err = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		if err != nil {
			return err
		}
		buf := make([]byte, 1024)
		_, _ = conn.Read(buf)
		return nil
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 3; i++ {
			err := connectUdp()
			if err != nil {
				return
			}
		}
	}()
	go func() {
		wg.Wait()
		close(macCh)
	}()

	timer := time.NewTimer(time.Second * 3)
	defer timer.Stop()

	select {
	case <-timer.C:
		return utils.Errorf("cannot fetch hw addr for %v[%v]", s.sample, s.config.Iface.Name)
	case hw := <-macCh:
		s.config.RemoteMac = hw
		log.Infof("use pcap proto to fetch gateway's hw address: %s", hw.String())
		return nil
	}
}

func (s *Scannerx) Scan(done chan struct{}, targetCh chan *SynxTarget, resultCh chan *synscan.SynScanResult) error {
	openPortLock := new(sync.Mutex)
	var openPortCount int

	var outputFile *os.File
	if s.config.outputFile != "" {
		var err error
		outputFile, err = os.OpenFile(s.config.outputFile, os.O_RDWR|os.O_CREATE, os.ModePerm)
		if err != nil {
			log.Errorf("open file %v failed; %s", s.config.outputFile, err)
		}
		if outputFile != nil {
			defer outputFile.Close()
		}
	}

	resultFilter := filter.NewFilter()
	defer resultFilter.Close()
	if s.OpenPortHandlers == nil {
		s.OpenPortHandlers = func(host net.IP, port int) {
			openPortLock.Lock()
			defer openPortLock.Unlock()

			addr := utils.HostPort(host.String(), port)
			if resultFilter.Exist(addr) {
				return
			}

			resultFilter.Insert(addr)

			if !(s.hosts.Contains(host.String()) && s.ports.Contains(port)) {
				return
			}
			openPortCount++
			result := &synscan.SynScanResult{
				Host: host.String(),
				Port: port,
			}
			s.config.callCallback(result)

			resultCh <- result

			if outputFile != nil {
				outputFile.Write(
					[]byte(fmt.Sprintf(
						"%s%v\n",
						s.config.outputFilePrefix,
						addr,
					)),
				)
			}
		}
	}

	wCtx, wCancel := context.WithCancel(context.Background())
	go s.HandlerZeroCopyReadPacket(wCtx, resultCh)
	time.Sleep(100 * time.Millisecond)

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.arpScan()
		s.sendPacket(s.Ctx, targetCh)
	}()
	wg.Wait()

	time.Sleep(s.config.waiting)

	wCancel()

	s.Handle.Close()

	done <- struct{}{}

	log.Infof("open port count: %d", openPortCount)
	return nil
}

func (s *Scannerx) sleepRateLimit() {
	if s.config.rateLimitDelayMs <= 0 {
		return
	}
	time.Sleep(time.Duration(s.config.rateLimitDelayMs*100) * time.Millisecond)
}

func (s *Scannerx) sendPacket(ctx context.Context, targetCh chan *SynxTarget) {
	log.Info("start send packet")
	for {
		select {
		case <-ctx.Done():
			log.Info("Context cancelled, stopping packet sending")
			return
		case target, ok := <-targetCh:
			if !ok {
				log.Debugf("target channel closed, stopping packet sending")
				return
			}
			host := target.Host
			port := target.Port
			proto := target.Mode
			packet, err := s.assemblePacket(host, port, proto)
			if err != nil {
				log.Errorf("assemble packet failed: %v", err)
				continue
			}
			err = s.Handle.WritePacketData(packet)
			if err != nil {
				log.Errorf("write to device failed: %v", err)
			}
		}
	}
}

func (s *Scannerx) assemblePacket(host string, port int, proto ProtocolType) ([]byte, error) {
	switch proto {
	case TCP:
		return s.assembleSynPacket(host, port)
	case UDP:
	case ICMP:
	case ARP:
		return s.assembleArpPacket(host)
	}
	return nil, nil
}

func (s *Scannerx) HandlerReadPacket(ctx context.Context, resultCh chan *synscan.SynScanResult) {
	packetSource := gopacket.NewPacketSource(s.Handle, s.Handle.LinkType())
	packetSource.Lazy = true
	packetSource.NoCopy = true
	packetSource.DecodeStreamsAsDatagrams = true

	for {
		select {
		case <-ctx.Done():
			return
		case packet := <-packetSource.Packets():
			if packet == nil {
				continue
			}
			s.handlePacket(packet, resultCh)
		}
	}
}

func (s *Scannerx) HandlerZeroCopyReadPacket(ctx context.Context, resultCh chan *synscan.SynScanResult) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			data, _, err := s.Handle.ZeroCopyReadPacketData()
			if errors.Is(err, pcap.NextErrorTimeoutExpired) || errors.Is(err, pcap.NextErrorReadError) || errors.Is(err, io.EOF) {
				continue
			} else if err != nil {
				log.Printf("Error reading packet: %v", err)
				continue
			}

			packet := gopacket.NewPacket(data, s.Handle.LinkType(), gopacket.Default)
			s.handlePacket(packet, resultCh)
		}
	}
}

func (s *Scannerx) handlePacket(packet gopacket.Packet, resultCh chan *synscan.SynScanResult) {
	if arpLayer := packet.Layer(layers.LayerTypeARP); arpLayer != nil {
		arp := arpLayer.(*layers.ARP)
		if arp.Operation == 2 {
			srcIP := net.IP(arp.SourceProtAddress)
			srcHw := net.HardwareAddr(arp.SourceHwAddress)
			s.onArp(srcIP, srcHw)
		}
	}

	if tcpSynLayer := packet.TransportLayer(); tcpSynLayer != nil {
		l, ok := tcpSynLayer.(*layers.TCP)
		if !ok {
			return
		}

		if l.SYN && l.ACK {
			if nl := packet.NetworkLayer(); nl != nil {
				s.OpenPortHandlers(net.ParseIP(nl.NetworkFlow().Src().String()), int(l.SrcPort))
			}
			return
		}
	}

}

func (s *Scannerx) Close() {
	s.Handle.Close()
}

func (s *Scannerx) initHandle() error {
	if s.config.Iface == nil {
		return utils.Errorf("iface is nil")
	}
	pcapIface, err := pcaputil.IfaceNameToPcapIfaceName(s.config.Iface.Name)
	if err != nil {
		return utils.Errorf("iface name to pcap iface name failed: %v", err)
	}
	handle, err := pcap.OpenLive(pcapIface, 65536, true, pcap.BlockForever)

	if err != nil {
		info, err := codec.GB18030ToUtf8([]byte(err.Error()))
		if err != nil {
			return utils.Errorf("cannot find pcap ifaceDevs: %v", err)
		}
		return utils.Errorf("cannot find pcap ifaceDevs: %v", string(info))
	}

	err = handle.SetBPFFilter(fmt.Sprintf("ether dst %s && (arp || tcp[tcpflags] == tcp-syn|tcp-ack)", s.config.Iface.HardwareAddr.String()))
	if err != nil {
		return utils.Errorf("SetBPFFilter failed: %v", err)
	}
	s.Handle = handle
	return nil
}

func (s *Scannerx) onArp(ip net.IP, hw net.HardwareAddr) {
	log.Infof("ARP: %s -> %s", ip.String(), hw.String())
	if s.MacHandlers != nil {
		s.MacHandlers(ip, hw)
	}

	s.macTable.Store(ip.String(), hw)
}

func (s *Scannerx) arpScan() {
	addrs, _ := s.config.Iface.Addrs()

	var ifaceIPNet *net.IPNet

	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok || ipNet == nil {
			continue
		}
		ifaceIPNet = ipNet
		break
	}
	for target := range s.hosts.Hosts() {
		s.sleepRateLimit()
		select {
		case <-s.Ctx.Done():
			return
		default:
			if !ifaceIPNet.Contains(net.ParseIP(target)) {
				continue
			}
			packet, err := s.assemblePacket(target, 0, ARP)
			if err != nil {
				log.Errorf("assemble packet failed: %v", err)
				return
			}
			err = s.Handle.WritePacketData(packet)
			if err != nil {
				log.Errorf("write to device failed: %v", err)
				return
			}
		}
	}
}
