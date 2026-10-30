package sfdb

import (
	"sort"
	"strings"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
)

// CheckIncludeCycleForWrite rejects a rule that would make the stored rule
// set's <include(...)> graph cyclic. Runtime include does not detect cycles —
// a cycle hangs the loading rule at scan time — so the acyclicity invariant
// is enforced only where rules enter the database: the validated import path
// (RegisterValid hook), the gRPC create/update handlers, and online-rule
// sync. Bulk paths (built-in rule init, ImportDatabase restore) bypass this
// gate, same as they bypass content validation.
//
// Every stored rule participates, keyed how runtime resolution looks
// libraries up (GetLibrary: title or included_name match) — a cycle node
// does not have to be an allow-included library. The candidate overlays any
// same-named stored rule, matching update semantics.
func CheckIncludeCycleForWrite(rule *schema.SyntaxFlowRule) error {
	return CheckIncludeCycleForWriteDB(consts.GetGormProfileDatabase(), rule)
}

// CheckIncludeCycleForWriteDB is CheckIncludeCycleForWrite against an
// explicit database; tests run it on an isolated temp database.
func CheckIncludeCycleForWriteDB(db *gorm.DB, rule *schema.SyntaxFlowRule) error {
	if rule == nil || db == nil {
		return nil
	}
	var stored []*schema.SyntaxFlowRule
	if err := db.Model(&schema.SyntaxFlowRule{}).Find(&stored).Error; err != nil {
		return utils.Wrap(err, "load syntaxflow rules for include-cycle check")
	}
	libraries := make(map[string]*schema.SyntaxFlowRule, len(stored)+1)
	for _, r := range stored {
		addIncludeCycleNode(libraries, r)
	}
	addIncludeCycleNode(libraries, rule)
	return checkRuleIncludeCycles(libraries)
}

// addIncludeCycleNode keys rule into the include graph under every name
// runtime <include(...)> resolution can reach it by (GetLibrary matches
// title or included_name), mirroring runtime library lookup.
func addIncludeCycleNode(libraries map[string]*schema.SyntaxFlowRule, rule *schema.SyntaxFlowRule) {
	if rule == nil {
		return
	}
	for _, name := range []string{rule.Title, rule.IncludedName} {
		if name = strings.TrimSpace(name); name != "" {
			libraries[name] = rule
		}
	}
}

// includeNativeCallName is the native-call name of <include(...)>, registered
// by ssaapi as NativeCall_Include. The name is part of the SyntaxFlow language
// surface, so it is referenced textually here.
const includeNativeCallName = "include"

// includeNamedParamKeys mirrors nativeCallInclude's named-param precedence:
// the first of these keys present on the call wins, regardless of value.
var includeNamedParamKeys = [3]string{"name", "rule", "rulename"}

// extractIncludeTargets returns the static include targets of one compiled
// rule, in source order. Every native-call param value is reduced to source
// text at compile time (quoted string, heredoc, or bare identifier — see
// VisitNativeCallActualParams), so the extracted names are exactly what
// runtime resolution looks up; there is no computed-target form to miss.
// Precedence mirrors nativeCallInclude: named name/rule/rulename param first
// (last write wins per key, first present key wins overall), then the first
// positional param. Whitespace-only targets are dropped.
//
// Sub-rules embedded as config values (filter statements / heredocs, marked
// SyntaxFlowRule at compile time) are walked recursively: a fragment that
// fails to compile standalone contributes no targets.
func extractIncludeTargets(frame *sfvm.SFFrame) []string {
	if frame == nil {
		return nil
	}
	var targets []string
	collectIncludeTargets(frame.Codes, &targets)
	return targets
}

