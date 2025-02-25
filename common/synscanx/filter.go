package synscanx

import (
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/hostsparser"
	"strconv"
	"strings"
	"time"
)

func (s *Scannerx) excludedHost(host string) bool {
	if s.config.excludeHosts != nil {
		if s.config.excludeHosts.Contains(host) {
			return true
		}
	}

	return false
}

func (s *Scannerx) excludedPort(port int) bool {
	if s.config.excludePorts != nil {
		if s.config.excludePorts.Contains(port) {
			return true
		}
	}

	return false
}

func (s *Scannerx) GetNonExcludedPorts(ports string) []int {
	var nonExcludedPorts []int
	var builder strings.Builder
	for _, port := range utils.ParseStringToPorts(ports) {
		if s.excludedPort(port) {
			continue
		}
		builder.WriteString(strconv.Itoa(port))
		builder.WriteString(",")
		nonExcludedPorts = append(nonExcludedPorts, port)
	}
	s.ports.Add(builder.String())
	if s.config.shuffle {
		utils.ShuffleInt(nonExcludedPorts)
	}
	return nonExcludedPorts
}

func (s *Scannerx) GetNonExcludedHosts(targets string) []string {
	var nonExcludedHosts []string

	for _, host := range utils.ParseStringToHosts(targets) {
		if !utils.IsIPv4(host) && !utils.IsIPv6(host) {
			for _, _host := range netx.LookupAll(host, netx.WithTimeout(3*time.Second)) {
				if s.excludedHost(_host) {
					continue
				}
				if utils.IsIPv4(_host) || utils.IsIPv6(_host) {
					nonExcludedHosts = append(nonExcludedHosts, _host)
				}
			}
		}
		if s.excludedHost(host) {
			continue
		}
		if utils.IsIPv4(host) || utils.IsIPv6(host) {
			nonExcludedHosts = append(nonExcludedHosts, host)
		}
	}
	s.hosts = hostsparser.NewHostsParser(s.ctx, strings.Join(nonExcludedHosts, ","))
	if s.config.shuffle {
		utils.ShuffleString(nonExcludedHosts)
	}
	return nonExcludedHosts
}
