package java

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestScanSimple(t *testing.T) {
	code := `package com.example.A;
	public class A {
		public static void main(String[] args) {
			bb1;
			a = 1;
			if (c){
				a = 2;
			}else {
				a = 3;
			}
			bb2;
		}
	}
	`
	ssatest.Check(t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		result := prog.SyntaxFlowChain("bb2<scanPrevious> as $result").Show(false)
		assert.Equal(t, 6, result.Len())
		result = prog.SyntaxFlowChain("bb1<scanNext> as $result").Show(false)
		assert.Equal(t, 6, result.Len())
		return nil
	}, ssaapi.WithLanguage(ssaapi.JAVA))
}

func TestScanPreviousWithParam(t *testing.T) {
	code := `package com.example.A;
	public class A {
		public static void main(String[] args) {
			bb1;
			a = 1;
			if (c){
				a = 2;
			}else {
				a = 3;
			}
			bb2;
		}
	}
	`
	


	ssatest.Check(t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		result := prog.SyntaxFlowChain("bb2<scanPrevious(until=`*?{opcode: const}`)> as $result").Show(false)
		assert.Equal(t, 2, result.Len())

		result = prog.SyntaxFlowChain("bb2<scanPrevious(hook=`*?{opcode: const}`)> as $result").Show(false)
		assert.Equal(t, 3, result.Len())

		result = prog.SyntaxFlowChain("bb2<scanPrevious(exclude=`*?{opcode: const}`)> as $result",sfvm.WithEnableDebug(true)).Show(true)
		assert.Equal(t, 3, result.Len())
		return nil
	}, ssaapi.WithLanguage(ssaapi.JAVA))
}