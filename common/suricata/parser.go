package suricata

import (
	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	rule "github.com/yaklang/yaklang/common/suricata/parser"
)

func Parse(data string) ([]*Rule, error) {
	lexer := rule.NewSuricataRuleLexer(antlr.NewInputStream(data))
	tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	parser := rule.NewSuricataRuleParser(tokenStream)
	parser.RemoveErrorListeners()
	//for _, t := range lexer.GetAllTokens() {
	//	fmt.Println(t)
	//}
	v := &RuleSyntaxVisitor{Raw: []byte(data)}
	v.VisitRules(parser.Rules().(*rule.RulesContext))
	if len(v.Rules) > 0 {
		return v.Rules, nil
	} else {
		return nil, v.MergeErrors()
	}
}
