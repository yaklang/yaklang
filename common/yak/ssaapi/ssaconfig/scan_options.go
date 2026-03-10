package ssaconfig

// ScanNodeConfig defines how a scan task selects its execution node.
// mode: "auto" (default) or "manual".
type ScanNodeConfig struct {
	Mode   string `json:"mode,omitempty"`
	NodeID string `json:"node_id,omitempty"`
}

// ScanScheduleConfig defines schedule preferences for project scans.
// interval_type: 1=day, 2=hour, 3=minute
// interval_time: repeat interval count for the chosen type
// sched_type: see server scheduler (usually 3 for periodic)
type ScanScheduleConfig struct {
	Enabled      bool   `json:"enabled,omitempty"`
	Time         string `json:"time,omitempty"` // "HH:mm"
	IntervalType int    `json:"interval_type,omitempty"`
	IntervalTime int    `json:"interval_time,omitempty"`
	SchedType    int    `json:"sched_type,omitempty"`
}

// --- ScanNode getters ---

func (c *Config) GetScanNodeMode() string {
	if c == nil || c.ScanNode == nil {
		return ""
	}
	return c.ScanNode.Mode
}

func (c *Config) GetScanNodeID() string {
	if c == nil || c.ScanNode == nil {
		return ""
	}
	return c.ScanNode.NodeID
}

// --- ScanSchedule getters ---

func (c *Config) GetScanSchedule() *ScanScheduleConfig {
	if c == nil {
		return nil
	}
	return c.ScanSchedule
}
