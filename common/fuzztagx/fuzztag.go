package fuzztagx

import (
	"errors"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/fuzztagx/parser"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/cartesian"
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
	var isDynFunRes bool
	if fun.Expand != nil {
		if isDynFun, ok := fun.Expand["IsDynFun"]; ok {
			if v, ok := isDynFun.(func(name, params string) bool); ok {
				isDynFunRes = v(name, params)
			}
		}
	}
	if fun.IsDyn || isDynFunRes {
		f.Labels = append(f.Labels, "dyn")
	}
	return fun.Fun(params)
}

type SimpleFuzzTag struct {
	parser.BaseTag
}

func (f *SimpleFuzzTag) Exec(raw *parser.FuzzResult, methods ...map[string]*parser.TagMethod) ([]*parser.FuzzResult, error) {
	data := string(raw.GetData())
	rawData := data
	data = strings.TrimSpace(data)
	var method func() ([]*parser.FuzzResult, error)
	isDyn := false
	labels := []string{}
	getFun := func(data string) (*parser.TagMethod, error) {
		data = strings.TrimSpace(data)
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
					originF := fun.Fun
					fun.Fun = func(s string) ([]*parser.FuzzResult, error) {
						if fun.Expand != nil {
							if isDynFun, ok := fun.Expand["IsDynFun"]; ok {
								if v, ok := isDynFun.(func(name, params string) bool); ok {
									if v(name, s) {
										set := utils.NewSet(labels)
										set.Add("dyn")
										f.Labels = set.List()
									}
								}
							}
						}
						return originF(s)
					}
					return fun, nil
				}
			}
		} else {
			return nil, errors.New("invalid method name")
		}
		return &parser.TagMethod{
			Name: "raw",
			Fun: func(s string) ([]*parser.FuzzResult, error) {
				return []*parser.FuzzResult{parser.NewFuzzResultWithData("")}, nil
			},
		}, nil
	}

	compile := func() (err error) {
		defer func() {
			if e := recover(); e != nil {
				err = utils.Error(e)
			}
		}()
		matchedPos := parser.IndexAllSubstrings(data, MethodLeft, MethodRight)
		if len(matchedPos) == 0 {
			f, err := getFun(data)
			if err != nil {
				return err
			}
			if f == nil {
				method = func() ([]*parser.FuzzResult, error) {
					return []*parser.FuzzResult{parser.NewFuzzResultWithData("")}, nil
				}
			}
			method = func() ([]*parser.FuzzResult, error) {
				return f.Fun("")
			}
		} else {
			escape := func(s string) string {
				s = strings.ReplaceAll(s, `\(`, `(`)
				s = strings.ReplaceAll(s, `\)`, `)`)
				return s
			}
			pre := 0
			methodStack := utils.NewStack[any]()
			for _, pos := range matchedPos {
				if pos[1]-1 > 0 {
					if data[pos[1]-1] == '\\' {
						continue
					}
				}
				if pos[0] == 0 {
					methodStack.Push(escape(data[pre:pos[1]]))
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
								f, err := getFun(v)
								if err != nil {
									return err
								}
								if f == nil {
									methodStack.Push(func() ([]*parser.FuzzResult, error) {
										return []*parser.FuzzResult{parser.NewFuzzResultWithData("")}, nil
									})
								} else {
									if len(params) != 0 && pre != pos[1] {
										return errors.New("error param")
									}
									if len(params) == 0 {
										strParam := escape(data[pre:pos[1]])
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
							}
							stop = true
						}
					}
					pre = pos[1] + 1
				}
			}
			if len(matchedPos) > 0 && utils.GetLastElement[[2]int](matchedPos)[1]+1 != len(data) {
				return errors.New(fmt.Sprintf("unresolved data: %s", escape(data[pre:])))
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
		return []*parser.FuzzResult{parser.NewFuzzResultWithData(fmt.Sprintf("{{%s}}", escaper.Escape(rawData)))}, nil
	}
	set := utils.NewSet(labels)
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
func NewGenerator(code string, table map[string]*parser.TagMethod, isSimple, syncTag bool) (*parser.Generator, error) {
	nodes, err := ParseFuzztag(code, isSimple)
	if err != nil {
		return nil, err
	}
	gener := parser.NewGenerator(nodes, table)
	gener.SetTagsSync(syncTag)
	return gener, nil
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
