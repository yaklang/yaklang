//go:build no_syntaxflow
// +build no_syntaxflow

package sfdb

import (
	"io"

	"github.com/yaklang/yaklang/common/schema"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

// Stub functions when SyntaxFlow support is excluded

func RegisterValid(handler func(*schema.SyntaxFlowRule) error) {}

func BuildFileSystem(rule *schema.SyntaxFlowRule) (fi.FileSystem, error) {
	// Return nil as stub, actual code should handle this
	return nil, nil
}

func GetLibrary(libname string) (*schema.SyntaxFlowRule, error) {
	return &schema.SyntaxFlowRule{}, nil
}

func MigrateSyntaxFlow(name string, rule *schema.SyntaxFlowRule) error {
	return nil
}

func GetRule(name string) (*schema.SyntaxFlowRule, error) {
	return nil, nil
}

func GetRulePure(name string) (*schema.SyntaxFlowRule, error) {
	return nil, nil
}

func YieldSyntaxFlowRules(db interface{}, ctx interface{}) chan *schema.SyntaxFlowRule {
	c := make(chan *schema.SyntaxFlowRule)
	close(c)
	return c
}

func DeleteSyntaxFlowRuleByRuleNameOrRuleId(name, id string) error {
	return nil
}

func CreateOrUpdateRuleWithGroup(rule *schema.SyntaxFlowRule, groups ...string) (*schema.SyntaxFlowRule, error) {
	return rule, nil
}

func QueryRuleByName(db interface{}, name string) (*schema.SyntaxFlowRule, error) {
	return nil, nil
}

func CreateRuleWithDefaultGroup(rule *schema.SyntaxFlowRule, groupNames ...string) error {
	return nil
}

func BatchAddGroupsForRulesByRuleId(db interface{}, ruleId string, groups ...string) error {
	return nil
}

func CheckSyntaxFlowRuleContent(content string) (*schema.SyntaxFlowRule, error) {
	return nil, nil
}

func ExportDatabase() io.Reader {
	return nil
}

func ImportDatabase(reader interface{}) error {
	return nil
}

func EmbedRuleVersion() interface{} {
	return nil
}

func ImportValidRule(system interface{}, ruleName, content string) error {
	return nil
}
