package sfaudit

import (
	"embed"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/utils"
)

//go:embed rules/*.sf
var ruleFS embed.FS

// The embedded .sf files are the single source of truth for the rule set:
// each file registers itself under the rule_id declared in its desc(...)
// header (which is the codeaudit finding ID callers rely on). Adding a rule
// means dropping a new file into rules/, no Go-side list to update.
var (
	registryOnce sync.Once
	registry     map[string]string // rule ID -> embedded file path
	registryErr  error
)

func buildRegistry() {
	entries, err := ruleFS.ReadDir("rules")
	if err != nil {
		registryErr = fmt.Errorf("sfaudit: read embedded rules: %w", err)
		return
	}
	reg := make(map[string]string, len(entries))
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".sf") {
			continue
		}
		path := "rules/" + ent.Name()
		content, err := ruleFS.ReadFile(path)
		if err != nil {
			registryErr = fmt.Errorf("sfaudit: read rule %s: %w", path, err)
			return
		}
		id, ok := extractRuleID(string(content))
		if !ok {
			registryErr = utils.Errorf("sfaudit: rule %s has no rule_id in its desc header", path)
			return
		}
		if prev, dup := reg[id]; dup {
			registryErr = utils.Errorf("sfaudit: duplicate rule_id %q in %s and %s", id, prev, path)
			return
		}
		reg[id] = path
	}
	if len(reg) == 0 {
		registryErr = utils.Errorf("sfaudit: no embedded rules found")
		return
	}
	registry = reg
}

// extractRuleID pulls the rule_id field out of a rule's desc header. The
// first rule_id line wins; a malformed one is treated as missing so the
// registry build fails fast instead of silently skipping the rule.
func extractRuleID(content string) (string, bool) {
	for _, line := range strings.Split(content, "\n") {
		t := strings.TrimSpace(line)
		if !strings.HasPrefix(t, "rule_id:") {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(t, "rule_id:"))
		if len(rest) < 2 || rest[0] != '"' {
			return "", false
		}
		if end := strings.Index(rest[1:], `"`); end >= 0 {
			return rest[1 : 1+end], true
		}
		return "", false
	}
	return "", false
}

func ruleRegistry() (map[string]string, error) {
	registryOnce.Do(buildRegistry)
	return registry, registryErr
}

// RuleIDs returns all registered rule IDs, sorted for determinism.
func RuleIDs() []string {
	reg, err := ruleRegistry()
	if err != nil {
		return nil
	}
	ids := make([]string, 0, len(reg))
	for id := range reg {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// HasRule reports whether a rule ID is registered.
func HasRule(id string) bool {
	reg, err := ruleRegistry()
	if err != nil {
		return false
	}
	_, ok := reg[id]
	return ok
}

// RuleContent returns the raw .sf content for a rule ID.
func RuleContent(id string) (string, error) {
	reg, err := ruleRegistry()
	if err != nil {
		return "", err
	}
	file, ok := reg[id]
	if !ok {
		return "", utils.Errorf("sfaudit: unknown rule %q", id)
	}
	content, err := ruleFS.ReadFile(file)
	if err != nil {
		return "", fmt.Errorf("sfaudit: read rule %q: %w", id, err)
	}
	return string(content), nil
}
