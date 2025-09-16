package synscanx

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/netstackvm"
	"net"
	"os"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/yaklang/pcap"
	"github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/common/synscan"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/hostsparser"
	"github.com/yaklang/yaklang/common/utils/netutil"
	"golang.org/x/time/rate"
)

type Scannerx struct {
	ctx    context.Context
	cancel context.CancelFunc

	config *SynxConfig

	// 取样IP
	sampleIP string
	// 存放未排除的目标
	hosts  *hostsparser.HostsParser
	_hosts *utils.HostsFilter
	// 存放未排除的端口
	ports *utils.PortsFilter
	// loopback 对应的实际IP
	loopbackMap      map[string]string
	OpenPortHandlers func(ip net.IP, port int)

	PacketChan chan []byte
	LoopPacket chan []byte

	ipOpenPortMap *sync.Map
	// MAC地址表
	macCacheTable *sync.Map
	MacHandlers   func(ip net.IP, addr net.HardwareAddr)

	// 缓存内外网段
	ifaceIPNetV4 *net.IPNet
	ifaceIPNetV6 *net.IPNet
	ifaceUpdated bool

	Handle    *pcap.Handle
	limiter   *rate.Limiter
	startTime time.Time
	// onSubmitTaskCallback: 每提交一个数据包的时候，这个 callback 调用一次
	onSubmitTaskCallback func(string, int)
	FromPing             bool
}

var routeCache struct {
	iface     *net.Interface
	gatewayIP net.IP
	srcIP     net.IP
	err       error
	once      sync.Once
}

func getRoute(sampleIP string) (*net.Interface, net.IP, net.IP, error) {
	routeCache.once.Do(func() {
		routeCache.iface, routeCache.gatewayIP, routeCache.srcIP, routeCache.err = netutil.Route(time.Second*2, sampleIP)
	})
	return routeCache.iface, routeCache.gatewayIP, routeCache.srcIP, routeCache.err
}

func NewScannerx(ctx context.Context, sample string, config *SynxConfig) (*Scannerx, error) {
	limitInterval := time.Duration(config.rateLimitDelayMs * float64(time.Millisecond))
	if ctx == nil {
		ctx = context.Background()
	}
	rootCtx, cancel := context.WithCancel(ctx)
	s := &Scannerx{
		ctx:           rootCtx,
		cancel:        cancel,
		config:        config,
		startTime:     time.Now(),
		ports:         utils.NewPortsFilter(),
		macCacheTable: new(sync.Map),
		PacketChan:    make(chan []byte, 1024),
		LoopPacket:    make(chan []byte, 1024),
		loopbackMap:   make(map[string]string),
		sampleIP:      sample,
		limiter:       rate.NewLimiter(rate.Every(limitInterval), config.rateLimitDelayGap),
	}
	if s.config.maxOpenPorts > 0 {
		s.ipOpenPortMap = new(sync.Map)
	}
	// 初始化发包相关的配置
	err := s.initEssentialInfo()
	if err != nil {
		return nil, err
	}

	return s, nil
}

func (s *Scannerx) initEssentialInfo() error {
	var iface *net.Interface
	var srcIP, gatewayIP net.IP
	var err error

	// 如果没有指定网卡名,就通过路由获取
	if s.config.netInterface == "" {
		iface, gatewayIP, srcIP, err = getRoute(s.sampleIP)
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
		_, gatewayIP, _, err = getRoute(srcIP.String())
		if err != nil {
			return utils.Errorf("get gateway failed: %s", err)
		}
	}

	s.config.Iface = iface
	s.config.SourceIP = srcIP
	s.config.SourceMac = iface.HardwareAddr
	s.config.GatewayIP = gatewayIP

	// 不确定扫描目标中是否存在回环地址，所以这里先初始化一个回环地址的映射表
	s.loopbackMap["127.0.0.1"] = s.config.SourceIP.String()
	return nil
}

func (s *Scannerx) rateLimit() {
	s.limiter.Wait(s.ctx)
}

func generateHostPort(nonExcludedHosts []string, nonExcludedPorts []int) <-chan *SynxTarget {
	out := make(chan *SynxTarget)
	go func() {
		defer close(out)
		for _, host := range nonExcludedHosts {
			for _, port := range nonExcludedPorts {
				out <- &SynxTarget{Host: host, Port: port}
			}
		}
	}()
	return out
}

func (s *Scannerx) SubmitTarget(targets, ports string) (<-chan *SynxTarget, error) {
	nonExcludedHosts := s.GetNonExcludedHosts(targets)
	nonExcludedPorts := s.GetNonExcludedPorts(ports)
	if len(nonExcludedHosts) == 0 || len(nonExcludedPorts) == 0 {
		s.cancel()
		return nil, errors.New("targets or ports is empty")
	}
	s.OnSubmitTask(func(h string, p int) {
		s.config.callSubmitTaskCallback(utils.HostPort(h, p))
	})

	tgCh := make(chan *SynxTarget)
	go func() {
		defer close(tgCh)
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
				log.Infof("submitTarget canceled")
				return
			case tgCh <- target:
			}
		}
	}()
	return tgCh, nil
}

