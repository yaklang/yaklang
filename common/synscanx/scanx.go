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
	ports            *utils.PortsFilter
	OpenPortHandlers func(ip net.IP, port int)
	// openReporter is the production result path. It knows the probe protocol.
	// OpenPortHandlers remains for callers that only need the address.
	openReporter func(ip net.IP, port int, proto string)

	ipOpenPortMap *sync.Map

	startTime   time.Time
	summaryOnce sync.Once
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
	at        time.Time
}

// routeCacheTTL bounds a process-wide route, including a cached failure, so a
// VPN or DHCP change is picked up by the next scan.
var routeCacheTTL = 30 * time.Second

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
	if ok && time.Since(entry.at) < routeCacheTTL {
		return entry.iface, entry.gatewayIP, entry.srcIP, entry.err
	}

	iface, gatewayIP, srcIP, err := routeLookup(time.Second*2, sampleIP)
	entry = routeCacheEntry{
		iface:     iface,
		gatewayIP: gatewayIP,
		srcIP:     srcIP,
		err:       err,
		at:        time.Now(),
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
		ctx:       rootCtx,
		cancel:    cancel,
		config:    config,
		startTime: time.Now(),
		ports:     utils.NewPortsFilter(),
		sampleIP:  sample,
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
		if len(nonExcludedHosts) == 0 || len(nonExcludedPorts) == 0 {
			return
		}
		// Rotate hosts before walking the next port so one filtered host does
		// not occupy the whole probe window before the next host is touched.
		total := len(nonExcludedHosts) * len(nonExcludedPorts)
		for i := 0; i < total; i++ {
			host := nonExcludedHosts[i%len(nonExcludedHosts)]
			port := nonExcludedPorts[i/len(nonExcludedHosts)]
			select {
			case <-ctx.Done():
				return
			case out <- &SynxTarget{Host: host, Port: port}:
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
	// Zip TCP and UDP so a long TCP list cannot sit in front of every UDP port.
	nonExcludedPorts = interleaveEncodedPorts(nonExcludedPorts)

	tgCh := make(chan *SynxTarget)
	go func() {
		defer close(tgCh)
		skipHost := ""
		for hp := range generateHostPort(s.ctx, nonExcludedHosts, nonExcludedPorts) {
			host := hp.Host
			port := hp.Port
			if skipHost != "" && host == skipHost {
				continue
			}
			skipHost = ""

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
			// The cap is per host: later hosts are still submitted.
			if s.config.maxOpenPorts > 0 && s.ipOpenPortMap != nil {
				v, ok := s.ipOpenPortMap.Load(host)
				if ok && openCount(v) >= s.config.maxOpenPorts {
					skipHost = host
					continue
				}
			}

			s.callOnSubmitTask(host, p)

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
	nonExcludedPorts = interleaveEncodedPorts(nonExcludedPorts)
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
				if !utils.IsIPv4(host) {
					if utils.IsIPv6(host) {
						log.Debugf("synscanx: skip ipv6 ping target %s", host)
						continue
					}
					log.Infof("Resolving %s", host)
					addrs := lookupIPv4(host)
					if len(addrs) == 0 {
						continue
					}
					host = addrs[0]
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
					if s.config.maxOpenPorts > 0 && s.ipOpenPortMap != nil {
						v, ok := s.ipOpenPortMap.Load(host)
						if ok && openCount(v) >= s.config.maxOpenPorts {
							break
						}
					}
					s.callOnSubmitTask(host, p)
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

func openCount(v interface{}) int {
	switch n := v.(type) {
	case int:
		return n
	case uint16:
		return int(n)
	case int32:
		return int(n)
	case int64:
		return int(n)
	default:
		return int(toUint16(v))
	}
}

func (s *Scannerx) reportOpen(ip net.IP, port int, proto string) {
	if s == nil || ip == nil {
		return
	}
	if s.openReporter != nil {
		s.openReporter(ip, port, proto)
		return
	}
	if s.OpenPortHandlers != nil {
		s.OpenPortHandlers(ip, port)
	}
}

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
		outputFile, err = os.OpenFile(s.config.outputFile, outputFileFlags(s.config.appendOutput), 0o644)
		if err != nil {
			log.Errorf("open file %v failed; %s", s.config.outputFile, err)
		}
	}

	resultFilter := filter.NewFilter()
	defer resultFilter.Close()

	var hostsFilter *utils.HostsFilter
	var portsFilter *utils.PortsFilter
	// 从扫描目标中过滤出想要的目标
	if s.config.initFilterHosts != "" {
		log.Infof("filter hosts: %s", s.config.initFilterHosts)
		hostsFilter = hostResultFilter(s.config.initFilterHosts)
	}
	if s.config.initFilterPorts != "" {
		log.Infof("filter ports: %s", s.config.initFilterPorts)
		portsFilter = utils.NewPortsFilter(s.config.initFilterPorts)
	}
	if s.openReporter == nil && s.OpenPortHandlers == nil {
		s.openReporter = func(host net.IP, port int, proto string) {
			if host == nil {
				return
			}
			if proto == "" {
				proto = "tcp"
			}
			addr := utils.HostPort(host.String(), port)
			dedup := proto + "://" + addr
			openPortLock.Lock()
			if !resultFilter.Insert(dedup) {
				openPortLock.Unlock()
				return
			}
			hostAllowed := s.ports != nil && s.ports.Contains(port)
			if hostAllowed {
				if s.FromPing {
					hostAllowed = s._hosts != nil && s._hosts.Contains(host.String())
				} else {
					hostAllowed = s.hosts != nil && s.hosts.Contains(host.String())
				}
			}
			if !hostAllowed || (hostsFilter != nil && !hostsFilter.Contains(host.String())) || (portsFilter != nil && !portsFilter.Contains(port)) {
				openPortLock.Unlock()
				return
			}
			if s.config.maxOpenPorts > 0 && s.ipOpenPortMap != nil {
				v, _ := s.ipOpenPortMap.LoadOrStore(host.String(), 0)
				s.ipOpenPortMap.Store(host.String(), openCount(v)+1)
			}
			ipCountMap[host.String()] = struct{}{}
			openPortCount++
			if outputFile != nil {
				recorded := addr
				if proto == "udp" {
					recorded = "udp://" + addr
				}
				_, _ = outputFile.Write([]byte(fmt.Sprintf("%s%s\n", s.config.outputFilePrefix, recorded)))
			}
			openPortLock.Unlock()

			result := &synscan.SynScanResult{
				Host:     host.String(),
				Port:     port,
				Protocol: proto,
			}
			s.config.callCallback(result)
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
			if outputFile != nil {
				_ = outputFile.Close()
			}
			wCancel()
			if s.halfOpen != nil {
				_ = s.halfOpen.Close()
			}
			close(resultCh)
		}()

		s.sendPacket(targetCh)
		s.summaryOnce.Do(func() {
			log.Infof("alive host count: %d open port count: %d cost: %v", len(ipCountMap), openPortCount, time.Since(s.startTime))
		})
	}()
	return resultCh, nil
}

func outputFileFlags(appendOutput bool) int {
	if appendOutput {
		return os.O_WRONLY | os.O_CREATE | os.O_APPEND
	}
	return os.O_WRONLY | os.O_CREATE | os.O_TRUNC
}

// probeDispatchPark is how many targets of one protocol may wait here while
// that protocol's workers are busy. The other protocol keeps moving until
// its own park fills. A single-protocol scan stops at this cap and waits for
// a worker, which is the same backpressure as a full window.
const probeDispatchPark = 8192

func interleaveEncodedPorts(ports []int) []int {
	var tcp, udp []int
	for _, port := range ports {
		proto, _ := utils.ParsePortToProtoPort(port)
		if proto == "udp" {
			udp = append(udp, port)
			continue
		}
		tcp = append(tcp, port)
	}
	if len(tcp) == 0 || len(udp) == 0 {
		return ports
	}
	out := make([]int, 0, len(ports))
	i, j := 0, 0
	for i < len(tcp) || j < len(udp) {
		if i < len(tcp) {
			out = append(out, tcp[i])
			i++
		}
		if j < len(udp) {
			out = append(out, udp[j])
			j++
		}
	}
	return out
}

func (s *Scannerx) sendPacket(targetCh <-chan *SynxTarget) {
	// TCP and UDP have separate worker pools. A silent UDP probe waits out
	// its own deadline without occupying a TCP worker, and a busy protocol
	// queue does not stop the dispatcher from handing work to the other one.
	_, inFlight := synPacketRate(s.config)
	if inFlight < 1 {
		inFlight = 1
	}
	if s.halfOpen == nil {
		for {
			select {
			case <-s.ctx.Done():
				log.Errorf("send packet canceled")
				return
			case _, ok := <-targetCh:
				if !ok {
					return
				}
			}
		}
	}
	tcpTargets := make(chan *SynxTarget)
	udpTargets := make(chan *SynxTarget)
	var workers sync.WaitGroup
	for i := 0; i < inFlight; i++ {
		workers.Add(2)
		go func() { defer workers.Done(); s.runProbes(tcpTargets) }()
		go func() { defer workers.Done(); s.runProbes(udpTargets) }()
	}
	defer func() {
		close(tcpTargets)
		close(udpTargets)
		workers.Wait()
	}()
	s.dispatchTargets(targetCh, tcpTargets, udpTargets)
}

func (s *Scannerx) dispatchTargets(in <-chan *SynxTarget, tcpOut, udpOut chan *SynxTarget) {
	park := probeDispatchPark
	var tcpQ, udpQ []*SynxTarget
	var tcpHead, udpHead *SynxTarget
	inputOpen := true
	for {
		if tcpHead == nil && len(tcpQ) > 0 {
			tcpHead = tcpQ[0]
			tcpQ = tcpQ[1:]
		}
		if udpHead == nil && len(udpQ) > 0 {
			udpHead = udpQ[0]
			udpQ = udpQ[1:]
		}
		tcpPending := len(tcpQ)
		if tcpHead != nil {
			tcpPending++
		}
		udpPending := len(udpQ)
		if udpHead != nil {
			udpPending++
		}
		if tcpHead == nil && udpHead == nil && !inputOpen {
			return
		}
		var tcpSend, udpSend chan *SynxTarget
		if tcpHead != nil {
			tcpSend = tcpOut
		}
		if udpHead != nil {
			udpSend = udpOut
		}
		var input <-chan *SynxTarget
		if inputOpen && tcpPending < park && udpPending < park {
			input = in
		}
		select {
		case <-s.ctx.Done():
			log.Errorf("send packet canceled")
			return
		case target, ok := <-input:
			if !ok {
				inputOpen = false
				continue
			}
			if target == nil {
				continue
			}
			switch target.Mode {
			case TCP:
				tcpQ = append(tcpQ, target)
			case UDP:
				udpQ = append(udpQ, target)
			default:
				log.Debugf("synscanx: skip %s:%d, unsupported mode %d", target.Host, target.Port, target.Mode)
			}
		case tcpSend <- tcpHead:
			tcpHead = nil
		case udpSend <- udpHead:
			udpHead = nil
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
