package fuzztagx

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/fuzztagx/parser"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	MethodLeft  = "("
	MethodRight = ")"
)
const YakHotPatchErr = "__YakHotPatchErr@"

type FuzzTag struct {
	parser.BaseTag
}
type (
	stepDataGetter  func() ([]byte, error)
	fuzztagCallInfo struct {
		name   string
		params string
		labels []string
	}
)

func parseFuzztagCall(content string) (info *fuzztagCallInfo, err error) {
	info = &fuzztagCallInfo{}
	matchedPos := parser.IndexAllSubstrings(content, MethodLeft, MethodRight)
	if len(matchedPos) == 0 {
		if isIdentifyString(content) {
			info.name = content
		}
	} else if len(matchedPos) > 1 && matchedPos[0][0] == 0 && matchedPos[len(matchedPos)-1][0] == 1 { // 第一个是左括号，最后一个右括号
		leftPos := matchedPos[0]
		rightPos := matchedPos[len(matchedPos)-1]
		if leftPos[0] == 0 && rightPos[0] == 1 && strings.TrimSpace(content[rightPos[1]+len(MethodRight):]) == "" {
			methodName := strings.TrimSpace(content[:leftPos[1]])
			if !isIdentifyString(methodName) {
				return nil, errors.New("method name is invalid")
			}
			info.name = methodName
			info.params = content[leftPos[1]+len(MethodLeft) : rightPos[1]]
		} else {
			return nil, errors.New("invalid quote")
		}
	} else {
		return nil, errors.New("invalid quote")
	}
	splits := strings.Split(info.name, "::")
	if len(splits) > 1 {
		info.name = splits[0]
		for _, label := range splits[1:] {
			label = strings.TrimSpace(label)
			if label == "" {
				continue
			}
			info.labels = append(info.labels, label)
		}
	}
	if info.name == "" {
		return nil, errors.New("fuzztag name is empty")
	}
	return info, nil
}

