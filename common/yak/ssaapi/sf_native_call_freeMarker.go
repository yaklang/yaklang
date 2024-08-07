package ssaapi

import (
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)


var (
	ftlVarExtractor = regexp.MustCompile(`\$\{([^?}]+)\}`)
	ftlSuffixExtractor = regexp.MustCompile(`spring.freemarker.suffix=(.*)`)
)

var nativeCallFreeMarker = sfvm.NativeCallFunc(func(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
	prog, err := fetchProgram(v)
	if err != nil {
		return false, nil, err
	}

	var ftls map[string][]string
	ftls = make(map[string][]string) // key is fileName,value is unsafe param
	// get suffix name
	var suffix string
	for name,data := range prog.Program.ExtraFile {
		if strings.HasSuffix(name,".properties") {
			if len(data) <= 128 {
				editor, _ := ssadb.GetIrSourceFromHash(data)
				if editor != nil {
					data = editor.GetSourceCode()
				}
			}
			if err != nil {
				log.Errorf("regexp compile error: %s", err)
				continue
			}
			matchs := ftlSuffixExtractor.FindStringSubmatch(data)					
			if len(matchs) > 1 {
				suffix = matchs[1]
				break
			}
		}
	}
	_=suffix
	for name,data := range prog.Program.ExtraFile {
		if strings.HasSuffix(name,".ftl") {
			if len(data) <= 128 {
				editor, _ := ssadb.GetIrSourceFromHash(data)
				if editor != nil {
					data = editor.GetSourceCode()
				}
			}
			matchs := ftlVarExtractor.FindAllStringSubmatch(data, -1)
			for _, match := range matchs {
				if len(match) >1{
					ftls[name] = append(ftls[name], match[1])
				}
			}
		}
	}
	
	return false, nil, nil
})