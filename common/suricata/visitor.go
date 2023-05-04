package suricata

type RuleSyntaxVisitor struct {
	Raw    []byte
	Errors []error
	Rules  []*Rule
}
