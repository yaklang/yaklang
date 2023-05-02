package synscan

import (
	"context"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
	"yaklang/common/log"
	"yaklang/common/utils"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"github.com/pkg/errors"
)

type Scanner struct {
	ctx    context.Context
	cancel context.CancelFunc
	iface  *net.Interface
	config *Config

	handlerWriteChan      chan []byte
	handler               *pcap.Handle
	localHandlerWriteChan chan []byte
	localHandler          *pcap.Handle

	opts gopacket.SerializeOptions

	// default dst hardware
	defaultDstHw     net.HardwareAddr
	defaultSrcIp     net.IP
	defaultGatewayIp net.IP

	_cache_eth          gopacket.SerializableLayer
	_loopback_linklayer gopacket.SerializableLayer

	arpHandlerMutex *sync.Mutex
	arpHandlers     map[string]arpHandler

	synAckHandlerMutex *sync.Mutex
	synAckHandlers     map[string]synAckHandler

	macChan               chan [2]net.HardwareAddr
	tmpTargetForDetectMAC string

	delayMs       float64
	delayGapCount int

	// onSubmitTaskCallback: 每提交一个数据包的时候，这个 callback 调用一次
	onSubmitTaskCallback func(string, int)
}

func (s *Scanner) SetRateLimit(ms float64, count int) {
	// ms 为
	s.delayMs = ms
	s.delayGapCount = count
}

func (s *Scanner) getLoopbackLinkLayer() gopacket.SerializableLayer {
	if s._loopback_linklayer != nil {
		return s._loopback_linklayer
	}
	s._loopback_linklayer = &layers.Loopback{
		Family: layers.ProtocolFamilyIPv4,
	}
	return s.getLoopbackLinkLayer()
}

var (
	cacheEthernetLock = new(sync.Mutex)
)

// 以进行一次连接的代价让操作系统帮我们src mac和det mac的获取
// 实际上不需要等包发出去，也无所谓这个端口是否开放
// dstPort可选，如果填了相当于多探测了这个端口一次
func (s *Scanner) getDefaultEthernet(target string, dstPort int, gateway string) error {
	cacheEthernetLock.Lock()
	defer cacheEthernetLock.Unlock()

	// 在加锁之后再判断一次
	if s._cache_eth != nil && s.defaultDstHw != nil {
		return nil
	}

	if gateway != "" && s.iface != nil {
		// 传入的网关不为空
		srcHw := s.iface.HardwareAddr
		dstHw, err := utils.ArpWithTimeout(5*time.Second, s.iface.Name, gateway)
		if err != nil {
			log.Warnf("utils.ArpWithTimeout cannot found dstHw: %v, target: %v, iface: %v, gateway: %v", err, target, s.iface.Name, gateway)
		}
		if dstHw != nil && srcHw != nil {
			s._cache_eth = &layers.Ethernet{
				SrcMAC:       srcHw,
				DstMAC:       dstHw,
				EthernetType: layers.EthernetTypeIPv4,
			}
			s.defaultDstHw = dstHw
			log.Infof("use arp proto to fetch gateway's hw address: %s", dstHw.String())
			return nil
		}
	}

	/*
		if u cannot fetch hw addr

		just try to send packet by user mode...
	*/
	if gateway != "" {
		s.tmpTargetForDetectMAC = gateway
	} else {
		s.tmpTargetForDetectMAC = target
	}
	defer func() {
		s.tmpTargetForDetectMAC = ""
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() {
		var d net.Dialer
		if dstPort == 0 {
			dstPort = 22
		}
		conn, _ := d.DialContext(ctx, "tcp", net.JoinHostPort(target, strconv.Itoa(dstPort)))
		defer func() {
			if conn != nil {
				_ = conn.Close()
			}
		}()
	}()
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case <-timer.C:
		return errors.New("get default eth timeout")
	case hw := <-s.macChan:
		s._cache_eth = &layers.Ethernet{
			SrcMAC:       hw[0],
			DstMAC:       hw[1],
			EthernetType: layers.EthernetTypeIPv4,
		}
		s.defaultDstHw = hw[1]
		return nil
	}
}

func (s *Scanner) getDefaultCacheEthernet(target string, dstPort int, gateway string) (gopacket.SerializableLayer, error) {
	var err error

	if s._cache_eth != nil && s.defaultDstHw != nil {
		return s._cache_eth, nil
	}
	count := 0
	for {
		if err = s.getDefaultEthernet(target, dstPort, gateway); err == nil {
			return s._cache_eth, nil
		}
		count += 1
		if count > 5 {
			return nil, err
		}
	}
}

