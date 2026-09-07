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
	y.EmitPop()
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
		// A fileName that is a single regexpLiteral (e.g. ${/.*\.java$/}) is
		// parsed as fileName because nameFilter includes regexpLiteral; strip the
		// surrounding slashes and compile it as a path regex so the pattern is
		// usable by sfpattern's compilePathMatcher.
		if strings.HasPrefix(text, "/") && strings.HasSuffix(text, "/") && len(text) >= 2 {
			inner := text[1 : len(text)-1]
			if reIns, err := regexp.Compile(inner); err == nil {
				return reIns.String(), nil
			}
		}
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

// isFileFilterMethod reports whether a field-call name is a chained file-filter
// method (regexp family). Only these are intercepted in $a.regexp(...) form;
// xpath/jsonpath keep their member-access meaning.
func isFileFilterMethod(name string) bool {
	switch strings.ToLower(name) {
	case "regexp", "re", "pattern_regex", "pattern-regex", "patternregex",
		"pattern_regex_not", "pattern-regex-not", "patternregexnot",
		"pattern_not_regex", "pattern-not-regex", "patternnotregex", "not_regexp", "not_re":
		return true
	}
	return false
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
	case "regexp", "re", "pattern_regex", "pattern-regex", "patternregex":
		// pattern_regex: Semgrep pattern-regex compatible alias of regexp/re.
		y.EmitFileFilterReg(fileInput, paramMap, paramList)
	case "pattern_regex_not", "pattern-regex-not", "patternregexnot":
		// pattern_regex_not: first param is positive, remaining are negative
		// (Semgrep pattern-regex + pattern-not-regex in one call).
		if paramMap == nil {
			paramMap = make(map[string]string)
		}
		paramMap["__sf_pattern_not_list"] = "1"
		y.EmitFileFilterReg(fileInput, paramMap, paramList)
	case "pattern_not_regex", "pattern-not-regex", "patternnotregex", "not_regexp", "not_re":
		// Negative content regex (Semgrep pattern-not-regex). Marked for backends;
		// sfpattern treats hits as candidates for set-difference / post-filter.
		if paramMap == nil {
			paramMap = make(map[string]string)
		}
		paramMap["__sf_pattern_not"] = "1"
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
				value := y.VisitFileFilterContentMethodParamValue(pv)
				paramMap[key] = value
			}
		} else {
			value, ok := item.FileFilterContentMethodParamValue().(*sf.FileFilterContentMethodParamValueContext)
			if !ok {
				continue
			}
			res := y.VisitFileFilterContentMethodParamValue(value)
			paramList = append(paramList, res)
		}
	}
	return paramMap, paramList
}

func (y *SyntaxFlowVisitor) VisitFileFilterContentMethodParamValue(raw sf.IFileFilterContentMethodParamValueContext) (res string) {
	if y == nil || raw == nil {
		return ""
	}
	i, _ := raw.(*sf.FileFilterContentMethodParamValueContext)
	if i == nil {
		return ""
	}
	defer func() {
		if newRes, err := strconv.Unquote(res); err == nil {
			res = newRes
		}
	}()

	if nameFilter := i.NameFilter(); nameFilter != nil {
		name, ok := nameFilter.(*sf.NameFilterContext)
		if !ok {
			return ""
		}
		//regexp literal
		if reg, ok := name.RegexpLiteral().(*sf.RegexpLiteralContext); ok {
			reg := reg.GetText()
			reg = reg[1 : len(reg)-1]
			if !regexp_utils.NewYakRegexpUtils(reg).CanUse() {
				log.Errorf("regexp compile failed: %s", reg)
				return ""
			}
			return reg
		} else {
			return nameFilter.GetText()
		}
	}

	if i.HereDoc() != nil {
		return y.VisitHereDoc(i.HereDoc())
	}

	return ""
}