func (f *FuzzTag) Exec(ctx context.Context, raw *parser.FuzzResult, yield func(result *parser.FuzzResult), methods map[string]*parser.TagMethod) error {
	runFun := func(method *parser.TagMethod, data string) error {
		if method.YieldFun != nil {
			return method.YieldFun(ctx, data, yield)
		} else {
			res, err := method.Fun(data)
			for _, re := range res {
				yield(re)
			}
			return err
		}
	}
	data := string(raw.GetData())
	name := ""
	params := ""
	info, err := parseFuzztagCall(data)
	if err != nil { // 对于编译错误，返回原文
		escaper := parser.NewDefaultEscaper(`\`, "{{", "}}")
		yield(parser.NewFuzzResultWithData(fmt.Sprintf("{{%s}}", escaper.Escape(data))))
		return nil
	}
	name = info.name
	params = info.params
	f.Labels = info.labels
	fun := methods[name]
	if fun == nil {
		yield(parser.NewFuzzResultWithData(""))
		return nil
		// return nil, utils.Errorf("fuzztag name %s not found", name)
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
	if fun.IsFlowControl {
		f.Labels = append(f.Labels, "flowcontrol")
	}
	return runFun(fun, params)
}

type SimpleFuzzTag struct {
	parser.BaseTag
}

func (f *SimpleFuzzTag) Exec(ctx context.Context, raw *parser.FuzzResult, yield func(result *parser.FuzzResult), methods map[string]*parser.TagMethod) error {
	data := string(raw.GetData())
	rawData := data
	data = strings.TrimSpace(data)
	var commonFuzztag *FuzzTag
	isDyn := false
	labels := []string{}
	compile := func() (err error) {
		defer func() {
			if e := recover(); e != nil {
				err = utils.Error(e)
			}
		}()
		matchedPos := parser.IndexAllSubstrings(data, MethodLeft, MethodRight)
		if len(matchedPos) == 0 {
			commonFuzztag = &FuzzTag{
				parser.BaseTag{
					Data: []parser.Node{parser.StringNode(data)},
				},
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
					params := []parser.Node{}
					for {
						if stop {
							break
						}
						if methodStack.IsEmpty() {
							return errors.New("match left brace failed")
						}
						item := methodStack.Pop()
						switch ret := item.(type) {
						case parser.Node:
							params = append(params, ret)
						case [2]int:
							if methodStack.Size() < 1 {
								return errors.New("match left brace failed")
							}
							item = methodStack.Pop()
							if v, ok := item.(string); !ok {
								return errors.New("match left brace failed")
							} else {
								funName := v
								before, after, ok := strings.Cut(funName, "::")
								if ok {
									funName = before
									labels = append(labels, strings.Split(after, "::")...)
								}
								newTag := &FuzzTag{
									parser.BaseTag{},
								}
								if len(params) != 0 && pre != pos[1] {
									return errors.New("error param")
								}
								if len(params) == 0 {
									strParam := escape(data[pre:pos[1]])
									newTag.Data = append(newTag.Data, parser.StringNode(fmt.Sprintf("%s(%s)", funName, strParam)))
									methodStack.Push(newTag)
								} else {
									newParams := []parser.Node{}
									for i := len(params) - 1; i >= 0; i-- {
										newParams = append(newParams, params[i])
									}
									newTag.Data = append(newTag.Data, parser.StringNode(fmt.Sprintf("%s(", funName)))
									newTag.Data = append(newTag.Data, newParams...)
									newTag.Data = append(newTag.Data, parser.StringNode(")"))
									methodStack.Push(newTag)
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
			if v, ok := imethod.(*FuzzTag); ok {
				commonFuzztag = v
			}
		}
		return nil
	}
	if isDyn {
		labels = append(labels, "dyn")
	}
	if err := compile(); err != nil { // 对于编译错误，返回原文
		escaper := parser.NewDefaultEscaper(`\`, "{{", "}}")
		yield(parser.NewFuzzResultWithData(fmt.Sprintf("{{%s}}", escaper.Escape(rawData))))
		return nil
	}
	if f.IsFlowControl() {
		labels = append(labels, "flowcontrol")
	}
	set := utils.NewSet(labels)
	f.Labels = set.List()
	commonFuzztag.Labels = f.Labels
	generator := parser.NewGenerator(ctx, []parser.Node{commonFuzztag}, methods)
	for generator.Next() {
		if generator.Error != nil {
			return generator.Error
		}
		res := generator.Result()
		if len(res.Source) > 0 { // hide fuzztag source
			res.Source = res.Source[:len(res.Source)-1]
		}
		yield(res)
	}
	return nil
}

type RawTag struct {
	parser.BaseTag
}

func (r *RawTag) Exec(ctx context.Context, raw *parser.FuzzResult, yield func(result *parser.FuzzResult), methodTable map[string]*parser.TagMethod) error {
	yield(parser.NewFuzzResultWithData(raw.GetData()))
	return nil
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
	return NewGeneratorEx(context.Background(), code, table, isSimple, syncTag)
}

func NewGeneratorEx(ctx context.Context, code string, table map[string]*parser.TagMethod, isSimple, syncTag bool) (*parser.Generator, error) {
	nodes, err := ParseFuzztag(code, isSimple)
	if err != nil {
		return nil, err
	}
	gener := parser.NewGenerator(ctx, nodes, table)
	gener.SetTagsSync(syncTag)
	return gener, nil
}

func isIdentifyString(s string) bool {
	return utils.MatchAllOfRegexp(s, "^[a-zA-Z_][a-zA-Z0-9_:-]*$")
}

func GetResultVerbose(f *parser.FuzzResult) []string {
	return getResultVerbose(f, map[*parser.FuzzResult]struct{}{})
}
func getResultVerbose(f *parser.FuzzResult, visitedMap map[*parser.FuzzResult]struct{}) []string {
	if visitedMap != nil {
		if _, ok := visitedMap[f]; ok {
			return []string{}
		} else {
			visitedMap[f] = struct{}{}
		}
	}
	var verboses []string
	for _, datum := range f.Source {
		switch ret := datum.(type) {
		case *parser.FuzzResult:
			verboses = append(verboses, getResultVerbose(ret, visitedMap)...)
		}
	}
	if !f.ByTag {
		return verboses
	}
	if f.Error != nil {
		errStr := f.Error.Error()
		if strings.HasPrefix(errStr, YakHotPatchErr) {
			f.Verbose = fmt.Sprintf("[%s]", errStr)
		} else {
			f.Verbose = fmt.Sprintf("[%s%s]", YakHotPatchErr, errStr)
		}
	} else if f.Verbose == "" {
		f.Verbose = utils.InterfaceToString(f.Data)
	}
	if f.Verbose == "" {
		return verboses
	}
	return append([]string{f.Verbose}, verboses...)
}
