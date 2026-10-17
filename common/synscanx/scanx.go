package synscanx

import (
	"context"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/common/synscan"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/hostsparser"
	"github.com/yaklang/yaklang/common/utils/netutil"
	"github.com/yaklang/yaklang/common/utils/netutil/netroute"
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

	ipOpenPortMap *sync.Map

	startTime time.Time
	// halfOpen sends TCP and UDP through netstackvm.
	halfOpen halfOpenSYN
	// onSubmitTaskCallback: 每提交一个数据包的时候，这个 callback 调用一次
	onSubmitTaskCallback func(string, int)
	FromPing             bool
}

type routeCacheEntry struct {
	iface     *net.Interface
	gatewayIP net.IP
	srcIP     net.IP
	err       error
}

var routeCache struct {
	sync.Mutex
	entries map[string]routeCacheEntry
}

var routeLookup = netutil.Route

func resetRouteCache() {
	routeCache.Lock()
	defer routeCache.Unlock()
	routeCache.entries = make(map[string]routeCacheEntry)
}

// toUint16 safely converts any numeric interface{} value to uint16.
// Handles int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64.
// Returns 0 for nil or non-numeric types.
func toUint16(v interface{}) uint16 {
	switch n := v.(type) {
	case uint16:
		return n
	case int:
		return uint16(n)
	case int8:
		return uint16(n)
	case int16:
		return uint16(n)
	case int32:
		return uint16(n)
	case int64:
		return uint16(n)
	case uint:
		return uint16(n)
	case uint8:
		return uint16(n)
	case uint32:
		return uint16(n)
	case uint64:
		return uint16(n)
	case float32:
		return uint16(n)
	case float64:
		return uint16(n)
	default:
		return 0
	}
}

func getRoute(sampleIP string) (*net.Interface, net.IP, net.IP, error) {
	routeCache.Lock()
	if routeCache.entries == nil {
		routeCache.entries = make(map[string]routeCacheEntry)
	}
	entry, ok := routeCache.entries[sampleIP]
	routeCache.Unlock()
	if ok {
		return entry.iface, entry.gatewayIP, entry.srcIP, entry.err
	}

	iface, gatewayIP, srcIP, err := routeLookup(time.Second*2, sampleIP)
	entry = routeCacheEntry{
		iface:     iface,
		gatewayIP: gatewayIP,
		srcIP:     srcIP,
		err:       err,
	}

	routeCache.Lock()
	if existing, ok := routeCache.entries[sampleIP]; ok {
		routeCache.Unlock()
		return existing.iface, existing.gatewayIP, existing.srcIP, existing.err
	}
	routeCache.entries[sampleIP] = entry
	routeCache.Unlock()
	return iface, gatewayIP, srcIP, err
}

