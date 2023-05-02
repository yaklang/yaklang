package antlr4nasl

import (
	"strings"
	"testing"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

func TestPocScanner(t *testing.T) {
	engine := New()
	//engine.GetVirtualMachine().GetConfig().SetStopRecover(true)
	engine.Init()
	engine.SetIncludePath("/Users/z3/Downloads/mageni-master/src/backend/scanner/incs")
	engine.LoadScript("/Users/z3/Downloads/mageni-master/src/backend/scanner/plugins/gb_apache_struts_detect.nasl")
	//sourceCache := []string{}
	//engine.GetVirtualMachine().GetConfig().SetStopRecover(true)
	engine.AddSmokeOnCode(func(code *yakvm.Code) bool {
		//if strings.Contains(*code.SourceCodeFilePath, "http_keep") && code.StartLineNumber > 657 && code.StartLineNumber < 706 {
		if strings.Contains(*code.SourceCodeFilePath, "http_func") && code.StartLineNumber > 295 && code.StartLineNumber < 345 {
			//if len(sourceCache) == 0 {
			//	sourceCache = strings.Split(*code.SourceCodePointer, "\n")
			//}
			println(code.StartLineNumber)
		}
		return true
	})
	err := engine.Scan("91.213.164.221", "443")
	if err != nil {
		log.Error(err)
	}
}
