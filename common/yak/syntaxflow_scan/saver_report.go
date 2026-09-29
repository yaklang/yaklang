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
func bindDBSaver(rt *ssaapi.ScanRuntime, kind schema.SyntaxflowResultKind, taskID string, noRisk bool) *dbSaver {
	if rt == nil {
		return nil
	}
	var saver *dbSaver
	rt.BindOnce("db", func() {
		rt.SetNoRiskDB(noRisk)
		saver = newDBSaver(kind, taskID, noRisk)
		rt.ListenResult(saver.ApplyResult)
		if !noRisk {
			rt.ListenRisk(saver)
		}
	})
	// A nil saver means another stage of this scan already owns the database
	// consumer; that stage flushes it.
	return saver
}

// ensureScanRuntime returns the runtime of this scan, creating it on first use.
func ensureScanRuntime(cfg *Config) *ssaapi.ScanRuntime {
	if cfg == nil {
		return nil
	}
	if cfg.scanRuntime == nil {
		cfg.scanRuntime = ssaapi.NewScanRuntime()
	}
	rt := cfg.scanRuntime
	if cfg.IsNoSaveRisk() {
		rt.SetNoRiskDB(true)
	}
	// The reporter consumes every stage of one scan, so it attaches exactly
	// once even when the caller supplied the runtime.
	rt.BindOnce("report", func() {
		bindReportSaver(rt, cfg.Reporter)
	})
	return rt
}
