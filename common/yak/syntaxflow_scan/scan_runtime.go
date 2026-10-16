package syntaxflow_scan

import "github.com/yaklang/yaklang/common/yak/ssaapi"

// Nested stages borrow the project runtime. Only the entry point registers
// its report and, when requested, its database handler.
func ensureScanRuntime(cfg *Config) *ssaapi.ScanRuntime {
	if cfg.scanRuntime == nil {
		cfg.scanRuntime = ssaapi.NewScanRuntime()
	}
	return cfg.scanRuntime
}
