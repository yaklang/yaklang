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
	"sync"
	"time"
)

type Scannerx struct {
	ctx    context.Context
	config *SynxConfig

	sampleIP         string
	hosts            *hostsparser.HostsParser
	ports            *utils.PortsFilter
	loopbackMap      map[string]string
	OpenPortHandlers func(ip net.IP, port int)

	// MAC地址表
	macCacheTable *sync.Map
	MacHandlers   func(ip net.IP, addr net.HardwareAddr)

	Handle *pcap.Handle

	limiter   *rate.Limiter
	startTime time.Time
	// onSubmitTaskCallback: 每提交一个数据包的时候，这个 callback 调用一次
	onSubmitTaskCallback func(string, int)
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

func (s *Scannerx) SubmitTarget(targets, ports string, targetCh chan *SynxTarget) {
	filteredHosts := s.GetNonExcludedHosts(targets)
	filtedPorts := s.GetNonExcludedPorts(ports)
	s.OnSubmitTask(func(h string, p int) {
		s.config.callSubmitTaskCallback(utils.HostPort(h, p))
	})
	for _, host := range filteredHosts {
		for _, port := range filtedPorts {
			s.rateLimit()
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
		s.sendPacket(s.ctx, targetCh)
	}()
	wg.Wait()

	time.Sleep(s.config.waiting)

	wCancel()

	s.Close()

	done <- struct{}{}
	log.Infof("open port count: %d", openPortCount)
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
			packet, err := s.assemblePacket(host, port, proto)
			if err != nil {
				log.Errorf("assemble packet failed: %v", err)
				continue
			}
			err = s.Handle.WritePacketData(packet)
			if err != nil {
				log.Errorf("write to device syn failed: %v", s.handleError(err))
				return
			}
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
