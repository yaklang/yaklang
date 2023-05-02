package hidsevent

import "yaklang.io/yaklang/common/gopsutil/net"

type ReverseShellInfo struct {
	Process       *ProcessMeta         `json:"process"`
	ParentProcess *ProcessMeta         `json:"parent_process"`
	Connections   []net.ConnectionStat `json:"connections"`
	VerboseReason string               `json:"verbose_reason"`
	Timestamp     int64                `json:"timestamp"`
}
