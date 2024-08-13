package java

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestScanPreviousSimple(t *testing.T) {
	code := `package com.example.A;
	public class A {
		public static void main(String[] args) {
			a = 1+1;
			if (c){
				a = 2+2;
			}else {
				a = 3 + 3;
			}
			bb;
		}
	}
	`
	ssatest.Check(t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		result := prog.SyntaxFlowChain("bb<scanPrevious> as $result",sfvm.WithEnableDebug(true)).Show()
		assert.Equal(t, 5, result.Len())
		return nil
	}, ssaapi.WithLanguage(ssaapi.JAVA))
}