package parsers

import (
	"errors"
	"fmt"
	"github.com/yaklang/yaklang/common/fp/fingerprint/rule"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"sort"
	"strconv"
	"strings"
)

var buildinOps = []string{"!==", "!=", "==", "=", "~="}
var buildinTokens = []string{"(", ")", "||", "&&", "\"", "\\"}

func init() {
	buildinTokens = append(buildinTokens, buildinOps...)
}
func ParseExpRule(rules ...*schema.GeneralRule) ([]*rule.FingerPrintRule, error) {
	res := []*rule.FingerPrintRule{}
	errs := []error{}
	for _, ruleInfo := range rules {
		exp := ruleInfo.MatchExpression
		cpe := ruleInfo.CPE
		if exp == "" {
			continue
		}
		r, err := compileExp(exp)
		if err != nil {
			errs = append(errs, fmt.Errorf("parse exp %s error: %v", exp, err))
			continue
		}
		if cpe.Product == "" {
			cpe.Product = ruleInfo.RuleName
		}
		r.MatchParam.Info = cpe
		res = append(res, r)
	}
	return res, utils.JoinErrors(errs...)
}

func compatibleSyntaxCompileExp(exp string) (*rule.FingerPrintRule, error) {
	splitTokens := []string{"||", "&&"}
	res := utils.IndexAllSubstrings(exp, splitTokens...)
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
		matchedStr := splitTokens[re[0]]
		cut(re[1] + len(matchedStr))
	}
	cut(len(exp))
	tmpItems := []any{}
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		if strings.HasPrefix(token, "(") {
			tmpItems = append(tmpItems, "(")
			token = token[1:]
		}
		if strings.HasSuffix(token, ")") {
			tmpItems = append(tmpItems, ")")
			token = token[:len(token)-1]
		}
		if utils.StringArrayContains(splitTokens, token) {
			tmpItems = append(tmpItems, token)
			continue
		}
		left, rightStr, ok := strings.Cut(token, "=")
		var right any
		if !ok {
			left = "branner"
			right = token
		} else {
			if len(rightStr) > 1 && rightStr[0] == '"' && rightStr[len(rightStr)-1] == '"' {
				right = rightStr[1 : len(rightStr)-1]
			} else {
				if token == "true" {
					right = true
				} else if token == "false" {
					right = false
				} else {
					v, err := strconv.Atoi(token)
					if err != nil {
						right = rightStr
					} else {
						right = v
					}
				}
			}
		}

		left = strings.TrimSpace(left)
		currentRule := &rule.FingerPrintRule{
			Method: "exp",
			MatchParam: &rule.MatchMethodParam{
				Op:     "=",
				Params: []any{left, right},
			},
		}
		tmpItems = append(tmpItems, currentRule)
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
func compileExp(exp string) (*rule.FingerPrintRule, error) {
	res := utils.IndexAllSubstrings(exp, buildinTokens...)
	newRes := [][2]int{}
	preN := -1
	for i := 0; i < len(res); i++ {
		if res[i][1] == preN {
			newRes[len(newRes)-1] = res[i]
		} else {
			newRes = append(newRes, res[i])
			preN = res[i][1]
		}
	}
	res = newRes
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
	currentStatus := "start"
	strBuf := ""
	escape := false
	var currentRule *rule.FingerPrintRule
	tmpItems := []any{}
	for {
		if index >= len(tokens) {
			break
		}
		token := tokens[index]
		originToken := token
		token = strings.TrimSpace(token)
		if token == "" && currentStatus != "stringValue" {
			index++
			continue
		}
		switch currentStatus {
		case "start":
			switch token {
			case "(", ")":
				tmpItems = append(tmpItems, token)
			default:
				data := token
				currentRule = &rule.FingerPrintRule{MatchParam: &rule.MatchMethodParam{}}
				tmpItems = append(tmpItems, currentRule)
				r := currentRule
				r.Method = "exp"
				r.MatchParam = &rule.MatchMethodParam{}
				r.MatchParam.Params = append(r.MatchParam.Params, data)
				currentStatus = "op"
			}
		case "condition":
			switch token {
			case "(", ")":
				tmpItems = append(tmpItems, token)
			case "||", "&&":
				tmpItems = append(tmpItems, token)
				currentStatus = "start"
			default:
				return nil, fmt.Errorf("unsupported condition %s", token)
			}
		case "op":
			if !utils.StringArrayContains(buildinOps, token) {
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
						currentRule.MatchParam.Params = append(currentRule.MatchParam.Params, token)
						//return nil, fmt.Errorf("invalid value: %s", token)
					} else {
						currentRule.MatchParam.Params = append(currentRule.MatchParam.Params, v)
					}
				}
				currentStatus = "condition"
			}
		case "stringValue":
			if escape {
				strBuf += originToken
				escape = false
				index++
				continue
			} else {
				switch token {
				case "\"":
					currentRule.MatchParam.Params = append(currentRule.MatchParam.Params, strBuf)
					strBuf = ""
					currentStatus = "condition"
				case "\\":
					escape = true
				default:
					strBuf += originToken
				}
			}
		default:
			return nil, fmt.Errorf("bug: unsupported status: %s", currentStatus)
		}
		index++
	}
	resStack := utils.NewStack[any]()
	opStack := utils.NewStack[string]()
	for i := 0; i < len(tmpItems); i++ {
		item := tmpItems[i]
		switch v := item.(type) {
		case *rule.FingerPrintRule:
			if v.MatchParam.Op == "" || len(v.MatchParam.Params) != 2 {
				return nil, errors.New("invalid rule")
			}
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
		sort.Slice(rs, func(i, j int) bool { return true })
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
