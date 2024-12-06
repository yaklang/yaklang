package rule

import "github.com/yaklang/yaklang/common/suricata/config"

type RuleSyntaxVisitor struct {
	Raw        []byte
	CompileRaw string
	Errors     []error
	Rules      []*Rule

	// 设置环境变量规则
	Config *config.Config
}
