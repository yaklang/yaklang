package synscanx

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"github.com/yaklang/pcap"
	"github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/common/synscan"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/hostsparser"
	"github.com/yaklang/yaklang/common/utils/netutil"
	"golang.org/x/time/rate"
	"net"
	"os"
	"runtime"
	"sync"
	"time"
)

type Scannerx struct {
	ctx    context.Context
	config *SynxConfig

	// 取样IP
	sampleIP string
	// 存放未排除的目标
	hosts *hostsparser.HostsParser
	//_hosts *utils.HostsFilter
	// 存放未排除的端口
	ports *utils.PortsFilter
	// loopback 对应的实际IP
	loopbackMap      map[string]string
	OpenPortHandlers func(ip net.IP, port int)

	ipOpenPortMap *sync.Map
	// MAC地址表
	macCacheTable *sync.Map
	MacHandlers   func(ip net.IP, addr net.HardwareAddr)

	Handle     *pcap.Handle
	ConnHandle *net.IPConn
	limiter    *rate.Limiter
	startTime  time.Time
	// onSubmitTaskCallback: 每提交一个数据包的时候，这个 callback 调用一次
	onSubmitTaskCallback func(string, int)
	FromPing             bool
}

func NewScannerx(ctx context.Context, sample string, config *SynxConfig) (*Scannerx, error) {
	limitInterval := time.Duration(config.rateLimitDelayMs * float64(time.Millisecond))
	s := &Scannerx{
		ctx:           ctx,
		config:        config,
		startTime:     time.Now(),
		macCacheTable: new(sync.Map),
		loopbackMap:   make(map[string]string),
		sampleIP:      sample,
		limiter:       rate.NewLimiter(rate.Every(limitInterval), config.rateLimitDelayGap),
	}
	if s.config.maxOpenPorts > 0 {
		s.ipOpenPortMap = new(sync.Map)
	}
	// 初始化发包相关的配置
	err := s.initEssentialInfo()

	err = s.initHandle()
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Scannerx) initEssentialInfo() error {
	var iface *net.Interface
	var srcIP, gatewayIP net.IP
	var err error

	if utils.IsLoopback(s.sampleIP) {
		iface, err = pcaputil.GetLoopBackNetInterface()
		if err != nil {
			return utils.Errorf("get loopback iface failed: %s", err)
		}
		gatewayIP = net.IPv4(127, 0, 0, 1)
		srcIP = net.IPv4(127, 0, 0, 1)
		if iface.HardwareAddr == nil {
			iface.HardwareAddr = net.HardwareAddr{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
		}
		s.loopbackMap[srcIP.String()] = s.sampleIP
	} else {
		// 如果没有指定网卡名,就通过路由获取
		if s.config.netInterface == "" {
			iface, gatewayIP, srcIP, err = netutil.Route(time.Second*2, s.sampleIP)
			if err != nil {
				return utils.Errorf("get iface failed: %s", err)
			}
		} else {
			// 支持 net interface name 和 pcap dev name
			iface, err = net.InterfaceByName(s.config.netInterface)
			if err != nil {
				iface, err = pcaputil.PcapIfaceNameToNetInterface(s.config.netInterface)
				if err != nil {
					return errors.Errorf("get iface failed: %s", err)
				}
			}
			addrs, err := iface.Addrs()
			if err != nil {
				return err
			}
			for _, addr := range addrs {
				ip := addr.(*net.IPNet).IP
				if utils.IsIPv6(ip.String()) {
					srcIP = ip
				}
				if utils.IsIPv4(ip.String()) {
					srcIP = ip
					break
				}
			}
			if srcIP == nil {
				return utils.Errorf("iface: %s has no addrs", iface.Name)
			}
			// 通过网卡名获取到网卡的 IP 地址后，再通过路由获取网关 IP 地址，网关 IP 地址用于获取网关的 MAC 地址，用于外网扫描
			_, gatewayIP, _, err = netutil.Route(time.Second*2, srcIP.String())
			if err != nil {
				return utils.Errorf("get gateway failed: %s", err)
			}
		}
	}

	s.config.Iface = iface
	s.config.SourceIP = srcIP
	s.config.SourceMac = iface.HardwareAddr
	s.config.GatewayIP = gatewayIP
	return nil
}

func (s *Scannerx) rateLimit() {
	s.limiter.Wait(s.ctx)
}

func generateHostPort(nonExcludedHosts []string, nonExcludedPorts []int) <-chan SynxTarget {
	out := make(chan SynxTarget)
	go func() {
		defer close(out)
		for _, host := range nonExcludedHosts {
			for _, port := range nonExcludedPorts {
				out <- SynxTarget{Host: host, Port: port}
			}
		}
	}()
	return out
}

func (s *Scannerx) SubmitTarget(targets, ports string, targetCh chan *SynxTarget) {
	nonExcludedHosts := s.GetNonExcludedHosts(targets)
	nonExcludedPorts := s.GetNonExcludedPorts(ports)
	s.OnSubmitTask(func(h string, p int) {
		s.config.callSubmitTaskCallback(utils.HostPort(h, p))
	})
	for hp := range generateHostPort(nonExcludedHosts, nonExcludedPorts) {
		host := hp.Host
		port := hp.Port
		s.rateLimit()
		if s.config.maxOpenPorts > 0 {
			v, ok := s.ipOpenPortMap.Load(host)
			if ok {
				if v.(uint16) >= s.config.maxOpenPorts {
					break
				}
			}
		}

		s.callOnSubmitTask(host, port)
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
		case <-s.ctx.Done():
			log.Infof("SubmitTarget canceled")
			return
		case targetCh <- target:
		}
	}
}

