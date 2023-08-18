package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func GenerateGolangCode(name string, save string) error {
	content := "package parser\n\n"
	byts, err := os.ReadFile(name)
	if err != nil {
		return err
	}
	ruleScanner := bufio.NewScanner(strings.NewReader(string(byts)))
	ruleScanner.Split(bufio.ScanLines)
	tagRuleMap := make(map[string][]string)
	//rootRule := newTagRule("root")
	_ = tagRuleMap
	NameMappingTmp := `var (
%v
)` + "\n\n"
	NameMappingElementTmp := "\t%s Rule = %s"
	eles := []string{}
	tagRuleTmp := `
type TagRule struct {
	Name        string
	Rules       []Rule
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

func init(){
%s

%s
}
`
	tagRuleEleTmp := `	%s := &TagRule{
		Name: %s,
	}`
	tagRuleEles := []string{}
	tagRuleEleAdds := []string{}
	for ruleScanner.Scan() {
		line := ruleScanner.Text()
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		if line[0] > 'a' && line[0] < 'z' { // 语法
			splitS := strings.Split(line, " = ")
			if len(splitS) != 2 {
				panic(fmt.Errorf("unexpect line: `%s`", line))
			}
			subRule := splitS[1]
			tagRuleEles = append(tagRuleEles, fmt.Sprintf(tagRuleEleTmp, splitS[0], strconv.Quote(splitS[0])))
			subRules := strings.Split(subRule, " ")
			for i := 0; i < len(subRules); i++ {
				split := strings.Split(subRules[i], "/")
				if len(split) == 2 {
					subRules[i] = fmt.Sprintf("NewFlagRule(%s,%s)", split[0], strconv.Quote(split[1]))
				}
			}
			tagRuleEleAdds = append(tagRuleEleAdds, fmt.Sprintf("\t%s.Rules = []Rule{%s}", splitS[0], strings.Join(subRules, ",")))
		} else { // 词法
			splitS := strings.Split(line, " = ")
			if len(splitS) != 2 {
				panic(fmt.Errorf("unexpect line: `%s`", line))
			}
			eles = append(eles, fmt.Sprintf(NameMappingElementTmp, splitS[0], strconv.Quote(splitS[1])))
		}
	}

	content += fmt.Sprintf(NameMappingTmp, strings.Join(eles, "\n"))
	content += fmt.Sprintf(tagRuleTmp+"\n\n", strings.Join(tagRuleEles, "\n"), strings.Join(append(tagRuleEleAdds, "\tRootRule = fuzzTag"), "\n"))
	return os.WriteFile(save, []byte(content), 0666)
}
