package standard_parser

import (
	"errors"
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
)

const (
	MethodLeft  = "("
	MethodRight = ")"
)

type FuzzTag struct {
	BaseTag
}

func (f *FuzzTag) IsNode() {
	//TODO implement me
	panic("implement me")
}

func (f *FuzzTag) Exec(data string, methods ...map[string]TagMethod) ([]FuzzResult, error) {
	name := ""
	params := ""
	labels := []string{}
	compile := func() error {
		matchedPos := IndexAllSubstrings(data, MethodLeft, MethodRight)
		if len(matchedPos) == 0 {
			if isIdentifyString(data) {
				name = data
			}
		} else if len(matchedPos) > 1 && matchedPos[0][0] == 0 && matchedPos[len(matchedPos)-1][0] == 1 { // 第一个是左括号，最后一个右括号
			leftPos := matchedPos[0]
			rightPos := matchedPos[len(matchedPos)-1]
			if leftPos[0] == 0 && rightPos[0] == 1 && strings.TrimSpace(data[rightPos[1]+len(MethodRight):]) == "" {
				methodName := strings.TrimSpace(data[:leftPos[1]])
				if !isIdentifyString(methodName) {
					return errors.New("method name is invalid")
				}
				name = methodName
				params = data[leftPos[1]+len(MethodLeft) : rightPos[1]]
			} else {
				return errors.New("invalid quote")
			}
		} else {
			return errors.New("invalid quote")
		}
		splits := strings.Split(name, "::")
		if len(splits) > 1 {
			name = splits[0]
			for _, label := range splits[1:] {
				label = strings.TrimSpace(label)
				if label == "" {
					continue
				}
				labels = append(labels, label)
			}
		}
		f.Labels = labels
		if name == "" {
			return errors.New("fuzztag name is empty")
		}
		return nil
	}
	if err := compile(); err != nil { // 对于编译错误，返回原文
		return []FuzzResult{FuzzResult(fmt.Sprintf("{{%s}}", data))}, nil
	}
	var fun TagMethod
	if f.Methods != nil {
		methods = append(methods, *f.Methods)
	}
	for _, fMap := range methods {
		if fun = fMap[name]; fun != nil {
			break
		}
	}
	if fun == nil {
		return nil, utils.Errorf("fuzztag name %s not found", name)
	}
	return fun(params)
}

func ParseFuzztag(code string, tagTypes ...*TagDefine) ([]Node, error) {
	return Parse(code, append([]*TagDefine{NewTagDefine("fuzztag", "{{", "}}", &FuzzTag{})}, tagTypes...)...)
}

func isIdentifyString(s string) bool {
	return utils.MatchAllOfRegexp(s, "^[a-zA-Z_][a-zA-Z0-9_:-]*$")
}
