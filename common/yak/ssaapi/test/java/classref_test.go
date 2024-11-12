package java

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestClassRef(t *testing.T) {
	ssatest.CheckWithName("class-ref-basic-1", t, AnnotationBasic, func(prog *ssaapi.Program) error {
		if prog.SyntaxFlowChain("*EntryClass --> as $ret", ssaapi.QueryWithEnableDebug(true)).Show().Len() <= 0 {
			t.Fatal("*EntryClass --> $ret not found")
		}

		if prog.SyntaxFlowChain("*EntryClass.methodE* --> as $ret", ssaapi.QueryWithEnableDebug(true)).Show().Len() <= 0 {
			t.Fatal("*EntryClass.methodE* --> $ret not found")
		}

		return nil
	}, ssaapi.WithLanguage(ssaapi.JAVA))
}
