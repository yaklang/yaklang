package sfvm

import (
	"github.com/yaklang/yaklang/common/syntaxflow/sf"
	"github.com/yaklang/yaklang/common/utils"
	"regexp"
	"strings"
)

func (y *SyntaxFlowVisitor) VisitFileFilterContent(raw sf.IFileFilterContentStatementContext) error {
	if y == nil || raw == nil {
		return nil
	}
	i, _ := raw.(*sf.FileFilterContentStatementContext)
	if i == nil {
		return nil
	}

	var f string // fileName or compiled reg expression for fileName
	var err error
	if i.FileFilterContentInput() != nil {
		f, err = y.VisitFileFilterContentInput(i.FileFilterContentInput())
		if err != nil {
			return err
		}
	}

	if i.FileFilterContentMethod() != nil {
		err = y.VisitFileFilterContentMethod(i.FileFilterContentMethod(), f)
	}
	return err
}

func (y *SyntaxFlowVisitor) VisitFileFilterContentInput(raw sf.IFileFilterContentInputContext) (string, error) {
	if y == nil || raw == nil {
		return "", nil
	}
	i, _ := raw.(*sf.FileFilterContentInputContext)
	if i == nil {
		return "", nil
	}

	if i.FileName() != nil {
		text := i.FileName().GetText()
		return text, nil
	} else if i.RegexpLiteral() != nil {
		reg := i.RegexpLiteral().GetText()
		reg = reg[1 : len(reg)-1]
		reIns, err := regexp.Compile(reg)
		if err != nil {
			return "", err
		}
		text := reIns.String()
		return text, nil
	}
	return "", utils.Error("file filter content input is not identifier or regexp literal")
}

func (y *SyntaxFlowVisitor) VisitFileFilterContentMethod(raw sf.IFileFilterContentMethodContext, f string) error {
	if y == nil || raw == nil {
		return nil
	}
	i, _ := raw.(*sf.FileFilterContentMethodContext)
	if i == nil {
		return nil
	}

	paramMap := make(map[string]string)
	var paramList []string

	if ret := i.FileFilterContentMethodParam(); ret != nil {
		paramMap, paramList = y.VisitFileFilterContentMethodParam(ret)
	}

	m := i.Identifier().GetText()
	m = strings.ToLower(m)
	switch m {
	case "xpath":
		y.EmitFileFilterXpath(f, paramMap, paramList)
	case "regexp", "re":
		y.EmitFileFilterReg(f, paramMap, paramList)
	case "jsonpath", "json":
		y.EmitFileFilterJsonPath(f, paramMap, paramList)
	default:
		return utils.Errorf("file filter method not support:%s", m)
	}
	return nil
}

func (y *SyntaxFlowVisitor) VisitFileFilterContentMethodParam(raw sf.IFileFilterContentMethodParamContext) (map[string]string, []string) {
	if y == nil || raw == nil {
		return nil, nil
	}
	i, _ := raw.(*sf.FileFilterContentMethodParamContext)
	if i == nil {
		return nil, nil
	}

	paramMap := make(map[string]string)
	var paramList []string
	for _, items := range i.AllFileFilterContentMethodParamItem() {
		item := items.(*sf.FileFilterContentMethodParamItemContext)
		if pk := item.FileFilterContentMethodParamKey(); pk != nil {
			key := pk.(*sf.FileFilterContentMethodParamKeyContext).Identifier().GetText()
			if pv := item.FileFilterContentMethodParamValue(); pv != nil {
				value := pv.(*sf.FileFilterContentMethodParamValueContext).NameFilter().GetText()
				paramMap[key] = value
			}
		} else {
			value := item.FileFilterContentMethodParamValue().(*sf.FileFilterContentMethodParamValueContext).NameFilter().GetText()
			paramList = append(paramList, value)
		}
	}
	return paramMap, paramList
}
