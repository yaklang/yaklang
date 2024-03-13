package rule

type RuleSyntaxVisitor struct {
	Raw        []byte
	CompileRaw string
	Errors     []error
	Rules      []*Rule

	// 设置环境变量规则
	Environment map[string]string
}
