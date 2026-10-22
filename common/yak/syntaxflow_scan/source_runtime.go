package syntaxflow_scan

import (
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/syntaxflow/sfpattern"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

// prepareSourceRuntimes decorates the existing rule scheduler; it does not
// implement a second rule executor. Rules retain their original channel order
// (including resume indices), and each query receives its own SFVM/root.
func (m *scanManager) prepareSourceRuntimes() (map[ssaapi.SyntaxFlowQueryInstance]*sfpattern.SourceScanRuntime, func()) {
	noop := func() {}
	if m.Config.disableSourceScanRuntime || m.IsStop() {
		return nil, noop
	}
	targets := m.Config.QueryTargets
	if len(targets) == 0 {
		for _, program := range m.Config.Programs {
			if program != nil {
				targets = append(targets, program)
			}
		}
	}
	canUse := false
	for _, target := range targets {
		switch target.(type) {
		case *ssaapi.SourceQueryTarget, *ssaapi.Program:
			canUse = true
		}
	}
	if !canUse {
		return nil, noop
	}
	start := time.Now()
	var rules []*schema.SyntaxFlowRule
	for rule := range m.ruleChan {
		rules = append(rules, rule)
	}
	// Only the finite selected rule set is buffered, not source files or values.
	m.ruleChan = make(chan *schema.SyntaxFlowRule, len(rules))
	for _, rule := range rules {
		m.ruleChan <- rule
	}
	close(m.ruleChan)

	var frames []*sfvm.SFFrame
	for _, rule := range rules {
		if m.IsStop() {
			return nil, noop
		}
		if rule == nil || !rule.IsSourceMode() {
			continue
		}
		frame, resave, err := m.Config.rulePreparationState().loader.Load(sfvm.NewSyntaxFlowVirtualMachine(), rule)
		if err != nil {
			// The regular query reports and counts this rule's compilation error.
			log.Debugf("source pre-scan: rule %s stays on legacy path: %v", rule.RuleName, err)
			continue
		}
		if resave && rule.ID > 0 {
			// Same migration gate as queryConfig.GetFrame. Load also memoizes
			// opcodes so concurrent target queries do not recompile this rule.
			sfdb.MigrateSyntaxFlow("", rule)
		}
		frames = append(frames, frame)
	}
	if len(frames) == 0 {
		return nil, noop
	}
	plan, err := sfpattern.NewSourceScanPlan(frames)
	if err != nil {
		log.Warnf("source pre-scan disabled for this scan; original matchers will run: %v", err)
		return nil, noop
	}
	runtimes := make(map[ssaapi.SyntaxFlowQueryInstance]*sfpattern.SourceScanRuntime)
	for _, target := range targets {
		if _, exists := runtimes[target]; exists {
			continue
		}
		var store sfpattern.SourceStore
		switch target := target.(type) {
		case *ssaapi.SourceQueryTarget:
			if target != nil {
				store = target.SourceStore()
			}
		case *ssaapi.Program:
			if target != nil && target.Program != nil {
				store = ssaapi.NewProgramSourceStore(target)
			}
		default:
			// Overlay/custom targets retain their own snapshot/routing semantics.
			continue
		}
		if store != nil {
			runtimes[target] = plan.StartStore(m.ctx, store)
		}
	}
	log.Infof("source pre-scan prepared: rules=%d targets=%d elapsed=%s", len(frames), len(runtimes), time.Since(start))
	return runtimes, func() {
		for target, runtime := range runtimes {
			runtime.Close()
			stats := runtime.Stats()
			log.Infof("source pre-scan %s: patterns=%d searches=%d workers=%d files=%d bytes=%d raw_hits=%d cached_searches=%d fallback_files=%d", target.GetProgramName(), stats.Patterns, stats.Searches, stats.Workers, stats.Files, stats.Bytes, stats.Hits, stats.Lookups, stats.FallbackFiles)
		}
		if err := plan.Close(); err != nil {
			log.Warnf("close source pre-scan: %v", err)
		}
	}
}
