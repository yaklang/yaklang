package ssaapi

import (
	"encoding/xml"
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/utils/xml2"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

var (
	mybatisVarExtractor = regexp.MustCompile(`\$\{\s*([^}]+)\s*}`)
)

type mybatisXMLMapper struct {
	FullClassName string
	ClassName     string
	Namespace     string
	frame         *sfvm.SFFrame
	prog          *Program
	entityStack   *utils.Stack[*mybatisXMLQuery]
}

func newMybatisXMLMapper(prog *Program, frame *sfvm.SFFrame) *mybatisXMLMapper {
	return &mybatisXMLMapper{
		prog:        prog,
		frame:       frame,
		entityStack: utils.NewStack[*mybatisXMLQuery](),
	}
}

type mybatisXMLQuery struct {
	mapper      *mybatisXMLMapper
	Id          string
	CheckParams []*checkParam
}

type checkParam struct {
	name string
	rng  memedit.RangeIf
}

func newMybatisXMLQuery(mapper *mybatisXMLMapper, id string) *mybatisXMLQuery {
	return &mybatisXMLQuery{
		mapper:      mapper,
		Id:          id,
		CheckParams: make([]*checkParam, 0),
	}
}

func (m *mybatisXMLQuery) AddCheckParam(name string, rng memedit.RangeIf) {
	m.CheckParams = append(m.CheckParams, &checkParam{
		name: name,
		rng:  rng,
	})
}

func (m *mybatisXMLQuery) Check() []sfvm.ValueOperator {
	var res []sfvm.ValueOperator

	for _, param := range m.CheckParams {
		res = append(res, m.SyntaxFlowFirst(param.name, param.rng))
		res = append(res, m.SyntaxFlowFinal(param.rng))
	}
	return res
}

func (m *mybatisXMLQuery) SyntaxFlowFirst(name string, rng memedit.RangeIf) sfvm.ValueOperator {
	if m.mapper == nil {
		return nil
	}

	token := utils.RandStringBytes(16)
	token = "_a" + token
	var builder strings.Builder
	builder.WriteString(m.mapper.ClassName)
	builder.WriteString(".")
	builder.WriteString(m.Id)
	builder.WriteString("(*?{!have: this && opcode: param && have: \"" + name + "\" } as $" + token + ")")
	return m.runRuleAndFixRng(token, builder.String(), rng)
}

func (m *mybatisXMLQuery) SyntaxFlowFinal(rng memedit.RangeIf) sfvm.ValueOperator {
	if m.mapper == nil {
		return nil
	}

	token := utils.RandStringBytes(16)
	token = "a" + token
	var builder strings.Builder
	builder.WriteString(m.mapper.ClassName)
	builder.WriteString(".")
	builder.WriteString(m.Id)
	builder.WriteString("(*?{!have: this && opcode: param } as $" + token + ")")
	return m.runRuleAndFixRng(token, builder.String(), rng)
}

func (m *mybatisXMLQuery) runRuleAndFixRng(token string, rule string, rng memedit.RangeIf) sfvm.ValueOperator {
	if m == nil || m.mapper == nil {
		return nil
	}
	prog := m.mapper.prog
	frame := m.mapper.frame
	if prog == nil || frame == nil {
		return nil
	}

	val := prog.NewValue(ssa.NewConst(rule))
	_, _, err := nativeCallEval(val, frame, nil)
	if err != nil {
		log.Warnf("mybatis-${...}: fetch query: %v, error: %v", rule, err)
	}
	results, ok := frame.GetSymbolTable().Get(token)
	defer func() {
		frame.GetSymbolTable().Delete(token)
	}()
	if !ok {
		return nil
	}

	editor := rng.GetEditor()
	if editor == nil {
		return results
	}
	fileName := ssa.NewConstWithRange(rng.GetText(), rng)
	fileVal := prog.NewValue(fileName)
	results.AppendPredecessor(fileVal, frame.WithPredecessorContext("mybatis-${...}"))
	return results
}

var nativeCallMybatisXML = func(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
	prog, err := fetchProgram(v)
	if err != nil {
		return false, nil, err
	}

	var res []sfvm.ValueOperator

	offset := 0
	for name, content := range prog.Program.ExtraFile {
		log.Debugf("start to handling: %v len: %v", name, len(content))
		if !strings.HasSuffix(name, ".xml") {
			continue
		}
		var editor *memedit.MemEditor
		if len(content) <= 128 {
			hash := content
			editor, _ = ssadb.GetIrSourceFromHash(hash)
			if editor != nil {
				content = editor.GetSourceCode()
			}
		} else {
			editor = memedit.NewMemEditorWithFileUrl(content, name)
		}

		mapperStack := utils.NewStack[*mybatisXMLMapper]()
		mapper := newMybatisXMLMapper(prog, frame)
		mapperStack.Push(mapper)

		onDirective := xml2.WithDirectiveHandler(func(directive xml.Directive) bool {
			offset += len(directive)
			if utils.MatchAnyOfSubString(string(directive), "dtd/mybatis-", "mybatis.org") {
				return true
			}
			return false
		})
		onStartElement := xml2.WithStartElementHandler(func(element xml.StartElement) {
			if element.Name.Local == "mapper" {
				mapperStack.Push(newMybatisXMLMapper(prog, frame))
				mapper := mapperStack.Peek()
				for _, attr := range element.Attr {
					if attr.Name.Local == "namespace" {
						mapper.Namespace = attr.Value
						mapper.FullClassName = attr.Value
						idx := strings.LastIndex(attr.Value, ".")
						if idx > 0 {
							mapper.ClassName = attr.Value[idx+1:]
						} else {
							mapper.ClassName = attr.Value
						}
					}
				}
				return
			}
			if element.Name.Local == "resultMap" {
				return
			}
			i := mapperStack.Peek()
			if utils.IsNil(i) {
				return
			}

			var id string
			last := i.entityStack.Peek()
			if last != nil {
				id = last.Id
			}
			query := newMybatisXMLQuery(i, id)
			for _, attr := range element.Attr {
				if attr.Name.Local == "id" {
					query.Id = attr.Value
				}
			}
			i.entityStack.Push(query)
		})
		onEndElement := xml2.WithEndElementHandler(func(element xml.EndElement) {
			if element.Name.Local == "mapper" {
				mapper := mapperStack.Pop()
				if mapper != nil {
					log.Infof("mapper: %v", mapper)
				}
			}
			if element.Name.Local == "resultMap" {
				return
			}
			i := mapperStack.Peek()
			if utils.IsNil(i) {
				return
			}
			i.entityStack.Pop()
		})
		onCharData := xml2.WithCharDataHandler(func(data xml.CharData, offset int64) {
			i := mapperStack.Peek()
			if utils.IsNil(i) {
				return
			}
			query := i.entityStack.Peek()
			if query == nil {
				return
			}
			for _, match := range mybatisVarExtractor.FindAllStringSubmatchIndex(string(data), -1) {
				start, end := match[2], match[3]
				variableName := string(data[start:end])
				rng := editor.GetRangeByPosition(
					editor.GetPositionByOffset(start+int(offset)),
					editor.GetPositionByOffset(end+int(offset)),
				)
				query.AddCheckParam(variableName, rng)
			}
			res = append(res, query.Check()...)
		})
		xml2.Handle(content, onDirective, onStartElement, onEndElement, onCharData)
	}
	if len(res) > 0 {
		return true, sfvm.NewValues(res), nil
	}
	return false, nil, nil
}
