package finscan

import (
	"context"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/pcapx/arpx"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/netutil"
	"net"
	"strings"
	"sync"
	"time"
)

type Scanner struct {
	ctx    context.Context
	cancel context.CancelFunc
	iface  *net.Interface
	config *Config

	handlerWriteChan chan []byte
	handlerIsAlive   *utils.AtomicBool
	//handler               *pcap.Handle
	localHandlerWriteChan chan []byte
	localHandlerIsAlive   *utils.AtomicBool
	//localHandler          *pcap.Handle

	opts gopacket.SerializeOptions

	// default dst hardware
	defaultDstHw     net.HardwareAddr
	defaultSrcIp     net.IP
	defaultGatewayIp net.IP

	_cache_eth          gopacket.SerializableLayer
	_loopback_linklayer gopacket.SerializableLayer

	rstAckHandlerMutex *sync.Mutex
	rstAckHandlers     map[string]rstAckHandler

	noRspHandlerMutex *sync.Mutex
	noRspHandlers     map[string]noRspHandler

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

func (s *Scanner) IsMacCached() bool {
	return s._cache_eth != nil && s.defaultDstHw != nil
}

var (
	cacheEthernetLock = new(sync.Mutex)
)

// 以进行一次连接的代价让操作系统帮我们src mac和det mac的获取
// 实际上不需要等包发出去，也无所谓这个端口是否开放
// dstPort可选，如果填了相当于多探测了这个端口一次
func (s *Scanner) getDefaultEthernet(target string) error {
	cacheEthernetLock.Lock()
	defer cacheEthernetLock.Unlock()

	// 在加锁之后再判断一次
	if s._cache_eth != nil && s.defaultDstHw != nil {
		return nil
	}
	s.tmpTargetForDetectMAC = target
	defer func() {
		s.tmpTargetForDetectMAC = ""
	}()
	//ctx, cancel := context.WithCancel(context.Background())
	//defer cancel()
	srcIFace, _, _, timeout := netutil.Route(time.Second*3, target)
	if timeout != nil {
		return timeout
	}
	srcMAC := srcIFace.HardwareAddr
	dstMAC, timeout := arpx.RouteAndArpWithTimeout(time.Second*3, target)
	if timeout != nil {
		return timeout
	}
	s._cache_eth = &layers.Ethernet{
		SrcMAC:       srcMAC,
		DstMAC:       dstMAC,
		EthernetType: layers.EthernetTypeIPv4,
	}
	s.defaultDstHw = dstMAC
	return nil

}

func (s *Scanner) getDefaultCacheEthernet(target string, dstPort int) (gopacket.SerializableLayer, error) {
	var err error

	if s._cache_eth != nil && s.defaultDstHw != nil {
		return s._cache_eth, nil
	}
	count := 0
	for {
		if err = s.getDefaultEthernet(target); err == nil {
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

	// 初始化本地端口，用来扫描本地环回地址
	log.Debug("start to create local network dev")
	var localIfaceName string
	devs, err := pcap.FindAllDevs()
	if err != nil {
		return nil, utils.Errorf("cannot find pcap ifaceDevs: %v", err)
	}
	for _, d := range devs {
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

	scannerCtx, cancel := context.WithCancel(ctx)
	scanner := &Scanner{
		ctx:                   scannerCtx,
		cancel:                cancel,
		iface:                 iface,
		config:                config,
		handlerWriteChan:      make(chan []byte, 100000),
		localHandlerWriteChan: make(chan []byte, 100000),
		handlerIsAlive:        utils.NewBool(true),
		localHandlerIsAlive:   utils.NewBool(true),
		//handler:               handler,
		//localHandler:          localHandler,

		// 默认扫描公网网卡的 网关IP
		defaultGatewayIp: gatewayIp,
		// 默认扫描的公网网卡的 IP
		defaultSrcIp: srcIp,

		opts: gopacket.SerializeOptions{
			FixLengths:       true,
			ComputeChecksums: true,
		},

		// SynAckHandler 用来处理端口开放
		rstAckHandlerMutex: new(sync.Mutex),
		rstAckHandlers:     make(map[string]rstAckHandler),
		noRspHandlerMutex:  new(sync.Mutex),
		noRspHandlers:      make(map[string]noRspHandler),
		macChan:            make(chan [2]net.HardwareAddr, 1),
	}

	packetHandle := func(packet gopacket.Packet) {
		if tcpLayer := packet.TransportLayer(); tcpLayer != nil {
			l, ok := tcpLayer.(*layers.TCP)
			if !ok {
				return
			}

			//端口扫描的响应包
			if scanner.config.TcpFilter(l) {
				if nl := packet.NetworkLayer(); nl != nil {
					scanner.onRstAck(net.ParseIP(nl.NetworkFlow().Src().String()), int(l.SrcPort))
				} else {
					scanner.onNoRsp(net.ParseIP(nl.NetworkFlow().Src().String()), int(l.SrcPort))
				}
				return
			}

		}
	}

	if !isLoopback { // 只有本地回环 则无需启动此handler
		// handle
		go func() {
			err := pcaputil.Start(
				pcaputil.WithDevice(iface.Name),
				pcaputil.WithEnableCache(true),
				pcaputil.WithBPFFilter("(tcp[tcpflags] & (tcp-rst) != 0)"),
				pcaputil.WithContext(ctx),
				pcaputil.WithNetInterfaceCreated(func(handle *pcap.Handle) {
					go func() {
						var counter int
						var total int64
						for {
							if scanner.delayMs > 0 && scanner.delayGapCount >= 0 {
								if counter > scanner.delayGapCount {
									counter = 0
									//fmt.Printf("rate limit trigger! for %vms\n", s.delayMs)
									scanner.sleepRateLimit()
								}
							}
							select {
							case packets, ok := <-scanner.handlerWriteChan:
								if !ok {
									continue
								}

								failedCount := 0
							RETRY_WRITE_IF:
								// 5-15 us (每秒可以开到 1000 * 200 个包最快)
								err := handle.WritePacketData(packets)

								total++
								counter++

								if err != nil {
									switch true {
									case utils.IContains(err.Error(), "no buffer space available"):
										if failedCount > 10 {
											log.Errorf("write device failed: for %v", err.Error())
											break
										}
										if scanner.delayMs > 0 {
											scanner.sleepRateLimit()
										} else {
											time.Sleep(time.Millisecond * 10)
										}
										failedCount++
										goto RETRY_WRITE_IF
									default:
										log.Errorf("iface: %v handler write failed: %s: retry", scanner.iface, err)
									}
								}
							case <-scanner.ctx.Done():
								return
							}
						}
					}()
				}),
				pcaputil.WithEveryPacket(packetHandle),
			)
			if err != nil {
				scanner.handlerIsAlive.UnSet()
			}
		}()
	} else {
		scanner.handlerIsAlive.UnSet()
	}

	//local handle
	go func() {
		err := pcaputil.Start(
			pcaputil.WithDevice(localIfaceName),
			pcaputil.WithEnableCache(true),
			pcaputil.WithBPFFilter("(tcp[tcpflags] & (tcp-rst) != 0)"),
			pcaputil.WithContext(ctx),
			pcaputil.WithNetInterfaceCreated(func(handle *pcap.Handle) {
				go func() {
					var counter int
					var total int64
					for {
						if scanner.delayMs > 0 && scanner.delayGapCount >= 0 {
							if counter > scanner.delayGapCount {
								counter = 0
								//fmt.Printf("rate limit trigger! for %vms\n", s.delayMs)
								scanner.sleepRateLimit()
							}
						}
						select {
						case localPackets, ok := <-scanner.localHandlerWriteChan:
							if !ok {
								continue
							}

							err := handle.WritePacketData(localPackets)

							total++
							counter++

							if err != nil {
								log.Errorf("loopback handler write failed: %s", err)
							}
						case <-scanner.ctx.Done():
							return
						}
					}
				}()
			}),
			pcaputil.WithEveryPacket(packetHandle),
		)
		if err != nil {
			scanner.localHandlerIsAlive.UnSet()
		}
	}()

	//scanner.daemon()

	//scanner.defaultDstHw, err = netutil.RouteAndArp(gatewayIp.String())
	//if err == utils.TargetIsLoopback {
	//	scanner.defaultDstHw = nil
	//}

	_ = scanner.getLoopbackLinkLayer()

	return scanner, nil
}

//func (s *Scanner) daemon() {
//	// handler
//	err := s.handler.SetBPFFilter("(tcp[tcpflags] & (tcp-rst) != 0)")
//	if err != nil {
//		log.Errorf("set bpf filter failed: %s", err)
//	}
//	source := gopacket.NewPacketSource(s.handler, s.handler.LinkType())
//	packets := source.Packets()
//
//	// local handler
//	err = s.localHandler.SetBPFFilter("(tcp[tcpflags] & (tcp-rst) != 0)")
//	if err != nil {
//		log.Errorf("set bpf filter failed for loopback: %s", err)
//	}
//	localSource := gopacket.NewPacketSource(s.localHandler, s.localHandler.LinkType())
//	localPackets := localSource.Packets()
//
//	handlePackets := func(packetStream chan gopacket.Packet) {
//		for {
//			select {
//			case packet, ok := <-packetStream:
//				if !ok {
//					return
//				}
//
//				if tcpLayer := packet.TransportLayer(); tcpLayer != nil {
//					l, ok := tcpLayer.(*layers.TCP)
//					if !ok {
//						continue
//					}
//
//					//端口扫描的响应包
//					if s.config.TcpFilter(l) {
//						if nl := packet.NetworkLayer(); nl != nil {
//							s.onRstAck(net.ParseIP(nl.NetworkFlow().Src().String()), int(l.SrcPort))
//						} else {
//							s.onNoRsp(net.ParseIP(nl.NetworkFlow().Src().String()), int(l.SrcPort))
//						}
//						continue
//					}
//
//				}
//			case <-s.ctx.Done():
//				return
//			}
//		}
//	}
//
//	go func() {
//		s.sendService()
//	}()
//
//	go func() {
//		handlePackets(packets)
//	}()
//
//	go func() {
//		handlePackets(localPackets)
//	}()
//}

//func (s *Scanner) Close() {
//	//s.handler.Close()
//	//s.localHandler.Close()
//}