//func (s *Scannerx) SubmitTargetFromPing(res chan string, ports string, ch chan *SynxTarget) {
//	nonExcludedPorts := s.GetNonExcludedPorts(ports)
//
//	ifaceIPNetV4, ifaceIPNetV6 := s.getInterfaceNetworks()
//	s.OnSubmitTask(func(h string, p int) {
//		s.config.callSubmitTaskCallback(utils.HostPort(h, p))
//	})
//
//	for {
//		select {
//		case <-s.ctx.Done():
//			log.Infof("SubmitTargetFromPing canceled")
//			return
//		case host, ok := <-res:
//			if !ok {
//				log.Infof("ping result channel closed")
//				return
//			}
//			if s.nonExcludedHost(host) {
//				return
//			}
//			s._hosts.Add(host)
//			if s.isInternalAddress(host, ifaceIPNetV4, ifaceIPNetV6) {
//				s.arp(host)
//			}
//			for _, port := range nonExcludedPorts {
//				s.rateLimit()
//				if s.config.maxOpenPorts > 0 {
//					v, ok := s.ipOpenPortMap.Load(host)
//					if ok {
//						if v.(uint16) >= s.config.maxOpenPorts {
//							break
//						}
//					}
//				}
//				s.callOnSubmitTask(host, port)
//				proto, p := utils.ParsePortToProtoPort(port)
//				target := &SynxTarget{
//					Host: host,
//					Port: p,
//					Mode: TCP, // 默认 TCP
//				}
//				if proto == "udp" {
//					target.Mode = UDP
//				}
//				ch <- target
//			}
//		}
//	}
//
//}

func (s *Scannerx) Scan(done chan struct{}, targetCh chan *SynxTarget, resultCh chan *synscan.SynScanResult) error {
	openPortLock := new(sync.Mutex)
	var openPortCount int
	ipCountMap := make(map[string]struct{}, 16)
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

	var hostsFilter *utils.HostsFilter
	var portsFilter *utils.PortsFilter
	// 从扫描目标中过滤出想要的目标
	if s.config.initFilterHosts != "" {
		log.Infof("filter hosts: %s", s.config.initFilterHosts)
		hostsFilter = utils.NewHostsFilter(s.config.initFilterHosts)
	}
	if s.config.initFilterPorts != "" {
		log.Infof("filter ports: %s", s.config.initFilterPorts)
		portsFilter = utils.NewPortsFilter(s.config.initFilterPorts)
	}
	if s.OpenPortHandlers == nil {
		s.OpenPortHandlers = func(host net.IP, port int) {
			openPortLock.Lock()
			defer openPortLock.Unlock()

			if s.loopbackMap[host.String()] != "" {
				host = net.ParseIP(s.loopbackMap[host.String()])
			}

			addr := utils.HostPort(host.String(), port)
			if resultFilter.Exist(addr) {
				return
			}

			resultFilter.Insert(addr)

			if !(s.hosts.Contains(host.String()) && s.ports.Contains(port)) {
				return
			}

			if hostsFilter != nil && !hostsFilter.Contains(host.String()) {
				return
			}
			if portsFilter != nil && !portsFilter.Contains(port) {
				return
			}
			if s.config.maxOpenPorts > 0 {
				v, _ := s.ipOpenPortMap.LoadOrStore(host.String(), 1)
				s.ipOpenPortMap.Store(host.String(), v.(uint16)+1)
			}

			if _, exists := ipCountMap[host.String()]; !exists {
				ipCountMap[host.String()] = struct{}{}
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
		time.Sleep(1 * time.Second)
		s.sendPacket(s.ctx, targetCh)
	}()
	wg.Wait()

	time.Sleep(s.config.waiting)

	wCancel()

	s.Close()

	done <- struct{}{}
	endTime := time.Now()
	log.Infof("alive host count: %d open port count: %d cost: %v", len(ipCountMap), openPortCount, endTime.Sub(s.startTime))
	return nil
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
			isLoopback, packet, err := s.assemblePacket(host, port, proto)
			if err != nil {
				log.Errorf("assemble packet failed: %v", err)
				continue
			}

			if (isLoopback && runtime.GOOS == "linux") || (isLoopback && s.hosts.Size() > 1) {
				log.Infof("loopback use net conn: %s", host)
				//tcpLayer := packet[20:]
				tcpConn4, err := net.ListenIP("ip4:tcp", &net.IPAddr{IP: net.ParseIP(fmt.Sprintf("0.0.0.0:%d", 12345))})
				if err != nil {
					log.Errorf("Error listening: %v", err)
					break
				}
				s.ConnHandle = tcpConn4
				tcpLayer := packet

				tcpConn4.SetDeadline(time.Now().Add(500 * time.Millisecond))
				// 发送数据
				_, err = tcpConn4.WriteTo(tcpLayer, &net.IPAddr{IP: net.ParseIP(host)})
				if err != nil {
					log.Errorf("Error sending data: %v", err)
					break
				}

			} else {
				err = s.Handle.WritePacketData(packet)
				if err != nil {
					log.Errorf("write to device syn failed: %v", s.handleError(err))
					break
				}
			}
		}
	}
}

func (s *Scannerx) assemblePacket(host string, port int, proto ProtocolType) (bool, []byte, error) {
	switch proto {
	case TCP:
		return s.assembleSynPacket(host, port)
	case UDP:
		return s.assembleUdpPacket(host, port)
	case ICMP:
	case ARP:
		return s.assembleArpPacket(host)
	}
	return false, nil, nil
}

func (s *Scannerx) Close() {
	s.Handle.Close()
}