func NewScannerx(ctx context.Context, sample string, config *SynxConfig) (*Scannerx, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	rootCtx, cancel := context.WithCancel(ctx)
	s := &Scannerx{
		ctx:         rootCtx,
		cancel:      cancel,
		config:      config,
		startTime:   time.Now(),
		ports:       utils.NewPortsFilter(),
		loopbackMap: make(map[string]string),
		sampleIP:    sample,
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
		// The OS default route may leave through another NIC (a tunnel). The
		// gateway has to sit on the interface that will actually send.
		gatewayIP, err = gatewayForSelectedInterface(iface, srcIP, s.sampleIP)
		if err != nil {
			return utils.Errorf("get gateway for %s failed: %s", iface.Name, err)
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

// gatewayForSelectedInterface asks for the route from srcIP. A next hop that
// belongs to a different NIC is rejected so a tunnel default route cannot be
// paired with a physical interface.
func gatewayForSelectedInterface(iface *net.Interface, src net.IP, sample string) (net.IP, error) {
	if iface == nil {
		return nil, utils.Errorf("interface is nil")
	}
	dst := net.ParseIP(sample)
	if dst == nil || dst.To4() == nil {
		resolved := netx.LookupFirst(sample, netx.WithTimeout(3*time.Second))
		dst = net.ParseIP(resolved)
	}
	if dst == nil || dst.To4() == nil {
		return nil, utils.Errorf("sample %s is not ipv4", sample)
	}
	if src != nil {
		src = src.To4()
	}
	router, err := netroute.New()
	if err != nil {
		return nil, err
	}
	got, gateway, _, err := router.RouteWithSrc(iface.HardwareAddr, src, dst.To4())
	if err != nil {
		return nil, err
	}
	if got != nil && iface.Name != "" && got.Name != "" && got.Name != iface.Name {
		return nil, utils.Errorf("route from %s left via %s", iface.Name, got.Name)
	}
	if !gatewayOnInterface(iface, gateway) {
		return nil, utils.Errorf("gateway %v is not on %s", gateway, iface.Name)
	}
	return gateway, nil
}

func gatewayOnInterface(iface *net.Interface, gateway net.IP) bool {
	if gateway == nil || gateway.IsUnspecified() {
		return true
	}
	gateway = gateway.To4()
	if gateway == nil || iface == nil {
		return false
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return false
	}
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if ok && ipNet.Contains(gateway) {
			return true
		}
	}
	return false
}

func generateHostPort(ctx context.Context, nonExcludedHosts []string, nonExcludedPorts []int) <-chan *SynxTarget {
	out := make(chan *SynxTarget)
	go func() {
		defer close(out)
		for _, host := range nonExcludedHosts {
			for _, port := range nonExcludedPorts {
				select {
				case <-ctx.Done():
					return
				case out <- &SynxTarget{Host: host, Port: port}:
				}
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
		for hp := range generateHostPort(s.ctx, nonExcludedHosts, nonExcludedPorts) {
			host := hp.Host
			port := hp.Port

			proto, p := utils.ParsePortToProtoPort(port)
			target := &SynxTarget{
				Host: host,
				Port: p,
				Mode: TCP, // 默认 TCP
			}
			if proto == "udp" {
				target.Mode = UDP
			}
			// Pace and admission live in the netstackvm session for TCP and UDP.
			if s.config.maxOpenPorts > 0 {
				v, ok := s.ipOpenPortMap.Load(host)
				if ok && toUint16(v) >= s.config.maxOpenPorts {
					break
				}
			}

			s.callOnSubmitTask(host, port)

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

				for _, port := range nonExcludedPorts {
					proto, p := utils.ParsePortToProtoPort(port)
					target := &SynxTarget{
						Host: host,
						Port: p,
						Mode: TCP, // 默认 TCP
					}
					if proto == "udp" {
						target.Mode = UDP
					}
					if s.config.maxOpenPorts > 0 {
						v, ok := s.ipOpenPortMap.Load(host)
						if ok && toUint16(v) >= s.config.maxOpenPorts {
							break
						}
					}
					s.callOnSubmitTask(host, port)
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
				v, _ := s.ipOpenPortMap.LoadOrStore(host.String(), uint16(0))
				s.ipOpenPortMap.Store(host.String(), toUint16(v)+1)
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

	wCtx, wCancel := context.WithCancel(s.ctx)
	if err := s.startHalfOpen(wCtx); err != nil {
		wCancel()
		if s.cancel != nil {
			s.cancel()
		}
		return nil, fmt.Errorf("synscanx netstackvm: %w", err)
	}
	// 异步执行扫描流程
	go func() {
		defer func() {
			if err := recover(); err != nil {
				utils.PrintCurrentGoroutineRuntimeStack()
			}
		}()
		defer func() {
			wCancel()
			if s.halfOpen != nil {
				_ = s.halfOpen.Close()
			}
			close(resultCh)
		}()

		s.sendPacket(targetCh)
		countOnce.Do(func() {
			log.Infof("alive host count: %d open port count: %d cost: %v", len(ipCountMap), openPortCount, time.Since(s.startTime))
		})
	}()
	return resultCh, nil
}

func (s *Scannerx) sendPacket(targetCh <-chan *SynxTarget) {
	// Worker count matches the session admission limit. Each worker blocks in
	// ProbeSYN or ProbeUDP until that target is finished, so outstanding probes
	// stay bounded by the session slots.
	_, inFlight := synPacketRate(s.config)
	var tcpTargets chan *SynxTarget
	var udpTargets chan *SynxTarget
	var workers sync.WaitGroup
	if s.halfOpen != nil {
		tcpTargets = make(chan *SynxTarget)
		udpTargets = make(chan *SynxTarget)
		for i := 0; i < inFlight; i++ {
			workers.Add(2)
			go func() { defer workers.Done(); s.runTCPProbes(tcpTargets) }()
			go func() { defer workers.Done(); s.runUDPProbes(udpTargets) }()
		}
		defer func() {
			close(tcpTargets)
			close(udpTargets)
			workers.Wait()
		}()
	}

	for {
		select {
		case <-s.ctx.Done():
			log.Error("send packet canceled")
			return
		case target, ok := <-targetCh:
			if !ok {
				return
			}
			if target == nil {
				continue
			}
			host := target.Host
			port := target.Port
			switch target.Mode {
			case TCP:
				if tcpTargets == nil {
					log.Debugf("synscanx: skip TCP %s:%d, half-open session is not open", host, port)
					continue
				}
				select {
				case <-s.ctx.Done():
					return
				case tcpTargets <- target:
				}
			case UDP:
				if udpTargets == nil {
					log.Debugf("synscanx: skip UDP %s:%d, half-open session is not open", host, port)
					continue
				}
				select {
				case <-s.ctx.Done():
					return
				case udpTargets <- target:
				}
			default:
				log.Debugf("synscanx: skip %s:%d, unsupported mode %d", host, port, target.Mode)
			}
		}
	}
}

func (s *Scannerx) Close() {
	if s.cancel != nil {
		s.cancel()
	}
	if s.halfOpen != nil {
		_ = s.halfOpen.Close()
	}
}
