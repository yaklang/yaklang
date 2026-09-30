package syntaxflow_scan

import (
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/sfreport"
)

// registerReport attaches the reporter as a consumer of risk updates.
//
// The scan already filtered the finding. The report only creates or replaces
// the entry for that one RiskUpdateItem. A reporter that does not implement
// the handler is left alone; one-shot export still uses AddSyntaxFlowResult.
func registerReport(rt *ssaapi.ScanRuntime, reporter sfreport.IReport) {
	if rt == nil || reporter == nil {
		return
	}
	handler, ok := reporter.(schema.RiskUpdateHandler)
	if !ok || handler == nil {
		return
	}
	rt.ListenRisk(handler)
}

// attachDBSaver attaches the consumer that writes risk rows. The query writes
// its own audit result when the result kind is database; this saver only
// creates or rewrites the risk row from each RiskUpdateItem.
func attachDBSaver(rt *ssaapi.ScanRuntime, kind schema.SyntaxflowResultKind, taskID string, noRisk bool) *dbSaver {
	if rt == nil || noRisk {
		return nil
	}
	saver := newDBSaver(kind, taskID, false)
	rt.ListenRisk(saver)
	return saver
}

// ensureScanRuntime returns the runtime of this scan, creating it on first use.
// Registration of report and database consumers belongs to the caller that
// created the runtime, so a nested stage sharing it does not register again.
func ensureScanRuntime(cfg *Config) *ssaapi.ScanRuntime {
	if cfg == nil {
		return nil
	}
	if cfg.scanRuntime == nil {
		cfg.scanRuntime = ssaapi.NewScanRuntime()
	}
	if cfg.IsNoSaveRisk() {
		cfg.scanRuntime.SetNoRiskDB(true)
	}
	return cfg.scanRuntime
}
