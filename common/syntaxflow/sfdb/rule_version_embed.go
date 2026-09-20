//go:build !irify_exclude

package sfdb

//go:generate go run ../../utils/embedfs/generate -package sfdb -var ruleVersionFS -output rule_version_resources_embed.go -build-tag !irify_exclude rule_versions.json

func readRuleVersions() ([]byte, error) {
	return ruleVersionFS.ReadFile("rule_versions.json")
}
