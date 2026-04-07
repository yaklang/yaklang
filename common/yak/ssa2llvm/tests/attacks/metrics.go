package attacks

import "strings"

// Metric defines a named evaluation measurement.
type Metric struct {
	Name        string
	Description string
	// Value is the metric result: 0.0 = worst (attack succeeds), 1.0 = best (attack blocked).
	Value float64
}

// Report aggregates evaluation results for a single compilation configuration.
type Report struct {
	// Label identifies the configuration (e.g. "baseline", "callret", "addsub+xor").
	Label   string
	Metrics []Metric
}

// String returns a human-readable summary.
func (r *Report) String() string {
	var b strings.Builder
	b.WriteString("=== Evaluation Report: " + r.Label + " ===\n")
	for _, m := range r.Metrics {
		b.WriteString("  " + m.Name + ": ")
		b.WriteString(formatScore(m.Value))
		b.WriteString("  (" + m.Description + ")\n")
	}
	return b.String()
}

func formatScore(v float64) string {
	if v >= 1.0 {
		return "1.00 [BLOCKED]"
	}
	if v <= 0.0 {
		return "0.00 [EXPOSED]"
	}
	return strings.TrimRight(strings.TrimRight(
		strings.Replace(
			strings.Replace(
				formatFloat(v), "0.", "0.", 1,
			), "1.", "1.", 1,
		), "0"), ".") + " [PARTIAL]"
}

func formatFloat(v float64) string {
	s := ""
	iv := int(v * 100)
	s = string(rune('0'+iv/100)) + "." + string(rune('0'+iv/10%10)) + string(rune('0'+iv%10))
	return s
}

// BuiltinTasks returns the fixed set of attack tasks used for regression.
func BuiltinTasks() []AttackTask {
	return []AttackTask{
		{
			Name:        "recover-function-names",
			Description: "Adversary attempts to recover original Yak function names from IR",
			ThreatLevel: ThreatStatic,
			Check:       checkFunctionNameRecovery,
		},
		{
			Name:        "recover-call-graph",
			Description: "Adversary attempts to reconstruct call edges between Yak functions",
			ThreatLevel: ThreatStatic,
			Check:       checkCallGraphRecovery,
		},
		{
			Name:        "identify-dispatcher",
			Description: "Adversary attempts to identify obfuscation dispatcher patterns",
			ThreatLevel: ThreatStatic,
			Check:       checkDispatcherRecovery,
		},
		{
			Name:        "structure-complexity",
			Description: "Measures structural complexity increase (basic block count ratio)",
			ThreatLevel: ThreatStatic,
			Check:       checkStructuralComplexity,
		},
	}
}

// checkFunctionNameRecovery scores how many original user function names
// remain visible in the IR. Score 1.0 = none visible, 0.0 = all visible.
func checkFunctionNameRecovery(result *AnalysisResult) float64 {
	if result == nil || len(result.FunctionNames) == 0 {
		return 1.0
	}

	// Known internal/runtime names that don't count as "recovered".
	skip := map[string]bool{
		"main":                    true,
		"yak_internal_atmain":     true,
		"yak_internal_main":      true,
		"yak_runtime_gc":          true,
		"yak_runtime_invoke":      true,
		"yak_internal_print_int":  true,
	}

	userFuncs := 0
	for _, name := range result.FunctionNames {
		if skip[name] || strings.HasPrefix(name, "yak_runtime_") || strings.HasPrefix(name, "yak_internal_") || strings.HasPrefix(name, "llvm.") {
			continue
		}
		userFuncs++
	}

	if userFuncs == 0 {
		return 1.0
	}
	// More visible user functions = lower score.
	// This is a baseline heuristic; real evaluation would be more nuanced.
	return 0.0
}

// checkCallGraphRecovery scores how many direct call edges between Yak
// functions are visible. Score 1.0 = no edges, 0.0 = all edges.
func checkCallGraphRecovery(result *AnalysisResult) float64 {
	if result == nil || len(result.CallEdges) == 0 {
		return 1.0
	}

	yakCallEdges := 0
	for _, edge := range result.CallEdges {
		callee := edge[1]
		if strings.HasPrefix(callee, "yak_runtime_") || strings.HasPrefix(callee, "yak_internal_") ||
			strings.HasPrefix(callee, "llvm.") || strings.HasPrefix(callee, "__yak_obf_") {
			continue
		}
		yakCallEdges++
	}

	if yakCallEdges == 0 {
		return 1.0
	}
	return 0.0
}

// checkDispatcherRecovery checks for presence of obvious dispatcher patterns
// (compare chains with constants). Simple heuristic for regression.
func checkDispatcherRecovery(result *AnalysisResult) float64 {
	if result == nil || result.RawIR == "" {
		return 1.0
	}

	// Count switch instructions—these are obvious dispatchers.
	switchCount := strings.Count(result.RawIR, "switch i")
	if switchCount > 0 {
		return 0.0
	}

	return 1.0
}

// checkStructuralComplexity measures basic block density. More blocks
// relative to functions suggests higher structural complexity. This is not
// a protection score per se but a complexity indicator normalized to [0,1].
func checkStructuralComplexity(result *AnalysisResult) float64 {
	if result == nil || len(result.FunctionNames) == 0 {
		return 0.0
	}

	ratio := float64(result.DistinctBlocks) / float64(len(result.FunctionNames))
	// Normalize: ≤2 blocks/func is trivial (0.0), ≥10 is complex (1.0).
	if ratio <= 2.0 {
		return 0.0
	}
	if ratio >= 10.0 {
		return 1.0
	}
	return (ratio - 2.0) / 8.0
}

// Evaluate runs all builtin attack tasks against an AnalysisResult and
// returns a Report.
func Evaluate(label string, result *AnalysisResult) *Report {
	tasks := BuiltinTasks()
	report := &Report{Label: label}
	for _, task := range tasks {
		score := task.Check(result)
		report.Metrics = append(report.Metrics, Metric{
			Name:        task.Name,
			Description: task.Description,
			Value:       score,
		})
	}
	return report
}
