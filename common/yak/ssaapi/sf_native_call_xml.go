package ssaapi

import (
	"encoding/xml"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/xml2"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"regexp"
	"strings"
)

var (
	mybatisVarExtractor = regexp.MustCompile(`\$\{\s*([^}]+)\s*}`)
)

type mybatisXMLMapper struct {
	FullClassName string
	ClassName     string
	Namespace     string
	entityStack   *utils.Stack[*mybatisXMLQuery]
}

func newMybatisXMLMapper() *mybatisXMLMapper {
	return &mybatisXMLMapper{
		entityStack: utils.NewStack[*mybatisXMLQuery](),
	}
}

type mybatisXMLQuery struct {
	mapper         *mybatisXMLMapper
	Id             string
	CheckParamName []string
}

func newMybatisXMLQuery(mapper *mybatisXMLMapper, id string) *mybatisXMLQuery {
	return &mybatisXMLQuery{
		mapper:         mapper,
		Id:             id,
		CheckParamName: make([]string, 0),
	}
}

func (m *mybatisXMLQuery) SyntaxFlowFirst(token string) string {
	if m.mapper == nil {
		return ""
	}
	var builder strings.Builder
	builder.WriteString(m.mapper.ClassName)
	builder.WriteString(".")
	builder.WriteString(m.Id)
	builder.WriteString("(*?{!have: this && opcode: param && any: " + strings.Join(m.CheckParamName, ",") + " } as $_" + token + ")")
	return builder.String()
}

func (m *mybatisXMLQuery) SyntaxFlowFinal(token string) string {
	if m.mapper == nil {
		return ""
	}
	var builder strings.Builder
	builder.WriteString(m.mapper.ClassName)
	builder.WriteString(".")
	builder.WriteString(m.Id)
	builder.WriteString("(*?{!have: this && opcode: param } as $_" + token + ")")

	return builder.String()
}

var nativeCallMybatisXML = func(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
	prog, err := fetchProgram(v)
	if err != nil {
		return false, nil, err
	}

	var vals []sfvm.ValueOperator

	for name, content := range prog.Program.ExtraFile {
		log.Debugf("start to handling: %v len: %v", name, len(content))
		if len(content) <= 128 {
			hash := content
			editor, _ := ssadb.GetIrSourceFromHash(hash)
			if editor != nil {
				content = editor.GetSourceCode()
			}
		}

		mapperStack := utils.NewStack[*mybatisXMLMapper]()
		mapper := newMybatisXMLMapper()
		mapperStack.Push(mapper)

		onDirective := xml2.WithDirectiveHandler(func(directive xml.Directive) bool {
			if utils.MatchAnyOfSubString(string(directive), "dtd/mybatis-", "mybatis.org") {
				return true
			}
			return false
		})
		onStartElement := xml2.WithStartElementHandler(func(element xml.StartElement) {
			if element.Name.Local == "mapper" {
				mapperStack.Push(newMybatisXMLMapper())
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
		onCharData := xml2.WithCharDataHandler(func(data xml.CharData) {
			i := mapperStack.Peek()
			if utils.IsNil(i) {
				return
			}
			query := i.entityStack.Peek()
			if query == nil {
				return
			}
			for _, groups := range mybatisVarExtractor.FindAllStringSubmatch(string(data), -1) {
				variableName := groups[1]
				query.CheckParamName = append(query.CheckParamName, variableName)
			}
			if len(query.CheckParamName) > 0 {
				token := utils.RandStringBytes(16)
				token = "a" + token
				for _, sf := range []string{
					query.SyntaxFlowFirst(token), query.SyntaxFlowFinal(token),
				} {
					if sf == "" {
						continue
					}
					val := prog.NewValue(ssa.NewConst(sf))

					_ = val.AppendPredecessor(v, frame.WithPredecessorContext("mybatis-${...}"))
					log.Infof("mybatis-${...}: fetch query: %v", sf)
					_, _, err := nativeCallEval(val, frame, nil)
					if err != nil {
						log.Warnf("mybatis-${...}: fetch query: %v, error: %v", sf, err)
					}
					results, ok := frame.GetSymbolTable().Get("_" + token)
					if !ok {
						continue
					}
					results.Recursive(func(operator sfvm.ValueOperator) error {
						vals = append(vals, operator)
						return nil
					})
				}
				frame.GetSymbolTable().Delete("_" + token)
			}
		})
		xml2.Handle(content, onDirective, onStartElement, onEndElement, onCharData)
	}
	return true, sfvm.NewValues(vals), nil
}
