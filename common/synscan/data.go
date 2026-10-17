package synscan

import (
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
)

type SynScanResult struct {
	Host     string
	Port     int
	Protocol string
}

func (s *SynScanResult) Show() {
	if s == nil {
		return
	}
	println(s.String())
}

func (s *SynScanResult) String() string {
	if s == nil {
		return ""
	}
	target := utils.HostPort(s.Host, s.Port)
	if s.Protocol == "udp" {
		target = "udp://" + target
	}
	return fmt.Sprintf("OPEN: %-20s from synscan", target)
}
