package differential

import (
	"fmt"
	"os"
	"sync"

	"github.com/yaklang/yaklang/common/suricata/engine"
	"github.com/yaklang/yaklang/common/suricata/rule"
	"github.com/yaklang/yaklang/common/suricata/state"
)

type Diagnostic struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
	GID     int    `json:"gid,omitempty"`
	SID     int    `json:"sid,omitempty"`
}

type LocalResult struct {
	Result
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
	Stats       engine.Stats `json:"stats"`
}

func RunYaklang(rulesPath, pcapPath string, envs ...string) LocalResult {
	result := LocalResult{Result: Result{Engine: EngineYaklang}}
	raw, err := os.ReadFile(rulesPath)
	if err != nil {
		result.Error = fmt.Sprintf("read rules: %v", err)
		return result
	}
	rules, err := rule.ParseStrictRule(string(raw), envs...)
	if err != nil {
		result.Error = fmt.Sprintf("parse rules: %v", err)
		return result
	}
	result.RuleStats = &RuleStats{Loaded: len(rules)}

	var mu sync.Mutex
	var alerts []Alert
	var diagnostics []Diagnostic
	stateful, err := engine.NewStatefulEngine(rules, engine.Config{
		State: state.DefaultConfig(),
		OnAlert: func(alert engine.Alert) {
			if alert.Rule == nil {
				return
			}
			mu.Lock()
			alerts = append(alerts, Alert{
				GID: alert.Rule.Gid,
				SID: alert.Rule.Sid,
				Rev: alert.Rule.Rev,
				Msg: alert.Rule.Message,
			})
			mu.Unlock()
		},
		OnDiagnostic: func(diagnostic engine.Diagnostic) {
			item := Diagnostic{Kind: diagnostic.Kind, Message: diagnostic.Message}
			if diagnostic.Rule != nil {
				item.GID = diagnostic.Rule.Gid
				item.SID = diagnostic.Rule.Sid
			}
			mu.Lock()
			diagnostics = append(diagnostics, item)
			mu.Unlock()
		},
	})
	if err != nil {
		result.Error = fmt.Sprintf("compile rules: %v", err)
		return result
	}
	defer stateful.Close()

	if err := stateful.OpenPcapFile(pcapPath); err != nil {
		result.ExitCode = 1
		result.Error = fmt.Sprintf("replay pcap: %v", err)
		return result
	}
	mu.Lock()
	result.Alerts = normalizeAlerts(alerts)
	result.Diagnostics = append([]Diagnostic(nil), diagnostics...)
	mu.Unlock()
	result.Stats = stateful.Stats()
	return result
}