func NewScanner(ctx context.Context, config *Config) (*Scanner, error) {
	// 初始化扫描网卡
	iface, gatewayIp, srcIp := config.Iface, config.GatewayIP, config.SourceIP
	if iface == nil {
		return nil, errors.New("empty iface")
	}
	_ = gatewayIp
	isLoopback := srcIp.IsLoopback()

	log.Debugf("start to init network dev: %v", iface.Name)
	ifaceName, err := utils.IfaceNameToPcapIfaceName(iface.Name)
	if err != nil {
		if _, ok := err.(*utils.ConvertIfaceNameError); !isLoopback || !ok {
			return nil, err
		}
	}

	// 初始化本地端口，用来扫描本地环回地址
	log.Debug("start to create local network dev")
	var localIfaceName string
	devs, err := pcap.FindAllDevs()
	if err != nil {
		return nil, utils.Errorf("cannot find pcap ifaceDevs: %v", err)
	}
	for _, d := range devs {
		utils.Debug(func() {
			log.Debugf(`
DEVICE: %v
DESC: %v
FLAGS: %v
`, d.Name, d.Description, net.Flags(d.Flags).String())
		})

		// 先获取地址 loopback
		for _, addr := range d.Addresses {
			if addr.IP.IsLoopback() {
				localIfaceName = d.Name
				log.Infof("fetch loopback by addr: %v", d.Name)
				break
			}
		}
		if localIfaceName != "" {
			break
		}

		// 默认 desc 获取 loopback
		if strings.Contains(strings.ToLower(d.Description), "adapter for loopback traffic capture") {
			log.Infof("found loopback by desc: %v", d.Name)
			localIfaceName = d.Name
			break
		}

		// 获取 flags
		if net.Flags(uint(d.Flags))&net.FlagLoopback == 1 {
			log.Infof("found loopback by flag: %v", d.Name)
			localIfaceName = d.Name
			break
		}
	}
	if localIfaceName == "" {
		return nil, utils.Errorf("no loopback iface found")
	}
	if isLoopback {
		ifaceName = localIfaceName
	}
	handler, err := pcap.OpenLive(ifaceName, 65535, false, pcap.BlockForever)
	if err != nil {
		return nil, errors.Errorf("open device[%v-%v] failed: %s", iface.Name, strconv.QuoteToASCII(iface.Name), err)
	}

	log.Infof("fetch local loopback pcapDev:[%v]", localIfaceName)
	localHandler, err := pcap.OpenLive(localIfaceName, 65535, false, pcap.BlockForever)
	if err != nil {
		return nil, utils.Errorf("open local iface failed: %s", err)
	}

	scannerCtx, cancel := context.WithCancel(ctx)
	scanner := &Scanner{
		ctx:                   scannerCtx,
		cancel:                cancel,
		iface:                 iface,
		config:                config,
		handlerWriteChan:      make(chan []byte, 100000),
		localHandlerWriteChan: make(chan []byte, 100000),
		handler:               handler,
		localHandler:          localHandler,

		defaultSrcIp:     srcIp,
		defaultGatewayIp: gatewayIp,

		opts: gopacket.SerializeOptions{
			FixLengths:       true,
			ComputeChecksums: true,
		},

		// ARP Handler 用来监控硬件设备信息
		arpHandlerMutex: new(sync.Mutex),
		arpHandlers:     make(map[string]arpHandler),

		// SynAckHandler 用来处理端口开放
		synAckHandlerMutex: new(sync.Mutex),
		synAckHandlers:     make(map[string]synAckHandler),
		macChan:            make(chan [2]net.HardwareAddr, 100),
	}

	scanner.daemon()

	//scanner.defaultDstHw, err = netutil.RouteAndArp(gatewayIp.String())
	//if err == utils.TargetIsLoopback {
	//	scanner.defaultDstHw = nil
	//}

	_ = scanner.getLoopbackLinkLayer()

	return scanner, nil
}

func (s *Scanner) daemon() {
	// handler
	err := s.handler.SetBPFFilter("(arp) or (tcp[tcpflags] & (tcp-syn) != 0)")
	if err != nil {
		log.Errorf("set bpf filter failed: %s", err)
	}
	source := gopacket.NewPacketSource(s.handler, s.handler.LinkType())
	packets := source.Packets()

	// local handler
	err = s.localHandler.SetBPFFilter("(arp) or (tcp[tcpflags] & (tcp-syn) != 0)")
	if err != nil {
		log.Errorf("set bpf filter failed for loopback: %s", err)
	}
	localSource := gopacket.NewPacketSource(s.localHandler, s.localHandler.LinkType())
	localPackets := localSource.Packets()

	handlePackets := func(packetStream chan gopacket.Packet) {
		for {
			select {
			case packet, ok := <-packetStream:
				if !ok {
					return
				}

				if arpLayer := packet.Layer(layers.LayerTypeARP); arpLayer != nil {
					switch arpLayer.LayerType() {
					case layers.LayerTypeARP:
						arp, ok := arpLayer.(*layers.ARP)
						if !ok {
							continue
						}
						srcIP := net.IP(arp.SourceProtAddress)
						srcHw := net.HardwareAddr(arp.SourceHwAddress)
						s.onARP(srcIP, srcHw)
					}
				}

				if tcpSynLayer := packet.TransportLayer(); tcpSynLayer != nil {
					l, ok := tcpSynLayer.(*layers.TCP)
					if !ok {
						continue
					}

					if l.SYN && l.ACK {
						if nl := packet.NetworkLayer(); nl != nil {
							s.onSynAck(net.ParseIP(nl.NetworkFlow().Src().String()), int(l.SrcPort))
						}
						continue
					}

					if l.SYN && !l.ACK && s.tmpTargetForDetectMAC != "" {
						nl := packet.NetworkLayer()
						if nl == nil {
							continue
						}

						if nl.NetworkFlow().Dst().String() != s.tmpTargetForDetectMAC {
							continue
						}
						eth := packet.LinkLayer()
						if eth == nil {
							continue
						}
						l, ok := eth.(*layers.Ethernet)
						if !ok {
							continue
						}
						// 缓存地址 mac 地址
						select {
						case s.macChan <- [2]net.HardwareAddr{l.SrcMAC, l.DstMAC}:
						default:
						}
					}
				}
			case <-s.ctx.Done():
				return
			}
		}
	}

	go func() {
		s.sendService()
	}()

	go func() {
		handlePackets(packets)
	}()

	go func() {
		handlePackets(localPackets)
	}()
}

func (s *Scanner) Close() {
	s.handler.Close()
	s.localHandler.Close()
}
