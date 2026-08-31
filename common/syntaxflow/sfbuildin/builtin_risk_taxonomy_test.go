//go:build !irify_exclude

package sfbuildin

import (
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/syntaxflow/sfrisk"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

// TestBuiltinRiskTypeTaxonomy validates compiled metadata, including per-alert
// overrides and last-assignment semantics. Run both default and -tags gzip_embed
// variants: their embedded resource sets are intentionally selected separately.
func TestBuiltinRiskTypeTaxonomy(t *testing.T) {
	InitEmbedFSWithNotify(nil)
	var violations []string
	rawTypes := make(map[string]bool)
	canonicalTypes := make(map[string]bool)
	rules := 0
	check := func(path, field, raw string) {
		if strings.TrimSpace(raw) == "" {
			return
		} // Missing metadata is not inferred by this display-only migration.
		rawTypes[raw] = true
		definition, ok := sfrisk.Lookup(raw)
		if !ok {
			violations = append(violations, fmt.Sprintf("%s %s: unregistered risk type %q", path, field, raw))
			return
		}
		canonicalTypes[definition.CanonicalName] = true
	}
	err := filesys.Recursive(".", filesys.WithFileSystem(ruleFSWithHash), filesys.WithFileStat(func(path string, info fs.FileInfo) error {
		if !strings.HasSuffix(info.Name(), ".sf") {
			return nil
		}
		raw, err := ruleFSWithHash.ReadFile(path)
		if err != nil {
			return err
		}
		rule, err := sfdb.CheckSyntaxFlowRuleContent(string(raw))
		if err != nil {
			violations = append(violations, fmt.Sprintf("%s: compile: %v", path, err))
			return nil
		}
		rules++
		check(path, "rule", rule.RiskType)
		for name, alert := range rule.AlertDesc {
			if alert != nil {
				check(path, "alert["+name+"]", alert.RiskType)
			}
		}
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if rules == 0 {
		t.Fatal("no built-in rules were inspected")
	}
	sort.Strings(violations)
	if len(violations) > 0 {
		t.Fatalf("risk type governance violations:\n%s", strings.Join(violations, "\n"))
	}
	t.Logf("compiled %d rules; %d raw risk types resolve to %d canonical display types", rules, len(rawTypes), len(canonicalTypes))
}
