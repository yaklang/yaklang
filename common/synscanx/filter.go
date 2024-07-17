package synscanx

import (
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/hostsparser"
	"strconv"
	"strings"
)

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
	var fp string
	for _, port := range utils.ParseStringToPorts(ports) {
		if s.filterPort(port) {
			continue
		}
		fp += strconv.Itoa(port) + ","
		filteredPorts = append(filteredPorts, port)
	}
	s.ports = utils.NewPortsFilter(fp)
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
	s.hosts = hostsparser.NewHostsParser(s.ctx, strings.Join(filteredHosts, ","))
	return filteredHosts
}
