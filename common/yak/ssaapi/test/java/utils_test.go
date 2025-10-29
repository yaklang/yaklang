package java

import (
	"fmt"
	"testing"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

type TestCase struct {
	Name    string
	Code    string
	SF      string
	Contain bool
	Expect  map[string][]string
}

func test(t *testing.T, tc *TestCase) {
	code := fmt.Sprintf(`
	package com.example.utils;
		%s
	`, tc.Code)
	ssatest.CheckSyntaxFlowEx(
		t, code,
		tc.SF, tc.Contain, tc.Expect,
		ssaapi.WithLanguage(ssaconfig.JAVA),
	)

}

func testExecTopDef(t *testing.T, tc *TestCase) {
	syntaxFlow := "Runtime.getRuntime().exec(* #-> * as $target)"
	log.Infof("TestExecTopDef code : %s", tc.Code)
	ssatest.CheckSyntaxFlowEx(t, tc.Code, syntaxFlow, tc.Contain,
		tc.Expect,
		ssaapi.WithLanguage(ssaconfig.JAVA),
	)
}

func testRequestTopDef(t *testing.T, tc *TestCase) {
	syntaxFlow := ".createDefault().execute(* #-> * as $target)"
	ssatest.CheckSyntaxFlowEx(t, tc.Code, syntaxFlow, tc.Contain,
		tc.Expect,
		ssaapi.WithLanguage(ssaconfig.JAVA))
}
