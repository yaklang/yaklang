package ssaapi

import (
	"bytes"
	"encoding/xml"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"io"
	"strings"
)

func xmlHandler(value string, docTypeHandler func(string) bool, handler func(xml.StartElement), dataHandler func(xml.CharData)) {
	decoder := xml.NewDecoder(strings.NewReader(value))

	doctype := false

	for {
		t, err := decoder.Token()
		if err != nil {
			if err != io.EOF {
				log.Errorf("error: %v", err)
			}
			break
		}

		if !doctype {
			switch se := t.(type) {
			case xml.Directive:
				se = bytes.TrimSpace(se)
				if strings.HasPrefix(string(se), `DOCTYPE`) || strings.HasPrefix(string(se), `doctype`) {
					doctype = true
					if docTypeHandler != nil {
						if !docTypeHandler(string(se)) {
							return
						}
					}
				}
			}
			continue
		}

		switch se := t.(type) {
		case xml.StartElement:
			if handler != nil {
				handler(se)
			}
		case xml.CharData:
			if dataHandler != nil {
				dataHandler(se)
			}
		case xml.Comment:
			log.Infof("xml.Comment: %v", string(se))
		case xml.ProcInst:
			log.Infof("xml.ProcIns target: %v, inst: %v", se.Target, string(se.Inst))
		default:
			log.Infof("unknown: %T", se)
		}
	}
}

var nativeCallMybatixXML = func(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
	prog, err := fetchProgram(v)
	if err != nil {
		return false, nil, err
	}

	type focusStmt struct {
		FullTypeNameClass string
		TypeName          string
	}

	for name, value := range prog.Program.ExtraFile {
		log.Infof("start to handling: %v", name)

		mapperStack := utils.NewStack[string]()
		xmlHandler(value, func(s string) bool {
			if utils.MatchAnyOfSubString(s, `mybatis.org`, `mybatis-3-mapper.dtd`) {
				return true
			}
			return false
		}, func(element xml.StartElement) {
			_ = element.Name
			if strings.ToLower(element.Name.Local) == "mapper" {
				mapperStack.Push(element.Name.Local)
			}
		}, func(data xml.CharData) {

		})
	}
	return true, v, nil
}
