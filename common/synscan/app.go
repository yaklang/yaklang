package synscan

import (
	"context"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/pcapx/arpx"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/common/utils"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/pkg/errors"
	"github.com/yaklang/pcap"
)

type Scanner struct {
	ctx    context.Context
	cancel context.CancelFunc
	iface  *net.Interface
	config *Config

	handlerWriteChan chan []byte
	handlerIsAlive   *utils.AtomicBool
	// handler               *pcap.Handle
	localHandlerWriteChan chan []byte
	localHandlerIsAlive   *utils.AtomicBool
	// localHandler          *pcap.Handle

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

var cacheEthernetLock = new(sync.Mutex)

// 以进行一次连接的代价让操作系统帮我们src mac和dst mac的获取
// 实际上不需要等包发出去，也无所谓这个端口是否开放
// dstPort可选，如果填了相当于多探测了这个端口一次
func (s *Scanner) getDefaultEthernet(target string, dstPort int, gateway string) error {
	cacheEthernetLock.Lock()
	defer cacheEthernetLock.Unlock()

	// 在加锁之后再判断一次
	if s._cache_eth != nil && s.defaultDstHw != nil {
		return nil
	}

	if s.iface != nil && s.iface.HardwareAddr == nil {
		// vpn 模式下，不需要获取网关的 mac 地址
		// vo
		return nil
	}

	if gateway != "" && gateway != "<nil>" && s.iface != nil && s.iface.HardwareAddr != nil {
		// 传入的网关不为空
		srcHw := s.iface.HardwareAddr
		dstHw, err := arpx.ArpWithTimeout(5*time.Second, s.iface.Name, gateway)
		if err != nil {
			log.Warnf("ArpWithTimeout cannot found dstHw: %v, target: %v, iface: %v, gateway: %v", err, target, s.iface.Name, gateway)
		}
		if dstHw != nil && srcHw != nil {
			s._cache_eth = &layers.Ethernet{
				SrcMAC:       srcHw,
				DstMAC:       dstHw,
				EthernetType: layers.EthernetTypeIPv4,
			}
			s.defaultDstHw = dstHw
			log.Infof("use arpx proto to fetch gateway's hw address: %s", dstHw.String())
			return nil
		}
	}

	/*
		if u cannot fetch hw addr

		just try to send packet by user mode...
	*/
	if gateway != "" && gateway != "<nil>" {
		s.tmpTargetForDetectMAC = gateway
	} else {
		s.tmpTargetForDetectMAC = target
	}
	defer func() {
		s.tmpTargetForDetectMAC = ""
	}()
	go func() {
		if dstPort == 0 {
			dstPort = 22
		}
		conn, _ := netx.DialTCPTimeout(10*time.Second, net.JoinHostPort(target, strconv.Itoa(dstPort)))
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
		return utils.Errorf("cannot fetch hw addr for %v[%v]", target, s.iface.Name)
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

func (s *Scanner) handlePacket(packet gopacket.Packet) {
	if arpLayer := packet.Layer(layers.LayerTypeARP); arpLayer != nil {
		switch arpLayer.LayerType() {
		case layers.LayerTypeARP:
			arp, ok := arpLayer.(*layers.ARP)
			if !ok {
				return
			}
			srcIP := net.IP(arp.SourceProtAddress)
			srcHw := net.HardwareAddr(arp.SourceHwAddress)
			s.onARP(srcIP, srcHw)
		}
	}

	if tcpSynLayer := packet.TransportLayer(); tcpSynLayer != nil {
		l, ok := tcpSynLayer.(*layers.TCP)
		if !ok {
			return
		}

		if l.SYN && l.ACK {
			if nl := packet.NetworkLayer(); nl != nil {
				s.onSynAck(net.ParseIP(nl.NetworkFlow().Src().String()), int(l.SrcPort))
			}
			return
		}

		if l.SYN && !l.ACK && s.tmpTargetForDetectMAC != "" {
			nl := packet.NetworkLayer()
			if nl == nil {
				return
			}

			if nl.NetworkFlow().Dst().String() != s.tmpTargetForDetectMAC {
				return
			}
			eth := packet.LinkLayer()
			if eth == nil {
				return
			}
			l, ok := eth.(*layers.Ethernet)
			if !ok {
				return
			}
			// 缓存地址 mac 地址
			select {
			case s.macChan <- [2]net.HardwareAddr{l.SrcMAC, l.DstMAC}:
			default:
			}
		}
	}
}

func (s *Scanner) startHandler(deviceName string, handlerWriteChan chan []byte, handlerIsAlive *utils.AtomicBool) {
	err := pcaputil.Start(
		pcaputil.WithDevice(deviceName),
		pcaputil.WithEnableCache(true),
		pcaputil.WithDisableAssembly(true),
		pcaputil.WithBPFFilter("(arp) or (tcp[tcpflags] & (tcp-syn) != 0)"),
		pcaputil.WithContext(s.ctx),
		pcaputil.WithNetInterfaceCreated(func(handle *pcap.Handle) {
			if handle != nil {
				go s.startWriting(handle, handlerWriteChan)
			}
		}),
		pcaputil.WithEveryPacket(s.handlePacket),
	)
	if err != nil {
		log.Errorf("start handler failed: %v", err)
		handlerIsAlive.UnSet()
	}
}

func (s *Scanner) startWriting(handle *pcap.Handle, packetsChan chan []byte) {
	var counter int
	var total int64
	for {
		if s.delayMs > 0 && s.delayGapCount > 0 {
			if counter > s.delayGapCount {
				counter = 0
				s.sleepRateLimit()
			}
		}
		select {
		case packets, ok := <-packetsChan:
			if !ok {
				continue
			}
			err := handle.WritePacketData(packets)

			total++
			counter++

			if err != nil {
				log.Errorf("write to device failed: %v", err)
			}
		case <-s.ctx.Done():
			return
		}
	}
}

func NewScanner(ctx context.Context, config *Config) (*Scanner, error) {
	// 初始化扫描网卡
	iface, gatewayIp, srcIp := config.Iface, config.GatewayIP, config.SourceIP
	if iface == nil {
		return nil, errors.New("empty iface")
	}
	// 检测本地回环
	isLoopback := srcIp.IsLoopback()

	log.Debugf("start to init network dev: %v", iface.Name)
	// 初始化本地端口，用来扫描本地环回地址
	log.Debug("start to create local network dev")
	var localIfaceName string
	devs, err := pcap.FindAllDevs()
	if err != nil {
		return nil, utils.Errorf("cannot find pcap ifaceDevs: %v", err)
	}
	for _, d := range devs { // 尝试获取本地回环网卡
		utils.Debug(func() {
			log.Debugf("\nDEVICE: %v\nDESC: %v\nFLAGS: %v\n", d.Name, d.Description, net.Flags(d.Flags).String())
		})

		// 先获取地址 loopback
		for _, addr := range d.Addresses {
			if addr.IP.IsLoopback() {
				localIfaceName = d.Name
				log.Debugf("fetch loopback by addr: %v", d.Name)
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

	scannerCtx, cancel := context.WithCancel(ctx)
	scanner := &Scanner{
		ctx:                   scannerCtx,
		cancel:                cancel,
		iface:                 iface,
		config:                config,
		handlerWriteChan:      make(chan []byte, 100000),
		localHandlerWriteChan: make(chan []byte, 100000),
		handlerIsAlive:        utils.NewBool(false),
		localHandlerIsAlive:   utils.NewBool(false),

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

	if !isLoopback {
		scanner.handlerIsAlive.Set()
		go scanner.startHandler(iface.Name, scanner.handlerWriteChan, scanner.handlerIsAlive)
	}

	scanner.localHandlerIsAlive.Set()
	go scanner.startHandler(localIfaceName, scanner.localHandlerWriteChan, scanner.localHandlerIsAlive)

	_ = scanner.getLoopbackLinkLayer()

	return scanner, nil
}
