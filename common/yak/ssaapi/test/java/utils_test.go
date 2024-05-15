package java

import (
	"testing"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

type TestCase struct {
	Name    string
	Code    string
	Contain bool
	Expect  []string
}

func testExecTopDef(t *testing.T, tc *TestCase) {
	syntaxFlow := "Runtime.getRuntime().exec(*) #-> * as $target"
	log.Infof("TestExecTopDef code : %s", tc.Code)
	ssatest.CheckSyntaxFlowEx(t, tc.Code, syntaxFlow, tc.Contain,
		map[string][]string{
			"target": tc.Expect,
		},
		ssaapi.WithLanguage(ssaapi.JAVA),
	)
}

func testRequestTopDef(t *testing.T, tc *TestCase) {
	syntaxFlow := ".createDefault().execute(*) #-> * as $target"
	ssatest.CheckSyntaxFlowEx(t, tc.Code, syntaxFlow, tc.Contain, map[string][]string{
		"target": tc.Expect,
	}, ssaapi.WithLanguage(ssaapi.JAVA))
}
