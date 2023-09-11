package rule

import (
	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/log"
	rule "github.com/yaklang/yaklang/common/suricata/parser"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
)

var presetEnv = map[string]string{
	"HOME_NET": utils.GetLocalIPAddress(),
}

func Parse(data string, envs ...string) ([]*Rule, error) {
	lexer := rule.NewSuricataRuleLexer(antlr.NewInputStream(data))
	tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	parser := rule.NewSuricataRuleParser(tokenStream)
	parser.RemoveErrorListeners()
	//for _, t := range lexer.GetAllTokens() {
	//	fmt.Println(t)
	//}
	v := &RuleSyntaxVisitor{Raw: []byte(data)}
	v.Environment = make(map[string]string)
	for k, val := range presetEnv {
		v.Environment[k] = val
	}
	for _, e := range envs {
		before, after, cut := strings.Cut(e, "=")
		if !cut {
			log.Warnf("env input:[%v] cannot parse as key=value", e)
			continue
		}
		v.Environment[before] = after
	}
	v.VisitRules(parser.Rules().(*rule.RulesContext))
	if len(v.Rules) > 0 {
		return v.Rules, nil
	} else {
		return nil, v.MergeErrors()
	}
}
