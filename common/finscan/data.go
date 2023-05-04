package finscan

import (
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
)

type FinScanResult struct {
	Host   string
	Port   int
	Status int
}

func (s *FinScanResult) Show() {
	if s == nil {
		return
	}
	println(s.String())
}

func (s *FinScanResult) String() string {
	if s == nil {
		return ""
	}
	return fmt.Sprintf("CLOSED: %-20s from finscan", utils.HostPort(s.Host, s.Port))
}