func (s *Scannerx) SubmitTargetFromPing(res chan string, ports string) <-chan *SynxTarget {
	tgCh := make(chan *SynxTarget)
	nonExcludedPorts := s.GetNonExcludedPorts(ports)

	s.OnSubmitTask(func(h string, p int) {
		s.config.callSubmitTaskCallback(utils.HostPort(h, p))
	})
	s._hosts = utils.NewHostsFilter()
	var lock sync.Mutex
	go func() {
		defer close(tgCh)
		for {
			select {
			case <-s.ctx.Done():
				log.Infof("SubmitTargetFromPing canceled")
				return
			case host, ok := <-res:
				if !ok {
					log.Debugf("ping result channel closed")
					return
				}
				if !utils.IsIPv4(host) && !utils.IsIPv6(host) {
					log.Infof("Resolving %s", host)
					host = netx.LookupFirst(host, netx.WithTimeout(3*time.Second))
				}

				if s.excludedHost(host) {
					continue
				}
				lock.Lock()
				s._hosts.Add(host)
				lock.Unlock()

				if s.isInternalAddress(host) {
					s.arp(host)
				}
				for _, port := range nonExcludedPorts {
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
						log.Infof("SubmitTargetFromPing canceled")
						return
					case tgCh <- target:
					}
				}
			}
		}
	}()
	return tgCh
}

var countOnce sync.Once

func (s *Scannerx) Scan(targetCh <-chan *SynxTarget) (chan *synscan.SynScanResult, error) {
	defer func() {
		if err := recover(); err != nil {
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()
	resultCh := make(chan *synscan.SynScanResult, 1024)
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
			originalHost := host.String()
			if s.loopbackMap[originalHost] != "" {
				host = net.ParseIP(s.loopbackMap[host.String()])
			}

			addr := utils.HostPort(host.String(), port)
			if resultFilter.Exist(addr) {
				return
			}

			resultFilter.Insert(addr)
			if s.FromPing {
				if !((s._hosts.Contains(host.String()) || s._hosts.Contains(originalHost)) && s.ports.Contains(port)) {
					return
				}
			} else {
				if !((s.hosts.Contains(host.String()) || s.hosts.Contains(originalHost)) && s.ports.Contains(port)) {
					return
				}

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

			if outputFile != nil {
				outputFile.Write(
					[]byte(fmt.Sprintf(
						"%s%v\n",
						s.config.outputFilePrefix,
						addr,
					)),
				)
			}

			select {
			case <-s.ctx.Done():
				return
			case resultCh <- result:
			}
		}
	}

	go func() {
		defer func() {
			if err := recover(); err != nil {
				utils.PrintCurrentGoroutineRuntimeStack()
			}
		}()
		defer func() {
			close(resultCh)
			close(s.PacketChan)
			close(s.LoopPacket)
		}()

		if !s.FromPing {
			s.arpScan()
			time.Sleep(1 * time.Second)
		}
		option := netstackvm.DefaultSYNScanOption()
		option = append(option, netstackvm.WithSelectedDeviceName(s.config.netInterface))
		vm, err := netstackvm.NewSystemNetStackVMWithoutDHCP(option...)
		if err != nil {
			log.Errorf("create netstack vm failed: %v", err)
			return
		}
		err = s.NetStackScan(vm, targetCh)
		if err != nil {
			log.Error(err)
			return
		}
	}()
	return resultCh, nil
}

func (s *Scannerx) NetStackScan(vm *netstackvm.NetStackVirtualMachine, targetCh <-chan *SynxTarget) error {
	wg := sync.WaitGroup{}
	for target := range targetCh {
		wg.Add(1)
		go func() {
			defer wg.Done()
			addr := utils.HostPort(target.Host, target.Port)
			switch target.Mode {
			case TCP:
				conn, err := vm.DialTCP(s.config.waiting, addr)
				if err != nil || conn == nil {
					return
				}
				conn.Close()
				s.OpenPortHandlers(net.ParseIP(target.Host), target.Port)
			case UDP:
			case ICMP:
				//todo
			case ARP:
				//todo
			default:
				log.Errorf("unsupported protocol: %v", target.Mode)
			}
		}()
	}
	wg.Wait()
	return nil
}

func (s *Scannerx) sendPacket(targetCh <-chan *SynxTarget) {
	for {
		select {
		case <-s.ctx.Done():
			log.Error("send packet canceled")
			return
		case target, ok := <-targetCh:
			if !ok {
				return
			}
			host := target.Host
			port := target.Port
			proto := target.Mode
			packet, err := s.assemblePacket(host, port, proto)
			if err != nil {
				log.Debugf("assemble packet failed: %v", err)
				continue
			}
			if utils.IsLoopback(host) {
				s.LoopPacket <- packet
			} else {
				s.PacketChan <- packet
			}
			//err = s.Handle.WritePacketData(packet)
			//if err != nil {
			//	log.Errorf("write to device syn failed: %v[%s:%d]", s.handleError(err), host, port)
			//	continue
			//}
		}
	}
}

func (s *Scannerx) assemblePacket(host string, port int, proto ProtocolType) ([]byte, error) {
	switch proto {
	case TCP:
		return s.assembleSynPacket(host, port)
	case UDP:
		return s.assembleUdpPacket(host, port)
	case ICMP:
	case ARP:
		return s.assembleArpPacket(host)
	}
	return nil, nil
}

func (s *Scannerx) Close() {
	s.Handle.Close()
}
