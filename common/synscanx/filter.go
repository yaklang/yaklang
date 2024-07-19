package synscanx

import (
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/hostsparser"
	"strconv"
	"strings"
)

func (s *Scannerx) nonExcludedHost(host string) bool {
	if s.config.excludeHosts != nil {
		if s.config.excludeHosts.Contains(host) {
			return true
		}
	}

	return false
}

func (s *Scannerx) nonExcludedPort(port int) bool {
	if s.config.ExcludePorts != nil {
		if s.config.ExcludePorts.Exist(fmt.Sprint(port)) {
			return true
		}
	}

	return false
}

func (s *Scannerx) GetNonExcludedPorts(ports string) []int {
	var nonExcludedPorts []int
	var fp string
	for _, port := range utils.ParseStringToPorts(ports) {
		if s.nonExcludedPort(port) {
			continue
		}
		fp += strconv.Itoa(port) + ","
		nonExcludedPorts = append(nonExcludedPorts, port)
	}
	s.ports = utils.NewPortsFilter(fp)
	if s.config.shuffle {
		utils.ShuffleInt(nonExcludedPorts)
	}
	return nonExcludedPorts
}

func (s *Scannerx) GetNonExcludedHosts(targets string) []string {
	var nonExcludedHosts []string

	for _, host := range utils.ParseStringToHosts(targets) {
		if s.nonExcludedHost(host) {
			continue
		}
		nonExcludedHosts = append(nonExcludedHosts, host)
	}
	s.hosts = hostsparser.NewHostsParser(s.ctx, strings.Join(nonExcludedHosts, ","))
	if s.config.shuffle {
		utils.ShuffleString(nonExcludedHosts)
	}
	return nonExcludedHosts
}
