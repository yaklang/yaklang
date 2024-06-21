package parsers

import (
	"fmt"
	"github.com/yaklang/yaklang/common/fp/fingerprint/rule"
	"github.com/yaklang/yaklang/common/utils"
	"strconv"
)

var buildinTokens = []string{"(", ")", "||", "&&", "=", "\"", "\\"}

func ParseExpRule(rules [][2]string) ([]*rule.FingerPrintRule, error) {
	res := []*rule.FingerPrintRule{}
	for _, ruleInfo := range rules {
		exp := ruleInfo[0]
		info := ruleInfo[1]
		r, err := compileExp(exp)
		if err != nil {
			return nil, err
		}
		r.MatchParam.Info = &rule.FingerprintInfo{
			Info: info,
		}
		res = append(res, r)
	}
	return res, nil
}

func compileExp(exp string) (*rule.FingerPrintRule, error) {
	res := utils.IndexAllSubstrings(exp, buildinTokens...)
	tokens := []string{}
	pre := 0
	cut := func(i int) {
		defer func() {
			pre = i
		}()
		if len(exp[pre:i]) == 0 {
			return
		}
		tokens = append(tokens, exp[pre:i])
	}
	for _, re := range res {
		cut(re[1])
		matchedStr := buildinTokens[re[0]]
		cut(re[1] + len(matchedStr))
	}
	cut(len(exp))
	index := 0
	currentStatus := "exp"
	strBuf := ""
	escape := false
	var currentRule *rule.FingerPrintRule
	tmpItems := []any{}
	for {
		if index >= len(tokens) {
			break
		}
		token := tokens[index]
		switch currentStatus {
		case "exp":
			switch token {
			case "(", ")", "||", "&&":
				tmpItems = append(tmpItems, token)
			default:
				data := token
				currentRule = &rule.FingerPrintRule{}
				tmpItems = append(tmpItems, currentRule)
				r := currentRule
				r.Method = "exp"
				r.MatchParam = &rule.MatchMethodParam{}
				r.MatchParam.Params = append(r.MatchParam.Params, data)
				currentStatus = "op"
				//if _, ok := vars[token]; ok {
				//	data := vars[token]
				//	currentRule = &rule.FingerPrintRule{}
				//	tmpItems = append(tmpItems, currentRule)
				//	r := currentRule
				//	r.Method = "exp"
				//	r.MatchParam = &rule.MatchMethodParam{}
				//	r.MatchParam.Params = append(r.MatchParam.Params, data)
				//	currentStatus = "op"
				//} else {
				//	return nil, fmt.Errorf("unsupported var %s", token)
				//}
			}
		case "op":
			if !utils.StringArrayContains([]string{"="}, token) {
				return nil, fmt.Errorf("unsupported op %s", token)
			}
			currentRule.MatchParam.Op = token
			currentStatus = "value"
		case "value":
			if token == "\"" { // string value
				currentStatus = "stringValue"
			} else { // number or bool value
				if token == "true" || token == "false" {
					currentRule.MatchParam.Params = append(currentRule.MatchParam.Params, token == "true")
				} else {
					v, err := strconv.Atoi(token)
					if err != nil {
						return nil, fmt.Errorf("invalid value: %s", token)
					}
					currentRule.MatchParam.Params = append(currentRule.MatchParam.Params, v)
				}
				currentStatus = "exp"
			}
		case "stringValue":
			if escape {
				strBuf += token
				escape = false
				continue
			} else {
				switch token {
				case "\"":
					currentRule.MatchParam.Params = append(currentRule.MatchParam.Params, strBuf)
					strBuf = ""
				case "\\":
					escape = true
				default:
					strBuf += token
				}
			}
		default:
			return nil, fmt.Errorf("bug: unsupported status: %s", token)
		}
		index++
	}
	resStack := utils.NewStack[any]()
	opStack := utils.NewStack[string]()
	for i := 0; i < len(tmpItems); i++ {
		item := tmpItems[i]
		switch v := item.(type) {
		case *rule.FingerPrintRule:
			resStack.Push(v)
		case string:
			switch v {
			case "(":
				opStack.Push(v)
			case ")":
				op := opStack.Pop()
				for op != "(" {
					resStack.Push(op)
					op = opStack.Pop()
				}
			default:
				for opStack.Peek() == "&&" {
					resStack.Push(opStack.Pop())
				}
				opStack.Push(v)
			}
		}
	}
	for !opStack.IsEmpty() {
		resStack.Push(opStack.Pop())
	}
	reverseResStack := utils.NewStack[any]()
	for !resStack.IsEmpty() {
		reverseResStack.Push(resStack.Pop())
	}
	resStack = reverseResStack
	newComplex := func(op string, rs []*rule.FingerPrintRule) *rule.FingerPrintRule {
		return &rule.FingerPrintRule{
			Method: "complex",
			MatchParam: &rule.MatchMethodParam{
				Condition: op,
				SubRules:  rs,
			},
		}
	}
	mergeStack := utils.NewStack[*rule.FingerPrintRule]()
	for !resStack.IsEmpty() {
		switch v := resStack.Pop().(type) {
		case *rule.FingerPrintRule:
			mergeStack.Push(v)
		case string:
			if v == "||" {
				mergeStack.Push(newComplex("or", mergeStack.PopN(2)))
			} else {
				mergeStack.Push(newComplex("and", mergeStack.PopN(2)))
			}
		}
	}
	return mergeStack.Pop(), nil
}
