package ssaapi

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils/memedit"
)

var (
	ftlVarExtractor    = regexp.MustCompile(`\$\{([^?}]+)\}!`)
	ftlSuffixExtractor = regexp.MustCompile(`spring.freemarker.suffix=(.*)`)
)

var nativeCallFreeMarker = sfvm.NativeCallFunc(func(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
	prog, err := fetchProgram(v)
	if err != nil {
		return false, nil, err
	}

	var ftls map[string][]string
	ftls = make(map[string][]string) // key is fileName,value is unsafe param
	var vals []sfvm.ValueOperator
	// get suffix name
	var suffix string
	prog.ForEachExtraFile(func(s string, me *memedit.MemEditor) bool {
		if strings.HasSuffix(s, ".properties") {
			matchs := ftlSuffixExtractor.FindStringSubmatch(me.GetSourceCode())
			if len(matchs) > 1 {
				suffix = matchs[1]
				return false
			}
		}
		return true
	})
	prog.ForEachExtraFile(func(s string, me *memedit.MemEditor) bool {
		matchs := ftlVarExtractor.FindAllStringSubmatch(me.GetSourceCode(), -1)
		for _, match := range matchs {
			if len(match) > 1 {
				ftls[s] = append(ftls[s], match[1])
			}
		}
		return true
	})

	v.Recursive(func(value sfvm.ValueOperator) error {
		valStr, err := strconv.Unquote(value.String())
		if err != nil {
			return nil
		}
		name := fmt.Sprintf("%s%s", valStr, suffix)
		for fileurl, match := range ftls {
			if strings.HasSuffix(fileurl, name) {
				for _, v := range match {
					vals = append(vals, prog.NewConstValue(v))
				}
			}
		}
		return nil
	})

	return true, sfvm.NewValues(vals), nil
})
