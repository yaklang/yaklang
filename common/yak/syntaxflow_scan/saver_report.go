package syntaxflow_scan

import (
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/sfreport"
)

// bindReportSaver attaches the scan's reporter as a consumer of the risk
// decisions. The rich result still flows through notifyResult; this handler
// only applies the cover decision so a replaced finding does not stay in the
// report.
func bindReportSaver(rt *ssaapi.ScanRuntime, reporter sfreport.IReport) {
	if rt == nil || reporter == nil {
		return
	}
	switch rep := reporter.(type) {
	case *sfreport.Report:
		rep.SetKeeper(rt.KeepRisk)
		rt.ListenRisk(schema.RiskUpdateHandlerFunc(rep.ApplyRiskUpdate))
	case *sfreport.SarifReport:
		rep.SetKeeper(rt.KeepRisk)
		rt.ListenRisk(schema.RiskUpdateHandlerFunc(rep.ApplyRiskUpdate))
	}
}

// bindDBSaver attaches the saver that owns result rows, the audit graph and
// risk rows. It is only registered when the scan actually wants database rows:
// a memory scan streams its findings instead.
func bindDBSaver(rt *ssaapi.ScanRuntime, kind schema.SyntaxflowResultKind, taskID string, noRisk bool) {
	if rt == nil {
		return
	}
	rt.SetNoRiskDB(noRisk)
	saver := newDBSaver(kind, taskID, noRisk)
	rt.ListenResult(saver.ApplyResult)
	if !noRisk {
		rt.ListenRisk(saver)
	}
}

// ensureScanRuntime returns the runtime of this scan, creating it on first use.
func ensureScanRuntime(cfg *Config) *ssaapi.ScanRuntime {
	if cfg == nil {
		return nil
	}
	if cfg.scanRuntime != nil {
		return cfg.scanRuntime
	}
	rt := ssaapi.NewScanRuntime()
	cfg.scanRuntime = rt
	if cfg.IsNoSaveRisk() {
		rt.SetNoRiskDB(true)
	}
	bindReportSaver(rt, cfg.Reporter)
	return rt
}
