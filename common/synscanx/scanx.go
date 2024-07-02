package synscanx

import (
	"context"
	"fmt"
	"github.com/google/gopacket"
	"github.com/yaklang/pcap"
	"github.com/yaklang/yaklang/common/synscan"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/hostsparser"
	"net"
	"strings"
	"time"
)

type Scannerx struct {
	Ctx    context.Context
	Cancel context.CancelFunc
	config *SynxConfig

	hosts *hostsparser.HostsParser

	OpenPortHandlers func(ip net.IP, port int)
	MacHandlers      func(ip net.IP, addr net.HardwareAddr)

	Handle *pcap.Handle

	startTime time.Time
}

type SynScanTarget struct {
	Host  string
	Port  int
	Proto string
}

func NewScannerx(ctx context.Context, config *SynxConfig) *Scannerx {
	s := &Scannerx{
		Ctx:       ctx,
		config:    config,
		startTime: time.Now(),
	}
	return s
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

	for _, p := range utils.ParseStringToPorts(ports) {
		_, p := utils.ParsePortToProtoPort(p)
		//if proto == "udp" {
		//	log.Errorf("UDP port is not supported in synscan, please use 'servicescan' to scan UDP port: %v", p)
		//	continue
		//}
		if s.filterPort(p) {
			continue
		}
		filteredPorts = append(filteredPorts, p)
	}

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

func (s *Scannerx) SubmitTask(targets, ports string, targetCh chan *synscan.SynScanResult) {
	filteredHosts := s.GetFilterHosts(targets)
	filtedPorts := s.GetFilterPorts(ports)

	for _, host := range filteredHosts {
		for _, port := range filtedPorts {
			targetCh <- &synscan.SynScanResult{
				Host: host,
				Port: port,
			}
		}
	}
}

func (s *Scannerx) Scan(done chan struct{}, targetCh, resultCh chan *synscan.SynScanResult) error {
	iface, gatewayIP, srcIP := s.config.Iface, s.config.GatewayIP, s.config.SourceIP
	if iface == nil {
		return utils.Errorf("iface is nil")
	}

	return nil
}

func (s *Scannerx) HandlerWritePacket() {

}

func (s *Scannerx) Close() {
	s.Handle.Close()
}

func (s *Scannerx) HandlerReadPacket(packet gopacket.Packet) {

}
