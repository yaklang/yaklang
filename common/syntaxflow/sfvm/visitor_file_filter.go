package sfvm

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sf"
	"github.com/yaklang/yaklang/common/utils"
	regexp_utils "github.com/yaklang/yaklang/common/utils/regexp-utils"
)

func (y *SyntaxFlowVisitor) VisitFileFilterContent(raw sf.IFileFilterContentStatementContext) error {
	if y == nil || raw == nil {
		return nil
	}
	i, _ := raw.(*sf.FileFilterContentStatementContext)
	if i == nil {
		return nil
	}

	y.EmitCheckStackTop()
	var fileInput string // fileName or compiled reg expression for fileName
	var err error
	if i.FileFilterContentInput() != nil {
		fileInput, err = y.VisitFileFilterContentInput(i.FileFilterContentInput())
		if err != nil {
			return err
		}
	}

	if i.FileFilterContentMethod() != nil {
		err = y.VisitFileFilterContentMethod(i.FileFilterContentMethod(), fileInput)
	}
	if ref, ok := i.RefVariable().(*sf.RefVariableContext); ok {
		varName := y.VisitRefVariable(ref)
		y.EmitUpdate(varName)
	} else {
		y.EmitPop()
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

func (y *SyntaxFlowVisitor) VisitFileFilterContentMethod(raw sf.IFileFilterContentMethodContext, fileInput string) error {
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
		y.EmitFileFilterXpath(fileInput, paramMap, paramList)
	case "regexp", "re":
		y.EmitFileFilterReg(fileInput, paramMap, paramList)
	case "jsonpath", "json":
		y.EmitFileFilterJsonPath(fileInput, paramMap, paramList)
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
			// y.VisitNameFilter()
			value, ok := item.FileFilterContentMethodParamValue().(*sf.FileFilterContentMethodParamValueContext)
			if !ok {
				continue
			}
			name, ok := value.NameFilter().(*sf.NameFilterContext)
			if !ok {
				continue
			}
			res := ""
			if reg, ok := name.RegexpLiteral().(*sf.RegexpLiteralContext); ok {
				reg := reg.GetText()
				reg = reg[1 : len(reg)-1]

				if !regexp_utils.NewYakRegexpUtils(reg).CanUse() {
					log.Errorf("regexp compile failed: %s", reg)
					continue
				}
				res = reg
			} else {
				res = name.Identifier().GetText()
			}
			if s, err := strconv.Unquote(res); err == nil {
				res = s
			}
			paramList = append(paramList, res)
		}
	}
	return paramMap, paramList
}
