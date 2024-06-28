package rule

import (
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"strconv"
)

type tmpGeneralRule struct {
	exp        string
	or         bool
	rebuildCPE func(cpe *CPE)
}

func DecompileFingerprintRuleOpCodes(codes []*OpCode) (*GeneralRule, error) {
	expStack := utils.NewStack[any]()
	var currentExpListP *[]*tmpGeneralRule
	//opPoint := [][3]int{}
	endPoint := map[int]struct{}{}
	for i := 0; i < len(codes); i++ {
		//if _, ok := endPoint[i]; ok {
		//	ds := expStack.Pop().(*[]*tmpGeneralRule)
		//	if len(*ds) == 1 {
		//		preRule := (*ds)[0]
		//		op := expStack.Pop().(string)
		//		rule := expStack.Pop().(*tmpGeneralRule)
		//		exp := fmt.Sprintf("%s %s %s", preRule.exp, op, rule.exp)
		//
		//	} else {
		//
		//	}
		//}
		code := codes[i]
		switch code.Op {
		case OpInfo:
			//expS := []string{}
			//pre := expStack.Pop().(*tmpGeneralRule).exp
			//for !expStack.IsEmpty() {
			//	//expS = append(expS, expStack.Pop().(*tmpGeneralRule).exp)
			//	exp := expStack.Pop().(*tmpGeneralRule).exp
			//	op := opPointStack.Pop()
			//
			//}
			//sort.Slice(expS, func(i, j int) bool { return true })

			if v := expStack.Pop().(*tmpGeneralRule); v.exp != "" {
				info := code.data[0].(*CPE)
				if v.rebuildCPE != nil {
					v.rebuildCPE(info)
				}
				expStack.Push(&GeneralRule{
					MatchExpression: v.exp,
					CPE:             info,
				})
			}
		case OpData:
			expStack.Push("banner")
		case OpExtractData:
			switch code.data[1] {
			case "header_item":
				expStack.Push(fmt.Sprintf("header_%s", utils.InterfaceToString(code.data[2])))
			default:
				expStack.Push(code.data[1])
			}
		case OpPush:
			s, ok := code.data[0].(string)
			if ok {
				expStack.Push(strconv.Quote(s))
			} else {
				expStack.Push(code.data[0])
			}
		case OpOr:
			expStack.Push("or")
			currentExp := []*tmpGeneralRule{}
			currentExpListP = &currentExp
			expStack.Push(currentExpListP)
			endPoint[code.data[0].(int)] = struct{}{}
		case OpAnd:
			expStack.Push("and")
			currentExp := []*tmpGeneralRule{}
			currentExpListP = &currentExp
			expStack.Push(currentExpListP)
			endPoint[code.data[0].(int)] = struct{}{}
		case OpEqual:
			d1 := expStack.Pop()
			d2 := expStack.Pop()
			*currentExpListP = append(*currentExpListP, &tmpGeneralRule{exp: fmt.Sprintf("%v == %v", d1, d2)})
		case OpContains:
			d := expStack.PopN(2)
			d1 := d[1]
			d2 := d[0]
			*currentExpListP = append(*currentExpListP, &tmpGeneralRule{exp: fmt.Sprintf("%v = %v", d1, d2)})
		case OpRegexpMatch:
			d := expStack.PopN(2)
			datas := []string{}
			switch ret := d[1].(type) {
			case string:
				datas = append(datas, ret)
			case []string:
				datas = ret
			}
			pattern := d[0].(string)
			varName := d[1].(string)
			exp := fmt.Sprintf("%v ~= %v", varName, pattern)
			if len(code.data) == 6 {
				*currentExpListP = append(*currentExpListP, &tmpGeneralRule{exp: exp, rebuildCPE: func(info *CPE) {
					getGroup := func(s *string, index int) {
						*s = fmt.Sprintf("$%d", index)
					}
					getGroup(&info.Vendor, code.data[0].(int))
					getGroup(&info.Product, code.data[1].(int))
					getGroup(&info.Version, code.data[2].(int))
					getGroup(&info.Update, code.data[3].(int))
					getGroup(&info.Edition, code.data[4].(int))
					getGroup(&info.Language, code.data[5].(int))
				}})
			} else {
				*currentExpListP = append(*currentExpListP, &tmpGeneralRule{exp: exp})
			}
		}
	}
	if expStack.Size() == 0 {
		return nil, nil
	}
	return expStack.Pop().(*GeneralRule), nil
}
