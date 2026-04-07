// Package attacks provides an evaluation framework for measuring obfuscation
// effectiveness against fixed attack tasks. It does not implement obfuscation
// itself; instead it defines threat models, metrics, and regression harnesses
// that all obfuscation passes (native, LLVM interop, virtualize) share.
package attacks

import (
	"fmt"
	"strings"
)

// ThreatLevel describes the capability tier of the assumed adversary.
type ThreatLevel int

const (
	// ThreatStatic: adversary has access to IR, assembly, and binary;
	// performs pattern matching and structural analysis only.
	ThreatStatic ThreatLevel = iota

	// ThreatSymbolic: adversary can run symbolic execution, taint
	// analysis, and automated deobfuscation scripts.
	ThreatSymbolic

	// ThreatDynamic: adversary can instrument, trace, and patch the
	// running binary; may write targeted deobfuscation tooling.
	ThreatDynamic
)

func (t ThreatLevel) String() string {
	switch t {
	case ThreatStatic:
		return "static"
	case ThreatSymbolic:
		return "symbolic"
	case ThreatDynamic:
		return "dynamic"
	default:
		return fmt.Sprintf("threat(%d)", int(t))
	}
}

// AttackTask defines a single evaluation scenario.
type AttackTask struct {
	// Name is a short identifier for the task (e.g. "recover-call-graph").
	Name string

	// Description explains what the adversary tries to achieve.
	Description string

	// ThreatLevel is the minimum adversary capability required.
	ThreatLevel ThreatLevel

	// Check runs the evaluation and returns a score between 0.0 (fully
	// recovered / attack succeeded) and 1.0 (attack completely blocked).
	Check func(result *AnalysisResult) float64
}

// AnalysisResult captures observable properties of a compiled binary or IR
// that attack tasks inspect.
type AnalysisResult struct {
	// RawIR is the LLVM IR text of the compiled module.
	RawIR string

	// FunctionNames lists all function symbols visible in the IR.
	FunctionNames []string

	// CallEdges lists caller→callee pairs found by simple pattern matching.
	CallEdges [][2]string

	// DistinctBlocks is the total number of basic blocks.
	DistinctBlocks int

	// BinarySize is the artifact size in bytes (0 if not measured).
	BinarySize int64

	// CompileTimeMs is the compilation wall-clock time in milliseconds.
	CompileTimeMs int64

	// RunTimeMs is the execution wall-clock time in milliseconds.
	RunTimeMs int64
}

// AnalyzeIR performs lightweight static analysis on LLVM IR text and populates
// an AnalysisResult. This is intentionally simple pattern matching—real
// adversary analysis would be far more sophisticated, but this gives us a
// consistent regression baseline.
func AnalyzeIR(ir string) *AnalysisResult {
	result := &AnalysisResult{RawIR: ir}

	for _, line := range strings.Split(ir, "\n") {
		trimmed := strings.TrimSpace(line)

		// Collect function definitions.
		if strings.HasPrefix(trimmed, "define ") {
			if name := extractFunctionName(trimmed); name != "" {
				result.FunctionNames = append(result.FunctionNames, name)
			}
		}

		// Count basic blocks (labels).
		if !strings.HasPrefix(trimmed, ";") && strings.HasSuffix(trimmed, ":") && !strings.HasPrefix(trimmed, "define") {
			result.DistinctBlocks++
		}

		// Collect call edges.
		if edge := extractCallEdge(trimmed); edge != [2]string{} {
			result.CallEdges = append(result.CallEdges, edge)
		}
	}

	return result
}

// extractFunctionName pulls "foo" from `define i64 @foo(...) {`.
func extractFunctionName(line string) string {
	atIdx := strings.Index(line, "@")
	if atIdx < 0 {
		return ""
	}
	rest := line[atIdx+1:]
	parenIdx := strings.Index(rest, "(")
	if parenIdx < 0 {
		return ""
	}
	name := rest[:parenIdx]
	// Handle quoted names like @"@main".
	name = strings.Trim(name, "\"")
	return name
}

// extractCallEdge pulls [caller, callee] from `call ... @callee(...)` lines.
// This is a rough heuristic for regression testing, not real analysis.
func extractCallEdge(line string) [2]string {
	callIdx := strings.Index(line, "call ")
	if callIdx < 0 {
		return [2]string{}
	}
	rest := line[callIdx:]
	atIdx := strings.Index(rest, "@")
	if atIdx < 0 {
		return [2]string{}
	}
	callee := rest[atIdx+1:]
	parenIdx := strings.Index(callee, "(")
	if parenIdx < 0 {
		return [2]string{}
	}
	callee = callee[:parenIdx]
	callee = strings.Trim(callee, "\"")
	if callee == "" {
		return [2]string{}
	}
	return [2]string{"<caller>", callee}
}
