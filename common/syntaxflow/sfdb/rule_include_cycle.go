package sfdb

import (
	"strings"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
)

// CheckIncludeCycleForWrite rejects a rule that would make the stored rule
// set's <include(...)> graph cyclic. Runtime include no longer detects cycles
// — a cycle hangs the loading rule at scan time — so the acyclicity
// invariant is enforced where rules enter the database: the validated import
// path (RegisterValid hook), the gRPC create/update handlers, and online-rule
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
	return sfvm.CheckRuleIncludeCycles(libraries)
}

// addIncludeCycleNode keys rule into the include graph under every name
// runtime <include(...)> resolution can reach it by (GetLibrary matches
// title or included_name), mirroring task-local snapshot keying.
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
