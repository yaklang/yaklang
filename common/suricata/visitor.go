package suricata

type RuleSyntaxVisitor struct {
	Raw    []byte
	Errors []error
	Rules  []*Rule

	// 设置环境变量规则
	Environment map[string]string
}
