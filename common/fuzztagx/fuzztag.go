package fuzztagx

import (
	"errors"
	"fmt"
	"github.com/yaklang/yaklang/common/fuzztagx/parser"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/cartesian"
	"strings"
)

const (
	MethodLeft  = "("
	MethodRight = ")"
)
const YakHotPatchErr = "__YakHotPatchErr@"

type FuzzTag struct {
	parser.BaseTag
}

func (f *FuzzTag) Exec(raw *parser.FuzzResult, methods ...map[string]*parser.TagMethod) ([]*parser.FuzzResult, error) {
	data := string(raw.GetData())
	name := ""
	params := ""
	labels := []string{}
	compile := func() error {
		matchedPos := parser.IndexAllSubstrings(data, MethodLeft, MethodRight)
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
		escaper := parser.NewDefaultEscaper(`\`, "{{", "}}")
		return []*parser.FuzzResult{parser.NewFuzzResultWithData(fmt.Sprintf("{{%s}}", escaper.Escape(data)))}, nil
	}
	var fun *parser.TagMethod
	if f.Methods != nil {
		methods = append(methods, *f.Methods)
	}
	for _, fMap := range methods {
		if fun = fMap[name]; fun != nil {
			break
		}
	}
	if fun == nil {
		return []*parser.FuzzResult{parser.NewFuzzResultWithData("")}, nil
		//return nil, utils.Errorf("fuzztag name %s not found", name)
	}
	if fun.IsDyn {
		f.Labels = append(f.Labels, "dyn")
	}
	return fun.Fun(params)
}

type SimpleFuzzTag struct {
	parser.BaseTag
}

func (f *SimpleFuzzTag) Exec(raw *parser.FuzzResult, methods ...map[string]*parser.TagMethod) ([]*parser.FuzzResult, error) {
	data := string(raw.GetData())
	var method func() ([]*parser.FuzzResult, error)
	isDyn := false
	labels := []string{}
	getFun := func(data string) *parser.TagMethod {
		if isIdentifyString(data) {
			name := data
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
			if f.Methods != nil {
				methods = append(methods, *f.Methods)
			}
			for _, fMap := range methods {
				if fun := fMap[name]; fun != nil {
					if fun.IsDyn {
						isDyn = true
					}
					return fun
				}
			}
		}
		return nil
	}

	compile := func() (err error) {
		defer func() {
			if e := recover(); e != nil {
				err = utils.Error(e)
			}
		}()
		matchedPos := parser.IndexAllSubstrings(data, MethodLeft, MethodRight)
		if len(matchedPos) == 0 {
			f := getFun(data)
			if f == nil {
				return fmt.Errorf("not found method: %s", data)
			}
			method = func() ([]*parser.FuzzResult, error) {
				return f.Fun("")
			}
		} else {
			pre := 0
			methodStack := utils.NewStack[any]()
			for _, pos := range matchedPos {
				if pos[0] == 0 {
					methodStack.Push(data[pre:pos[1]])
					methodStack.Push(pos)
					pre = pos[1] + 1
				} else {

					stop := false
					params := []func() ([]*parser.FuzzResult, error){}
					for {
						if stop {
							break
						}
						if methodStack.IsEmpty() {
							return errors.New("match left brace failed")
						}
						item := methodStack.Pop()
						switch ret := item.(type) {
						case func() ([]*parser.FuzzResult, error):
							params = append(params, ret)
						case [2]int:
							if methodStack.Size() < 1 {
								return errors.New("match left brace failed")
							}
							item = methodStack.Pop()
							if v, ok := item.(string); !ok {
								return errors.New("match left brace failed")
							} else {
								f := getFun(v)
								if f == nil {
									return fmt.Errorf("not found method: %s", v)
								}
								if len(params) != 0 && pre != pos[1] {
									return errors.New("error param")
								}
								if len(params) == 0 {
									strParam := data[pre:pos[1]]
									methodStack.Push(func() ([]*parser.FuzzResult, error) {
										return f.Fun(strParam)
									})
								} else {
									methodStack.Push(func() ([]*parser.FuzzResult, error) {
										paramForFun := [][]*parser.FuzzResult{}
										for _, param := range params {
											res, err := param()
											if err != nil {
												return nil, fmt.Errorf("eval function %s error: %w", v, err)
											}
											paramForFun = append(paramForFun, res)
										}
										reverseParamForFun := [][]*parser.FuzzResult{}
										for i := len(paramForFun) - 1; i >= 0; i-- {
											reverseParamForFun = append(reverseParamForFun, paramForFun[i])
										}
										cartesianParams, err := cartesian.Product(reverseParamForFun)
										if err != nil {
											return nil, err
										}
										result := []*parser.FuzzResult{}
										for _, params := range cartesianParams {
											paramStr := ""
											for _, param := range params {
												paramStr += string(param.GetData())
											}
											res, err := f.Fun(paramStr)
											if err != nil {
												return nil, fmt.Errorf("eval function %s error: %w", v, err)
											}
											result = append(result, res...)
										}
										return result, nil
									})
								}

							}
							stop = true
						}
					}
					pre = pos[1] + 1
				}
			}
			if methodStack.Size() != 1 {
				return errors.New("error method stack")
			}
			imethod := methodStack.Pop()
			if v, ok := imethod.(func() ([]*parser.FuzzResult, error)); ok {
				method = v
			}
		}
		return nil
	}
	if isDyn {
		labels = append(labels, "dyn")
	}
	if err := compile(); err != nil { // 对于编译错误，返回原文
		escaper := parser.NewDefaultEscaper(`\`, "{{", "}}")
		return []*parser.FuzzResult{parser.NewFuzzResultWithData(fmt.Sprintf("{{%s}}", escaper.Escape(data)))}, nil
	}
	set := utils.NewSet[string]()
	set.AddList(labels)
	f.Labels = set.List()
	return method()
}

type RawTag struct {
	parser.BaseTag
}

func (r *RawTag) Exec(result *parser.FuzzResult, m ...map[string]*parser.TagMethod) ([]*parser.FuzzResult, error) {
	return []*parser.FuzzResult{result}, nil
}

func ParseFuzztag(code string, simple bool) ([]parser.Node, error) {
	if simple {
		return parser.Parse(code,
			parser.NewTagDefine("fuzztag", "{{", "}}", &SimpleFuzzTag{}),
			parser.NewTagDefine("rawtag", "{{=", "=}}", &RawTag{}, true),
		)
	} else {
		return parser.Parse(code,
			parser.NewTagDefine("fuzztag", "{{", "}}", &FuzzTag{}),
			parser.NewTagDefine("rawtag", "{{=", "=}}", &RawTag{}, true),
		)
	}

}
func NewGenerator(code string, table map[string]*parser.TagMethod, isSimple bool) (*parser.Generator, error) {
	nodes, err := ParseFuzztag(code, isSimple)
	if err != nil {
		return nil, err
	}
	return parser.NewGenerator(nodes, table), nil
}
func isIdentifyString(s string) bool {
	return utils.MatchAllOfRegexp(s, "^[a-zA-Z_][a-zA-Z0-9_:-]*$")
}
func GetResultVerbose(f *parser.FuzzResult) []string {
	var verboses []string
	for _, datum := range f.Source {
		switch ret := datum.(type) {
		case *parser.FuzzResult:
			verboses = append(verboses, GetResultVerbose(ret)...)
		}
	}
	if !f.ByTag {
		return verboses
	}
	if f.Error != nil {
		f.Verbose = fmt.Sprintf("[%s]", YakHotPatchErr+f.Error.Error())
	} else if f.Verbose == "" {
		f.Verbose = utils.InterfaceToString(f.Data)
	}
	return append([]string{f.Verbose}, verboses...)
}