// collectIncludeTargets appends the include targets of codes to targets,
// recursing into embedded sub-rule config values. Each nesting level compiles
// a strict substring of its parent source, so the recursion terminates.
func collectIncludeTargets(codes []*sfvm.SFI, targets *[]string) {
	for _, sfi := range codes {
		if sfi == nil {
			continue
		}
		if sfi.OpCode == sfvm.OpNativeCall && sfi.UnaryStr == includeNativeCallName {
			if target := includeCallTarget(sfi.SyntaxFlowConfig); target != "" {
				*targets = append(*targets, target)
			}
		}
		for _, item := range sfi.SyntaxFlowConfig {
			if item == nil || !item.SyntaxFlowRule || strings.TrimSpace(item.Value) == "" {
				continue
			}
			if sub, err := sfvm.CompileRule(item.Value); err == nil {
				collectIncludeTargets(sub.Codes, targets)
			}
		}
	}
}

// includeCallTarget resolves an include native call's target with
// nativeCallInclude's param precedence: named name/rule/rulename first (last
// write wins per key, first present key wins overall), then the first
// positional param. Returns "" when no usable target exists.
func includeCallTarget(config []*sfvm.RecursiveConfigItem) string {
	target := ""
	for _, key := range includeNamedParamKeys {
		if value, ok := lastStringConfigValue(config, key); ok {
			target = value
			break
		}
	}
	if target == "" {
		if value, ok := lastStringConfigValue(config, "0"); ok {
			target = value
		}
	}
	return strings.TrimSpace(target)
}

// lastStringConfigValue returns the value of the last config item carrying
// key (NativeCallActualParams.GetString semantics for duplicates).
func lastStringConfigValue(config []*sfvm.RecursiveConfigItem, key string) (string, bool) {
	var value string
	var exists bool
	for _, item := range config {
		if item == nil || item.Key != key {
			continue
		}
		value, exists = item.Value, true
	}
	return value, exists
}

// checkRuleIncludeCycles rejects a rule set whose <include(...)> edges form
// a cycle, reporting the cycle path (e.g. "a -> b -> a"). It runs only
// inside CheckIncludeCycleForWriteDB, at rule-write time. There is no
// runtime cycle detector — a cyclic set that slips past the write gate
// hangs the loading rule at scan time — so the check errs on the side of
// coverage.
//
// Scope, deliberately conservative (no false positives):
//   - only edges whose target is another known rule are followed; a target
//     outside the set has no known content and is skipped;
//   - a rule that fails to compile contributes no edges and is skipped: it
//     cannot execute includes at scan time either, and failing the whole
//     check would let one legacy broken row block every later write;
//   - rules carrying stored opcodes (sfdb rows) load through the opcode fast
//     path; content is recompiled only when the opcodes are absent or stale
//     (version-gated, see ToOpCodes).
func checkRuleIncludeCycles(libraries map[string]*schema.SyntaxFlowRule) error {
	if len(libraries) == 0 {
		return nil
	}
	names := make([]string, 0, len(libraries))
	for name := range libraries {
		names = append(names, name)
	}
	sort.Strings(names)

	edges := make(map[string][]string, len(libraries))
	for _, name := range names {
		rule := libraries[name]
		if rule == nil {
			continue
		}
		frame, _, err := sfvm.NewSyntaxFlowVirtualMachine().Load(rule)
		if err != nil {
			continue
		}
		for _, target := range extractIncludeTargets(frame) {
			if _, ok := libraries[target]; ok {
				edges[name] = append(edges[name], target)
			}
		}
	}

	const (
		white = iota
		gray
		black
	)
	color := make(map[string]int, len(libraries))
	var path []string
	var visit func(name string) []string
	visit = func(name string) []string {
		color[name] = gray
		path = append(path, name)
		for _, next := range edges[name] {
			switch color[next] {
			case gray:
				from := 0
				for i, n := range path {
					if n == next {
						from = i
						break
					}
				}
				cycle := append([]string{}, path[from:]...)
				return append(cycle, next)
			case white:
				if cycle := visit(next); cycle != nil {
					return cycle
				}
			}
		}
		path = path[:len(path)-1]
		color[name] = black
		return nil
	}
	for _, name := range names {
		if color[name] != white {
			continue
		}
		if cycle := visit(name); cycle != nil {
			return utils.Errorf(
				"syntaxflow library include cycle detected: %s",
				strings.Join(cycle, " -> "),
			)
		}
	}
	return nil
}
