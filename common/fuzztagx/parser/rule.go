package parser

var (
	Start            Rule = "start"
	TagLeft          Rule = "{{"
	TagRight         Rule = "}}"
	MethodParamLeft  Rule = "("
	MethodParamRight Rule = ")"
	IdentifierStart  Rule = "[A-Z_]"
	IdentifierOther  Rule = "[a-zA-Z_]"
	Param            Rule = ".* | Method | FuzzTag"
	Empty            Rule = "[ |\\n]+"
)

type TagRule struct {
	Name  string
	Rules []Rule
}

type Rule interface {
}

type RuleWithFlag struct {
	Rule
	Flag string
}

func NewFlagRule(rule Rule, flag string) Rule {
	return &RuleWithFlag{
		Rule: rule,
		Flag: flag,
	}
}

var RootRule *TagRule

func init() {
	identifier := &TagRule{
		Name: "identifier",
	}
	method := &TagRule{
		Name: "method",
	}
	methodWithEmpty := &TagRule{
		Name: "methodWithEmpty",
	}
	fuzzTag := &TagRule{
		Name: "fuzzTag",
	}

	identifier.Rules = []Rule{IdentifierStart, IdentifierOther}
	method.Rules = []Rule{identifier, MethodParamLeft, Param, MethodParamRight}
	methodWithEmpty.Rules = []Rule{NewFlagRule(Empty, "*"), method, NewFlagRule(Empty, "*")}
	fuzzTag.Rules = []Rule{TagLeft, NewFlagRule(methodWithEmpty, "+"), TagRight}
	RootRule = fuzzTag
}
