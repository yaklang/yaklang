//go:build !irify_exclude

package sfdb

func readRuleVersions() ([]byte, error) {
	return ruleVersionFS.ReadFile("rule_versions.json")
}
