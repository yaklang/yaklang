package spec

import (
	"fmt"
	"yaklang.io/yaklang/common/fp"
)

type ScanFingerprintTask struct {
	Hosts          string              `json:"hosts"`
	Ports          string              `json:"ports"`
	Protos         []fp.TransportProto `json:"protos"`
	TimeoutSeconds int                 `json:"timeout_seconds"`
}

func (s *ScanFingerprintTask) String() string {
	return fmt.Sprintf("[TASK] (%v) hosts:%v ports:%v", s.Protos, s.Hosts, s.Ports)
}
