package loop_ssa_api_discovery

import "strings"

const (
	Phase4ModeDeepMining = "deep_mining"
	Phase4ModeBatchScan  = "batch_scan"
)

// NormalizePhase4Mode returns deep_mining (default) or batch_scan.
func NormalizePhase4Mode(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	switch s {
	case Phase4ModeBatchScan, "batch", "greybox", "灰盒批量", "批量灰盒", "congin":
		return Phase4ModeBatchScan
	case Phase4ModeDeepMining, "deep", "深度挖掘", "深度", "":
		return Phase4ModeDeepMining
	default:
		if strings.Contains(s, "batch") || strings.Contains(s, "批量") {
			return Phase4ModeBatchScan
		}
		return Phase4ModeDeepMining
	}
}

func (rt *Runtime) Phase4Mode() string {
	if rt == nil || strings.TrimSpace(rt.Phase4ModeRaw) == "" {
		return Phase4ModeDeepMining
	}
	return NormalizePhase4Mode(rt.Phase4ModeRaw)
}

func (pl *PipelineState) Phase4Mode() string {
	if pl == nil {
		return Phase4ModeDeepMining
	}
	pl.mu.RLock()
	defer pl.mu.RUnlock()
	return NormalizePhase4Mode(pl.Phase4ModeRaw)
}
