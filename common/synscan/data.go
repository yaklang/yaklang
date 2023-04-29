package synscan

import (
	"fmt"
	"yaklang/common/utils"
)

type SynScanResult struct {
	Host string
	Port int
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
	return fmt.Sprintf("OPEN: %-20s from synscan", utils.HostPort(s.Host, s.Port))
}